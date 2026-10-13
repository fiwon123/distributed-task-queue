//go:build integration

package worker

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fiwon123/distributed-task-queue/backend/internal/queue"
	"github.com/fiwon123/distributed-task-queue/backend/migrations"
)

// integrationStore connects to the Postgres named by DATABASE_URL, applies
// migrations, and skips the test when no database is configured. Run with:
//
//	go test -tags integration ./...
func integrationStore(t *testing.T) *queue.Store {
	t.Helper()

	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL is not set; skipping Postgres integration test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping: %v", err)
	}
	if err := queue.Migrate(ctx, pool, migrations.FS); err != nil {
		pool.Close()
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(pool.Close)
	return queue.New(pool)
}

// TestProcessIntegrationTimeoutDeadLetters is the end-to-end timeout check: it
// runs a real store, claim and worker against Postgres. A sleep job with a
// one-second timeout and a single attempt must be cancelled and dead-lettered
// with a last_error that names the timeout.
func TestProcessIntegrationTimeoutDeadLetters(t *testing.T) {
	store := integrationStore(t)
	ctx := context.Background()

	// A unique queue keeps this run from claiming jobs left by a previous run.
	queueName := fmt.Sprintf("it-timeout-%d", time.Now().UnixNano())

	timeout := 1
	maxAttempts := 1
	job, err := store.Enqueue(ctx, queue.EnqueueParams{
		Type:           "sleep",
		Queue:          queueName,
		MaxAttempts:    &maxAttempts,
		TimeoutSeconds: &timeout,
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if job.TimeoutSeconds == nil || *job.TimeoutSeconds != timeout {
		t.Fatalf("enqueued TimeoutSeconds = %v, want %d", job.TimeoutSeconds, timeout)
	}

	claimed, err := store.Claim(ctx, queue.ClaimParams{
		Queues: []string{queueName},
		Lease:  30 * time.Second,
		Limit:  1,
	})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("claimed %d jobs, want 1", len(claimed))
	}
	if claimed[0].TimeoutSeconds == nil || *claimed[0].TimeoutSeconds != timeout {
		t.Fatalf("claimed TimeoutSeconds = %v, want %d", claimed[0].TimeoutSeconds, timeout)
	}

	w := New(store, map[string]Handler{
		"sleep": func(ctx context.Context, _ queue.Job) error {
			select {
			case <-time.After(10 * time.Second):
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}, Config{Lease: 30 * time.Second}, discardLogger())

	start := time.Now()
	w.process(ctx, 0, claimed[0])
	if elapsed := time.Since(start); elapsed > 6*time.Second {
		t.Fatalf("process took %v; the per-job timeout did not fire", elapsed)
	}

	got, err := store.Get(ctx, job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if got.Status != queue.StatusDead {
		t.Fatalf("status = %q, want dead (max_attempts=1)", got.Status)
	}
	if got.LastError == nil || !strings.Contains(*got.LastError, "execution timed out") {
		t.Errorf("last_error = %v, want it to mention the timeout", got.LastError)
	}
}
