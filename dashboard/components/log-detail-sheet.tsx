"use client";

import { useEffect, useState } from "react";
import { format } from "date-fns";
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetDescription } from "@/components/ui/sheet";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import { CopyButton } from "@/components/copy-button";
import { DirectionBadge, MethodBadge, ModeBadge, StatusBadge, StreamBadge } from "@/components/status-badge";
import { formatBytes } from "@/lib/utils";
import type { LogEvent } from "@/lib/api/types";

export function LogDetailSheet({
  projectId,
  eventId,
  initialEvent,
  open,
  onOpenChange,
}: {
  projectId: string;
  eventId: string | null;
  initialEvent?: LogEvent;
  open: boolean;
  onOpenChange: (v: boolean) => void;
}) {
  const [event, setEvent] = useState<LogEvent | null>(initialEvent ?? null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open || !eventId) return;
    let cancelled = false;
    setLoading(true);
    setError(null);
    fetch(`/api/projects/${projectId}/logs/${eventId}`)
      .then(async (r) => {
        if (!r.ok) throw new Error(`status ${r.status}`);
        return r.json();
      })
      .then((data: LogEvent) => {
        if (!cancelled) setEvent(data);
      })
      .catch((e: Error) => {
        if (!cancelled) setError(e.message);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [open, eventId, projectId]);

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full overflow-y-auto sm:max-w-2xl">
        <SheetHeader>
          <SheetTitle>{event ? `${event.method} ${event.host}${event.path}` : "Event details"}</SheetTitle>
          <SheetDescription>
            {event ? `Captured ${format(new Date(event.ts), "yyyy-MM-dd HH:mm:ss.SSS")}` : "Loading…"}
          </SheetDescription>
          {event && (
            <div className="flex flex-wrap items-center gap-2 pt-2">
              <MethodBadge method={event.method} />
              <code className="break-all font-mono text-xs text-muted-foreground">
                {event.host}
                {event.path}
              </code>
              <ModeBadge source={event.source} />
              {event.is_stream && (
                <StreamBadge
                  isStream
                  idleTimeout={event.stream_idle_timeout}
                  chunkCount={event.stream_chunk_count}
                />
              )}
              <Badge variant="outline" className="font-mono text-[10px]">
                {event.event_id.slice(0, 8)}
              </Badge>
              <CopyButton size="icon" value={event.event_id} label="Event ID" />
            </div>
          )}
        </SheetHeader>

        {loading && (
          <div className="mt-6 space-y-2">
            <Skeleton className="h-8 w-full" />
            <Skeleton className="h-32 w-full" />
            <Skeleton className="h-32 w-full" />
          </div>
        )}

        {error && (
          <div className="mt-6 rounded-md border border-destructive/30 bg-destructive/5 p-4 text-sm text-destructive">
            Failed to load event: {error}
          </div>
        )}

        {event && !loading && (
          <div className="mt-6 space-y-6">
            <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
              <Stat label="Status" value={<StatusBadge status={event.status} />} />
              <Stat label="Total" value={`${event.latency_ms} ms`} />
              <Stat label="Req size" value={formatBytes(event.req_size)} />
              <Stat label="Res size" value={formatBytes(event.res_size)} />
            </div>

            <div>
              <div className="mb-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">
                Latency breakdown
              </div>
              <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
                <Stat label="Upstream" value={`${event.upstream_latency_ms} ms`} />
                <Stat label="TTFB" value={`${event.upstream_ttfb_ms} ms`} />
                <Stat
                  label={event.source === "proxy-gateway" ? "Proxy overhead" : "SDK overhead"}
                  value={`${Math.max(0, event.latency_ms - event.upstream_latency_ms)} ms`}
                />
                <Stat
                  label="Resp body copy"
                  value={`${Math.max(0, event.upstream_latency_ms - event.upstream_ttfb_ms)} ms`}
                />
              </div>
              <LatencyBar
                total={event.latency_ms}
                upstream={event.upstream_latency_ms}
                ttfb={event.upstream_ttfb_ms}
              />
            </div>

            {event.is_stream && (
              <div>
                <div className="mb-2 flex items-center gap-2">
                  <div className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                    Stream details
                  </div>
                  <StreamBadge
                    isStream
                    idleTimeout={event.stream_idle_timeout}
                    chunkCount={event.stream_chunk_count}
                  />
                </div>
                <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
                  <Stat
                    label="Chunks flushed"
                    value={(event.stream_chunk_count ?? 0).toLocaleString()}
                  />
                  <Stat
                    label="Stream duration"
                    value={`${event.stream_duration_ms ?? 0} ms`}
                  />
                  <Stat
                    label="Idle timeout"
                    value={
                      event.stream_idle_timeout ? (
                        <span className="text-destructive">Triggered</span>
                      ) : (
                        <span className="text-muted-foreground">No</span>
                      )
                    }
                  />
                </div>
                {event.stream_idle_timeout && (
                  <div className="mt-2 rounded-md border border-destructive/30 bg-destructive/5 p-2 text-xs text-destructive">
                    The proxy idle-timeout watchdog cancelled this stream because
                    the upstream stopped sending bytes for longer than the
                    configured threshold (<code className="font-mono">STREAM_IDLE_TIMEOUT_SECONDS</code>).
                  </div>
                )}
              </div>
            )}

            <Tabs defaultValue="overview" className="w-full">
              <TabsList className="grid w-full grid-cols-4">
                <TabsTrigger value="overview">Overview</TabsTrigger>
                <TabsTrigger value="headers">Headers</TabsTrigger>
                <TabsTrigger value="bodies">Bodies</TabsTrigger>
                <TabsTrigger value="raw">Raw</TabsTrigger>
              </TabsList>

              <TabsContent value="overview" className="space-y-3 pt-3 text-sm">
                <Field label="Direction" value={<DirectionBadge direction={event.direction} />} />
                <Field label="Method" value={event.method} />
                <Field label="Host" value={event.host} />
                <Field label="Path" value={event.path} mono />
                <Field label="Query" value={event.query || "—"} mono />
                <Field label="Source" value={event.source} />
                <Field label="Client IP" value={event.client_ip || "—"} mono />
                <Field label="User-Agent" value={event.user_agent || "—"} />
                <Field label="Trace ID" value={event.trace_id || "—"} mono />
                {event.error && <Field label="Error" value={event.error} className="text-destructive" />}
              </TabsContent>

              <TabsContent value="headers" className="space-y-4 pt-3">
                <HeaderBlock title="Request headers" headers={event.req_headers} />
                <HeaderBlock title="Response headers" headers={event.res_headers} />
              </TabsContent>

              <TabsContent value="bodies" className="space-y-4 pt-3">
                <BodyBlock title="Request body" body={event.req_body} truncated={event.req_truncated} />
                <BodyBlock title="Response body" body={event.res_body} truncated={event.res_truncated} />
              </TabsContent>

              <TabsContent value="raw" className="pt-3">
                <pre className="max-h-[60vh] overflow-auto rounded-md bg-muted p-4 text-xs">
                  {JSON.stringify(event, null, 2)}
                </pre>
              </TabsContent>
            </Tabs>
          </div>
        )}
      </SheetContent>
    </Sheet>
  );
}

