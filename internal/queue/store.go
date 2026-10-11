package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a job id does not exist.
var ErrNotFound = errors.New("job not found")

// ErrNotRequeueable is returned when a job exists but is not in a state that can
// be requeued (only dead and failed jobs can).
var ErrNotRequeueable = errors.New("job is not in a requeueable state")

// Store persists jobs in PostgreSQL. It is safe for concurrent use.
type Store struct {
	pool *pgxpool.Pool
}

// New returns a Store backed by pool.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// normalize validates an enqueue request and fills in its defaults. Keeping it
// pure (no database) lets it be unit-tested without Postgres and keeps the SQL
// path free of branching.
func normalize(p EnqueueParams) (EnqueueParams, error) {
	p.Type = strings.TrimSpace(p.Type)
	if p.Type == "" {
		return EnqueueParams{}, &ValidationError{Message: "type is required"}
	}

	if len(p.Payload) == 0 {
		p.Payload = json.RawMessage(`{}`)
	}
	if !json.Valid(p.Payload) {
		return EnqueueParams{}, &ValidationError{Message: "payload must be valid JSON"}
	}

	p.Queue = strings.TrimSpace(p.Queue)
	if p.Queue == "" {
		p.Queue = DefaultQueue
	}

	if p.MaxAttempts == nil {
		n := DefaultMaxAttempts
		p.MaxAttempts = &n
	} else if *p.MaxAttempts < 1 {
		return EnqueueParams{}, &ValidationError{Message: "max_attempts must be at least 1"}
	}

	return p, nil
}

const enqueueSQL = `
INSERT INTO jobs (type, payload, queue, run_at, max_attempts)
VALUES ($1, $2, $3, COALESCE($4, now()), $5)
RETURNING id::text, type, payload, queue, status, attempts, max_attempts,
          run_at, locked_until, last_error, created_at, completed_at`

// Enqueue inserts a new pending job and returns the stored row. A requested
// run_at in the past is allowed; the job is immediately claimable.
func (s *Store) Enqueue(ctx context.Context, p EnqueueParams) (Job, error) {
	p, err := normalize(p)
	if err != nil {
		return Job{}, err
	}
	row := s.pool.QueryRow(ctx, enqueueSQL,
		p.Type, []byte(p.Payload), p.Queue, p.RunAt, *p.MaxAttempts)
	return scanJob(row)
}

const getSQL = `
SELECT id::text, type, payload, queue, status, attempts, max_attempts,
       run_at, locked_until, last_error, created_at, completed_at
FROM jobs
WHERE id = $1`

// Get returns the job with the given id, or ErrNotFound if none exists.
func (s *Store) Get(ctx context.Context, id string) (Job, error) {
	return scanJob(s.pool.QueryRow(ctx, getSQL, id))
}

// rowScanner is the subset of pgx.Row scanJob needs, so it can scan from either
// QueryRow or a future single-connection variant.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanJob(row rowScanner) (Job, error) {
	var (
		j       Job
		payload []byte
		status  string
	)
	err := row.Scan(
		&j.ID, &j.Type, &payload, &j.Queue, &status, &j.Attempts,
		&j.MaxAttempts, &j.RunAt, &j.LockedUntil, &j.LastError,
		&j.CreatedAt, &j.CompletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Job{}, ErrNotFound
		}
		return Job{}, fmt.Errorf("scan job: %w", err)
	}
	j.Payload = json.RawMessage(payload)
	j.Status = Status(status)
	return j, nil
}

const requeueSQL = `
UPDATE jobs
SET status       = 'pending',
    attempts     = 0,
    run_at       = now(),
    locked_until = NULL,
    last_error   = NULL,
    completed_at = NULL
WHERE id = $1 AND status IN ('dead', 'failed')
RETURNING id::text, type, payload, queue, status, attempts, max_attempts,
          run_at, locked_until, last_error, created_at, completed_at`

// Requeue returns a dead or failed job to the queue with a fresh retry budget:
// status pending, attempts reset to 0, lease and error cleared, run_at now.
//
// It returns ErrNotRequeueable when the job exists but is not dead/failed, and
// ErrNotFound when no such job exists.
func (s *Store) Requeue(ctx context.Context, id string) (Job, error) {
	job, err := scanJob(s.pool.QueryRow(ctx, requeueSQL, id))
	if err == nil {
		return job, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Job{}, err
	}

	// The UPDATE matched no row: either the id is unknown or the job is in a
	// state that cannot be requeued. Distinguish the two for the caller.
	_, getErr := s.Get(ctx, id)
	switch {
	case errors.Is(getErr, ErrNotFound):
		return Job{}, ErrNotFound
	case getErr != nil:
		return Job{}, getErr
	default:
		return Job{}, ErrNotRequeueable
	}
}

// Stats is a per-queue breakdown of jobs by status.
type Stats struct {
	Queue  string         `json:"queue"`
	Counts map[Status]int `json:"counts"`
	Total  int            `json:"total"`
}

// allStatuses is the fixed order/census reported by QueueStats, so every state
// appears in the output even when its count is zero.
var allStatuses = []Status{
	StatusPending, StatusRunning, StatusCompleted, StatusFailed, StatusDead,
}

const queueStatsSQL = `
SELECT status, count(*)
FROM jobs
WHERE queue = $1
GROUP BY status`

// QueueStats returns the count of jobs in each status for a queue. A queue with
// no jobs reports all-zero counts rather than an error.
func (s *Store) QueueStats(ctx context.Context, queueName string) (Stats, error) {
	rows, err := s.pool.Query(ctx, queueStatsSQL, queueName)
	if err != nil {
		return Stats{}, fmt.Errorf("queue stats: %w", err)
	}
	defer rows.Close()

	stats := Stats{Queue: queueName, Counts: make(map[Status]int, len(allStatuses))}
	for _, st := range allStatuses {
		stats.Counts[st] = 0
	}
	for rows.Next() {
		var (
			status string
			count  int
		)
		if err := rows.Scan(&status, &count); err != nil {
			return Stats{}, fmt.Errorf("scan queue stats: %w", err)
		}
		stats.Counts[Status(status)] = count
		stats.Total += count
	}
	if err := rows.Err(); err != nil {
		return Stats{}, fmt.Errorf("iterate queue stats: %w", err)
	}
	return stats, nil
}
