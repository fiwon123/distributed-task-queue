package worker

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/fiwon123/distributed-task-queue/internal/queue"
)

var testFence = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fakeStore is an in-memory Store for worker tests. No database required.
type fakeStore struct {
	mu sync.Mutex

	claimQueue  []queue.Job
	claimErr    error
	claimCalls  int
	completeErr error
	failErr     error

	completeCalls []completeCall
	failCalls     []queue.FailParams
}

type completeCall struct {
	id    string
	fence time.Time
}

func (f *fakeStore) Claim(_ context.Context, _ queue.ClaimParams) ([]queue.Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.claimCalls++
	if f.claimErr != nil {
		return nil, f.claimErr
	}
	if len(f.claimQueue) == 0 {
		return nil, nil
	}
	job := f.claimQueue[0]
	f.claimQueue = f.claimQueue[1:]
	return []queue.Job{job}, nil
}

func (f *fakeStore) Complete(_ context.Context, id string, fence time.Time) (queue.Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.completeCalls = append(f.completeCalls, completeCall{id: id, fence: fence})
	if f.completeErr != nil {
		return queue.Job{}, f.completeErr
	}
	return queue.Job{ID: id, Status: queue.StatusCompleted}, nil
}

func (f *fakeStore) Fail(_ context.Context, p queue.FailParams) (queue.Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failCalls = append(f.failCalls, p)
	if f.failErr != nil {
		return queue.Job{}, f.failErr
	}
	return queue.Job{ID: p.ID, Status: queue.StatusFailed, Attempts: 1}, nil
}

func claimedJob(id, typ string, attempts int) queue.Job {
	return queue.Job{
		ID:          id,
		Type:        typ,
		Payload:     json.RawMessage(`{}`),
		Queue:       queue.DefaultQueue,
		Status:      queue.StatusRunning,
		Attempts:    attempts,
		MaxAttempts: 3,
		LockedUntil: &testFence,
	}
}

func TestProcessSuccess(t *testing.T) {
	store := &fakeStore{}
	var got queue.Job
	w := New(store, map[string]Handler{
		"echo": func(_ context.Context, job queue.Job) error { got = job; return nil },
	}, Config{}, discardLogger())

	w.process(context.Background(), 0, claimedJob("job-1", "echo", 1))

	if got.ID != "job-1" {
		t.Fatalf("handler got job %q, want job-1", got.ID)
	}
	if len(store.completeCalls) != 1 {
		t.Fatalf("Complete calls = %d, want 1", len(store.completeCalls))
	}
	if store.completeCalls[0].id != "job-1" || !store.completeCalls[0].fence.Equal(testFence) {
		t.Errorf("Complete = %+v, want id=job-1 fence=%v", store.completeCalls[0], testFence)
	}
	if len(store.failCalls) != 0 {
		t.Errorf("Fail calls = %d, want 0", len(store.failCalls))
	}
}

func TestProcessHandlerErrorSchedulesRetry(t *testing.T) {
	store := &fakeStore{}
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	w := New(store, map[string]Handler{
		"boom": func(context.Context, queue.Job) error { return errors.New("kaboom") },
	}, Config{BackoffBase: time.Second, BackoffMax: time.Minute}, discardLogger())
	w.now = func() time.Time { return now }

	w.process(context.Background(), 0, claimedJob("job-2", "boom", 1))

	if len(store.failCalls) != 1 {
		t.Fatalf("Fail calls = %d, want 1", len(store.failCalls))
	}
	call := store.failCalls[0]
	if call.ID != "job-2" || call.Cause != "kaboom" {
		t.Errorf("Fail = %+v, want id=job-2 cause=kaboom", call)
	}
	if !call.Fence.Equal(testFence) {
		t.Errorf("Fail fence = %v, want %v", call.Fence, testFence)
	}
	if want := now.Add(time.Second); !call.RetryAt.Equal(want) {
		t.Errorf("RetryAt = %v, want %v", call.RetryAt, want)
	}
	if len(store.completeCalls) != 0 {
		t.Errorf("Complete calls = %d, want 0", len(store.completeCalls))
	}
}

