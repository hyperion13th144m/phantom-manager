// Package server exposes the manager over HTTP on the loopback interface.
//
// The listener is bound to 127.0.0.1 on purpose. WSL2's localhost forwarding
// makes that reachable from the Windows browser as http://localhost:<port>,
// which is all we need; binding 0.0.0.0 would additionally expose an unauth-
// enticated control surface to the LAN.
package server

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"time"

	"github.com/hyperion13th144m/phantom-manager/internal/envcheck"
	"github.com/hyperion13th144m/phantom-manager/internal/jobs"
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
	mux.Handle("GET /", http.FileServer(http.FS(s.web)))
	return mux
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
