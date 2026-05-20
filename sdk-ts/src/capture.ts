/**
 * Outbound capture mode: wrap the native `fetch` so the caller's runtime makes
 * the upstream call, and the SDK ships an event to ingest-api.
 *
 * Use this when:
 *   - your runtime can't reach proxy-gateway (Lambda + VPC, edge worker, etc.)
 *   - you already have many `fetch` call-sites and don't want a URL rewrite
 *   - you need per-call sampling / redaction that the proxy can't see
 *
 * For most projects, prefer `@echoproxy/sdk/proxy` — proxy-gateway gives you
 * authoritative TTFB/upstream timing measured server-side.
 *
 * Usage — drop-in wrapper:
 *
 * ```ts
 * import { Client as IngestClient } from "@echoproxy/sdk/ingest";
 * import { captureFetch } from "@echoproxy/sdk/capture";
 *
 * const client = new IngestClient();           // reads ECHOPROXY_API_KEY etc.
 * const fetch = captureFetch(client);          // same signature as global fetch
 *
 * const res = await fetch("https://api.example.com/v1/users");
 * ```
 *
 * Or wrap globalThis.fetch process-wide:
 *
 * ```ts
 * globalThis.fetch = captureFetch(client, globalThis.fetch);
 * ```
 *
 * Events land in ingest-api with source = sdk-ts, direction = outbound. The
 * dashboard mode badge shows "capture".
 */

import type { CaptureEvent, Client } from "./ingest.js";

export type CaptureFetch = (
  input: string | URL | Request,
  init?: RequestInit,
) => Promise<Response>;

/**
 * Build a fetch wrapper that captures every outbound request. The returned
 * function has the same signature as the global `fetch` and is safe to use as
 * a drop-in. The base fetch defaults to `globalThis.fetch` so callers can pass
 * a custom one (useful for tests or to layer over an existing wrapper).
 */
export function captureFetch(
  client: Client,
  base: typeof globalThis.fetch = globalThis.fetch.bind(globalThis),
): CaptureFetch {
  return async (input, init = {}) => {
    const url = toURL(input);
    const reqHeaders = headersFromInput(input, init);
    const reqBody = await readReqBody(input, init);
    const start = nowMs();

    let res: Response;
    try {
      res = await base(input as Parameters<typeof globalThis.fetch>[0], init);
    } catch (err) {
      // Network failure — still emit an event so the dashboard can show it.
      safeCapture(client, {
        direction: "outbound",
        method: methodOf(input, init),
        scheme: url.protocol.replace(/:$/, ""),
        host: url.host,
        path: url.pathname,
        query: url.search.replace(/^\?/, ""),
        status: 0,
        latency_ms: nowMs() - start,
        req_size: reqBody?.byteLength ?? 0,
        req_headers: reqHeaders,
        req_body: reqBody ?? "",
      });
      throw err;
    }

    const latencyMs = nowMs() - start;
    // Clone so the caller can still read the body downstream.
    const cloned = res.clone();
    let resBody: Uint8Array = new Uint8Array();
    try {
      resBody = new Uint8Array(await cloned.arrayBuffer());
    } catch {
      /* fail open — body not readable (e.g. already consumed in some runtimes) */
    }

    safeCapture(client, {
      direction: "outbound",
      method: methodOf(input, init),
      scheme: url.protocol.replace(/:$/, ""),
      host: url.host,
      path: url.pathname,
      query: url.search.replace(/^\?/, ""),
      status: res.status,
      latency_ms: latencyMs,
      req_size: reqBody?.byteLength ?? 0,
      res_size: resBody.byteLength,
      req_headers: reqHeaders,
      res_headers: headersToRecord(res.headers),
      req_body: reqBody ?? "",
      res_body: resBody,
    });
    return res;
  };
}

function safeCapture(client: Client, ev: CaptureEvent): void {
  try {
    client.capture(ev);
  } catch {
    /* fail open — capture must never break the caller's hot path */
  }
}

function toURL(input: string | URL | Request): URL {
  if (typeof input === "string") return new URL(input);
  if (input instanceof URL) return input;
  return new URL(input.url);
}

function methodOf(input: string | URL | Request, init: RequestInit): string {
  if (init.method) return init.method;
  if (input instanceof Request) return input.method;
  return "GET";
}

function headersFromInput(
  input: string | URL | Request,
  init: RequestInit,
): Record<string, string> {
  const h = new Headers();
  if (input instanceof Request) {
    input.headers.forEach((v, k) => h.set(k, v));
  }
  if (init.headers) {
    new Headers(init.headers).forEach((v, k) => h.set(k, v));
  }
  return headersToRecord(h);
}

function headersToRecord(h: Headers): Record<string, string> {
  const out: Record<string, string> = {};
  h.forEach((v, k) => {
    out[k] = v;
  });
  return out;
}

async function readReqBody(
  input: string | URL | Request,
  init: RequestInit,
): Promise<Uint8Array | undefined> {
  // Easy cases first — caller passed body as init.body.
  if (init.body !== undefined && init.body !== null) {
    return await bodyInitToBytes(init.body);
  }
  // Request object — clone so we don't consume the caller's body.
  if (input instanceof Request) {
    try {
      const buf = await input.clone().arrayBuffer();
      return new Uint8Array(buf);
    } catch {
      return undefined;
    }
  }
  return undefined;
}

async function bodyInitToBytes(body: BodyInit): Promise<Uint8Array | undefined> {
  if (typeof body === "string") return new TextEncoder().encode(body);
  if (body instanceof Uint8Array) return body;
  if (body instanceof ArrayBuffer) return new Uint8Array(body);
  if (body instanceof Blob) return new Uint8Array(await body.arrayBuffer());
  if (body instanceof URLSearchParams) return new TextEncoder().encode(body.toString());
  if (body instanceof FormData) {
    // FormData has no cheap serialization; skip body capture rather than
    // re-encoding multipart. The wire still gets headers + status + size=0.
    return undefined;
  }
  if (body instanceof ReadableStream) {
    // Don't drain a stream — we'd starve the real fetch. Skip body capture.
    return undefined;
  }
  return undefined;
}

function nowMs(): number {
  // performance.now() is monotonic and present in Node 18+, Bun, Deno, browsers.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const perf: any = (globalThis as any).performance;
  if (perf && typeof perf.now === "function") return perf.now();
  return Date.now();
}
