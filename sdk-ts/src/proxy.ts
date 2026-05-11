/**
 * Drop-in replacement for the global `fetch` that routes every call through
 * the EchoProxy proxy. Same signature as `fetch`, so existing code only needs
 * the import swapped:
 *
 * ```ts
 * import { fetch } from "@echoproxy/sdk/proxy";
 * await fetch("https://api.example.com/v1/users");
 * ```
 *
 * Configuration is environment-driven and read at first use:
 *
 *   ECHOPROXY_API_KEY     required
 *   ECHOPROXY_PROXY_URL   default http://localhost:8080
 *
 * Override programmatically with {@link configure}.
 */

type ProxyConfig = {
  apiKey: string;
  proxyURL: string;
};

let _cfg: ProxyConfig | null = null;

function loadEnv(): ProxyConfig {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const env = (globalThis as any)?.process?.env ?? {};
  const apiKey = env.ECHOPROXY_API_KEY ?? "";
  const proxyURL = (env.ECHOPROXY_PROXY_URL ?? "http://localhost:8080").replace(/\/$/, "");
  return { apiKey, proxyURL };
}

function ensure(): ProxyConfig {
  if (_cfg === null) _cfg = loadEnv();
  if (!_cfg.apiKey) {
    throw new Error("echoproxy: ECHOPROXY_API_KEY env not set");
  }
  return _cfg;
}

/** Override env defaults. Useful in tests or multi-tenant apps. */
export function configure(apiKey: string, proxyURL = "http://localhost:8080"): void {
  _cfg = { apiKey, proxyURL: proxyURL.replace(/\/$/, "") };
}

/** Drop-in replacement for the global `fetch`. Same signature, same return. */
export async function fetch(input: string | URL | Request, init: RequestInit = {}): Promise<Response> {
  const { apiKey, proxyURL } = ensure();
  const targetURL =
    typeof input === "string" || input instanceof URL ? new URL(input.toString()) : new URL(input.url);
  const target = `${targetURL.protocol}//${targetURL.host}`;
  const proxied = `${proxyURL}${targetURL.pathname}${targetURL.search}`;
  const headers = new Headers(init.headers ?? (input instanceof Request ? input.headers : undefined));
  headers.set("X-Echo-Key", apiKey);
  headers.set("X-Echo-Target", target);

  // Preserve method/body when caller passed a Request.
  let finalInit: RequestInit = { ...init, headers };
  if (input instanceof Request && !init.method) finalInit.method = input.method;
  if (input instanceof Request && !init.body && input.body) finalInit.body = input.body;

  return globalThis.fetch(proxied, finalInit);
}

/* ─── convenience helpers — mirror common axios/got names ──────────────── */

export const get = (url: string, init?: RequestInit): Promise<Response> =>
  fetch(url, { ...init, method: "GET" });

export const head = (url: string, init?: RequestInit): Promise<Response> =>
  fetch(url, { ...init, method: "HEAD" });

export const del = (url: string, init?: RequestInit): Promise<Response> =>
  fetch(url, { ...init, method: "DELETE" });

export const post = (url: string, body?: unknown, init?: RequestInit): Promise<Response> =>
  send("POST", url, body, init);

export const put = (url: string, body?: unknown, init?: RequestInit): Promise<Response> =>
  send("PUT", url, body, init);

export const patch = (url: string, body?: unknown, init?: RequestInit): Promise<Response> =>
  send("PATCH", url, body, init);

function send(method: string, url: string, body?: unknown, init?: RequestInit): Promise<Response> {
  const headers = new Headers(init?.headers ?? {});
  let payload: BodyInit | null | undefined;
  if (body === undefined || body === null) {
    payload = body as null | undefined;
  } else if (typeof body === "string" || body instanceof FormData || body instanceof URLSearchParams || body instanceof ArrayBuffer || body instanceof Blob) {
    payload = body as BodyInit;
  } else {
    if (!headers.has("Content-Type")) headers.set("Content-Type", "application/json");
    payload = JSON.stringify(body);
  }
  return fetch(url, { ...init, method, headers, body: payload });
}
