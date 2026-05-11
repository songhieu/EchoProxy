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
