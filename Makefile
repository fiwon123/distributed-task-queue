# distributed-task-queue — local development gate.
#
# CI runs only on `dev` -> `main` PRs, so `make check` is the gate that
# feature branches rely on. Until `go.mod` exists, the Go toolchain steps are
# SKIPPED (a skipped run is not a passed run), keeping scaffolding PRs usable.

GO ?= go

.PHONY: help check fmt vet test build run run-worker clean \
        preflight dev-up dev-down dev-restart dev-log dev-build reset \
        shell opencode dev-shell dev-opencode

# Opt-in: `make opencode AUTO=1` (or `make dev-opencode AUTO=1`) adds --auto —
# opencode auto-approves permissions that are not explicitly denied; the
# secret-file deny rules still apply. Default: prompt.
AUTO ?= 0
OPENCODE_AUTO := $(if $(filter 1 true yes on,$(AUTO)), --auto,)

help: ## List available targets
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

## check: gofmt + vet + test + build (the local gate before pushing)
check:
	@if [ ! -f go.mod ]; then \
	  echo "==> no go.mod yet — Go toolchain checks SKIPPED (skipped is not passed)"; \
	else \
	  $(MAKE) fmt vet test build; \
	fi

fmt:
	@echo "==> gofmt"
	@out="$$(gofmt -l .)"; \
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

dev-shell: preflight ## Open a shell in the dev container
	@docker compose exec dev bash || docker compose run --rm dev bash

dev-opencode: preflight ## Run opencode inside the dev sandbox (AUTO=1 => --auto)
	@if [ -z "$$(docker compose ps -q dev 2>/dev/null)" ]; then echo "[opencode] starting dev stack..."; HOST_UID=$$(id -u) HOST_GID=$$(id -g) GH_TOKEN=$$(scripts/resolve-gh-token.sh 2>/dev/null) GITHUB_TOKEN=$$(scripts/resolve-gh-token.sh 2>/dev/null) docker compose up -d dev; fi
	@if [ -t 0 ]; then docker compose exec -it dev zsh -lc "cd /workspace && opencode$(OPENCODE_AUTO)" || docker compose exec -it dev bash -lc "opencode$(OPENCODE_AUTO)"; else docker compose exec -T dev zsh -lc "cd /workspace && opencode$(OPENCODE_AUTO)" || docker compose exec -T dev bash -lc "opencode$(OPENCODE_AUTO)"; fi
