# distributed-task-queue — local development gate.
#
# CI runs only on `dev` -> `main` PRs, so `make check` is the gate that
# feature branches rely on. Until `go.mod` exists, the Go toolchain steps are
# SKIPPED (a skipped run is not a passed run), keeping scaffolding PRs usable.

GO ?= go
NODE ?= node
NPM ?= npm

.PHONY: help check fmt vet test build run run-worker openapi-json clean \
        frontend-install frontend-deps frontend-build frontend-test frontend-lint frontend-check \
        frontend-src-guard \
        preflight require-agent dev-up dev-down dev-restart dev-log dev-build reset \
        shell opencode opencode-auto dev-shell dev-opencode dev-opencode-auto \
        prod-up prod-down prod-logs prod-ps prod-reset demo

# Extra flags for the opencode invocation. `make opencode-auto` /
# `make dev-opencode-auto` set this to --auto, which auto-approves permissions
# that are not explicitly denied; the secret-file deny rules still apply.
OPENCODE_ARGS ?=

help: ## List available targets
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

## check: gofmt + vet + test + build + frontend lint/test (the local gate before pushing)
check:
	@if [ ! -f go.mod ]; then \
	  echo "==> no go.mod yet — Go toolchain checks SKIPPED (skipped is not passed)"; \
	else \
	  $(MAKE) fmt vet test build; \
	fi
	@$(MAKE) frontend-check

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

## openapi-json: regenerate docs/openapi.json from docs/openapi.yaml (commit the result)
openapi-json:
	$(GO) run ./docs/generate

run:
	$(GO) run ./backend/cmd/api

run-worker:
	$(GO) run ./backend/cmd/worker

clean:
	@echo "cleaning"
	rm -f ./api ./worker

# --- Frontend dashboard -------------------------------------------------------------
# The built bundle in frontend/dist is committed so `go build ./...` and CI work with
# only Go installed. Rebuild it with `make frontend-build` after changing frontend/src.

frontend-deps:
	@if [ ! -d frontend/node_modules ]; then \
	  echo "==> installing frontend dependencies"; \
	  cd frontend && $(NPM) ci; \
	fi

frontend-install: ## Install dashboard dependencies (npm ci)
	cd frontend && $(NPM) ci

frontend-build: frontend-deps ## Build the dashboard into frontend/dist (embedded by go:embed)
	cd frontend && $(NPM) run build

frontend-test: frontend-deps ## Run dashboard unit tests (vitest)
	cd frontend && $(NPM) run test

frontend-lint: frontend-deps ## Lint (oxlint) + format-check (oxfmt) the dashboard
	cd frontend && $(NPM) run lint && $(NPM) run format:check

## frontend-check: lint + test the dashboard; skips loudly when Node is absent
frontend-check:
	@$(MAKE) frontend-src-guard
	@if ! command -v $(NODE) >/dev/null 2>&1; then \
	  echo "==> frontend checks SKIPPED (node not found — skipped is not passed)"; \
	else \
	  $(MAKE) frontend-lint frontend-test; \
	fi

# The dashboard is TypeScript-only: Vite resolves a stray .js BEFORE the real
# .tsx source, so a leftover transpile silently shadows the file it was
# generated from and `frontend/dist` gets built from stale code. Fail loudly
# instead (see issue #75).
frontend-src-guard:
	@echo "==> frontend/src stray-JS guard"
	@bad=$$(find frontend/src -type f \( -name '*.js' -o -name '*.jsx' \) 2>/dev/null); \
	if [ -n "$$bad" ]; then \
	  echo "stray JavaScript in the TS-only frontend/src (Vite would prefer it over .tsx):"; \
	  echo "$$bad"; \
	  echo "delete them: rm frontend/src/*.js frontend/src/*.test.js"; \
	  exit 1; \
	fi

# --- Dev sandbox (docker compose) ----------------------------------------------

# Pull the host-port overrides from a repo-root .env (the same file docker
# compose reads for its ${VAR} substitution) so the URLs `dev-up` / `prod-up`
# print agree with the ports the stack actually binds after
# `cp .env.example .env`. `?=` keeps the shell environment / command line
# authoritative — matching docker-compose's own precedence (shell env > .env) —
# and the ${VAR:-default} fallbacks still apply when .env is absent or the
# variable is not defined there. Values may carry trailing # comments / quotes
# (as compose tolerates); only HOST_UID/HOST_GID/HOME are never read this way
# (they are auto-derived or intentionally not overridable via .env).
ENV_VAL = $(shell sed -n 's/^$1=//p' .env 2>/dev/null | sed 's/^[[:space:]]*//; s/[[:space:]]*#.*$$//; s/^"//; s/"$$//' | tail -1)
FRONTEND_PORT ?= $(call ENV_VAL,FRONTEND_PORT)
HOST_PORT ?= $(call ENV_VAL,HOST_PORT)
API_PORT ?= $(call ENV_VAL,API_PORT)
POSTGRES_PORT ?= $(call ENV_VAL,POSTGRES_PORT)
export FRONTEND_PORT HOST_PORT API_PORT POSTGRES_PORT

