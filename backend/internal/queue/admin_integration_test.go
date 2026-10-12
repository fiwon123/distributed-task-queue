//go:build integration

package queue

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestRequeueDeadJobRevives walks a job to dead-letter and back: requeue resets
// the retry budget and clears the lease/error so the job is claimable again.
func TestRequeueDeadJobRevives(t *testing.T) {
	pool := testPool(t)
	store := New(pool)
	ctx := context.Background()
	q := testQueue(t)
	defer cleanupQueue(t, pool, q)

	maxAttempts := 1
	job, err := store.Enqueue(ctx, EnqueueParams{Type: "noop", Queue: q, MaxAttempts: &maxAttempts})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	claimed := claimOne(t, store, q, time.Minute)
	dead, err := store.Fail(ctx, FailParams{
		ID:      claimed.ID,
		Cause:   "boom",
		RetryAt: time.Now(),
		Fence:   *claimed.LockedUntil,
	})
	if err != nil {
		t.Fatalf("fail: %v", err)
	}
	if dead.Status != StatusDead {
		t.Fatalf("status = %q, want %q", dead.Status, StatusDead)
	}

	revived, err := store.Requeue(ctx, job.ID)
	if err != nil {
		t.Fatalf("requeue: %v", err)
	}
	if revived.Status != StatusPending {
		t.Errorf("status = %q, want %q", revived.Status, StatusPending)
	}
	if revived.Attempts != 0 {
		t.Errorf("attempts = %d, want 0 (fresh retry budget)", revived.Attempts)
	}
	if revived.LockedUntil != nil {
		t.Errorf("locked_until = %v, want nil", revived.LockedUntil)
	}
	if revived.LastError != nil {
		t.Errorf("last_error = %v, want nil", revived.LastError)
	}
	if revived.CompletedAt != nil {
		t.Errorf("completed_at = %v, want nil", revived.CompletedAt)
	}

	// The requeued job is claimable again.
	again := claimOne(t, store, q, time.Minute)
	if again.ID != job.ID || again.Attempts != 1 {
		t.Fatalf("reclaimed = %s attempts %d, want %s attempts 1", again.ID, again.Attempts, job.ID)
	}
	if _, err := store.Complete(ctx, again.ID, *again.LockedUntil); err != nil {
		t.Fatalf("complete after requeue: %v", err)
	}
}

