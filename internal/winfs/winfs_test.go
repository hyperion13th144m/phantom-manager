package winfs

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/hyperion13th144m/phantom-manager/internal/wslenv"
)

func decodeCommand(t *testing.T, enc string) string {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		t.Fatalf("base64: %v", err)
	}
	if len(b)%2 != 0 {
		t.Fatalf("UTF-16LE payload has odd length %d", len(b))
	}
	units := make([]uint16, len(b)/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(b[i*2:])
	}
	return string(utf16.Decode(units))
}

func TestEncodeCommandRoundTrips(t *testing.T) {
	// Japanese text is the case that matters: it is what makes the encoding
	// choice visible, and mojibake here becomes mojibake in the picker.
	const script = `Get-ChildItem -LiteralPath 'P:\特許データ'`
	if got := decodeCommand(t, encodeCommand(script)); got != script {
		t.Errorf("round trip = %q, want %q", got, script)
	}
}

func TestPSQuoteDoublesSingleQuotes(t *testing.T) {
	cases := map[string]string{
		`P:\jpodata`:    `'P:\jpodata'`,
		`C:\a'b`:        `'C:\a''b'`,
		`C:\$env:TEMP`:  `'C:\$env:TEMP'`, // no expansion inside single quotes
		"C:\\back`tick": "'C:\\back`tick'",
	}
	for in, want := range cases {
		if got := psQuote(in); got != want {
			t.Errorf("psQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidatePathRejectsControlCharacters(t *testing.T) {
	for _, bad := range []string{"", "   ", "C:\\a\nB", "C:\\a\x00b", "C:\\a\rb"} {
		if err := validatePath(bad); err == nil {
			t.Errorf("validatePath(%q) = nil, want an error", bad)
		}
	}
	if err := validatePath(`P:\jpodata`); err != nil {
		t.Errorf("validatePath on a normal path: %v", err)
	}
}

func TestParent(t *testing.T) {
	cases := map[string]string{
		`C:\Windows\System32`:            `C:\Windows`,
		`C:\Windows`:                     `C:\`,
		`C:\`:                            ``, // a root climbs back to the drive list
		`C:`:                             ``,
		`P:\jpodata\raw`:                 `P:\jpodata`,
		`\\192.168.11.250\patent-bi\a\b`: `\\192.168.11.250\patent-bi\a`,
		`\\192.168.11.250\patent-bi\a`:   `\\192.168.11.250\patent-bi`,
		`\\192.168.11.250\patent-bi`:     ``, // a share root is as shallow as UNC goes
		``:                               ``,
	}
	for in, want := range cases {
		if got := Parent(in); got != want {
			t.Errorf("Parent(%q) = %q, want %q", in, got, want)
		}
	}
}

func requireWindows(t *testing.T) *Client {
	t.Helper()
	if !wslenv.IsWSL() {
		t.Skip("WSL 環境でのみ実行できます")
	}
	return New()
}

func TestDrivesListsTheSystemDrive(t *testing.T) {
	c := requireWindows(t)
	drives, err := c.Drives(context.Background())
	if err != nil {
		t.Fatalf("Drives: %v", err)
	}
	var sawC bool
	for _, d := range drives {
		if d.Name == "C" {
			sawC = true
			if d.Network {
				t.Errorf("C: reported as a network drive: %+v", d)
			}
		}
		// A mapped drive must carry the share it points at; that UNC is what
		// tells the user which server a letter refers to.
		if d.Network && d.UNC == "" {
			t.Errorf("network drive without a UNC: %+v", d)
		}
	}
	if !sawC {
		t.Errorf("C: missing from %+v", drives)
	}
}

func TestListDirsAndExists(t *testing.T) {
	c := requireWindows(t)
	ctx := context.Background()

	entries, err := c.ListDirs(ctx, `C:\`)
	if err != nil {
		t.Fatalf("ListDirs: %v", err)
	}
	var sawWindows bool
	for _, e := range entries {
		if strings.EqualFold(e.Name, "Windows") {
			sawWindows = true
			if e.Path != `C:\Windows` {
				t.Errorf("path = %q, want C:\\Windows", e.Path)
			}
		}
	}
	if !sawWindows {
		t.Errorf("C:\\Windows missing from %d entries", len(entries))
	}

	ok, err := c.Exists(ctx, `C:\Windows`)
	if err != nil || !ok {
		t.Errorf("Exists(C:\\Windows) = %v, %v; want true, nil", ok, err)
	}

	ok, err = c.Exists(ctx, `C:\this-directory-does-not-exist-xyz`)
	if err != nil {
		t.Errorf("a missing path is a false result, not an error: %v", err)
	}
	if ok {
		t.Error("Exists on a missing path = true")
	}
}

// A quote in the path must be answered, not turned into a script error.
func TestExistsHandlesQuotesInPath(t *testing.T) {
	c := requireWindows(t)
	ok, err := c.Exists(context.Background(), `C:\it's-not-here`)
	if err != nil {
		t.Fatalf("Exists with a quoted path: %v", err)
	}
	if ok {
		t.Error("Exists on a missing path = true")
	}
}

// Listing a directory that is not there must be an empty result, since the
// picker calls this on whatever the user typed.
func TestListDirsOnMissingPathIsEmpty(t *testing.T) {
	c := requireWindows(t)
	entries, err := c.ListDirs(context.Background(), `C:\no-such-directory-xyz`)
	if err != nil {
		t.Fatalf("ListDirs: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("entries = %+v, want none", entries)
	}
}
