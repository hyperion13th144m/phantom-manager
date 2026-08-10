// Package envcheck answers "is this machine ready to run phantom?".
//
// It is the successor to the old manager's 環境チェック panel. Running inside
// WSL makes every check cheaper than before: no wsl.exe round trip, no distro
// discovery, no hunting for docker.exe under /mnt/c. What replaces that is a
// check the old manager never needed — whether Docker Desktop's WSL integration
// is switched on for this distro, which is the one failure that makes docker
// look installed while refusing to do anything.
package envcheck

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hyperion13th144m/phantom-manager/internal/runner"
	"github.com/hyperion13th144m/phantom-manager/internal/wslenv"
)

// Check states, rendered as ○ / ✕ / ! in the UI.
const (
	StateOK    = "ok"
	StateNG    = "ng"
	StateWarn  = "warn"
	StateUnset = "unset"
)

// Check is one row of the environment panel.
type Check struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	State  string `json:"state"`
	Detail string `json:"detail"`
	Hint   string `json:"hint,omitempty"` // what to do about it
}

// Result is the whole panel plus the facts the rest of the UI needs.
type Result struct {
	Checks     []Check `json:"checks"`
	Distro     string  `json:"distro"`
	ReleaseDir string  `json:"releaseDir"`
	EnvFile    string  `json:"envFile"`
	Ready      bool    `json:"ready"` // every required check passed
	CheckedAt  string  `json:"checkedAt"`
}

// probeTimeout bounds each external command. `docker info` against a stopped
// Docker Desktop can hang for a long time; the panel must still come back.
const probeTimeout = 20 * time.Second

// EnvFileName is the compose environment file the new phantom-release requires.
// The old manager wrote a plain .env, which compose no longer picks up.
const EnvFileName = ".env.docker"

// Run performs every check against the given phantom-release directory.
func Run(ctx context.Context, releaseDir string) Result {
	res := Result{
		Distro:     wslenv.DistroName(),
		ReleaseDir: releaseDir,
		EnvFile:    filepath.Join(releaseDir, EnvFileName),
		CheckedAt:  time.Now().Format(time.RFC3339),
	}
	res.Checks = []Check{
		checkWSL(res.Distro),
		checkDocker(ctx),
		checkCompose(ctx),
		checkGit(ctx),
		checkRelease(releaseDir),
		checkEnvFile(res.EnvFile),
	}

	res.Ready = true
	for _, c := range res.Checks {
		if c.State == StateNG {
			res.Ready = false
		}
	}
	return res
}

func checkWSL(distro string) Check {
	c := Check{ID: "wsl", Label: "WSL2"}
	if !wslenv.IsWSL() {
		c.State = StateNG
		c.Detail = "WSL 環境ではありません"
		c.Hint = "この manager は WSL2 の Ubuntu 内で実行してください。"
		return c
	}
	if distro == "" {
		c.State = StateWarn
		c.Detail = "ディストリ名を取得できませんでした"
		c.Hint = "$WSL_DISTRO_NAME が空です。取込スクリプトのコピー先 UNC を組み立てられません。"
		return c
	}
	c.State = StateOK
	c.Detail = distro
	return c
}

// checkDocker distinguishes three failures that look alike from the UI but need
// different fixes: docker missing entirely, Docker Desktop's WSL integration
// switched off for this distro, and the daemon simply not running.
func checkDocker(ctx context.Context) Check {
	c := Check{ID: "docker", Label: "Docker"}
	if _, err := exec.LookPath("docker"); err != nil {
		c.State = StateNG
		c.Detail = "docker コマンドが見つかりません"
		c.Hint = "Docker Desktop for Windows をインストールしてください。"
		return c
	}

	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	out, code := runner.Capture(ctx, "", "docker", []string{"version", "--format", "{{.Server.Version}}"})
	out = strings.TrimSpace(out)

	if code != 0 {
		c.State = StateNG
		// This exact message comes from the shim Docker Desktop leaves on PATH
		// when integration is off for the distro. Without special-casing it the
		// user sees "docker failed" and has no idea the fix is a checkbox.
		if strings.Contains(out, "could not be found in this WSL 2 distro") {
			c.Detail = "Docker Desktop の WSL 統合が無効です"
			c.Hint = fmt.Sprintf("Docker Desktop の Settings → Resources → WSL Integration で %s を有効にしてください。", wslenv.DistroName())
			return c
		}
		c.Detail = firstLine(out)
		c.Hint = "Docker Desktop が起動しているか確認してください。"
		return c
	}

	c.State = StateOK
	c.Detail = "Server " + out
	return c
}

func checkCompose(ctx context.Context) Check {
	c := Check{ID: "compose", Label: "Docker Compose"}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	out, code := runner.Capture(ctx, "", "docker", []string{"compose", "version", "--short"})
	out = strings.TrimSpace(out)
	if code != 0 {
		c.State = StateNG
		c.Detail = "docker compose が使えません"
		c.Hint = "Compose V2 が必要です。Docker Desktop を更新してください。"
		return c
	}
	c.State = StateOK
	c.Detail = "v" + strings.TrimPrefix(out, "v")
	return c
}

func checkGit(ctx context.Context) Check {
	c := Check{ID: "git", Label: "Git"}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	out, code := runner.Capture(ctx, "", "git", []string{"--version"})
	if code != 0 {
		c.State = StateNG
		c.Detail = "git コマンドが見つかりません"
		c.Hint = "sudo apt install git を実行してください。"
		return c
	}
	c.State = StateOK
	c.Detail = strings.TrimSpace(out)
	return c
}

// checkRelease is not fatal: a missing checkout is the normal state on a fresh
// machine, and cloning it is the first thing the user does here.
func checkRelease(dir string) Check {
	c := Check{ID: "release", Label: "phantom-release"}
	if st, err := os.Stat(filepath.Join(dir, ".git")); err == nil && st.IsDir() {
		c.State = StateOK
		c.Detail = dir
		return c
	}
	if st, err := os.Stat(dir); err == nil && st.IsDir() {
		c.State = StateWarn
		c.Detail = dir + " は Git リポジトリではありません"
		c.Hint = "別のディレクトリを指定するか、中身を空にしてから clone してください。"
		return c
	}
	c.State = StateUnset
	c.Detail = "未取得"
	c.Hint = "「クローン」で phantom-release を取得してください。"
	return c
}

func checkEnvFile(path string) Check {
	c := Check{ID: "env", Label: EnvFileName}
	if _, err := os.Stat(path); err == nil {
		c.State = StateOK
		c.Detail = path
		return c
	}
	c.State = StateUnset
	c.Detail = "未作成"
	c.Hint = "データディレクトリを設定して保存してください。"
	return c
}

func firstLine(s string) string {
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}
