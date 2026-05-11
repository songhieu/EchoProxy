/**
 * Ingest client + Express/connect-style middleware. Captures every inbound
 * request matching the configured route filter and ships events to ingest-api
 * in batches.
 *
 * Configuration (all read once at first use):
 *   ECHOPROXY_API_KEY           required
 *   ECHOPROXY_INGEST_URL        default http://localhost:8081
 *   ECHOPROXY_CAPTURE_ROUTES    comma-separated regex; empty = allow all
 *   ECHOPROXY_IGNORE_ROUTES     comma-separated regex; matched routes are skipped
 *   ECHOPROXY_MAX_BODY_BYTES    default 65536
 *   ECHOPROXY_BATCH_SIZE        default 500
 *   ECHOPROXY_FLUSH_INTERVAL_MS default 2000
 */

import type { IncomingMessage, ServerResponse } from "node:http";

export type IngestConfig = {
  apiKey: string;
  ingestURL: string;
  captureRoutes: RegExp[];
  ignoreRoutes: RegExp[];
  maxBodyBytes: number;
  batchSize: number;
  bufferSize: number;
  flushIntervalMs: number;
  sampleRate: number;
};

export type CaptureEvent = {
  direction?: "inbound" | "outbound";
  method: string;
  scheme: string;
  host: string;
  path: string;
  query?: string;
  status: number;
  latency_ms: number;
  req_size?: number;
  res_size?: number;
  req_headers?: Record<string, string>;
  res_headers?: Record<string, string>;
  req_body?: Uint8Array | string;
  res_body?: Uint8Array | string;
  client_ip?: string;
  user_agent?: string;
  trace_id?: string;
  attributes?: Record<string, string>;
};

const SOURCE = "sdk-ts";
const VERSION = "0.1.0";

function envInt(name: string, def: number): number {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const v = (globalThis as any)?.process?.env?.[name];
  if (!v) return def;
  const n = Number(v);
  return Number.isFinite(n) && n > 0 ? n : def;
}

function envStr(name: string, def = ""): string {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  return (globalThis as any)?.process?.env?.[name] ?? def;
}

function envRoutes(name: string): RegExp[] {
  return envStr(name)
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean)
    .map((p) => {
      try {
        return new RegExp(p);
      } catch {
        return null;
      }
    })
    .filter((r): r is RegExp => r !== null);
}

function loadConfig(overrides: Partial<IngestConfig> = {}): IngestConfig {
  return {
    apiKey: overrides.apiKey ?? envStr("ECHOPROXY_API_KEY"),
    ingestURL: overrides.ingestURL ?? envStr("ECHOPROXY_INGEST_URL", "http://localhost:8081"),
    captureRoutes: overrides.captureRoutes ?? envRoutes("ECHOPROXY_CAPTURE_ROUTES"),
    ignoreRoutes: overrides.ignoreRoutes ?? envRoutes("ECHOPROXY_IGNORE_ROUTES"),
    maxBodyBytes: overrides.maxBodyBytes ?? envInt("ECHOPROXY_MAX_BODY_BYTES", 64 * 1024),
    batchSize: overrides.batchSize ?? envInt("ECHOPROXY_BATCH_SIZE", 500),
    bufferSize: overrides.bufferSize ?? envInt("ECHOPROXY_BUFFER_SIZE", 10_000),
    flushIntervalMs: overrides.flushIntervalMs ?? envInt("ECHOPROXY_FLUSH_INTERVAL_MS", 2000),
    sampleRate: overrides.sampleRate ?? 1.0,
  };
}

export class Client {
  readonly cfg: IngestConfig;
  private buf: Record<string, unknown>[] = [];
  private dropped = 0;
  private timer: ReturnType<typeof setInterval> | null = null;

  constructor(overrides: Partial<IngestConfig> = {}) {
    this.cfg = loadConfig(overrides);
    if (this.cfg.apiKey && this.cfg.flushIntervalMs > 0) {
      this.timer = setInterval(() => this.flush(), this.cfg.flushIntervalMs);
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const t: any = this.timer;
      if (typeof t.unref === "function") t.unref();
    }
  }

  routeAllowed(path: string): boolean {
    for (const r of this.cfg.ignoreRoutes) if (r.test(path)) return false;
    if (this.cfg.captureRoutes.length === 0) return true;
    return this.cfg.captureRoutes.some((r) => r.test(path));
  }

  capture(ev: CaptureEvent): void {
    if (!this.cfg.apiKey) return;
    if (this.cfg.sampleRate < 1.0 && Math.random() > this.cfg.sampleRate) return;
    if (this.buf.length >= this.cfg.bufferSize) {
      this.buf.shift();
      this.dropped++;
    }
    this.buf.push(this.normalize(ev));
    if (this.buf.length >= this.cfg.batchSize) void this.flush();
  }

