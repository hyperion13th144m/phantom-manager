// Package runner executes external commands and reports their output line by
// line. Ported from the old CommandRunner.cs, keeping the behaviour that made
// the old manager debuggable: the command itself is logged before it runs, and
// stdout and stderr are both streamed to the same sink.
//
// Unlike the old manager, nothing here goes through a shell. Arguments are
// passed as a slice to exec.Command, so there is no quoting to get wrong.
package runner

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Line kinds, matching the SSE event kinds the browser renders.
const (
	KindCmd = "cmd" // the command line itself, echoed before execution
	KindOut = "out"
	KindErr = "err"
)

// ExitNotFound is reported when the executable could not be started at all.
// The old manager used 9009 here (cmd.exe's "command not found"); keeping the
// value means the UI can special-case "not installed" the same way.
const ExitNotFound = 9009

// maxLineBytes bounds a single output line. `docker compose build` and `pull`
// emit progress lines far longer than bufio.Scanner's 64 KiB default, which
// would otherwise abort the scan mid-command.
const maxLineBytes = 1 << 20

// Line is one piece of command output.
type Line struct {
	Kind string
	Text string
}

// Result is the outcome of a finished command.
type Result struct {
	ExitCode int
	Output   string
}

// OK reports whether the command exited successfully.
func (r Result) OK() bool { return r.ExitCode == 0 }

// Display renders a command the way it is shown in the log pane. The old
// manager wrote "> wsl.exe -d Ubuntu-20.04 -- bash -lc ..."; we write the same
// shape for the process we actually run, so a user can copy the line out of the
// log and run it themselves.
func Display(name string, args []string) string {
	var b strings.Builder
	b.WriteString(name)
	for _, a := range args {
		b.WriteByte(' ')
		if a == "" || strings.ContainsAny(a, " \t\"'$&|<>*?()[]{}#;") {
			b.WriteString(quoteForDisplay(a))
			continue
		}
		b.WriteString(a)
	}
	return b.String()
}

// quoteForDisplay wraps a value in single quotes for display only. This is
// never fed back to a shell, so the escaping only needs to be readable.
func quoteForDisplay(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// killGrace bounds how long we wait for a killed process tree to release the
// output pipes before giving up on them.
const killGrace = 5 * time.Second

// Run executes a command, streaming every output line to onLine as it arrives.
// onLine is called from a single goroutine at a time, so callers do not need
// their own locking.
//
// It returns the exit code and the accumulated output. A non-zero exit is not
// an error: callers decide what a failure means. The returned error is only
// non-nil when the process could not be started or the context ended.
func Run(ctx context.Context, dir, name string, args []string, onLine func(Line)) (Result, error) {
	var (
		mu  sync.Mutex
		buf strings.Builder
	)
	// Both scanners and the caller's goroutine funnel through this, which is
	// what makes onLine safe to call from a plain non-thread-safe closure.
	emit := func(kind, text string) {
		mu.Lock()
		defer mu.Unlock()
		if kind != KindCmd {
			buf.WriteString(text)
			buf.WriteByte('\n')
		}
		if onLine != nil {
			onLine(Line{Kind: kind, Text: text})
		}
	}
	emit(KindCmd, Display(name, args))

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	// Run the child in its own process group and kill the whole group on
	// cancellation. Killing only the direct child is not enough: `sh -c` and
	// `docker compose` both leave grandchildren holding the output pipes open,
	// which would leave us blocked reading them long after the context ended.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
	cmd.WaitDelay = killGrace

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Result{ExitCode: ExitNotFound}, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return Result{ExitCode: ExitNotFound}, err
	}

	if err := cmd.Start(); err != nil {
		emit(KindErr, err.Error())
		return Result{ExitCode: ExitNotFound, Output: err.Error()}, nil
	}

	var wg sync.WaitGroup
	scan := func(r io.Reader, kind string) {
		defer wg.Done()
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 0, 64*1024), maxLineBytes)
		for sc.Scan() {
			emit(kind, strings.TrimRight(sc.Text(), "\r"))
		}
	}
	wg.Add(2)
	go scan(stdout, KindOut)
	go scan(stderr, KindErr)
	wg.Wait()

	exitCode := 0
	if err := cmd.Wait(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			exitCode = ee.ExitCode()
		} else {
			emit(KindErr, err.Error())
			exitCode = ExitNotFound
		}
	}
	if ctx.Err() != nil {
		return Result{ExitCode: exitCode, Output: buf.String()}, ctx.Err()
	}
	return Result{ExitCode: exitCode, Output: buf.String()}, nil
}

// Capture runs a command without logging it and returns its combined output.
// This is the quiet path used by the environment checks, where a failure is an
// expected answer ("docker is not installed") rather than something to show.
func Capture(ctx context.Context, dir, name string, args []string) (string, int) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return string(out), ee.ExitCode()
		}
		return string(out), ExitNotFound
	}
	return string(out), 0
}

// Errorf builds the error a caller reports when a command exits non-zero.
func Errorf(name string, args []string, r Result) error {
	return fmt.Errorf("%s は終了コード %d で失敗しました", Display(name, args), r.ExitCode)
}
