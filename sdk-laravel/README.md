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

## Capture outbound HTTP

```php
use GuzzleHttp\Client as Guzzle;
use GuzzleHttp\HandlerStack;
use Echoproxy\Sdk\Client;
use Echoproxy\Sdk\Http\GuzzleMiddleware;

$stack = HandlerStack::create();
$stack->push(GuzzleMiddleware::create(app(Client::class)));
$guzzle = new Guzzle(['handler' => $stack]);
```

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
