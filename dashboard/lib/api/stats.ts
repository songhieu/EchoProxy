import { redirect } from "next/navigation";
import type { DistBucket, EndpointStat, LogEvent, MinuteMetric, TimeBucket } from "./types";

export type AnalyticsRange = {
  from: string; // ISO
  to: string; // ISO
  api_key_id?: number;
  method?: string;
  host?: string;
  path?: string; // case-insensitive substring
  direction?: "inbound" | "outbound";
};

const STATS_API = process.env.STATS_API_URL ?? "http://localhost:8084";

async function get<T>(path: string, token: string): Promise<T> {
  const res = await fetch(STATS_API + path, {
    headers: { Authorization: `Bearer ${token}` },
    cache: "no-store",
  });
  // Stale token → auto-logout. Matches auth-api request() behavior.
  if (res.status === 401) redirect("/api/auth/signout?callbackUrl=/login");
  if (!res.ok) {
    // Surface the stats-api error body so the dashboard route can return
    // something diagnosable instead of swallowing the message.
    const body = await res.text().catch(() => "");
    throw new Error(`stats-api ${res.status}: ${body.slice(0, 400) || "<empty body>"}`);
  }
  return res.json() as Promise<T>;
}

async function getList<T>(path: string, token: string): Promise<T[]> {
  const data = await get<T[] | null>(path, token);
  return data ?? [];
}

export async function listLogs(
  token: string,
  projectID: string | number,
  q: {
    method?: string;
    status?: number;
    path?: string;
    direction?: string;
    from?: string;
    to?: string;
    limit?: number;
    is_stream?: boolean | "true" | "false" | "";
  } = {},
) {
  const params = new URLSearchParams();
  for (const [k, v] of Object.entries(q)) {
    if (v === undefined || v === null || v === "") continue;
    params.set(k, typeof v === "boolean" ? (v ? "true" : "false") : String(v));
  }
  return getList<LogEvent>(`/v1/projects/${projectID}/logs?${params.toString()}`, token);
}

export async function getLog(token: string, projectID: string | number, eventID: string) {
  return get<LogEvent>(`/v1/projects/${projectID}/logs/${eventID}`, token);
}

export async function getMinuteMetrics(
  token: string,
  projectID: string | number,
  from?: string,
  to?: string,
) {
  const params = new URLSearchParams();
  if (from) params.set("from", from);
  if (to) params.set("to", to);
  return getList<MinuteMetric>(`/v1/projects/${projectID}/metrics?${params.toString()}`, token);
}

export async function getTopPaths(token: string, projectID: string | number, limit = 20) {
  return getList<{ path: string; method: string; count: number; avg_latency_ms: number }>(
    `/v1/projects/${projectID}/top-paths?limit=${limit}`,
    token,
  );
}

function rangeQS(r: AnalyticsRange): string {
  const params = new URLSearchParams({ from: r.from, to: r.to });
  if (r.api_key_id) params.set("api_key_id", String(r.api_key_id));
  if (r.method) params.set("method", r.method);
  if (r.host) params.set("host", r.host);
  if (r.path) params.set("path", r.path);
  if (r.direction) params.set("direction", r.direction);
  return params.toString();
}

export async function getTimeSeries(token: string, projectID: string | number, r: AnalyticsRange) {
  return getList<TimeBucket>(`/v1/projects/${projectID}/timeseries?${rangeQS(r)}`, token);
}

export async function getDistribution(
  token: string,
  projectID: string | number,
  kind: "status" | "method" | "host",
  r: AnalyticsRange,
) {
  const qs = rangeQS(r);
  return getList<DistBucket>(`/v1/projects/${projectID}/distribution?kind=${kind}&${qs}`, token);
}

export async function getEndpoints(
  token: string,
  projectID: string | number,
  r: AnalyticsRange,
  limit = 50,
) {
  return getList<EndpointStat>(`/v1/projects/${projectID}/endpoints?limit=${limit}&${rangeQS(r)}`, token);
}
