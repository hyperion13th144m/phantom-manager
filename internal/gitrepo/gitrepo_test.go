package gitrepo

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// origin builds a throwaway upstream repository with two tags on it, standing
// in for phantom-release. Everything is local, so the tests never touch the
// network.
func origin(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	git("init", "-q", "-b", "main")
	// docker-compose.yml is half of what Ready() looks for.
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-qm", "initial")
	git("tag", "v1.0.9")
	if err := os.WriteFile(filepath.Join(dir, "CHANGELOG.md"), []byte("v1.0.10\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-qm", "second")
	git("tag", "v1.0.10")
	return dir
}

func cloned(t *testing.T) *Repo {
	t.Helper()
	r := New(filepath.Join(t.TempDir(), "phantom-release"))
	if err := r.Clone(context.Background(), origin(t), nil); err != nil {
		t.Fatalf("Clone: %v", err)
	}
	return r
}

func TestCloneProducesAReadyCheckout(t *testing.T) {
	r := cloned(t)
	if !r.Ready() {
		t.Fatal("Ready() = false after clone")
	}
	info := r.Status(context.Background())
	if !info.Ready || info.Branch != "main" || info.Detached {
		t.Errorf("info = %+v, want a ready checkout on main", info)
	}
	if info.Head == "" {
		t.Error("head commit is empty")
	}
}

// A directory holding unrelated files must not be clobbered, and the refusal
// has to come before the transfer rather than as a git error after it.
func TestCloneRefusesANonEmptyDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "target")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "important.txt"), []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := New(dir)
	if err := r.Clone(context.Background(), origin(t), nil); err == nil {
		t.Fatal("Clone into a non-empty directory succeeded")
	}
	if _, err := os.Stat(filepath.Join(dir, "important.txt")); err != nil {
		t.Errorf("existing file was disturbed: %v", err)
	}
}

// Tags sort by version, not lexically: v1.0.10 is newer than v1.0.9 even
// though it sorts before it as a string.
func TestTagsAreSortedByVersion(t *testing.T) {
	info := cloned(t).Status(context.Background())
	want := []string{"v1.0.10", "v1.0.9"}
	if len(info.Tags) != len(want) {
		t.Fatalf("tags = %v, want %v", info.Tags, want)
	}
	for i := range want {
		if info.Tags[i] != want[i] {
			t.Fatalf("tags = %v, want %v", info.Tags, want)
		}
	}
}

func TestCheckoutPinsTheVersion(t *testing.T) {
	r := cloned(t)
	ctx := context.Background()
	if err := r.Checkout(ctx, "v1.0.9", nil); err != nil {
		t.Fatalf("Checkout: %v", err)
	}
	info := r.Status(ctx)
	if !info.Detached {
		t.Error("HEAD is not detached after checking out a tag")
	}
	if info.Tag != "v1.0.9" {
		t.Errorf("tag = %q, want v1.0.9", info.Tag)
	}
	if info.Branch != "" {
		t.Errorf("branch = %q, want empty on a detached HEAD", info.Branch)
	}
}

