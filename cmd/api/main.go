// Command api serves the distributed-task-queue HTTP API.
//
// It connects to PostgreSQL, optionally applies migrations, and exposes
// POST /v1/jobs and GET /v1/jobs/{id} until it receives SIGINT/SIGTERM, then
// shuts down gracefully.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fiwon123/distributed-task-queue/internal/httpapi"
	"github.com/fiwon123/distributed-task-queue/internal/queue"
	"github.com/fiwon123/distributed-task-queue/migrations"
)

func main() {
	if err := run(); err != nil {
		slog.Error("api exited with error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	databaseURL := flag.String("database-url", os.Getenv("DATABASE_URL"),
		"PostgreSQL connection string (defaults to $DATABASE_URL)")
	applyMigrations := flag.Bool("migrate", true, "apply database migrations on startup")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	if *databaseURL == "" {
		return errors.New("no database URL: set DATABASE_URL or pass -database-url")
	}

	// Cancel the context on SIGINT/SIGTERM so the server drains and exits.
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

	srv := &http.Server{
		Addr:              *addr,
		Handler:           httpapi.New(queue.New(pool), logger).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("http api listening", "addr", *addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	select {
	case err := <-serveErr:
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
		logger.Info("shutting down")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	return nil
}