  async flush(): Promise<void> {
    if (this.buf.length === 0 || !this.cfg.apiKey) return;
    const batch = this.buf;
    this.buf = [];
    try {
      await globalThis.fetch(`${this.cfg.ingestURL.replace(/\/$/, "")}/v1/events:batch`, {
        method: "POST",
        headers: { "Content-Type": "application/json", "X-Echo-Key": this.cfg.apiKey },
        body: JSON.stringify({ events: batch }),
      });
    } catch {
      this.dropped += batch.length;
    }
  }

  close(): void {
    if (this.timer) clearInterval(this.timer);
    this.timer = null;
    void this.flush();
  }

  get droppedCount(): number {
    return this.dropped;
  }

  private normalize(ev: CaptureEvent): Record<string, unknown> {
    const reqBody = capAndEncode(ev.req_body, this.cfg.maxBodyBytes);
    const resBody = capAndEncode(ev.res_body, this.cfg.maxBodyBytes);
    return {
      event_id: cryptoHex16(),
      timestamp_ns: BigInt(Date.now()) * 1_000_000n,
      source: SOURCE,
      sdk_version: VERSION,
      direction: ev.direction ?? "inbound",
      method: ev.method,
      scheme: ev.scheme,
      host: ev.host,
      path: ev.path,
      query: ev.query ?? "",
      status: ev.status,
      latency_ms: ev.latency_ms,
      req_size: ev.req_size ?? 0,
      res_size: ev.res_size ?? 0,
      req_headers: ev.req_headers ?? {},
      res_headers: ev.res_headers ?? {},
      req_body: reqBody,
      res_body: resBody,
      client_ip: ev.client_ip ?? "",
      user_agent: ev.user_agent ?? "",
      trace_id: ev.trace_id ?? "",
      attributes: ev.attributes ?? {},
    };
  }
}

function capAndEncode(body: Uint8Array | string | undefined, max: number): string {
  if (!body) return "";
  const bytes = typeof body === "string" ? new TextEncoder().encode(body) : body;
  const trimmed = bytes.length > max ? bytes.slice(0, max) : bytes;
  if (typeof Buffer !== "undefined") return Buffer.from(trimmed).toString("base64");
  // browser fallback
  let bin = "";
  for (const b of trimmed) bin += String.fromCharCode(b);
  return btoa(bin);
}

function cryptoHex16(): string {
  const arr = new Uint8Array(16);
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  (globalThis as any).crypto.getRandomValues(arr);
  return Array.from(arr, (b) => b.toString(16).padStart(2, "0")).join("");
}

/** Express / connect-style middleware. Drop into `app.use(...)`. */
export function expressMiddleware(client: Client) {
  return function (req: IncomingMessage & { originalUrl?: string }, res: ServerResponse, next: () => void): void {
    const path = (req.url ?? "").split("?")[0];
    if (!client.routeAllowed(path)) {
      next();
      return;
    }
    const start = Date.now();
    const reqChunks: Buffer[] = [];
    req.on("data", (c) => reqChunks.push(c));

    const resChunks: Buffer[] = [];
    const origWrite = res.write.bind(res);
    const origEnd = res.end.bind(res);
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    res.write = function (chunk: any, ...rest: any[]): boolean {
      if (chunk) resChunks.push(Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk));
      return origWrite(chunk, ...rest);
    };
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    res.end = function (chunk?: any, ...rest: any[]): ServerResponse {
      if (chunk) resChunks.push(Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk));
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const r = (origEnd as any)(chunk, ...rest);
      try {
        const reqBody = Buffer.concat(reqChunks);
        const resBody = Buffer.concat(resChunks);
        const url = new URL(req.url ?? "/", "http://placeholder");
        client.capture({
          direction: "inbound",
          method: req.method ?? "",
          scheme: (req.socket as { encrypted?: boolean }).encrypted ? "https" : "http",
          host: (req.headers.host ?? "") as string,
          path: url.pathname,
          query: url.searchParams.toString(),
          status: res.statusCode,
          latency_ms: Date.now() - start,
          req_size: reqBody.length,
          res_size: resBody.length,
          req_headers: flatHeaders(req.headers),
          res_headers: flatHeaders(res.getHeaders()),
          req_body: reqBody,
          res_body: resBody,
          client_ip: (req.socket.remoteAddress ?? "") as string,
          user_agent: (req.headers["user-agent"] ?? "") as string,
          trace_id: (req.headers["traceparent"] ?? "") as string,
        });
      } catch {
        /* fail open */
      }
      return r;
    };

    next();
  };
}

function flatHeaders(h: Record<string, string | string[] | number | undefined>): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(h)) {
    if (v == null) continue;
    out[k] = Array.isArray(v) ? v.join(",") : String(v);
  }
  return out;
}
