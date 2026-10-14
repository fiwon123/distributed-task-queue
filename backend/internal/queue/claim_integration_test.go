//go:build integration

package queue

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// testQueue returns a per-test queue name so integration tests never share rows.
func testQueue(t *testing.T) string {
	t.Helper()
	return "it_" + strings.ReplaceAll(t.Name(), "/", "_")
}

func enqueueMany(t *testing.T, store *Store, q string, n, maxAttempts int) []Job {
	t.Helper()
	ctx := context.Background()
	jobs := make([]Job, 0, n)
	for i := 0; i < n; i++ {
		job, err := store.Enqueue(ctx, EnqueueParams{
			Type:        "noop",
			Queue:       q,
			MaxAttempts: &maxAttempts,
		})
		if err != nil {
			t.Fatalf("enqueue: %v", err)
		}
		jobs = append(jobs, job)
	}
	return jobs
}

func cleanupQueue(t *testing.T, pool *pgxpool.Pool, q string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), "DELETE FROM jobs WHERE queue = $1", q); err != nil {
		t.Errorf("cleanup queue %q: %v", q, err)
	}
}

func claimOne(t *testing.T, store *Store, q string, lease time.Duration) Job {
	t.Helper()
	jobs, err := store.Claim(context.Background(), ClaimParams{
		Queues: []string{q},
		Lease:  lease,
		Limit:  1,
	})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("claim returned %d jobs, want 1", len(jobs))
	}
	return jobs[0]
}

// TestClaimConcurrentNoDoubleClaim is the FOR UPDATE SKIP LOCKED property: two
// concurrent claimers must never receive the same job.
func TestClaimConcurrentNoDoubleClaim(t *testing.T) {
	pool := testPool(t)
	store := New(pool)
	ctx := context.Background()
	q := testQueue(t)
	defer cleanupQueue(t, pool, q)

	const total = 20
	enqueueMany(t, store, q, total, 5)

	var mu sync.Mutex
	seen := map[string]int{}
	var wg sync.WaitGroup

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				jobs, err := store.Claim(ctx, ClaimParams{
					Queues: []string{q},
					Lease:  time.Minute,
					Limit:  5,
				})
				if err != nil {
					t.Errorf("claim: %v", err)
					return
				}
				if len(jobs) == 0 {
					return
				}
				for _, job := range jobs {
					mu.Lock()
					seen[job.ID]++
					mu.Unlock()
					if _, err := store.Complete(ctx, job.ID, *job.LockedUntil); err != nil {
						t.Errorf("complete %s: %v", job.ID, err)
					}
				}
			}
		}()
	}
	wg.Wait()

	if len(seen) != total {
		t.Fatalf("claimed %d distinct jobs, want %d", len(seen), total)
	}
	for id, n := range seen {
		if n != 1 {
			t.Errorf("job %s claimed %d times, want 1", id, n)
		}
	}
}

// TestExtendLeaseKeepsJobHidden covers the heartbeat store call: renewing the
// lease before it lapses keeps a job claimed past its original lease, the
// renewed fence is what completes it, and renewal only works while the caller
// still holds the lease.
func TestExtendLeaseKeepsJobHidden(t *testing.T) {
	pool := testPool(t)
	store := New(pool)
	ctx := context.Background()
	q := testQueue(t)
	defer cleanupQueue(t, pool, q)

	maxAttempts := 5
	job, err := store.Enqueue(ctx, EnqueueParams{Type: "noop", Queue: q, MaxAttempts: &maxAttempts})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	claimed := claimOne(t, store, q, 100*time.Millisecond)
	if claimed.ID != job.ID {
		t.Fatalf("claimed %s, want %s", claimed.ID, job.ID)
	}

	// Renew with a generous lease before the original one lapses.
	renewed, err := store.ExtendLease(ctx, claimed.ID, *claimed.LockedUntil, 30*time.Second)
	if err != nil {
		t.Fatalf("extend lease: %v", err)
	}
	if !renewed.After(*claimed.LockedUntil) {
		t.Errorf("renewed lease %v not after original %v", renewed, *claimed.LockedUntil)
	}

	// Past the original lease but well within the renewed one: not claimable.
	time.Sleep(250 * time.Millisecond)
	jobs, err := store.Claim(ctx, ClaimParams{Queues: []string{q}, Lease: time.Minute, Limit: 1})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("renewed job was reclaimed, got %d jobs", len(jobs))
	}

	// The original fence is stale; the renewed fence completes the job.
	if _, err := store.Complete(ctx, job.ID, *claimed.LockedUntil); !errors.Is(err, ErrLeaseLost) {
		t.Errorf("complete with original fence = %v, want ErrLeaseLost", err)
	}
	if _, err := store.Complete(ctx, job.ID, renewed); err != nil {
		t.Fatalf("complete with renewed fence: %v", err)
	}

	// Renewal only works while the job is running.
	if _, err := store.ExtendLease(ctx, job.ID, renewed, time.Minute); !errors.Is(err, ErrLeaseLost) {
		t.Errorf("extend after complete = %v, want ErrLeaseLost", err)
	}
}

