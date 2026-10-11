# AGENTS.md

> **Source**: Product sections below are populated from `PROJECT_CONTEXT.md`.

### Product

## Project Objective

distributed-task-queue — a durable, PostgreSQL-backed distributed task queue
built from scratch in Go. Enqueue jobs, run workers, and survive crashes without
losing work.

### Problem

A single-process task runner loses in-flight work when the process crashes and
cannot spread load across machines. A durable queue needs the job state to
outlive the workers: a job claimed by a worker that then died must become
runnable again, retries must be bounded, and failed jobs must land somewhere
inspectable instead of vanishing.

### Goal

Users can run an HTTP API and one or more worker processes against a shared
PostgreSQL database and: enqueue jobs, poll their status, run concurrent workers,
watch failed jobs retry with exponential backoff, recover jobs abandoned by
workers killed mid-job, and requeue dead-lettered jobs. The MVP is the
database-backed version only — Redis or a separate broker is out of scope until
the database-backed queue works.

### Main Technology

- Language: Go (standard toolchain)
- Storage: PostgreSQL — the single source of truth for job state
- DB access: pgx (v5); `FOR UPDATE SKIP LOCKED` for atomic claims
- HTTP API: `net/http` (`POST /v1/jobs`, `GET /v1/jobs/{id}`,
  `POST /v1/jobs/{id}/requeue`, `GET /v1/queues/{name}/stats`)
- Workers: in-process goroutine pool polling the jobs table
- Orchestration: Docker Compose (dev sandbox + local PostgreSQL service)
- Testing: `go test` (standard library `testing`)

### Core Architecture

```text
Client (curl / HTTP client)
  ↓
HTTP API (POST /v1/jobs, GET /v1/jobs/{id}, POST /v1/jobs/{id}/requeue, GET /v1/queues/{name}/stats)
  ↓
PostgreSQL (jobs table — the queue itself: due jobs, leases, attempts, status)
  ↕
Worker pool (claim due jobs with FOR UPDATE SKIP LOCKED, run handlers, update status)
```

## Project Structure

**Not implemented yet.** The tree currently contains only workflow and
configuration files. The intended layout, to be built in order (see Build Order
in `PROJECT_CONTEXT.md`), is:

```
distributed-task-queue/
├── cmd/api/             # HTTP API entry point (flags, wiring, startup)
├── cmd/worker/          # worker entry point (poll loop, pool, graceful shutdown)
├── internal/httpapi/    # HTTP handlers for /v1/jobs and /v1/queues/{name}/stats
├── internal/queue/      # the job store: schema, claim (FOR UPDATE SKIP LOCKED), status
├── internal/worker/     # worker pool: claim, dispatch, retry/backoff, leases
├── migrations/          # SQL migrations for the jobs table
├── scripts/             # dev-sandbox helper scripts (entrypoint, token resolution, …)
├── Dockerfile           # dev sandbox image (Go + gh + air fast reload)
├── docker-compose.yml   # dev sandbox + local Postgres (`dev` + `db` services)
├── mise.toml            # toolchain pins (go, gh) — host + dev image
├── .air.toml            # air fast-reload config (runs ./cmd/api)
├── Makefile
├── opencode.json
└── .github/workflows/ci.yml
```

## Development Commands

```bash
go build ./...           # build all packages
go vet ./...             # static analysis
go test ./...            # run tests
gofmt -l .               # list unformatted files (empty = clean)
go run ./cmd/api         # run the HTTP API (once implemented)
go run ./cmd/worker      # run a worker (once implemented)
```

### Run All Checks

```bash
make check
```

`make check` runs `gofmt -l`, `go vet`, `go test`, and `go build` — the same gate
CI enforces. Prefer it over listing commands separately: agents follow whatever
`check` runs, so a check missing from it is a check nobody performs. Add new
checks to the `Makefile`.

> **Until `go.mod` exists, `make check` skips the Go toolchain steps** with an
> explicit "SKIPPED (skipped is not passed)" message — a skipped run verifies
> nothing, so scaffolding PRs are judged by review, not by `make`.

### Dev sandbox

The docker compose `dev` service is the development environment (gh + opencode
pre-configured, `air` fast reload, no docker socket); a `db` service runs
PostgreSQL for local development. Host-only tools are bootstrapped with
`./scripts/setup-host-tools.sh` (installs mise, authenticates gh). Key targets:

```bash
make dev-up        # build + start the sandbox and Postgres (requires docker on the host)
make dev-restart   # restart the dev container after Dockerfile/deps changes
make dev-shell     # shell inside the container (alias: make shell)
make dev-opencode  # run opencode inside the container (alias: make opencode)
make dev-log       # follow container logs
make dev-down      # stop and remove the sandbox
```

`make dev-up` forwards `GH_TOKEN` or `GITHUB_TOKEN` (whichever is valid) into
the container at runtime — values stay in the host environment, never in the
image. Host `~/.config/gh`, `~/.config/opencode`, `~/.gitconfig`, and the
`opencode` binary are mounted read-only. Go module/build caches live under
`/workspace/.go`; opencode/gh runtime state under `/workspace/.xdg-*` (both
gitignored).

