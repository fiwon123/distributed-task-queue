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
