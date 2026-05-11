import { getSession } from "@/lib/auth";
import { listAPIKeys } from "@/lib/api/auth";
import { isRedirectError } from "@/lib/utils";
import { PageHeader } from "@/components/page-header";
import { IntegrateView } from "./integrate-view";

export const dynamic = "force-dynamic";

export default async function IntegratePage({ params }: { params: { id: string } }) {
  const session = await getSession();
  let keys: Awaited<ReturnType<typeof listAPIKeys>> = [];
  try {
    keys = await listAPIKeys(session.token, params.id);
  } catch (e) {
    if (isRedirectError(e)) throw e;
    /* surfaced inside view */
  }
  const active = keys.filter((k) => k.status === "active");
  return (
    <div className="space-y-6">
      <PageHeader
        title="Integrate"
        description="Drop these snippets into your codebase to start sending traffic. Both proxy and SDK modes use the same backend pipeline; pick whichever suits your stack."
      />
      <IntegrateView projectId={params.id} keys={active.map((k) => ({ id: k.id, prefix: k.prefix, description: k.description }))} />
    </div>
  );
}
