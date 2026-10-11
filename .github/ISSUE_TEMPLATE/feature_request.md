---
name: Feature Request
about: Propose a new capability for the task queue, the worker, or the HTTP API
title: "feat: "
labels: ["enhancement"]
assignees: ["fiwon123"]
---

## Description

<What should the system do that it does not do today?>

## Acceptance criteria

- [ ] <criterion 1>
- [ ] <criterion 2>

## Design notes

<Which queue concern does this touch (enqueue, claim, retries/backoff, leases,
dead-letter, API, graceful shutdown)? Any trade-offs — delivery is at-least-once,
handlers must be idempotent, and there is no strict global ordering. Say why the
simplest approach is acceptable.>

## Alternatives considered

<Other approaches considered, if any.>
