//go:build integration

package queue

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestListJobsFiltersAndOrders(t *testing.T) {
	pool := testPool(t)
	store := New(pool)
	ctx := context.Background()
	q := testQueue(t)
	defer cleanupQueue(t, pool, q)

	// Three pending jobs in q, plus one in a separate queue that must not leak.
	maxAttempts := 3
	for i := 0; i < 3; i++ {
		if _, err := store.Enqueue(ctx, EnqueueParams{Type: "noop", Queue: q, MaxAttempts: &maxAttempts}); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
	}
	otherQ := q + "_other"
	defer cleanupQueue(t, pool, otherQ)
	if _, err := store.Enqueue(ctx, EnqueueParams{Type: "noop", Queue: otherQ}); err != nil {
		t.Fatalf("enqueue other: %v", err)
	}

	all, err := store.ListJobs(ctx, ListParams{Queue: q})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("listed %d jobs, want 3", len(all))
	}
	// Newest first.
	for i := 1; i < len(all); i++ {
		if all[i].CreatedAt.After(all[i-1].CreatedAt) {
			t.Errorf("jobs not newest-first: %v then %v", all[i-1].CreatedAt, all[i].CreatedAt)
		}
	}

	// limit is honoured.
	limited, err := store.ListJobs(ctx, ListParams{Queue: q, Limit: 2})
	if err != nil {
		t.Fatalf("list limited: %v", err)
	}
	if len(limited) != 2 {
		t.Errorf("listed %d jobs, want 2", len(limited))
	}

	// Status filter: claim one so it becomes running, then filter.
	claimed := claimOne(t, store, q, time.Minute)
	running, err := store.ListJobs(ctx, ListParams{Queue: q, Status: StatusRunning})
	if err != nil {
		t.Fatalf("list running: %v", err)
	}
	if len(running) != 1 || running[0].ID != claimed.ID {
		t.Errorf("running = %+v, want only %s", running, claimed.ID)
	}
	pending, err := store.ListJobs(ctx, ListParams{Queue: q, Status: StatusPending})
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(pending) != 2 {
		t.Errorf("pending = %d, want 2", len(pending))
	}
}

func TestListJobsValidation(t *testing.T) {
	pool := testPool(t)
	store := New(pool)
	ctx := context.Background()

	_, err := store.ListJobs(ctx, ListParams{Status: "bogus"})
	var invalid *ValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("error = %v, want *ValidationError", err)
	}

	if _, err := store.ListJobs(ctx, ListParams{Limit: -1}); !errors.As(err, &invalid) {
		t.Fatalf("negative limit error = %v, want *ValidationError", err)
	}
}

func TestListQueues(t *testing.T) {
	pool := testPool(t)
	store := New(pool)
	ctx := context.Background()
	q1 := testQueue(t) + "_a"
	q2 := testQueue(t) + "_b"
	defer cleanupQueue(t, pool, q1)
	defer cleanupQueue(t, pool, q2)

	enqueueMany(t, store, q1, 2, 3)
	enqueueMany(t, store, q2, 1, 3)

	queues, err := store.ListQueues(ctx)
	if err != nil {
		t.Fatalf("list queues: %v", err)
	}
	got := map[string]int{}
	for _, qi := range queues {
		got[qi.Name] = qi.Total
	}
	if got[q1] != 2 || got[q2] != 1 {
		t.Errorf("queues = %v, want %s=2 %s=1", got, q1, q2)
	}

	// An unknown queue is simply absent, not zero-valued.
	if _, ok := got[q1+"_absent"]; ok {
		t.Errorf("unexpected queue in list: %v", got)
	}
}
