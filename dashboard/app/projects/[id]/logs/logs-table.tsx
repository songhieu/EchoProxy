"use client";

import { useState } from "react";
import { format } from "date-fns";
import { Card, CardContent } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { LogDetailSheet } from "@/components/log-detail-sheet";
import { DirectionBadge, MethodBadge, StatusBadge } from "@/components/status-badge";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import type { LogEvent } from "@/lib/api/types";

export function LogsTable({ projectId, logs }: { projectId: string; logs: LogEvent[] }) {
  const [selected, setSelected] = useState<LogEvent | null>(null);

  return (
    <>
      <Card>
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-[14rem]">Time</TableHead>
                <TableHead className="w-16">Dir</TableHead>
                <TableHead>Method</TableHead>
                <TableHead>Host + path</TableHead>
                <TableHead>Status</TableHead>
                <TableHead className="text-right">Total</TableHead>
                <TableHead className="text-right" title="Upstream RoundTrip">Upstream</TableHead>
                <TableHead className="text-right" title="Time-to-first-byte from upstream">TTFB</TableHead>
                <TableHead className="text-right" title="Proxy-side overhead = total − upstream">Overhead</TableHead>
                <TableHead>Source</TableHead>
                <TableHead>Trace</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {logs.map((l) => (
                <TableRow
                  key={l.event_id}
                  onClick={() => setSelected(l)}
                  className={cn("cursor-pointer", selected?.event_id === l.event_id && "bg-muted/50")}
                >
                  <TableCell className="font-mono text-xs">
                    {format(new Date(l.ts), "HH:mm:ss.SSS")}
                    <div className="text-[10px] text-muted-foreground">
                      {format(new Date(l.ts), "yyyy-MM-dd")}
                    </div>
                  </TableCell>
                  <TableCell>
                    <DirectionBadge direction={l.direction} />
                  </TableCell>
                  <TableCell>
                    <MethodBadge method={l.method} />
                  </TableCell>
                  <TableCell className="max-w-md font-mono text-xs">
                    <div className="truncate">
                      <span className="text-muted-foreground">{l.host}</span>
                      {l.path}
                      {l.query && <span className="text-muted-foreground">?{l.query}</span>}
                    </div>
                  </TableCell>
                  <TableCell>
                    <StatusBadge status={l.status} />
                  </TableCell>
                  <TableCell className={cn("text-right font-mono text-xs", l.latency_ms > 1000 && "text-destructive")}>
                    {l.latency_ms} ms
                  </TableCell>
                  <TableCell className="text-right font-mono text-xs text-muted-foreground">
                    {l.upstream_latency_ms} ms
                  </TableCell>
                  <TableCell className="text-right font-mono text-xs text-muted-foreground">
                    {l.upstream_ttfb_ms} ms
                  </TableCell>
                  <TableCell className="text-right font-mono text-xs">
                    {Math.max(0, l.latency_ms - l.upstream_latency_ms)} ms
                  </TableCell>
                  <TableCell>
                    <Badge variant="outline" className="font-mono text-[10px]">
                      {l.source}
                    </Badge>
                  </TableCell>
                  <TableCell className="font-mono text-[10px] text-muted-foreground">
                    {l.trace_id ? l.trace_id.slice(0, 12) : "—"}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      <LogDetailSheet
        projectId={projectId}
        eventId={selected?.event_id ?? null}
        initialEvent={selected ?? undefined}
        open={!!selected}
        onOpenChange={(v) => !v && setSelected(null)}
      />
    </>
  );
}