preflight: ## Check host prerequisites for the dev sandbox
	@mkdir -p "$(HOME)/.config/opencode" "$(HOME)/.config/gh" && touch "$(HOME)/.gitconfig"
	@scripts/resolve-gh-token.sh --check >/dev/null 2>&1 || true

require-agent:
	@test "$$(head -c4 "$(HOME)/.opencode/bin/opencode" 2>/dev/null | od -An -tx1 | tr -d ' 
')" = "7f454c46" || { echo "ERROR: Linux ELF opencode required" >&2; exit 1; }

require-agent:
	@test "$$(head -c4 "$(HOME)/.opencode/bin/opencode" 2>/dev/null | od -An -tx1 | tr -d ' \n')" = "7f454c46" || { echo "ERROR: Linux ELF opencode required" >&2; exit 1; }

dev-up: preflight ## Build and start the dev stack (db + sandbox + worker + Vite)
	@HOST_UID=$$(id -u) HOST_GID=$$(id -g) GH_TOKEN=$$(scripts/resolve-gh-token.sh 2>/dev/null) GITHUB_TOKEN=$$(scripts/resolve-gh-token.sh 2>/dev/null) docker compose --profile dev up -d --build
	@echo ""
	@echo "  console (Vite): http://localhost:$${FRONTEND_PORT:-5173}/"
	@echo "  dev API (air):  http://localhost:$${HOST_PORT:-8081}/"
	@echo "  API docs:       http://localhost:$${HOST_PORT:-8081}/docs"
	@echo "  seed jobs:      API_URL=http://localhost:$${HOST_PORT:-8081} make demo"
	@echo "  logs:           make dev-log"
	@echo ""

dev-restart: preflight ## Restart the dev container after Dockerfile/deps changes
	@docker compose --profile dev restart dev

dev-down: ## Stop and remove the dev stack (db + sandbox + worker + frontend)
	@docker compose --profile dev down

dev-log: ## Follow dev sandbox + worker + frontend logs
	@docker compose --profile dev logs -f dev worker frontend

dev-build: preflight ## Rebuild the dev image without starting
	@docker compose --profile dev build

reset: ## Stop both stacks and remove the Postgres volume
	@docker compose --profile dev --profile prod down -v

shell: dev-shell ## Alias for dev-shell
opencode: dev-opencode ## Alias for dev-opencode (run opencode inside the sandbox)
opencode-auto: dev-opencode-auto ## Alias for dev-opencode-auto (run opencode with --auto)

dev-shell: preflight ## Open a shell in the dev container
	@docker compose --profile dev exec dev bash || docker compose --profile dev run --rm dev bash

dev-opencode: preflight ## Run opencode inside the dev sandbox
	@if [ -z "$$(docker compose --profile dev ps -q dev 2>/dev/null)" ]; then echo "[opencode] starting dev stack..."; HOST_UID=$$(id -u) HOST_GID=$$(id -g) GH_TOKEN=$$(scripts/resolve-gh-token.sh 2>/dev/null) GITHUB_TOKEN=$$(scripts/resolve-gh-token.sh 2>/dev/null) docker compose --profile dev up -d dev; fi
	@if [ -t 0 ]; then docker compose --profile dev exec -it dev zsh -lc "cd /workspace && opencode $(OPENCODE_ARGS)" || docker compose --profile dev exec -it dev bash -lc "opencode $(OPENCODE_ARGS)"; else docker compose --profile dev exec -T dev zsh -lc "cd /workspace && opencode $(OPENCODE_ARGS)" || docker compose --profile dev exec -T dev bash -lc "opencode $(OPENCODE_ARGS)"; fi

# Same recipe, with --auto: auto-approve permissions that are not explicitly
# denied (every secret-file deny rule still applies).
dev-opencode-auto: OPENCODE_ARGS = --auto
dev-opencode-auto: dev-opencode ## Run opencode inside the dev sandbox with --auto

# --- Production stack (docker compose, profile: prod) --------------------------
# Brings up Postgres + API + worker from the `runtime` image; the API serves the
# embedded dashboard at / and the OpenAPI docs at /docs. Gated behind the `prod`
# profile so it never drags in the dev sandbox.

prod-up: ## Build + start the production stack (db + api + worker), in the background
	@docker compose --profile prod up -d --build
	@echo ""
	@echo "  dashboard:  http://localhost:$${API_PORT:-8080}/"
	@echo "  API docs:   http://localhost:$${API_PORT:-8080}/docs"
	@echo "  seed jobs:  make demo"
	@echo "  logs:       make prod-logs"
	@echo ""

prod-down: ## Stop the production stack (keeps the Postgres volume)
	@docker compose --profile prod down

prod-logs: ## Follow api + worker logs
	@docker compose --profile prod logs -f api worker

prod-ps: ## Show production-stack service status
	@docker compose --profile prod ps

prod-reset: ## Stop the production stack and delete the Postgres volume
	@docker compose --profile prod down -v

demo: ## Enqueue demo jobs against the running stack (needs curl)
	@scripts/demo.sh

.PHONY: screenshots
screenshots:
	cd frontend && node tools/screenshot.cjs
