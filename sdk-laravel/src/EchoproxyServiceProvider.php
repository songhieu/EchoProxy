<?php

declare(strict_types=1);

namespace Echoproxy\Sdk;

use Illuminate\Support\ServiceProvider;
use Echoproxy\Sdk\Redact\Redactor;

final class EchoproxyServiceProvider extends ServiceProvider
{
    public function register(): void
    {
        $this->mergeConfigFrom(__DIR__ . '/../config/echoproxy.php', 'echoproxy');

        $this->app->singleton(Client::class, function ($app): Client {
            $cfg = $app['config']['echoproxy'];
            $redactor = new Redactor(
                extraHeaders: (array) ($cfg['redact']['extra_headers'] ?? []),
                extraJsonFields: (array) ($cfg['redact']['extra_json_fields'] ?? []),
                disableDefaults: (bool) ($cfg['redact']['disable_defaults'] ?? false),
            );
            return new Client(
                apiKey: (string) ($cfg['api_key'] ?? ''),
                endpoint: (string) ($cfg['endpoint'] ?? 'http://localhost:8081'),
                bufferSize: (int) ($cfg['buffer_size'] ?? 10_000),
                batchSize: (int) ($cfg['batch_size'] ?? 500),
                maxBodyBytes: (int) ($cfg['max_body_bytes'] ?? 65_536),
                sampleRate: (float) ($cfg['sample_rate'] ?? 1.0),
                redactor: $redactor,
            );
        });

        $this->app->singleton(ProxyClient::class, function ($app): ProxyClient {
            $cfg = $app['config']['echoproxy'];
            return new ProxyClient(
                apiKey: (string) ($cfg['api_key'] ?? ''),
                proxyUrl: (string) ($cfg['proxy_url'] ?? 'http://localhost:8080'),
            );
        });
    }

    public function boot(): void
    {
        $this->publishes([
            __DIR__ . '/../config/echoproxy.php' => config_path('echoproxy.php'),
        ], 'echoproxy-config');
    }
}
