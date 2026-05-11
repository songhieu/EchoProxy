"use client";

import { useEffect, useRef, useState } from "react";
import { format } from "date-fns";
import { Pause, Play, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { LogDetailSheet } from "@/components/log-detail-sheet";
import { DirectionBadge, MethodBadge, StatusBadge } from "@/components/status-badge";
import { cn } from "@/lib/utils";
import type { LogEvent } from "@/lib/api/types";

const MAX_EVENTS = 200;
const POLL_MS = 2000;
const ALL_DIRS = "all";
type Dir = typeof ALL_DIRS | "inbound" | "outbound";

export function LiveTail({ projectId }: { projectId: string }) {
  const [events, setEvents] = useState<LogEvent[]>([]);
  const [paused, setPaused] = useState(false);
  const [direction, setDirection] = useState<Dir>(ALL_DIRS);
  const [selected, setSelected] = useState<LogEvent | null>(null);
  const [error, setError] = useState<string | null>(null);
  const seen = useRef<Set<string>>(new Set());

  useEffect(() => {
    // Reset the de-dup window so changing the filter re-fetches without leaks.
    seen.current.clear();
    setEvents([]);
  }, [direction]);

  useEffect(() => {
    if (paused) return;
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout> | null = null;

    const poll = async () => {
      try {
        const params = new URLSearchParams({ limit: "50" });
        if (direction !== ALL_DIRS) params.set("direction", direction);
        const res = await fetch(`/api/projects/${projectId}/logs?${params.toString()}`);
        if (!res.ok) throw new Error(`status ${res.status}`);
        const data: LogEvent[] = await res.json();
        if (cancelled) return;
        setError(null);
        setEvents((prev) => {
          const fresh: LogEvent[] = [];
          for (const e of data) {
            if (!seen.current.has(e.event_id)) {
              seen.current.add(e.event_id);
              fresh.push(e);
            }
          }
          if (fresh.length === 0) return prev;
          const merged = [...fresh, ...prev].slice(0, MAX_EVENTS);
          if (seen.current.size > MAX_EVENTS * 2) {
            seen.current = new Set(merged.map((e) => e.event_id));
          }
          return merged;
        });
      } catch (e) {
        if (!cancelled) setError((e as Error).message);
      } finally {
        if (!cancelled) timer = setTimeout(poll, POLL_MS);
      }
    };
    poll();
    return () => {
      cancelled = true;
      if (timer) clearTimeout(timer);
    };
  }, [paused, projectId, direction]);

  return (
    <>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-wrap items-center gap-3">
          <Tabs value={direction} onValueChange={(v) => setDirection(v as Dir)}>
            <TabsList>
              <TabsTrigger value={ALL_DIRS}>All</TabsTrigger>
              <TabsTrigger value="inbound">Inbound</TabsTrigger>
              <TabsTrigger value="outbound">Outbound</TabsTrigger>
            </TabsList>
          </Tabs>
          <Button variant={paused ? "default" : "outline"} onClick={() => setPaused((p) => !p)}>
            {paused ? <Play /> : <Pause />}
            {paused ? "Resume" : "Pause"}
          </Button>
          <Button
            variant="ghost"
            onClick={() => {
              setEvents([]);
              seen.current.clear();
            }}
          >
            <Trash2 /> Clear
          </Button>
          <Badge variant={paused ? "secondary" : "success"} className="gap-1.5">
            <span
              className={cn(
                "h-1.5 w-1.5 rounded-full",
                paused ? "bg-muted-foreground" : "animate-pulse bg-success-foreground",
              )}
            />
            {paused ? "Paused" : "Live"}
          </Badge>
          <span className="text-xs text-muted-foreground">
            {events.length} / {MAX_EVENTS}
          </span>
        </div>
        {error && <span className="text-xs text-destructive">{error}</span>}
      </div>

      <Card>
        <CardContent className="p-0">
          {events.length === 0 ? (
            <div className="p-12 text-center text-sm text-muted-foreground">
              {paused ? "Paused. Resume to start streaming." : "Waiting for events…"}
            </div>
          ) : (
            <div className="divide-y">
              {events.map((e) => (
                <button
                  key={e.event_id}
                  onClick={() => setSelected(e)}
                  className="flex w-full items-center gap-3 px-4 py-2 text-left text-sm hover:bg-muted/50"
                >
                  <span className="font-mono text-xs text-muted-foreground">
                    {format(new Date(e.ts), "HH:mm:ss.SSS")}
                  </span>
                  <DirectionBadge direction={e.direction} />
                  <MethodBadge method={e.method} />
                  <span className="flex-1 truncate font-mono text-xs">
                    <span className="text-muted-foreground">{e.host}</span>
                    {e.path}
                  </span>
                  <StatusBadge status={e.status} />
                  <span className={cn("w-20 text-right text-xs", e.latency_ms > 1000 && "text-destructive")}>
                    {e.latency_ms} ms
                  </span>
                  <Badge variant="outline" className="font-mono text-[10px]">
                    {e.source}
                  </Badge>
                </button>
              ))}
            </div>
          )}
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
