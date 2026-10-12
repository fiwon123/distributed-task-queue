## Testing Requirements

### General

- Tests are mandatory for new features and bug fixes
- Write tests before or alongside implementation
- Mock external services when possible — never call a real database or external
  service in a unit test
- Aim for critical-path coverage, not 100% line coverage

### Prerequisites / Services

- Unit tests must run with no external services — the job store's claim and
  scheduling logic and the worker's dispatch/backoff are tested in-process
  against a fake store.
- Integration tests that need PostgreSQL (claim behaviour under `FOR UPDATE SKIP
  LOCKED`, migrations, lease reclaim) are opt-in: gate them behind the
  `integration` build tag (`//go:build integration`) and run them against the
  compose `db` service. Document the command in `AGENTS.md`.
- Prefer running unit tests in isolation; keep database tests clearly separated.

### Go

- **Framework**: the standard library `testing` package (`go test`)
- **Location**: `*_test.go` files next to the code they test
  (e.g. `backend/internal/queue/store_test.go`)
- **Run**: `go test ./...` (unit); `go test -tags integration ./...` (with Postgres)

#### Test Structure

```
backend/internal/queue/
├── store.go
└── store_test.go        # tests for the package it sits in

backend/internal/worker/
├── worker.go
└── worker_test.go
```

#### Conventions

- Test functions: `func TestXxx(t *testing.T)`
- Prefer table-driven tests: a slice of cases with `name`, inputs, and expected output
- Use `t.Run(name, ...)` for subtests and `t.Helper()` in helpers
- Tests live in the same package (`package queue`) unless you need to test only
  the exported API
- Because delivery is at-least-once, assert that a reclaimed job is handled
  idempotently rather than assuming exactly-once
- Use a fake store for unit tests; only integration tests touch a real database

### Frontend (React + TypeScript)

- **Framework**: Vitest + React Testing Library (jsdom)
- **Location**: `*_test.tsx` / `*_test.ts` next to the component under `frontend/src/`
- **Run**: `make frontend-test` (or `cd frontend && npm run test`)
- **Lint/format**: `make frontend-lint` (oxlint + `oxfmt --check`)
- Stub the network — components call the `/v1` API; install a `fetch` stub rather
  than hitting a real server (see the existing `installFetch` helper in
  `frontend/src/App.test.tsx`)

### Linting

```bash
# Go
gofmt -l .          # list unformatted files (empty = clean)
go vet ./...        # static analysis
```

### Before Committing

1. Run unit tests: `go test ./...`
2. Run `gofmt -l .` (empty output = clean)
3. Run `go vet ./...`
4. Run `go build ./...`
5. Verify no regressions

> `make check` runs the four gates above together. It is the local gate: CI runs
> only on `dev` → `main` PRs, never on a feature branch.

### What counts as done

CI runs only on `dev` → `main` PRs, so a feature branch has no CI signal — run
`make check` locally and say what you ran. A run that never executes (never
triggered, or gated out) is `skipped`, not `passed`. **Skipped is not passed.**
If you cannot run a test — for example the main agent has no Postgres for an
integration test — say the state is unverified rather than claiming it passed;
delegate to the `tester` subagent or run it in the dev sandbox instead.
