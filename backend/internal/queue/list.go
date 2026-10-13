package queue

import (
	"context"
	"fmt"
	"strings"
)

// Listing limits. A request may ask for fewer; a request for more is clamped
// rather than rejected, and a zero limit means DefaultListLimit.
const (
	DefaultListLimit = 50
	MaxListLimit     = 500
)

// ListParams filters a job listing. All fields are optional: an empty Queue or
// Status matches everything, and a zero Limit means DefaultListLimit.
type ListParams struct {
	Queue  string
	Status Status
	Limit  int
}

// normalize validates a list request and fills in its defaults.
func (p ListParams) normalize() (ListParams, error) {
	p.Queue = strings.TrimSpace(p.Queue)
	if p.Status != "" && !p.Status.Valid() {
		return ListParams{}, &ValidationError{Message: fmt.Sprintf("unknown status %q", p.Status)}
	}
	switch {
	case p.Limit == 0:
		p.Limit = DefaultListLimit
	case p.Limit < 0:
		return ListParams{}, &ValidationError{Message: "limit must not be negative"}
	case p.Limit > MaxListLimit:
		p.Limit = MaxListLimit
	}
	return p, nil
}

const listJobsColumns = `id::text, type, payload, queue, priority, status, attempts,
       max_attempts, timeout_seconds, run_at, locked_until, last_error,
       created_at, completed_at`

// ListJobs returns jobs newest first, filtered by queue and/or status.
func (s *Store) ListJobs(ctx context.Context, p ListParams) ([]Job, error) {
	p, err := p.normalize()
	if err != nil {
		return nil, err
	}

	var (
		where []string
		args  []any
	)
	if p.Queue != "" {
		args = append(args, p.Queue)
		where = append(where, fmt.Sprintf("queue = $%d", len(args)))
	}
	if p.Status != "" {
		args = append(args, p.Status)
		where = append(where, fmt.Sprintf("status = $%d", len(args)))
	}

	var sb strings.Builder
	sb.WriteString("SELECT " + listJobsColumns + " FROM jobs")
	if len(where) > 0 {
		sb.WriteString(" WHERE " + strings.Join(where, " AND "))
	}
	args = append(args, p.Limit)
	sb.WriteString(fmt.Sprintf(" ORDER BY created_at DESC, id DESC LIMIT $%d", len(args)))

	rows, err := s.pool.Query(ctx, sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer rows.Close()

	jobs := make([]Job, 0, p.Limit)
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

// QueueInfo summarizes one queue.
type QueueInfo struct {
	Name  string `json:"name"`
	Total int    `json:"total"`
}

const listQueuesSQL = `
SELECT queue, count(*)
FROM jobs
GROUP BY queue
ORDER BY queue`

// ListQueues returns every queue that has at least one job, with its total.
func (s *Store) ListQueues(ctx context.Context) ([]QueueInfo, error) {
	rows, err := s.pool.Query(ctx, listQueuesSQL)
	if err != nil {
		return nil, fmt.Errorf("list queues: %w", err)
	}
	defer rows.Close()

	queues := make([]QueueInfo, 0)
	for rows.Next() {
		var qi QueueInfo
		if err := rows.Scan(&qi.Name, &qi.Total); err != nil {
			return nil, fmt.Errorf("scan queue: %w", err)
		}
		queues = append(queues, qi)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate queues: %w", err)
	}
	return queues, nil
}
