import { ArrowDownLeft, ArrowUpRight } from "lucide-react";
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
