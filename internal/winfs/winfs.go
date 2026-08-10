// Package winfs inspects the Windows side of the machine through PowerShell.
//
// It exists because the manager has to talk about paths that Windows can see,
// not paths that WSL can see. The mirror script is a .bat run by robocopy in
// the user's Windows session, so the source directory has to be named in that
// session's namespace. /mnt is not that namespace: mapped network drives do not
// appear there at all (on the development machine P: -> \\192.168.11.250\
// patent-bi has no /mnt/p, and /mnt/j exists but is empty), so browsing /mnt
// would silently hide exactly the directories this shop copies from.
//
// Every call shells out to powershell.exe, which costs roughly half a second.
// That is fine for a directory picker and is why nothing here is on a hot path.
package winfs

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/hyperion13th144m/phantom-manager/internal/wslenv"
)

// preamble is prepended to every script.
//
//   - $ProgressPreference: without this, PowerShell writes CLIXML progress
//     records to stderr that turn up interleaved in any combined capture.
//   - OutputEncoding: without this, output comes back as CP932 and every
//     Japanese path is mojibake.
//
// Results are always fetched as JSON. Format-Table output is not just harder to
// parse, it truncates values to the terminal width — a UNC path read from a
// table can silently lose its tail.
const preamble = `$ProgressPreference='SilentlyContinue';[Console]::OutputEncoding=[System.Text.Encoding]::UTF8;`

// defaultTimeout bounds a single PowerShell call. A disconnected network drive
// can make Get-ChildItem hang until SMB gives up, and the UI must not hang with
// it.
const defaultTimeout = 30 * time.Second

// ErrInterop reports that Windows could not be reached at all, which is what
// happens when /etc/wsl.conf disables interop.
var ErrInterop = errors.New("Windows 側のコマンドを実行できませんでした（WSL interop が無効の可能性があります）")

// Drive is one entry of the Windows filesystem namespace.
type Drive struct {
	Name    string `json:"name"`          // "P"
	Root    string `json:"root"`          // "P:\\"
	UNC     string `json:"unc,omitempty"` // "\\\\192.168.11.250\\patent-bi" for mapped drives
	Network bool   `json:"network"`
}

// Entry is a subdirectory of some Windows directory.
type Entry struct {
	Name string `json:"name"`
	Path string `json:"path"` // full Windows path, e.g. P:\jpodata\raw
}

// Adapter is a network interface with a default gateway.
type Adapter struct {
	Alias       string `json:"alias"`
	IP          string `json:"ip"`
	Description string `json:"desc"`
}

// Client runs PowerShell scripts.
type Client struct {
	exe     string
	timeout time.Duration
}

// New returns a Client bound to the powershell.exe found on this machine.
func New() *Client {
	return &Client{exe: wslenv.PowerShellPath(), timeout: defaultTimeout}
}

// Drives lists local and mapped network drives. UNC is set only for mapped
// drives, where it is the share the letter points at.
func (c *Client) Drives(ctx context.Context) ([]Drive, error) {
	var res struct {
		Items []struct {
			Name string `json:"name"`
			Root string `json:"root"`
			UNC  string `json:"unc"`
		} `json:"items"`
	}
	script := `@{items=@(Get-PSDrive -PSProvider FileSystem | ForEach-Object { @{name=$_.Name; root=$_.Root; unc=$_.DisplayRoot} })} | ConvertTo-Json -Compress -Depth 5`
	if err := c.run(ctx, script, &res); err != nil {
		return nil, err
	}
	drives := make([]Drive, 0, len(res.Items))
	for _, it := range res.Items {
		drives = append(drives, Drive{
			Name:    it.Name,
			Root:    it.Root,
			UNC:     it.UNC,
			Network: it.UNC != "",
		})
	}
	return drives, nil
}

// ListDirs returns the subdirectories of a Windows directory. Files are left
// out: the picker only ever selects a directory to copy from.
func (c *Client) ListDirs(ctx context.Context, winPath string) ([]Entry, error) {
	if err := validatePath(winPath); err != nil {
		return nil, err
	}
	var res struct {
		Items []Entry `json:"items"`
	}
	// -LiteralPath, not -Path: bracket characters are legal in Windows
	// directory names and would otherwise be read as wildcards.
	// SilentlyContinue keeps an unreadable subdirectory from failing the whole
	// listing, which is common on network shares.
	script := fmt.Sprintf(
		`@{items=@(Get-ChildItem -LiteralPath %s -Directory -ErrorAction SilentlyContinue | Sort-Object Name | ForEach-Object { @{name=$_.Name; path=$_.FullName} })} | ConvertTo-Json -Compress -Depth 5`,
		psQuote(winPath))
	if err := c.run(ctx, script, &res); err != nil {
		return nil, err
	}
	return res.Items, nil
}

