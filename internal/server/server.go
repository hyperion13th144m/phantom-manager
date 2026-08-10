// Package server exposes the manager over HTTP on the loopback interface.
//
// The listener is bound to 127.0.0.1 on purpose. WSL2's localhost forwarding
// makes that reachable from the Windows browser as http://localhost:<port>,
// which is all we need; binding 0.0.0.0 would additionally expose an unauth-
// enticated control surface to the LAN.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/hyperion13th144m/phantom-manager/internal/compose"
	"github.com/hyperion13th144m/phantom-manager/internal/envcheck"
	"github.com/hyperion13th144m/phantom-manager/internal/envfile"
	"github.com/hyperion13th144m/phantom-manager/internal/gitrepo"
	"github.com/hyperion13th144m/phantom-manager/internal/jobs"
	"github.com/hyperion13th144m/phantom-manager/internal/mirror"
	"github.com/hyperion13th144m/phantom-manager/internal/paths"
	"github.com/hyperion13th144m/phantom-manager/internal/runner"
	"github.com/hyperion13th144m/phantom-manager/internal/winfs"
	"github.com/hyperion13th144m/phantom-manager/internal/wslenv"
)

// Config is everything the handlers need to know about the environment.
type Config struct {
	Version    string
	Port       int
	ReleaseDir string
}

// Server wires the routes to the job manager and the embedded web assets.
type Server struct {
	cfg  Config
	jobs *jobs.Manager
	web  fs.FS
}

// New creates a Server. web is the root of the UI assets (index.html at its top
// level), normally an embed.FS subtree.
func New(cfg Config, web fs.FS, mgr *jobs.Manager) *Server {
	return &Server{cfg: cfg, jobs: mgr, web: web}
}

// Handler builds the route table.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/jobs", s.handleJobStatus)
	mux.HandleFunc("GET /api/events", s.handleEvents)
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("GET /api/state", s.handleState)
	mux.HandleFunc("POST /api/jobs/cancel", s.handleJobCancel)
	mux.HandleFunc("GET /api/repo", s.handleRepo)
	mux.HandleFunc("POST /api/repo/clone", s.handleRepoClone)
	mux.HandleFunc("POST /api/repo/pull", s.handleRepoPull)
	mux.HandleFunc("POST /api/repo/fetch", s.handleRepoFetch)
	mux.HandleFunc("POST /api/repo/checkout", s.handleRepoCheckout)
	mux.HandleFunc("POST /api/repo/unpin", s.handleRepoUnpin)
	mux.HandleFunc("GET /api/env", s.handleEnvGet)
	mux.HandleFunc("POST /api/env", s.handleEnvSave)
	mux.HandleFunc("GET /api/lan-addresses", s.handleLanAddresses)
	mux.HandleFunc("GET /api/browse", s.handleBrowse)
	mux.HandleFunc("POST /api/mirror-script", s.handleMirrorScript)
	mux.HandleFunc("POST /api/open", s.handleOpen)
	mux.HandleFunc("GET /api/compose/ps", s.handleComposePs)
	mux.HandleFunc("POST /api/compose/{op}", s.handleComposeOp)
	mux.Handle("GET /", http.FileServer(http.FS(s.web)))
	return mux
}

// handleEnvGet returns the current .env.docker values, or the WSL defaults when
// the file has not been generated yet.
func (s *Server) handleEnvGet(w http.ResponseWriter, r *http.Request) {
	settings, found, err := envfile.Load(s.cfg.ReleaseDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"settings":  settings,
		"exists":    found,
		"path":      envfile.Path(s.cfg.ReleaseDir),
		"hasSample": fileExists(envfile.SamplePath(s.cfg.ReleaseDir)),
	})
}

// handleEnvSave writes .env.docker. It is refused while services are up: a
// running project has these directories bind-mounted, and changing the file
// underneath it leaves the manager's view and the containers' reality
// disagreeing.
func (s *Server) handleEnvSave(w http.ResponseWriter, r *http.Request) {
	if !s.guard(w, r, ActionSaveEnv) {
		return
	}
	var settings envfile.Settings
	if err := decodeJSON(r, &settings); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := envfile.Save(s.cfg.ReleaseDir, settings); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	path := envfile.Path(s.cfg.ReleaseDir)
	s.jobs.Announce(path + " を保存しました")
	s.jobs.Announce(fmt.Sprintf("取込先 %s / 展開先 %s を作成しました", settings.SrcDir, settings.DataDir))
	writeJSON(w, http.StatusOK, map[string]any{"path": path, "settings": settings})
}

