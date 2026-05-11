// Code snippets shown on the Integrate page. Variables (api key, host, path)
// are interpolated at render time. Keep this file pure data + small helpers
// so adding a new language is one entry.

export type Snippet = {
  id: string;
  label: string;
  language: string; // hint for the badge in the code block
  build: (vars: TemplateVars) => string;
};

export type Section = {
  id: string;
  title: string;
  description: string;
  snippets: Snippet[];
};

export type TemplateVars = {
  apiKey: string; // user pastes their full key; UI shows prefix or placeholder
  proxyURL: string; // e.g. http://localhost:8080
  ingestURL: string; // e.g. http://localhost:8081
  grpcAddr: string; // e.g. localhost:8082
  upstream: string; // example target host (e.g. https://api.example.com)
};

const PROXY_SNIPPETS: Snippet[] = [
  {
    id: "proxy-go",
    label: "Go",
    language: "go",
    build: (v) => `// Drop-in for net/http. Same signatures as http.Get / http.Post / http.Do.
// Reads ECHOPROXY_API_KEY + ECHOPROXY_PROXY_URL from env.

// Install:
//   go get echoproxy/sdk-reference-go/proxy

// .env:
//   ECHOPROXY_API_KEY=${v.apiKey}
//   ECHOPROXY_PROXY_URL=${v.proxyURL}

package main

import (
	"fmt"
	"strings"

	sid "echoproxy/sdk-reference-go/proxy"
)

func main() {
	res, err := sid.Get("${v.upstream}/v1/users")
	if err != nil { panic(err) }
	defer res.Body.Close()
	fmt.Println(res.Status)

	// Or POST JSON, mirroring http.Post:
	body := strings.NewReader(\`{"name":"Alice"}\`)
	sid.Post("${v.upstream}/v1/users", "application/json", body)
}`,
  },
  {
    id: "proxy-python",
    label: "Python (requests)",
    language: "python",
    build: (v) => `# Drop-in for requests. Same signatures as requests.get / .post / .request.
# Reads ECHOPROXY_API_KEY + ECHOPROXY_PROXY_URL from env.

# pip install echoproxy-sdk
# export ECHOPROXY_API_KEY=${v.apiKey}
# export ECHOPROXY_PROXY_URL=${v.proxyURL}

import echoproxy.proxy as r

res = r.get("${v.upstream}/v1/users")
print(res.status_code, res.json())

# r.post / r.put / r.patch / r.delete — same signatures as requests.*
r.post("${v.upstream}/v1/users", json={"name": "Alice"})`,
  },
  {
    id: "proxy-ts",
    label: "Node / TypeScript",
    language: "typescript",
    build: (v) => `// Drop-in for the global fetch. Same signature, same Response.
// Reads ECHOPROXY_API_KEY + ECHOPROXY_PROXY_URL from process.env.

// npm install @echoproxy/sdk
// .env:
//   ECHOPROXY_API_KEY=${v.apiKey}
//   ECHOPROXY_PROXY_URL=${v.proxyURL}

import { fetch, get, post } from "@echoproxy/sdk/proxy";

const res = await fetch("${v.upstream}/v1/users");
console.log(res.status, await res.json());

// Or convenience helpers:
await post("${v.upstream}/v1/users", { name: "Alice" });`,
  },
  {
    id: "proxy-laravel",
    label: "Laravel (PHP)",
    language: "php",
    build: (v) => `# Drop-in for the Http facade. Same shape: Sid::get/post/put/patch/delete.
# Reads ECHOPROXY_API_KEY + ECHOPROXY_PROXY_URL from .env.

# 1) Install:
composer require echoproxy/sdk-laravel

# 2) .env:
#   ECHOPROXY_API_KEY=${v.apiKey}
#   ECHOPROXY_PROXY_URL=${v.proxyURL}

<?php
use Echoproxy\\Sdk\\Facades\\Sid;

\$users   = Sid::get('${v.upstream}/v1/users')->json();
\$created = Sid::post('${v.upstream}/v1/users', ['name' => 'Alice'])->json();`,
  },
  {
    id: "proxy-headers",
    label: "Raw headers (any language)",
    language: "bash",
    build: (v) => `# Two headers are all the proxy needs. Any HTTP client in any language
# can integrate by setting them — the SDKs above just read env and inject
# these for you.

# Required headers
X-Echo-Key:    ${v.apiKey}
X-Echo-Target: ${v.upstream}     # scheme://host[:port] of the upstream

# URL: keep your original path + query, send to:
#   ${v.proxyURL}<path>?<query>

# Example with cURL:
curl -X POST '${v.proxyURL}/v1/users' \\
  -H 'X-Echo-Key: ${v.apiKey}' \\
  -H 'X-Echo-Target: ${v.upstream}' \\
  -H 'Content-Type: application/json' \\
  -d '{"name":"Alice"}'`,
  },
];

