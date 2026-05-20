import { ArrowDownLeft, ArrowUpRight, Radio, AlertCircle, Network, Package } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { statusVariant } from "@/lib/utils";

// ModeBadge distinguishes how an event was captured:
//   - Proxy   → the request went through proxy-gateway (Go did the actual
//               upstream RoundTrip). Source = "proxy-gateway" (legacy: "proxy").
//               Full latency breakdown (upstream / TTFB / overhead) is
//               authoritative.
//   - Capture → the SDK called the upstream itself, then shipped an event
//               to ingest-api. Source = "sdk-*". upstream_latency_ms is the
//               transport time as measured by the SDK; "overhead" reflects
//               the in-app wrapper cost (usually tiny), not a proxy hop.
export function ModeBadge({ source }: { source?: string }) {
  // Accept the pre-0.4 "proxy" source so existing ClickHouse rows still
  // render with the correct badge without a data migration.
  const isProxy = source === "proxy-gateway" || source === "proxy";
  if (isProxy) {
    return (
      <Badge
        variant="default"
        className="gap-1 font-mono text-[10px]"
        title="Proxy mode — proxy-gateway forwarded the request to the upstream. Latency breakdown is measured server-side."
      >
        <Network className="h-3 w-3" /> proxy
      </Badge>
    );
  }
  return (
    <Badge
      variant="secondary"
      className="gap-1 font-mono text-[10px]"
      title="Capture mode — the SDK called the upstream directly and shipped the event to ingest-api. upstream_latency_ms is the SDK's transport measurement."
    >
      <Package className="h-3 w-3" /> capture
    </Badge>
  );
}

export function DirectionBadge({ direction }: { direction?: string }) {
  if (direction === "inbound") {
    return (
      <Badge variant="secondary" className="gap-1 font-medium">
        <ArrowDownLeft className="h-3 w-3" /> in
      </Badge>
    );
  }
  if (direction === "outbound") {
    return (
      <Badge variant="outline" className="gap-1 font-medium">
        <ArrowUpRight className="h-3 w-3" /> out
      </Badge>
    );
  }
  return <Badge variant="outline">—</Badge>;
}

export function StatusBadge({ status }: { status: number }) {
  return (
    <Badge variant={statusVariant(status)} className="font-mono">
      {status || "—"}
    </Badge>
  );
}

// StreamBadge marks responses the proxy detected as a stream (SSE / gRPC /
// chunked) and flushed chunk-by-chunk. The `idle` variant indicates the
// stream was terminated by the proxy's idle-timeout watchdog.
export function StreamBadge({
  isStream,
  idleTimeout,
  chunkCount,
}: {
  isStream?: boolean;
  idleTimeout?: boolean;
  chunkCount?: number;
}) {
  if (!isStream) return null;
  if (idleTimeout) {
    return (
      <Badge
        variant="destructive"
        className="gap-1 font-mono text-[10px]"
        title="Stream terminated by the proxy idle-timeout watchdog"
      >
        <AlertCircle className="h-3 w-3" /> STREAM idle
      </Badge>
    );
  }
  return (
    <Badge
      variant="default"
      className="gap-1 font-mono text-[10px]"
      title={chunkCount ? `Streamed ${chunkCount} chunks to client` : "Streaming response"}
    >
      <Radio className="h-3 w-3" /> STREAM
      {typeof chunkCount === "number" && chunkCount > 0 && (
        <span className="opacity-70">{chunkCount}</span>
      )}
    </Badge>
  );
}

export function MethodBadge({ method }: { method: string }) {
  const m = method.toUpperCase();
  const variant: "default" | "secondary" | "destructive" | "outline" =
    m === "GET" ? "secondary" : m === "POST" || m === "PUT" || m === "PATCH" ? "default" : "outline";
  return (
    <Badge variant={variant} className="font-mono">
      {m}
    </Badge>
  );
}
