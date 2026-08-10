// Package mirror generates the .bat that copies source data into
// PHANTOM_SRC_DIR.
//
// The copy runs on the Windows side, not here. Reading tens of thousands of
// files through /mnt goes over 9p and is slow enough to be unusable, so the
// manager writes a robocopy script that the user runs from Explorer, exactly
// as the old manager did. What changed is that the script is now written from
// inside WSL while its contents remain Windows paths, and that the destination
// is PHANTOM_SRC_DIR reached over \\wsl.localhost\ rather than a directory
// inside the repository.
package mirror

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/text/encoding/japanese"

	"github.com/hyperion13th144m/phantom-manager/internal/wslenv"
)

// Patterns are the files worth copying out of the インターネット出願ソフト data.
//
// The first six are the old manager's and are business requirements: the
// electronic filing data proper. The last five were added for cendrillon,
// which can now index documents that have no XML left, only HTML and images.
//
// robocopy matches these case-insensitively, so lowercase .htm and .jpg are
// picked up too. *.HTM does not match .html — robocopy reads it as "extension
// is exactly HTM" — which is why *.HTML is listed separately.
var Patterns = []string{
	"*AAA.JWX", "*AAA.JPC", "*NNF.JWX", "*NNF.JPC", "*AFM.XML", "*NFM.XML",
	"*.HTM", "*.HTML", "*.GIF", "*.JPG", "*.PNG",
}

// threads is the /MT value. Sixteen is where the measured gain flattens out;
// /MT:32 was only marginally faster and asks more of a network share.
const threads = "16"

// Spec is everything the script needs, in Windows terms.
type Spec struct {
	Source   string // e.g. P:\jpodata
	Dest     string // e.g. \\wsl.localhost\Ubuntu-20.04\home\u\phantom\data\src
	Log      string // e.g. \\wsl.localhost\Ubuntu-20.04\home\u\phantom\mirror.log
	Patterns []string
}

// Result describes what was generated, in both namespaces, because the user
// has to find the file from Windows while the manager wrote it from Linux.
type Result struct {
	Path     string   `json:"path"`     // Linux path of the .bat
	UNC      string   `json:"unc"`      // the same file as Windows sees it
	Source   string   `json:"source"`   // Windows source directory
	Dest     string   `json:"dest"`     // Windows destination
	Log      string   `json:"log"`      // Windows log path
	Patterns []string `json:"patterns"` // what will be copied
}

// Generate writes the script for copying sourceWin into srcDir.
//
// sourceWin is a Windows path, which is the only form that works for every
// source: a mapped network drive has no /mnt equivalent to translate from.
// srcDir is the Linux PHANTOM_SRC_DIR the containers will read.
func Generate(batPath, sourceWin, srcDir string) (Result, error) {
	if strings.TrimSpace(sourceWin) == "" {
		return Result{}, fmt.Errorf("取込元フォルダを指定してください")
	}
	if strings.TrimSpace(srcDir) == "" {
		return Result{}, fmt.Errorf("取込先 (PHANTOM_SRC_DIR) が未設定です")
	}

	dest, err := windowsView(srcDir)
	if err != nil {
		return Result{}, err
	}
	logPath, err := windowsView(strings.TrimSuffix(batPath, filepath.Ext(batPath)) + ".log")
	if err != nil {
		return Result{}, err
	}
	batUNC, err := windowsView(batPath)
	if err != nil {
		return Result{}, err
	}

	spec := Spec{Source: sourceWin, Dest: dest, Log: logPath, Patterns: Patterns}
	encoded, err := Encode(Render(spec))
	if err != nil {
		return Result{}, err
	}
	if err := os.MkdirAll(filepath.Dir(batPath), 0o755); err != nil {
		return Result{}, fmt.Errorf("出力先を作成できませんでした: %w", err)
	}
	if err := os.WriteFile(batPath, encoded, 0o644); err != nil {
		return Result{}, fmt.Errorf("取込スクリプトを書き出せませんでした: %w", err)
	}

	return Result{
		Path:     batPath,
		UNC:      batUNC,
		Source:   NormalizePath(sourceWin),
		Dest:     spec.Dest,
		Log:      spec.Log,
		Patterns: spec.Patterns,
	}, nil
}

// windowsView renders a Linux path the way Windows reaches it. wslpath is
// asked first because it handles /mnt correctly — a PHANTOM_SRC_DIR under
// /mnt/d belongs to D:, not to \\wsl.localhost — and it answers for paths that
// do not exist yet. The computed UNC is the fallback for when interop is off.
func windowsView(linuxPath string) (string, error) {
	if w, err := wslenv.WindowsPath(linuxPath); err == nil && w != "" {
		return w, nil
	}
	if unc := wslenv.UNCPath(linuxPath); unc != "" {
		return unc, nil
	}
	return "", fmt.Errorf("%s を Windows から見たパスに変換できませんでした", linuxPath)
}

