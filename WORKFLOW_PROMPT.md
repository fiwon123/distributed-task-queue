# Development Workflow Prompt

You are the primary AI agent for `distributed-task-queue`. Drive development
work using this workflow. Every change is tracked end-to-end on GitHub:
**issue → branch → PR → merge**, with the issue and PR assigned, labeled,
linked, and commented at each milestone.

## 1. Plan and understand

- Understand the requirement; ask clarifying questions if ambiguous.
- Explore the codebase (read relevant files, follow existing patterns).
- **Create or understand the GitHub issue**: ensure it exists, is assigned
  (`--assignee fiwon123`), labeled per commit type, has a milestone
  (current active milestone, e.g. `v0.1-MVP`), and post a "Starting work on
  this — Milestone: <milestone-name>" comment.
- Output: Goal, Files to modify, Dependencies, Risks, Steps. Get user approval
  before proceeding to implementation.

## 2. Branch

- Always start from an up-to-date `dev`:

```bash
git checkout dev
git pull origin dev
git checkout -b <type>/<issue-number>-<slug>
```

- Branch types: `feat/`, `fix/`, `refactor/`, `docs/`, `test/`, `chore/`, `ci/`
- Examples: `feat/4-enqueue-endpoint`, `fix/9-lease-reclaim`, `refactor/12-claim-transaction`

## 3. Implement

- Make small, focused commits with conventional commits: `feat:`, `fix:`,
  `refactor:`, `docs:`, `test:`, `chore:`, `ci:`
- Go specifics: run `gofmt` on changed files, keep the standard library's
  `testing` package for tests, and keep handlers idempotent — delivery is
  at-least-once, so a handler may run more than once.
- Do NOT create issues/PRs from subagents — that responsibility stays with the
  primary agent.

## 4. Test

- Run the local gate: `make check` (gofmt + vet + test + build).
  **Until `go.mod` exists, the Go steps are skipped — a skipped run is not a
  passed run**, so scaffolding changes are verified by review, not by `make`.
- Unit tests must not require Postgres. Integration tests that need the database
  are gated behind a build tag (e.g. `//go:build integration`) and run against
  the compose `db` service.
- CI does not run on feature branches — local testing is your gate.
- If using the dev sandbox: `make dev-up`, `make shell`, `make opencode` as
  appropriate.

## 5. Push & PR

```bash
git push origin <branch>
gh pr create --base dev --title "<type>: <description>" \
  --body "Closes #<issue-number>" --label <label> --assignee fiwon123 \
  --milestone "<milestone-name>"
```

- Label per commit type: `feat` → `enhancement`, `fix` → `bug`, `chore` → `chore`,
  `refactor` → `refactor`, `docs` → `documentation`, `test` → `test`, `ci` → `ci`.
- Fill out the PR template completely; link the issue with `Closes #<issue-number>`.
- Comment on the issue: `PR opened: #<pr-number>`.

## 6. Review & Merge

- Self-review the diff for bugs, security, and regressions.
- Merge only when explicitly instructed; prefer squash merge, then delete the branch:

```bash
gh pr merge <pr-number> --squash --delete-branch
```

- After merging to `dev`:
  - Comment on the issue: `Merged into dev — ready for release`
  - Comment on the PR: `Merged into dev — thanks!`
  - **Close the issue explicitly**: `gh issue close <issue-number>`.
    `Closes #N` only auto-closes on the repository's default branch (`main`);
    a feature PR merges into `dev`, so without this step issues stay open forever.
- Always return to `dev` after the merge:
  `git checkout dev && git pull origin dev`

## 7. Release (dev → main) — user-initiated

- Never create a release PR or merge to `main` on your own. The user must
  explicitly ask ("Can we merge dev to main?").
- Create the release PR `dev` → `main` with label `release`. CI runs on this PR.
- This PR closes no issues — all milestone issues were closed at step 6.
- Never pass `--delete-branch` to a release merge: the head branch is `dev`
  itself, so `--delete-branch` deletes the integration branch.
- After CI passes, ask the user to confirm the merge; then merge `main` back
  into `dev` and verify `dev` survived:
  `git ls-remote --heads origin dev` (must print a line).

## 8. Cleanup

- Return to `dev`, pull, prune:

```bash
git checkout dev
git pull origin dev
git remote prune origin
```

- Always end on `dev` — the working branch with the latest integrated features.

## Important

- **Skipped runs are not passed.** A run that never executes (never triggered,
  or gated out) is not coverage. Do not claim a change is "CI passed" unless a
  run actually executed the job that covers it.
- The dev sandbox container has **no docker socket** (by design). The agent
  inside it cannot run `docker`/`docker compose`; it reaches Postgres over the
  compose network at hostname `db`.
- GitHub credentials come from the host environment: `GH_TOKEN` or
  `GITHUB_TOKEN` are forwarded into the sandbox at runtime — they are never
  baked into the image.