function Stat({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="rounded-md border bg-card p-3">
      <div className="text-[11px] uppercase tracking-wider text-muted-foreground">{label}</div>
      <div className="mt-1 text-sm font-medium">{value}</div>
    </div>
  );
}

function Field({
  label,
  value,
  mono,
  className,
}: {
  label: string;
  value: React.ReactNode;
  mono?: boolean;
  className?: string;
}) {
  return (
    <div className="grid grid-cols-3 gap-3">
      <div className="text-muted-foreground">{label}</div>
      <div className={`col-span-2 break-all ${mono ? "font-mono text-xs" : ""} ${className ?? ""}`}>{value}</div>
    </div>
  );
}

function HeaderBlock({ title, headers }: { title: string; headers?: Record<string, string> }) {
  const entries = headers ? Object.entries(headers) : [];
  return (
    <div className="rounded-md border">
      <div className="border-b px-3 py-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">
        {title}
      </div>
      {entries.length === 0 ? (
        <div className="p-3 text-xs text-muted-foreground">No headers captured.</div>
      ) : (
        <table className="w-full text-xs">
          <tbody>
            {entries.map(([k, v]) => (
              <tr key={k} className="border-b last:border-0">
                <td className="w-1/3 px-3 py-2 font-mono text-muted-foreground">{k}</td>
                <td className="break-all px-3 py-2 font-mono">{v}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}

function BodyBlock({
  title,
  body,
  truncated,
}: {
  title: string;
  body?: string;
  truncated?: boolean;
}) {
  return (
    <div className="rounded-md border">
      <div className="flex items-center justify-between border-b px-3 py-2">
        <div className="text-xs font-medium uppercase tracking-wider text-muted-foreground">{title}</div>
        <div className="flex items-center gap-2">
          {truncated && <Badge variant="warning">truncated</Badge>}
          {body && <CopyButton value={body} size="icon" />}
        </div>
      </div>
      <pre className="max-h-72 overflow-auto p-3 text-xs">{body || <span className="text-muted-foreground">No body captured.</span>}</pre>
    </div>
  );
}

// LatencyBar renders a stacked timeline:
//   [ proxy-in | upstream-ttfb | upstream-rest | proxy-out ]
// where the segments sum to total latency. We approximate proxy_in as 0
// because the proxy timestamps the request at the start of Execute.
function LatencyBar({ total, upstream, ttfb }: { total: number; upstream: number; ttfb: number }) {
  if (total <= 0) return null;
  const safeUpstream = Math.min(upstream, total);
  const safeTtfb = Math.min(ttfb, safeUpstream);
  const proxyOut = Math.max(0, total - safeUpstream);
  const ttfbPct = (safeTtfb / total) * 100;
  const upstreamRestPct = ((safeUpstream - safeTtfb) / total) * 100;
  const proxyOutPct = (proxyOut / total) * 100;
  return (
    <div className="mt-3 space-y-1">
      <div className="flex h-2 overflow-hidden rounded">
        <div
          className="bg-blue-500"
          style={{ width: `${ttfbPct}%` }}
          title={`Upstream TTFB: ${safeTtfb} ms`}
        />
        <div
          className="bg-blue-300"
          style={{ width: `${upstreamRestPct}%` }}
          title={`Upstream stream: ${safeUpstream - safeTtfb} ms`}
        />
        <div
          className="bg-amber-500"
          style={{ width: `${proxyOutPct}%` }}
          title={`Proxy overhead (response copy): ${proxyOut} ms`}
        />
      </div>
      <div className="flex justify-between text-[10px] text-muted-foreground">
        <span>Upstream TTFB</span>
        <span>Upstream stream</span>
        <span>Proxy overhead</span>
      </div>
    </div>
  );
}