The `dev` service reaches Postgres at the compose hostname `db`:

```text
postgres://taskqueue@db:5432/taskqueue?sslmode=disable
```

That URL is also exported into the sandbox as `DATABASE_URL`. From the host (not
the container), the same server is published on `${POSTGRES_PORT:-5432}` and
answers as `postgres://taskqueue@localhost:5432/taskqueue?sslmode=disable`. The
local `db` service uses `POSTGRES_HOST_AUTH_METHOD=trust` and an empty password
on purpose — it is a throwaway dev database bound to localhost, never a deployed
one.

## Runtime Environment

The AI agent (opencode) runs at the repository root. For development, a docker
compose **dev sandbox** provides the full toolchain (Go, `gh`, `air` fast
reload) with host GitHub config mounted read-only — see Development Commands.
The in-container agent has **no docker socket** (by design): it cannot manage
containers, so Postgres is reached over the compose network, not by running
docker.

Install Go, GitHub CLI, and OpenCode as needed for your environment; on a fresh
host, `./scripts/setup-host-tools.sh` bootstraps mise + gh.

### What the agent CAN do

- Run the Go toolchain (`go build`, `go test`, `go vet`, `gofmt`, `go run`)
- Run git and GitHub CLI commands
- Start local API and worker processes
- Start/use the dev sandbox from the host: `make dev-up`, `make dev-shell`, `make dev-opencode`
- Connect to the dev Postgres from the host on `${POSTGRES_PORT:-5432}`
- Run every script under `scripts/` (entrypoint, token resolution, sandbox bring-up)

### What the agent CANNOT do

- Run `docker` or `docker compose` commands in this environment (no docker on the host here)
- Access the Docker socket — including inside the dev sandbox, which mounts none by design
- Modify the host filesystem beyond the workspace

## Environment Variables

- `DATABASE_URL` — the Postgres connection string (`postgres://taskqueue@db:5432/taskqueue?sslmode=disable`
  inside the sandbox). Not a secret in this project: the local `db` service uses
  trust auth and is never deployed.
- `GH_TOKEN` / `GITHUB_TOKEN` — GitHub credentials, read from the host and
  forwarded into the dev sandbox at runtime. Either name works; neither is
  defined here, and values are never baked into the image.
- `POSTGRES_PORT` — host port for the dev Postgres (defaults to `5432`; set it if
  the host already runs a database on `5432`).
- `HOST_HOME` — optional override for the host home directory used by the
  read-only config mounts (defaults to `$HOME`).
- This project reads API/worker configuration from command-line flags plus
  `DATABASE_URL`. If a `.env` or `.env.*` file is introduced later, it is
  gitignored and the secret-file rules below apply.

## Key Conventions

- **Assignee**: `fiwon123` — `workflow.md` tells agents to read `ASSIGNEE` from here before assigning issues and PRs
- **GitHub credentials**: `GH_TOKEN` or `GITHUB_TOKEN` — either name works. Do not set either here: values belong in the host environment, and the dev sandbox forwards whichever is valid at runtime.
- **Dev sandbox**: docker compose `dev` + `db` services (gh + opencode pre-configured, no docker socket); `make dev-up` / `dev-restart` / `dev-shell` / `dev-opencode` / `dev-down`
- **Model assignments**: an example, not a recommendation — `opencode.json` assigns `opencode/big-pickle` to `build` and `opencode/mimo-v2.6-flash-free` to `plan`. Change them to whatever your install offers; no prompt depends on which model an agent runs. Verify an id with `opencode models` first.
- Linting / formatting: `gofmt` for all Go code; `go vet` for static analysis
- Testing: the standard `testing` package (`go test ./...`); Postgres integration tests are gated behind a build tag so unit tests need no services
- CI: GitHub Actions runs formatting + vet + test + build on `dev` → `main` PRs only — never on feature branch PRs or push to `dev`
- Local testing: run `make check` before pushing feature branches
- Delivery semantics: at-least-once — handlers must be idempotent; there is no exactly-once guarantee
- Storage scope: Postgres is the source of truth; no Redis / external broker in the MVP

## Environment and Secret-File Rules

- Never open, read, print, summarize, quote, or send the contents of `.env`, `.env.*`, `.envrc`, or any other environment/secret file unless the user explicitly gives permission in the current conversation. `.env.example` is the one exception: it holds placeholders and is safe to read.
- Never run commands that reveal environment values — `printenv`, `env`, and the Docker equivalents.
- Never include secret values in the final answer. Refer to them only by variable name.
- If the user explicitly permits reading environment configuration, read only explicitly approved non-secret variables and check existence rather than printing values.

### Enforcement (backstop, not the permission)

