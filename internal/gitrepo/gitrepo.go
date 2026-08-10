// Package gitrepo manages the phantom-release checkout.
//
// Ported from the old GitRepository.cs. The shape is the same — clone, fetch
// tags, list tags, checkout, report the checked-out tag — with pull added,
// since the new requirements ask for it. Version pinning is kept: the machine
// this was written on had phantom-release sitting on a detached v1.0.36, so
// checking out a tag is how this is actually operated.
//
// Every git invocation is a direct exec with an argument slice. The old
// manager built bash command strings and shipped them through wsl.exe, which
// is why WslCommand had to care about quoting; none of that survives the move
// inside WSL.
package gitrepo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hyperion13th144m/phantom-manager/internal/runner"
)

// DefaultURL is the repository the manager clones.
const DefaultURL = "https://github.com/hyperion13th144m/phantom-release"

// Repo is a phantom-release checkout at a fixed path.
type Repo struct {
	Dir string
}

// New returns a Repo rooted at dir.
func New(dir string) *Repo { return &Repo{Dir: dir} }

// Info describes the checkout for the UI.
type Info struct {
	Dir      string   `json:"dir"`
	Exists   bool     `json:"exists"`
	Ready    bool     `json:"ready"` // a real phantom-release checkout
	Branch   string   `json:"branch,omitempty"`
	Tag      string   `json:"tag,omitempty"`      // set only when a tag is checked out
	Detached bool     `json:"detached"`           // HEAD is not on a branch
	Head     string   `json:"head,omitempty"`     // short commit id
	Dirty    bool     `json:"dirty"`              // uncommitted changes present
	Tags     []string `json:"tags,omitempty"`     // newest first
	Describe string   `json:"describe,omitempty"` // nearest tag, even when not exact
}

// Ready reports whether dir holds a phantom-release checkout. The old manager
// tested for .git plus docker-compose.yml; the same pair still distinguishes a
// real checkout from an empty or unrelated directory.
func (r *Repo) Ready() bool {
	if st, err := os.Stat(filepath.Join(r.Dir, ".git")); err != nil || !st.IsDir() {
		return false
	}
	_, err := os.Stat(filepath.Join(r.Dir, "docker-compose.yml"))
	return err == nil
}

// Exists reports whether the directory is there at all.
func (r *Repo) Exists() bool {
	st, err := os.Stat(r.Dir)
	return err == nil && st.IsDir()
}

// Status reads the checkout without touching the network.
func (r *Repo) Status(ctx context.Context) Info {
	info := Info{Dir: r.Dir, Exists: r.Exists(), Ready: r.Ready()}
	if !info.Ready {
		return info
	}

	// symbolic-ref fails on a detached HEAD, which is exactly how the old
	// manager decided a tag was checked out.
	if out, code := r.capture(ctx, "symbolic-ref", "--short", "-q", "HEAD"); code == 0 {
		info.Branch = strings.TrimSpace(out)
	} else {
		info.Detached = true
	}

	if out, code := r.capture(ctx, "rev-parse", "--short", "HEAD"); code == 0 {
		info.Head = strings.TrimSpace(out)
	}
	// --exact-match answers "is HEAD precisely this tag", which is the question
	// the version display asks. A plain describe would report the nearest tag
	// and make a checkout look pinned when it is not.
	if out, code := r.capture(ctx, "describe", "--exact-match", "--tags", "HEAD"); code == 0 {
		info.Tag = strings.TrimSpace(out)
	}
	if out, code := r.capture(ctx, "describe", "--tags", "--always"); code == 0 {
		info.Describe = strings.TrimSpace(out)
	}
	// -uno counts only tracked modifications. Untracked files must not count:
	// .env.docker is generated into the checkout and phantom-release does not
	// ignore it, so counting it would make every checkout and pull impossible
	// the moment the user saved their settings. Git itself lets untracked files
	// through and only objects when one would be overwritten.
	if out, code := r.capture(ctx, "status", "--porcelain", "-uno"); code == 0 && strings.TrimSpace(out) != "" {
		info.Dirty = true
	}
	info.Tags = r.tags(ctx)
	return info
}

// tags lists local tags, newest first. -v:refname sorts by version rather than
// lexically, so v1.0.10 lands after v1.0.9.
func (r *Repo) tags(ctx context.Context) []string {
	out, code := r.capture(ctx, "tag", "--list", "--sort=-v:refname")
	if code != 0 {
		return nil
	}
	var tags []string
	for _, line := range strings.Split(out, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			tags = append(tags, t)
		}
	}
	return tags
}

