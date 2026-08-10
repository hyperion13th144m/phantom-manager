package mirror

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/text/encoding/japanese"
)

func spec() Spec {
	return Spec{
		Source:   `P:\jpodata`,
		Dest:     `\\wsl.localhost\Ubuntu-20.04\home\yuichiro\phantom\data\src`,
		Log:      `\\wsl.localhost\Ubuntu-20.04\home\yuichiro\phantom\mirror.log`,
		Patterns: Patterns,
	}
}

func TestPatternsCoverTheFilingDataAndTheHTMLDocuments(t *testing.T) {
	// The first six are a business requirement carried over from the old
	// manager and must not drift.
	want := []string{"*AAA.JWX", "*AAA.JPC", "*NNF.JWX", "*NNF.JPC", "*AFM.XML", "*NFM.XML"}
	for i, p := range want {
		if Patterns[i] != p {
			t.Errorf("Patterns[%d] = %q, want %q", i, Patterns[i], p)
		}
	}
	// *.HTM does not match .html under robocopy, so both have to be listed.
	for _, p := range []string{"*.HTM", "*.HTML", "*.GIF", "*.JPG", "*.PNG"} {
		if !contains(Patterns, p) {
			t.Errorf("%s missing from Patterns", p)
		}
	}
	if len(Patterns) != 11 {
		t.Errorf("len(Patterns) = %d, want 11", len(Patterns))
	}
}

func TestRenderQuotesPathsAndPatterns(t *testing.T) {
	out := Render(spec())
	if !strings.Contains(out, `set "ORIG=P:\jpodata"`) {
		t.Errorf("source assignment missing:\n%s", out)
	}
	if !strings.Contains(out, `set "DATA_DIR=\\wsl.localhost\Ubuntu-20.04\home\yuichiro\phantom\data\src"`) {
		t.Errorf("destination assignment missing:\n%s", out)
	}
	for _, p := range Patterns {
		if !strings.Contains(out, `"`+p+`"`) {
			t.Errorf("pattern %s is not quoted in the command", p)
		}
	}
	if !strings.Contains(out, `/LOG:"`+spec().Log+`"`) {
		t.Errorf("log path is not quoted:\n%s", out)
	}
}

// cmd.exe wants CRLF.
func TestRenderUsesWindowsLineEndings(t *testing.T) {
	out := Render(spec())
	if strings.Contains(strings.ReplaceAll(out, "\r\n", ""), "\n") {
		t.Error("found a bare LF; every line must end CRLF")
	}
}

// robocopy's defaults are a million retries thirty seconds apart, which turns
// one locked file on a network share into a run that never ends.
func TestRenderLimitsRetries(t *testing.T) {
	out := Render(spec())
	for _, flag := range []string{"/E", "/MT:16", "/R:2", "/W:5", "/TEE"} {
		if !strings.Contains(out, flag) {
			t.Errorf("%s missing from the robocopy command:\n%s", flag, out)
		}
	}
}

// Every file crosses the 9p bridge to \\wsl.localhost, where per-file latency
// dominates. Copying single-threaded leaves that latency unoverlapped.
func TestRenderCopiesInParallel(t *testing.T) {
	out := Render(spec())
	if !strings.Contains(out, "/MT:") {
		t.Errorf("no /MT in the robocopy command:\n%s", out)
	}
	// /MT is incompatible with /IPG and /EFSRAW; neither may creep in.
	for _, bad := range []string{"/IPG", "/EFSRAW"} {
		if strings.Contains(out, bad) {
			t.Errorf("%s cannot be combined with /MT", bad)
		}
	}
}

// robocopy reports success with non-zero codes: 1 means files were copied.
// Passing that through would make every successful run look like a failure.
func TestRenderTreatsRobocopySuccessCodesAsSuccess(t *testing.T) {
	out := Render(spec())
	if !strings.Contains(out, "if %ERRORLEVEL% GEQ 8") {
		t.Errorf("exit code handling missing:\n%s", out)
	}
	if !strings.Contains(out, "exit /b 0") {
		t.Errorf("no successful exit path:\n%s", out)
	}
}

// A UTF-8 .bat is read by cmd.exe as CP932 and every Japanese path in it turns
// to mojibake, so the encoding is load-bearing.
func TestEncodeProducesShiftJIS(t *testing.T) {
	s := spec()
	s.Source = `P:\特許データ`
	encoded, err := Encode(Render(s))
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if strings.Contains(string(encoded), "特許データ") {
		t.Error("output is still UTF-8")
	}

	back, err := japanese.ShiftJIS.NewDecoder().Bytes(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(string(back), `set "ORIG=P:\特許データ"`) {
		t.Errorf("round trip lost the path:\n%s", back)
	}
}

// Silently writing a broken path would produce a script that copies nothing.
func TestEncodeRejectsCharactersShiftJISCannotHold(t *testing.T) {
	s := spec()
	s.Source = `P:\🙂`
	if _, err := Encode(Render(s)); err == nil {
		t.Error("Encode accepted a character Shift_JIS cannot represent")
	}
}

func TestNormalizePath(t *testing.T) {
	cases := map[string]string{
		// A bare drive root means "the current directory on D:" to robocopy;
		// "D:\." names the root itself.
		`D:\`:                     `D:\.`,
		`D:`:                      `D:\.`,
		`  P:\  `:                 `P:\.`,
		`D:\jpodata`:              `D:\jpodata`,
		`D:\jpodata\`:             `D:\jpodata`,
		`\\server\share`:          `\\server\share`,
		`\\server\share\`:         `\\server\share`,
		`\\server\share\jpodata\`: `\\server\share\jpodata`,
		``:                        ``,
	}
	for in, want := range cases {
		if got := NormalizePath(in); got != want {
			t.Errorf("NormalizePath(%q) = %q, want %q", in, got, want)
		}
	}
}

// A trailing separator would escape the closing quote in set "ORIG=...".
func TestNormalizePathKeepsQuotingIntact(t *testing.T) {
	s := spec()
	s.Source = `P:\jpodata\`
	if strings.Contains(Render(s), `set "ORIG=P:\jpodata\"`) {
		t.Error("a trailing backslash escaped the closing quote")
	}
}

func TestGenerateWritesTheScript(t *testing.T) {
	batPath := filepath.Join(t.TempDir(), "mirror.bat")
	res, err := Generate(batPath, `P:\jpodata`, "/home/yuichiro/phantom/data/src")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if res.Path != batPath {
		t.Errorf("path = %q, want %q", res.Path, batPath)
	}
	// The user has to find this file from Explorer, so a Windows-side name for
	// it is part of the result.
	if res.UNC == "" {
		t.Error("no Windows path for the generated script")
	}
	if len(res.Patterns) != len(Patterns) {
		t.Errorf("patterns = %v", res.Patterns)
	}

	raw, err := os.ReadFile(batPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(raw), "@echo off") {
		t.Errorf("unexpected script start: %.40q", raw)
	}
	// The log sits beside the script rather than inside the copied tree.
	if !strings.HasSuffix(strings.ToLower(res.Log), "mirror.log") {
		t.Errorf("log = %q, want it beside the script", res.Log)
	}
}

func TestGenerateRejectsMissingInput(t *testing.T) {
	batPath := filepath.Join(t.TempDir(), "mirror.bat")
	if _, err := Generate(batPath, "  ", "/home/u/src"); err == nil {
		t.Error("Generate accepted an empty source")
	}
	if _, err := Generate(batPath, `P:\jpodata`, ""); err == nil {
		t.Error("Generate accepted an empty destination")
	}
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
