<?php

declare(strict_types=1);

namespace Echoproxy\Sdk\Tests;

use PHPUnit\Framework\TestCase;
use Echoproxy\Sdk\Redact\Redactor;

final class RedactorTest extends TestCase
{
    public function test_headers_default_denylist(): void
    {
        $r = new Redactor();
        $out = $r->headers([
            'Authorization' => 'Bearer x',
            'Cookie' => 'sid=1',
            'User-Agent' => 'ua',
        ]);
        $this->assertSame(Redactor::MASK, $out['Authorization']);
        $this->assertSame(Redactor::MASK, $out['Cookie']);
        $this->assertSame('ua', $out['User-Agent']);
    }

    public function test_json_field_masking(): void
    {
        $r = new Redactor();
        $raw = json_encode([
            'user' => 'a',
            'password' => 'p',
            'nested' => ['api_key' => 'k'],
        ]);
        $out = json_decode($r->body($raw, 'application/json'), true);
        $this->assertSame(Redactor::MASK, $out['password']);
        $this->assertSame(Redactor::MASK, $out['nested']['api_key']);
        $this->assertSame('a', $out['user']);
    }

    public function test_jwt_pattern(): void
    {
        $body = 'token=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9'
              . '.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4ifQ'
              . '.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c ok';
        $out = (new Redactor())->body($body, 'text/plain');
        $this->assertStringNotContainsString('eyJhbGciOi', $out);
        $this->assertStringContainsString(Redactor::MASK, $out);
    }

    public function test_credit_card_luhn_only(): void
    {
        $r = new Redactor();
        $valid = 'card 4111-1111-1111-1111 ok';
        $invalid = 'id 1234-5678-9012-3456 ok';
        $this->assertStringNotContainsString('4111-1111-1111-1111', $r->body($valid, 'text/plain'));
        $this->assertStringContainsString('1234-5678-9012-3456', $r->body($invalid, 'text/plain'));
    }
}
