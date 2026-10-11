---
name: Bug Report
about: Report a bug in the task queue, the worker, or the HTTP API
title: "fix: "
labels: ["bug"]
assignees: ["fiwon123"]
---

## Description

<What went wrong? Describe the bug clearly.>

## To Reproduce

```bash
# exact commands (e.g. go test ./... , go run ./cmd/api ... , or a curl against /v1/jobs)
```

## Expected behaviour

<What you expected to happen.>

## Actual behaviour

<What actually happened — include the exact error output where possible.>

## Environment

- OS:
- Go version (`go version`):
- Postgres version (`SELECT version();`):
- How it ran (host `go run`, dev sandbox, workers count):

## Notes

<Logs, stack traces, suspected cause, or anything else relevant. Did a job get
double-run, stuck, or dropped? Note whether it involved a crash/lease expiry.>
