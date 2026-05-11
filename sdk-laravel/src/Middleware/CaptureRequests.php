<?php

declare(strict_types=1);

namespace Echoproxy\Sdk\Middleware;

use Closure;
use Illuminate\Http\Request;
use Symfony\Component\HttpFoundation\Response;
use Echoproxy\Sdk\Client;

/**
 * Server middleware: captures every inbound request + the response the app
 * returned. Works with Laravel and Lumen via $kernel->pushMiddleware(...).
 */
final class CaptureRequests
{
    public function __construct(private readonly Client $client) {}

    public function handle(Request $request, Closure $next): Response
    {
        if (!$this->client->routeAllowed($request->getPathInfo())) {
            return $next($request);
        }
        $start = microtime(true);
        $response = $next($request);
        $latencyMs = (int) ((microtime(true) - $start) * 1000);

        $this->client->capture([
            'direction'  => Client::DIRECTION_INBOUND,
            'method'     => $request->getMethod(),
            'scheme'     => $request->getScheme(),
            'host'       => $request->getHost(),
            'path'       => $request->getPathInfo(),
            'query'      => $request->getQueryString() ?? '',
            'status'     => $response->getStatusCode(),
            'latency_ms' => $latencyMs,
            'req_size'   => strlen($request->getContent()),
            'res_size'   => strlen($response->getContent()),
            'req_headers' => self::flatten($request->headers->all()),
            'res_headers' => self::flatten($response->headers->all()),
            'req_body'   => $request->getContent(),
            'res_body'   => $response->getContent(),
            'client_ip'  => $request->ip() ?? '',
            'user_agent' => (string) $request->userAgent(),
            'trace_id'   => (string) $request->header('traceparent', ''),
            'attributes' => [
                'route' => optional($request->route())->getName() ?? '',
            ],
        ]);

        return $response;
    }

    public function terminate(Request $request, Response $response): void
    {
        $this->client->flush();
    }

    /**
     * @param array<string, list<string|null>> $h
     * @return array<string, string>
     */
    private static function flatten(array $h): array
    {
        $out = [];
        foreach ($h as $name => $vs) {
            $out[$name] = implode(', ', array_filter($vs ?? [], 'is_string'));
        }
        return $out;
    }
}
