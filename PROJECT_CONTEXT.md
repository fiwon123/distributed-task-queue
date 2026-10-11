# Project Context

## Identity

- **Project Name**: distributed-task-queue
- **One-liner**: A durable, PostgreSQL-backed distributed task queue built from scratch in Go — enqueue jobs, run workers, and survive crashes without losing work.

## Problem

A single-process task runner loses in-flight work when the process crashes and
cannot spread load across machines. A durable queue needs the job state to
outlive the workers: a job claimed by a worker that then died must become
runnable again, retries must be bounded so a poison job cannot loop forever, and
failed jobs must land somewhere inspectable instead of vanishing.

## Goal

Users can run an HTTP API and one or more worker processes against a shared
PostgreSQL database and: enqueue jobs, poll their status, run concurrent workers,
watch failed jobs retry with exponential backoff, recover jobs abandoned by
workers killed mid-job, and requeue dead-lettered jobs. The MVP is the
database-backed version only — Redis or a separate broker is deliberately out of
scope until the database-backed queue works.

## Stack

- **Language**: Go (standard toolchain)
- **Storage**: PostgreSQL — the single source of truth for job state
- **DB access**: pgx (v5) — `FOR UPDATE SKIP LOCKED` for atomic job claims
- **HTTP API**: `net/http` (`POST /v1/jobs`, `GET /v1/jobs/{id}`,
  `POST /v1/jobs/{id}/requeue`, `GET /v1/queues/{name}/stats`)
- **Workers**: an in-process goroutine pool that polls the jobs table
- **Containerization**: Docker Compose (dev sandbox + a local PostgreSQL service)
- **Testing**: `go test` (standard library `testing`; Postgres integration tests
  gated behind a build tag so unit tests run with no services)

## Architecture

```text
Client (curl / HTTP client)
  ↓
HTTP API (POST /v1/jobs, GET /v1/jobs/{id}, POST /v1/jobs/{id}/requeue, GET /v1/queues/{name}/stats)
  ↓
PostgreSQL (jobs table — the queue itself: due jobs, leases, attempts, status)
  ↕
Worker pool (claim due jobs with FOR UPDATE SKIP LOCKED, run handlers, update status)
```

## Solution

The jobs table *is* the queue. Producers enqueue a row; workers claim due rows
inside a short transaction using `FOR UPDATE SKIP LOCKED`, which lets many
workers pull work concurrently without ever claiming the same job. A claimed job
is hidden behind a lease (`locked_until`); if the worker dies before finishing,
the lease expires and the job becomes claimable again. Failed attempts are
rescheduled with exponential backoff until `max_attempts` is reached, at which
point the job moves to a dead-letter state where it can be inspected and
requeued manually.

### MVP Features

- Enqueue a job: `type` + JSON payload, optionally scheduled via `run_at`
- Poll a job: status, attempts, `last_error`, timestamps
- Multiple concurrent workers claiming jobs safely (`FOR UPDATE SKIP LOCKED`)
- Retries with exponential backoff, bounded by `max_attempts`
- Leases: a claimed job is hidden until `locked_until`; a crashed worker's job is
  reclaimed once the lease expires
- Dead-letter state for jobs that exhaust attempts, with a requeue endpoint
- Graceful shutdown: stop claiming, let in-flight jobs finish, exit cleanly
- Per-queue stats

## API

| Method | Path | Purpose |
|---|---|---|
| POST | /v1/jobs | Enqueue a job (`type`, `payload`, optional `run_at`/`max_attempts`) |
| GET | /v1/jobs/{id} | Job status (state, attempts, last_error, timestamps) |
| POST | /v1/jobs/{id}/requeue | Requeue a dead-lettered or failed job |
| GET | /v1/queues/{name}/stats | Counts by state for a queue |

## Data Model

The `jobs` table:

- `id` — job identifier
- `type` — handler name the worker dispatches on
- `payload` — JSON (`jsonb`) the handler receives
- `queue` — queue name (drives `/v1/queues/{name}/stats`); defaults to `default`
- `status` — one of `pending`, `running`, `completed`, `failed`, `dead`
- `attempts` — how many times the job has been attempted
- `max_attempts` — retry ceiling before the job is dead-lettered
- `run_at` — when the job next becomes claimable (enables backoff)
- `locked_until` — the lease expiry for the worker currently holding the job
- `last_error` — the most recent failure message
- `created_at`, `completed_at` — timestamps

## Design Choices

- **Postgres is the queue**: no Redis, no separate broker in the MVP.
  `FOR UPDATE SKIP LOCKED` gives safe concurrent claims without a lock service.
- **At-least-once, not exactly-once**: a worker can die after a job's side effect
  but before its status update, so delivery is at-least-once. Handlers must be
  idempotent. There is no exactly-once guarantee.
- **Leases, not heartbeats**: a claimed job is hidden until `locked_until`; a
  crashed worker's lease expires and the job becomes claimable again. This is how
  abandoned work is recovered.
- **Bounded retries with backoff**: each failed attempt schedules the next
  `run_at` further out; a job that exhausts `max_attempts` moves to `dead` rather
  than retrying forever.
- **No strict global ordering**: jobs are independent. If ordering matters, use
  one worker per queue or serialize on a key inside the handler.
- **Claims happen in one transaction**: select due jobs, mark them running, and
  set the lease atomically, so two workers cannot both win the same row.

## Build Order

1. **Schema + enqueue/status**: the `jobs` table migration, `POST /v1/jobs` and
   `GET /v1/jobs/{id}` — end-to-end tested against Postgres
2. **One worker**: poll for due jobs, run a handler, mark completed/failed
   (a single worker, no concurrency yet)
3. **Concurrent claiming**: `FOR UPDATE SKIP LOCKED` so N workers never claim the
   same job; test with two workers
4. **Retries**: attempts + exponential backoff via `run_at`; move to `dead` at
   `max_attempts`
5. **Leases**: `locked_until` hides running jobs; reclaim jobs from a worker
   killed mid-job
6. **Dead-letter**: dead state + `POST /v1/jobs/{id}/requeue`
7. **Graceful shutdown**: stop claiming, drain in-flight jobs, exit cleanly

## Topology

- **Components**: `.` (single component, flat Go module)
- **Services (dev)**: `dev` (sandbox) + `db` (PostgreSQL)

## Key Commands

```bash
go mod tidy
go build ./...
go vet ./...
go test ./...
go run ./cmd/api       # run the HTTP API
go run ./cmd/worker    # run a worker
make check             # local gate (Go steps skip until go.mod exists)
make dev-up            # start the dev sandbox + Postgres (docker compose)
make dev-shell         # shell inside the sandbox (alias: make shell)
make dev-opencode      # run opencode inside the sandbox (alias: make opencode)
make dev-down          # stop the sandbox
```

## Notes

- Postgres is the source of truth. The queue is a table, not an in-memory list.
- Delivery is at-least-once: handlers must be idempotent. There is no
  exactly-once guarantee.
- No Redis / external broker in the MVP — add one only after the database-backed
  version works.
- This repo is prepared as a setup-only skeleton + workflow; future sessions
  implement the queue step by step.
