"use client";

import { format } from "date-fns";
import {
  Area,
  AreaChart,
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  ComposedChart,
  Legend,
  Line,
  LineChart,
  Pie,
  PieChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import type { DistBucket, TimeBucket } from "@/lib/api/types";

const TOOLTIP_STYLE = {
  background: "hsl(var(--popover))",
  border: "1px solid hsl(var(--border))",
  borderRadius: 6,
  fontSize: 12,
};

function bucketLabel(v: string, denseHours: boolean): string {
  const d = new Date(v);
  return denseHours ? format(d, "HH:mm") : format(d, "MM-dd HH:mm");
}

export function VolumeChart({ data, denseHours }: { data: TimeBucket[]; denseHours: boolean }) {
  if (data.length === 0) return <Empty>No traffic in this range.</Empty>;
  return (
    <div className="h-72 w-full">
      <ResponsiveContainer>
        <BarChart data={data} margin={{ top: 8, right: 8, left: 0, bottom: 0 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="hsl(var(--border))" />
          <XAxis
            dataKey="bucket"
            tickFormatter={(v) => bucketLabel(v as string, denseHours)}
            stroke="hsl(var(--muted-foreground))"
            fontSize={11}
          />
          <YAxis stroke="hsl(var(--muted-foreground))" fontSize={11} allowDecimals={false} />
          <Tooltip
            contentStyle={TOOLTIP_STYLE}
            labelFormatter={(v) => format(new Date(v as string), "yyyy-MM-dd HH:mm")}
          />
          <Legend wrapperStyle={{ fontSize: 12 }} />
          <Bar dataKey="ok" name="2xx-3xx" stackId="s" fill="hsl(var(--success))" />
          <Bar dataKey="err_4xx" name="4xx" stackId="s" fill="hsl(var(--warning))" />
          <Bar dataKey="err_5xx" name="5xx" stackId="s" fill="hsl(var(--destructive))" />
        </BarChart>
      </ResponsiveContainer>
    </div>
  );
}

export function LatencyChart({ data, denseHours }: { data: TimeBucket[]; denseHours: boolean }) {
  if (data.length === 0) return <Empty>No latency data in this range.</Empty>;
  return (
    <div className="h-72 w-full">
      <ResponsiveContainer>
        <ComposedChart data={data} margin={{ top: 8, right: 8, left: 0, bottom: 0 }}>
          <defs>
            <linearGradient id="p99g" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor="hsl(var(--destructive))" stopOpacity={0.4} />
              <stop offset="100%" stopColor="hsl(var(--destructive))" stopOpacity={0} />
            </linearGradient>
            <linearGradient id="p95g" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor="hsl(var(--warning))" stopOpacity={0.4} />
              <stop offset="100%" stopColor="hsl(var(--warning))" stopOpacity={0} />
            </linearGradient>
            <linearGradient id="p50g" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor="hsl(var(--primary))" stopOpacity={0.5} />
              <stop offset="100%" stopColor="hsl(var(--primary))" stopOpacity={0} />
            </linearGradient>
          </defs>
          <CartesianGrid strokeDasharray="3 3" stroke="hsl(var(--border))" />
          <XAxis
            dataKey="bucket"
            tickFormatter={(v) => bucketLabel(v as string, denseHours)}
            stroke="hsl(var(--muted-foreground))"
            fontSize={11}
          />
          <YAxis stroke="hsl(var(--muted-foreground))" fontSize={11} unit=" ms" />
          <Tooltip
            contentStyle={TOOLTIP_STYLE}
            labelFormatter={(v) => format(new Date(v as string), "yyyy-MM-dd HH:mm")}
          />
          <Legend wrapperStyle={{ fontSize: 12 }} />
          <Area type="monotone" dataKey="p99" name="total p99" stroke="hsl(var(--destructive))" fill="url(#p99g)" strokeWidth={2} />
          <Area type="monotone" dataKey="p95" name="total p95" stroke="hsl(var(--warning))" fill="url(#p95g)" strokeWidth={2} />
          <Area type="monotone" dataKey="p50" name="total p50" stroke="hsl(var(--primary))" fill="url(#p50g)" strokeWidth={2} />
          <Line type="monotone" dataKey="upstream_p99" name="upstream p99" stroke="#3b82f6" strokeDasharray="4 2" strokeWidth={1.5} dot={false} />
          <Line type="monotone" dataKey="overhead_p99" name="proxy overhead p99" stroke="#f59e0b" strokeDasharray="4 2" strokeWidth={1.5} dot={false} />
        </ComposedChart>
      </ResponsiveContainer>
    </div>
  );
}

export function ErrorRateChart({ data, denseHours }: { data: TimeBucket[]; denseHours: boolean }) {
  if (data.length === 0) return <Empty>No data.</Empty>;
  const series = data.map((b) => {
    const total = b.ok + b.err_4xx + b.err_5xx;
    return { bucket: b.bucket, error_rate: total ? ((b.err_4xx + b.err_5xx) / total) * 100 : 0 };
  });
  return (
    <div className="h-56 w-full">
      <ResponsiveContainer>
        <LineChart data={series} margin={{ top: 8, right: 8, left: 0, bottom: 0 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="hsl(var(--border))" />
          <XAxis
            dataKey="bucket"
            tickFormatter={(v) => bucketLabel(v as string, denseHours)}
            stroke="hsl(var(--muted-foreground))"
            fontSize={11}
          />
          <YAxis stroke="hsl(var(--muted-foreground))" fontSize={11} unit="%" domain={[0, "auto"]} />
          <Tooltip
            contentStyle={TOOLTIP_STYLE}
            labelFormatter={(v) => format(new Date(v as string), "yyyy-MM-dd HH:mm")}
            formatter={(v: number) => [`${v.toFixed(2)}%`, "Error rate"]}
          />
          <Line type="monotone" dataKey="error_rate" stroke="hsl(var(--destructive))" strokeWidth={2} dot={false} />
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
}

const STATUS_COLORS: Record<string, string> = {
  "2xx": "hsl(var(--success))",
  "3xx": "hsl(var(--primary))",
  "4xx": "hsl(var(--warning))",
  "5xx": "hsl(var(--destructive))",
  "0xx": "hsl(var(--muted-foreground))",
};

const METHOD_COLORS = [
  "hsl(var(--primary))",
  "hsl(var(--success))",
  "hsl(var(--warning))",
  "hsl(var(--destructive))",
  "hsl(var(--muted-foreground))",
  "hsl(220 70% 50%)",
  "hsl(280 60% 50%)",
  "hsl(160 60% 40%)",
];

export function DistributionPie({ data, palette }: { data: DistBucket[]; palette: "status" | "method" }) {
  if (data.length === 0) return <Empty>No data.</Empty>;
  const total = data.reduce((a, d) => a + Number(d.count || 0), 0);
  return (
    <div className="h-72 w-full">
      <ResponsiveContainer>
        <PieChart>
          <Pie
            data={data}
            dataKey="count"
            nameKey="key"
            innerRadius={50}
            outerRadius={90}
            paddingAngle={2}
            stroke="none"
            label={({ key, count }) => `${key} (${((Number(count) / total) * 100).toFixed(1)}%)`}
            labelLine={false}
          >
            {data.map((d, i) => (
              <Cell
                key={d.key}
                fill={
                  palette === "status"
                    ? STATUS_COLORS[d.key] ?? "hsl(var(--muted-foreground))"
                    : METHOD_COLORS[i % METHOD_COLORS.length]
                }
              />
            ))}
          </Pie>
          <Tooltip
            contentStyle={TOOLTIP_STYLE}
            formatter={(v: number, _n, p) => [Number(v).toLocaleString(), p.payload.key]}
          />
        </PieChart>
      </ResponsiveContainer>
    </div>
  );
}

function Empty({ children }: { children: React.ReactNode }) {
  return <div className="flex h-72 items-center justify-center text-sm text-muted-foreground">{children}</div>;
}
