// Command api serves the distributed-task-queue HTTP API and dashboard.
//
// It connects to PostgreSQL, optionally applies migrations, serves the /v1
// endpoints plus the embedded web dashboard at /, and runs until it receives
// SIGINT/SIGTERM, then shuts down gracefully.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fiwon123/distributed-task-queue/docs"
	"github.com/fiwon123/distributed-task-queue/internal/httpapi"
	"github.com/fiwon123/distributed-task-queue/internal/queue"
	"github.com/fiwon123/distributed-task-queue/migrations"
	"github.com/fiwon123/distributed-task-queue/web"
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

	api := httpapi.New(queue.New(pool), logger)
	// Serve the embedded dashboard. fs.Sub only fails on an invalid path, which
	// would be a build-time embedding bug, so a failure is fatal rather than a
	// silent "no UI".
	dashboard, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		return fmt.Errorf("locate embedded dashboard: %w", err)
	}
	api = api.WithDashboard(dashboard).WithOpenAPI(docs.FS)

	srv := &http.Server{
		Addr:              *addr,
		Handler:           api.Handler(),
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