`opencode.json` denies reading, editing, and bash access to `.env*` paths, and
denies commands that dump the environment. It matches **command strings and file
paths**, not intent, so it has known gaps: an allowed interpreter can still read a
file, and a repo-wide `grep` ignores `.gitignore`. The `.gitignore` entries for
`.env` and `.envrc` are what protect the Grep tool — keep them.

## Development Workflow

### Git Workflow

- Branch from `dev` (not `main`)
- Never push directly to `dev` or `main`
- Branch naming: `<type>/<issue-number>-<slug>` (e.g., `feat/4-enqueue-endpoint`)
- Branch types: `feat/`, `fix/`, `refactor/`, `docs/`, `test/`, `chore/`, `ci/`
- Use conventional commits: `feat:`, `fix:`, `refactor:`, `docs:`, `test:`, `chore:`, `ci:`
- All testing is local for feature branches — CI only runs on `dev` → `main` PRs
- Do NOT merge pull requests unless explicitly instructed
- Do NOT automatically create release PRs or merge to main — user must explicitly request
- Always return to `dev` branch after completing any merge
- Every change is tracked on GitHub: **issue → branch → PR → merge**

### Linking

- Every branch includes the issue number: `feat/4-enqueue-endpoint`
- Every PR references its issue: `Closes #<number>`
- Every issue is assigned to a milestone before work begins
- Every PR is assigned to a milestone when created
- No orphaned branches, PRs, or issues

### Branch Strategy

```text
main          ← release merges (CI runs here — dev→main PRs only)
  └── dev     ← integration branch (all feature branches merge here)
        ├── feat/4-enqueue-endpoint
        ├── fix/9-lease-reclaim
        └── refactor/12-claim-transaction
```

- Feature branches: `<type>/<issue>-<slug>` → merge to `dev`
- Release: `dev` → `main` (PR triggers CI — user must explicitly request)
- Hotfixes: `fix/<issue>-<slug>` → merge to `dev`, cherry-pick to `main` if urgent

### Milestones

- Every issue MUST be assigned to a milestone before work begins
- Milestones represent releases or sprint iterations
- Use `gh issue edit <number> --milestone "<milestone-name>"`
- Track milestone progress on the GitHub Milestones page

### Agents

Agent definitions live in `opencode.json` and `.opencode/agents/`:

- **build** (primary): Full development work with all tools enabled
- **plan** (primary): Analysis and planning without making changes — restricted to read-only subagents (explore, reviewer only)
- **reviewer** (subagent): Reviews code for bugs, security, and regressions
- **backend** (subagent): Implements the Go API, the job store, and the worker — edit scoped to `cmd/**` and `internal/**`
- **tester** (subagent): Writes and runs Go tests — edit scoped to `*_test.go` and `testdata/`

This project is single-stack (Go, no frontend), so the `frontend` and `visual`
agents from the engineering-workflow templates are deliberately not defined here —
a subagent whose edit scope points at a directory that does not exist is an
expensive way to say nothing.

Each agent's instructions live in exactly one place: the body of
`.opencode/agents/<name>.md`. Model, mode, and permissions live in
`opencode.json`. Do not add a `prompt` key to `opencode.json` — it creates a
second prompt source that drifts from the markdown.

Subagent permissions must match what the markdown tells the agent to do. If
`plan.md` lists an agent but `task` denies it, the agent is instructed to use a
tool it cannot reach.

### Skills

- Load relevant skills when working on specific domains
- `github-workflow` (`SKILL.md`) is the git/issue/PR command cookbook; `workflow.md` remains the policy authority.

### Session Memory (3-tier)

- At the START of every chat: read `.opencode/memory/global.md` then `.opencode/memory/current.md` and resume from "Next action".
- Update `global.md` at meaningful milestones (issue created, PR opened, merged, decision made); update `current.md` after each step and at session end; rotate the oldest archive so only 3 remain.
- Keep sizes bounded: `global.md` ≤ ~200 lines, `current.md` ≤ ~100 lines, archive files ≤ ~50 lines each; never store secrets/tokens/credentials.
- The whole `.opencode/memory/` directory is gitignored and must never be committed. Full rules: `.opencode/instructions/memory.md`.

### Rules for Agents

- branch strategy and naming conventions;
- CI strategy (which PRs trigger CI, local testing requirements);
- milestone and issue conventions;
- development commands (test, format, vet, build);
- environment and secret-file rules;
- project-specific constraints (at-least-once delivery, Postgres-as-queue).

### CI triggers (read this before claiming a change is verified)

CI runs on release PRs (`dev` → `main`) and manual dispatch — never on a
feature-branch push. Two things can make a run verify nothing:

- a `paths:` filter that the change does not touch means the workflow does not
  trigger at all; and
- a job gated behind a label or `if:` condition comes back `skipped`, not `passed`.

**Skipped is not passed.** A triggered run whose jobs were all gated out, or a
run that never happened, is not coverage. Do not describe a change as "CI passed"
unless a run actually executed the job that covers it; if it did not, say the
change is unverified and test it locally.
