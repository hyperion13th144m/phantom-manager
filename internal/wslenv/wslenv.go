// Package wslenv covers the parts of the environment that only exist because we
// run inside WSL2: the distro name, path translation, and launching Windows
// programs through interop.
//
// Nothing here may be fatal. Interop can be switched off entirely via
// /etc/wsl.conf ([interop] enabled=false), and the manager still has to work —
// it just falls back to printing a path or URL for the user to copy.
package wslenv

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Absolute fallbacks for Windows programs. Looking these up on PATH is not
// enough: /etc/wsl.conf may set [interop] appendWindowsPath=false, in which case
// interop still works but no .exe is on PATH. That is the case on the machine
// this was developed against.
const (
	explorerPath   = "/mnt/c/Windows/explorer.exe"
	powerShellPath = `/mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe`
)

// IsWSL reports whether we are running inside WSL.
func IsWSL() bool {
	if os.Getenv("WSL_DISTRO_NAME") != "" {
		return true
	}
	b, err := os.ReadFile("/proc/sys/kernel/osrelease")
	return err == nil && strings.Contains(strings.ToLower(string(b)), "microsoft")
}

// DistroName returns the WSL distribution name as Windows knows it. This is the
// name that appears in \\wsl.localhost\<distro>\, and it is not necessarily the
// Ubuntu version: the development machine reports "Ubuntu-20.04" while actually
// running Ubuntu 24.04. The old manager hardcoded "Ubuntu-20.04", which is
// exactly the bug this avoids.
func DistroName() string {
	if n := os.Getenv("WSL_DISTRO_NAME"); n != "" {
		return n
	}
	// Fallback: \\wsl.localhost\<distro>\ is what wslpath reports for /.
	//
	// wslpath consults WSL_DISTRO_NAME itself, and an empty value breaks it
	// ("wslpath: /", exit 1) where an absent one works. Drop the variable so
	// the fallback is not defeated by the same emptiness that triggered it.
	if out, code := captureWithoutDistroName("wslpath", "-w", "/"); code == 0 {
		s := strings.Trim(strings.TrimSpace(out), `\`)
		if parts := strings.Split(s, `\`); len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}
	return ""
}

// UNCPath renders a Linux path the way Windows reaches it, as
// \\wsl.localhost\<distro>\home\user\.... Ported from the old manager's
// ToWslUncPath, with the distro no longer hardcoded.
//
// This is computed rather than shelled out to wslpath because it is used to
// build the mirror script's destination, which must be produced even if the
// path does not exist yet.
func UNCPath(linuxPath string) string {
	distro := DistroName()
	if distro == "" {
		return ""
	}
	rel := strings.ReplaceAll(strings.TrimPrefix(strings.TrimSpace(linuxPath), "/"), "/", `\`)
	return `\\wsl.localhost\` + distro + `\` + rel
}

// WindowsPath converts a Linux path to its Windows form via wslpath. For paths
// under /mnt this yields a drive path (/mnt/d/x -> D:\x); elsewhere it yields
// the \\wsl.localhost\ form.
func WindowsPath(linuxPath string) (string, error) {
	out, code := capture("wslpath", "-w", linuxPath)
	if code != 0 {
		return "", fmt.Errorf("wslpath -w %s に失敗しました", linuxPath)
	}
	return strings.TrimSpace(out), nil
}

// LinuxPath converts a Windows path to its WSL form via wslpath. It only
// succeeds for paths WSL actually has mounted, so a mapped network drive will
// fail here — which is why the mirror script is built from Windows paths
// directly instead of round-tripping through Linux ones.
func LinuxPath(winPath string) (string, error) {
	out, code := capture("wslpath", "-u", winPath)
	if code != 0 {
		return "", fmt.Errorf("wslpath -u %s に失敗しました", winPath)
	}
	return strings.TrimSpace(out), nil
}

// PowerShellPath returns the powershell.exe to invoke, preferring PATH and
// falling back to the absolute location.
func PowerShellPath() string {
	if p, err := exec.LookPath("powershell.exe"); err == nil {
		return p
	}
	return powerShellPath
}

// Open asks Windows to open a path or URL with its default handler. A failure
// is reported so the caller can fall back to showing the target instead.
func Open(target string) error {
	if p, err := exec.LookPath("wslview"); err == nil {
		if _, code := capture(p, target); code == 0 {
			return nil
		}
	}
	explorer := explorerPath
	if p, err := exec.LookPath("explorer.exe"); err == nil {
		explorer = p
	}
	// explorer.exe exits 1 even when it successfully opened the target, so the
	// exit code cannot be trusted here. Only a failure to start is an error.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, explorer, target)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// captureWithoutDistroName runs a command with WSL_DISTRO_NAME removed from
// the environment entirely.
func captureWithoutDistroName(name string, args ...string) (string, int) {
	env := make([]string, 0, len(os.Environ()))
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "WSL_DISTRO_NAME=") {
			continue
		}
		env = append(env, kv)
	}
	return captureEnv(env, name, args...)
}

func capture(name string, args ...string) (string, int) {
	return captureEnv(nil, name, args...)
}

func captureEnv(env []string, name string, args ...string) (string, int) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = env // nil means inherit
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if ok := asExitError(err, &ee); ok {
			return string(out), ee.ExitCode()
		}
		return string(out), -1
	}
	return string(out), 0
}

func asExitError(err error, target **exec.ExitError) bool {
	ee, ok := err.(*exec.ExitError)
	if ok {
		*target = ee
	}
	return ok
}
