package worker

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fiwon123/distributed-task-queue/backend/internal/queue"
)

var testFence = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fakeStore is an in-memory Store for worker tests. No database required.
type fakeStore struct {
	mu sync.Mutex

	claimQueue []queue.Job
	claimErr   error
	claimCalls int

	completeErr error
	failErr     error

	extendCalls  []extendCall
	extendErr    error
	extendNew    time.Time     // new fence returned by ExtendLease; zero = fence+lease
	extendSeq    []time.Time   // fences returned in order; the last value repeats
	extendSignal chan struct{} // buffered; receives a token on each ExtendLease call

	completeCalls []completeCall
	failCalls     []queue.FailParams
}

type extendCall struct {
	id    string
	fence time.Time
	lease time.Duration
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

func (f *fakeStore) ExtendLease(_ context.Context, id string, fence time.Time, lease time.Duration) (time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.extendCalls = append(f.extendCalls, extendCall{id: id, fence: fence, lease: lease})
	if f.extendSignal != nil {
		select {
		case f.extendSignal <- struct{}{}:
		default:
		}
	}
	if f.extendErr != nil {
		return time.Time{}, f.extendErr
	}
	if len(f.extendSeq) > 0 {
		next := f.extendSeq[0]
		if len(f.extendSeq) > 1 {
			f.extendSeq = f.extendSeq[1:]
		}
		return next, nil
	}
	if f.extendNew.IsZero() {
		return fence.Add(lease), nil
	}
	return f.extendNew, nil
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

// TestProcessTimeoutFailsAttempt verifies a job configured with timeout_seconds
// runs under a deadline: a handler that outlives it is cancelled and the attempt
// is failed with a message naming the timeout (rather than completing).
func TestProcessTimeoutFailsAttempt(t *testing.T) {
	store := &fakeStore{}
	w := New(store, map[string]Handler{
		"sleep": func(ctx context.Context, _ queue.Job) error {
			<-ctx.Done()
			return ctx.Err()
		},
	}, Config{Lease: 30 * time.Second}, discardLogger())

	job := claimedJob("job-timeout", "sleep", 1)
	timeout := 1
	job.TimeoutSeconds = &timeout

	start := time.Now()
	w.process(context.Background(), 0, job)
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("process took %v; the per-job timeout did not fire", elapsed)
	}

	if len(store.completeCalls) != 0 {
		t.Fatalf("Complete calls = %d, want 0", len(store.completeCalls))
	}
	if len(store.failCalls) != 1 {
		t.Fatalf("Fail calls = %d, want 1", len(store.failCalls))
	}
	if cause := store.failCalls[0].Cause; !strings.Contains(cause, "execution timed out after 1 seconds") {
		t.Errorf("Fail cause = %q, want it to name the timeout", cause)
	}
}

// TestProcessTimeoutFailsAttemptEvenIfHandlerIgnoresCtx covers a handler that
// never observes ctx and returns nil after overrunning its deadline: the worker
// must still fail the attempt rather than completing it.
func TestProcessTimeoutFailsAttemptEvenIfHandlerIgnoresCtx(t *testing.T) {
	store := &fakeStore{}
	w := New(store, map[string]Handler{
		"stubborn": func(context.Context, queue.Job) error {
			time.Sleep(2000 * time.Millisecond)
			return nil
		},
	}, Config{Lease: 30 * time.Second}, discardLogger())

	job := claimedJob("job-stubborn", "stubborn", 1)
	timeout := 1
	job.TimeoutSeconds = &timeout

	w.process(context.Background(), 0, job)

	if len(store.completeCalls) != 0 {
		t.Fatalf("Complete calls = %d, want 0 (a timed-out job must not complete)", len(store.completeCalls))
	}
	if len(store.failCalls) != 1 {
		t.Fatalf("Fail calls = %d, want 1", len(store.failCalls))
	}
	if cause := store.failCalls[0].Cause; !strings.Contains(cause, "execution timed out after 1 seconds") {
		t.Errorf("Fail cause = %q, want it to name the timeout", cause)
	}
}

// TestProcessWithoutTimeoutIsNotDeadlineBound verifies that a job without
// timeout_seconds is not cancelled by the worker: a short wait completes
// normally, so the feature is opt-in per job.
func TestProcessWithoutTimeoutIsNotDeadlineBound(t *testing.T) {
	store := &fakeStore{}
	w := New(store, map[string]Handler{
		"work": func(ctx context.Context, _ queue.Job) error {
			select {
			case <-time.After(10 * time.Millisecond):
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}, Config{}, discardLogger())

	w.process(context.Background(), 0, claimedJob("job-notimeout", "work", 1))

	if len(store.completeCalls) != 1 || len(store.failCalls) != 0 {
		t.Errorf("complete=%d fail=%d, want the job to complete",
			len(store.completeCalls), len(store.failCalls))
	}
}

// TestProcessRenewsLeaseWhileHandlerRuns verifies the heartbeat: the handler is
// allowed to run longer than the original lease, and the terminal Complete uses
// the renewed fence.
func TestProcessRenewsLeaseWhileHandlerRuns(t *testing.T) {
	renewed := testFence.Add(time.Minute)
	store := &fakeStore{
		extendNew:    renewed,
		extendSignal: make(chan struct{}, 1),
	}
	w := New(store, map[string]Handler{
		"slow": func(_ context.Context, _ queue.Job) error {
			// Wait until the heartbeat has renewed the lease once.
			select {
			case <-store.extendSignal:
			case <-time.After(2 * time.Second):
			}
			return nil
		},
	}, Config{Lease: 30 * time.Millisecond}, discardLogger())

	w.process(context.Background(), 0, claimedJob("job-renew", "slow", 1))

	store.mu.Lock()
	defer store.mu.Unlock()

	if len(store.extendCalls) < 1 {
		t.Fatalf("ExtendLease calls = %d, want >= 1", len(store.extendCalls))
	}
	if !store.extendCalls[0].fence.Equal(testFence) {
		t.Errorf("ExtendLease fence = %v, want original %v", store.extendCalls[0].fence, testFence)
	}
	if len(store.completeCalls) != 1 {
		t.Fatalf("Complete calls = %d, want 1", len(store.completeCalls))
	}
	if !store.completeCalls[0].fence.Equal(renewed) {
		t.Errorf("Complete fence = %v, want renewed %v", store.completeCalls[0].fence, renewed)
	}
	if len(store.failCalls) != 0 {
		t.Errorf("Fail calls = %d, want 0", len(store.failCalls))
	}
}

// TestProcessRenewsLeaseMultipleTimes verifies the renewable fence advances
// across two consecutive renewals and that Complete uses the latest one.
func TestProcessRenewsLeaseMultipleTimes(t *testing.T) {
	first := testFence.Add(30 * time.Second)
	second := testFence.Add(60 * time.Second)
	store := &fakeStore{
		extendSeq:    []time.Time{first, second},
		extendSignal: make(chan struct{}, 2),
	}
	w := New(store, map[string]Handler{
		"slow": func(_ context.Context, _ queue.Job) error {
			for i := 0; i < 2; i++ {
				select {
				case <-store.extendSignal:
				case <-time.After(2 * time.Second):
					t.Error("heartbeat did not renew the lease")
					return nil
				}
			}
			return nil
		},
	}, Config{Lease: 30 * time.Millisecond}, discardLogger())

	w.process(context.Background(), 0, claimedJob("job-renew-2", "slow", 1))

	store.mu.Lock()
	defer store.mu.Unlock()

	if len(store.extendCalls) < 2 {
		t.Fatalf("ExtendLease calls = %d, want >= 2", len(store.extendCalls))
	}
	// The second renewal must be fenced on the first renewal's fence.
	if !store.extendCalls[1].fence.Equal(first) {
		t.Errorf("second ExtendLease fence = %v, want first-renewed %v", store.extendCalls[1].fence, first)
	}
	if len(store.completeCalls) != 1 {
		t.Fatalf("Complete calls = %d, want 1", len(store.completeCalls))
	}
	if !store.completeCalls[0].fence.Equal(second) {
		t.Errorf("Complete fence = %v, want second-renewed %v", store.completeCalls[0].fence, second)
	}
}

// TestProcessLeaseLostAbortsHandler verifies that when the lease is lost while
// the handler runs, the handler's context is cancelled and no terminal update
// is written (the job now belongs to another worker).
func TestProcessLeaseLostAbortsHandler(t *testing.T) {
	store := &fakeStore{
		extendErr:    queue.ErrLeaseLost,
		extendSignal: make(chan struct{}, 1),
	}
	w := New(store, map[string]Handler{
		"slow": func(ctx context.Context, _ queue.Job) error {
			<-ctx.Done()
			return ctx.Err()
		},
	}, Config{Lease: 30 * time.Millisecond}, discardLogger())

	w.process(context.Background(), 0, claimedJob("job-lost", "slow", 1))

	store.mu.Lock()
	defer store.mu.Unlock()

	if len(store.extendCalls) == 0 {
		t.Error("ExtendLease was not called")
	}
	if len(store.completeCalls) != 0 || len(store.failCalls) != 0 {
		t.Errorf("terminal calls = complete:%d fail:%d, want 0 when the lease is lost",
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

// TestRunDrainsInFlightJob verifies that cancelling Run stops new claims but
// lets an already-running job finish before Run returns.
func TestRunDrainsInFlightJob(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})

	store := &fakeStore{claimQueue: []queue.Job{claimedJob("job-drain", "slow", 1)}}
	w := New(store, map[string]Handler{
		"slow": func(context.Context, queue.Job) error {
			close(started)
			<-release
			return nil
		},
	}, Config{Concurrency: 1, PollInterval: time.Millisecond, ShutdownTimeout: 5 * time.Second}, discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() { runDone <- w.Run(ctx) }()

	<-started // the handler is now running with a claimed job
	cancel()  // begin shutdown while the job is in flight

	// Run must not return while the job is still running: it should be draining.
	select {
	case err := <-runDone:
		t.Fatalf("Run returned during drain (err=%v); in-flight job was abandoned", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(release) // let the handler finish

	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after the in-flight job finished")
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.completeCalls) != 1 {
		t.Errorf("Complete calls = %d, want 1 (drained job should be completed)", len(store.completeCalls))
	}
}

// TestRunShutdownTimeoutAbandonsHangingJob verifies that a job which outruns the
// drain window does not block shutdown forever.
func TestRunShutdownTimeoutAbandonsHangingJob(t *testing.T) {
	started := make(chan struct{})

	store := &fakeStore{claimQueue: []queue.Job{claimedJob("job-hang", "hang", 1)}}
	w := New(store, map[string]Handler{
		"hang": func(ctx context.Context, _ queue.Job) error {
			close(started)
			<-ctx.Done() // only returns once the drain window closes
			return ctx.Err()
		},
	}, Config{Concurrency: 1, PollInterval: time.Millisecond, ShutdownTimeout: 50 * time.Millisecond}, discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- w.Run(ctx) }()

	<-started
	cancel()

	// ShutdownTimeout is 50ms; allow generous slack but require it to return.
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return; a hanging job blocked shutdown past ShutdownTimeout")
	}
}

func TestConfigShutdownTimeoutDefault(t *testing.T) {
	got := Config{}.withDefaults()
	if got.ShutdownTimeout <= 0 {
		t.Errorf("ShutdownTimeout = %v, want a positive default", got.ShutdownTimeout)
	}
}
