# @echoproxy/sdk

TypeScript / JavaScript SDK for the [EchoProxy](../README.md) HTTP observability platform.

## Capture outbound HTTP — pick a mode

There are **two** ways to capture outbound calls. Pick one per project — see [`docs/inbound-vs-outbound.md`](../docs/inbound-vs-outbound.md) for the full comparison.

|                       | Proxy mode (`@echoproxy/sdk/proxy`) | Capture mode (`@echoproxy/sdk/capture`) |
|-----------------------|-------------------------------------|------------------------------------------|
| Who calls the upstream | **proxy-gateway** (Go)             | **your runtime** (native `fetch`)        |
| Where the event is emitted | proxy-gateway → Kafka          | SDK → ingest-api → Kafka                 |
| `upstream_latency_ms` | server-side, authoritative          | client-side measurement                  |
| `upstream_ttfb_ms`    | yes                                 | 0 (fetch doesn't expose TTFB)            |
| Dashboard `source`    | `proxy-gateway`                     | `sdk-ts`                                 |
| Dashboard mode badge  | **proxy**                           | **capture**                              |
| Code change           | swap `fetch` import                 | wrap `fetch` with `captureFetch`         |
| Best for              | most projects                       | edge runtimes / firewalled environments  |

## Install

```bash
npm install @echoproxy/sdk
# or pnpm / yarn / bun

export ECHOPROXY_API_KEY=sk_live_xxx
export ECHOPROXY_PROXY_URL=http://localhost:8080  # optional, default http://localhost:8080
export ECHOPROXY_INGEST_URL=http://localhost:8081 # capture mode only, default http://localhost:8081
```

## Proxy mode — drop-in for `fetch`

```ts
import { fetch } from "@echoproxy/sdk/proxy";

// Same signature as global fetch — just import this one.
const res = await fetch("https://api.example.com/v1/users", {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({ name: "Alice" }),
});
```

Or use the convenience helpers:

```ts
import { get, post, put, patch, del } from "@echoproxy/sdk/proxy";

await get("https://api.example.com/v1/users");
await post("https://api.example.com/v1/users", { name: "Alice" });
```

## Capture mode — wrap `fetch`

Use this when your runtime can't reach `proxy-gateway:8080` (edge worker, Lambda + private VPC, etc.) or you want per-call sampling / redaction that the proxy can't see.

```ts
import { IngestClient, captureFetch } from "@echoproxy/sdk";

const client = new IngestClient();           // reads ECHOPROXY_API_KEY + ECHOPROXY_INGEST_URL
const fetch = captureFetch(client);          // same signature as global fetch

const res = await fetch("https://api.example.com/v1/users");
```

Or wrap `globalThis.fetch` process-wide:

```ts
globalThis.fetch = captureFetch(client, globalThis.fetch);
```

Events land in `ingest-api` with `source = sdk-ts`, `direction = outbound`, and the dashboard mode badge shows **capture**.

## Override the env

```ts
import { configure } from "@echoproxy/sdk/proxy";

configure("sk_live_xxx", "https://proxy.echoproxy.io");
```

## Runtime

Node 18+ (uses native `fetch`). Works in Bun, Deno, and edge runtimes.
