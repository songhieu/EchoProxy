# proxy-gateway bench

Two flavors:

| Script              | Goal                                            |
|---------------------|-------------------------------------------------|
| `k6.js`             | SLO gate. Fixed 5000 RPS for 60s, fails CI if p99 ≥ 20ms. |
| `k6-stress.js`      | Capacity test. Ramps RPS across stages and reports per-stage latency so you can find the breaking point. |

## Prereqs

```bash
make up              # full stack incl. proxy-gateway + upstream-mock
make migrate-postgres
```

## SLO gate (existing)

```bash
make bench-proxy
```

## Stress test (new)

```bash
make bench-stress
# or
proxy-gateway/bench/run-stress.sh
```

What it does:

1. Seeds `sk_test_demo` into Postgres (idempotent — `seed-key.sql`).
2. Smoke-checks one request through the proxy → upstream-mock.
3. Snapshots `proxy_dropped_events_total` from the admin port.
4. Runs `k6-stress.js` with stages **1k → 5k → 10k → 20k → 35k → 50k RPS**, 30s each.
5. Re-reads the drop counter and prints the delta.

The k6 summary tags each stage so you'll see lines like:

```
http_req_duration{stage:03_10000rps}  ... p(99)=8.41ms
http_req_duration{stage:05_35000rps}  ... p(99)=42.3ms   ← SLO broken here
http_req_failed{stage:06_50000rps}    ... 0.13%          ← errors appear
```

The largest stage where **p99 < 20ms AND failure rate < 0.1%** is the proxy's
sustainable RPS on this host.

## Knobs

```bash
STAGES="500,1000,2500,5000" STAGE_DUR=20s make bench-stress
PROXY_URL=http://prod-proxy:8080 ADMIN_URL=http://prod-proxy:6060 make bench-stress
```

## What the numbers mean

- **`proxy_request_duration_seconds`** (in proxy metrics) is end-to-end including upstream. The k6-side `http_req_duration` adds the bench host → proxy hop too. For overhead-only numbers, look at the proxy's own histogram via Prometheus.
- **`proxy_dropped_events_total` Δ** > 0 means the async event channel filled up. That happens before request latency degrades — drops are the earliest signal of overload.
- The mock upstream sleeps ~1ms (`mock-upstream/main.go`), so `http_req_duration` ≈ proxy overhead + 1ms + network.

## Caveats

- Run on a host with enough CPU. On a 4-core laptop you'll cap out around 10–20k RPS — that's the laptop, not the proxy.
- `--network host` only works on Linux. On macOS, the script still runs via `docker run` against `localhost:8080`, but expect lower numbers due to the docker network hop.
- `ALLOW_PRIVATE_TARGETS=true` must be set on proxy-gateway (already set in `docker-compose.yml`).
