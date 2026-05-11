<?php

declare(strict_types=1);

namespace Echoproxy\Sdk\Redact;

/**
 * Mirror of pkg/redact (Go) so the Laravel SDK applies the same defense-in-depth
 * scrubbing rules as the proxy and ingest-api.
 */
final class Redactor
{
    public const MASK = '[REDACTED]';

    public const DEFAULT_HEADERS = [
        'authorization',
        'proxy-authorization',
        'cookie',
        'set-cookie',
        'x-api-key',
        'x-auth-token',
        'x-csrf-token',
        'x-xsrf-token',
        'x-session-token',
        'x-access-token',
        'x-echo-key',
        'x-forwarded-authorization',
        'www-authenticate',
    ];

    public const DEFAULT_JSON_FIELDS = [
        'password', 'passwd', 'pwd',
        'secret', 'client_secret',
        'token', 'access_token', 'refresh_token', 'id_token', 'session_token',
        'api_key', 'apikey', 'auth_token', 'authorization',
        'private_key', 'privatekey',
        'credit_card', 'cardnumber', 'card_number', 'cvv', 'cvc',
        'ssn',
    ];

    private const PATTERNS = [
        'jwt' => '/eyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}/',
        'bearer' => '/Bearer\s+[A-Za-z0-9._\-]{20,}/i',
        'aws_access_key' => '/AKIA[0-9A-Z]{16}/',
        'github_token' => '/gh[pousr]_[A-Za-z0-9]{36,}/',
        'stripe_live' => '/sk_live_[A-Za-z0-9]{20,}/',
        'stripe_test' => '/sk_test_[A-Za-z0-9]{20,}/',
        'google_api' => '/AIza[0-9A-Za-z_\-]{35}/',
        'slack_token' => '/xox[baprs]-[A-Za-z0-9\-]{10,}/',
    ];

    /** @var array<string, true> */
    private array $headerSet;

    /** @var array<string, true> */
    private array $jsonSet;

    private bool $maskCreditCards;

    /**
     * @param string[] $extraHeaders
     * @param string[] $extraJsonFields
     */
    public function __construct(
        array $extraHeaders = [],
        array $extraJsonFields = [],
        bool $disableDefaults = false,
    ) {
        $headers = $disableDefaults ? [] : self::DEFAULT_HEADERS;
        $json = $disableDefaults ? [] : self::DEFAULT_JSON_FIELDS;
        $this->headerSet = array_fill_keys(
            array_map('strtolower', array_merge($headers, $extraHeaders)),
            true,
        );
        $this->jsonSet = array_fill_keys(
            array_map('strtolower', array_merge($json, $extraJsonFields)),
            true,
        );
        $this->maskCreditCards = !$disableDefaults;
    }

    /**
     * @param array<string, string> $headers
     * @return array<string, string>
     */
    public function headers(array $headers): array
    {
        $out = [];
        foreach ($headers as $name => $value) {
            $out[$name] = isset($this->headerSet[strtolower($name)]) ? self::MASK : $value;
        }
        return $out;
    }

    public function body(string $body, string $contentType = ''): string
    {
        if ($body === '') {
            return $body;
        }
        if ($this->isJson($contentType, $body)) {
            $decoded = json_decode($body, true);
            if (is_array($decoded)) {
                $this->maskJsonRecursive($decoded);
                $body = json_encode($decoded, JSON_UNESCAPED_SLASHES) ?: $body;
            }
        }
        return $this->maskPatterns($body);
    }

    /** @param array<mixed, mixed> $data */
    private function maskJsonRecursive(array &$data): void
    {
        foreach ($data as $key => &$value) {
            if (is_string($key) && isset($this->jsonSet[strtolower($key)])) {
                $value = self::MASK;
                continue;
            }
            if (is_array($value)) {
                $this->maskJsonRecursive($value);
            }
        }
    }

    private function maskPatterns(string $body): string
    {
        foreach (self::PATTERNS as $re) {
            $body = preg_replace($re, self::MASK, $body) ?? $body;
        }
        if ($this->maskCreditCards) {
            $body = preg_replace_callback(
                '/\b(?:\d[ -]*?){13,16}\b/',
                fn (array $m): string => $this->luhn($m[0]) ? self::MASK : $m[0],
                $body,
            ) ?? $body;
        }
        return $body;
    }

    private function isJson(string $contentType, string $body): bool
    {
        if (stripos($contentType, 'json') !== false) {
            return true;
        }
        $trim = ltrim($body);
        return $trim !== '' && ($trim[0] === '{' || $trim[0] === '[');
    }

    private function luhn(string $digits): bool
    {
        $clean = preg_replace('/[ -]/', '', $digits) ?? '';
        $n = strlen($clean);
        if ($n < 13 || $n > 19) {
            return false;
        }
        $sum = 0;
        $odd = true;
        for ($i = $n - 1; $i >= 0; $i--) {
            if (!ctype_digit($clean[$i])) {
                return false;
            }
            $d = (int) $clean[$i];
            if (!$odd) {
                $d *= 2;
                if ($d > 9) {
                    $d -= 9;
                }
            }
            $sum += $d;
            $odd = !$odd;
        }
        return $sum % 10 === 0;
    }
}
