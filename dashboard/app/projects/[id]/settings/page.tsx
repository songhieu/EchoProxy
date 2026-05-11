import { notFound } from "next/navigation";
import { getSession } from "@/lib/auth";
import { getProject } from "@/lib/api/auth";
import { isRedirectError } from "@/lib/utils";
import { PageHeader } from "@/components/page-header";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { RetentionForm } from "./retention-form";

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
    </div>
  );
}
