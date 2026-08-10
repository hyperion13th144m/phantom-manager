// Package paths holds the default on-disk locations the manager works with.
// Ported from the old AppPaths.cs, but rooted at $HOME instead of the
// application directory: the manager now runs inside WSL as the same uid=1000
// user that the phantom containers run as.
package paths

import (
	"os"
	"path/filepath"
	"strings"
)

// Home returns the user's home directory, falling back to $HOME.
func Home() string {
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		return h
	}
	return os.Getenv("HOME")
}

// Expand resolves a leading "~" to the home directory. The old WslCommand did
// this by string replacement because it handed paths to a remote shell; here we
// resolve it in-process so no shell is involved.
func Expand(p string) string {
	if p == "~" {
		return Home()
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(Home(), p[2:])
	}
	return p
}

// PhantomDir is the root of everything the manager creates: ~/phantom.
func PhantomDir() string { return filepath.Join(Home(), "phantom") }

// DefaultReleaseDir is where phantom-release is cloned. Same location the old
// manager used, so an existing checkout is picked up as-is.
func DefaultReleaseDir() string { return filepath.Join(PhantomDir(), "phantom-release") }

// DefaultDataDir is PHANTOM_DATA_DIR: written by crow/queen/noir/violet/
// cendrillon, read by mona/panther.
func DefaultDataDir() string { return filepath.Join(PhantomDir(), "data") }

// DefaultSrcDir is PHANTOM_SRC_DIR: where the mirror script copies to.
func DefaultSrcDir() string { return filepath.Join(DefaultDataDir(), "src") }

// DefaultMirrorBat is where the generated robocopy script is written. It has to
// live somewhere reachable from Windows Explorer via \\wsl.localhost\<distro>\.
func DefaultMirrorBat() string { return filepath.Join(PhantomDir(), "mirror.bat") }

// ConfigFile stores the manager's own settings (chosen source directory etc.).
func ConfigFile() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "phantom-manager", "config.json")
	}
	return filepath.Join(Home(), ".config", "phantom-manager", "config.json")
}
