# Contributing

Contributions are welcome. Please follow these conventions:

- Branch from `dev` (not `main`): `<type>/<issue-number>-<slug>`
- Types: `feat/`, `fix/`, `refactor/`, `docs/`, `test/`, `chore/`, `ci/`
- Use conventional commits: `feat:`, `fix:`, `refactor:`, `docs:`, `test:`, `chore:`, `ci:`
- Run `make check` locally before pushing (Go + frontend gates)
- Open PR against `dev`; reference the issue with `Closes #<number>`
- One feature per PR; tests for features belong in the same PR
- CI runs only on `dev` → `main` PRs (release PRs); feature branches need local checks
- Never push directly to `dev` or `main`

See `AGENTS.md` and `workflow.md` for full workflow details.
