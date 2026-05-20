<?php

declare(strict_types=1);

namespace Echoproxy\Sdk\Http;

use Closure;
use GuzzleHttp\TransferStats;
use Psr\Http\Message\RequestInterface;
use Psr\Http\Message\ResponseInterface;
use Echoproxy\Sdk\Client; // phpcs:ignore

/**
 * Guzzle handler-stack middleware: captures every outbound HTTP call. Add via:
 *
 *   $stack = HandlerStack::create();
 *   $stack->push(GuzzleMiddleware::create($client));
 *   new \GuzzleHttp\Client(['handler' => $stack]);
 */
final class GuzzleMiddleware
{
    public static function create(Client $client): Closure
    {
        return static function (callable $handler) use ($client): callable {
            return static function (RequestInterface $req, array $opts) use ($handler, $client) {
                $start = microtime(true);
                $reqBody = (string) $req->getBody();

                // Capture transport-level timing via on_stats. transferTime is
                // the network RoundTrip seen by the HTTP handler; latency_ms
                // below is wall time including PHP overhead. We chain any
                // user-supplied on_stats so we don't clobber it.
                $stats = null;
                $userOnStats = $opts['on_stats'] ?? null;
                $opts['on_stats'] = static function (TransferStats $s) use (&$stats, $userOnStats): void {
                    $stats = $s;
                    if (is_callable($userOnStats)) {
                        ($userOnStats)($s);
                    }
                };

                return $handler($req, $opts)->then(static function (ResponseInterface $res) use ($req, $reqBody, $start, $client, &$stats) {
                    $latency = (int) ((microtime(true) - $start) * 1000);

                    // Fall back to wall time if the handler didn't provide
                    // transfer stats (non-cURL handlers should still call
                    // on_stats, but be defensive).
                    $upstreamLatency = $latency;
                    $upstreamTtfb    = 0;
                    if ($stats instanceof TransferStats) {
                        $upstreamLatency = (int) ($stats->getTransferTime() * 1000);
                        $handlerStats = $stats->getHandlerStats();
                        if (is_array($handlerStats) && isset($handlerStats['starttransfer_time'])) {
                            $upstreamTtfb = (int) (((float) $handlerStats['starttransfer_time']) * 1000);
                        }
                    }

                    $resBody = (string) $res->getBody();
                    $client->capture([
                        'direction'           => Client::DIRECTION_OUTBOUND,
                        'method'              => $req->getMethod(),
                        'scheme'              => $req->getUri()->getScheme(),
                        'host'                => $req->getUri()->getHost(),
                        'path'                => $req->getUri()->getPath(),
                        'query'               => $req->getUri()->getQuery(),
                        'status'              => $res->getStatusCode(),
                        'latency_ms'          => $latency,
                        'upstream_latency_ms' => $upstreamLatency,
                        'upstream_ttfb_ms'    => $upstreamTtfb,
                        'req_size'            => strlen($reqBody),
                        'res_size'            => strlen($resBody),
                        'req_headers'         => self::flatten($req->getHeaders()),
                        'res_headers'         => self::flatten($res->getHeaders()),
                        'req_body'            => $reqBody,
                        'res_body'            => $resBody,
                    ]);
                    // Reset response body stream so caller can still read it.
                    $res->getBody()->rewind();
                    return $res;
                });
            };
        };
    }

    /**
     * @param array<string, string[]> $headers
     * @return array<string, string>
     */
    private static function flatten(array $headers): array
    {
        $out = [];
        foreach ($headers as $name => $values) {
            $out[$name] = implode(', ', $values);
        }
        return $out;
    }
}