// Exists reports whether a Windows path is reachable from the user's session.
// This is the check that has to pass before a mirror script is generated: a
// path the manager cannot confirm is a path robocopy will fail on.
func (c *Client) Exists(ctx context.Context, winPath string) (bool, error) {
	if err := validatePath(winPath); err != nil {
		return false, err
	}
	var res struct {
		Items []bool `json:"items"`
	}
	script := fmt.Sprintf(`@{items=@([bool](Test-Path -LiteralPath %s -PathType Container))} | ConvertTo-Json -Compress`, psQuote(winPath))
	if err := c.run(ctx, script, &res); err != nil {
		return false, err
	}
	return len(res.Items) > 0 && res.Items[0], nil
}

// LanIPv4 returns the Windows host's LAN address, used as PHANTOM_PUBLIC_URL
// when phantom should be reachable from other machines.
//
// The old manager picked this by excluding adapters named "vEthernet" or
// described as "Hyper-V". Requiring a default gateway is a better test of the
// same idea — the WSL and Hyper-V switches have none — and it does not depend on
// how the adapters happen to be named.
func (c *Client) LanIPv4(ctx context.Context) ([]Adapter, error) {
	var res struct {
		Items []Adapter `json:"items"`
	}
	script := `@{items=@(Get-NetIPConfiguration -ErrorAction SilentlyContinue | Where-Object { $_.IPv4Address -and $_.IPv4DefaultGateway } | ForEach-Object { @{alias=$_.InterfaceAlias; ip=$_.IPv4Address[0].IPAddress; desc=$_.InterfaceDescription} })} | ConvertTo-Json -Compress -Depth 5`
	if err := c.run(ctx, script, &res); err != nil {
		return nil, err
	}
	out := make([]Adapter, 0, len(res.Items))
	for _, a := range res.Items {
		if a.IP == "" || strings.HasPrefix(a.IP, "169.254.") || a.IP == "127.0.0.1" {
			continue
		}
		out = append(out, a)
	}
	return out, nil
}

// Parent returns the directory containing a Windows path, or "" when the path
// is already a root. "" is what the picker uses to mean "show the drive list",
// so a drive root and a share root both climb back to it.
//
// This is string work rather than a PowerShell call: it happens on every click
// in the picker, and each call costs half a second.
func Parent(winPath string) string {
	p := strings.TrimRight(strings.TrimSpace(winPath), `\`)
	if p == "" {
		return ""
	}

	if strings.HasPrefix(p, `\\`) {
		// \\server\share is the shallowest a UNC path goes.
		parts := strings.Split(strings.TrimPrefix(p, `\\`), `\`)
		if len(parts) <= 2 {
			return ""
		}
		return `\\` + strings.Join(parts[:len(parts)-1], `\`)
	}

	i := strings.LastIndex(p, `\`)
	if i < 0 {
		return "" // a bare "C:" is already a root
	}
	head := p[:i]
	// "C:\dir" climbs to "C:\", not to "C:".
	if len(head) == 2 && head[1] == ':' {
		return head + `\`
	}
	return head
}

// run executes a script and decodes its JSON output.
func (c *Client) run(ctx context.Context, script string, out any) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	// -EncodedCommand sidesteps two layers of quoting at once: WSL's interop
	// rebuilds a Windows command line from argv using rules that do not match
	// Go's, and PowerShell then parses that line again. A base64 UTF-16LE blob
	// survives both untouched.
	cmd := exec.CommandContext(ctx, c.exe, "-NoProfile", "-NonInteractive", "-EncodedCommand", encodeCommand(preamble+script))

	// stdout only. PowerShell uses stderr for its own streams, and mixing them
	// in would corrupt the JSON.
	stdout, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			msg := strings.TrimSpace(string(ee.Stderr))
			if msg != "" {
				return fmt.Errorf("PowerShell: %s", firstLine(msg))
			}
			return fmt.Errorf("PowerShell が終了コード %d で失敗しました", ee.ExitCode())
		}
		if ctx.Err() != nil {
			return fmt.Errorf("PowerShell の実行がタイムアウトしました: %w", ctx.Err())
		}
		return fmt.Errorf("%w: %v", ErrInterop, err)
	}
	if err := json.Unmarshal(stdout, out); err != nil {
		return fmt.Errorf("PowerShell の出力を解釈できませんでした: %w", err)
	}
	return nil
}

// psQuote renders a string as a PowerShell single-quoted literal, where the
// only escape is a doubled quote. No expansion happens inside single quotes, so
// a path containing $ or ` is safe.
func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// validatePath rejects input that has no business in a path. Quoting already
// makes injection impossible; this catches malformed requests early and keeps
// a stray newline from splitting the script.
func validatePath(p string) error {
	if strings.TrimSpace(p) == "" {
		return errors.New("パスが空です")
	}
	if strings.ContainsAny(p, "\x00\r\n") {
		return errors.New("パスに使用できない文字が含まれています")
	}
	return nil
}

// encodeCommand renders a script the way -EncodedCommand expects it: UTF-16LE,
// then base64.
func encodeCommand(script string) string {
	units := utf16.Encode([]rune(script))
	b := make([]byte, len(units)*2)
	for i, u := range units {
		binary.LittleEndian.PutUint16(b[i*2:], u)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func firstLine(s string) string {
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return s[:i]
	}
	return s
}
