# distributed-task-queue — local development gate.
#
# CI runs only on `dev` -> `main` PRs, so `make check` is the gate that
# feature branches rely on. Until `go.mod` exists, the Go toolchain steps are
# SKIPPED (a skipped run is not a passed run), keeping scaffolding PRs usable.

GO ?= go
NODE ?= node
NPM ?= npm

.PHONY: help check fmt vet test build run run-worker clean \
        web-install web-deps web-build web-test web-lint web-check \
        preflight dev-up dev-down dev-restart dev-log dev-build reset \
        shell opencode opencode-auto dev-shell dev-opencode dev-opencode-auto

# Extra flags for the opencode invocation. `make opencode-auto` /
# `make dev-opencode-auto` set this to --auto, which auto-approves permissions
# that are not explicitly denied; the secret-file deny rules still apply.
OPENCODE_ARGS ?=

help: ## List available targets
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

## check: gofmt + vet + test + build (the local gate before pushing)
check:
	@if [ ! -f go.mod ]; then \
	  echo "==> no go.mod yet — Go toolchain checks SKIPPED (skipped is not passed)"; \
	else \
	  $(MAKE) fmt vet test build; \
	fi
	@$(MAKE) web-check

# Format only the Go files git knows about (tracked + untracked, minus
# gitignored). This keeps dependency caches that live under the repo — in the
# dev sandbox GOPATH/GOMODCACHE is /workspace/.go — out of the check, which
# `gofmt -l .` would otherwise walk.
fmt:
	@echo "==> gofmt"
	@out="$$(git ls-files -co --exclude-standard -z -- '*.go' | xargs -0 -r gofmt -l)"; \
	if [ -n "$$out" ]; then echo "unformatted files:"; echo "$$out"; exit 1; fi

vet:
	@echo "==> go vet"
	$(GO) vet ./...

test:
	@echo "==> go test"
	$(GO) test ./...

build:
	@echo "==> go build"
	$(GO) build ./...

run:
	$(GO) run ./cmd/api

run-worker:
	$(GO) run ./cmd/worker

clean:
	@echo "cleaning"
	rm -f ./api ./worker

# --- Web dashboard -------------------------------------------------------------
# The built bundle in web/dist is committed so `go build ./...` and CI work with
# only Go installed. Rebuild it with `make web-build` after changing web/src.

web-deps:
	@if [ ! -d web/node_modules ]; then \
	  echo "==> installing web dependencies"; \
	  cd web && $(NPM) ci; \
	fi

web-install: ## Install dashboard dependencies (npm ci)
	cd web && $(NPM) ci

web-build: web-deps ## Build the dashboard into web/dist (embedded by go:embed)
	cd web && $(NPM) run build

web-test: web-deps ## Run dashboard unit tests (vitest)
	cd web && $(NPM) run test

web-lint: web-deps ## Lint (oxlint) + format-check (oxfmt) the dashboard
	cd web && $(NPM) run lint && $(NPM) run format:check

## web-check: lint + test the dashboard; skips loudly when Node is absent
web-check:
	@if ! command -v $(NODE) >/dev/null 2>&1; then \
	  echo "==> web checks SKIPPED (node not found — skipped is not passed)"; \
	else \
	  $(MAKE) web-lint web-test; \
	fi

# --- Dev sandbox (docker compose) ----------------------------------------------

preflight: ## Check host prerequisites for the dev sandbox
	@test -x "$(HOME)/.opencode/bin/opencode" || { echo "ERROR: opencode not found at ~/.opencode/bin/opencode" >&2; exit 1; }
	@mkdir -p "$(HOME)/.config/opencode" "$(HOME)/.config/gh" && touch "$(HOME)/.gitconfig"
	@scripts/resolve-gh-token.sh --check >/dev/null 2>&1 || true

dev-up: preflight ## Build and start the dev sandbox + Postgres
	@HOST_UID=$$(id -u) HOST_GID=$$(id -g) GH_TOKEN=$$(scripts/resolve-gh-token.sh 2>/dev/null) GITHUB_TOKEN=$$(scripts/resolve-gh-token.sh 2>/dev/null) docker compose up -d --build

dev-restart: preflight ## Restart the dev container after Dockerfile/deps changes
	@docker compose restart dev

dev-down: ## Stop and remove the dev sandbox
	@docker compose down

dev-log: ## Follow dev container logs
	@docker compose logs -f dev

dev-build: preflight ## Rebuild the dev image without starting
	@docker compose build

reset: ## Stop the dev sandbox and remove the Postgres volume
	@docker compose down -v

shell: dev-shell ## Alias for dev-shell
opencode: dev-opencode ## Alias for dev-opencode (run opencode inside the sandbox)
opencode-auto: dev-opencode-auto ## Alias for dev-opencode-auto (run opencode with --auto)

dev-shell: preflight ## Open a shell in the dev container
	@docker compose exec dev bash || docker compose run --rm dev bash

dev-opencode: preflight ## Run opencode inside the dev sandbox
	@if [ -z "$$(docker compose ps -q dev 2>/dev/null)" ]; then echo "[opencode] starting dev stack..."; HOST_UID=$$(id -u) HOST_GID=$$(id -g) GH_TOKEN=$$(scripts/resolve-gh-token.sh 2>/dev/null) GITHUB_TOKEN=$$(scripts/resolve-gh-token.sh 2>/dev/null) docker compose up -d dev; fi
	@if [ -t 0 ]; then docker compose exec -it dev zsh -lc "cd /workspace && opencode $(OPENCODE_ARGS)" || docker compose exec -it dev bash -lc "opencode $(OPENCODE_ARGS)"; else docker compose exec -T dev zsh -lc "cd /workspace && opencode $(OPENCODE_ARGS)" || docker compose exec -T dev bash -lc "opencode $(OPENCODE_ARGS)"; fi

# Same recipe, with --auto: auto-approve permissions that are not explicitly
# denied (every secret-file deny rule still applies).
dev-opencode-auto: OPENCODE_ARGS = --auto
dev-opencode-auto: dev-opencode ## Run opencode inside the dev sandbox with --auto
