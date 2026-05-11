<?php

declare(strict_types=1);

namespace Echoproxy\Sdk;

use GuzzleHttp\Client as Guzzle;
use GuzzleHttp\Exception\GuzzleException;
use Echoproxy\Sdk\Redact\Redactor;

/**
 * Captures HTTP events and ships them to ingest-api in batches.
 *
 * The buffer is in-memory only; in PHP-FPM the SDK flushes synchronously at
 * request shutdown via Client::flush(), so by default events ship inline. For
 * Laravel queue or octane setups, callers can wire a queued flush.
 */
final class Client
{
    public const VERSION = '0.1.0';
    public const SOURCE  = 'sdk-laravel';
    public const DIRECTION_INBOUND = 'inbound';
    public const DIRECTION_OUTBOUND = 'outbound';

    /** @var array<int, array<string, mixed>> */
    private array $buffer = [];

    private Redactor $redactor;
    private Guzzle $http;

    /** @var string[] regex patterns; empty = allow all */
    private array $captureRoutes;
    /** @var string[] regex patterns; matched routes are skipped */
    private array $ignoreRoutes;

    /**
     * @param string[]|null $captureRoutes
     * @param string[]|null $ignoreRoutes
     */
    public function __construct(
        private readonly string $apiKey,
        private readonly string $endpoint,
        private readonly int $bufferSize = 10_000,
        private readonly int $batchSize = 500,
        private readonly int $maxBodyBytes = 65_536,
        private readonly float $sampleRate = 1.0,
        ?Redactor $redactor = null,
        ?Guzzle $http = null,
        ?array $captureRoutes = null,
        ?array $ignoreRoutes = null,
    ) {
        $this->redactor = $redactor ?? new Redactor();
        $this->http     = $http ?? new Guzzle(['timeout' => 5]);
        $this->captureRoutes = $captureRoutes ?? self::envRoutes('ECHOPROXY_CAPTURE_ROUTES');
        $this->ignoreRoutes  = $ignoreRoutes ?? self::envRoutes('ECHOPROXY_IGNORE_ROUTES');
    }

    /** @return string[] */
    private static function envRoutes(string $name): array
    {
        $raw = (string) (getenv($name) ?: '');
        return array_values(array_filter(array_map('trim', explode(',', $raw))));
    }

    public function routeAllowed(string $path): bool
    {
        foreach ($this->ignoreRoutes as $pat) {
            if (@preg_match("#$pat#", $path)) {
                return false;
            }
        }
        if ($this->captureRoutes === []) {
            return true;
        }
        foreach ($this->captureRoutes as $pat) {
            if (@preg_match("#$pat#", $path)) {
                return true;
            }
        }
        return false;
    }

    /**
     * @param array<string, mixed> $event
     */
    public function capture(array $event): void
    {
        if ($this->apiKey === '') {
            return;
        }
        if ($this->sampleRate < 1.0 && mt_rand() / mt_getrandmax() > $this->sampleRate) {
            return;
        }
        if (count($this->buffer) >= $this->bufferSize) {
            array_shift($this->buffer); // drop oldest
        }
        $this->buffer[] = $this->normalize($event);
        if (count($this->buffer) >= $this->batchSize) {
            $this->flush();
        }
    }

    public function flush(): void
    {
        if ($this->buffer === [] || $this->apiKey === '') {
            return;
        }
        $events = $this->buffer;
        $this->buffer = [];
        try {
            $this->http->post($this->endpoint . '/v1/events:batch', [
                'headers' => [
                    'Content-Type' => 'application/json',
                    'X-Echo-Key'    => $this->apiKey,
                ],
                'json' => ['events' => $events],
            ]);
        } catch (GuzzleException) {
            // Fail open: never propagate ingest failures into host app.
        }
    }

    /**
     * @param array<string, mixed> $event
     * @return array<string, mixed>
     */
    private function normalize(array $event): array
    {
        $event['source']      ??= self::SOURCE;
        $event['sdk_version'] ??= self::VERSION;
        $event['event_id']    ??= bin2hex(random_bytes(16));
        $event['timestamp_ns'] ??= (int) (microtime(true) * 1_000_000_000);

        $reqHeaders = $event['req_headers'] ?? [];
        $resHeaders = $event['res_headers'] ?? [];
        $event['req_headers'] = $this->redactor->headers(is_array($reqHeaders) ? $reqHeaders : []);
        $event['res_headers'] = $this->redactor->headers(is_array($resHeaders) ? $resHeaders : []);

        $reqCT = $event['req_headers']['Content-Type'] ?? '';
        $resCT = $event['res_headers']['Content-Type'] ?? '';
        $event['req_body'] = $this->capAndRedact($event['req_body'] ?? '', $reqCT);
        $event['res_body'] = $this->capAndRedact($event['res_body'] ?? '', $resCT);

        return $event;
    }

    private function capAndRedact(mixed $body, string $contentType): string
    {
        $b = is_string($body) ? $body : (string) ($body ?? '');
        if ($b === '') {
            return '';
        }
        if (strlen($b) > $this->maxBodyBytes) {
            $b = substr($b, 0, $this->maxBodyBytes);
        }
        return base64_encode($this->redactor->body($b, $contentType));
    }

    public function dropped(): int
    {
        return 0; // PHP-FPM lifecycle: per-request buffer; cross-request drops are not tracked
    }
}