// TestClaimReclaimsExpiredLease covers a worker that dies mid-job: after its
// lease expires the job is claimable again and the attempt is counted.
func TestClaimReclaimsExpiredLease(t *testing.T) {
	pool := testPool(t)
	store := New(pool)
	ctx := context.Background()
	q := testQueue(t)
	defer cleanupQueue(t, pool, q)

	maxAttempts := 5
	job, err := store.Enqueue(ctx, EnqueueParams{Type: "noop", Queue: q, MaxAttempts: &maxAttempts})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	first := claimOne(t, store, q, 100*time.Millisecond)
	if first.ID != job.ID || first.Attempts != 1 {
		t.Fatalf("first claim = %s attempts %d, want %s attempts 1", first.ID, first.Attempts, job.ID)
	}

	// Deliberately never complete; let the lease lapse.
	time.Sleep(250 * time.Millisecond)

	second := claimOne(t, store, q, time.Minute)
	if second.ID != job.ID {
		t.Errorf("reclaimed job %s, want %s", second.ID, job.ID)
	}
	if second.Attempts != 2 {
		t.Errorf("attempts after reclaim = %d, want 2", second.Attempts)
	}
}

// TestClaimReapsExhaustedExpiredLease covers a worker that dies on the job's
// final attempt: the job must not stay 'running' forever.
func TestClaimReapsExhaustedExpiredLease(t *testing.T) {
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

	claimed := claimOne(t, store, q, 100*time.Millisecond)
	if claimed.Attempts != 1 {
		t.Fatalf("attempts = %d, want 1", claimed.Attempts)
	}

	time.Sleep(250 * time.Millisecond)

	// The next claim reaps the exhausted lease and claims nothing.
	jobs, err := store.Claim(ctx, ClaimParams{Queues: []string{q}, Lease: time.Minute, Limit: 1})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("reaped job should not be claimable, got %d jobs", len(jobs))
	}

	got, err := store.Get(ctx, job.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != StatusDead {
		t.Errorf("status = %q, want %q", got.Status, StatusDead)
	}
	if got.CompletedAt == nil {
		t.Error("dead-lettered job should have completed_at set")
	}
}

// TestCompleteThenFailLifecycle walks success, retry, and dead-letter.
func TestCompleteThenFailLifecycle(t *testing.T) {
	pool := testPool(t)
	store := New(pool)
	ctx := context.Background()
	q := testQueue(t)
	defer cleanupQueue(t, pool, q)

	maxAttempts := 2
	// Success path.
	okJob, err := store.Enqueue(ctx, EnqueueParams{Type: "noop", Queue: q, MaxAttempts: &maxAttempts})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	claimed := claimOne(t, store, q, time.Minute)
	completed, err := store.Complete(ctx, claimed.ID, *claimed.LockedUntil)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if completed.Status != StatusCompleted || completed.CompletedAt == nil {
		t.Errorf("completed = %+v", completed)
	}
	if _, err := store.Get(ctx, okJob.ID); err != nil {
		t.Fatalf("get completed: %v", err)
	}

	// Retry then dead-letter path.
	failJob, err := store.Enqueue(ctx, EnqueueParams{Type: "noop", Queue: q, MaxAttempts: &maxAttempts})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	first := claimOne(t, store, q, time.Minute)
	failed, err := store.Fail(ctx, FailParams{
		ID:      first.ID,
		Cause:   "boom",
		RetryAt: time.Now().Add(-time.Second), // due immediately
		Fence:   *first.LockedUntil,
	})
	if err != nil {
		t.Fatalf("fail: %v", err)
	}
	if failed.Status != StatusFailed {
		t.Fatalf("status after first fail = %q, want %q", failed.Status, StatusFailed)
	}
	if failed.Attempts != 1 || failed.LastError == nil || *failed.LastError != "boom" {
		t.Errorf("failed = %+v", failed)
	}

	second := claimOne(t, store, q, time.Minute)
	if second.ID != failJob.ID || second.Attempts != 2 {
		t.Fatalf("second claim = %s attempts %d, want %s attempts 2", second.ID, second.Attempts, failJob.ID)
	}
	dead, err := store.Fail(ctx, FailParams{
		ID:      second.ID,
		Cause:   "boom again",
		RetryAt: time.Now().Add(time.Minute),
		Fence:   *second.LockedUntil,
	})
	if err != nil {
		t.Fatalf("fail: %v", err)
	}
	if dead.Status != StatusDead || dead.CompletedAt == nil {
		t.Errorf("dead-lettered = %+v", dead)
	}
}