// Clone fetches the repository into Dir, creating the parent directory first.
func (r *Repo) Clone(ctx context.Context, url string, log func(runner.Line)) error {
	if r.Ready() {
		return fmt.Errorf("%s には既に phantom-release があります", r.Dir)
	}
	// git refuses to clone into a non-empty directory, and a half-populated one
	// is a worse thing to discover after the network transfer than before it.
	if entries, err := os.ReadDir(r.Dir); err == nil && len(entries) > 0 {
		return fmt.Errorf("%s は空ではありません。別のディレクトリを指定するか、中身を退避してください", r.Dir)
	}
	if err := os.MkdirAll(filepath.Dir(r.Dir), 0o755); err != nil {
		return fmt.Errorf("親ディレクトリを作成できませんでした: %w", err)
	}
	if url == "" {
		url = DefaultURL
	}
	return r.run(ctx, "", log, "clone", url, r.Dir)
}

// Pull fast-forwards the current branch.
func (r *Repo) Pull(ctx context.Context, log func(runner.Line)) error {
	if err := r.requireReady(); err != nil {
		return err
	}
	info := r.Status(ctx)
	// Pulling on a detached HEAD does nothing useful and the error git gives is
	// obscure. Say what state the checkout is in instead.
	if info.Detached {
		tag := info.Tag
		if tag == "" {
			tag = info.Head
		}
		return fmt.Errorf("タグ %s をチェックアウト中のため pull できません。先にブランチへ切り替えてください", tag)
	}
	if info.Dirty {
		return errors.New("ローカルに変更があるため pull できません")
	}
	// --ff-only keeps a surprise merge commit out of a checkout the user is not
	// expected to be editing.
	return r.run(ctx, "", log, "pull", "--ff-only", "--tags")
}

// FetchTags refreshes the tag list from the remote. --prune-tags drops tags
// deleted upstream, which the old manager's plain --prune did not do.
func (r *Repo) FetchTags(ctx context.Context, log func(runner.Line)) error {
	if err := r.requireReady(); err != nil {
		return err
	}
	return r.run(ctx, "", log, "fetch", "--tags", "--prune", "--prune-tags")
}

// Checkout switches to a tag, leaving HEAD detached. That detached state is
// what Status reports back as the pinned version.
func (r *Repo) Checkout(ctx context.Context, tag string, log func(runner.Line)) error {
	if err := r.requireReady(); err != nil {
		return err
	}
	if strings.TrimSpace(tag) == "" {
		return errors.New("バージョンを選択してください")
	}
	if info := r.Status(ctx); info.Dirty {
		return errors.New("ローカルに変更があるためチェックアウトできません")
	}
	// refs/tags/ pins the interpretation: a tag and a branch may share a name,
	// and a bare name would resolve to the branch.
	return r.run(ctx, "", log, "checkout", "--detach", "refs/tags/"+tag)
}

// DefaultBranch reports the branch the remote points HEAD at, falling back to
// main. Checking out a tag detaches HEAD, and this is the way back.
func (r *Repo) DefaultBranch(ctx context.Context) string {
	if out, code := r.capture(ctx, "symbolic-ref", "--short", "-q", "refs/remotes/origin/HEAD"); code == 0 {
		if _, branch, ok := strings.Cut(strings.TrimSpace(out), "/"); ok && branch != "" {
			return branch
		}
	}
	return "main"
}

// CheckoutDefaultBranch leaves a pinned version and returns to tracking the
// remote's default branch, which is what makes pull possible again.
func (r *Repo) CheckoutDefaultBranch(ctx context.Context, log func(runner.Line)) error {
	if err := r.requireReady(); err != nil {
		return err
	}
	if info := r.Status(ctx); info.Dirty {
		return errors.New("ローカルに変更があるため切り替えできません")
	}
	return r.run(ctx, "", log, "checkout", r.DefaultBranch(ctx))
}

func (r *Repo) requireReady() error {
	if !r.Ready() {
		return fmt.Errorf("phantom-release が見つかりません: %s", r.Dir)
	}
	return nil
}

// run executes git in the checkout and fails on a non-zero exit.
func (r *Repo) run(ctx context.Context, dir string, log func(runner.Line), args ...string) error {
	if dir == "" {
		dir = r.Dir
	}
	// A clone runs before the directory exists, so fall back to its parent.
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		dir = filepath.Dir(dir)
	}
	res, err := runner.Run(ctx, dir, "git", args, log)
	if err != nil {
		return err
	}
	if !res.OK() {
		return runner.Errorf("git", args, res)
	}
	return nil
}

func (r *Repo) capture(ctx context.Context, args ...string) (string, int) {
	return runner.Capture(ctx, r.Dir, "git", args)
}
