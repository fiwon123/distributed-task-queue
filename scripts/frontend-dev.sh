#!/bin/bash
# frontend-dev.sh — run the Vite dev server for the dashboard inside the dev
# stack, so `make dev-up` gives hot reload on the frontend as well as the
# (air-reloaded) API.
#
#   docker compose --profile dev up   # starts this via the `frontend` service
#
# Dependencies are installed on first use, and reinstalled whenever the Node/npm
# version or the lockfile changes. node_modules installed on a *host* (e.g. macOS
# or a different libc) is not valid inside this Linux image, so the install is
# stamped with `node -v` / `npm -v`: a missing or mismatched stamp forces a
# container-side `npm ci`.
set -euo pipefail

FRONTEND_DIR="${FRONTEND_DIR:-/workspace/frontend}"
API_PROXY="${API_PROXY:-http://localhost:8080}"

cd "$FRONTEND_DIR"

STAMP="node_modules/.install-stamp"
WANT="node $(node -v) npm $(npm -v)"

if [ ! -f "$STAMP" ] || [ "$(cat "$STAMP" 2>/dev/null)" != "$WANT" ] || [ package-lock.json -nt "$STAMP" ]; then
    echo "[frontend] installing dashboard dependencies (npm ci)..."
    npm ci
    printf '%s\n' "$WANT" >"$STAMP"
fi

# The compose service publishes this port (FRONTEND_PORT on the host side), so
# it is fixed inside the container — keep in sync with docker-compose.yml.
echo "[frontend] starting Vite on 0.0.0.0:5173 (proxying the API at ${API_PROXY})"
exec npm run dev -- --host 0.0.0.0 --port 5173 --strictPort
