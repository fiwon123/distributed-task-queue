# distributed-task-queue

A durable, PostgreSQL-backed distributed task queue built from scratch in Go.
Producers enqueue jobs over HTTP; one or more workers pull them from the
database, run them, and survive crashes without losing work.

> **Educational project — not production-ready.** There is no TLS, no
> authentication, and no operational hardening. The goal is to understand how a
> durable queue recovers abandoned work and bounds retries, not to run this in
> front of real traffic.

## Problem

A single-process task runner loses in-flight work when the process crashes and
cannot spread load across machines. A durable queue keeps the job state in a
shared database so that work outlives the workers running it: a job claimed by a
worker that then died becomes runnable again, retries are bounded, and failed
jobs land somewhere inspectable instead of vanishing.

## What you can do

Once built, you can run the API and one or more workers and:

- enqueue jobs (a `type` plus a JSON payload), optionally scheduled for later;
- poll a job's status, attempts, and last error;
- run several workers concurrently against one database without double-claiming;
- watch failed jobs retry with exponential backoff;
- recover jobs abandoned by a worker killed mid-job, via leases;
- inspect dead-lettered jobs and requeue them;
- read per-queue statistics.

## Status

**Phase 3 landed — requeue, stats, and graceful shutdown.** The `jobs` table,
`POST /v1/jobs` / `GET /v1/jobs/{id}`, a worker pool that claims jobs with
`FOR UPDATE SKIP LOCKED`, exponential-backoff retries, lease-based recovery of
jobs abandoned by a dead worker, dead-letter requeue, per-queue stats, and
graceful shutdown are all implemented and tested. Only the dashboard remains in
the build order below.

## Stack

- **Language**: Go (standard toolchain)
- **Storage**: PostgreSQL — the queue is a table, not an in-memory list
- **DB access**: pgx (v5); `FOR UPDATE SKIP LOCKED` for atomic claims
- **HTTP API**: `net/http`
- **Workers**: in-process goroutine pool polling the jobs table
- **Local environment**: Docker Compose dev sandbox + a Postgres service
- **Testing**: standard `testing` package (`go test`); Postgres integration tests
  gated behind a build tag

## API

All four endpoints are implemented.

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/v1/jobs` | Enqueue a job (`type`, `payload`, optional `run_at`/`max_attempts`) |
| `GET` | `/v1/jobs/{id}` | Job status, attempts, last error, timestamps |
| `POST` | `/v1/jobs/{id}/requeue` | Requeue a dead-lettered or failed job |
| `GET` | `/v1/queues/{name}/stats` | Counts by state for a queue |

## Design choices

- **Postgres is the queue** — no Redis, no separate broker in the MVP; the jobs
  table plus `FOR UPDATE SKIP LOCKED` gives safe concurrent claims.
- **At-least-once, not exactly-once** — a worker can die after a job's side
  effect but before its status update, so handlers must be idempotent.
- **Leases for abandoned work** — a claimed job is hidden until `locked_until`;
  when a worker dies, the lease expires and the job becomes claimable again.
- **Bounded retries with backoff** — each failure pushes `run_at` further out;
  a job that exhausts `max_attempts` moves to `dead`.
- **No strict global ordering** — jobs are independent; serialize inside a
  handler or use one worker per queue if order matters.
- **Graceful shutdown** — stop claiming, drain in-flight jobs, exit cleanly.

## Build order

- [x] 1. Schema + enqueue/status: `jobs` table + `POST /v1/jobs`, `GET /v1/jobs/{id}`
- [x] 2. One worker: poll, run, mark completed/failed
- [x] 3. Concurrent claiming: `FOR UPDATE SKIP LOCKED`; test with two workers
- [x] 4. Retries: attempts + exponential backoff; dead at `max_attempts`
- [x] 5. Leases: `locked_until`; reclaim a job from a worker killed mid-job
- [x] 6. Dead-letter: dead state + requeue endpoint
- [x] 7. Graceful shutdown: stop claiming and drain

## Done when

You can start the API and two workers, enqueue a batch of jobs, kill one worker
mid-job, and observe that every job still completes exactly as many times as its
handlers allow — the killed worker's job is reclaimed after its lease expires,
and a job that always fails lands in the dead-letter state after `max_attempts`.

## Development

The docker compose **dev sandbox** gives you the full toolchain (Go, `gh`, `air`
fast reload) with your host GitHub config mounted read-only and
`GH_TOKEN`/`GITHUB_TOKEN` forwarded at runtime — never baked into the image. A
`db` service provides PostgreSQL; the sandbox reaches it at hostname `db`.

```bash
make dev-up        # build + start the sandbox and Postgres (needs docker on the host)
make dev-restart   # restart after Dockerfile/deps changes
make shell         # shell inside the sandbox
make opencode      # run opencode inside the sandbox
make dev-log       # follow container logs
make dev-down      # stop and remove the sandbox
make reset         # stop and remove the sandbox, including the Postgres volume
```

Direct toolchain (host or inside the sandbox):

```bash
go build ./...       # build
go test ./...        # test
go vet ./...         # static analysis
gofmt -l .           # formatting (empty output = clean)
go run ./cmd/api     # run the HTTP API
go run ./cmd/worker  # run a worker
make check           # gofmt + vet + test + build (the local gate)
```

The project follows an issue-first, branch-based workflow — see
[`WORKFLOW_PROMPT.md`](WORKFLOW_PROMPT.md) and [`AGENTS.md`](AGENTS.md).
