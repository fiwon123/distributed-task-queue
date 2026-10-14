.PHONY: build vet test fmt frontend-install frontend-build frontend-test frontend-lint frontend-build-prod check openapi-json dev-up dev-restart dev-shell dev-opencode dev-log dev-down prod-up prod-down reset screenshots

GO := go
NPM := npm
BIN := $(shell pwd)/bin
PATH := $(BIN):$(PATH)

check:
	gofmt -l .
	$(GO) vet ./...
	$(GO) test ./...
	$(GO) build ./...
	@echo "--- frontend gates ---"
	@cd frontend && npx oxlint --quiet || true
	@cd frontend && npx oxfmt --check src || true
	@cd frontend && npx vitest --run --reporter=dot || true

build:
	$(GO) build ./...

vet:
	$(GO) vet ./...

test:
	$(GO) test ./...

fmt:
	gofmt -s -w .

frontend-install:
	cd frontend && $(NPM) ci

frontend-build:
	cd frontend && $(NPM) run build

frontend-test:
	cd frontend && $(NPM) run test -- --run

frontend-lint:
	cd frontend && $(NPM) run lint

frontend-build-prod:
	cd frontend && $(NPM) run build

openapi-json:
	$(GO) run ./docs/generate

screenshots:
	cd frontend && node tools/screenshot.cjs

include Makefile.compose
