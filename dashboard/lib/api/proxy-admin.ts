// Client for the proxy-gateway admin endpoint. Exposes /admin/config so the
// dashboard can display the effective timeouts the proxy is running with.
// The admin port (default :6060) is on a private network; this fetcher
// fails gracefully when the endpoint is unreachable.

export type ProxyAdminConfig = {
  upstream_timeout_seconds: number;
  stream_idle_timeout_seconds: number;
  body_cap_bytes: number;
  allow_private_targets: boolean;
};

const PROXY_ADMIN = process.env.PROXY_ADMIN_URL ?? "http://localhost:6060";

export async function getProxyConfig(): Promise<ProxyAdminConfig | null> {
  try {
    const res = await fetch(`${PROXY_ADMIN}/admin/config`, {
      cache: "no-store",
      // Admin endpoint is unauthenticated by design (private network only).
      headers: { Accept: "application/json" },
    });
    if (!res.ok) return null;
    return (await res.json()) as ProxyAdminConfig;
  } catch {
    return null;
  }
}
