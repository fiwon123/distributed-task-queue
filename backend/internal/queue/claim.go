package queue

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrLeaseLost is returned when a worker tries to complete or fail a job whose
// lease it no longer holds — the lease expired and another worker reclaimed it.
// The late worker must not touch the row.
var ErrLeaseLost = errors.New("job lease lost")

// ClaimParams selects due jobs to claim.
type ClaimParams struct {
	// Queues to draw from. At least one is required.
	Queues []string
	// Lease is how long the claim hides the job before it can be reclaimed.
	Lease time.Duration
	// Limit caps how many jobs a single claim returns.
	Limit int
}

// FailParams records a failed attempt against a claimed job.
type FailParams struct {
	// ID of the job.
	ID string
	// Cause is stored as last_error.
	Cause string
	// RetryAt is when the job should next become claimable. Ignored when the
	// job has exhausted max_attempts and is dead-lettered instead.
	RetryAt time.Time
	// Fence is the locked_until value the worker received from Claim; the update
	// only applies if the lease is still the worker's.
	Fence time.Time
}

// reapExhaustedSQL dead-letters jobs whose worker died on their final attempt:
// the lease expired and there is no attempt left to give them.
const reapExhaustedSQL = `
UPDATE jobs
SET status       = 'dead',
    locked_until = NULL,
    completed_at = now(),
    last_error   = COALESCE(last_error, 'lease expired after final attempt')
WHERE status = 'running'
  AND locked_until <= now()
  AND attempts >= max_attempts`

// claimSQL atomically claims up to $2 due jobs from $1, hides them until the
// lease ($3 seconds) expires, and counts the attempt. A job is due when it is
// pending/failed and run_at has passed, or when it is running under an expired
// lease (reclaim). SKIP LOCKED lets concurrent claimers pass over rows already
// locked by another worker instead of blocking.
const claimSQL = `
WITH due AS (
    SELECT id
    FROM jobs
    WHERE queue = ANY($1::text[])
      AND attempts < max_attempts
      AND (
            (status IN ('pending', 'failed') AND run_at <= now())
         OR (status = 'running' AND locked_until <= now())
      )
    ORDER BY run_at
    FOR UPDATE SKIP LOCKED
    LIMIT $2
)
UPDATE jobs AS j
SET status       = 'running',
    attempts     = j.attempts + 1,
    locked_until = now() + make_interval(secs => $3::double precision)
FROM due
WHERE j.id = due.id
RETURNING j.id::text, j.type, j.payload, j.queue, j.status, j.attempts,
          j.max_attempts, j.run_at, j.locked_until, j.last_error,
          j.created_at, j.completed_at`

// Claim marks up to Limit due jobs running and returns them. It is safe to run
// concurrently from many workers: FOR UPDATE SKIP LOCKED guarantees no job is
// returned to two callers. Expired-lease jobs that exhausted their attempts are
// dead-lettered first so they cannot linger in 'running' forever.
func (s *Store) Claim(ctx context.Context, p ClaimParams) ([]Job, error) {
	if len(p.Queues) == 0 {
		return nil, &ValidationError{Message: "at least one queue is required"}
	}
	if p.Limit < 1 {
		return nil, &ValidationError{Message: "limit must be at least 1"}
	}
	if p.Lease <= 0 {
		return nil, &ValidationError{Message: "lease must be positive"}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin claim: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once committed

	if _, err := tx.Exec(ctx, reapExhaustedSQL); err != nil {
		return nil, fmt.Errorf("reap exhausted leases: %w", err)
	}

	rows, err := tx.Query(ctx, claimSQL, p.Queues, p.Limit, p.Lease.Seconds())
	if err != nil {
		return nil, fmt.Errorf("claim jobs: %w", err)
	}
	jobs, err := collectJobs(rows)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit claim: %w", err)
	}
	return jobs, nil
}

const completeSQL = `
UPDATE jobs
SET status       = 'completed',
    locked_until = NULL,
    completed_at = now()
WHERE id = $1 AND status = 'running' AND locked_until = $2
RETURNING id::text, type, payload, queue, status, attempts, max_attempts,
          run_at, locked_until, last_error, created_at, completed_at`

// Complete marks a claimed job completed. fence is the locked_until the worker
// got from Claim; if the lease was reclaimed the update matches no row and
// ErrLeaseLost is returned.
func (s *Store) Complete(ctx context.Context, id string, fence time.Time) (Job, error) {
	return s.fencedUpdate(ctx, completeSQL, id, fence)
}

const failSQL = `
UPDATE jobs
SET status       = CASE WHEN attempts >= max_attempts THEN 'dead' ELSE 'failed' END,
    run_at       = CASE WHEN attempts >= max_attempts THEN run_at ELSE $3 END,
    last_error   = $4,
    locked_until = NULL,
    completed_at = CASE WHEN attempts >= max_attempts THEN now() ELSE NULL END
WHERE id = $1 AND status = 'running' AND locked_until = $2
RETURNING id::text, type, payload, queue, status, attempts, max_attempts,
          run_at, locked_until, last_error, created_at, completed_at`

// Fail records a failed attempt. While the job has attempts left it is
// rescheduled for RetryAt; once attempts reach max_attempts it is dead-lettered.
// Like Complete, it is fenced on the lease.
func (s *Store) Fail(ctx context.Context, p FailParams) (Job, error) {
	return s.fencedUpdate(ctx, failSQL, p.ID, p.Fence, p.RetryAt, p.Cause)
}

const extendLeaseSQL = `
UPDATE jobs
SET locked_until = now() + make_interval(secs => $3::double precision)
WHERE id = $1 AND status = 'running' AND locked_until = $2
RETURNING locked_until`

// ExtendLease renews the lease on a running job the caller still holds, so a
// handler that outlives its original lease is not reclaimed while it is still
// running. fence is the locked_until the worker last saw; the update only
// applies while the worker still holds the lease, and the returned value is the
// new fence the worker must use for its terminal Complete/Fail. When the lease
// lapsed and another worker reclaimed the job, ErrLeaseLost is returned.
func (s *Store) ExtendLease(ctx context.Context, id string, fence time.Time, lease time.Duration) (time.Time, error) {
	if lease <= 0 {
		return time.Time{}, &ValidationError{Message: "lease must be positive"}
	}

	var newFence time.Time
	err := s.pool.QueryRow(ctx, extendLeaseSQL, id, fence, lease.Seconds()).Scan(&newFence)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return time.Time{}, ErrLeaseLost
		}
		return time.Time{}, fmt.Errorf("extend lease: %w", err)
	}
	return newFence, nil
}

// fencedUpdate runs an UPDATE ... RETURNING that is conditional on the caller
// still holding the lease. A missing row means the lease was lost.
func (s *Store) fencedUpdate(ctx context.Context, sql, id string, args ...any) (Job, error) {
	full := append([]any{id}, args...)
	job, err := scanJob(s.pool.QueryRow(ctx, sql, full...))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Job{}, ErrLeaseLost
		}
		return Job{}, err
	}
	return job, nil
}

// collectJobs drains rows into a slice, closing them first.
func collectJobs(rows pgx.Rows) ([]Job, error) {
	defer rows.Close()
	jobs := make([]Job, 0)
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate jobs: %w", err)
	}
	return jobs, nil
}
