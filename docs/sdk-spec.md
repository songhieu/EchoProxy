# EchoProxy SDK Specification

This document is the contract every SDK (Go, Laravel/PHP, Python, Node, Java...) must implement. The reference implementation is `sdk-reference-go/`. Other languages are ports of the same behavior.

## Goals

- **Fail-open.** SDK errors must never propagate to the host application.
- **Async.** All I/O (network, disk) happens on a background thread/process.
- **Bounded memory.** Buffers have hard caps; drop-on-overflow is the default.
- **Forward-compatible.** SDKs ignore unknown fields; backend ignores unknown SDK versions.

## Wire protocol

Two transports are supported. SDK MUST implement at least HTTP/JSON.

### HTTP/JSON (required)

```
POST {endpoint_http}/v1/events:batch
Content-Type: application/json
X-Echo-Key: sk_live_xxx

{
  "events": [
    {
      "event_id": "01HK...",
      "timestamp_ns": 1715168700000000000,
      "source": "sdk-laravel",
      "sdk_version": "0.1.0",
      "method": "GET",
      "scheme": "https",
      "host": "api.example.com",
      "path": "/v1/users",
      "query": "page=2",
      "status": 200,
      "latency_ms": 42,
      "req_size": 0,
      "res_size": 1024,
      "req_headers": {"User-Agent": "..."},
      "res_headers": {"Content-Type": "application/json"},
      "req_body": "<base64>",
      "res_body": "<base64>",
      "req_body_truncated": false,
      "res_body_truncated": false,
      "client_ip": "1.2.3.4",
      "user_agent": "...",
      "trace_id": "00-...-...-01",
      "attributes": {"route": "users.show", "user_id": "42"}
    }
  ]
}
```

Server replies with `202 Accepted`:

```json
{ "accepted": 1, "rejected": 0, "reason": "" }
```

Error codes:
- `401` — missing or unknown API key.
- `403` — API key revoked.
- `400` — invalid body shape.
- `503` — backpressure; SDK should retry with exponential backoff.

### gRPC (optional but recommended for Go/Java/.NET)

Service: `echoproxy.v1.EventIngest`
- `Ingest(IngestRequest) returns (IngestResponse)` — unary batch.
- `IngestStream(stream IngestRequest) returns (stream IngestResponse)` — bidirectional streaming for high throughput.

Authenticate via metadata: `x-echo-key: sk_live_xxx`.

## SDK behavior contract

### 1. Capture

Each SDK MUST provide:
- An HTTP **server** middleware that captures inbound requests + their responses.
- An HTTP **client** wrapper that captures outbound calls (Guzzle/PHP, requests/Python, undici/Node, http.Client/Go).

The captured event includes method, host, scheme, path, query, status, latency, request/response headers, request/response bodies (capped), client IP, user-agent, trace ID, and free-form attributes.

### 2. Buffering

- In-memory ring buffer. Default capacity: **10,000 events**.
- Default flush triggers: every **2 seconds** OR every **500 events**, whichever comes first.
- On buffer full → **drop oldest**, increment a per-process counter accessible via `Dropped()` (or equivalent).

### 3. Body capture

- Default cap: **65,536 bytes** per body. Truncated bodies set `req_body_truncated` / `res_body_truncated` to `true`.
- SDK MUST allow disabling body capture independently for request and response.
- SDK MUST NOT decompress bodies; pass-through.

### 4. Header masking

SDKs MUST mask the following headers (replace value with `[REDACTED]`) before sending:
- `Authorization`, `Proxy-Authorization`, `WWW-Authenticate`
- `Cookie`, `Set-Cookie`
- `X-Api-Key`, `X-Auth-Token`, `X-Access-Token`, `X-Session-Token`
- `X-CSRF-Token`, `X-XSRF-Token`
- `X-Echo-Key`, `X-Forwarded-Authorization`

SDKs MUST allow callers to extend this denylist (case-insensitive). Even when an SDK fails to scrub, `ingest-api` re-applies the same rules server-side as a final line of defense.

### 4a. Body scrubbing

SDKs MUST scrub bodies before sending. The reference Go implementation in `pkg/redact` does:

- **JSON-aware masking**: walks the JSON tree and replaces values for any key matching the denylist (`password`, `token`, `secret`, `api_key`, `access_token`, `refresh_token`, `id_token`, `private_key`, `credit_card`, `cvv`, `ssn`, …) with `[REDACTED]`. Case-insensitive, applied at any depth.
- **Regex pattern scrubbing** for non-JSON bodies (and as a second pass on JSON):
  - JWT: `eyJ...payload.signature`
  - Bearer tokens (in any context)
  - AWS access keys: `AKIA[0-9A-Z]{16}`
  - GitHub PATs: `gh[pousr]_...`
  - Stripe live/test keys: `sk_live_...`, `sk_test_...`
  - Google API keys: `AIza...`
  - Slack tokens: `xox[baprs]-...`
  - Credit card numbers (Luhn-validated to suppress false positives)

Other-language SDKs SHOULD use a comparable library or port the same rules.

### 5. Sampling

SDK MUST accept a `sample_rate` in [0.0, 1.0]. Below the rate, events are skipped before buffering.

### 6. Retry

- On transient failure (5xx, network error): exponential backoff with jitter, max 3 retries, then drop and increment counter.
- On `401`/`403`: do NOT retry; surface a one-time warning log.

### 7. Identification

Every event SDKs send MUST set:
- `source`: `sdk-<lang>` (e.g. `sdk-laravel`, `sdk-python`).
- `sdk_version`: semver, matches the SDK package version.

Backend overwrites `project_id` and `api_key_id` from the API key — SDKs cannot fake them.

### 8. Fail-open

SDK errors MUST be caught internally. Capture, flush, and serialization MUST never throw into the host application code path.

## Versioning

- SDK semver follows the rules in `.claude/skills/echoproxy-event-schema/SKILL.md`.
- Major version bumps only when the backend introduces a new `echoproxy.vX` package.
- Minor version bumps when adding new fields to the wire format.
- Patch version bumps for bug fixes that don't change the wire format.

## Reference Go SDK

`sdk-reference-go/` is the canonical implementation. New SDKs should port:
- The `Client` struct + `Capture` method shape.
- The buffering + flush loop (see `flushLoop` in `sdk.go`).
- The `Middleware` HTTP handler wrapper.
- The header masking helper.

Tests in `sdk-reference-go/sdk_test.go` define the contract assertions every SDK should pass against a real `ingest-api`.
