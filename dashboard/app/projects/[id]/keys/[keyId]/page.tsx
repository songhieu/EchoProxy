import Link from "next/link";
import { ArrowLeft } from "lucide-react";
import { getSession } from "@/lib/auth";
import { getAPIKey } from "@/lib/api/auth";
import { isRedirectError } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { PageHeader } from "@/components/page-header";
import { KeySettingsForm } from "./settings-form";

export default async function KeySettingsPage({
  params,
}: {
  params: { id: string; keyId: string };
}) {
  const session = await getSession();
  let key: Awaited<ReturnType<typeof getAPIKey>> | null = null;
  let error: string | null = null;
  try {
    key = await getAPIKey(session.token, params.id, params.keyId);
  } catch (e) {
    if (isRedirectError(e)) throw e;
    error = (e as Error).message;
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title={`Key ${key?.prefix ?? params.keyId}…`}
        description="Edit the allowlist, rate limit, body cap, and per-key redaction rules."
        action={
          <Button variant="outline" asChild>
            <Link href={`/projects/${params.id}/keys`}>
              <ArrowLeft /> Back to keys
            </Link>
          </Button>
        }
      />

      {error && (
        <div className="rounded-md border border-destructive/30 bg-destructive/5 p-4 text-sm text-destructive">
          {error}
        </div>
      )}

      {key && <KeySettingsForm projectId={params.id} apiKey={key} />}
    </div>
  );
}
