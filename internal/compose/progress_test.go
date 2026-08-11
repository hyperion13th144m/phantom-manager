package compose

import (
	"testing"

	"github.com/hyperion13th144m/phantom-manager/internal/runner"
)

// Real stderr from compose 5.3.1 on this machine: `build es` and `pull`, which
// is where the red wall came from.
var composeProgress = []string{
	" Image phantom-elasticsearch Building ",
	" Image phantom-elasticsearch Built",
	" Image nginx:1.29-alpine Pulling ",
	" Image nginx:1.29-alpine Pulled ",
	" Image ghcr.io/hyperion13th144m/phantom-crow@sha256:4675b3a998fc8cbbfc7ea16a42da9faa5288ae1e59cfdb80219ccc968fdc17f2 Pulling ",
	" Container crow Starting",
	" Container crow Started",
	" Container es Waiting",
	" Container es Healthy",
	" Network phantom-network Created",
	" Volume phantom-es-data Created",
	" Container crow Removed",
}

func TestProgressLinesAreNotErrors(t *testing.T) {
	for _, line := range composeProgress {
		if !isProgress(line) {
			t.Errorf("isProgress(%q) = false, want true", line)
		}
	}
}

// Anything that reports trouble has to keep its colour, including the lines
// compose writes in the same "<resource> <name> <status>" shape.
func TestDiagnosticsStayErrors(t *testing.T) {
	for _, line := range []string{
		"Error response from daemon: pull access denied for phantom-elasticsearch",
		" Container crow Error",
		" Container crow Unhealthy",
		" Image es Failed",
		" Container crow Exited",
		"service \"es\" has neither an image nor a build context specified",
		"",
		"failed to solve: process did not complete successfully: exit code 1",
		// A shape that matches the grammar but names something compose does not.
		" Widget crow Started",
	} {
		if isProgress(line) {
			t.Errorf("isProgress(%q) = true, want false", line)
		}
	}
}

func TestDemoteProgressRewritesOnlyStderrProgress(t *testing.T) {
	var got []runner.Line
	sink := demoteProgress(func(l runner.Line) { got = append(got, l) })

	in := []runner.Line{
		{Kind: runner.KindCmd, Text: "docker compose pull"},
		{Kind: runner.KindErr, Text: " Image nginx:1.29-alpine Pulled "},
		{Kind: runner.KindErr, Text: "Error response from daemon: unauthorized"},
		// Buildkit writes to stdout; it must pass through untouched.
		{Kind: runner.KindOut, Text: "#5 [1/2] FROM docker.elastic.co/elasticsearch"},
	}
	for _, l := range in {
		sink(l)
	}

	want := []string{runner.KindCmd, runner.KindOut, runner.KindErr, runner.KindOut}
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d", len(got), len(want))
	}
	for i, kind := range want {
		if got[i].Kind != kind {
			t.Errorf("line %d (%q) kind = %s, want %s", i, got[i].Text, got[i].Kind, kind)
		}
		if got[i].Text != in[i].Text {
			t.Errorf("line %d text = %q, want %q", i, got[i].Text, in[i].Text)
		}
	}
}

func TestDemoteProgressPassesThroughNil(t *testing.T) {
	if demoteProgress(nil) != nil {
		t.Error("demoteProgress(nil) should stay nil so runner skips the callback")
	}
}
