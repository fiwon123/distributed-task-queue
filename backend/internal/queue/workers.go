// Package queue helpers for worker registry.
package queue

import (
	"context"
		"fmt"
	"strings"
	"time"

		)

// WorkerInfo describes a running worker process.
type WorkerInfo struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Queues      []string   `json:"queues"`
	Types       []string   `json:"types"`
	Concurrency int        `json:"concurrency"`
	StartedAt   time.Time  `json:"started_at"`
	LastSeenAt  time.Time  `json:"last_seen_at"`
	Alive       bool       `json:"alive"`
}

// UpsertWorker registers or updates a worker's heartbeat.
func (s *Store) UpsertWorker(ctx context.Context, id, name string, queues, types []string, concurrency int, startedAt *time.Time) (WorkerInfo, error) {
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	if name == "" {
		return WorkerInfo{}, &ValidationError{Message: "worker name required"}
	}
	if concurrency < 1 {
		concurrency = 1
	}
	if startedAt == nil {
		now := time.Now()
		startedAt = &now
	}
	rows, err := s.pool.Query(ctx, `INSERT INTO workers (id, name, queues, types, concurrency, started_at, last_seen_at)
		VALUES (COALESCE(NULLIF($1,''), gen_random_uuid()), $2, $3::text[], $4::text[], $5, $6, now())
		ON CONFLICT (id) DO UPDATE SET name=EXCLUDED.name, queues=EXCLUDED.queues, types=EXCLUDED.types, concurrency=EXCLUDED.concurrency, last_seen_at=now()
		RETURNING id::text, name, queues::text[], types::text[], concurrency, started_at, last_seen_at`,
		id, name, queues, types, concurrency, startedAt)
	if err != nil {
		return WorkerInfo{}, fmt.Errorf("upsert worker: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return WorkerInfo{}, fmt.Errorf("upsert worker returned no rows")
	}
	var w WorkerInfo
	var qarr, tarr []string
	if err := rows.Scan(&w.ID, &w.Name, &qarr, &tarr, &w.Concurrency, &w.StartedAt, &w.LastSeenAt); err != nil {
		return WorkerInfo{}, err
	}
	w.Queues = qarr
	w.Types = tarr
	w.Alive = true
	_ = rows.Err()
	return w, nil
}

// ListWorkers returns registered workers.
func (s *Store) ListWorkers(ctx context.Context) ([]WorkerInfo, error) {
	rows, err := s.pool.Query(ctx, `SELECT id::text, name, queues::text[], types::text[], concurrency, started_at, last_seen_at, (last_seen_at > now() - interval '30 seconds')
		FROM workers
		ORDER BY last_seen_at DESC, name`)
	if err != nil {
		return nil, fmt.Errorf("list workers: %w", err)
	}
	defer rows.Close()
	var out []WorkerInfo
	for rows.Next() {
		var w WorkerInfo
		var qarr, tarr []string
		if err := rows.Scan(&w.ID, &w.Name, &qarr, &tarr, &w.Concurrency, &w.StartedAt, &w.LastSeenAt, &w.Alive); err != nil {
			return nil, err
		}
		w.Queues = qarr
		w.Types = tarr
		out = append(out, w)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteWorker removes a worker registration.
func (s *Store) DeleteWorker(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return &ValidationError{Message: "worker id required"}
	}
	_, err := s.pool.Exec(ctx, "DELETE FROM workers WHERE id::text = $1", id)
	if err != nil {
		return fmt.Errorf("delete worker: %w", err)
	}
	return nil
}
