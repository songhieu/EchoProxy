import Link from "next/link";
import { KeyRound, Settings } from "lucide-react";
import { getSession } from "@/lib/auth";
import { listAPIKeys } from "@/lib/api/auth";
import { PageHeader } from "@/components/page-header";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { EmptyState } from "@/components/ui/empty-state";
import { CreateKeyDialog } from "./create-dialog";
import { RevokeKeyButton } from "./revoke-button";
import { formatRelative, isRedirectError } from "@/lib/utils";

export default async function KeysPage({ params }: { params: { id: string } }) {
  const session = await getSession();
  let keys: Awaited<ReturnType<typeof listAPIKeys>> = [];
  let error: string | null = null;
  try {
    keys = await listAPIKeys(session.token, params.id);
  } catch (e) {
    if (isRedirectError(e)) throw e;
    error = (e as Error).message;
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="API keys"
        description="Each key authorizes proxy traffic and SDK ingestion. Set an allowlist to restrict the upstream hosts a key can reach."
        action={<CreateKeyDialog projectId={params.id} />}
      />

      {error && (
        <div className="rounded-md border border-destructive/30 bg-destructive/5 p-4 text-sm text-destructive">
          auth-api unavailable: {error}
        </div>
      )}

      {keys.length === 0 ? (
        <EmptyState
          icon={KeyRound}
          title="No API keys yet"
          description="Create your first key to start sending traffic through the proxy or with an SDK."
          action={<CreateKeyDialog projectId={params.id} />}
        />
      ) : (
        <Card>
          <CardContent className="p-0">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Key</TableHead>
                  <TableHead>Allowlist</TableHead>
                  <TableHead>Body cap</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Rate</TableHead>
                  <TableHead>Description</TableHead>
                  <TableHead>Created</TableHead>
                  <TableHead className="text-right"></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {keys.map((k) => (
                  <TableRow key={k.id}>
                    <TableCell>
                      <code className="font-mono text-xs" title="Prefix shown for identification only — the full key was returned once at creation and is not stored.">
                        {k.prefix}…
                      </code>
                    </TableCell>
                    <TableCell className="max-w-[18rem]">
                      {k.allowlist.length === 0 ? (
                        <span className="text-xs text-muted-foreground">All hosts</span>
                      ) : (
                        <div className="flex flex-wrap gap-1">
                          {k.allowlist.map((h) => (
                            <Badge key={h} variant="outline" className="font-mono text-[10px]">
                              {h}
                            </Badge>
                          ))}
                        </div>
                      )}
                    </TableCell>
                    <TableCell className="text-xs">
                      {k.body_cap ? `${k.body_cap.toLocaleString()} B` : "default"}
                    </TableCell>
                    <TableCell>
                      <Badge variant={k.status === "active" ? "success" : "secondary"}>{k.status}</Badge>
                    </TableCell>
                    <TableCell className="text-xs">
                      {k.rate_limit_rps > 0 ? `${k.rate_limit_rps} rps` : "—"}
                    </TableCell>
                    <TableCell className="max-w-[14rem] truncate text-sm">{k.description || "—"}</TableCell>
                    <TableCell className="text-xs text-muted-foreground">
                      {formatRelative(k.created_at)}
                    </TableCell>
                    <TableCell className="text-right">
                      <Button variant="ghost" size="sm" asChild>
                        <Link href={`/projects/${params.id}/keys/${k.id}`}>
                          <Settings /> Settings
                        </Link>
                      </Button>
                      {k.status === "active" && <RevokeKeyButton projectId={params.id} keyId={k.id} />}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
