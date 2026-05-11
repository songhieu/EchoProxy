# @echoproxy/sdk

TypeScript / JavaScript SDK for the [EchoProxy](../README.md) HTTP observability platform. Drop-in replacement for the global `fetch`.

## Install

```bash
npm install @echoproxy/sdk
# or pnpm / yarn / bun

export ECHOPROXY_API_KEY=sk_live_xxx
export ECHOPROXY_PROXY_URL=http://localhost:8080  # optional, default http://localhost:8080
```

## Use

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

## Override the env

```ts
import { configure } from "@echoproxy/sdk/proxy";

configure("sk_live_xxx", "https://proxy.echoproxy.io");
```

## Runtime

Node 18+ (uses native `fetch`). Works in Bun, Deno, and edge runtimes.
