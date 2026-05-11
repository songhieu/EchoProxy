"use client";

import Link from "next/link";
import { useMemo, useState } from "react";
import { AlertTriangle, Code2, KeyRound, Eye, EyeOff, Server } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { CodeBlock } from "@/components/code-block";
import { EmptyState } from "@/components/ui/empty-state";
import { SECTIONS, type TemplateVars } from "./snippets";

type KeyOption = { id: number; prefix: string; description: string };

const PLACEHOLDER_KEY = "<YOUR_API_KEY>";
const NO_KEY = "__no_key";

export function IntegrateView({
  projectId,
  keys,
}: {
  projectId: string;
  keys: KeyOption[];
}) {
  const [keyId, setKeyId] = useState<string>(keys[0] ? String(keys[0].id) : NO_KEY);
  const [pasteKey, setPasteKey] = useState<string>("");
  const [reveal, setReveal] = useState(false);
  const [proxyURL, setProxyURL] = useState("http://localhost:8080");
  const [ingestURL, setIngestURL] = useState("http://localhost:8081");
  const [grpcAddr, setGRPCAddr] = useState("localhost:8082");
  const [upstream, setUpstream] = useState("https://api.example.com");

  const selected = keys.find((k) => String(k.id) === keyId);

  const vars = useMemo<TemplateVars>(
    () => ({
      apiKey: pasteKey || (selected ? `${selected.prefix}${PLACEHOLDER_KEY.slice(selected.prefix.length || 0)}` : PLACEHOLDER_KEY),
      proxyURL,
      ingestURL,
      grpcAddr,
      upstream,
    }),
    [pasteKey, selected, proxyURL, ingestURL, grpcAddr, upstream],
  );

  if (keys.length === 0) {
    return (
      <EmptyState
        icon={KeyRound}
        title="Create an API key first"
        description="Integration snippets need an active API key to render."
        action={
          <Button asChild>
            <Link href={`/projects/${projectId}/keys`}>Go to API keys</Link>
          </Button>
        }
      />
    );
  }

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <Server className="h-4 w-4" /> Connection
          </CardTitle>
          <CardDescription>
            Snippets below auto-update with these values. Defaults match the local docker-compose stack — change them
            for staging/production.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid gap-4 md:grid-cols-2">
            <div className="space-y-2">
              <label className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
                API key
              </label>
              <div className="flex gap-2">
                <Select value={keyId} onValueChange={setKeyId}>
                  <SelectTrigger className="w-64">
                    <SelectValue placeholder="Pick a key" />
                  </SelectTrigger>
                  <SelectContent>
                    {keys.map((k) => (
                      <SelectItem key={k.id} value={String(k.id)}>
                        <span className="font-mono">{k.prefix}…</span>{" "}
                        {k.description && <span className="text-muted-foreground">— {k.description}</span>}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <div className="relative flex-1">
                  <Input
                    placeholder="Paste your full key (shown only at creation)"
                    type={reveal ? "text" : "password"}
                    value={pasteKey}
                    onChange={(e) => setPasteKey(e.target.value)}
                    className="pr-10 font-mono text-xs"
                  />
                  <button
                    type="button"
                    onClick={() => setReveal((r) => !r)}
                    className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                    aria-label={reveal ? "Hide key" : "Show key"}
                  >
                    {reveal ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                  </button>
                </div>
              </div>
              <p className="text-xs text-muted-foreground">
                The full key is only shown once at creation. If you didn&apos;t save it, create a new key — the
                snippets will use a placeholder until you paste one.
              </p>
            </div>

            <div className="space-y-2">
              <label className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
                Endpoints
              </label>
              <div className="grid grid-cols-2 gap-2">
                <Input
                  value={proxyURL}
                  onChange={(e) => setProxyURL(e.target.value)}
                  placeholder="Proxy URL"
                  className="font-mono text-xs"
                />
                <Input
                  value={ingestURL}
                  onChange={(e) => setIngestURL(e.target.value)}
                  placeholder="Ingest URL (HTTP)"
                  className="font-mono text-xs"
                />
                <Input
                  value={grpcAddr}
                  onChange={(e) => setGRPCAddr(e.target.value)}
                  placeholder="Ingest gRPC (host:port)"
                  className="font-mono text-xs"
                />
                <Input
                  value={upstream}
                  onChange={(e) => setUpstream(e.target.value)}
                  placeholder="Example upstream (proxy mode)"
                  className="font-mono text-xs"
                />
              </div>
            </div>
          </div>

          {!pasteKey && (
            <div className="flex items-start gap-2 rounded-md border border-warning/40 bg-warning/5 p-3 text-sm">
              <AlertTriangle className="mt-0.5 h-4 w-4 text-warning" />
              <div>
                <span className="font-medium">Snippets show a placeholder key.</span> Paste your full{" "}
                <code className="font-mono">sk_live_…</code> above to render runnable code.
              </div>
            </div>
          )}
        </CardContent>
      </Card>

      <Tabs defaultValue={SECTIONS[0].id} className="w-full">
        <TabsList>
          {SECTIONS.map((s) => (
            <TabsTrigger key={s.id} value={s.id} className="gap-1.5">
              <Code2 className="h-3.5 w-3.5" /> {s.title}
            </TabsTrigger>
          ))}
        </TabsList>
        {SECTIONS.map((s) => (
          <TabsContent key={s.id} value={s.id} className="space-y-4">
            <div className="rounded-md border bg-muted/30 p-4 text-sm">
              <div className="flex items-center gap-2">
                <Badge variant="outline">{s.snippets.length} languages</Badge>
                <span className="text-muted-foreground">{s.description}</span>
              </div>
            </div>
            <SnippetTabs section={s} vars={vars} />
          </TabsContent>
        ))}
      </Tabs>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Verify your integration</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3 text-sm">
          <ol className="list-decimal space-y-1 pl-5">
            <li>Run one of the snippets against a real or test endpoint.</li>
            <li>
              Open the{" "}
              <Link href={`/projects/${projectId}/live`} className="font-medium underline-offset-4 hover:underline">
                Live tail
              </Link>{" "}
              page — captured events appear within ~2 seconds.
            </li>
            <li>
              Open the{" "}
              <Link href={`/projects/${projectId}/logs`} className="font-medium underline-offset-4 hover:underline">
                Logs
              </Link>{" "}
              page to inspect the full request/response payload (with redaction applied).
            </li>
          </ol>
        </CardContent>
      </Card>
    </div>
  );
}

function SnippetTabs({ section, vars }: { section: (typeof SECTIONS)[number]; vars: TemplateVars }) {
  const [active, setActive] = useState(section.snippets[0].id);
  const current = section.snippets.find((s) => s.id === active) ?? section.snippets[0];
  return (
    <div className="space-y-3">
      <Tabs value={active} onValueChange={setActive}>
        <TabsList className="flex flex-wrap">
          {section.snippets.map((s) => (
            <TabsTrigger key={s.id} value={s.id}>
              {s.label}
            </TabsTrigger>
          ))}
        </TabsList>
      </Tabs>
      <CodeBlock code={current.build(vars)} language={current.language} />
    </div>
  );
}
