// Package envfile generates phantom-release's .env.docker.
//
// The new phantom-release reads its compose settings from .env.docker, passed
// explicitly as `docker compose --env-file .env.docker`. The old manager wrote
// a plain .env that compose picked up implicitly; that no longer works, and the
// variable names changed with it (SRC_DIR, a repository-relative path, became
// PHANTOM_SRC_DIR, an absolute one with no default).
//
// The old manager regenerated the file by copying the sample over it and
// running sed. That threw away anything the user had tuned — memory limits, the
// Elasticsearch password — every time the data directory was saved. Here the
// existing file is the base when there is one, and only the keys the manager
// owns are rewritten.
package envfile

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/hyperion13th144m/phantom-manager/internal/paths"
)

// File names inside the phantom-release checkout.
const (
	Name       = ".env.docker"
	SampleName = ".env.docker.sample"
)

// Managed keys. Everything else in the file is left exactly as found.
const (
	keySrcDir     = "PHANTOM_SRC_DIR"
	keyHTMLSrcDir = "PHANTOM_HTML_SRC_DIR"
	keyDataDir    = "PHANTOM_DATA_DIR"
	keyHTTPPort   = "PHANTOM_HTTP_PORT"
	keyPublicURL  = "PHANTOM_PUBLIC_URL"

	keyESJavaOpts = "ES_JAVA_OPTS"
	keyESMemLimit = "ES_MEM_LIMIT"
)

// Settings are the values the manager owns.
type Settings struct {
	SrcDir    string `json:"srcDir"`    // PHANTOM_SRC_DIR, the mirror script's destination
	DataDir   string `json:"dataDir"`   // PHANTOM_DATA_DIR, written by the pipeline
	HTTPPort  int    `json:"httpPort"`  // PHANTOM_HTTP_PORT, the only port published
	PublicURL string `json:"publicUrl"` // PHANTOM_PUBLIC_URL, used for joker -> fox links
}

// Defaults are the values for a WSL2 machine.
//
// The sample ships /mnt/disk/jpodata/src and /var/lib/phantom/data, which need
// `sudo install -d -o 1000 -g 1000` because the containers run as uid 1000.
// Under $HOME that is already true: the default WSL user is uid 1000, so the
// directories work without sudo.
func Defaults() Settings {
	return Settings{
		SrcDir:    paths.DefaultSrcDir(),
		DataDir:   paths.DefaultDataDir(),
		HTTPPort:  8080,
		PublicURL: "http://localhost:8080",
	}
}

// Path is the .env.docker location for a checkout.
func Path(releaseDir string) string { return filepath.Join(releaseDir, Name) }

// SamplePath is the .env.docker.sample location for a checkout.
func SamplePath(releaseDir string) string { return filepath.Join(releaseDir, SampleName) }

// Exists reports whether the checkout already has a generated file.
func Exists(releaseDir string) bool {
	_, err := os.Stat(Path(releaseDir))
	return err == nil
}

// Load reads the managed values out of an existing .env.docker, filling in
// defaults for anything absent. The second result reports whether a file was
// found.
func Load(releaseDir string) (Settings, bool, error) {
	s := Defaults()
	data, err := os.ReadFile(Path(releaseDir))
	if err != nil {
		if os.IsNotExist(err) {
			return s, false, nil
		}
		return s, false, err
	}
	values := parse(string(data))
	if v, ok := values[keySrcDir]; ok && v != "" {
		s.SrcDir = v
	}
	if v, ok := values[keyDataDir]; ok && v != "" {
		s.DataDir = v
	}
	if v, ok := values[keyHTTPPort]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			s.HTTPPort = n
		}
	}
	if v, ok := values[keyPublicURL]; ok && v != "" {
		s.PublicURL = v
	}
	return s, true, nil
}