// handleLanAddresses offers the Windows host's LAN addresses for
// PHANTOM_PUBLIC_URL, for when phantom should be reachable from other machines.
func (s *Server) handleLanAddresses(w http.ResponseWriter, r *http.Request) {
	adapters, err := winfs.New().LanIPv4(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"adapters": adapters})
}

// handleBrowse walks the Windows filesystem for the source picker. With no
// path it lists drives; with one it lists that directory's subdirectories.
//
// This browses the Windows namespace rather than /mnt because mapped network
// drives are not in /mnt at all, and a network share is where this shop's data
// actually lives.
func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	client := winfs.New()
	path := strings.TrimSpace(r.URL.Query().Get("path"))

	if path == "" {
		drives, err := client.Drives(r.Context())
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"path": "", "drives": drives})
		return
	}

	entries, err := client.ListDirs(r.Context(), path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	exists, err := client.Exists(r.Context(), path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"path":    path,
		"exists":  exists,
		"parent":  winfs.Parent(path),
		"entries": entries,
	})
}

// handleMirrorScript generates the robocopy .bat. The source is validated
// against Windows first: a path the manager cannot see is a path robocopy will
// fail on, and finding that out now beats finding it out from a batch window
// that has already closed.
func (s *Server) handleMirrorScript(w http.ResponseWriter, r *http.Request) {
	if !s.guard(w, r, ActionMirrorScript) {
		return
	}
	var body struct {
		Source string `json:"source"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	settings, _, err := envfile.Load(s.cfg.ReleaseDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	ok, err := winfs.New().Exists(r.Context(), body.Source)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if !ok {
		writeError(w, http.StatusBadRequest, fmt.Errorf("取込元フォルダが見つかりません: %s", body.Source))
		return
	}

	// The destination has to exist before robocopy writes into it, and it must
	// be owned by uid 1000 rather than created later by docker as root.
	if err := os.MkdirAll(settings.SrcDir, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	res, err := mirror.Generate(paths.DefaultMirrorBat(), body.Source, settings.SrcDir)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.jobs.Announce("取込スクリプトを生成しました: " + res.UNC)
	s.jobs.Announce(fmt.Sprintf("%s → %s", res.Source, res.Dest))
	writeJSON(w, http.StatusOK, res)
}

// handleComposePs returns the service table. It reports a failure as data
// rather than an error status: "not started yet" and ".env.docker missing" are
// normal states for this panel, not faults.
func (s *Server) handleComposePs(w http.ResponseWriter, r *http.Request) {
	settings, _, err := envfile.Load(s.cfg.ReleaseDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	body := map[string]any{"url": settings.PublicURL}

	services, err := compose.New(s.cfg.ReleaseDir).Ps(r.Context())
	if err != nil {
		body["services"] = []compose.Service{}
		body["error"] = err.Error()
	} else {
		body["services"] = services
	}
	writeJSON(w, http.StatusOK, body)
}

// composeOps are the four operations the requirements ask for. build and pull
// are separate because es is built from infra/es while the other twelve
// services are digest-pinned images; a plain `compose pull` fails outright
// trying to fetch the locally built one.
var composeOps = map[string]struct {
	label  string
	action Action
	run    func(*compose.Client, context.Context, func(runner.Line)) error
}{
	"build": {"ビルド", ActionBuild, (*compose.Client).Build},
	"pull":  {"イメージの取得", ActionPull2, (*compose.Client).Pull},
	"up":    {"サービスの起動", ActionUp, (*compose.Client).Up},
	"down":  {"サービスの停止", ActionDown, (*compose.Client).Down},
}

func (s *Server) handleComposeOp(w http.ResponseWriter, r *http.Request) {
	op, ok := composeOps[r.PathValue("op")]
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Errorf("不明な操作です: %s", r.PathValue("op")))
		return
	}
	if !s.guard(w, r, op.action) {
		return
	}
	client := compose.New(s.cfg.ReleaseDir)
	s.startJob(w, op.label, func(ctx context.Context, l *jobs.Log) error {
		return op.run(client, ctx, lineSink(l))
	})
}

// handleOpen asks Windows to open a path or URL. Failure is reported rather
// than raised: interop can be switched off, and the UI then shows the path for
// the user to open themselves.
func (s *Server) handleOpen(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Target string `json:"target"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := wslenv.Open(body.Target); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"opened": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"opened": true})
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// repo returns a handle on the phantom-release checkout.
func (s *Server) repo() *gitrepo.Repo { return gitrepo.New(s.cfg.ReleaseDir) }

