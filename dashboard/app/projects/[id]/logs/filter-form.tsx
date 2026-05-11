"use client";

import { useState } from "react";
import { useRouter, usePathname } from "next/navigation";
import { Filter, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Card, CardContent } from "@/components/ui/card";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  TimeRangePicker,
  preset as toPreset,
  custom as toCustom,
  type RangeValue,
  type PresetKey,
} from "@/components/time-range";

const ANY = "__any";
const ALL_DIRS = "all";
type Dir = typeof ALL_DIRS | "inbound" | "outbound";
const METHODS = ["GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"];
const PRESET_KEYS: PresetKey[] = ["15m", "1h", "6h", "24h", "7d", "30d"];

function initialRange(defaults: { range?: string; from?: string; to?: string }): RangeValue {
  if (defaults.from && defaults.to) {
    return toCustom(new Date(defaults.from), new Date(defaults.to));
  }
  const p = (defaults.range && (PRESET_KEYS as string[]).includes(defaults.range)
    ? defaults.range
    : "1h") as PresetKey;
  return toPreset(p);
}

export function LogsFilter({
  defaults,
}: {
  defaults: {
    method?: string;
    status?: string;
    path?: string;
    direction?: string;
    range?: string;
    from?: string;
    to?: string;
  };
}) {
  const router = useRouter();
  const pathname = usePathname();

  const [method, setMethod] = useState<string>(defaults.method || ANY);
  const [status, setStatus] = useState<string>(defaults.status ?? "");
  const [path, setPath] = useState<string>(defaults.path ?? "");
  const [direction, setDirection] = useState<Dir>(
    defaults.direction === "inbound" || defaults.direction === "outbound"
      ? (defaults.direction as Dir)
      : ALL_DIRS,
  );
  const [range, setRange] = useState<RangeValue>(() => initialRange(defaults));

  const hasFilters = Boolean(
    defaults.method || defaults.status || defaults.path || defaults.direction
      || defaults.from || defaults.to
      || (defaults.range && defaults.range !== "1h"),
  );

  const buildParams = (overrides: Partial<{ direction: Dir; range: RangeValue }> = {}): URLSearchParams => {
    const params = new URLSearchParams();
    if (method && method !== ANY) params.set("method", method);
    if (status) params.set("status", status);
    if (path) params.set("path", path);
    const dir = overrides.direction ?? direction;
    if (dir !== ALL_DIRS) params.set("direction", dir);
    const r = overrides.range ?? range;
    if (r.kind === "custom") {
      params.set("from", r.from.toISOString());
      params.set("to", r.to.toISOString());
    } else if (r.preset !== "1h") {
      params.set("range", r.preset);
    }
    return params;
  };

  const push = (params: URLSearchParams) => {
    const qs = params.toString();
    router.push(qs ? `${pathname}?${qs}` : (pathname ?? "/"));
  };

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    push(buildParams());
  };

  const onDirectionChange = (v: string) => {
    const dir = v as Dir;
    setDirection(dir);
    push(buildParams({ direction: dir }));
  };

  const onRangeChange = (v: RangeValue) => {
    setRange(v);
    push(buildParams({ range: v }));
  };

  const clear = () => {
    setMethod(ANY);
    setStatus("");
    setPath("");
    setDirection(ALL_DIRS);
    setRange(toPreset("1h"));
    router.push(pathname ?? "/");
  };

  return (
    <Card>
      <CardContent className="space-y-3 p-4">
        <div className="flex items-center justify-between gap-3">
          <Tabs value={direction} onValueChange={onDirectionChange}>
            <TabsList>
              <TabsTrigger value={ALL_DIRS}>All traffic</TabsTrigger>
              <TabsTrigger value="inbound">Inbound</TabsTrigger>
              <TabsTrigger value="outbound">Outbound</TabsTrigger>
            </TabsList>
          </Tabs>
          <TimeRangePicker value={range} onChange={onRangeChange} />
        </div>
        <form className="flex flex-wrap items-end gap-3" onSubmit={submit}>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="method-filter" className="text-xs">
              Method
            </Label>
            <Select value={method} onValueChange={setMethod}>
              <SelectTrigger id="method-filter" className="w-32">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ANY}>Any</SelectItem>
                {METHODS.map((m) => (
                  <SelectItem key={m} value={m}>
                    {m}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="flex flex-col gap-1.5">
            <Label htmlFor="status-filter" className="text-xs">
              Status
            </Label>
            <Input
              id="status-filter"
              type="number"
              placeholder="200"
              className="w-28"
              value={status}
              onChange={(e) => setStatus(e.target.value)}
            />
          </div>

          <div className="flex min-w-[16rem] flex-1 flex-col gap-1.5">
            <Label htmlFor="path-filter" className="text-xs">
              Path contains
            </Label>
            <Input
              id="path-filter"
              placeholder="/api/v1/users"
              value={path}
              onChange={(e) => setPath(e.target.value)}
            />
          </div>

          <Button type="submit">
            <Filter /> Apply
          </Button>
          {hasFilters && (
            <Button type="button" variant="ghost" onClick={clear}>
              <X /> Clear
            </Button>
          )}
        </form>
      </CardContent>
    </Card>
  );
}
