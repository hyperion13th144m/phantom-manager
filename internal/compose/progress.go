package compose

import (
	"strings"

	"github.com/hyperion13th144m/phantom-manager/internal/runner"
)

// Compose writes its progress display to stderr — every "Image x Pulling",
// every "Container y Started" — and the log pane paints stderr red. The result
// was that a completely normal pull looked like a wall of errors, which is
// exactly the opposite of what the colour is for.
//
// So the lines compose emits to report work in progress are moved to the out
// stream, and everything else it writes to stderr is left alone. The matching
// is deliberately narrow: a status this code does not recognise stays red. If
// compose grows a new one, the worst case is a line that looks alarming, not a
// failure that goes unnoticed.

// progressResources are the object kinds compose names in a progress line.
var progressResources = map[string]bool{
	"Container": true,
	"Image":     true,
	"Network":   true,
	"Service":   true,
	"Volume":    true,
}

// progressStatuses are the statuses that mean work is happening or has
// finished. Statuses that report trouble — Error, Warning, Failed, Unhealthy,
// Exited — are deliberately absent, so they keep standing out.
var progressStatuses = map[string]bool{
	"Building":    true,
	"Built":       true,
	"Created":     true,
	"Creating":    true,
	"Downloaded":  true,
	"Downloading": true,
	"Extracting":  true,
	"Healthy":     true,
	"Pulled":      true,
	"Pulling":     true,
	"Pushed":      true,
	"Pushing":     true,
	"Recreate":    true,
	"Recreated":   true,
	"Removed":     true,
	"Removing":    true,
	"Restarted":   true,
	"Restarting":  true,
	"Running":     true,
	"Skipped":     true,
	"Started":     true,
	"Starting":    true,
	"Stopped":     true,
	"Stopping":    true,
	"Verifying":   true,
	"Waiting":     true,
}

// isProgress reports whether a line is one of compose's progress updates.
//
// The shape is "<resource> <name> <status>", indented by a space and padded to
// columns, e.g. " Image phantom-elasticsearch Built ". A status can run to
// several words ("Skipped - no build stage"), so only its first word is read.
func isProgress(text string) bool {
	fields := strings.Fields(text)
	if len(fields) < 3 {
		return false
	}
	return progressResources[fields[0]] && progressStatuses[fields[2]]
}

// demoteProgress wraps a log sink so compose's progress lines arrive as output
// rather than as errors. A nil sink is passed through unchanged.
func demoteProgress(log func(runner.Line)) func(runner.Line) {
	if log == nil {
		return nil
	}
	return func(line runner.Line) {
		if line.Kind == runner.KindErr && isProgress(line.Text) {
			line.Kind = runner.KindOut
		}
		log(line)
	}
}