const SDK_SNIPPETS: Snippet[] = [
  {
    id: "inbound-go",
    label: "Go",
    language: "go",
    build: (v) => `// Server middleware. Captures every inbound request matching the route
// filters and ships events with direction="inbound".

// .env (route filters are comma-separated regex; both empty = capture all):
//   ECHOPROXY_API_KEY=${v.apiKey}
//   ECHOPROXY_ENDPOINT=${v.ingestURL}
//   ECHOPROXY_CAPTURE_ROUTES=^/api/.*
//   ECHOPROXY_IGNORE_ROUTES=^/api/healthz$,^/api/metrics$

package main

import (
	"net/http"
	"os"

	sdk "echoproxy/sdk-reference-go"
)

func main() {
	client, _ := sdk.New(sdk.Config{
		APIKey:       os.Getenv("ECHOPROXY_API_KEY"),
		EndpointHTTP: os.Getenv("ECHOPROXY_ENDPOINT"),
		// Routes also picked up automatically from the env vars above
	})
	defer client.Close(nil)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/users", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(\`{"ok":true}\`))
	})
	http.ListenAndServe(":8080", client.Middleware(mux))
}`,
  },
  {
    id: "inbound-python",
    label: "Python (FastAPI / ASGI)",
    language: "python",
    build: (v) => `# .env (route filters are comma-separated regex):
#   ECHOPROXY_API_KEY=${v.apiKey}
#   ECHOPROXY_ENDPOINT=${v.ingestURL}
#   ECHOPROXY_CAPTURE_ROUTES=^/api/.*
#   ECHOPROXY_IGNORE_ROUTES=^/api/healthz$,^/api/metrics$

import echoproxy
from echoproxy.asgi import CaptureMiddleware
from fastapi import FastAPI

client = echoproxy.Client()  # picks up ECHOPROXY_* env automatically

app = FastAPI()
app.add_middleware(CaptureMiddleware, client=client)

@app.get("/api/users")
def list_users():
    return [{"id": 1, "name": "Alice"}]`,
  },
  {
    id: "inbound-python-wsgi",
    label: "Python (Flask / WSGI)",
    language: "python",
    build: (v) => `# Flask, Django WSGI, Pyramid — anything WSGI-compliant.

# .env:
#   ECHOPROXY_API_KEY=${v.apiKey}
#   ECHOPROXY_ENDPOINT=${v.ingestURL}
#   ECHOPROXY_CAPTURE_ROUTES=^/api/.*
#   ECHOPROXY_IGNORE_ROUTES=^/api/healthz$

import echoproxy
from echoproxy.wsgi import CaptureMiddleware
from flask import Flask

client = echoproxy.Client()  # env-driven
app = Flask(__name__)
app.wsgi_app = CaptureMiddleware(app.wsgi_app, client)

@app.get("/api/users")
def list_users():
    return [{"id": 1, "name": "Alice"}]`,
  },
  {
    id: "inbound-ts",
    label: "Node / Express",
    language: "typescript",
    build: (v) => `// .env:
//   ECHOPROXY_API_KEY=${v.apiKey}
//   ECHOPROXY_INGEST_URL=${v.ingestURL}
//   ECHOPROXY_CAPTURE_ROUTES=^/api/.*
//   ECHOPROXY_IGNORE_ROUTES=^/api/healthz$

import express from "express";
import { IngestClient, expressMiddleware } from "@echoproxy/sdk";

const app = express();
const client = new IngestClient(); // env-driven
app.use(expressMiddleware(client));

app.get("/api/users", (_req, res) => res.json({ ok: true }));
app.listen(3000);`,
  },
  {
    id: "inbound-laravel",
    label: "Laravel (PHP)",
    language: "php",
    build: (v) => `# 1) Install:
composer require echoproxy/sdk-laravel

# 2) .env (route filters are comma-separated regex):
#   ECHOPROXY_API_KEY=${v.apiKey}
#   ECHOPROXY_ENDPOINT=${v.ingestURL}
#   ECHOPROXY_CAPTURE_ROUTES=^/api/.*
#   ECHOPROXY_IGNORE_ROUTES=^/api/healthz$,^/api/metrics$

# 3) Register the middleware in bootstrap/app.php (Laravel 11):
->withMiddleware(function (\$middleware) {
    \$middleware->append(\\Echoproxy\\Sdk\\Middleware\\CaptureRequests::class);
})

# Or in Laravel 10's app/Http/Kernel.php $middleware array.`,
  },
  {
    id: "sdk-http-json",
    label: "HTTP/JSON (any language)",
    language: "bash",
    build: (v) => `# Every SDK speaks HTTP/JSON underneath. If your language has no SDK yet,
# POST batches directly. Bodies must be base64-encoded; everything else is plain JSON.
# Server re-applies the same redaction rules — no secrets leak even if you forget to scrub.

curl -X POST '${v.ingestURL}/v1/events:batch' \\
  -H 'X-Echo-Key: ${v.apiKey}' \\
  -H 'Content-Type: application/json' \\
  -d '{
    "events": [
      {
        "method": "GET",
        "host": "api.example.com",
        "path": "/v1/users",
        "status": 200,
        "latency_ms": 42,
        "source": "sdk-custom",
        "sdk_version": "0.1.0",
        "req_headers": {"User-Agent": "my-app/1.0"},
        "res_headers": {"Content-Type": "application/json"},
        "req_body": "",
        "res_body": "eyJ1c2VycyI6W119"
      }
    ]
  }'`,
  },
  {
    id: "sdk-grpc-go",
    label: "gRPC (Go)",
    language: "go",
    build: (v) => `// gRPC is the fastest transport. Use it for high-throughput backends.
// Generated code lives in echoproxy/pkg/event from api/event.proto.

package main

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"echoproxy/pkg/event"
)

func main() {
	conn, _ := grpc.NewClient("${v.grpcAddr}", grpc.WithTransportCredentials(insecure.NewCredentials()))
	defer conn.Close()
	cli := event.NewEventIngestClient(conn)

	ctx := metadata.AppendToOutgoingContext(context.Background(), "x-echo-key", "${v.apiKey}")
	cli.Ingest(ctx, &event.IngestRequest{
		Events: []*event.HttpEvent{
			{Method: "GET", Host: "api.example.com", Path: "/v1/users", Status: 200, LatencyMs: 42},
		},
	})
}`,
  },
];

export const SECTIONS: Section[] = [
  {
    id: "outbound",
    title: "Outbound — calls your app makes",
    description:
      "Your app calling third-party APIs. Drop-in replacements for the native HTTP client of each language — same signature, env-driven.",
    snippets: PROXY_SNIPPETS,
  },
  {
    id: "inbound",
    title: "Inbound — calls your app receives",
    description:
      "Server middleware that captures every request your app handles. Use ECHOPROXY_CAPTURE_ROUTES / ECHOPROXY_IGNORE_ROUTES (comma-separated regex) to whitelist or skip specific paths.",
    snippets: SDK_SNIPPETS,
  },
];