// Render builds the batch file. Lines are joined with CRLF, which is what
// cmd.exe expects.
func Render(spec Spec) string {
	patterns := spec.Patterns
	if len(patterns) == 0 {
		patterns = Patterns
	}
	quoted := make([]string, len(patterns))
	for i, p := range patterns {
		quoted[i] = `"` + p + `"`
	}

	lines := []string{
		"@echo off",
		"rem phantom-manager が生成した取込スクリプトです。",
		"rem 取込元から PHANTOM_SRC_DIR へ、必要な拡張子だけをコピーします。",
		"",
		`set "ORIG=` + NormalizePath(spec.Source) + `"`,
		`set "DATA_DIR=` + NormalizePath(spec.Dest) + `"`,
		"",
		// /E copies subdirectories including empty ones, as the old script did.
		// /MT:16 copies with sixteen threads instead of one. This is the single
		// biggest win in the command: the destination is reached over
		// \\wsl.localhost\, and every file crosses the 9p bridge between
		// Windows and WSL, which costs far more in per-file latency than in
		// bandwidth. Overlapping those waits took 2000 small files from 11.4s
		// to 2.8s when measured on this machine.
		// /R and /W matter more than they look: robocopy's default is a million
		// retries thirty seconds apart, so one locked file on a network share
		// stalls the whole run for days.
		// /NP drops the per-file percentage counter, which otherwise makes the
		// log unreadable and enormous.
		`robocopy "%ORIG%" "%DATA_DIR%" ` + strings.Join(quoted, " ") + ` /E /MT:` + threads + ` /R:2 /W:5 /NP /LOG:"` + spec.Log + `" /TEE`,
		"",
		"rem robocopy は正常時も 0 以外を返します（1=コピー実行, 2=余分なファイル,",
		"rem 4=不一致）。8 以上が本当の失敗なので、そこだけエラーとして扱います。",
		"if %ERRORLEVEL% GEQ 8 (",
		"  echo.",
		`  echo コピーに失敗しました。ログを確認してください: ` + spec.Log,
		"  pause",
		"  exit /b %ERRORLEVEL%",
		")",
		"echo.",
		"echo コピーが完了しました。",
		"pause",
		"exit /b 0",
		"",
	}
	return strings.Join(lines, "\r\n")
}

// Encode converts the script to Shift_JIS.
//
// This is not cosmetic. cmd.exe reads a .bat in the console's ANSI code page,
// which is CP932 on a Japanese Windows. A UTF-8 file with Japanese in it — a
// path, or the messages above — is read as mojibake, and a mangled path is a
// script that copies nothing.
func Encode(script string) ([]byte, error) {
	// The encoder rejects runes CP932 has no room for rather than silently
	// substituting them, so an unrepresentable path is reported instead of
	// being written as a broken script.
	b, err := japanese.ShiftJIS.NewEncoder().Bytes([]byte(script))
	if err != nil {
		return nil, fmt.Errorf("Shift_JIS に変換できない文字がパスに含まれています: %w", err)
	}
	return b, nil
}

// NormalizePath prepares a path for robocopy.
//
// A bare drive root confuses robocopy, which reads "D:\" as the drive's current
// directory; "D:\." names the root itself. Ported from the old
// NormalizeRobocopyPath. Trailing separators are dropped everywhere else,
// because "D:\dir\" would escape the closing quote in the generated command.
func NormalizePath(p string) string {
	t := strings.TrimSpace(p)
	if t == "" {
		return t
	}
	if isDriveRoot(t) {
		return strings.TrimRight(t, `\/`) + `\.`
	}
	if isUNCRoot(t) {
		return strings.TrimRight(t, `\/`)
	}
	return strings.TrimRight(t, `\/`)
}

// isDriveRoot matches "D:", "D:\" and "D:/".
func isDriveRoot(p string) bool {
	trimmed := strings.TrimRight(p, `\/`)
	return len(trimmed) == 2 && trimmed[1] == ':' && isDriveLetter(trimmed[0])
}

// isUNCRoot matches "\\server\share" with nothing after it.
func isUNCRoot(p string) bool {
	if !strings.HasPrefix(p, `\\`) {
		return false
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(p, `\\`), `\`), `\`)
	return len(parts) == 2
}

func isDriveLetter(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}
