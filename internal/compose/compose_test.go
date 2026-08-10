package compose

import (
	"strings"
	"testing"
)

// Real output from the Docker Desktop compose on this machine (v2.16.0): a
// single JSON array, no Ports field, Publishers duplicated per address family,
// and null for a service that publishes nothing.
const arrayOutput = `[{"ID":"dd343c27db11","Name":"ctest-a-1","Image":"alpine:3.20","Command":"sleep 300","Project":"ctest","Service":"a","Created":1786377430,"State":"running","Status":"Up Less than a second","Health":"","ExitCode":0,"Publishers":[{"URL":"0.0.0.0","TargetPort":80,"PublishedPort":18099,"Protocol":"tcp"},{"URL":"::","TargetPort":80,"PublishedPort":18099,"Protocol":"tcp"}]},{"ID":"1779207108d7","Name":"ctest-b-1","Image":"alpine:3.20","Command":"sleep 300","Project":"ctest","Service":"b","Created":1786377430,"State":"running","Status":"Up Less than a second","Health":"","ExitCode":0,"Publishers":null}]`

// What compose 2.21 and later print.
const linesOutput = `{"Name":"phantom-nginx-1","Image":"nginx:1.29-alpine","Service":"nginx","State":"running","Status":"Up 2 minutes","Health":"healthy","Ports":"0.0.0.0:8080->8080/tcp","Publishers":[{"URL":"0.0.0.0","TargetPort":8080,"PublishedPort":8080,"Protocol":"tcp"}]}
{"Name":"phantom-es-1","Image":"phantom-elasticsearch","Service":"es","State":"exited","Status":"Exited (137) 1 minute ago","Health":"","Ports":"","Publishers":null}`

// The old manager split on newlines and parsed each one, which on a single-line
// array yields nothing usable. Both shapes have to work.
func TestParsePsAcceptsAJSONArray(t *testing.T) {
	got, err := parsePs(arrayOutput)
	if err != nil {
		t.Fatalf("parsePs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d services, want 2", len(got))
	}
	if got[0].Name != "a" || !got[0].Running {
		t.Errorf("first service = %+v", got[0])
	}
	if got[0].Container != "ctest-a-1" {
		t.Errorf("container = %q, want ctest-a-1", got[0].Container)
	}
}

func TestParsePsAcceptsJSONLines(t *testing.T) {
	got, err := parsePs(linesOutput)
	if err != nil {
		t.Fatalf("parsePs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d services, want 2", len(got))
	}
	// Sorted by service name: es before nginx.
	if got[0].Name != "es" || got[1].Name != "nginx" {
		t.Errorf("services = %q, %q", got[0].Name, got[1].Name)
	}
	if got[0].Running {
		t.Error("an exited service is reported as running")
	}
	if got[1].Health != "healthy" {
		t.Errorf("health = %q, want healthy", got[1].Health)
	}
}

// Compose 2.16 reports no Ports field, so Publishers is the only source, and it
// lists the same mapping once for IPv4 and once for IPv6.
func TestFormatPortsCollapsesDuplicatePublishers(t *testing.T) {
	got, err := parsePs(arrayOutput)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Ports != "18099->80/tcp" {
		t.Errorf("ports = %q, want a single 18099->80/tcp", got[0].Ports)
	}
	if got[1].Ports != "" {
		t.Errorf("ports = %q, want empty for a service with none", got[1].Ports)
	}
}

// Newer compose fills Ports in directly; that string wins over rebuilding it.
func TestFormatPortsPrefersThePortsField(t *testing.T) {
	got, err := parsePs(linesOutput)
	if err != nil {
		t.Fatal(err)
	}
	if got[1].Ports != "0.0.0.0:8080->8080/tcp" {
		t.Errorf("ports = %q, want the Ports field verbatim", got[1].Ports)
	}
}

func TestParsePsHandlesNoContainers(t *testing.T) {
	for _, in := range []string{"", "  \n ", "[]"} {
		got, err := parsePs(in)
		if err != nil {
			t.Errorf("parsePs(%q): %v", in, err)
		}
		if len(got) != 0 {
			t.Errorf("parsePs(%q) = %+v, want none", in, got)
		}
	}
}

func TestParsePsRejectsGarbage(t *testing.T) {
	if _, err := parsePs("not json at all"); err == nil {
		t.Error("parsePs accepted garbage")
	}
}

// Every command must carry the env file: the new phantom-release has no
// defaults for PHANTOM_SRC_DIR or PHANTOM_DATA_DIR.
func TestArgsAlwaysPassTheEnvFile(t *testing.T) {
	got := strings.Join(New("/tmp/release").args("up", "-d"), " ")
	want := "compose --env-file .env.docker up -d"
	if got != want {
		t.Errorf("args = %q, want %q", got, want)
	}
}

func TestPreflightExplainsWhatIsMissing(t *testing.T) {
	err := New("/tmp/definitely-not-a-checkout-xyz").preflight()
	if err == nil {
		t.Fatal("preflight passed on a missing checkout")
	}
	if !strings.Contains(err.Error(), "phantom-release") {
		t.Errorf("error = %q, should name what is missing", err)
	}

	// A checkout with no .env.docker has to say so, since compose's own error
	// names a path relative to its working directory.
	dir := t.TempDir()
	err = New(dir).preflight()
	if err == nil || !strings.Contains(err.Error(), ".env.docker") {
		t.Errorf("error = %v, should name the missing env file", err)
	}
}