func TestProcessUnknownTypeFails(t *testing.T) {
	store := &fakeStore{}
	w := New(store, map[string]Handler{}, Config{}, discardLogger())

	w.process(context.Background(), 0, claimedJob("job-3", "mystery", 1))

	if len(store.failCalls) != 1 {
		t.Fatalf("Fail calls = %d, want 1", len(store.failCalls))
	}
	if store.failCalls[0].Cause == "" {
		t.Error("expected a cause describing the missing handler")
	}
}

func TestProcessLeaseLostOnComplete(t *testing.T) {
	store := &fakeStore{completeErr: queue.ErrLeaseLost}
	w := New(store, map[string]Handler{
		"echo": func(context.Context, queue.Job) error { return nil },
	}, Config{}, discardLogger())

	// Must not panic and must not fall back to a Fail call.
	w.process(context.Background(), 0, claimedJob("job-4", "echo", 1))

	if len(store.completeCalls) != 1 {
		t.Fatalf("Complete calls = %d, want 1", len(store.completeCalls))
	}
	if len(store.failCalls) != 0 {
		t.Errorf("Fail calls = %d, want 0", len(store.failCalls))
	}
}

func TestProcessJobWithoutLeaseIsIgnored(t *testing.T) {
	store := &fakeStore{}
	job := claimedJob("job-5", "echo", 1)
	job.LockedUntil = nil
	w := New(store, map[string]Handler{
		"echo": func(context.Context, queue.Job) error { return nil },
	}, Config{}, discardLogger())

	w.process(context.Background(), 0, job)

	if len(store.completeCalls) != 0 || len(store.failCalls) != 0 {
		t.Errorf("store should not be called without a lease: complete=%d fail=%d",
			len(store.completeCalls), len(store.failCalls))
	}
}

func TestBackoff(t *testing.T) {
	w := New(&fakeStore{}, nil, Config{
		BackoffBase: 100 * time.Millisecond,
		BackoffMax:  400 * time.Millisecond,
	}, discardLogger())

	tests := []struct {
		attempts int
		want     time.Duration
	}{
		{1, 100 * time.Millisecond},
		{2, 200 * time.Millisecond},
		{3, 400 * time.Millisecond},
		{4, 400 * time.Millisecond}, // capped
		{10, 400 * time.Millisecond},
	}
	for _, tc := range tests {
		if got := w.backoff(tc.attempts); got != tc.want {
			t.Errorf("backoff(%d) = %v, want %v", tc.attempts, got, tc.want)
		}
	}
}

func TestConfigDefaults(t *testing.T) {
	got := Config{}.withDefaults()

	if len(got.Queues) != 1 || got.Queues[0] != queue.DefaultQueue {
		t.Errorf("Queues = %v, want [%s]", got.Queues, queue.DefaultQueue)
	}
	if got.Concurrency != 1 {
		t.Errorf("Concurrency = %d, want 1", got.Concurrency)
	}
	if got.PollInterval <= 0 || got.Lease <= 0 || got.BackoffBase <= 0 || got.BackoffMax <= 0 {
		t.Errorf("durations not defaulted: %+v", got)
	}
	if got.BackoffMax < got.BackoffBase {
		t.Errorf("BackoffMax %v < BackoffBase %v", got.BackoffMax, got.BackoffBase)
	}
}

func TestRunProcessesThenStopsOnCancel(t *testing.T) {
	store := &fakeStore{claimQueue: []queue.Job{claimedJob("job-6", "echo", 1)}}
	done := make(chan struct{})
	w := New(store, map[string]Handler{
		"echo": func(context.Context, queue.Job) error { return nil },
	}, Config{Concurrency: 1, PollInterval: 5 * time.Millisecond}, discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Cancel once the job has been completed so Run can observe it and stop.
	go func() {
		for {
			store.mu.Lock()
			n := len(store.completeCalls)
			store.mu.Unlock()
			if n > 0 {
				close(done)
				cancel()
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()

	runDone := make(chan error, 1)
	go func() { runDone <- w.Run(ctx) }()

	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancel")
	}

	<-done
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.completeCalls) != 1 {
		t.Errorf("Complete calls = %d, want 1", len(store.completeCalls))
	}
}
