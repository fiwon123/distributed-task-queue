// Package worker runs jobs claimed from the queue.
//
// A Worker is a small goroutine pool: each goroutine polls the store for a due
// job, dispatches it to a Handler by job type, and records success or failure.
// Claiming is safe across processes (the store uses FOR UPDATE SKIP LOCKED), so
// several Worker processes can run against one database without double-claiming.
//
// Delivery is at-least-once: a job whose worker dies is reclaimed after its
// lease expires and runs again, so Handlers must be idempotent.
package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/fiwon123/distributed-task-queue/backend/internal/queue"
)

// Handler runs a single job. Returning an error marks the attempt failed and
// schedules a retry; once the job exhausts max_attempts it is dead-lettered.
// Handlers should honour ctx cancellation.
type Handler func(ctx context.Context, job queue.Job) error

// Store is the subset of the job store the worker needs. The production
// implementation is *queue.Store.
type Store interface {
	Claim(ctx context.Context, p queue.ClaimParams) ([]queue.Job, error)
	ExtendLease(ctx context.Context, id string, fence time.Time, lease time.Duration) (time.Time, error)
	Complete(ctx context.Context, id string, fence time.Time) (queue.Job, error)
	Fail(ctx context.Context, p queue.FailParams) (queue.Job, error)
}

// Config tunes a Worker. Zero fields fall back to sensible defaults, so a zero
// Config is usable.
type Config struct {
	// Queues to poll. Empty defaults to the "default" queue.
	Queues []string
	// Concurrency is the number of polling goroutines.
	Concurrency int
	// PollInterval is how long to wait after an empty claim (or a claim error).
	PollInterval time.Duration
	// Lease is how long a claimed job is hidden before it can be reclaimed.
	Lease time.Duration
	// BackoffBase is the delay before the first retry.
	BackoffBase time.Duration
	// BackoffMax caps the exponential backoff delay.
	BackoffMax time.Duration
	// ShutdownTimeout bounds how long in-flight jobs may keep running after Run
	// is cancelled. After it elapses the in-flight context is cancelled and any
	// unfinished job is left to its lease.
	ShutdownTimeout time.Duration
}

func (c Config) withDefaults() Config {
	if len(c.Queues) == 0 {
		c.Queues = []string{queue.DefaultQueue}
	}
	if c.Concurrency < 1 {
		c.Concurrency = 1
	}
	if c.PollInterval <= 0 {
		c.PollInterval = time.Second
	}
	if c.Lease <= 0 {
		c.Lease = 30 * time.Second
	}
	if c.BackoffBase <= 0 {
		c.BackoffBase = time.Second
	}
	if c.BackoffMax <= 0 {
		c.BackoffMax = time.Minute
	}
	if c.BackoffMax < c.BackoffBase {
		c.BackoffMax = c.BackoffBase
	}
	if c.ShutdownTimeout <= 0 {
		c.ShutdownTimeout = 30 * time.Second
	}
	return c
}

// Worker polls the queue and runs jobs with a pool of goroutines.
type Worker struct {
	store    Store
	handlers map[string]Handler
	cfg      Config
	log      *slog.Logger
	now      func() time.Time // injectable for tests
}

// New returns a Worker that dispatches to handlers by job type. A nil logger
// falls back to slog.Default.
func New(store Store, handlers map[string]Handler, cfg Config, logger *slog.Logger) *Worker {
	if logger == nil {
		logger = slog.Default()
	}
	copied := make(map[string]Handler, len(handlers))
	for typ, h := range handlers {
		copied[typ] = h
	}
	return &Worker{
		store:    store,
		handlers: copied,
		cfg:      cfg.withDefaults(),
		log:      logger,
		now:      time.Now,
	}
}

// Run starts the polling goroutines and blocks until ctx is cancelled, then
// stops claiming and lets in-flight jobs finish. In-flight work gets up to
// ShutdownTimeout; after that its context is cancelled and any unfinished job is
// abandoned to its lease (and reclaimed later).
func (w *Worker) Run(ctx context.Context) error {
	// workCtx governs in-flight jobs. It is independent of ctx so a job keeps
	// running during the drain window, and is cancelled when that window closes.
	workCtx, cancelWork := context.WithCancel(context.Background())
	defer cancelWork()

	go func() {
		<-ctx.Done()
		w.log.Info("shutdown requested; draining in-flight jobs", "timeout", w.cfg.ShutdownTimeout)
		timer := time.NewTimer(w.cfg.ShutdownTimeout)
		defer timer.Stop()
		select {
		case <-timer.C:
			w.log.Warn("shutdown timeout reached; cancelling in-flight jobs")
		case <-workCtx.Done():
		}
		cancelWork()
	}()

	var wg sync.WaitGroup
	for i := 0; i < w.cfg.Concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			w.loop(ctx, workCtx, id)
		}(i)
	}
	wg.Wait()
	return nil
}

