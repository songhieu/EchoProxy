# echoproxy/sdk-laravel

Laravel/PHP SDK for the [EchoProxy](../README.md) HTTP observability platform.

## Install

```bash
composer require echoproxy/sdk-laravel
```

```env
ECHOPROXY_API_KEY=sk_live_xxx
ECHOPROXY_ENDPOINT=http://localhost:8081
```

## Capture inbound requests

`bootstrap/app.php` (Laravel 11):
```php
->withMiddleware(function ($middleware) {
    $middleware->append(\Echoproxy\Sdk\Middleware\CaptureRequests::class);
})
```

`Kernel::$middleware` (Laravel 10):
```php
\Echoproxy\Sdk\Middleware\CaptureRequests::class,
```

## Capture outbound HTTP — pick a mode

There are **two** ways to capture outbound calls. Pick one per project.

|                       | Proxy mode (`ProxyClient`) | Capture mode (`GuzzleMiddleware`) |
|-----------------------|----------------------------|-----------------------------------|
| Who calls the upstream | **proxy-gateway** (Go)    | **your app** (Guzzle)             |
| Where the event is emitted | proxy-gateway → Kafka | SDK → ingest-api → Kafka          |
| Latency added         | ~1 hop to proxy-gateway    | ~µs (event buffered + flushed at request shutdown) |
| `upstream_latency_ms` | measured server-side via `httptrace` (authoritative) | measured via Guzzle `on_stats` → `TransferStats::getTransferTime()` |
| `upstream_ttfb_ms`    | yes, real TTFB             | yes when cURL handler is used (`starttransfer_time`); else 0 |
| Body capture cap      | enforced in proxy-gateway  | enforced in SDK                   |
| Code change           | swap `Http::*` for `ProxyClient::*` | wire one Guzzle handler-stack push |
| Dashboard `source`    | `proxy-gateway`            | `sdk-laravel`                     |
| Dashboard mode badge  | **proxy**                  | **capture**                       |

### When to use which

- **Proxy mode** — the default. Use whenever your app can reach `proxy-gateway:8080`. You get the most accurate timing (TTFB, upstream RoundTrip, proxy overhead) and don't have to manage buffer/flush.
- **Capture mode** — use when (a) your runtime can't reach the proxy, (b) you have many existing call-sites using Guzzle directly (lots of vendor SDKs do — Stripe, AWS, etc.) and don't want to refactor, or (c) you need per-call programmatic context (custom redaction, sample rate) that the proxy can't see.

### Proxy mode — drop-in for `Http::*`

```php
use Echoproxy\Sdk\ProxyClient;

$client = app(ProxyClient::class); // bound by EchoproxyServiceProvider

$res = $client->get('https://api.openai.com/v1/models');
$res = $client->post('https://api.stripe.com/v1/charges', ['amount' => 1000]);
```

### Capture mode — Guzzle handler stack

```php
use GuzzleHttp\Client as Guzzle;
use GuzzleHttp\HandlerStack;
use Echoproxy\Sdk\Client;
use Echoproxy\Sdk\Http\GuzzleMiddleware;

$stack = HandlerStack::create();
$stack->push(GuzzleMiddleware::create(app(Client::class)));
$guzzle = new Guzzle(['handler' => $stack]);
```

Works with any library that accepts a Guzzle `HandlerStack`. The middleware uses Guzzle's `on_stats` to measure transport time separately from PHP overhead — chains any caller-supplied `on_stats`, so it composes safely.

## Redaction

The SDK ships with the same defense-in-depth scrub list as the Go reference:
- `Authorization`, `Cookie`, `X-Api-Key`, `X-Auth-Token`, `X-CSRF-Token`, …
- JSON fields: `password`, `token`, `secret`, `api_key`, `credit_card`, …
- Patterns: JWT, Bearer, AWS keys, Stripe, GitHub, Google API, Slack, Luhn-validated cards.

Extend in `config/echoproxy.php`:
```php
'redact' => [
    'extra_headers' => ['X-Customer-Email'],
    'extra_json_fields' => ['account_number'],
],
```

`ingest-api` re-applies the same rules server-side, so misconfiguration here is not a wire-leak risk.
