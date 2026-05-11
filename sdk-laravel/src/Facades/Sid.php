<?php

declare(strict_types=1);

namespace Echoproxy\Sdk\Facades;

use Illuminate\Support\Facades\Facade;
use Echoproxy\Sdk\ProxyClient;

/**
 * Drop-in replacement for the Http facade. Same method shape, traffic
 * automatically routed through the EchoProxy proxy:
 *
 *   Sid::get('https://api.example.com/v1/users');
 *   Sid::post('https://api.example.com/v1/users', ['name' => 'Alice']);
 *
 * @method static \Illuminate\Http\Client\Response get(string $url, array $query = [])
 * @method static \Illuminate\Http\Client\Response head(string $url, array $query = [])
 * @method static \Illuminate\Http\Client\Response post(string $url, array $data = [])
 * @method static \Illuminate\Http\Client\Response put(string $url, array $data = [])
 * @method static \Illuminate\Http\Client\Response patch(string $url, array $data = [])
 * @method static \Illuminate\Http\Client\Response delete(string $url, array $data = [])
 * @method static \Illuminate\Http\Client\Response send(string $method, string $url, array $options = [])
 * @method static \Illuminate\Http\Client\PendingRequest pending()
 */
final class Sid extends Facade
{
    protected static function getFacadeAccessor(): string
    {
        return ProxyClient::class;
    }
}