// TestFencedUpdatesRejectStaleLease covers the lease fence.
func TestFencedUpdatesRejectStaleLease(t *testing.T) {
	pool := testPool(t)
	store := New(pool)
	ctx := context.Background()
	q := testQueue(t)
	defer cleanupQueue(t, pool, q)

	maxAttempts := 3
	job, err := store.Enqueue(ctx, EnqueueParams{Type: "noop", Queue: q, MaxAttempts: &maxAttempts})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	claimed := claimOne(t, store, q, time.Minute)

	// A fence that does not match the current lease must be rejected.
	if _, err := store.Complete(ctx, job.ID, claimed.LockedUntil.Add(time.Second)); !errors.Is(err, ErrLeaseLost) {
		t.Errorf("stale-fence complete error = %v, want ErrLeaseLost", err)
	}
	if _, err := store.Fail(ctx, FailParams{
		ID: job.ID, Cause: "x", RetryAt: time.Now(), Fence: claimed.LockedUntil.Add(time.Second),
	}); !errors.Is(err, ErrLeaseLost) {
		t.Errorf("stale-fence fail error = %v, want ErrLeaseLost", err)
	}

	// The correct fence succeeds...
	if _, err := store.Complete(ctx, job.ID, *claimed.LockedUntil); err != nil {
		t.Fatalf("complete with correct fence: %v", err)
	}
	// ...and a second completion finds no running row.
	if _, err := store.Complete(ctx, job.ID, *claimed.LockedUntil); !errors.Is(err, ErrLeaseLost) {
		t.Errorf("second complete error = %v, want ErrLeaseLost", err)
	}
}

// TestClaimPrefersPriorityThenRunAt verifies the claim ordering: higher
// priority first, run_at breaking ties. jobD is due earliest but has the lowest
// priority, so it must be claimed last.
func TestClaimPrefersPriorityThenRunAt(t *testing.T) {
	pool := testPool(t)
	store := New(pool)
	ctx := context.Background()
	q := testQueue(t)
	defer cleanupQueue(t, pool, q)

	now := time.Now()
	enqueueAt := func(typ string, priority int, runAt time.Time) Job {
		t.Helper()
		job, err := store.Enqueue(ctx, EnqueueParams{
			Type:     typ,
			Queue:    q,
			Priority: intPtr(priority),
			RunAt:    &runAt,
		})
		if err != nil {
			t.Fatalf("enqueue %s: %v", typ, err)
		}
		if job.Priority != priority {
			t.Fatalf("%s priority = %d, want %d", typ, job.Priority, priority)
		}
		return job
	}
	jobA := enqueueAt("a", 1, now.Add(-3*time.Second))
	jobB := enqueueAt("b", 5, now.Add(-2*time.Second))
	jobC := enqueueAt("c", 5, now.Add(-1*time.Second))
	jobD := enqueueAt("d", 0, now.Add(-4*time.Second))

	claimed, err := store.Claim(ctx, ClaimParams{Queues: []string{q}, Lease: time.Minute, Limit: 4})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	got := make([]string, 0, len(claimed))
	for _, job := range claimed {
		got = append(got, job.ID)
	}
	want := []string{jobB.ID, jobC.ID, jobA.ID, jobD.ID}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("claim order = %v, want %v (priority DESC, run_at ASC)", got, want)
	}
}

// TestClaimLimitTakesHighestPriority guards the SQL side of the ordering: with
// more due jobs than the limit, the CTE's ORDER BY + LIMIT must pick the
// highest-priority jobs (not the in-memory sort, which would reorder whatever
// rows came back). If the SQL ordering regressed to ascend, this test fails
// while TestClaimPrefersPriorityThenRunAt would still pass.
func TestClaimLimitTakesHighestPriority(t *testing.T) {
	pool := testPool(t)
	store := New(pool)
	ctx := context.Background()
	q := testQueue(t)
	defer cleanupQueue(t, pool, q)

	now := time.Now()
	// Five due jobs with priorities 0..4; only the two highest may win.
	ids := make([]string, 5)
	for p := 0; p < 5; p++ {
		job, err := store.Enqueue(ctx, EnqueueParams{
			Type:     fmt.Sprintf("p%d", p),
			Queue:    q,
			Priority: intPtr(p),
			RunAt:    &now,
		})
		if err != nil {
			t.Fatalf("enqueue priority %d: %v", p, err)
		}
		ids[p] = job.ID
	}

	claimed, err := store.Claim(ctx, ClaimParams{Queues: []string{q}, Lease: time.Minute, Limit: 2})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(claimed) != 2 {
		t.Fatalf("claimed %d jobs, want 2", len(claimed))
	}
	if claimed[0].ID != ids[4] || claimed[1].ID != ids[3] {
		t.Errorf("limit 2 claimed priorities %d/%d, want 4/3 (SQL ordering)", claimed[0].Priority, claimed[1].Priority)
	}
}

func TestClaimSkipsFutureRunAt(t *testing.T) {
	pool := testPool(t)
	store := New(pool)
	ctx := context.Background()
	q := testQueue(t)
	defer cleanupQueue(t, pool, q)

	future := time.Now().Add(time.Hour)
	maxAttempts := 3
	if _, err := store.Enqueue(ctx, EnqueueParams{
		Type: "noop", Queue: q, RunAt: &future, MaxAttempts: &maxAttempts,
	}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	jobs, err := store.Claim(ctx, ClaimParams{Queues: []string{q}, Lease: time.Minute, Limit: 1})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("future job should not be claimable, got %d jobs", len(jobs))
	}
}