func (s *Server) handleRepo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.repo().Status(r.Context()))
}

func (s *Server) handleRepoClone(w http.ResponseWriter, r *http.Request) {
	if !s.guard(w, r, ActionClone) {
		return
	}
	repo := s.repo()
	s.startJob(w, "phantom-release のクローン", func(ctx context.Context, l *jobs.Log) error {
		return repo.Clone(ctx, gitrepo.DefaultURL, lineSink(l))
	})
}

func (s *Server) handleRepoPull(w http.ResponseWriter, r *http.Request) {
	if !s.guard(w, r, ActionPull) {
		return
	}
	repo := s.repo()
	s.startJob(w, "phantom-release の更新", func(ctx context.Context, l *jobs.Log) error {
		return repo.Pull(ctx, lineSink(l))
	})
}

func (s *Server) handleRepoFetch(w http.ResponseWriter, r *http.Request) {
	if !s.guard(w, r, ActionFetch) {
		return
	}
	repo := s.repo()
	s.startJob(w, "バージョン一覧の取得", func(ctx context.Context, l *jobs.Log) error {
		return repo.FetchTags(ctx, lineSink(l))
	})
}

func (s *Server) handleRepoCheckout(w http.ResponseWriter, r *http.Request) {
	if !s.guard(w, r, ActionCheckout) {
		return
	}
	var body struct {
		Tag string `json:"tag"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	repo := s.repo()
	s.startJob(w, "バージョン "+body.Tag+" のチェックアウト", func(ctx context.Context, l *jobs.Log) error {
		return repo.Checkout(ctx, body.Tag, lineSink(l))
	})
}

// handleRepoUnpin leaves a checked-out tag. Without it, pinning a version is a
// one-way door: pull refuses to run on a detached HEAD.
func (s *Server) handleRepoUnpin(w http.ResponseWriter, r *http.Request) {
	if !s.guard(w, r, ActionUnpin) {
		return
	}
	repo := s.repo()
	s.startJob(w, "最新ブランチへの切り替え", func(ctx context.Context, l *jobs.Log) error {
		return repo.CheckoutDefaultBranch(ctx, lineSink(l))
	})
}

func (s *Server) handleJobCancel(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"cancelled": s.jobs.Cancel()})
}

// startJob hands work to the job manager and answers the caller. A busy manager
// is a 409 rather than a queued request: the operations here change the same
// checkout and compose project, so running two is never what was meant.
func (s *Server) startJob(w http.ResponseWriter, name string, fn func(context.Context, *jobs.Log) error) {
	st, err := s.jobs.Start(name, fn)
	if errors.Is(err, jobs.ErrBusy) {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":   jobs.ErrBusy.Error(),
			"running": st.Name,
		})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusAccepted, st)
}

// lineSink adapts the runner's output callback to the job log.
func lineSink(l *jobs.Log) func(runner.Line) {
	return func(line runner.Line) { l.Emit(line.Kind, line.Text) }
}

func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("リクエストを解釈できませんでした: %w", err)
	}
	return nil
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error()})
}

// handleStatus runs the environment checks. The old manager ran these on Shown
// and after every operation; the browser does the same on load and after each
// job finishes.
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	res := envcheck.Run(r.Context(), s.cfg.ReleaseDir)
	writeJSON(w, http.StatusOK, res)
}

// Jobs exposes the job manager so main can post the startup banner.
func (s *Server) Jobs() *jobs.Manager { return s.jobs }

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"version":    s.cfg.Version,
		"port":       s.cfg.Port,
		"releaseDir": s.cfg.ReleaseDir,
	})
}

func (s *Server) handleJobStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.jobs.Status())
}

// handleEvents is the SSE stream that carries the log pane. A client gets the
// replay buffer first so a freshly opened browser sees what already happened.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream; charset=utf-8")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	ch, backlog, cancel := s.jobs.Subscribe()
	defer cancel()

	enc := json.NewEncoder(w)
	send := func(ev jobs.Event) bool {
		if _, err := w.Write([]byte("data: ")); err != nil {
			return false
		}
		if err := enc.Encode(ev); err != nil { // Encode appends the newline
			return false
		}
		if _, err := w.Write([]byte("\n")); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	for _, ev := range backlog {
		if !send(ev) {
			return
		}
	}
	flusher.Flush()

	// Comment heartbeats keep the connection from being reaped while idle and
	// let the browser notice a dead server promptly.
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-ch:
			if !send(ev) {
				return
			}
		case <-ticker.C:
			if _, err := w.Write([]byte(": ping\n\n")); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
