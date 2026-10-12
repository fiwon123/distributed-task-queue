//go:build integration

package queue

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fiwon123/distributed-task-queue/backend/migrations"
)

// testPool connects to the Postgres named by DATABASE_URL, applies migrations,
// and skips the test when no database is configured. Run with:
//
//	go test -tags integration ./...
func testPool(t *testing.T) *pgxpool.Pool {
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
	if err := Migrate(ctx, pool, migrations.FS); err != nil {
		pool.Close()
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestMigrateIsIdempotent(t *testing.T) {
	pool := testPool(t)

	// A second run must be a no-op rather than an error.
	if err := Migrate(context.Background(), pool, migrations.FS); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}

func TestStoreEnqueueAndGet(t *testing.T) {
	pool := testPool(t)
	store := New(pool)
	ctx := context.Background()

	runAt := time.Now().Add(time.Hour).UTC().Truncate(time.Microsecond)
	maxAttempts := 7

	job, err := store.Enqueue(ctx, EnqueueParams{
		Type:        "email",
		Payload:     json.RawMessage(`{"to":"a@b.c","n":1}`),
		Queue:       "integration",
		RunAt:       &runAt,
		MaxAttempts: &maxAttempts,
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM jobs WHERE id = $1", job.ID)
	})

	if job.ID == "" {
		t.Fatal("expected a server-generated id")
	}
	if job.Status != StatusPending {
		t.Errorf("status = %q, want %q", job.Status, StatusPending)
	}
	if job.Queue != "integration" {
		t.Errorf("queue = %q, want %q", job.Queue, "integration")
	}
	if job.MaxAttempts != 7 {
		t.Errorf("max_attempts = %d, want 7", job.MaxAttempts)
	}
	if job.Attempts != 0 {
		t.Errorf("attempts = %d, want 0", job.Attempts)
	}
	if !job.RunAt.Equal(runAt) {
		t.Errorf("run_at = %v, want %v", job.RunAt, runAt)
	}
	if job.LockedUntil != nil || job.LastError != nil || job.CompletedAt != nil {
		t.Errorf("new job should have null lease/error/completed_at, got %+v", job)
	}

	assertJSONEqual(t, job.Payload, `{"to":"a@b.c","n":1}`)

	got, err := store.Get(ctx, job.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != job.ID || got.Type != job.Type || got.Queue != job.Queue {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, job)
	}

	if _, err := store.Get(ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, ErrNotFound) {
		t.Errorf("get unknown id error = %v, want ErrNotFound", err)
	}
}

func TestStoreEnqueueValidation(t *testing.T) {
	pool := testPool(t)
	store := New(pool)

	_, err := store.Enqueue(context.Background(), EnqueueParams{})
	var invalid *ValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("error = %v, want *ValidationError", err)
	}
}

// assertJSONEqual compares two JSON documents for semantic equality; jsonb
// normalizes whitespace and key order, so byte comparison would be brittle.
func assertJSONEqual(t *testing.T, got json.RawMessage, want string) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal(got, &g); err != nil {
		t.Fatalf("decode payload %s: %v", got, err)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("decode want %s: %v", want, err)
	}
	if !reflect.DeepEqual(g, w) {
		t.Errorf("payload = %s, want %s", got, want)
	}
}
