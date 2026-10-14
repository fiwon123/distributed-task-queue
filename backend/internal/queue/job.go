// Package queue is the durable job store. Postgres is the source of truth: the
// jobs table *is* the queue, and every state transition is a row update rather
// than in-memory bookkeeping.
package queue

import (
	"encoding/json"
	"errors"
	"time"
)

// Status is a job's lifecycle state.
type Status string

const (
	// StatusPending is a job that has been enqueued and is not yet claimed.
	StatusPending Status = "pending"
	// StatusRunning is a job currently held by a worker under a lease.
	StatusRunning Status = "running"
	// StatusCompleted is a job whose handler finished successfully.
	StatusCompleted Status = "completed"
	// StatusFailed is a job whose last attempt failed but which may be retried.
	StatusFailed Status = "failed"
	// StatusDead is a job that exhausted its attempts and is no longer retried.
	StatusDead Status = "dead"
)

// AllStatuses is the fixed census of job states, in lifecycle order.
var AllStatuses = []Status{
	StatusPending, StatusRunning, StatusCompleted, StatusFailed, StatusDead,
}

// Valid reports whether s is one of the five known statuses.
func (s Status) Valid() bool {
	for _, known := range AllStatuses {
		if s == known {
			return true
		}
	}
	return false
}

// DefaultQueue is the queue a job lands in when the request omits one.
const DefaultQueue = "default"

// DefaultMaxAttempts is the retry ceiling when the request omits one.
const DefaultMaxAttempts = 3

// Job is one row in the jobs table: a unit of work and its durable state.
type Job struct {
	ID             string          `json:"id"`
	Type           string          `json:"type"`
	Payload        json.RawMessage `json:"payload"`
	Queue          string          `json:"queue"`
	Priority       int             `json:"priority"`
	Status         Status          `json:"status"`
	Attempts       int             `json:"attempts"`
	MaxAttempts    int             `json:"max_attempts"`
	TimeoutSeconds *int            `json:"timeout_seconds,omitempty"`
	RunAt          time.Time       `json:"run_at"`
	LockedUntil    *time.Time      `json:"locked_until,omitempty"`
	LastError      *string         `json:"last_error,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	CompletedAt    *time.Time      `json:"completed_at,omitempty"`
}

// EnqueueParams describes a job to enqueue. Type is required; Payload defaults
// to an empty object, Queue to DefaultQueue, MaxAttempts to
// DefaultMaxAttempts, TimeoutSeconds to no timeout, and Priority to 0.
type EnqueueParams struct {
	Type           string
	Payload        json.RawMessage
	Queue          string
	Priority       *int
	RunAt          *time.Time
	MaxAttempts    *int
	TimeoutSeconds *int
}

// ValidationError reports a request rejected because of its input (a client
// error), as opposed to a storage failure. Callers can map it to HTTP 400.
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

var (
	// ErrJobRunning is returned when a job is running and cannot be deleted.
	ErrJobRunning = errors.New("job is running")
)
