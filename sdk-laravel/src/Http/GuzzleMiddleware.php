<?php

declare(strict_types=1);

namespace Echoproxy\Sdk\Http;

use Closure;
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
                return $handler($req, $opts)->then(static function (ResponseInterface $res) use ($req, $reqBody, $start, $client) {
                    $latency = (int) ((microtime(true) - $start) * 1000);
                    $resBody = (string) $res->getBody();
                    $client->capture([
                        'direction'   => Client::DIRECTION_OUTBOUND,
                        'method'      => $req->getMethod(),
                        'scheme'      => $req->getUri()->getScheme(),
                        'host'        => $req->getUri()->getHost(),
                        'path'        => $req->getUri()->getPath(),
                        'query'       => $req->getUri()->getQuery(),
                        'status'      => $res->getStatusCode(),
                        'latency_ms'  => $latency,
                        'req_size'    => strlen($reqBody),
                        'res_size'    => strlen($resBody),
                        'req_headers' => self::flatten($req->getHeaders()),
                        'res_headers' => self::flatten($res->getHeaders()),
                        'req_body'    => $reqBody,
                        'res_body'    => $resBody,
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
