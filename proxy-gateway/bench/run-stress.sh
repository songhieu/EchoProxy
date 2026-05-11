#!/usr/bin/env bash
# Stress-test proxy-gateway: ramp RPS until p99 breaks the 20ms SLO.
#
# Assumes `make up && make migrate-postgres` has already been run.
# Seeds sk_test_demo, snapshots drop counters before/after, runs k6.
#
# Knobs:
#   STAGES="1000,5000,10000,20000,35000,50000"  per-stage RPS
#   STAGE_DUR=30s                                duration per stage
#   PROXY_URL=http://localhost:8080
#   UPSTREAM_URL=http://upstream-mock:9000/echo
#   ADMIN_URL=http://localhost:6060

set -euo pipefail

cd "$(dirname "$0")"

PROXY_URL="${PROXY_URL:-http://localhost:8080}"
ADMIN_URL="${ADMIN_URL:-http://localhost:6060}"
UPSTREAM_URL="${UPSTREAM_URL:-http://upstream-mock:9000}"
ECHO_KEY="${ECHO_KEY:-sk_test_demo}"
STAGES="${STAGES:-1000,5000,10000,20000,35000,50000}"
STAGE_DUR="${STAGE_DUR:-30s}"

red()   { printf '\033[31m%s\033[0m\n' "$*"; }
green() { printf '\033[32m%s\033[0m\n' "$*"; }
blue()  { printf '\033[34m%s\033[0m\n' "$*"; }

require() {
  command -v "$1" >/dev/null 2>&1 || { red "missing: $1"; exit 1; }
}
require docker
require curl

# 1. Sanity: stack up?
if ! curl -fsS "${ADMIN_URL}/healthz" >/dev/null 2>&1; then
  red "proxy-gateway admin (${ADMIN_URL}/healthz) unreachable. Run \`make up\` first."
  exit 1
fi

# 2. Seed the bench API key (idempotent). Find the postgres container by
#    label so we work with both `docker compose` and legacy `docker-compose`.
blue "→ seeding API key (sk_test_demo)"
pg_container=$(docker ps --filter 'label=com.docker.compose.service=postgres' \
                          --filter 'status=running' \
                          --format '{{.Names}}' | head -n1)
if [[ -z "$pg_container" ]]; then
  red "no running postgres container found (label com.docker.compose.service=postgres)"
  exit 1
fi
docker exec -i "$pg_container" \
  psql -U echoproxy -d echoproxy -v ON_ERROR_STOP=1 < seed-key.sql >/dev/null

# 3. Smoke check: one request must succeed before we ramp.
blue "→ smoke check"
code=$(curl -s -o /dev/null -w '%{http_code}' \
  -H "X-Echo-Key: ${ECHO_KEY}" \
  -H "X-Echo-Target: ${UPSTREAM_URL}" \
  -X POST -d '{"foo":"bar"}' "${PROXY_URL}/echo")
if [[ "$code" != "200" ]]; then
  red "smoke check failed: HTTP ${code}. Check proxy logs and upstream-mock."
  exit 1
fi
green "  ok (200)"

# 4. Snapshot drop counters before.
metric() {
  curl -fsS "${ADMIN_URL}/metrics" | awk -v k="$1" '$1==k {print $2; exit}'
}
dropped_before=$(metric proxy_dropped_events_total || echo 0)
dropped_before=${dropped_before:-0}

# 5. Run k6 (host network so it can hit localhost:8080 and the docker
#    upstream-mock hostname resolution is irrelevant — proxy resolves it).
blue "→ running k6 stress (stages: ${STAGES}, ${STAGE_DUR} each)"
mkdir -p results
ts=$(date +%Y%m%d_%H%M%S)
summary="results/stress_${ts}.json"

docker run --rm -i --network host \
  -e PROXY_URL="${PROXY_URL}" \
  -e UPSTREAM_URL="${UPSTREAM_URL}" \
  -e ECHO_KEY="${ECHO_KEY}" \
  -e STAGES="${STAGES}" \
  -e STAGE_DUR="${STAGE_DUR}" \
  -v "$PWD:/work" -w /work \
  grafana/k6 run --summary-export="${summary}" k6-stress.js

# 6. Snapshot drop counters after; report delta.
dropped_after=$(metric proxy_dropped_events_total || echo 0)
dropped_after=${dropped_after:-0}
delta=$(( dropped_after - dropped_before ))

echo
blue "──── post-run ────"
echo "proxy_dropped_events_total: ${dropped_before} → ${dropped_after} (Δ ${delta})"
echo "summary written: proxy-gateway/bench/${summary}"
echo
echo "Tip: scan the k6 output above for the highest stage where"
echo "  http_req_duration{stage:NN_Xrps}  p(99) is still < 20ms"
echo "  AND http_req_failed{stage:NN_Xrps} rate is still < 0.1%."
echo "That is the proxy's sustainable RPS on this host."
