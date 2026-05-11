<?php

declare(strict_types=1);

namespace Echoproxy\Sdk;

use Illuminate\Http\Client\PendingRequest;
use Illuminate\Http\Client\Response;
use Illuminate\Support\Facades\Http;

/**
 * Drop-in replacement for Laravel's Http facade that routes every call
 * through the EchoProxy proxy. Method signatures mirror Http::* so existing
 * call-sites only need the import swapped.
 *
 * Configuration (from config/echoproxy.php or env):
 *   ECHOPROXY_API_KEY     required
 *   ECHOPROXY_PROXY_URL   default http://localhost:8080
 */
final class ProxyClient
{
    public function __construct(
        private readonly string $apiKey,
        private readonly string $proxyUrl,
    ) {}

    /**
     * Build a PendingRequest with the proxy headers and base URL applied.
     * Use this for advanced flows (multipart, retries, etc.).
     */
    public function pending(): PendingRequest
    {
        return Http::withHeaders([
            'X-Echo-Key' => $this->apiKey,
        ]);
    }

    public function get(string $url, array $query = []): Response
    {
        return $this->send('GET', $url, ['query' => $query]);
    }

    public function head(string $url, array $query = []): Response
    {
        return $this->send('HEAD', $url, ['query' => $query]);
    }

    public function post(string $url, array $data = []): Response
    {
        return $this->send('POST', $url, ['json' => $data]);
    }

    public function put(string $url, array $data = []): Response
    {
        return $this->send('PUT', $url, ['json' => $data]);
    }

    public function patch(string $url, array $data = []): Response
    {
        return $this->send('PATCH', $url, ['json' => $data]);
    }

    public function delete(string $url, array $data = []): Response
    {
        return $this->send('DELETE', $url, ['json' => $data]);
    }

    /**
     * Low-level send. method = GET/POST/...; options follow Guzzle conventions.
     */
    public function send(string $method, string $url, array $options = []): Response
    {
        $parts = parse_url($url);
        $scheme = $parts['scheme'] ?? 'https';
        $host = $parts['host'] ?? '';
        $port = isset($parts['port']) ? ':' . $parts['port'] : '';
        $target = $scheme . '://' . $host . $port;
        $path = $parts['path'] ?? '/';
        $query = isset($parts['query']) ? '?' . $parts['query'] : '';
        $proxied = rtrim($this->proxyUrl, '/') . $path . $query;

        return Http::withHeaders([
            'X-Echo-Key' => $this->apiKey,
            'X-Echo-Target' => $target,
        ])->send(strtoupper($method), $proxied, $options);
    }
}
