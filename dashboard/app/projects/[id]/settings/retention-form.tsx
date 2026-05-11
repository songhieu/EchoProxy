"use client";

import { useState, useTransition } from "react";
import { Save } from "lucide-react";
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
import { setRetention } from "./actions";

const PRESETS = [
  { value: "1",  label: "1 day"   },
  { value: "7",  label: "7 days"  },
  { value: "14", label: "14 days" },
  { value: "30", label: "30 days (default)" },
  { value: "60", label: "60 days" },
  { value: "90", label: "90 days (cap)" },
  { value: "custom", label: "Custom..." },
];

export function RetentionForm({
  projectId,
  initial,
}: {
  projectId: number;
  initial: number;
}) {
  const matchedPreset = PRESETS.find((p) => p.value === String(initial))?.value ?? "custom";
  const [preset, setPreset] = useState<string>(matchedPreset);
  const [custom, setCustom] = useState<string>(String(initial));
  const [feedback, setFeedback] = useState<string | null>(null);
  const [pending, start] = useTransition();

  const days = preset === "custom" ? Number(custom) : Number(preset);
  const isValid = Number.isInteger(days) && days >= 1 && days <= 90;
  const dirty = days !== initial;

  const submit = () => {
    setFeedback(null);
    start(async () => {
      const res = await setRetention(projectId, days);
      setFeedback(res.ok ? `Saved. Cleanup will enforce ${days}d on next run.` : res.error ?? "Failed");
    });
  };

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-end gap-3">
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="retention-preset" className="text-xs">
            Retention
          </Label>
          <Select value={preset} onValueChange={setPreset}>
            <SelectTrigger id="retention-preset" className="w-56">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {PRESETS.map((p) => (
                <SelectItem key={p.value} value={p.value}>
                  {p.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        {preset === "custom" && (
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="retention-custom" className="text-xs">
              Days (1-90)
            </Label>
            <Input
              id="retention-custom"
              type="number"
              min={1}
              max={90}
              value={custom}
              onChange={(e) => setCustom(e.target.value)}
              className="w-24"
            />
          </div>
        )}
        <Button onClick={submit} disabled={!isValid || !dirty || pending}>
          <Save /> {pending ? "Saving..." : "Save"}
        </Button>
      </div>
      {feedback && (
        <p className="text-xs text-muted-foreground">{feedback}</p>
      )}
    </div>
  );
}
