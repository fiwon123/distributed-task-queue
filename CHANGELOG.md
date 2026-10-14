# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Expand About page into "Concepts & how it works" with theory, architecture mapping, core concepts (durability, at-least-once, leases, FOR UPDATE SKIP LOCKED, retries/backoff, dead-letter, priority), job lifecycle diagram, key characteristics table, and tests
- Screenshot tooling (Playwright) with mocked API fixtures and `make screenshots` target; docs/screenshots scaffold
- Workers API scaffold and routes (GET /v1/workers) groundwork
- Job delete endpoint and terminal purge endpoint (backend)
- Jobs list enhancements: keyset pagination, type filter, live refresh, load more
- Health/readiness endpoints (/healthz, /readyz)
- Project renamed to "Distributed Task Queue" (display title)

### Changed
- About page content and styling expanded for educational clarity
- README framing for presentation (work in progress)

## [0.6.0] - Previous release context
- Console improvements and embedded SPA behavior
- OpenAPI schema enforcement and docs vendoring
- Worker lease renewal/heartbeat and graceful shutdown

