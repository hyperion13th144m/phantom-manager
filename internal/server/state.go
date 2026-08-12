package server

import (
	"context"
	"net/http"
	"path/filepath"

	"github.com/hyperion13th144m/phantom-manager/internal/compose"
	"github.com/hyperion13th144m/phantom-manager/internal/envcheck"
	"github.com/hyperion13th144m/phantom-manager/internal/envfile"
	"github.com/hyperion13th144m/phantom-manager/internal/gitrepo"
	"github.com/hyperion13th144m/phantom-manager/internal/jobs"
)

// Action names an operation. The same string is the capability key, the button's
// data-action in the UI, and the suffix of the endpoint that performs it, so
// there is exactly one place where an operation is named.
type Action string

const (
	ActionClone    Action = "repo/clone"
	ActionPull     Action = "repo/pull"
	ActionFetch    Action = "repo/fetch"
	ActionCheckout Action = "repo/checkout"
	ActionUnpin    Action = "repo/unpin"

	ActionSaveEnv Action = "env/save"

	ActionBrowse       Action = "mirror/browse"
	ActionMirrorScript Action = "mirror/create"

	ActionBuild    Action = "compose/build"
	ActionPull2    Action = "compose/pull"
	ActionUp       Action = "compose/up"
	ActionDown     Action = "compose/down"
	ActionRemoveES Action = "compose/es-volume-rm"
)

// Capability is whether an action may run, and why not when it may not. The
// reason is shown on the disabled control, so a greyed-out button explains
// itself instead of just being dead.
type Capability struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}

// Facts are the conditions every capability is derived from.
//
// The old manager had a single "repoReady" covering everything, which conflated
// two different requirements: git operations need a checkout, compose
// operations need a compose file. Keeping them apart matters most for down —
// under the combined rule, a checkout that stopped looking like a git
// repository took away the only way to stop the containers it had started.
type Facts struct {
	Busy            bool
	RepoExists      bool // the directory is there at all
	RepoReady       bool // it is a git checkout of phantom-release
	ProjectReady    bool // docker-compose.yml is present
	EnvTemplate     bool // .env.docker or .env.docker.sample is present
	EnvExists       bool // .env.docker has been generated
	ServicesRunning bool
	ContainersExist bool // the project has containers, running or stopped
}

// State is the whole picture the UI renders from, gathered in one pass so the
// panels can never disagree with each other.
type State struct {
	Checks       envcheck.Result       `json:"checks"`
	Repo         gitrepo.Info          `json:"repo"`
	Env          envfile.Settings      `json:"env"`
	EnvPath      string                `json:"envPath"`
	EnvSaved     bool                  `json:"envSaved"`
	Services     []compose.Service     `json:"services"`
	PublicURL    string                `json:"publicUrl"`
	ComposeError string                `json:"composeError,omitempty"`
	Job          jobs.Status           `json:"job"`
	Can          map[Action]Capability `json:"can"`
}