// Status must only report a tag when HEAD is exactly on it. Reporting the
// nearest tag would make a moving checkout look pinned.
func TestStatusReportsNoTagWhenHeadIsPastIt(t *testing.T) {
	info := cloned(t).Status(context.Background())
	if info.Tag != "v1.0.10" {
		t.Fatalf("setup: tag = %q, want v1.0.10", info.Tag)
	}

	r := cloned(t)
	if err := os.WriteFile(filepath.Join(r.Dir, "extra.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	commit(t, r.Dir)
	after := r.Status(context.Background())
	if after.Tag != "" {
		t.Errorf("tag = %q, want empty one commit past the tag", after.Tag)
	}
	if after.Describe == "" {
		t.Error("describe should still name the nearest tag")
	}
}

// Pinning a version must not be a one-way door.
func TestCheckoutDefaultBranchUndoesAPin(t *testing.T) {
	r := cloned(t)
	ctx := context.Background()
	if err := r.Checkout(ctx, "v1.0.9", nil); err != nil {
		t.Fatalf("Checkout: %v", err)
	}
	if err := r.CheckoutDefaultBranch(ctx, nil); err != nil {
		t.Fatalf("CheckoutDefaultBranch: %v", err)
	}
	info := r.Status(ctx)
	if info.Detached || info.Branch != "main" {
		t.Errorf("info = %+v, want main checked out", info)
	}
	// Pull has to work again once the pin is gone.
	if err := r.Pull(ctx, nil); err != nil {
		t.Errorf("Pull after unpinning: %v", err)
	}
}

func TestPullRefusesOnADetachedHead(t *testing.T) {
	r := cloned(t)
	ctx := context.Background()
	if err := r.Checkout(ctx, "v1.0.9", nil); err != nil {
		t.Fatalf("Checkout: %v", err)
	}
	err := r.Pull(ctx, nil)
	if err == nil {
		t.Fatal("Pull on a detached HEAD succeeded")
	}
	// The message has to name the situation, since git's own error does not.
	if !strings.Contains(err.Error(), "v1.0.9") {
		t.Errorf("error = %q, should mention the checked-out tag", err)
	}
}

// The manager writes .env.docker into the checkout and phantom-release does
// not ignore it. If an untracked file counted as a local change, saving the
// data directory would lock the user out of checkout and pull for good.
func TestGeneratedEnvFileDoesNotBlockOperations(t *testing.T) {
	r := cloned(t)
	ctx := context.Background()
	if err := os.WriteFile(filepath.Join(r.Dir, ".env.docker"), []byte("PHANTOM_DATA_DIR=/tmp\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if info := r.Status(ctx); info.Dirty {
		t.Error("an untracked file marked the checkout dirty")
	}
	if err := r.Checkout(ctx, "v1.0.9", nil); err != nil {
		t.Errorf("Checkout with an untracked file present: %v", err)
	}
	if _, err := os.Stat(filepath.Join(r.Dir, ".env.docker")); err != nil {
		t.Errorf(".env.docker did not survive the checkout: %v", err)
	}
}

func TestPullRefusesWithLocalChanges(t *testing.T) {
	r := cloned(t)
	if err := os.WriteFile(filepath.Join(r.Dir, "docker-compose.yml"), []byte("edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := r.Pull(context.Background(), nil); err == nil {
		t.Fatal("Pull with local changes succeeded")
	}
}

func TestPullFastForwards(t *testing.T) {
	up := origin(t)
	r := New(filepath.Join(t.TempDir(), "phantom-release"))
	ctx := context.Background()
	if err := r.Clone(ctx, up, nil); err != nil {
		t.Fatalf("Clone: %v", err)
	}
	before := r.Status(ctx).Head

	if err := os.WriteFile(filepath.Join(up, "new.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commit(t, up)

	if err := r.Pull(ctx, nil); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if after := r.Status(ctx).Head; after == before {
		t.Errorf("HEAD did not move: still %s", after)
	}
}

func TestOperationsRefuseAMissingCheckout(t *testing.T) {
	r := New(filepath.Join(t.TempDir(), "nothing-here"))
	ctx := context.Background()
	if err := r.Pull(ctx, nil); err == nil {
		t.Error("Pull on a missing checkout succeeded")
	}
	if err := r.Checkout(ctx, "v1.0.9", nil); err == nil {
		t.Error("Checkout on a missing checkout succeeded")
	}
	if err := r.FetchTags(ctx, nil); err == nil {
		t.Error("FetchTags on a missing checkout succeeded")
	}
	if info := r.Status(ctx); info.Ready || info.Exists {
		t.Errorf("info = %+v, want neither ready nor existing", info)
	}
}

func TestCheckoutRejectsAnEmptyTag(t *testing.T) {
	if err := cloned(t).Checkout(context.Background(), "  ", nil); err == nil {
		t.Error("Checkout with an empty tag succeeded")
	}
}

func commit(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{{"add", "."}, {"commit", "-qm", "change"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
}