// Validate checks the settings before anything is written or created.
func (s Settings) Validate() error {
	for _, d := range []struct {
		label string
		value string
	}{{"取込先", s.SrcDir}, {"展開先", s.DataDir}} {
		switch {
		case strings.TrimSpace(d.value) == "":
			return fmt.Errorf("%sディレクトリを指定してください", d.label)
		case !filepath.IsAbs(d.value):
			// compose resolves a relative path against its own working
			// directory, not the user's, so a relative value silently mounts
			// something inside the checkout.
			return fmt.Errorf("%sディレクトリは絶対パスで指定してください: %s", d.label, d.value)
		case strings.ContainsAny(d.value, "\r\n"):
			return fmt.Errorf("%sディレクトリに改行を含められません", d.label)
		}
	}
	if s.HTTPPort < 1 || s.HTTPPort > 65535 {
		return fmt.Errorf("公開ポートが範囲外です: %d", s.HTTPPort)
	}
	if strings.TrimSpace(s.PublicURL) == "" {
		return fmt.Errorf("公開 URL を指定してください")
	}
	if u, err := url.Parse(s.PublicURL); err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("公開 URL の形式が正しくありません: %s", s.PublicURL)
	}
	return nil
}

// EnsureDirs creates the bind mount targets.
//
// This has to happen before `compose up`. Docker creates a missing bind mount
// source itself, owned by root, and the containers then cannot write to it —
// the failure surfaces much later as a permission error inside a service. The
// old manager pre-created its directories for the same reason.
func (s Settings) EnsureDirs() error {
	for _, dir := range []string{s.SrcDir, s.DataDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("ディレクトリを作成できませんでした: %w", err)
		}
	}
	return nil
}

// Save writes .env.docker, using the existing file as the base when there is
// one and the sample otherwise.
func Save(releaseDir string, s Settings) error {
	if err := s.Validate(); err != nil {
		return err
	}
	base, fromSample, err := readBase(releaseDir)
	if err != nil {
		return err
	}
	if fromSample {
		base = applyMemoryProfile(base)
	}
	if err := s.EnsureDirs(); err != nil {
		return err
	}
	rendered := Render(base, s)
	return os.WriteFile(Path(releaseDir), []byte(rendered), 0o644)
}

// readBase returns the text to render over, and whether it came from the
// sample. Only a first generation may impose defaults; once the file exists,
// its values are the user's.
func readBase(releaseDir string) (string, bool, error) {
	if data, err := os.ReadFile(Path(releaseDir)); err == nil {
		return string(data), false, nil
	}
	data, err := os.ReadFile(SamplePath(releaseDir))
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, fmt.Errorf("%s が見つかりません。phantom-release を取得してください", SamplePath(releaseDir))
		}
		return "", false, err
	}
	return string(data), true, nil
}

// wsl2Memory sizes Elasticsearch for a WSL2 VM.
//
// The sample is written for a Linux host that can hand the containers all of
// its memory. WSL2 gives the VM half the machine by default, and INSTALL.md has
// it pinned to 10 GB on a 16 GB PC, of which the VM itself takes 1–1.5 GB. The
// sample's 2 GB heap inside a 4 GB limit does not leave enough of the remainder
// for violet, whose peak while loading CLIP is 4 GB and cannot be reduced
// — the pipeline is OOM killed partway through instead.
//
// These are the values phantom-release documents for this exact case in
// docs/production.md ("16 GB のホストでの回し方"), where the arithmetic is
// worked through stage by stage.
var wsl2Memory = map[string]string{
	keyESJavaOpts: "-Xms1g -Xmx1g",
	keyESMemLimit: "2g",
}

