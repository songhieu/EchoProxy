# echoproxy-sdk

Python SDK for the [EchoProxy](../README.md) HTTP observability platform.

## Install

```bash
pip install echoproxy-sdk
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

## Capture outbound HTTP

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
