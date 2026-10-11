// Package httpapi serves the task-queue HTTP API. Handlers depend on the
// JobStore interface rather than a concrete store, so they can be unit-tested
// against a fake with no database.
package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/fiwon123/distributed-task-queue/internal/queue"
)

// JobStore is the subset of the job store the HTTP API needs. The production
// implementation is *queue.Store.
type JobStore interface {
	Enqueue(ctx context.Context, p queue.EnqueueParams) (queue.Job, error)
	Get(ctx context.Context, id string) (queue.Job, error)
}

// Server serves the /v1 HTTP API.
type Server struct {
	store JobStore
	log   *slog.Logger
}

// New returns a Server backed by store. A nil logger falls back to
// slog.Default.
func New(store JobStore, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{store: store, log: logger}
}

// Handler builds the routed http.Handler for the API.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/jobs", s.handleEnqueue)
	mux.HandleFunc("GET /v1/jobs/{id}", s.handleGetJob)
	return mux
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
