import { getSession } from "@/lib/auth";
import { listAPIKeys } from "@/lib/api/auth";
import { isRedirectError } from "@/lib/utils";
import { PageHeader } from "@/components/page-header";
import { AnalyticsDashboard } from "./analytics-dashboard";

export const dynamic = "force-dynamic";

export default async function AnalyticsPage({ params }: { params: { id: string } }) {
  const session = await getSession();
  let keys: Awaited<ReturnType<typeof listAPIKeys>> = [];
  try {
    keys = await listAPIKeys(session.token, params.id);
  } catch (e) {
    if (isRedirectError(e)) throw e;
    /* surfaced inside dashboard */
  }
  return (
    <div className="space-y-6">
      <PageHeader
        title="Analytics"
        description="Request volume, latency, error rates and per-endpoint breakdown."
      />
      <AnalyticsDashboard
        projectId={params.id}
        apiKeys={keys.map((k) => ({ id: k.id, prefix: k.prefix, description: k.description }))}
      />
    </div>
  );
}
