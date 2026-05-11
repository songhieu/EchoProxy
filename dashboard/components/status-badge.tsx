import { ArrowDownLeft, ArrowUpRight, Radio, AlertCircle } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { statusVariant } from "@/lib/utils";

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