// loop polls for jobs using runCtx (so claiming stops on shutdown) and processes
// each claimed job using workCtx (so in-flight work can drain).
func (w *Worker) loop(runCtx, workCtx context.Context, id int) {
	w.log.Debug("worker started", "worker", id)
	defer w.log.Debug("worker stopped", "worker", id)

	for {
		if runCtx.Err() != nil {
			return
		}

		jobs, err := w.store.Claim(runCtx, queue.ClaimParams{
			Queues: w.cfg.Queues,
			Lease:  w.cfg.Lease,
			Limit:  1,
		})
		if err != nil {
			if runCtx.Err() != nil {
				return
			}
			w.log.Error("claim failed", "worker", id, "error", err)
			if !sleepCtx(runCtx, w.cfg.PollInterval) {
				return
			}
			continue
		}
		if len(jobs) == 0 {
			if !sleepCtx(runCtx, w.cfg.PollInterval) {
				return
			}
			continue
		}

		for _, job := range jobs {
			w.process(workCtx, id, job)
		}
	}
}

// process runs one claimed job and records its outcome.
//
// While the handler runs, a heartbeat renews the job's lease so a handler that
// outlives its original lease is not reclaimed by another worker and run twice
// even though nothing crashed. If the lease is lost anyway (it lapsed and
// someone else took the job), the handler's context is cancelled and the row is
// left to its new owner.
//
// A job configured with a per-job timeout (timeout_seconds) runs under a
// deadline: when it expires the handler's context is cancelled and the attempt
// fails with a clear timeout error, which retries or dead-letters per the usual
// rules.
func (w *Worker) process(ctx context.Context, workerID int, job queue.Job) {
	if job.LockedUntil == nil {
		w.log.Error("claimed job is missing its lease", "worker", workerID, "job", job.ID)
		return
	}
	handler, ok := w.handlers[job.Type]
	if !ok {
		w.fail(ctx, workerID, job, *job.LockedUntil,
			fmt.Errorf("no handler registered for job type %q", job.Type))
		return
	}

	jobCtx := ctx
	var cancelJob context.CancelFunc
	if job.TimeoutSeconds != nil && *job.TimeoutSeconds > 0 {
		jobCtx, cancelJob = context.WithTimeout(ctx, time.Duration(*job.TimeoutSeconds)*time.Second)
	} else {
		jobCtx, cancelJob = context.WithCancel(ctx)
	}
	defer cancelJob()
	renewer := newLeaseRenewer(w.store, job.ID, w.cfg.Lease, *job.LockedUntil, w.log, cancelJob)
	stop := w.startHeartbeat(jobCtx, renewer)

	runErr := handler(jobCtx, job)
	stop() // stop renewing before the terminal update so the fence is stable

	fence, leaseLost := renewer.state()
	if leaseLost {
		w.log.Warn("lease lost while running job; leaving it to the new owner",
			"worker", workerID, "job", job.ID)
		return
	}

	// A fired deadline is a failure even when the handler returned nil: a
	// handler that ignores ctx and finishes anyway still overran its budget, and
	// completing the job would hide that. No timeout means jobCtx can never be
	// DeadlineExceeded.
	if jobCtx.Err() == context.DeadlineExceeded {
		runErr = fmt.Errorf("execution timed out after %d seconds", *job.TimeoutSeconds)
	}
	if runErr != nil {
		w.fail(ctx, workerID, job, fence, runErr)
		return
	}

	if _, err := w.store.Complete(ctx, job.ID, fence); err != nil {
		w.logLeaseOrError("complete", workerID, job.ID, err)
		return
	}
	w.log.Info("job completed",
		"worker", workerID, "job", job.ID, "type", job.Type, "attempts", job.Attempts)
}

