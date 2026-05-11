"use client";

import { useEffect, useState } from "react";
import { Calendar } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Separator } from "@/components/ui/separator";

// ── Types ────────────────────────────────────────────────────────────────────

export type PresetKey = "15m" | "1h" | "6h" | "24h" | "7d" | "30d";

/** Discriminated union: either a sliding preset or a frozen absolute window. */
export type RangeValue =
  | { kind: "preset"; preset: PresetKey }
  | { kind: "custom"; from: Date; to: Date };

const PRESETS: { value: PresetKey; label: string }[] = [
  { value: "15m", label: "Last 15 minutes" },
  { value: "1h",  label: "Last hour" },
  { value: "6h",  label: "Last 6 hours" },
  { value: "24h", label: "Last 24 hours" },
  { value: "7d",  label: "Last 7 days" },
  { value: "30d", label: "Last 30 days" },
];

const PRESET_MS: Record<PresetKey, number> = {
  "15m": 15 * 60_000,
  "1h":  60 * 60_000,
  "6h":  6 * 60 * 60_000,
  "24h": 24 * 60 * 60_000,
  "7d":  7 * 24 * 60 * 60_000,
  "30d": 30 * 24 * 60 * 60_000,
};

const PRESET_LABEL: Record<PresetKey, string> = Object.fromEntries(
  PRESETS.map((p) => [p.value, p.label]),
) as Record<PresetKey, string>;

// ── Public helpers ───────────────────────────────────────────────────────────

/** Materialise a RangeValue to absolute Dates. Presets re-evaluate to "now"
 *  every call (sliding); custom values pass through. */
export function rangeToDates(v: RangeValue): { from: Date; to: Date } {
  if (v.kind === "custom") return { from: v.from, to: v.to };
  const to = new Date();
  return { from: new Date(to.getTime() - PRESET_MS[v.preset]), to };
}

export function preset(preset: PresetKey): RangeValue {
  return { kind: "preset", preset };
}

export function custom(from: Date, to: Date): RangeValue {
  return { kind: "custom", from, to };
}

/** Render label like "Last hour" or "May 11 12:00 → May 11 18:30". */
export function rangeLabel(v: RangeValue): string {
  if (v.kind === "preset") return PRESET_LABEL[v.preset];
  return `${fmtShort(v.from)} → ${fmtShort(v.to)}`;
}

// Back-compat alias for the old string-based API (analytics-dashboard etc.).
export type Range = PresetKey;

// ── Picker UI ────────────────────────────────────────────────────────────────

export function TimeRangePicker({
  value,
  onChange,
}: {
  value: RangeValue;
  onChange: (v: RangeValue) => void;
}) {
  const [open, setOpen] = useState(false);
  const isCustom = value.kind === "custom";

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button variant="outline" className="font-normal">
          <Calendar /> {rangeLabel(value)}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-72 p-2">
        <div className="flex flex-col">
          {PRESETS.map((p) => (
            <button
              key={p.value}
              type="button"
              onClick={() => {
                onChange(preset(p.value));
                setOpen(false);
              }}
              className={
                "rounded-sm px-2 py-1.5 text-left text-sm hover:bg-accent " +
                (value.kind === "preset" && value.preset === p.value ? "bg-accent" : "")
              }
            >
              {p.label}
            </button>
          ))}
          <Separator className="my-2" />
          <CustomRangeForm
            initial={isCustom ? value : null}
            onApply={(from, to) => {
              onChange(custom(from, to));
              setOpen(false);
            }}
          />
        </div>
      </PopoverContent>
    </Popover>
  );
}

function CustomRangeForm({
  initial,
  onApply,
}: {
  initial: { from: Date; to: Date } | null;
  onApply: (from: Date, to: Date) => void;
}) {
  // Default to the last hour if no custom range yet.
  const now = new Date();
  const defFrom = initial?.from ?? new Date(now.getTime() - 3600_000);
  const defTo = initial?.to ?? now;
  const [from, setFrom] = useState<string>(toLocalInput(defFrom));
  const [to, setTo] = useState<string>(toLocalInput(defTo));
  const [err, setErr] = useState<string | null>(null);

  // Reset inputs whenever the underlying initial changes (e.g. switching tabs).
  useEffect(() => {
    setFrom(toLocalInput(defFrom));
    setTo(toLocalInput(defTo));
    setErr(null);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [initial?.from?.getTime(), initial?.to?.getTime()]);

  const submit = () => {
    const f = fromLocalInput(from);
    const t = fromLocalInput(to);
    if (!f || !t) return setErr("Invalid date");
    if (f >= t) return setErr("From must be before To");
    onApply(f, t);
  };

  return (
    <div className="space-y-2 px-1">
      <div className="text-xs font-medium text-muted-foreground">Custom range</div>
      <div className="space-y-1.5">
        <Label htmlFor="trp-from" className="text-[11px]">
          From
        </Label>
        <Input
          id="trp-from"
          type="datetime-local"
          step={60}
          value={from}
          onChange={(e) => setFrom(e.target.value)}
        />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="trp-to" className="text-[11px]">
          To
        </Label>
        <Input
          id="trp-to"
          type="datetime-local"
          step={60}
          value={to}
          onChange={(e) => setTo(e.target.value)}
        />
      </div>
      {err && <p className="text-[11px] text-destructive">{err}</p>}
      <Button size="sm" className="w-full" onClick={submit}>
        Apply
      </Button>
    </div>
  );
}

// ── Format helpers ───────────────────────────────────────────────────────────

function pad(n: number): string {
  return n < 10 ? `0${n}` : String(n);
}

/** Format Date → "2026-05-11T15:34" for <input type="datetime-local">. */
function toLocalInput(d: Date): string {
  return (
    `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}` +
    `T${pad(d.getHours())}:${pad(d.getMinutes())}`
  );
}

/** Inverse of toLocalInput. Returns null for malformed input. */
function fromLocalInput(s: string): Date | null {
  if (!s) return null;
  const d = new Date(s);
  return isNaN(d.getTime()) ? null : d;
}

/** Short label "May 11 15:34". */
function fmtShort(d: Date): string {
  const m = ["Jan","Feb","Mar","Apr","May","Jun","Jul","Aug","Sep","Oct","Nov","Dec"][d.getMonth()];
  return `${m} ${d.getDate()} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}
