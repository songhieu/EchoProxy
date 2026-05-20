# Inbound vs Outbound capture

EchoProxy captures HTTP traffic from two sides of a service. Pick one
or both per project — they share the same wire schema, same Kafka topic,
same ClickHouse table. The `direction` column on every event tells them
apart.

```
                       ┌─────────────────────────────────┐
                       │       Your application          │
   client ──────────▶ │   ← inbound capture here        │
   (browser, mobile,   │     (server middleware)         │
    other services)    │                                 │
                       │   outbound capture here →      │ ─────────▶ upstream
                       │     (HTTP-client wrapper        │   (Stripe, OpenAI,
                       │      or proxy URL rewrite)      │    internal API)
                       └─────────────────────────────────┘
```

| | Inbound | Outbound |
|-|---------|----------|
| What you see | requests *clients send to you* + responses *you returned* | requests *you send to upstreams* + responses *they returned* |
| Captured by | server middleware inside your app | HTTP-client wrapper OR proxy URL rewrite |
| Direction tag | `inbound` | `outbound` |
| Typical use | API behavior debugging, latency-from-the-client's-view, "who sent that weird payload" | Vendor integration debugging (Stripe webhooks, OpenAI, payment gateways), SSRF audit, dependency latency |
| Code change required | Add 1 middleware to your HTTP framework | Either none (proxy mode) or swap the HTTP client constructor (SDK mode) |
| Latency overhead | ~ms (buffer body, then forward) | ~ms (proxy hop) OR ~µs (SDK fire-and-forget) |

## When to use which

- **Just shipped a new API and want to see what real clients send?** → inbound middleware. Catches malformed payloads, missing headers, weird user agents.
- **A vendor webhook is randomly failing?** → inbound middleware on your webhook handler. You'll see the exact body the vendor sent + your 500/400 response.
- **Your app calls 5 external APIs and one is slow?** → outbound capture. Latency breakdown shows which dependency is the bottleneck.
- **You want zero-instrumentation outbound debugging?** → outbound proxy mode. Point traffic at proxy-gateway, no code change.
- **You can't reach the proxy from your runtime (Fly.io firewall, on-prem, etc.)?** → outbound SDK mode. SDK ships events directly to ingest-api over HTTP/gRPC.
- **All of the above** → use both. Project owners filter by direction in the dashboard's logs page.

## Outbound: proxy mode vs capture mode

For outbound you pick **how** the SDK captures, not whether to capture:

|                       | **Proxy mode**                       | **Capture mode**                      |
|-----------------------|--------------------------------------|---------------------------------------|
| Who calls the upstream | proxy-gateway (Go) does the RoundTrip | your app does the RoundTrip          |
| Where the event is emitted | proxy-gateway → Kafka            | SDK buffer → ingest-api → Kafka       |
| Event `source`        | `proxy-gateway`                      | `sdk-go` / `sdk-python` / `sdk-laravel` / `sdk-ts` |
| Dashboard mode badge  | **proxy** (network icon)             | **capture** (package icon)            |
| `upstream_latency_ms` | server-side `httptrace` (authoritative) | client-side measurement              |
| `upstream_ttfb_ms`    | always real TTFB                     | depends on the runtime (Go/Python OK, PHP needs cURL handler) |
| Body cap enforced     | proxy-gateway                        | SDK                                   |
| Code change in app    | swap HTTP-client import              | wrap the HTTP client                  |
| Best for              | most projects — accurate, no buffer/flush state in your process | runtimes that can't reach the proxy; per-call programmatic context |
| Avoid when            | your runtime is on a network island  | you want the timing the proxy reports |

### Rule of thumb

1. **Can your runtime reach `proxy-gateway:8080`?** → use **proxy mode**. It's more accurate and you don't have to manage the SDK's in-memory buffer.
2. **Can't reach the proxy** (Lambda + private VPC, edge worker, Fly.io firewall, on-prem service that talks only to its own VPC) → use **capture mode**. Events go directly to ingest-api (`:8081`) over HTTP.
3. **You have many existing call-sites** using Guzzle/httpx/`net/http` and don't want to refactor → use **capture mode** (`GuzzleMiddleware`, `httpx_hook`, `RoundTripper` wrapper). One line per HTTP client.

Both modes land in the same ClickHouse table and feed the same dashboard. Logs and analytics filter by `source` and `direction`. The mode badge on each row makes the distinction visible at a glance.

### Per-SDK mapping

| Language        | Proxy mode               | Capture mode                       |
|-----------------|--------------------------|------------------------------------|
| **Go**          | `sdk-reference-go/proxy` (`sid.Get`, `sid.Client()`) | `sdk-reference-go/capture` (`capture.NewTransport`) |
| **Python**      | `echoproxy.proxy.session()` (drop-in for `requests`) | `echoproxy.httpx_hook.hooks()` (httpx event hooks) |
| **Laravel/PHP** | `Echoproxy\Sdk\ProxyClient` (drop-in for `Http::*`) | `Echoproxy\Sdk\Http\GuzzleMiddleware` (Guzzle handler stack) |
| **TypeScript**  | `@echoproxy/sdk/proxy` (`fetch` wrapper) | `@echoproxy/sdk/capture` (`captureFetch`) |

## Filtering in the dashboard

`/projects/<id>/logs` has a tab bar:

| Tab | What it shows |
|-----|---------------|
| **All traffic** | every event regardless of direction |
| **Inbound** | only `direction = inbound` (your server's perspective on its callers) |
| **Outbound** | only `direction = outbound` (your server's perspective on its dependencies) |

Analytics page applies the same tab. Pie charts of "top hosts" become
meaningful only when scoped — `Inbound + host=api.yourapp.com` =
"endpoints your users hit", `Outbound + host=api.stripe.com` =
"how often we call Stripe".

## Route filtering (skip noisy endpoints)

Both middleware and HTTP-client wrappers honor `CaptureRoutes` /
`IgnoreRoutes` (or env `ECHOPROXY_CAPTURE_ROUTES` /
`ECHOPROXY_IGNORE_ROUTES`, comma-separated Go regexp). Common pattern:

```bash
# Drop the noise. Health checks fire 10x/sec from k8s probes.
export ECHOPROXY_IGNORE_ROUTES='^/healthz$,^/readyz$,^/metrics$'
```

```bash
# Only capture /api/v1/* — your customer-facing surface, not internal calls.
export ECHOPROXY_CAPTURE_ROUTES='^/api/v1/'
```

If both are set: a path must match `CaptureRoutes` AND not match
`IgnoreRoutes` to be captured.

## Body capture

Per-request bodies (req + res) are captured up to `BODY_CAP_BYTES`
(default 64 KB per side). Above the cap, the body is truncated and the
event has `req_body_truncated = 1` / `res_body_truncated = 1` so you
know not to trust the bytes for forensics.

Set per-API-key in the dashboard or via env. Common settings:

| Workload | Body cap |
|----------|----------|
| Webhook receivers (small payloads, debugging gold) | 256 KB |
| GraphQL endpoints (queries small, responses large) | 128 KB |
| File-upload endpoints | 0 (skip body capture entirely) |
| Default | 64 KB |

Truncation is the safest default. Storage cost scales linearly with
this number times your request volume — see [retention.md](retention.md)
for the storage math.