// TestRequeueFailedJob verifies a job that failed but is not yet dead (retry
// pending) can also be requeued.
func TestRequeueFailedJob(t *testing.T) {
	pool := testPool(t)
	store := New(pool)
	ctx := context.Background()
	q := testQueue(t)
	defer cleanupQueue(t, pool, q)

	maxAttempts := 3
	if _, err := store.Enqueue(ctx, EnqueueParams{Type: "noop", Queue: q, MaxAttempts: &maxAttempts}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	claimed := claimOne(t, store, q, time.Minute)
	failed, err := store.Fail(ctx, FailParams{
		ID:      claimed.ID,
		Cause:   "boom",
		RetryAt: time.Now().Add(time.Hour), // not due yet
		Fence:   *claimed.LockedUntil,
	})
	if err != nil {
		t.Fatalf("fail: %v", err)
	}
	if failed.Status != StatusFailed {
		t.Fatalf("status = %q, want %q", failed.Status, StatusFailed)
	}

	revived, err := store.Requeue(ctx, failed.ID)
	if err != nil {
		t.Fatalf("requeue: %v", err)
	}
	if revived.Status != StatusPending || revived.Attempts != 0 || revived.LastError != nil {
		t.Errorf("revived = %+v, want pending/attempts=0/last_error=nil", revived)
	}
	if time.Until(revived.RunAt) > time.Minute {
		t.Errorf("run_at = %v, want ~now (immediately due)", revived.RunAt)
	}
}

// TestRequeueRejectsNonRequeueable covers unknown ids and jobs in states that
// cannot be requeued.
func TestRequeueRejectsNonRequeueable(t *testing.T) {
	pool := testPool(t)
	store := New(pool)
	ctx := context.Background()
	q := testQueue(t)
	defer cleanupQueue(t, pool, q)

	if _, err := store.Requeue(ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, ErrNotFound) {
		t.Errorf("requeue unknown id error = %v, want ErrNotFound", err)
	}

	if _, err := store.Enqueue(ctx, EnqueueParams{Type: "noop", Queue: q}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	// A pending job cannot be requeued.
	jobs, err := store.Claim(ctx, ClaimParams{Queues: []string{q}, Lease: time.Minute, Limit: 1})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	running := jobs[0]
	if _, err := store.Requeue(ctx, running.ID); !errors.Is(err, ErrNotRequeueable) {
		t.Errorf("requeue running job error = %v, want ErrNotRequeueable", err)
	}

	// A completed job cannot be requeued either.
	if _, err := store.Complete(ctx, running.ID, *running.LockedUntil); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if _, err := store.Requeue(ctx, running.ID); !errors.Is(err, ErrNotRequeueable) {
		t.Errorf("requeue completed job error = %v, want ErrNotRequeueable", err)
	}
}

// TestRequeueDeadBulk checks that bulk requeue revives every dead-lettered job
// in a queue, leaves other jobs alone, and is idempotent.
func TestRequeueDeadBulk(t *testing.T) {
	pool := testPool(t)
	store := New(pool)
	ctx := context.Background()
	q := testQueue(t)
	defer cleanupQueue(t, pool, q)

	maxAttempts := 1
	// Two jobs that exhaust their single attempt and become dead.
	deadIDs := make([]string, 0, 2)
	for i := 0; i < 2; i++ {
		job, err := store.Enqueue(ctx, EnqueueParams{Type: "noop", Queue: q, MaxAttempts: &maxAttempts})
		if err != nil {
			t.Fatalf("enqueue: %v", err)
		}
		claimed := claimOne(t, store, q, time.Minute)
		if claimed.ID != job.ID {
			t.Fatalf("claimed %s, want %s", claimed.ID, job.ID)
		}
		dead, err := store.Fail(ctx, FailParams{
			ID: claimed.ID, Cause: "boom", RetryAt: time.Now(), Fence: *claimed.LockedUntil,
		})
		if err != nil {
			t.Fatalf("fail: %v", err)
		}
		if dead.Status != StatusDead {
			t.Fatalf("status = %q, want %q", dead.Status, StatusDead)
		}
		deadIDs = append(deadIDs, job.ID)
	}
	// A pending job that bulk requeue must leave untouched.
	pending, err := store.Enqueue(ctx, EnqueueParams{Type: "noop", Queue: q})
	if err != nil {
		t.Fatalf("enqueue pending: %v", err)
	}

	n, err := store.RequeueDead(ctx, q)
	if err != nil {
		t.Fatalf("requeue dead: %v", err)
	}
	if n != 2 {
		t.Fatalf("requeued = %d, want 2", n)
	}

	for _, id := range deadIDs {
		got, err := store.Get(ctx, id)
		if err != nil {
			t.Fatalf("get %s: %v", id, err)
		}
		if got.Status != StatusPending || got.Attempts != 0 || got.LockedUntil != nil || got.LastError != nil {
			t.Errorf("revived %s = %+v, want pending/attempts=0/no lease/no error", id, got)
		}
	}

	got, err := store.Get(ctx, pending.ID)
	if err != nil {
		t.Fatalf("get pending: %v", err)
	}
	if got.Status != StatusPending || got.Attempts != 0 {
		t.Errorf("pending job = %+v, want untouched pending/attempts=0", got)
	}

	// Idempotent: no dead jobs remain, so a second call revives nothing.
	again, err := store.RequeueDead(ctx, q)
	if err != nil {
		t.Fatalf("requeue dead again: %v", err)
	}
	if again != 0 {
		t.Errorf("second requeue dead = %d, want 0", again)
	}
}

// TestRequeueDeadUnknownQueueRevivesNothing verifies a queue with no dead jobs
// reports 0 rather than an error.
func TestRequeueDeadUnknownQueueRevivesNothing(t *testing.T) {
	pool := testPool(t)
	store := New(pool)
	ctx := context.Background()
	q := testQueue(t) + "_empty"
	defer cleanupQueue(t, pool, q)

	n, err := store.RequeueDead(ctx, q)
	if err != nil {
		t.Fatalf("requeue dead: %v", err)
	}
	if n != 0 {
		t.Errorf("requeued = %d, want 0", n)
	}
}

// TestQueueStatsCountsByStatus checks the per-status census includes a job in
// each state and zero-fills the rest.
func TestQueueStatsCountsByStatus(t *testing.T) {
	pool := testPool(t)
	store := New(pool)
	ctx := context.Background()
	q := testQueue(t)
	defer cleanupQueue(t, pool, q)

	maxAttempts := 2
	// completed
	if _, err := store.Enqueue(ctx, EnqueueParams{Type: "noop", Queue: q, MaxAttempts: &maxAttempts}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	a := claimOne(t, store, q, time.Minute)
	if _, err := store.Complete(ctx, a.ID, *a.LockedUntil); err != nil {
		t.Fatalf("complete: %v", err)
	}
	// failed (retry scheduled in the future)
	if _, err := store.Enqueue(ctx, EnqueueParams{Type: "noop", Queue: q, MaxAttempts: &maxAttempts}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	b := claimOne(t, store, q, time.Minute)
	if _, err := store.Fail(ctx, FailParams{
		ID: b.ID, Cause: "boom", RetryAt: time.Now().Add(time.Hour), Fence: *b.LockedUntil,
	}); err != nil {
		t.Fatalf("fail: %v", err)
	}
	// running (claimed, not finished)
	if _, err := store.Enqueue(ctx, EnqueueParams{Type: "noop", Queue: q, MaxAttempts: &maxAttempts}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	c := claimOne(t, store, q, time.Minute)
	if c.Status != StatusRunning {
		t.Fatalf("status = %q, want %q", c.Status, StatusRunning)
	}
	// pending (never claimed)
	if _, err := store.Enqueue(ctx, EnqueueParams{Type: "noop", Queue: q, MaxAttempts: &maxAttempts}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	stats, err := store.QueueStats(ctx, q)
	if err != nil {
		t.Fatalf("queue stats: %v", err)
	}
	want := map[Status]int{
		StatusPending:   1,
		StatusRunning:   1,
		StatusCompleted: 1,
		StatusFailed:    1,
		StatusDead:      0,
	}
	if stats.Queue != q {
		t.Errorf("queue = %q, want %q", stats.Queue, q)
	}
	if stats.Total != 4 {
		t.Errorf("total = %d, want 4", stats.Total)
	}
	for st, n := range want {
		if stats.Counts[st] != n {
			t.Errorf("counts[%s] = %d, want %d", st, stats.Counts[st], n)
		}
	}
}

// TestQueueStatsUnknownQueueIsZero verifies a queue with no jobs reports zeros
// rather than an error.
func TestQueueStatsUnknownQueueIsZero(t *testing.T) {
	pool := testPool(t)
	store := New(pool)
	ctx := context.Background()
	q := testQueue(t) + "_empty"
	defer cleanupQueue(t, pool, q)

	stats, err := store.QueueStats(ctx, q)
	if err != nil {
		t.Fatalf("queue stats: %v", err)
	}
	if stats.Total != 0 {
		t.Errorf("total = %d, want 0", stats.Total)
	}
	if len(stats.Counts) != len(AllStatuses) {
		t.Errorf("counts = %v, want all %d statuses present", stats.Counts, len(AllStatuses))
	}
	for _, st := range AllStatuses {
		if n, ok := stats.Counts[st]; !ok || n != 0 {
			t.Errorf("counts[%s] = %d (present=%v), want 0", st, n, ok)
		}
	}
}

// TestQueueStatsDeadLetter checks a dead-lettered job is counted as dead.
func TestQueueStatsDeadLetter(t *testing.T) {
	pool := testPool(t)
	store := New(pool)
	ctx := context.Background()
	q := testQueue(t)
	defer cleanupQueue(t, pool, q)

	maxAttempts := 1
	if _, err := store.Enqueue(ctx, EnqueueParams{Type: "noop", Queue: q, MaxAttempts: &maxAttempts}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	claimed := claimOne(t, store, q, time.Minute)
	if _, err := store.Fail(ctx, FailParams{
		ID: claimed.ID, Cause: "boom", RetryAt: time.Now(), Fence: *claimed.LockedUntil,
	}); err != nil {
		t.Fatalf("fail: %v", err)
	}

	stats, err := store.QueueStats(ctx, q)
	if err != nil {
		t.Fatalf("queue stats: %v", err)
	}
	if stats.Counts[StatusDead] != 1 || stats.Total != 1 {
		t.Errorf("stats = %+v, want dead=1 total=1", stats)
	}
}
