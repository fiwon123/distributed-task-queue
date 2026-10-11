package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/fiwon123/distributed-task-queue/internal/queue"
	"github.com/fiwon123/distributed-task-queue/internal/worker"
)

// handlers returns the demo handlers the worker dispatches on. Real deployments
// would register their own; these exist so the queue can be exercised end to end.
//
//	{"type":"echo","payload":{...}}              logs and succeeds
//	{"type":"fail","payload":{...}}              always fails (retries then dead-letters)
//	{"type":"sleep","payload":{"seconds":2}}     sleeps, then succeeds
func handlers(logger *slog.Logger) map[string]worker.Handler {
	return map[string]worker.Handler{
		"echo": func(_ context.Context, job queue.Job) error {
			logger.Info("echo", "job", job.ID, "payload", string(job.Payload))
			return nil
		},
		"fail": func(_ context.Context, job queue.Job) error {
			return errors.New("demo handler always fails")
		},
		"sleep": sleepHandler(logger),
	}
}

func sleepHandler(logger *slog.Logger) worker.Handler {
	return func(ctx context.Context, job queue.Job) error {
		var payload struct {
			Seconds float64 `json:"seconds"`
		}
		_ = json.Unmarshal(job.Payload, &payload) // a bad payload just uses the default
		if payload.Seconds <= 0 {
			payload.Seconds = 1
		}
		d := time.Duration(payload.Seconds * float64(time.Second))

		logger.Info("sleeping", "job", job.ID, "duration", d)
		timer := time.NewTimer(d)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return nil
		}
	}
}
