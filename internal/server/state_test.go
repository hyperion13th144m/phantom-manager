package server

import "testing"

// ready is the steady state: a real checkout, settings saved, nothing running.
func ready() Facts {
	return Facts{
		RepoExists:   true,
		RepoReady:    true,
		ProjectReady: true,
		EnvTemplate:  true,
		EnvExists:    true,
	}
}

func mustAllow(t *testing.T, f Facts, actions ...Action) {
	t.Helper()
	can := capabilities(f)
	for _, a := range actions {
		if !can[a].Allowed {
			t.Errorf("%s should be allowed for %+v (reason given: %q)", a, f, can[a].Reason)
		}
	}
}

func mustRefuse(t *testing.T, f Facts, actions ...Action) {
	t.Helper()
	can := capabilities(f)
	for _, a := range actions {
		if can[a].Allowed {
			t.Errorf("%s should be refused for %+v", a, f)
		}
		// A dead button with no explanation is the thing this replaces.
		if can[a].Reason == "" {
			t.Errorf("%s is refused without a reason", a)
		}
	}
}

// A running job takes everything out of play. This is the rule the old
// SetBusy existed for.
func TestNothingIsAllowedWhileBusy(t *testing.T) {
	f := ready()
	f.Busy = true
	can := capabilities(f)
	for action, c := range can {
		if c.Allowed {
			t.Errorf("%s is allowed while a job runs", action)
		}
	}
}

// Editing .env.docker or moving the checkout while services run leaves the
// manager's view and the containers' mounts disagreeing.
func TestRunningServicesLockSettingsAndCheckout(t *testing.T) {
	f := ready()
	f.ServicesRunning = true
	mustRefuse(t, f, ActionSaveEnv, ActionPull, ActionFetch, ActionCheckout, ActionUnpin, ActionUp, ActionMirrorScript)
	mustAllow(t, f, ActionDown)
}

// Fetching and building images does not touch a running project; nothing picks
// the new images up until the next up.
func TestBuildAndPullAreAllowedWhileServicesRun(t *testing.T) {
	f := ready()
	f.ServicesRunning = true
	mustAllow(t, f, ActionBuild, ActionPull2)
}

func TestDownNeedsSomethingRunning(t *testing.T) {
	mustRefuse(t, ready(), ActionDown)
}

// Refusing before the transfer beats git failing after it.
func TestCloneOnlyWhenTheDirectoryIsAbsent(t *testing.T) {
	mustRefuse(t, ready(), ActionClone)
	mustAllow(t, Facts{}, ActionClone)
}

// Down is the way out of a bad state, so it must not be gated on the same
// conditions that might have gone wrong. Under the old manager's combined
// repoReady rule, a checkout that stopped looking like a git repository took
// away the only button that could stop the containers it had started.
func TestDownStaysAvailableWhenTheCheckoutIsUnusable(t *testing.T) {
	f := Facts{RepoExists: true, ServicesRunning: true} // no .git, no compose file, no env
	mustAllow(t, f, ActionDown)
	mustRefuse(t, f, ActionUp, ActionBuild, ActionPull2)
}

// Compose needs a compose file; git needs a checkout. Conflating the two is
// what created the trap above.
func TestComposeNeedsTheProjectNotTheCheckout(t *testing.T) {
	f := ready()
	f.RepoReady = false // e.g. an unpacked release rather than a clone
	mustAllow(t, f, ActionBuild, ActionPull2, ActionUp)
	mustRefuse(t, f, ActionPull, ActionFetch, ActionCheckout)
}

// Saving needs a template to start from, not git.
func TestSavingEnvNeedsATemplate(t *testing.T) {
	f := ready()
	f.EnvTemplate = false
	mustRefuse(t, f, ActionSaveEnv)

	f = ready()
	f.RepoReady = false
	mustAllow(t, f, ActionSaveEnv)
}

func TestWithoutACheckoutOnlyCloneAndBrowseAreOffered(t *testing.T) {
	f := Facts{}
	mustAllow(t, f, ActionClone, ActionBrowse)
	mustRefuse(t, f,
		ActionPull, ActionFetch, ActionCheckout, ActionUnpin,
		ActionSaveEnv, ActionMirrorScript,
		ActionBuild, ActionPull2, ActionUp, ActionDown)
}

// Every compose command carries --env-file, so without the file none of them
// can run. Saving it is what unblocks the rest.
func TestWithoutTheEnvFileComposeIsBlockedButSavingIsNot(t *testing.T) {
	f := ready()
	f.EnvExists = false
	mustRefuse(t, f, ActionBuild, ActionPull2, ActionUp, ActionMirrorScript)
	mustAllow(t, f, ActionSaveEnv)
}

// Starting no longer requires a pinned tag: tracking a branch is a supported
// way to run this, unlike in the old manager.
func TestStartingDoesNotRequireAPinnedVersion(t *testing.T) {
	mustAllow(t, ready(), ActionUp)
}

// The UI shows the reason on the disabled control, so it has to name the
// condition that actually blocked the action rather than the first one checked.
func TestReasonNamesTheBlockingCondition(t *testing.T) {
	f := ready()
	f.ServicesRunning = true
	if got := capabilities(f)[ActionSaveEnv].Reason; got != "サービスを停止してから操作してください" {
		t.Errorf("reason = %q, want the running-services message", got)
	}

	f = ready()
	f.EnvExists = false
	if got := capabilities(f)[ActionUp].Reason; got != ".env.docker を保存してください" {
		t.Errorf("reason = %q, want the missing-env message", got)
	}
}
