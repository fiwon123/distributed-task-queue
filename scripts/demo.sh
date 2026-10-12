#!/bin/bash
# demo.sh — enqueue a few demo jobs against a running stack so the dashboard has
# something to show. Safe to run repeatedly; each run adds new jobs.
#
#   make demo                          # against http://localhost:8080
#   API_PORT=9090 make demo            # against another host port
#   API_URL=http://host:8080 make demo # explicit base URL
set -uo pipefail

API_URL="${API_URL:-http://localhost:${API_PORT:-8080}}"
QUEUE="${DEMO_QUEUE:-default}"

if ! command -v curl >/dev/null 2>&1; then
    echo "demo: curl is required" >&2
    exit 1
fi

# Wait for the API (and its database) to answer.
printf 'Waiting for the API at %s ' "$API_URL"
ready=""
for _ in $(seq 1 60); do
    if curl -fsS -o /dev/null "$API_URL/v1/queues" 2>/dev/null; then
        ready=1
        break
    fi
    printf '.'
    sleep 1
done
echo

if [ -z "$ready" ]; then
    echo "demo: API did not become ready at $API_URL" >&2
    echo "demo: is the stack up? try 'make dev-up' or 'make prod-up' first" >&2
    exit 1
fi

# enqueue <json-body> — POST a job, echo the new job id (or fail loudly).
enqueue() {
    local body="$1" resp
    if ! resp="$(curl -fsS -X POST "$API_URL/v1/jobs" \
        -H 'content-type: application/json' -d "$body")"; then
        echo "demo: enqueue failed for $body" >&2
        return 1
    fi
    printf '%s' "$resp" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p'
}

echo "Seeding demo jobs into queue '$QUEUE':"
echo

echo "  echo   (succeeds)"
id="$(enqueue "{\"type\":\"echo\",\"queue\":\"$QUEUE\",\"payload\":{\"msg\":\"hello from the demo\"}}")"
echo "         id=$id"

echo "  echo   (succeeds)"
id="$(enqueue "{\"type\":\"echo\",\"queue\":\"$QUEUE\",\"payload\":{\"msg\":\"a second echo\"}}")"
echo "         id=$id"

echo "  sleep  (2s, then succeeds)"
id="$(enqueue "{\"type\":\"sleep\",\"queue\":\"$QUEUE\",\"payload\":{\"seconds\":2}}")"
echo "         id=$id"

echo "  fail   (always fails; max_attempts=2 → dead-letters)"
id="$(enqueue "{\"type\":\"fail\",\"queue\":\"$QUEUE\",\"max_attempts\":2,\"payload\":{\"nope\":true}}")"
echo "         id=$id"

echo
echo "Done. Watch the worker process them with:  make prod-logs  (dev stack: make dev-log)"
echo "Dashboard:                                 ${API_URL%/}/"
echo "API docs:                                  ${API_URL%/}/docs"
echo
echo "Tip: the 'fail' job retries once, then moves to the dead-letter state —"
echo "     find it with  curl -s '${API_URL%/}/v1/jobs?queue=$QUEUE&status=dead'"
echo "     and requeue it with  curl -sX POST '${API_URL%/}/v1/jobs/<id>/requeue'"
