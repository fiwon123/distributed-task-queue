package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/fiwon123/distributed-task-queue/backend/internal/queue"
)

// maxBodyBytes bounds a request body so a malformed client cannot exhaust
// memory. 1 MiB is generous for a job payload.
const maxBodyBytes = 1 << 20

// enqueueRequest is the JSON body of POST /v1/jobs.
type enqueueRequest struct {
	Type        string          `json:"type"`
	Payload     json.RawMessage `json:"payload"`
	Queue       string          `json:"queue"`
	RunAt       *time.Time      `json:"run_at"`
	MaxAttempts *int            `json:"max_attempts"`
}

// handleEnqueue implements POST /v1/jobs.
func (s *Server) handleEnqueue(w http.ResponseWriter, r *http.Request) {
	var req enqueueRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		if errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "request body is required")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}

	job, err := s.store.Enqueue(r.Context(), queue.EnqueueParams{
		Type:        req.Type,
		Payload:     req.Payload,
		Queue:       req.Queue,
		RunAt:       req.RunAt,
		MaxAttempts: req.MaxAttempts,
	})
	if err != nil {
		var invalid *queue.ValidationError
		if errors.As(err, &invalid) {
			writeError(w, http.StatusBadRequest, invalid.Message)
			return
		}
		s.log.Error("enqueue job", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, job)
}

// handleGetJob implements GET /v1/jobs/{id}.
func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !isUUID(id) {
		// A malformed id cannot name an existing job.
		writeError(w, http.StatusNotFound, "job not found")
		return
	}

	job, err := s.store.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, queue.ErrNotFound) {
			writeError(w, http.StatusNotFound, "job not found")
			return
		}
		s.log.Error("get job", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, job)
}

// isUUID reports whether s looks like a canonical 8-4-4-4-12 hex UUID. It keeps
// a malformed id from reaching Postgres, which would surface a 500.
func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !isHexDigit(c) {
				return false
			}
		}
	}
	return true
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// handleRequeue implements POST /v1/jobs/{id}/requeue.
func (s *Server) handleRequeue(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !isUUID(id) {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}

	job, err := s.store.Requeue(r.Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, queue.ErrNotFound):
			writeError(w, http.StatusNotFound, "job not found")
		case errors.Is(err, queue.ErrNotRequeueable):
			writeError(w, http.StatusConflict, "job is not in a requeueable state")
		default:
			s.log.Error("requeue job", "id", id, "error", err)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	writeJSON(w, http.StatusOK, job)
}

// requeueDeadResponse is the body of POST /v1/queues/{name}/requeue-dead.
type requeueDeadResponse struct {
	Queue    string `json:"queue"`
	Requeued int    `json:"requeued"`
}

// handleRequeueDead implements POST /v1/queues/{name}/requeue-dead: it returns
// every dead-lettered job in the named queue to the pending state and reports
// how many jobs it revived. A queue with no dead jobs reports 0, not an error.
func (s *Server) handleRequeueDead(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if strings.TrimSpace(name) == "" {
		writeError(w, http.StatusBadRequest, "queue name is required")
		return
	}

	requeued, err := s.store.RequeueDead(r.Context(), name)
	if err != nil {
		s.log.Error("requeue dead jobs", "queue", name, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, requeueDeadResponse{Queue: name, Requeued: requeued})
}

// handleQueueStats implements GET /v1/queues/{name}/stats.
func (s *Server) handleQueueStats(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if strings.TrimSpace(name) == "" {
		writeError(w, http.StatusBadRequest, "queue name is required")
		return
	}

	stats, err := s.store.QueueStats(r.Context(), name)
	if err != nil {
		s.log.Error("queue stats", "queue", name, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

// handleListJobs implements GET /v1/jobs?queue=&status=&limit=.
func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	p := queue.ListParams{
		Queue:  query.Get("queue"),
		Status: queue.Status(query.Get("status")),
	}
	if raw := query.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "limit must be an integer")
			return
		}
		p.Limit = n
	}

	jobs, err := s.store.ListJobs(r.Context(), p)
	if err != nil {
		var invalid *queue.ValidationError
		if errors.As(err, &invalid) {
			writeError(w, http.StatusBadRequest, invalid.Message)
			return
		}
		s.log.Error("list jobs", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs, "count": len(jobs)})
}

// handleListQueues implements GET /v1/queues.
func (s *Server) handleListQueues(w http.ResponseWriter, r *http.Request) {
	queues, err := s.store.ListQueues(r.Context())
	if err != nil {
		s.log.Error("list queues", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"queues": queues})
}
