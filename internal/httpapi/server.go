// Package httpapi serves the task-queue HTTP API. Handlers depend on the
// JobStore interface rather than a concrete store, so they can be unit-tested
// against a fake with no database.
package httpapi

import (
	"context"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"

	"github.com/fiwon123/distributed-task-queue/internal/queue"
)

// JobStore is the subset of the job store the HTTP API needs. The production
// implementation is *queue.Store.
type JobStore interface {
	Enqueue(ctx context.Context, p queue.EnqueueParams) (queue.Job, error)
	Get(ctx context.Context, id string) (queue.Job, error)
	Requeue(ctx context.Context, id string) (queue.Job, error)
	QueueStats(ctx context.Context, queue string) (queue.Stats, error)
	ListJobs(ctx context.Context, p queue.ListParams) ([]queue.Job, error)
	ListQueues(ctx context.Context) ([]queue.QueueInfo, error)
}

// Server serves the /v1 HTTP API and, when a dashboard is configured, the
// embedded single-page app.
type Server struct {
	store     JobStore
	log       *slog.Logger
	dashboard fs.FS // optional; nil disables the UI
}

// New returns a Server backed by store. A nil logger falls back to
// slog.Default. The dashboard is disabled until WithDashboard is called.
func New(store JobStore, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{store: store, log: logger}
}

// WithDashboard mounts a single-page app at "/". fsys must be rooted at the
// built asset directory (e.g. fs.Sub(web.Dist, "dist")). Unknown non-/v1 paths
// fall back to index.html so client-side routes work on refresh. It returns the
// receiver for chaining.
func (s *Server) WithDashboard(fsys fs.FS) *Server {
	s.dashboard = fsys
	return s
}

// Handler builds the routed http.Handler for the API.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/jobs", s.handleEnqueue)
	mux.HandleFunc("GET /v1/jobs", s.handleListJobs)
	mux.HandleFunc("GET /v1/jobs/{id}", s.handleGetJob)
	mux.HandleFunc("POST /v1/jobs/{id}/requeue", s.handleRequeue)
	mux.HandleFunc("GET /v1/queues", s.handleListQueues)
	mux.HandleFunc("GET /v1/queues/{name}/stats", s.handleQueueStats)
	if s.dashboard != nil {
		mux.Handle("/", s.spaHandler())
	}
	return mux
}

// spaHandler serves the embedded dashboard, falling back to index.html for
// routes that do not correspond to a real asset (so deep links survive a
// refresh). Requests under /v1 never reach here: their more specific patterns
// win.
func (s *Server) spaHandler() http.Handler {
	files := http.FileServerFS(s.dashboard)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(s.dashboard, p); err != nil {
			// Not an asset: hand it to the SPA's client-side router.
			clone := r.Clone(r.Context())
			clone.URL.Path = "/"
			files.ServeHTTP(w, clone)
			return
		}
		files.ServeHTTP(w, r)
	})
}

// writeJSON writes v as a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	// The header is already written; an encode error here is not actionable.
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error body: {"error": "..."}.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
