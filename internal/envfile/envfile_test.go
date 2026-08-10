package envfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sample mirrors the shape of the real .env.docker.sample: comments, tuned
// values the manager must not touch, and PHANTOM_HTML_SRC_DIR set to a path
// that does not exist on a WSL machine.
const sample = `# docker-compose.yml（本番）用の設定サンプル

# インターネット出願ソフトの電子データ
PHANTOM_SRC_DIR=/mnt/disk/jpodata/src

# 未設定なら PHANTOM_SRC_DIR を見る
PHANTOM_HTML_SRC_DIR=/mnt/disk/jpodata/html-src

# 展開先
PHANTOM_DATA_DIR=/var/lib/phantom/data

PHANTOM_HTTP_PORT=8080
PHANTOM_PUBLIC_URL=http://localhost:8080

ES_PASSWORD=changeme
ES_JAVA_OPTS=-Xms2g -Xmx2g
# VIOLET_MEM_LIMIT=6g
`

func settings(t *testing.T) Settings {
	t.Helper()
	root := t.TempDir()
	return Settings{
		SrcDir:    filepath.Join(root, "data", "src"),
		DataDir:   filepath.Join(root, "data"),
		HTTPPort:  8080,
		PublicURL: "http://localhost:8080",
	}
}

func TestRenderReplacesOnlyTheManagedKeys(t *testing.T) {
	s := settings(t)
	got := parse(Render(sample, s))

	if got[keySrcDir] != s.SrcDir {
		t.Errorf("%s = %q, want %q", keySrcDir, got[keySrcDir], s.SrcDir)
	}
	if got[keyDataDir] != s.DataDir {
		t.Errorf("%s = %q, want %q", keyDataDir, got[keyDataDir], s.DataDir)
	}
	// Tuning the manager does not own has to survive.
	if got["ES_PASSWORD"] != "changeme" {
		t.Errorf("ES_PASSWORD = %q, want it untouched", got["ES_PASSWORD"])
	}
	if got["ES_JAVA_OPTS"] != "-Xms2g -Xmx2g" {
		t.Errorf("ES_JAVA_OPTS = %q, want it untouched", got["ES_JAVA_OPTS"])
	}
}

// The sample's value would make cendrillon mount a directory that does not
// exist, which docker then creates as root.
func TestRenderCommentsOutHTMLSrcDir(t *testing.T) {
	out := Render(sample, settings(t))
	if v, ok := parse(out)[keyHTMLSrcDir]; ok {
		t.Errorf("%s is still set to %q, want it commented out", keyHTMLSrcDir, v)
	}
	if !strings.Contains(out, "# PHANTOM_HTML_SRC_DIR=/mnt/disk/jpodata/html-src") {
		t.Errorf("the original line should be kept as a comment:\n%s", out)
	}
}

func TestRenderPreservesCommentsAndOrder(t *testing.T) {
	out := Render(sample, settings(t))
	for _, want := range []string{
		"# docker-compose.yml（本番）用の設定サンプル",
		"# 展開先",
		"# VIOLET_MEM_LIMIT=6g",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("comment %q was lost", want)
		}
	}
	if strings.Index(out, keySrcDir+"=") > strings.Index(out, keyDataDir+"=") {
		t.Error("key order changed")
	}
}

func TestRenderAppendsKeysMissingFromTheBase(t *testing.T) {
	s := settings(t)
	out := Render("# nothing here\n", s)
	got := parse(out)
	for _, k := range []string{keySrcDir, keyDataDir, keyHTTPPort, keyPublicURL} {
		if got[k] == "" {
			t.Errorf("%s missing from:\n%s", k, out)
		}
	}
}

// A duplicate assignment later in the file would win over the one we wrote.
func TestRenderNeutralisesDuplicateKeys(t *testing.T) {
	base := "PHANTOM_DATA_DIR=/first\nES_PASSWORD=x\nPHANTOM_DATA_DIR=/second\n"
	s := settings(t)
	out := Render(base, s)

	if got := parse(out)[keyDataDir]; got != s.DataDir {
		t.Errorf("%s = %q, want %q", keyDataDir, got, s.DataDir)
	}
	if !strings.Contains(out, "# PHANTOM_DATA_DIR=/second") {
		t.Errorf("the duplicate should be commented out:\n%s", out)
	}
}