// startHeartbeat renews the lease on a running job every Lease/3 until stop is
// called or the lease is lost. The returned stop function is idempotent and
// blocks until the heartbeat goroutine has exited, so once it returns the
// renewer's fence is stable and no renewal can race the terminal update.
func (w *Worker) startHeartbeat(ctx context.Context, r *leaseRenewer) func() {
	interval := w.cfg.Lease / 3
	if interval <= 0 {
		interval = w.cfg.Lease
	}

	done := make(chan struct{})
	stopped := make(chan struct{})
	var once sync.Once
	stop := func() {
		once.Do(func() {
			close(done)
			<-stopped
		})
	}

	go func() {
		defer close(stopped)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				// The handler's context was cancelled from outside (for example
				// the shutdown drain window closed), so we stop renewing and let
				// the lease lapse.
				return
			case <-ticker.C:
				if !r.renew(ctx) {
					return
				}
			}
		}
	}()
	return stop
}

// leaseRenewer tracks the fence of a claimed job while its lease is renewed by
// the heartbeat. It is safe for concurrent use: the heartbeat goroutine writes,
// and process reads the final state after stopping the heartbeat.
type leaseRenewer struct {
	store  Store
	id     string
	lease  time.Duration
	log    *slog.Logger
	cancel context.CancelFunc

	mu    sync.Mutex
	fence time.Time
	lost  bool
}

func newLeaseRenewer(store Store, id string, lease time.Duration, fence time.Time, log *slog.Logger, cancel context.CancelFunc) *leaseRenewer {
	return &leaseRenewer{
		store:  store,
		id:     id,
		lease:  lease,
		log:    log,
		cancel: cancel,
		fence:  fence,
	}
}

// renew extends the lease once. It returns true if the worker still holds the
// lease, and false when it was lost — after cancelling the handler so it stops
// touching a job another worker now owns.
func (r *leaseRenewer) renew(ctx context.Context) bool {
	r.mu.Lock()
	fence := r.fence
	r.mu.Unlock()

	newFence, err := r.store.ExtendLease(ctx, r.id, fence, r.lease)
	if err != nil {
		if errors.Is(err, queue.ErrLeaseLost) {
			r.mu.Lock()
			r.lost = true
			r.mu.Unlock()
			r.log.Warn("lease lost while running job", "job", r.id)
			r.cancel()
			return false
		}
		// A transient renewal failure is not fatal: the next tick retries with
		// the current fence well before the lease would expire. If the failed
		// call actually committed and only the response was lost, the next
		// renewal is rejected as stale; the lease is treated as lost and the job
		// is reclaimed and re-run — which at-least-once delivery already allows.
		r.log.Error("extend lease", "job", r.id, "error", err)
		return true
	}

	r.mu.Lock()
	r.fence = newFence
	r.mu.Unlock()
	return true
}

// state returns the current lease fence and whether the lease was lost.
func (r *leaseRenewer) state() (time.Time, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.fence, r.lost
}

// fail records a failed attempt, scheduling a retry or dead-lettering. fence is
// the lease value the caller last saw, renewed or original.
func (w *Worker) fail(ctx context.Context, workerID int, job queue.Job, fence time.Time, cause error) {
	retryAt := w.now().Add(w.backoff(job.Attempts))
	failed, err := w.store.Fail(ctx, queue.FailParams{
		ID:      job.ID,
		Cause:   cause.Error(),
		RetryAt: retryAt,
		Fence:   fence,
	})
	if err != nil {
		w.logLeaseOrError("fail", workerID, job.ID, err)
		return
	}

	if failed.Status == queue.StatusDead {
		w.log.Warn("job dead-lettered",
			"worker", workerID, "job", job.ID, "type", job.Type,
			"attempts", failed.Attempts, "error", cause)
		return
	}
	w.log.Warn("job attempt failed; will retry",
		"worker", workerID, "job", job.ID, "type", job.Type,
		"attempts", failed.Attempts, "retry_at", retryAt, "error", cause)
}

func (w *Worker) logLeaseOrError(op string, workerID int, jobID string, err error) {
	if errors.Is(err, queue.ErrLeaseLost) {
		w.log.Warn("lease lost before "+op, "worker", workerID, "job", jobID)
		return
	}
	w.log.Error(op+" failed", "worker", workerID, "job", jobID, "error", err)
}

// backoff returns the delay before the next attempt, doubling each time from
// BackoffBase and capped at BackoffMax. attempts is the number of attempts
// already made (>= 1).
func (w *Worker) backoff(attempts int) time.Duration {
	d := w.cfg.BackoffBase
	for i := 1; i < attempts; i++ {
		if d >= w.cfg.BackoffMax {
			return w.cfg.BackoffMax
		}
		d *= 2
	}
	if d > w.cfg.BackoffMax {
		return w.cfg.BackoffMax
	}
	return d
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
