# echoproxy

Python SDK for the [EchoProxy](../README.md) HTTP observability platform.

## Install

```bash
pip install echoproxy
export ECHOPROXY_API_KEY=sk_live_xxx
export ECHOPROXY_ENDPOINT=http://localhost:8081
```

## Capture inbound requests

### FastAPI / Starlette / Django ASGI

```python
import echoproxy
from echoproxy.asgi import CaptureMiddleware

client = echoproxy.Client()
app.add_middleware(CaptureMiddleware, client=client)
```

### Flask / Django WSGI

```python
import echoproxy
from echoproxy.wsgi import CaptureMiddleware

client = echoproxy.Client()
app.wsgi_app = CaptureMiddleware(app.wsgi_app, client)
```

## Capture outbound HTTP — pick a mode

There are **two** ways to capture outbound calls. Pick one per project.

|                       | Proxy mode (`echoproxy.proxy`) | Capture mode (`echoproxy.httpx_hook`) |
|-----------------------|--------------------------------|---------------------------------------|
| Who calls the upstream | **proxy-gateway** (Go)         | **your app** (httpx)                  |
| Where the event is emitted | proxy-gateway → Kafka      | SDK → ingest-api → Kafka              |
| Latency added         | ~1 hop to proxy-gateway        | ~µs (event buffered + async flush)    |
| `upstream_latency_ms` | measured server-side via `httptrace` (authoritative) | measured client-side from `time.perf_counter()` |
| `upstream_ttfb_ms`    | yes, real TTFB                 | 0 (httpx doesn't expose TTFB)         |
| Body capture cap      | enforced in proxy-gateway      | enforced in SDK                       |
| Code change           | swap `requests` import         | add 1 line to `httpx.Client(...)`     |
| Dashboard `source`    | `proxy-gateway`                | `sdk-python`                          |
| Dashboard mode badge  | **proxy**                      | **capture**                           |

### When to use which

- **Proxy mode** — the default. Use whenever your app can reach `proxy-gateway:8080`. You get the most accurate timing (TTFB, upstream RoundTrip, proxy overhead) and don't have to manage flush/buffer state in your process.
- **Capture mode** — use when (a) your runtime can't reach the proxy (Lambda with VPC restrictions, Fly.io firewall, edge runtime), (b) you already use httpx and want zero hop, or (c) you need to capture calls from libraries that ignore `http_proxy` env vars.

### Proxy mode — drop-in for `requests`

```python
import echoproxy.proxy as sid   # mirrors requests.* API

r = sid.get("https://api.openai.com/v1/models")
r = sid.post("https://api.stripe.com/v1/charges", data={...})
```

Or get an explicit Session:

```python
import echoproxy.proxy as sid
session = sid.session(api_key="sk_live_xxx", proxy_url="http://proxy-gateway:8080")
session.get("https://api.example.com/users")
```

### Capture mode — httpx event hooks

```python
import httpx, echoproxy
from echoproxy import httpx_hook

client = echoproxy.Client()
http = httpx.Client(event_hooks=httpx_hook.hooks(client))
```

## Redaction

The SDK ships with the same scrub list as the Go reference (`pkg/redact`):
- Headers: `Authorization`, `Cookie`, `X-Api-Key`, `X-Auth-Token`, `X-CSRF-Token`, …
- JSON fields: `password`, `token`, `secret`, `api_key`, `credit_card`, …
- Patterns: JWT, Bearer, AWS keys, Stripe, GitHub, Google API, Slack, Luhn-validated cards.

Extend:

```python
from echoproxy import Client, Config, Redactor

client = Client(Config(
    redactor=Redactor(
        extra_headers=("X-Customer-Email",),
        extra_json_fields=("account_number",),
    ),
))
```

`ingest-api` re-applies the same rules server-side, so misconfiguration here is not a wire-leak risk.
