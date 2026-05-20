<?php
/**
 * Smoke example for the Laravel SDK *capture mode*.
 *
 * Wires GuzzleMiddleware onto a Guzzle handler stack so the app makes the
 * upstream call itself and the SDK ships the event to ingest-api with
 * source = sdk-laravel.
 *
 * The full GuzzleMiddleware (src/Http/GuzzleMiddleware.php) accepts the
 * Echoproxy\Sdk\Client. To keep the smoke standalone we instantiate Client
 * directly with env-derived config; in a real Laravel app the
 * EchoproxyServiceProvider binds it as a singleton.
 *
 * Run: composer install && ECHOPROXY_API_KEY=sk_test_demo ECHOPROXY_TAG=abc \
 *      php examples/capture_smoke.php
 */

declare(strict_types=1);

require __DIR__ . '/../vendor/autoload.php';

use Echoproxy\Sdk\Client;
use Echoproxy\Sdk\Http\GuzzleMiddleware;
use GuzzleHttp\Client as Guzzle;
use GuzzleHttp\HandlerStack;

$apiKey   = getenv('ECHOPROXY_API_KEY')   ?: 'sk_test_demo';
$endpoint = getenv('ECHOPROXY_ENDPOINT')  ?: 'http://localhost:8081';
$target   = getenv('ECHOPROXY_EXAMPLE_TARGET') ?: 'http://upstream-mock:9000';
$tag      = getenv('ECHOPROXY_TAG') ?: 'php-default';

$client = new Client($apiKey, $endpoint);

$stack = HandlerStack::create();
$stack->push(GuzzleMiddleware::create($client));
$guzzle = new Guzzle(['handler' => $stack, 'timeout' => 5, 'http_errors' => false]);

$url = "{$target}/api/users/sdkbench-php-capture-{$tag}";
$res = $guzzle->get($url);
$body = (string) $res->getBody();

printf(
    "php sdk (capture): %s -> %d (%d bytes)\n",
    $url,
    $res->getStatusCode(),
    strlen($body),
);

// Drain the SDK buffer before exit so the smoke event is shipped.
$client->flush();
exit($res->getStatusCode() === 200 ? 0 : 1);