// capabilities is the condition table, ported from Form1.cs SetBusy.
//
// Three of the old rules carry real consequences and are kept verbatim in
// spirit:
//
//   - .env.docker may not be saved while services run. A running project has
//     those directories bind-mounted; changing the file underneath it makes the
//     manager's view and the containers' reality disagree.
//   - The checkout may not move while services run, for the same reason applied
//     to docker-compose.yml.
//   - Cloning is refused when the directory is already there, before the
//     transfer rather than after it.
//
// Two of the old rules are dropped. Starting compose no longer requires a tag
// to be checked out — tracking a branch is now a supported way to run this —
// and the SSL and database-initialisation controls no longer exist.
//
// build and pull are deliberately allowed while services run: they only fetch
// and produce images, which nothing picks up until the next up.
func capabilities(f Facts) map[Action]Capability {
	const (
		whyBusy      = "実行中の処理が終わるまで操作できません"
		whyNoRepo    = "phantom-release を取得してください"
		whyNoProject = "docker-compose.yml が見つかりません"
		whyNoSample  = ".env.docker.sample が見つかりません"
		whyNoEnv     = ".env.docker を保存してください"
		whyRunning   = "サービスを停止してから操作してください"
		whyStopped   = "サービスが起動していません"
		whyHaveRepo  = "既に取得済みです"
		// docker refuses to remove a volume any container still references, and
		// "停止" removes the containers as well as stopping them.
		whyContainers = "「停止」でコンテナを削除してから操作してください"
	)

	allow := func(conds ...struct {
		ok     bool
		reason string
	}) Capability {
		for _, c := range conds {
			if !c.ok {
				return Capability{Allowed: false, Reason: c.reason}
			}
		}
		return Capability{Allowed: true}
	}
	cond := func(ok bool, reason string) struct {
		ok     bool
		reason string
	} {
		return struct {
			ok     bool
			reason string
		}{ok, reason}
	}

	notBusy := cond(!f.Busy, whyBusy)
	repoReady := cond(f.RepoReady, whyNoRepo)
	projectReady := cond(f.ProjectReady, whyNoProject)
	envExists := cond(f.EnvExists, whyNoEnv)
	notRunning := cond(!f.ServicesRunning, whyRunning)

	return map[Action]Capability{
		ActionClone:    allow(notBusy, cond(!f.RepoExists, whyHaveRepo)),
		ActionPull:     allow(notBusy, repoReady, notRunning),
		ActionFetch:    allow(notBusy, repoReady, notRunning),
		ActionCheckout: allow(notBusy, repoReady, notRunning),
		ActionUnpin:    allow(notBusy, repoReady, notRunning),

		// Writing the file needs a template to start from, not a git checkout.
		ActionSaveEnv: allow(notBusy, cond(f.EnvTemplate, whyNoSample), notRunning),

		ActionBrowse: allow(notBusy),
		// Generating the script is harmless in itself, but running it copies
		// into PHANTOM_SRC_DIR, which crow is reading while the pipeline is up.
		// It needs PHANTOM_SRC_DIR, so it needs the env file — not git.
		ActionMirrorScript: allow(notBusy, envExists, notRunning),

		ActionBuild: allow(notBusy, projectReady, envExists),
		ActionPull2: allow(notBusy, projectReady, envExists),
		ActionUp:    allow(notBusy, projectReady, envExists, notRunning),
		// Down deliberately asks for nothing but running containers. It is the
		// way out of a bad state, so it must not be gated on the same
		// conditions that might have gone wrong; if the project files are
		// unusable, compose says so, which beats a disabled button.
		ActionDown: allow(notBusy, cond(f.ServicesRunning, whyStopped)),
		// Throwing away the index needs the compose file to find the volume by,
		// and every container gone — a stopped one still holds the volume, so
		// "no container at all" is the condition, not "nothing running".
		ActionRemoveES: allow(notBusy, projectReady, envExists, cond(!f.ContainersExist, whyContainers)),
	}
}

// gather collects the facts and everything the UI draws.
func (s *Server) gather(ctx context.Context) State {
	repo := s.repo().Status(ctx)
	settings, envSaved, _ := envfile.Load(s.cfg.ReleaseDir)

	st := State{
		Checks:    envcheck.Run(ctx, s.cfg.ReleaseDir),
		Repo:      repo,
		Env:       settings,
		EnvPath:   envfile.Path(s.cfg.ReleaseDir),
		EnvSaved:  envSaved,
		PublicURL: settings.PublicURL,
		Job:       s.jobs.Status(),
		Services:  []compose.Service{},
	}

	// A compose failure is a normal state for this panel — nothing started yet,
	// no env file — so it is reported alongside an empty table rather than as an
	// error for the whole request.
	if services, err := compose.New(s.cfg.ReleaseDir).Ps(ctx); err != nil {
		st.ComposeError = err.Error()
	} else {
		st.Services = services
	}

	running := false
	for _, svc := range st.Services {
		if svc.Running {
			running = true
			break
		}
	}

	st.Can = capabilities(Facts{
		Busy:            s.jobs.Busy(),
		RepoExists:      repo.Exists,
		RepoReady:       repo.Ready,
		ProjectReady:    fileExists(filepath.Join(s.cfg.ReleaseDir, "docker-compose.yml")),
		EnvTemplate:     envSaved || fileExists(envfile.SamplePath(s.cfg.ReleaseDir)),
		EnvExists:       envSaved,
		ServicesRunning: running,
		ContainersExist: len(st.Services) > 0,
	})
	return st
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.gather(r.Context()))
}

// guard refuses an action the current state does not permit, so the rules hold
// even when the request did not come from a button this manager drew.
func (s *Server) guard(w http.ResponseWriter, r *http.Request, action Action) bool {
	can := s.gather(r.Context()).Can[action]
	if can.Allowed {
		return true
	}
	writeJSON(w, http.StatusConflict, map[string]any{"error": can.Reason, "action": action})
	return false
}