// Saving twice must be stable: the second save reads back its own output.
func TestSaveIsIdempotentAndKeepsUserEdits(t *testing.T) {
	release := t.TempDir()
	if err := os.WriteFile(SamplePath(release), []byte(sample), 0o644); err != nil {
		t.Fatal(err)
	}
	s := settings(t)
	if err := Save(release, s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Stand in for the user tuning something by hand afterwards.
	data, err := os.ReadFile(Path(release))
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(data), "ES_PASSWORD=changeme", "ES_PASSWORD=secret", 1)
	if err := os.WriteFile(Path(release), []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}

	s.HTTPPort = 9090
	if err := Save(release, s); err != nil {
		t.Fatalf("second Save: %v", err)
	}
	got := parse(readFile(t, Path(release)))
	if got["ES_PASSWORD"] != "secret" {
		t.Errorf("ES_PASSWORD = %q, the manual edit was lost", got["ES_PASSWORD"])
	}
	if got[keyHTTPPort] != "9090" {
		t.Errorf("%s = %q, want 9090", keyHTTPPort, got[keyHTTPPort])
	}
	if v, ok := got[keyHTMLSrcDir]; ok {
		t.Errorf("%s reappeared as %q on the second save", keyHTMLSrcDir, v)
	}
}

func TestSaveCreatesTheBindMountTargets(t *testing.T) {
	release := t.TempDir()
	if err := os.WriteFile(SamplePath(release), []byte(sample), 0o644); err != nil {
		t.Fatal(err)
	}
	s := settings(t)
	if err := Save(release, s); err != nil {
		t.Fatalf("Save: %v", err)
	}
	for _, dir := range []string{s.SrcDir, s.DataDir} {
		st, err := os.Stat(dir)
		if err != nil || !st.IsDir() {
			t.Errorf("%s was not created: %v", dir, err)
		}
	}
}

func TestSaveWithoutASampleExplainsWhy(t *testing.T) {
	err := Save(t.TempDir(), settings(t))
	if err == nil {
		t.Fatal("Save without a sample succeeded")
	}
	if !strings.Contains(err.Error(), SampleName) {
		t.Errorf("error = %q, should name the missing sample", err)
	}
}

func TestLoadReadsBackWhatWasSaved(t *testing.T) {
	release := t.TempDir()
	if err := os.WriteFile(SamplePath(release), []byte(sample), 0o644); err != nil {
		t.Fatal(err)
	}
	want := settings(t)
	want.HTTPPort = 18080
	want.PublicURL = "http://192.168.11.6:18080"
	if err := Save(release, want); err != nil {
		t.Fatal(err)
	}

	got, found, err := Load(release)
	if err != nil || !found {
		t.Fatalf("Load: %v, found=%v", err, found)
	}
	if got != want {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}
}

func TestLoadFallsBackToDefaults(t *testing.T) {
	got, found, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if found {
		t.Error("found = true with no file present")
	}
	if got != Defaults() {
		t.Errorf("Load() = %+v, want defaults %+v", got, Defaults())
	}
}

func TestValidate(t *testing.T) {
	ok := settings(t)
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid settings rejected: %v", err)
	}

	cases := map[string]func(Settings) Settings{
		"relative src dir":  func(s Settings) Settings { s.SrcDir = "data/src"; return s },
		"relative data dir": func(s Settings) Settings { s.DataDir = "./data"; return s },
		"empty src dir":     func(s Settings) Settings { s.SrcDir = "  "; return s },
		"port zero":         func(s Settings) Settings { s.HTTPPort = 0; return s },
		"port too large":    func(s Settings) Settings { s.HTTPPort = 70000; return s },
		"empty url":         func(s Settings) Settings { s.PublicURL = ""; return s },
		"url without host":  func(s Settings) Settings { s.PublicURL = "localhost:8080"; return s },
		"newline in path":   func(s Settings) Settings { s.DataDir = "/tmp/a\nb"; return s },
	}
	for name, mutate := range cases {
		if err := mutate(ok).Validate(); err == nil {
			t.Errorf("%s: Validate() = nil, want an error", name)
		}
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
