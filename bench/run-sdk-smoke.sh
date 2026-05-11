#!/usr/bin/env bash
# End-to-end smoke for every SDK's proxy mode. For each language SDK:
#   1. set ECHOPROXY_TAG to a unique value
#   2. invoke the SDK's proxy_smoke example to issue one request
#   3. wait for log-consumer flush
#   4. query ClickHouse for the event by path; assert exactly one row
#
# Each SDK is run independently so a single language's tooling failure does
# not block the others. The script exits non-zero if any SDK fails to land
# its event in ClickHouse.

set -uo pipefail

cd "$(dirname "$0")/.."

PROXY_URL="${PROXY_URL:-http://localhost:8080}"
TARGET="${ECHOPROXY_EXAMPLE_TARGET:-http://upstream-mock:9000}"
KEY="${ECHOPROXY_API_KEY:-sk_test_demo}"
RUN_ID="${RUN_ID:-$(date +%s)-$$}"

green() { printf '\033[32m%s\033[0m\n' "$*"; }
red()   { printf '\033[31m%s\033[0m\n' "$*"; }
blue()  { printf '\033[34m%s\033[0m\n' "$*"; }

ch_query() {
  docker exec echoproxy-clickhouse-1 clickhouse-client -d echoproxy --query "$1"
}

verify_event() {
  local lang=$1 tag=$2
  local path="/api/users/sdkbench-${lang}-${tag}"
  # Up to 8s for log-consumer to batch+flush.
  for _ in $(seq 1 16); do
    n=$(ch_query "SELECT count() FROM http_events WHERE path = '${path}' AND ts > now() - INTERVAL 60 SECOND")
    if [[ "$n" == "1" ]]; then
      green "  ✓ ${lang}: event landed in ClickHouse (path=${path})"
      return 0
    fi
    sleep 0.5
  done
  red "  ✗ ${lang}: event NOT found (path=${path}, count=${n:-0})"
  return 1
}

results=()
record() {
  local lang=$1 ok=$2
  if [[ "$ok" == "0" ]]; then
    results+=("✓ ${lang}")
  else
    results+=("✗ ${lang}")
  fi
}

# ─── 1. Sanity ──────────────────────────────────────────────────────────────
blue "→ checking proxy is up"
if ! curl -fsS "${PROXY_URL%:*}:6060/healthz" >/dev/null 2>&1; then
  red "proxy admin /healthz unreachable. Run \`make up\` first."
  exit 1
fi

# Make sure the bench API key is seeded onto the visible project.
docker exec -i echoproxy-postgres-1 psql -U echoproxy -d echoproxy -v ON_ERROR_STOP=1 \
  < proxy-gateway/bench/seed-key.sql >/dev/null

env_common=(
  "ECHOPROXY_API_KEY=${KEY}"
  "ECHOPROXY_PROXY_URL=${PROXY_URL}"
  "ECHOPROXY_EXAMPLE_TARGET=${TARGET}"
)

# ─── 2. Go ──────────────────────────────────────────────────────────────────
blue "→ Go SDK"
go_tag="${RUN_ID}-go"
(
  cd sdk-reference-go
  env "${env_common[@]}" ECHOPROXY_TAG="${go_tag}" \
    go run ./examples/proxy_smoke
) || true
verify_event "go" "${go_tag}"; record "go" $?

# ─── 3. Python ──────────────────────────────────────────────────────────────
blue "→ Python SDK"
py_tag="${RUN_ID}-py"
py_venv=".venv-sdk-smoke"
(
  cd sdk-python
  if [[ ! -d "${py_venv}" ]]; then
    python3 -m venv "${py_venv}"
    "./${py_venv}/bin/pip" install --quiet --upgrade pip
    "./${py_venv}/bin/pip" install --quiet -e . requests
  fi
  env "${env_common[@]}" ECHOPROXY_TAG="${py_tag}" \
    "./${py_venv}/bin/python" examples/proxy_smoke.py
) || true
verify_event "py" "${py_tag}"; record "py" $?

# ─── 4. TypeScript ──────────────────────────────────────────────────────────
blue "→ TypeScript SDK"
ts_tag="${RUN_ID}-ts"
(
  cd sdk-ts
  [[ -d node_modules ]] || pnpm install --silent
  env "${env_common[@]}" ECHOPROXY_TAG="${ts_tag}" \
    node --experimental-strip-types examples/proxy_smoke.ts
) || true
verify_event "ts" "${ts_tag}"; record "ts" $?

# ─── 5. Laravel (raw PHP via Guzzle) ────────────────────────────────────────
blue "→ Laravel SDK (Guzzle wire contract)"
php_tag="${RUN_ID}-php"
(
  cd sdk-laravel
  [[ -d vendor ]] || composer install --quiet --no-interaction
  env "${env_common[@]}" ECHOPROXY_TAG="${php_tag}" \
    php examples/proxy_smoke.php
) || true
verify_event "php" "${php_tag}"; record "php" $?

# ─── Report ─────────────────────────────────────────────────────────────────
echo
blue "──── results ────"
for r in "${results[@]}"; do echo "  $r"; done

# Exit non-zero if any line starts with ✗
if printf '%s\n' "${results[@]}" | grep -q '^✗'; then
  exit 1
fi
green "All SDKs passed end-to-end."