// applyMemoryProfile rewrites the sample's Elasticsearch sizing before the
// managed keys are rendered over it.
//
// This runs only on a first generation. Someone who has since raised the heap
// because they gave WSL2 more memory should keep it, and Save leaves an
// existing file's values alone for the same reason it preserves every other
// setting the manager does not own.
func applyMemoryProfile(sample string) string {
	seen := map[string]bool{}

	var out strings.Builder
	sc := bufio.NewScanner(strings.NewReader(sample))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		key := keyOf(line)
		v, ok := wsl2Memory[key]
		if !ok {
			out.WriteString(line + "\n")
			continue
		}
		if seen[key] {
			// A later duplicate would override what we just wrote.
			out.WriteString("# " + line + "\n")
			continue
		}
		seen[key] = true
		// The sample's value is kept as a comment: it is what a machine with
		// more memory to spare should go back to.
		out.WriteString("# phantom-manager: WSL2 に 10GB を割り当てた 16GB 機に合わせています\n")
		out.WriteString("# " + line + "\n")
		out.WriteString(key + "=" + v + "\n")
	}

	// A sample that never mentioned them still needs the values: compose falls
	// back to the compose file's own defaults otherwise, which are the large
	// ones this profile exists to avoid.
	var missing []string
	for _, k := range []string{keyESJavaOpts, keyESMemLimit} {
		if !seen[k] {
			missing = append(missing, k+"="+wsl2Memory[k])
		}
	}
	if len(missing) > 0 {
		if !strings.HasSuffix(out.String(), "\n\n") {
			out.WriteString("\n")
		}
		out.WriteString("# phantom-manager: WSL2 に 10GB を割り当てた 16GB 機に合わせています\n")
		out.WriteString(strings.Join(missing, "\n") + "\n")
	}
	return out.String()
}

// Render rewrites the managed keys in base, leaving comments, blank lines and
// every other setting untouched.
func Render(base string, s Settings) string {
	want := map[string]string{
		keySrcDir:    s.SrcDir,
		keyDataDir:   s.DataDir,
		keyHTTPPort:  strconv.Itoa(s.HTTPPort),
		keyPublicURL: s.PublicURL,
	}
	seen := map[string]bool{}

	var out strings.Builder
	sc := bufio.NewScanner(strings.NewReader(base))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		key := keyOf(line)

		// The sample sets PHANTOM_HTML_SRC_DIR to a real path. Left alone it
		// would make cendrillon mount a directory that does not exist here, and
		// docker would create it as root. Commenting it out makes compose fall
		// back to PHANTOM_SRC_DIR, which is where the mirror script puts the
		// HTML and images anyway.
		if key == keyHTMLSrcDir {
			if !seen[keyHTMLSrcDir] {
				seen[keyHTMLSrcDir] = true
				out.WriteString("# phantom-manager: HTML と画像も PHANTOM_SRC_DIR へまとめて取り込むため未設定にしています\n")
				out.WriteString("# " + line + "\n")
			}
			continue
		}

		if v, ok := want[key]; ok {
			if seen[key] {
				// A later duplicate would override the value we just wrote.
				out.WriteString("# " + line + "\n")
				continue
			}
			seen[key] = true
			out.WriteString(key + "=" + v + "\n")
			continue
		}
		out.WriteString(line + "\n")
	}

	// Anything the base never mentioned gets appended.
	var missing []string
	for _, k := range []string{keySrcDir, keyDataDir, keyHTTPPort, keyPublicURL} {
		if !seen[k] {
			missing = append(missing, k+"="+want[k])
		}
	}
	if len(missing) > 0 {
		if !strings.HasSuffix(out.String(), "\n\n") {
			out.WriteString("\n")
		}
		out.WriteString("# phantom-manager が追加した設定\n")
		out.WriteString(strings.Join(missing, "\n") + "\n")
	}
	return out.String()
}

// keyOf returns the variable a line assigns, or "" for comments and blanks.
func keyOf(line string) string {
	t := strings.TrimSpace(line)
	if t == "" || strings.HasPrefix(t, "#") {
		return ""
	}
	k, _, ok := strings.Cut(t, "=")
	if !ok {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(k, "export "))
}

// parse reads assignments, ignoring comments. Values are taken literally to end
// of line, with surrounding quotes stripped the way compose does.
func parse(text string) map[string]string {
	values := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		key := keyOf(line)
		if key == "" {
			continue
		}
		_, v, _ := strings.Cut(strings.TrimSpace(line), "=")
		values[key] = unquote(strings.TrimSpace(v))
	}
	return values
}

func unquote(v string) string {
	if len(v) >= 2 {
		if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
			return v[1 : len(v)-1]
		}
	}
	return v
}
