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

	"github.com/fiwon123/distributed-task-queue/backend/internal/queue"
)

// JobStore is the subset of the job store the HTTP API needs. The production
// implementation is *queue.Store.
type JobStore interface {
	Enqueue(ctx context.Context, p queue.EnqueueParams) (queue.Job, error)
	Get(ctx context.Context, id string) (queue.Job, error)
	Requeue(ctx context.Context, id string) (queue.Job, error)
	RequeueDead(ctx context.Context, queue string) (int, error)
	QueueStats(ctx context.Context, queue string) (queue.Stats, error)
	ListJobs(ctx context.Context, p queue.ListParams) ([]queue.Job, error)
	ListQueues(ctx context.Context) ([]queue.QueueInfo, error)
	Ping(ctx context.Context) error
}

// Server serves the /v1 HTTP API and, when a dashboard is configured, the
// embedded single-page app.
type Server struct {
	store     JobStore
	log       *slog.Logger
	dashboard fs.FS // optional; nil disables the UI
	apiDocs   fs.FS // optional; serves /openapi.yaml and /docs when set
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
// built asset directory (e.g. fs.Sub(frontend.Dist, "dist")). Unknown non-/v1 paths
// fall back to index.html so client-side routes work on refresh. It returns the
// receiver for chaining.
func (s *Server) WithDashboard(fsys fs.FS) *Server {
	s.dashboard = fsys
	return s
}

// WithOpenAPI mounts the API documentation: fsys must contain openapi.yaml,
// openapi.json, docs.html, and a swagger/ asset directory, served at
// /openapi.yaml, /openapi.json, /docs, and /swagger/ respectively. It returns the
// receiver for chaining.
func (s *Server) WithOpenAPI(fsys fs.FS) *Server {
	s.apiDocs = fsys
	return s
}

// route is one method+path binding in the API surface.
type route struct {
	method  string
	pattern string
	handler http.HandlerFunc
}

// routes is the single source of truth for the /v1 API surface. Handler
// registers exactly these, and the OpenAPI drift test in openapi_test.go fails
// if this list and docs/openapi.yaml disagree.
func (s *Server) routes() []route {
	return []route{
		{http.MethodPost, "/v1/jobs", s.handleEnqueue},
		{http.MethodGet, "/v1/jobs", s.handleListJobs},
		{http.MethodGet, "/v1/jobs/{id}", s.handleGetJob},
		{http.MethodPost, "/v1/jobs/{id}/requeue", s.handleRequeue},
		{http.MethodGet, "/v1/queues", s.handleListQueues},
		{http.MethodPost, "/v1/queues/{name}/requeue-dead", s.handleRequeueDead},
		{http.MethodGet, "/v1/queues/{name}/stats", s.handleQueueStats},
	}
}

// Handler builds the routed http.Handler for the API.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	for _, rt := range s.routes() {
		mux.HandleFunc(rt.method+" "+rt.pattern, rt.handler)
	}
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /readyz", s.handleReady)
	if s.apiDocs != nil {
		mux.HandleFunc("GET /openapi.yaml", s.handleOpenAPISpec)
		mux.HandleFunc("GET /openapi.json", s.handleOpenAPIJSON)
		mux.HandleFunc("GET /docs", s.handleAPIDocs)
		// The vendored Swagger UI assets are served from /swagger/; a missing
		// subdirectory just leaves those routes unregistered.
		if assets, err := fs.Sub(s.apiDocs, "swagger"); err == nil {
			mux.Handle("GET /swagger/", http.StripPrefix("/swagger/", http.FileServerFS(assets)))
		}
	}
	if s.dashboard != nil {
		mux.Handle("/", s.spaHandler())
	}
	return mux
}


// handleHealth is a liveness probe.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleReady is a readiness probe that checks DB connectivity.
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Ping(r.Context()); err != nil {
		s.log.Error("readiness ping failed", "error", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleOpenAPISpec serves the embedded OpenAPI document.
func (s *Server) handleOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	body, err := fs.ReadFile(s.apiDocs, "openapi.yaml")
	if err != nil {
		s.log.Error("read openapi spec", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	_, _ = w.Write(body)
}

// handleOpenAPIJSON serves the embedded document as JSON. It is a generated
// artifact (see docs/generate) so this handler needs no YAML dependency.
func (s *Server) handleOpenAPIJSON(w http.ResponseWriter, r *http.Request) {
	body, err := fs.ReadFile(s.apiDocs, "openapi.json")
	if err != nil {
		s.log.Error("read openapi json", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(body)
}

// handleAPIDocs serves the embedded Swagger UI page.
func (s *Server) handleAPIDocs(w http.ResponseWriter, r *http.Request) {
	body, err := fs.ReadFile(s.apiDocs, "docs.html")
	if err != nil {
		s.log.Error("read api docs page", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(body)
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
