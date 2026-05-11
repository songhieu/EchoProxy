import { PageHeader } from "@/components/page-header";
import { LiveTail } from "./live-tail";

export const dynamic = "force-dynamic";

export default function LivePage({ params }: { params: { id: string } }) {
  return (
    <div className="space-y-6">
      <PageHeader
        title="Live tail"
        description="Polls the most recent events every 2 seconds. Pause to inspect specific events."
      />
      <LiveTail projectId={params.id} />
    </div>
  );
}
