import { ScrollText } from "lucide-react";
import { getSession } from "@/lib/auth";
import { listLogs } from "@/lib/api/stats";
import { isRedirectError } from "@/lib/utils";
import { PageHeader } from "@/components/page-header";
import { EmptyState } from "@/components/ui/empty-state";
import { LogsFilter } from "./filter-form";
import { LogsTable } from "./logs-table";

export const dynamic = "force-dynamic";

// Preset ranges follow what Datadog / Sentry / Honeycomb expose by default.
const PRESET_MS: Record<string, number> = {
  "15m":  15 * 60 * 1000,
  "1h":   60 * 60 * 1000,
  "6h":   6  * 60 * 60 * 1000,
  "24h":  24 * 60 * 60 * 1000,
  "7d":   7  * 24 * 60 * 60 * 1000,
  "30d":  30 * 24 * 60 * 60 * 1000,
};

type ResolvedRange =
  | { kind: "preset"; preset: string; from: string; to: string }
  | { kind: "custom"; from: string; to: string };

function resolveRange(searchParams: { range?: string; from?: string; to?: string }): ResolvedRange {
  // Custom always wins when both bounds are present.
  if (searchParams.from && searchParams.to) {
    return { kind: "custom", from: searchParams.from, to: searchParams.to };
  }
  const preset = searchParams.range && PRESET_MS[searchParams.range] ? searchParams.range : "1h";
  const ms = PRESET_MS[preset];
  const to = new Date();
  const from = new Date(to.getTime() - ms);
  return { kind: "preset", preset, from: from.toISOString(), to: to.toISOString() };
}

export default async function LogsPage({
  params,
  searchParams,
}: {
  params: { id: string };
  searchParams: { method?: string; status?: string; path?: string; direction?: string; range?: string; from?: string; to?: string; is_stream?: string };
}) {
  const session = await getSession();
  const direction =
    searchParams.direction === "inbound" || searchParams.direction === "outbound"
      ? searchParams.direction
      : undefined;
  const isStream =
    searchParams.is_stream === "true" ? true
      : searchParams.is_stream === "false" ? false
      : undefined;
  const range = resolveRange(searchParams);

  let logs: Awaited<ReturnType<typeof listLogs>> = [];
  let error: string | null = null;
  try {
    logs = await listLogs(session.token, params.id, {
      method: searchParams.method,
      status: searchParams.status ? Number(searchParams.status) : undefined,
      path: searchParams.path,
      direction,
      is_stream: isStream,
      from: range.from,
      to: range.to,
      limit: 200,
    });
  } catch (e) {
    if (isRedirectError(e)) throw e;
    error = (e as Error).message;
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Logs"
        description="Every HTTP request captured by the proxy or an SDK. Click any row to inspect headers and body."
      />

      <LogsFilter
        defaults={{
          ...searchParams,
          direction,
          range: range.kind === "preset" ? range.preset : "custom",
          from: range.kind === "custom" ? range.from : undefined,
          to:   range.kind === "custom" ? range.to   : undefined,
        }}
      />

      {error && (
        <div className="rounded-md border border-destructive/30 bg-destructive/5 p-4 text-sm text-destructive">
          stats-api unavailable: {error}
        </div>
      )}

      {logs.length === 0 ? (
        <EmptyState
          icon={ScrollText}
          title="No events match"
          description="Loosen the filters above, expand the time range, or send traffic through the proxy / an SDK to populate logs."
        />
      ) : (
        <LogsTable projectId={params.id} logs={logs} />
      )}
    </div>
  );
}
