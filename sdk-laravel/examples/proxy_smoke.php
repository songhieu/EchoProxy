<?php
/**
 * Smoke example for the Laravel SDK proxy mode.
 *
 * The full ProxyClient (src/ProxyClient.php) wraps Laravel's Http facade and
 * needs a booted Laravel app. To keep the smoke standalone we exercise the
 * underlying wire contract directly with Guzzle — which is what ProxyClient
 * emits on the wire. For a real Laravel app, register
 * Echoproxy\Sdk\EchoproxyServiceProvider and resolve ProxyClient from the
 * container.
 *
 * Run: composer install && ECHOPROXY_API_KEY=sk_test_demo ECHOPROXY_TAG=abc \
 *      php examples/proxy_smoke.php
 */

declare(strict_types=1);

require __DIR__ . '/../vendor/autoload.php';

use GuzzleHttp\Client;

$apiKey   = getenv('ECHOPROXY_API_KEY')   ?: 'sk_test_demo';
$proxyUrl = getenv('ECHOPROXY_PROXY_URL') ?: 'http://localhost:8080';
$target   = getenv('ECHOPROXY_EXAMPLE_TARGET') ?: 'http://upstream-mock:9000';
$tag      = getenv('ECHOPROXY_TAG') ?: 'php-default';

$client = new Client([
    'base_uri' => $proxyUrl,
    'timeout'  => 5,
    'http_errors' => false,
    'headers'  => [
        'X-Echo-Key'    => $apiKey,
        'X-Echo-Target' => $target,
    ],
]);

$path = "/api/users/sdkbench-php-{$tag}";
$res  = $client->get($path);
$body = (string) $res->getBody();

printf(
    "php sdk: %s%s -> %d (%d bytes)\n",
    $proxyUrl,
    $path,
    $res->getStatusCode(),
    strlen($body),
);

exit($res->getStatusCode() === 200 ? 0 : 1);
