# sdk-reference-go

Reference Go SDK for the [EchoProxy](../README.md) HTTP observability platform.
This is the spec implementation every other-language SDK mirrors — see
[`docs/sdk-spec.md`](../docs/sdk-spec.md).

## Install

```bash
go get github.com/songhieu/EchoProxy/sdk-reference-go
export ECHOPROXY_API_KEY=sk_live_xxx
export ECHOPROXY_ENDPOINT=http://localhost:8081
```

## Capture inbound requests

```go
import sdk "github.com/songhieu/EchoProxy/sdk-reference-go"

client, _ := sdk.New(sdk.Config{
    APIKey:       os.Getenv("ECHOPROXY_API_KEY"),
    EndpointHTTP: os.Getenv("ECHOPROXY_ENDPOINT"),
})
defer client.Close(context.Background())

http.Handle("/api/", client.Middleware(yourHandler))
```

## Capture outbound HTTP — pick a mode

There are **two** ways to capture outbound calls. Pick one per project.

|                       | Proxy mode (`sdk-reference-go/proxy`) | Capture mode (`Client.Capture`)        |
|-----------------------|---------------------------------------|----------------------------------------|
| Who calls the upstream | **proxy-gateway** (Go)               | **your app** (`net/http`)              |
| Where the event is emitted | proxy-gateway → Kafka            | SDK → ingest-api → Kafka               |
| Latency added         | ~1 hop to proxy-gateway               | ~µs (channel send, async flush)        |
| `upstream_latency_ms` | measured server-side via `httptrace` (authoritative) | measured client-side via `httptrace.ClientTrace` |
| `upstream_ttfb_ms`    | yes, real TTFB                        | yes, real TTFB (the SDK wires `httptrace`) |
| Body capture cap      | enforced in proxy-gateway             | enforced in SDK                        |
| Code change           | swap `http.*` for `proxy.*`           | wrap `http.RoundTripper` (1 line)      |
| Dashboard `source`    | `proxy-gateway`                       | `sdk-go`                               |
| Dashboard mode badge  | **proxy**                             | **capture**                            |

### When to use which

- **Proxy mode** — the default. Use whenever your service can reach `proxy-gateway:8080`. Zero code change beyond import swap, accurate timing, no buffer/flush state in your process.
- **Capture mode** — use when (a) your runtime can't reach the proxy, (b) you want fine-grained sampling/redaction per call-site, or (c) you're instrumenting a library you don't own (wrap its `http.RoundTripper`).

### Proxy mode — drop-in for `net/http`

```go
import sid "github.com/songhieu/EchoProxy/sdk-reference-go/proxy"

res, err := sid.Get("https://api.openai.com/v1/models")
res, err := sid.Post("https://api.stripe.com/v1/charges", "application/json", body)
```

Or get a real `*http.Client` (handy for libraries that accept one):

```go
httpClient := sid.Client() // *http.Client that rewrites every request through proxy-gateway
res, _ := httpClient.Get("https://api.example.com/users")
```

Config is env-driven (`ECHOPROXY_API_KEY`, `ECHOPROXY_PROXY_URL`); override programmatically with `sid.Configure(apiKey, proxyURL)`.

### Capture mode — `capture.NewTransport`

The SDK ships a packaged `http.RoundTripper` that measures upstream latency + TTFB via `httptrace` and emits an outbound event:

```go
import (
    "context"
    "net/http"
    "os"

    sdk "github.com/songhieu/EchoProxy/sdk-reference-go"
    "github.com/songhieu/EchoProxy/sdk-reference-go/capture"
)

client, _ := sdk.New(sdk.Config{
    APIKey:       os.Getenv("ECHOPROXY_API_KEY"),
    EndpointHTTP: os.Getenv("ECHOPROXY_ENDPOINT"),
})
defer client.Close(context.Background())

httpClient := &http.Client{
    Transport: capture.NewTransport(nil, client),  // nil → http.DefaultTransport
}
res, err := httpClient.Get("https://api.example.com/users")
```

Or wrap a custom base transport (preserves your timeouts, dialer, etc.):

```go
base := &http.Transport{ /* your tuned defaults */ }
httpClient := &http.Client{Transport: capture.NewTransport(base, client)}
```

For libraries that take an `http.Client`, hand them `httpClient`. To instrument the global default:

```go
http.DefaultClient.Transport = capture.NewTransport(nil, client)
```

Every call lands in the same dashboard as proxy-mode events, tagged `source = sdk-go`. `upstream_latency_ms` and `upstream_ttfb_ms` are populated from `httptrace`, so the latency breakdown matches proxy mode.

If you need full control over what to capture (custom fields, sampling per call-site, instrumenting a library that doesn't use `http.RoundTripper`), fall back to `Client.CaptureWithDirection(sdk.DirectionOutbound, sdk.CaptureInput{...})` directly.

## Redaction

Default scrub list mirrors `pkg/redact`:
- Headers: `Authorization`, `Cookie`, `X-Api-Key`, `X-Auth-Token`, `X-CSRF-Token`, …
- JSON fields: `password`, `token`, `secret`, `api_key`, `credit_card`, …
- Patterns: JWT, Bearer, AWS keys, Stripe, GitHub, Google API, Slack, Luhn-validated cards.

Extend:

```go
sdk.New(sdk.Config{
    RedactHeaders:    []string{"X-Customer-Email"},
    RedactJSONFields: []string{"account_number"},
})
```

`ingest-api` re-applies the same rules server-side, so misconfiguration here is not a wire-leak risk.
