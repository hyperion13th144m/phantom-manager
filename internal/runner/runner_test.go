package runner

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestRunStreamsBothStreamsAndEchoesCommand(t *testing.T) {
	var lines []Line
	res, err := Run(context.Background(), "", "sh", []string{"-c", "echo out1; echo err1 >&2; echo out2"},
		func(l Line) { lines = append(lines, l) })
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.OK() {
		t.Fatalf("exit code = %d, want 0", res.ExitCode)
	}

	if len(lines) == 0 || lines[0].Kind != KindCmd {
		t.Fatalf("first line should echo the command, got %+v", lines)
	}
	if !strings.Contains(lines[0].Text, "echo out1") {
		t.Errorf("command line not echoed: %q", lines[0].Text)
	}

	got := map[string]string{}
	for _, l := range lines[1:] {
		got[l.Text] = l.Kind
	}
	if got["out1"] != KindOut || got["out2"] != KindOut {
		t.Errorf("stdout lines misclassified: %v", got)
	}
	if got["err1"] != KindErr {
		t.Errorf("stderr line misclassified: %v", got)
	}
	if !strings.Contains(res.Output, "out1") || !strings.Contains(res.Output, "err1") {
		t.Errorf("accumulated output missing lines: %q", res.Output)
	}
}

func TestRunReportsExitCodeWithoutError(t *testing.T) {
	res, err := Run(context.Background(), "", "sh", []string{"-c", "exit 3"}, nil)
	if err != nil {
		t.Fatalf("a non-zero exit must not be an error, got %v", err)
	}
	if res.ExitCode != 3 {
		t.Errorf("exit code = %d, want 3", res.ExitCode)
	}
}

func TestRunMissingExecutable(t *testing.T) {
	res, err := Run(context.Background(), "", "definitely-not-a-command-xyz", nil, nil)
	if err != nil {
		t.Fatalf("a missing executable is reported via exit code, got %v", err)
	}
	if res.ExitCode != ExitNotFound {
		t.Errorf("exit code = %d, want %d", res.ExitCode, ExitNotFound)
	}
}

// Long lines are routine in `docker compose build` output and used to truncate
// under bufio.Scanner's default 64 KiB limit.
func TestRunHandlesVeryLongLines(t *testing.T) {
	const n = 200000
	var got string
	_, err := Run(context.Background(), "", "sh",
		[]string{"-c", "printf 'x%.0s' $(seq 1 " + itoa(n) + "); echo"},
		func(l Line) {
			if l.Kind == KindOut {
				got = l.Text
			}
		})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(got) != n {
		t.Errorf("line length = %d, want %d", len(got), n)
	}
}

func TestRunHonoursContextCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := Run(ctx, "", "sh", []string{"-c", "sleep 10"}, nil)
	if err == nil {
		t.Fatal("expected a context error")
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("cancellation took %v, process was not killed", elapsed)
	}
}

func TestDisplayQuotesArgumentsThatNeedIt(t *testing.T) {
	got := Display("docker", []string{"compose", "--env-file", ".env.docker", "-f", "a b.yml"})
	want := "docker compose --env-file .env.docker -f 'a b.yml'"
	if got != want {
		t.Errorf("Display() = %q, want %q", got, want)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
