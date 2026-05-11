"use client";

import { useEffect, useMemo, useState } from "react";
import { Activity, AlertCircle, BarChart3, Gauge, RefreshCw, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { TimeRangePicker, preset as toPreset, rangeToDates, type RangeValue } from "@/components/time-range";
import { MethodBadge } from "@/components/status-badge";
import {
  DistributionPie,
  ErrorRateChart,
  LatencyChart,
  VolumeChart,
} from "@/components/charts/analytics-charts";
import type { DistBucket, EndpointStat, TimeBucket } from "@/lib/api/types";
import { cn } from "@/lib/utils";

type APIKeyOption = { id: number; prefix: string; description: string };

const METHODS_FILTER = ["GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"];
const ALL_KEYS = "__all_keys";
const ANY_METHOD = "__any_method";
const ANY_HOST = "__any_host";
const ALL_DIRS = "all";
type DirSel = typeof ALL_DIRS | "inbound" | "outbound";

export function AnalyticsDashboard({
  projectId,
  apiKeys,
}: {
  projectId: string;
  apiKeys: APIKeyOption[];
}) {
  const [range, setRange] = useState<RangeValue>(toPreset("1h"));
  const [keyId, setKeyId] = useState<string>(ALL_KEYS);
  const [method, setMethod] = useState<string>(ANY_METHOD);
  const [host, setHost] = useState<string>(ANY_HOST);
  const [pathInput, setPathInput] = useState<string>("");
  const [pathFilter, setPathFilter] = useState<string>("");
  const [direction, setDirection] = useState<DirSel>(ALL_DIRS);
  const [refreshTick, setRefreshTick] = useState(0);

  const [series, setSeries] = useState<TimeBucket[]>([]);
  const [statusDist, setStatusDist] = useState<DistBucket[]>([]);
  const [methodDist, setMethodDist] = useState<DistBucket[]>([]);
  const [hostDist, setHostDist] = useState<DistBucket[]>([]);
  const [endpoints, setEndpoints] = useState<EndpointStat[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const denseHours = useMemo(() => {
    const { from, to } = rangeToDates(range);
    return to.getTime() - from.getTime() <= 6 * 60 * 60_000;
  }, [range]);

  useEffect(() => {
    let cancelled = false;
    const { from, to } = rangeToDates(range);
    const params = new URLSearchParams({ from: from.toISOString(), to: to.toISOString() });
    if (keyId !== ALL_KEYS) params.set("api_key_id", keyId);
    if (method !== ANY_METHOD) params.set("method", method);
    if (host !== ANY_HOST) params.set("host", host);
    if (pathFilter) params.set("path", pathFilter);
    if (direction !== ALL_DIRS) params.set("direction", direction);
    const qs = params.toString();

    // Host distribution should NOT be filtered by host (so the dropdown
    // always shows all available hosts for the current range/key/method).
    const hostParams = new URLSearchParams(params);
    hostParams.delete("host");
    const hostQS = hostParams.toString();

    setLoading(true);
    setError(null);

    Promise.all([
      fetch(`/api/projects/${projectId}/analytics/timeseries?${qs}`).then(asJson),
      fetch(`/api/projects/${projectId}/analytics/distribution?kind=status&${qs}`).then(asJson),
      fetch(`/api/projects/${projectId}/analytics/distribution?kind=method&${qs}`).then(asJson),
      fetch(`/api/projects/${projectId}/analytics/distribution?kind=host&${hostQS}`).then(asJson),
      fetch(`/api/projects/${projectId}/analytics/endpoints?limit=50&${qs}`).then(asJson),
    ])
      .then(([ts, statusD, methodD, hostD, ep]) => {
        if (cancelled) return;
        setSeries(ts as TimeBucket[]);
        setStatusDist(statusD as DistBucket[]);
        setMethodDist(methodD as DistBucket[]);
        setHostDist(hostD as DistBucket[]);
        setEndpoints(ep as EndpointStat[]);
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
  }, [projectId, range, keyId, method, host, pathFilter, direction, refreshTick]);

  const summary = useMemo(() => deriveSummary(series), [series]);

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center gap-2">
        <Tabs value={direction} onValueChange={(v) => setDirection(v as DirSel)}>
          <TabsList>
            <TabsTrigger value={ALL_DIRS}>All traffic</TabsTrigger>
            <TabsTrigger value="inbound">Inbound</TabsTrigger>
            <TabsTrigger value="outbound">Outbound</TabsTrigger>
          </TabsList>
        </Tabs>
        <TimeRangePicker value={range} onChange={setRange} />

        <Select value={keyId} onValueChange={setKeyId}>
          <SelectTrigger className="w-56">
            <SelectValue placeholder="All API keys" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL_KEYS}>All API keys</SelectItem>
            {apiKeys.map((k) => (
              <SelectItem key={k.id} value={String(k.id)}>
                {k.prefix}… {k.description ? `(${k.description})` : ""}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Select value={method} onValueChange={setMethod}>
          <SelectTrigger className="w-32">
            <SelectValue placeholder="Any method" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ANY_METHOD}>Any method</SelectItem>
            {METHODS_FILTER.map((m) => (
              <SelectItem key={m} value={m}>
                {m}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Select value={host} onValueChange={setHost}>
          <SelectTrigger className="w-56">
            <SelectValue placeholder="Any host" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ANY_HOST}>Any host</SelectItem>
            {hostDist.map((h) => (
              <SelectItem key={h.key} value={h.key}>
                {h.key}{" "}
                <span className="ml-1 text-xs text-muted-foreground">
                  ({Number(h.count).toLocaleString()})
                </span>
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <form
          className="flex items-center gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            setPathFilter(pathInput);
          }}
        >
          <Input
            placeholder="Path contains…"
            className="w-56"
            value={pathInput}
            onChange={(e) => setPathInput(e.target.value)}
          />
          {pathFilter && (
            <Button
              type="button"
              variant="ghost"
              size="icon"
              onClick={() => {
                setPathInput("");
                setPathFilter("");
              }}
            >
              <X />
            </Button>
          )}
        </form>

        <Button variant="outline" onClick={() => setRefreshTick((t) => t + 1)}>
          <RefreshCw className={cn(loading && "animate-spin")} />
          Refresh
        </Button>
      </div>

      {(host !== ANY_HOST || pathFilter) && (
        <div className="flex flex-wrap items-center gap-2 text-xs">
          <span className="text-muted-foreground">Active filters:</span>
          {host !== ANY_HOST && (
            <Badge variant="secondary" className="gap-1.5">
              host: <span className="font-mono">{host}</span>
              <button onClick={() => setHost(ANY_HOST)} className="ml-0.5 opacity-60 hover:opacity-100">
                <X className="h-3 w-3" />
              </button>
            </Badge>
          )}
          {pathFilter && (
            <Badge variant="secondary" className="gap-1.5">
              path: <span className="font-mono">{pathFilter}</span>
              <button
                onClick={() => {
                  setPathFilter("");
                  setPathInput("");
                }}
                className="ml-0.5 opacity-60 hover:opacity-100"
              >
                <X className="h-3 w-3" />
              </button>
            </Badge>
          )}
        </div>
      )}

      {error && (
        <div className="rounded-md border border-destructive/30 bg-destructive/5 p-4 text-sm text-destructive">
          {error}
        </div>
      )}

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <KPICard
          icon={BarChart3}
          label="Requests"
          value={summary.requests.toLocaleString()}
          sub={`${summary.rps.toFixed(1)} req/s`}
        />
        <KPICard
          icon={AlertCircle}
          label="Error rate"
          value={`${summary.errorRate.toFixed(2)}%`}
          sub={`${summary.errors.toLocaleString()} errors`}
          tone={summary.errorRate > 5 ? "destructive" : summary.errorRate > 1 ? "warning" : "default"}
        />
        <KPICard
          icon={Gauge}
          label="Latency p95"
          value={`${Math.round(summary.p95)} ms`}
          sub={`p99 ${Math.round(summary.p99)} ms`}
        />
        <KPICard
          icon={Activity}
          label="Latency p50"
          value={`${Math.round(summary.p50)} ms`}
          sub={`max ${summary.max} ms`}
        />
      </div>

      <Card>
        <CardHeader className="flex-row items-center justify-between">
          <CardTitle>Request volume</CardTitle>
          <CardSubtitle loading={loading} count={series.length} unit="buckets" />
        </CardHeader>
        <CardContent>
          {loading ? <Skeleton className="h-72 w-full" /> : <VolumeChart data={series} denseHours={denseHours} />}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Latency percentiles</CardTitle>
        </CardHeader>
        <CardContent>
          {loading ? <Skeleton className="h-72 w-full" /> : <LatencyChart data={series} denseHours={denseHours} />}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Error rate</CardTitle>
        </CardHeader>
        <CardContent>
          {loading ? <Skeleton className="h-56 w-full" /> : <ErrorRateChart data={series} denseHours={denseHours} />}
        </CardContent>
      </Card>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>Status code distribution</CardTitle>
          </CardHeader>
          <CardContent>
            {loading ? <Skeleton className="h-72 w-full" /> : <DistributionPie data={statusDist} palette="status" />}
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>Method distribution</CardTitle>
          </CardHeader>
          <CardContent>
            {loading ? <Skeleton className="h-72 w-full" /> : <DistributionPie data={methodDist} palette="method" />}
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Endpoints</CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          <EndpointsTabs endpoints={endpoints} loading={loading} />
        </CardContent>
      </Card>
    </div>
  );
}

function EndpointsTabs({ endpoints, loading }: { endpoints: EndpointStat[]; loading: boolean }) {
  const sorted = useMemo(
    () => ({
      most: [...endpoints].sort((a, b) => b.requests - a.requests),
      slowest: [...endpoints].sort((a, b) => b.p99 - a.p99),
      errors: [...endpoints].filter((e) => e.errors > 0).sort((a, b) => b.error_rate - a.error_rate),
    }),
    [endpoints],
  );

  return (
    <Tabs defaultValue="most" className="w-full">
      <div className="border-b px-4 pt-3">
        <TabsList>
          <TabsTrigger value="most">Most traffic</TabsTrigger>
          <TabsTrigger value="slowest">Slowest (p99)</TabsTrigger>
          <TabsTrigger value="errors">Highest error rate</TabsTrigger>
        </TabsList>
      </div>
      <TabsContent value="most" className="m-0">
        <EndpointTable rows={sorted.most} loading={loading} />
      </TabsContent>
      <TabsContent value="slowest" className="m-0">
        <EndpointTable rows={sorted.slowest} loading={loading} />
      </TabsContent>
      <TabsContent value="errors" className="m-0">
        <EndpointTable rows={sorted.errors} loading={loading} emptyHint="No 4xx/5xx in this range." />
      </TabsContent>
    </Tabs>
  );
}

function EndpointTable({
  rows,
  loading,
  emptyHint,
}: {
  rows: EndpointStat[];
  loading: boolean;
  emptyHint?: string;
}) {
  if (loading) {
    return (
      <div className="space-y-2 p-4">
        {Array.from({ length: 5 }).map((_, i) => (
          <Skeleton key={i} className="h-8 w-full" />
        ))}
      </div>
    );
  }
  if (rows.length === 0) {
    return <div className="p-8 text-center text-sm text-muted-foreground">{emptyHint ?? "No endpoints in this range."}</div>;
  }
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead className="w-24">Method</TableHead>
          <TableHead>Path</TableHead>
          <TableHead className="text-right">Requests</TableHead>
          <TableHead className="text-right">Errors</TableHead>
          <TableHead className="text-right">Error rate</TableHead>
          <TableHead className="text-right">p50</TableHead>
          <TableHead className="text-right">p95</TableHead>
          <TableHead className="text-right">p99</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.slice(0, 50).map((r, i) => (
          <TableRow key={`${r.method}:${r.path}:${i}`}>
            <TableCell>
              <MethodBadge method={r.method} />
            </TableCell>
            <TableCell className="max-w-md truncate font-mono text-xs">{r.path}</TableCell>
            <TableCell className="text-right">{Number(r.requests).toLocaleString()}</TableCell>
            <TableCell className="text-right">{Number(r.errors).toLocaleString()}</TableCell>
            <TableCell className="text-right">
              {r.errors > 0 ? (
                <Badge variant={r.error_rate > 0.05 ? "destructive" : "warning"}>
                  {(r.error_rate * 100).toFixed(2)}%
                </Badge>
              ) : (
                <span className="text-xs text-muted-foreground">—</span>
              )}
            </TableCell>
            <TableCell className="text-right">{Math.round(r.p50)} ms</TableCell>
            <TableCell className="text-right">{Math.round(r.p95)} ms</TableCell>
            <TableCell className={cn("text-right", r.p99 > 1000 && "text-destructive")}>
              {Math.round(r.p99)} ms
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

function KPICard({
  icon: Icon,
  label,
  value,
  sub,
  tone,
}: {
  icon: React.ElementType;
  label: string;
  value: string;
  sub?: string;
  tone?: "default" | "warning" | "destructive";
}) {
  const accent =
    tone === "destructive" ? "text-destructive" : tone === "warning" ? "text-warning" : "text-foreground";
  return (
    <Card>
      <CardContent className="p-5">
        <div className="flex items-center justify-between text-xs uppercase tracking-wider text-muted-foreground">
          {label}
          <Icon className="h-4 w-4" />
        </div>
        <div className={cn("mt-2 text-2xl font-semibold tabular-nums", accent)}>{value}</div>
        {sub && <div className="mt-0.5 text-xs text-muted-foreground">{sub}</div>}
      </CardContent>
    </Card>
  );
}

function CardSubtitle({ loading, count, unit }: { loading: boolean; count: number; unit: string }) {
  return (
    <div className="text-xs text-muted-foreground">
      {loading ? "Loading…" : `${count} ${unit}`}
    </div>
  );
}

function deriveSummary(series: TimeBucket[]) {
  if (series.length === 0) {
    return { requests: 0, errors: 0, errorRate: 0, p50: 0, p95: 0, p99: 0, max: 0, rps: 0 };
  }
  let total = 0;
  let errs = 0;
  let max = 0;
  let p50Sum = 0;
  let p95Sum = 0;
  let p99Sum = 0;
  for (const b of series) {
    const ok = Number(b.ok || 0);
    const e4 = Number(b.err_4xx || 0);
    const e5 = Number(b.err_5xx || 0);
    total += ok + e4 + e5;
    errs += e4 + e5;
    if (Number(b.max) > max) max = Number(b.max);
    p50Sum += Number(b.p50 || 0);
    p95Sum += Number(b.p95 || 0);
    p99Sum += Number(b.p99 || 0);
  }
  const first = new Date(series[0].bucket).getTime();
  const last = new Date(series[series.length - 1].bucket).getTime();
  const seconds = Math.max((last - first) / 1000, 1);
  return {
    requests: total,
    errors: errs,
    errorRate: total ? (errs / total) * 100 : 0,
    p50: p50Sum / series.length,
    p95: p95Sum / series.length,
    p99: p99Sum / series.length,
    max,
    rps: total / seconds,
  };
}

async function asJson(r: Response): Promise<unknown> {
  if (!r.ok) {
    const t = await r.text().catch(() => "");
    throw new Error(`${r.status}: ${t || r.statusText}`);
  }
  return r.json();
}
