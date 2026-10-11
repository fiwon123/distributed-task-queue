package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/fiwon123/distributed-task-queue/internal/queue"
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
