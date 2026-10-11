## Description

<Describe your changes in detail>

## Type of Change

- [ ] Bug fix (non-breaking change which fixes an issue)
- [ ] New feature (non-breaking change which adds functionality)
- [ ] Breaking change (fix or feature that would cause existing functionality to not work as expected)
- [ ] Documentation update
- [ ] Refactoring (no functional changes)
- [ ] CI/CD changes
- [ ] Other (please describe)

## Related Issues

Closes #<issue-number>

## Changes Made

- <change 1>
- <change 2>

## Testing

- [ ] `go test ./...` passes
- [ ] `gofmt -l .` prints nothing
- [ ] `go vet ./...` passes
- [ ] `go build ./...` succeeds
- [ ] Postgres integration tests run (if the change touches the job store): `go test -tags integration ./...`
- [ ] Manual testing performed (describe below, if applicable)

> `make check` runs the four gates above together. CI runs only on `dev` → `main`
> PRs, so this local run is the gate for a feature branch. Skipped is not passed.

## Correctness (queue changes)

- [ ] Handlers remain idempotent — delivery is at-least-once, so a handler may run more than once
- [ ] Concurrent claims stay safe (`FOR UPDATE SKIP LOCKED`), if this touches claiming

## Checklist

- [ ] My code follows the project's style guidelines
- [ ] I have performed a self-review of my own code
- [ ] I have commented my code, particularly in hard-to-understand areas
- [ ] I have made corresponding changes to the documentation
- [ ] New and existing tests pass locally with my changes
