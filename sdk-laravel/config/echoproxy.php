<?php

return [
    'api_key'        => env('ECHOPROXY_API_KEY'),
    'endpoint'       => env('ECHOPROXY_ENDPOINT', 'http://localhost:8081'),
    'proxy_url'      => env('ECHOPROXY_PROXY_URL', 'http://localhost:8080'),
    'buffer_size'    => (int) env('ECHOPROXY_BUFFER_SIZE', 10_000),
    'flush_interval' => (float) env('ECHOPROXY_FLUSH_INTERVAL', 2.0), // seconds
    'batch_size'     => (int) env('ECHOPROXY_BATCH_SIZE', 500),
    'max_body_bytes' => (int) env('ECHOPROXY_MAX_BODY_BYTES', 65_536),
    'sample_rate'    => (float) env('ECHOPROXY_SAMPLE_RATE', 1.0),
    'redact'         => [
        // additional header names (case-insensitive) on top of the package defaults
        'extra_headers' => [],
        // additional JSON field names on top of the defaults
        'extra_json_fields' => [],
        // skip the package defaults entirely (advanced)
        'disable_defaults' => false,
    ],
];
