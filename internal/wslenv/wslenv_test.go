package wslenv

import (
	"os"
	"strings"
	"testing"
)

func TestUNCPathBuildsTheWindowsView(t *testing.T) {
	// DistroName reads the environment, so pin it rather than depending on the
	// machine the test runs on.
	t.Setenv("WSL_DISTRO_NAME", "Ubuntu-20.04")

	cases := map[string]string{
		"/home/yuichiro/phantom/data/src": `\\wsl.localhost\Ubuntu-20.04\home\yuichiro\phantom\data\src`,
		"/":                               `\\wsl.localhost\Ubuntu-20.04\`,
		"  /home/u  ":                     `\\wsl.localhost\Ubuntu-20.04\home\u`,
	}
	for in, want := range cases {
		if got := UNCPath(in); got != want {
			t.Errorf("UNCPath(%q) = %q, want %q", in, got, want)
		}
	}
}

// The distro name is whatever Windows registered, which need not match the
// Ubuntu release inside it. Hardcoding it was a bug in the old manager.
func TestDistroNameComesFromTheEnvironment(t *testing.T) {
	t.Setenv("WSL_DISTRO_NAME", "Ubuntu-22.04")
	if got := DistroName(); got != "Ubuntu-22.04" {
		t.Errorf("DistroName() = %q, want Ubuntu-22.04", got)
	}
}

// Without the environment variable, DistroName falls back to parsing wslpath.
// Both an empty and an absent value have to work: wslpath itself reads the same
// variable and refuses to run when it is set but empty.
func TestDistroNameFallsBackToWslpath(t *testing.T) {
	if !IsWSL() {
		t.Skip("fallback path needs a real WSL environment")
	}
	want := os.Getenv("WSL_DISTRO_NAME")
	if want == "" {
		t.Skip("WSL_DISTRO_NAME is not set, nothing to compare against")
	}

	t.Run("empty", func(t *testing.T) {
		t.Setenv("WSL_DISTRO_NAME", "")
		if got := DistroName(); got != want {
			t.Errorf("DistroName() = %q, want %q", got, want)
		}
	})

	t.Run("absent", func(t *testing.T) {
		if err := os.Unsetenv("WSL_DISTRO_NAME"); err != nil {
			t.Fatalf("Unsetenv: %v", err)
		}
		t.Cleanup(func() { os.Setenv("WSL_DISTRO_NAME", want) })
		if got := DistroName(); got != want {
			t.Errorf("DistroName() = %q, want %q", got, want)
		}
	})
}

func TestWindowsPathTranslatesMountedDrives(t *testing.T) {
	if !IsWSL() {
		t.Skip("WSL 環境でのみ実行できます")
	}
	if _, err := os.Stat("/mnt/c"); err != nil {
		t.Skip("/mnt/c is not mounted")
	}
	got, err := WindowsPath("/mnt/c")
	if err != nil {
		t.Fatalf("WindowsPath: %v", err)
	}
	if !strings.HasPrefix(strings.ToUpper(got), "C:") {
		t.Errorf("WindowsPath(/mnt/c) = %q, want a C: path", got)
	}
}

func TestLinuxPathTranslatesBack(t *testing.T) {
	if !IsWSL() {
		t.Skip("WSL 環境でのみ実行できます")
	}
	got, err := LinuxPath(`C:\`)
	if err != nil {
		t.Fatalf("LinuxPath: %v", err)
	}
	if !strings.HasPrefix(got, "/mnt/c") {
		t.Errorf("LinuxPath(C:\\) = %q, want /mnt/c", got)
	}
}
