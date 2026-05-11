# EchoProxy

> Self-hosted HTTP observability. Capture every request / response, store the
> timeline in ClickHouse, explore from a Next.js dashboard. The proxy mode
> needs zero code changes in your app — just point traffic at it.

[![CI](https://github.com/songhieu/EchoProxy/actions/workflows/ci.yml/badge.svg)](https://github.com/songhieu/EchoProxy/actions/workflows/ci.yml)
[![Release](https://github.com/songhieu/EchoProxy/actions/workflows/release.yml/badge.svg)](https://github.com/songhieu/EchoProxy/actions/workflows/release.yml)
[![Latest release](https://img.shields.io/github/v/release/songhieu/EchoProxy?display_name=tag&sort=semver&logo=github&label=release)](https://github.com/songhieu/EchoProxy/releases/latest)
[![PyPI](https://img.shields.io/pypi/v/echoproxy-sdk?logo=pypi&label=pypi)](https://pypi.org/project/echoproxy-sdk/)
[![Packagist](https://img.shields.io/packagist/v/echoproxy/sdk-laravel?logo=packagist&label=packagist)](https://packagist.org/packages/echoproxy/sdk-laravel)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8.svg?logo=go)](https://go.dev)
[![Next.js](https://img.shields.io/badge/Next.js-14-black.svg?logo=nextdotjs)](https://nextjs.org)
[![ClickHouse](https://img.shields.io/badge/ClickHouse-24.8-FFCC01.svg?logo=clickhouse)](https://clickhouse.com)
[![ghcr](https://img.shields.io/badge/ghcr.io-songhieu%2FEchoProxy--*-2088FF.svg?logo=github)](https://github.com/songhieu?tab=packages&repo_name=echoproxy)

EchoProxy is a "Datadog APM but self-hosted, focused on HTTP traffic" — built
for backend teams who want to debug request/response timelines without
instrumenting every service, and who don't want to ship raw request bodies
to a third-party SaaS.

---

## Features

- **Captures both directions** — outbound (your app → upstreams) **and** inbound (clients → your app). One schema, one dashboard, filterable.
- **Zero-instrumentation proxy mode** for outbound — set `X-Echo-Target: https://api.upstream.com` and route traffic through `proxy-gateway:8080`. No SDK install required.
- **Framework middleware** for inbound — drop into Express, FastAPI / Starlette, Flask / Django, Laravel HTTP kernel, Go `http.Handler`. Captures incoming requests + your responses.
- **SDK mode** for richer in-app context — official SDKs for **Go**, **Python**, **TypeScript**, and **Laravel/PHP**.
- **Per-project retention** (1–90 days), configurable from the dashboard. Cleanup binary enforces nightly via ClickHouse mutations.
- **Latency breakdown** per request — total / upstream RoundTrip / upstream TTFB / proxy overhead, captured via `httptrace.ClientTrace`.
- **Auth failure logging** — invalid / revoked / disallowed keys, rate-limit hits, all surface in the dashboard with `error` tags (`invalid_api_key`, `revoked_api_key`, `target_not_allowed`, …).
- **Defense-in-depth redaction** — applied at SDK, proxy, *and* ingest-api. Default rules cover JWTs, AWS / Stripe / GitHub / Slack tokens, Luhn-validated credit cards, common credential headers and JSON fields.
- **Hot path < 20ms p99** in proxy-gateway: in-memory API-key cache, capped body capture via `io.TeeReader`, async drop-on-overflow event channel, franz-go async Kafka producer.
- **Custom date range** down to minute precision in logs + analytics. Plus presets (15m / 1h / 6h / 24h / 7d / 30d) for the common case.
- **Built-in stress test** (`make bench-stress`) — ramps proxy from 1k → 50k RPS with realistic mock traffic, reports per-stage p50/p99/error rate.

## Architecture

```
              ┌── inbound: SDK middleware ──┐
              ▼                              │
Client ─HTTP─────────────▶  Your app  ──────┴───SDK──▶ ingest-api
                                │                        (HTTP :8081
                                │                         + gRPC :8082)
                                │
                            ┌───┴── outbound ────┐
                            ▼                    ▼
                       proxy-gateway       SDK HTTP wrapper
                       (X-Echo-Target)     (Go / Py / TS / PHP)
                            │                    │
                            ▼                    ▼
                         Upstream            ingest-api
                                                 │
              ┌──────── all paths converge ◀────┘
              ▼
       Kafka (http_events)
              │
              ▼
     log-consumer ──▶ ClickHouse
                            │
                            ▼
     auth-api  ── Postgres (users/projects/api_keys)
     stats-api ── ClickHouse + Redis cache ──▶ dashboard (Next.js)
     cleanup   ── nightly retention sweep
```

Three capture paths, one event schema:

- **Outbound via proxy** — route traffic at `proxy-gateway:8080` with `X-Echo-Target`. Zero code change.
- **Outbound via SDK** — wrap the HTTP client (`fetch`, `requests`, `http.Client`, Guzzle). For runtimes that can't reach the proxy.
- **Inbound via SDK middleware** — wrap your web framework (Express, FastAPI, Laravel HTTP kernel, Go `http.Handler`). Captures incoming traffic + responses you return.

Events from any path land in the same Kafka topic and ClickHouse table,
distinguished by the `direction` column. See
[`docs/inbound-vs-outbound.md`](docs/inbound-vs-outbound.md) for the
decision guide.

Every Go service follows Clean Architecture (`domain ← usecase ← adapter ← infra/cmd`).
proxy-gateway keeps p99 overhead < 20ms by making the hot path I/O-free —
detail in `.claude/skills/go-http-proxy-low-latency/`.

## Quick start (Docker)

Requires Docker Desktop or Docker Engine + Compose v2.

### 1. Clone & boot

```bash
git clone https://github.com/songhieu/EchoProxy.git
cd echoproxy
make up
```

`make up` builds every image and brings up the full stack:
proxy-gateway, ingest-api, log-consumer, auth-api, stats-api, cleanup,
dashboard, postgres, clickhouse, kafka, redis, prometheus, grafana,
upstream-mock. First boot takes ~2 minutes (Go builds).

### 2. Migrate

On a **fresh** `make up`, postgres + clickhouse auto-apply everything
under `migrations/` via `docker-entrypoint-initdb.d`. You can verify or
re-run idempotently:

```bash
make migrate          # runs all *.sql in migrations/{postgres,clickhouse}
```

Add a new migration later? Drop a numbered file into `migrations/postgres/`
or `migrations/clickhouse/` and rerun `make migrate` — the
init dir only fires on a brand-new container, so existing stacks need
this manual step.

### 3. Sign up + create a project

Open http://localhost:3000 → **Sign up**. You'll be logged in
automatically and dropped at `/projects`. Hit **+ New project**, give it
a name (e.g. "production"). The project auto-creates with default
retention 30 days (configurable later in **Settings**).

### 4. Create an API key

Inside the project, go to **API keys** → **+ New key**.

- **Allowlist**: leave empty to accept any upstream host. Add
  `api.stripe.com` etc. to lock the key to specific hosts.
- **Body cap**: defaults to 64 KB; raise if you have large payloads.
- **Rate limit (RPS)**: 0 = unlimited.

The raw key (`sk_live_…`) is shown **once**. Copy it now.

### 5. Send your first request

```bash
export KEY="sk_live_xxx"          # the key shown in step 4
export PROXY="http://localhost:8080"

# Proxy mode — zero code change required
curl -H "X-Echo-Key: $KEY" \
     -H "X-Echo-Target: https://httpbin.org" \
     $PROXY/get
```

### 6. Watch it land

- **Logs** tab: your `GET /get` appears within ~1 second with full
  headers, body, status, and the
  total / upstream / TTFB / overhead breakdown.
- **Analytics** tab: request volume + latency percentiles chart.
- **Live tail** tab: SSE stream as new events arrive.

### 7. (Optional) Try SDK mode

If you prefer to keep traffic on the same hostnames in your app code:

```bash
ECHOPROXY_API_KEY=$KEY bash bench/run-sdk-smoke.sh
# runs one request through each of Go / Python / TypeScript / Laravel,
# then verifies the event landed in ClickHouse
```

Failed: `bash bench/run-sdk-smoke.sh` exits non-zero, prints which SDK
broke. Pass: 4×✓ + "All SDKs passed end-to-end."

## SDK examples

Every SDK supports two capture directions — see
[`docs/inbound-vs-outbound.md`](docs/inbound-vs-outbound.md) for the
concept and when to pick each:

- **Outbound** — calls *your app makes* to upstreams. Use either the
  proxy URL rewrite (zero code change) or the SDK's HTTP-client wrapper.
- **Inbound** — calls *clients make to your app*. Drop the framework
  middleware into your HTTP kernel.

Both directions share env vars: `ECHOPROXY_API_KEY` (required),
`ECHOPROXY_PROXY_URL` (default `http://localhost:8080`) for outbound proxy
mode, or `ECHOPROXY_ENDPOINT_HTTP` (default `http://localhost:8081`) when
shipping events directly to ingest-api.

### Go

**Outbound (proxy mode, zero code change)**:

```go
import sid "echoproxy/sdk-reference-go/proxy"

res, err := sid.Get("https://api.example.com/users/42")
// or drop into an existing http.Client:
client := &http.Client{Timeout: 5*time.Second, Transport: &sid.Transport{}}
```

**Inbound (server middleware)**:

```go
import sdk "echoproxy/sdk-reference-go"

c, _ := sdk.New(sdk.Config{
    APIKey:       os.Getenv("ECHOPROXY_API_KEY"),
    EndpointHTTP: "http://ingest-api:8081",
    IgnoreRoutes: []string{`^/healthz$`, `^/metrics$`},
})
defer c.Close()

mux := http.NewServeMux()
mux.HandleFunc("/api/...", yourHandler)
http.ListenAndServe(":8000", c.Middleware(mux))
```

### Python

**Outbound (proxy mode)**:

```python
import echoproxy.proxy as sid

r = sid.session().get("https://api.example.com/users/42", timeout=5)
```

**Inbound (WSGI — Flask / Django)**:

```python
from echoproxy import Client
from echoproxy.wsgi import CaptureMiddleware

client = Client(api_key=os.environ["ECHOPROXY_API_KEY"],
                endpoint_http="http://ingest-api:8081",
                ignore_routes=[r"^/healthz$"])
app.wsgi_app = CaptureMiddleware(app.wsgi_app, client)
```

**Inbound (ASGI — FastAPI / Starlette)**:

```python
from echoproxy import Client
from echoproxy.asgi import CaptureMiddleware

client = Client(api_key=os.environ["ECHOPROXY_API_KEY"],
                endpoint_http="http://ingest-api:8081")
app.add_middleware(CaptureMiddleware, client=client)
```

### TypeScript / Node

**Outbound (proxy mode)**:

```ts
import { fetch } from "@echoproxy/sdk/proxy";

const r = await fetch("https://api.example.com/users/42");
```

**Inbound (Express / connect)**:

```ts
import express from "express";
import { IngestClient, expressMiddleware } from "@echoproxy/sdk";

const client = new IngestClient({
  apiKey: process.env.ECHOPROXY_API_KEY!,
  endpoint: "http://ingest-api:8081",
  ignoreRoutes: [/^\/healthz$/, /^\/metrics$/],
});

const app = express();
app.use(expressMiddleware(client));   // mount BEFORE your routes
app.get("/api/users/:id", handler);
```

### Laravel / PHP

**Outbound (proxy mode, via service provider)**:

```php
use Echoproxy\Sdk\ProxyClient;

$res = app(ProxyClient::class)->get("https://api.example.com/users/42");
```

**Outbound (Guzzle middleware — for non-Laravel PHP apps)**:

```php
$stack = HandlerStack::create();
$stack->push(GuzzleMiddleware::create($client));
$http = new GuzzleHttp\Client(['handler' => $stack]);
```

**Inbound (Laravel HTTP middleware)**:

```php
// app/Http/Kernel.php
protected $middleware = [
    \Echoproxy\Sdk\Middleware\CaptureRequests::class,
    // ... your other middleware
];
```

Then register the SDK in a service provider:

```php
$this->app->singleton(\Echoproxy\Sdk\Client::class, function () {
    return new \Echoproxy\Sdk\Client(
        apiKey: env('ECHOPROXY_API_KEY'),
        endpoint: env('ECHOPROXY_ENDPOINT_HTTP', 'http://ingest-api:8081'),
        ignoreRoutes: ['#^/healthz$#', '#^/up$#'],
    );
});
```

### Wire contract (any language)

If your stack has no SDK, send the proxy two headers and call its URL:

```bash
curl -H "X-Echo-Key: $KEY" -H "X-Echo-Target: https://api.example.com" \
     http://localhost:8080/users/42
```

End-to-end SDK smoke (all 4 languages exercised against the live stack):

```bash
bash bench/run-sdk-smoke.sh
```

## Service endpoints

| Service        | Port  | Notes                                       |
|----------------|-------|---------------------------------------------|
| proxy-gateway  | 8080  | Public; routes via `X-Echo-Target`          |
| proxy admin    | 6060  | Internal: `/metrics`, `/healthz`, pprof     |
| ingest-api HTTP| 8081  | `POST /v1/events:batch`                     |
| ingest-api gRPC| 8082  | `echoproxy.v1.EventIngest`                  |
| auth-api       | 8083  | `POST /v1/login`, `/v1/projects/...`        |
| stats-api      | 8084  | `GET /v1/projects/:id/logs` etc.            |
| dashboard      | 3000  | Next.js                                     |
| Prometheus     | 9090  | http://localhost:9090                       |
| Grafana        | 3001  | http://localhost:3001 (anon admin)          |

## Configuration

Per-service env vars are documented in each `cmd/*/main.go` and the
`docker-compose.yml`. Most-tuned knobs:

| Env | Default | What |
|-----|---------|------|
| `EVENT_CHAN_SIZE` | `100000` | Proxy → Kafka in-memory queue. Bigger absorbs longer log-consumer outages, costs more RAM. |
| `BODY_CAP_BYTES` | `65536` | Per-side hard cap on captured req/res body. |
| `KAFKA_NUM_PARTITIONS` | `3` | **Raise to 32-128 for >100M req/day.** |
| `BATCH_SIZE` (log-consumer) | `1000` | Rows per ClickHouse insert batch. |
| `APIKEY_REFRESH_SECONDS` | `3` (compose) / `10` (default) | Proxy cache TTL for API key lookups. |
| `JWT_SECRET` | dev only | **Must be ≥ 32 chars** in production. |

See [`docs/retention.md`](docs/retention.md) for retention tuning.

## Documentation

| Doc | What |
|-----|------|
| [`docs/inbound-vs-outbound.md`](docs/inbound-vs-outbound.md) | Concept + decision guide for inbound (server middleware) vs outbound (proxy / SDK) capture |
| [`docs/sdk-publishing.md`](docs/sdk-publishing.md) | One tag → publish 4 SDKs to PyPI / npm / Packagist / Go proxy (CI automated) |
| [`docs/deployment/`](docs/deployment/) | Pick a platform: [docker-compose](docs/deployment/docker-compose.md) (single VM) or [Kubernetes via Helm](docs/deployment/kubernetes.md) |
| [`docs/retention.md`](docs/retention.md) | Per-project retention, cleanup scheduling, TTL tiers |
| [`docs/sdk-spec.md`](docs/sdk-spec.md) | Contract every SDK must implement |
| [`docs/architecture.md`](docs/architecture.md) | Deeper design notes |
| [`docs/runbook.md`](docs/runbook.md) | On-call: drop counter spikes, Kafka lag, CH disk |
| [`deploy/helm/echoproxy/`](deploy/helm/echoproxy/) | Helm chart (services, ingress, ServiceMonitor, CronJob) |
| [`proxy-gateway/bench/README.md`](proxy-gateway/bench/README.md) | Stress test harness (k6 + mock upstream) |
| [`.claude/skills/`](.claude/skills/) | Project-scoped skills for AI assistants working in this repo |

## Repo layout

```
echoproxy/
├── api/event.proto              # Wire schema (single source of truth)
├── pkg/event/                   # Generated Go bindings + producer wrapper
├── pkg/redact/                  # Shared scrubber (applied at SDK + proxy + ingest)
├── pkg/ratelimit/               # Token-bucket limiter (Redis-backed)
├── proxy-gateway/               # Reverse proxy (X-Echo-Target dynamic routing)
├── ingest-api/                  # HTTP + gRPC ingestion for SDKs
├── log-consumer/                # Kafka → ClickHouse batch insert
├── auth-api/                    # Users / projects / API keys
├── stats-api/                   # Read-side analytics for the dashboard
├── cleanup/                     # Nightly retention sweep binary
├── sdk-reference-go/            # Reference SDK
├── sdk-python/                  # Python SDK (echoproxy package)
├── sdk-ts/                      # @echoproxy/sdk
├── sdk-laravel/                 # Echoproxy\Sdk\ namespace
├── dashboard/                   # Next.js 14 App Router UI
├── bench/                       # Cross-service end-to-end benches
├── migrations/{postgres,clickhouse}/
├── deploy/                      # Dockerfile.go, prometheus.yml, grafana/, k8s/
├── docs/                        # sdk-spec, architecture, runbook, retention
├── .claude/skills/              # Project-scoped skills
├── docker-compose.yml
├── go.work
└── Makefile
```

## Development

```bash
# Only the infra (Kafka, ClickHouse, Postgres, Redis) — useful when running services from your IDE
make up-infra

# Build / test / vet every Go module
make build
make test
make vet

# Regenerate Go proto bindings (after editing api/event.proto)
make proto-gen

# Dashboard locally (hot reload)
cd dashboard && pnpm install && pnpm dev

# Stress test the proxy (ramps 1k → 50k RPS)
make bench-stress

# Force a retention sweep right now
make cleanup
```

Each Go module is a separate `go.mod` joined by `go.work`. Add a new
service: create `mynew-service/go.mod`, add it to `go.work`, copy the
docker-compose service block + adjust `SERVICE` and `BIN` args.

## Production deployment

Two supported paths, both deploy the same images and use the same wire
schema — pick what your team already runs:

### Kubernetes (Helm)

Pull the chart + images straight from GitHub Container Registry:

```bash
helm install echoproxy oci://ghcr.io/songhieu/charts/echoproxy \
  --version 0.2.0 \
  --namespace echoproxy --create-namespace \
  -f my-values.yaml
```

Or from the source tree (the chart references the same `ghcr.io/songhieu/echoproxy-*` images):

```bash
helm install echoproxy ./deploy/helm/echoproxy \
  --namespace echoproxy --create-namespace \
  -f my-values.yaml
```

Chart deploys all 6 Go services + dashboard + cleanup CronJob with
optional Ingress (nginx/cert-manager friendly) and Prometheus
ServiceMonitor. Stateful infra (Kafka / ClickHouse / Postgres / Redis)
is bring-your-own (managed services or sibling operators).

Full walkthrough: [`docs/deployment/kubernetes.md`](docs/deployment/kubernetes.md).

### Docker Compose (single VM)

Repo root `docker-compose.yml` is production-runnable on a single
beefy VM (8 vCPU / 16 GB RAM) to ~100M req/day. Add Caddy for TLS,
cron the backup script, you're done.

Full walkthrough: [`docs/deployment/docker-compose.md`](docs/deployment/docker-compose.md).

### Container images

Every release pushes 7 multi-arch images (linux/amd64 + linux/arm64) to
GitHub Container Registry:

```
ghcr.io/songhieu/echoproxy-proxy-gateway:<tag>
ghcr.io/songhieu/echoproxy-ingest-api:<tag>
ghcr.io/songhieu/echoproxy-log-consumer:<tag>
ghcr.io/songhieu/echoproxy-auth-api:<tag>
ghcr.io/songhieu/echoproxy-stats-api:<tag>
ghcr.io/songhieu/echoproxy-cleanup:<tag>
ghcr.io/songhieu/echoproxy-dashboard:<tag>
```

Tag conventions (matched by `docker/metadata-action`):

| Trigger | Tags produced |
|---------|---------------|
| Push to `main` | `latest`, `main`, `sha-<short>` |
| Push tag `v1.2.3` | `1.2.3`, `1.2`, `latest`, `sha-<short>` |

Build locally with the same `Dockerfile.go` used in CI:

```bash
make build-images            # local docker build
make buildx-push             # multi-arch buildx push (matches CI)
TAG=v0.2.0 make buildx-push  # stamp a release tag
```

### Platform notes

- **Kafka**: docker-compose ships a single-broker KRaft setup. Production
  wants ≥3 brokers with `replication.factor=3`. Topic-level
  `retention.ms=86400000` (24h) is enough — events are durable in ClickHouse.
- **ClickHouse**: single-node `MergeTree` is fine to ~500M req/day on
  decent hardware. Above that, switch to `ReplicatedMergeTree` +
  `Distributed` tables across N shards. Per-project retention works
  unchanged on a cluster.
- **Postgres**: `auth_api` does ~1 query per authenticated request
  (the user-existence check). Behind PgBouncer is fine; primary + replica
  for HA.
- **JWT_SECRET**: rotate from the dev default. The auth middleware
  verifies the user still exists, so rotating + redeploying forces a
  clean re-login for everyone (their cookies become invalid).

## Testing

```bash
make test                                # all Go modules
(cd sdk-ts && pnpm test)                 # TS SDK
(cd sdk-python && pytest tests/)         # Python SDK
(cd sdk-laravel && ./vendor/bin/phpunit) # Laravel SDK
bash bench/run-sdk-smoke.sh              # all 4 SDKs end-to-end
make bench-stress                        # capacity test against running proxy
```

## Contributing

Pull requests welcome. Before submitting:

- `make vet test` must pass.
- For changes touching `api/event.proto`: run `make proto-gen` and commit
  the regenerated bindings. New fields must use new tag numbers
  (back-compat — see `.claude/skills/echoproxy-event-schema/`).
- For changes touching `proxy-gateway/internal/usecase/proxy_request.go`
  (hot path): run `make bench-stress` and report before/after numbers
  in the PR description.
- New SDK methods must mirror the contract in `docs/sdk-spec.md`.

## License

[MIT](LICENSE) © 2026 songhieu
