import { notFound } from "next/navigation";
import { getSession } from "@/lib/auth";
import { getProject } from "@/lib/api/auth";
import { getProxyConfig } from "@/lib/api/proxy-admin";
import { isRedirectError, formatBytes } from "@/lib/utils";
import { PageHeader } from "@/components/page-header";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { RetentionForm } from "./retention-form";
import { DeleteProjectButton } from "./delete-project";

export const dynamic = "force-dynamic";

export default async function SettingsPage({ params }: { params: { id: string } }) {
  const session = await getSession();
  let project: Awaited<ReturnType<typeof getProject>> | null = null;
  try {
    project = await getProject(session.token, params.id);
  } catch (e) {
    if (isRedirectError(e)) throw e;
    notFound();
  }
  if (!project) notFound();

  // Proxy admin config is process-wide (not per-project) — same values apply
  // to every project routed through this proxy-gateway instance.
  const proxyCfg = await getProxyConfig();

  return (
    <div className="space-y-6">
      <PageHeader
        title="Settings"
        description={`Project ${project.name} • #${project.id}`}
      />

      <Card>
        <CardHeader>
          <CardTitle>Data retention</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <p className="text-sm text-muted-foreground">
            How long to keep raw events (headers + bodies) in ClickHouse before
            they are deleted. The platform cap is 90 days; the cleanup job
            (<code className="rounded bg-muted px-1 text-xs">cleanup/cmd/cleanup</code>)
            enforces shorter per-project values nightly. Aggregates
            (per-minute counts, latency percentiles) are kept 180 days
            independent of this setting. See{" "}
            <code className="rounded bg-muted px-1 text-xs">docs/retention.md</code>.
          </p>
          <RetentionForm projectId={project.id} initial={project.retention_days} />
        </CardContent>
      </Card>

      <Card className="border-destructive/30">
        <CardHeader>
          <CardTitle className="text-destructive">Danger zone</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <p className="text-sm text-muted-foreground">
            Deleting a project drops the project row + all API keys
            (Postgres ON DELETE CASCADE). Events stay in ClickHouse until
            the retention window above expires. Action is irreversible.
          </p>
          <DeleteProjectButton projectId={project.id} projectName={project.name} />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            Proxy timeouts
            <Badge variant="outline" className="font-normal">Process-wide</Badge>
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <p className="text-sm text-muted-foreground">
            Timeouts the <code className="rounded bg-muted px-1 text-xs">proxy-gateway</code> is
            currently enforcing. These are read-only here because they are set
            by environment variables on the proxy process — change them with
            an env-var update and a restart, not per project.
          </p>

          {!proxyCfg ? (
            <div className="rounded-md border border-amber-500/30 bg-amber-500/5 p-3 text-xs text-amber-700 dark:text-amber-300">
              Could not reach the proxy admin endpoint at
              {" "}<code className="font-mono">PROXY_ADMIN_URL</code>. The proxy may
              be unreachable from the dashboard host, or the admin port is not
              exposed. The proxy itself still applies its configured timeouts.
            </div>
          ) : (
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <SettingRow
                label="Upstream response header timeout"
                value={`${proxyCfg.upstream_timeout_seconds}s`}
                env="UPSTREAM_TIMEOUT_SECONDS"
                hint="Max time waiting for the upstream's response headers. Body streaming is not bounded by this."
              />
              <SettingRow
                label="Stream idle timeout"
                value={
                  proxyCfg.stream_idle_timeout_seconds > 0
                    ? `${proxyCfg.stream_idle_timeout_seconds}s`
                    : "disabled"
                }
                env="STREAM_IDLE_TIMEOUT_SECONDS"
                hint="For streaming responses (SSE / gRPC / chunked), the proxy cancels the upstream after this many seconds of inactivity. Events get stream_idle_timeout=true when this fires."
              />
              <SettingRow
                label="Body capture cap"
                value={formatBytes(proxyCfg.body_cap_bytes)}
                env="BODY_CAP_BYTES"
                hint="Per-request / per-response capture limit. Streams pass through unbounded; only the captured slice is bounded."
              />
              <SettingRow
                label="Private targets"
                value={proxyCfg.allow_private_targets ? "Allowed" : "Blocked"}
                env="ALLOW_PRIVATE_TARGETS"
                hint="When false, loopback / RFC 1918 hosts are rejected as upstream targets to mitigate SSRF."
              />
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function SettingRow({
  label,
  value,
  env,
  hint,
}: {
  label: string;
  value: string;
  env: string;
  hint?: string;
}) {
  return (
    <div className="rounded-md border bg-card p-3">
      <div className="flex items-start justify-between gap-3">
        <div className="text-[11px] uppercase tracking-wider text-muted-foreground">{label}</div>
        <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-[10px]">{env}</code>
      </div>
      <div className="mt-1 font-mono text-base font-medium">{value}</div>
      {hint && <p className="mt-1 text-xs text-muted-foreground">{hint}</p>}
    </div>
  );
}
