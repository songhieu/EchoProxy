import type { AnalyticsRange } from "./stats";

export function parseRange(sp: URLSearchParams): AnalyticsRange {
  return {
    from: sp.get("from") ?? "",
    to: sp.get("to") ?? "",
    api_key_id: sp.get("api_key_id") ? Number(sp.get("api_key_id")) : undefined,
    method: sp.get("method") ?? undefined,
    host: sp.get("host") ?? undefined,
    path: sp.get("path") ?? undefined,
    direction: (sp.get("direction") as "inbound" | "outbound") || undefined,
  };
}
