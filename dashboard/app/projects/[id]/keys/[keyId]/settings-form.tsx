"use client";

import { useState, useTransition } from "react";
import { Loader2, Save, ShieldCheck } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Badge } from "@/components/ui/badge";
import { Separator } from "@/components/ui/separator";
import type { APIKey } from "@/lib/api/types";
import { updateKeyAction } from "./actions";

export function KeySettingsForm({ projectId, apiKey }: { projectId: string; apiKey: APIKey }) {
  const [pending, start] = useTransition();
  const [allowlist, setAllowlist] = useState(apiKey.allowlist.join(", "));
  const [bodyCap, setBodyCap] = useState(String(apiKey.body_cap || 0));
  const [rateLimit, setRateLimit] = useState(String(apiKey.rate_limit_rps || 0));
  const [description, setDescription] = useState(apiKey.description || "");
  const [headerDeny, setHeaderDeny] = useState(
    (apiKey.redact_rules?.header_denylist ?? []).join(", "),
  );
  const [jsonDeny, setJsonDeny] = useState(
    (apiKey.redact_rules?.json_field_denylist ?? []).join(", "),
  );
  const [disableDefaults, setDisableDefaults] = useState(
    Boolean(apiKey.redact_rules?.disable_defaults),
  );

  const submit = () => {
    const split = (s: string) =>
      s
        .split(",")
        .map((x) => x.trim())
        .filter(Boolean);

    start(async () => {
      const res = await updateKeyAction(projectId, String(apiKey.id), {
        allowlist: split(allowlist),
        body_cap: Number(bodyCap) || 0,
        rate_limit_rps: Number(rateLimit) || 0,
        description,
        redact_rules: {
          header_denylist: split(headerDeny),
          json_field_denylist: split(jsonDeny),
          disable_defaults: disableDefaults,
        },
      });
      if (!res.ok) {
        toast.error(res.error);
        return;
      }
      toast.success("Key updated");
    });
  };

  return (
    <div className="grid gap-6 lg:grid-cols-3">
      <Card className="lg:col-span-2">
        <CardHeader>
          <CardTitle>General</CardTitle>
          <CardDescription>Identity and traffic controls.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-5">
          <div className="space-y-2">
            <Label htmlFor="description">Description</Label>
            <Input
              id="description"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="Production checkout API"
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="allowlist">Upstream allowlist</Label>
            <Textarea
              id="allowlist"
              rows={2}
              value={allowlist}
              onChange={(e) => setAllowlist(e.target.value)}
              placeholder="api.example.com, api.payments.example.com"
            />
            <p className="text-xs text-muted-foreground">
              Comma-separated hostnames. Empty list allows all hosts (dev only).
            </p>
          </div>

          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="body_cap">Body capture cap (bytes)</Label>
              <Input
                id="body_cap"
                type="number"
                min={0}
                value={bodyCap}
                onChange={(e) => setBodyCap(e.target.value)}
              />
              <p className="text-xs text-muted-foreground">0 uses the proxy default (64 KB).</p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="rate">Rate limit (requests / second)</Label>
              <Input
                id="rate"
                type="number"
                min={0}
                value={rateLimit}
                onChange={(e) => setRateLimit(e.target.value)}
              />
              <p className="text-xs text-muted-foreground">0 disables rate limiting.</p>
            </div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <ShieldCheck className="h-4 w-4" /> Redaction
          </CardTitle>
          <CardDescription>Custom rules merged with the global defaults.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-5">
          <div className="space-y-2">
            <Label htmlFor="hdr">Extra header denylist</Label>
            <Textarea
              id="hdr"
              rows={2}
              value={headerDeny}
              onChange={(e) => setHeaderDeny(e.target.value)}
              placeholder="X-Internal-Token, X-Customer-Email"
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="jsn">Extra JSON field denylist</Label>
            <Textarea
              id="jsn"
              rows={2}
              value={jsonDeny}
              onChange={(e) => setJsonDeny(e.target.value)}
              placeholder="account_number, kyc_id"
            />
          </div>
          <Separator />
          <label className="flex items-start gap-2 text-sm">
            <input
              type="checkbox"
              checked={disableDefaults}
              onChange={(e) => setDisableDefaults(e.target.checked)}
              className="mt-0.5 h-4 w-4 rounded border-input"
            />
            <span>
              <span className="font-medium">Disable defaults</span>
              <span className="block text-xs text-muted-foreground">
                Advanced. Skips the package default header / JSON / regex rules. Use only when
                you have a strict allowlist of fields you want raw.
              </span>
            </span>
          </label>
          <div className="flex flex-wrap gap-1">
            <Badge variant="outline" className="font-mono text-[10px]">Authorization</Badge>
            <Badge variant="outline" className="font-mono text-[10px]">Cookie</Badge>
            <Badge variant="outline" className="font-mono text-[10px]">password</Badge>
            <Badge variant="outline" className="font-mono text-[10px]">api_key</Badge>
            <Badge variant="outline" className="font-mono text-[10px]">JWT regex</Badge>
            <Badge variant="outline" className="font-mono text-[10px]">credit-card (Luhn)</Badge>
          </div>
        </CardContent>
      </Card>

      <div className="lg:col-span-3 flex justify-end">
        <Button onClick={submit} disabled={pending}>
          {pending ? <Loader2 className="animate-spin" /> : <Save />}
          Save changes
        </Button>
      </div>
    </div>
  );
}
