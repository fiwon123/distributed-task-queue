// Command worker runs a pool of workers against the task queue.
//
// It connects to PostgreSQL, optionally applies migrations, registers the demo
// handlers (echo, fail, sleep), and polls for jobs until SIGINT/SIGTERM.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fiwon123/distributed-task-queue/internal/queue"
	"github.com/fiwon123/distributed-task-queue/internal/worker"
	"github.com/fiwon123/distributed-task-queue/migrations"
)

func main() {
	if err := run(); err != nil {
		slog.Error("worker exited with error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	queues := flag.String("queues", queue.DefaultQueue, "comma-separated queues to poll")
	concurrency := flag.Int("concurrency", 4, "number of concurrent workers")
	pollInterval := flag.Duration("poll-interval", time.Second, "wait between empty polls")
	lease := flag.Duration("lease", 30*time.Second, "how long a claimed job is hidden")
	backoffBase := flag.Duration("backoff-base", time.Second, "delay before the first retry")
	backoffMax := flag.Duration("backoff-max", time.Minute, "cap on the retry backoff")
	shutdownTimeout := flag.Duration("shutdown-timeout", 30*time.Second, "how long in-flight jobs may finish during shutdown")
	databaseURL := flag.String("database-url", os.Getenv("DATABASE_URL"),
		"PostgreSQL connection string (defaults to $DATABASE_URL)")
	applyMigrations := flag.Bool("migrate", true, "apply database migrations on startup")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	if *databaseURL == "" {
		return errors.New("no database URL: set DATABASE_URL or pass -database-url")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, *databaseURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	if *applyMigrations {
		if err := queue.Migrate(ctx, pool, migrations.FS); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
		logger.Info("migrations applied")
	}

	w := worker.New(queue.New(pool), handlers(logger), worker.Config{
		Queues:          splitQueues(*queues),
		Concurrency:     *concurrency,
		PollInterval:    *pollInterval,
		Lease:           *lease,
		BackoffBase:     *backoffBase,
		BackoffMax:      *backoffMax,
		ShutdownTimeout: *shutdownTimeout,
	}, logger)

	logger.Info("worker pool started",
		"queues", splitQueues(*queues), "concurrency", *concurrency, "lease", *lease)

	if err := w.Run(ctx); err != nil {
		return fmt.Errorf("run worker: %w", err)
	}
	logger.Info("worker pool stopped")
	return nil
}

// splitQueues parses a comma-separated queue list, trimming blanks.
func splitQueues(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
