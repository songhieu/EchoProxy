import Link from "next/link";
import { ArrowRight, Folder } from "lucide-react";
import { getSession } from "@/lib/auth";
import { listProjects } from "@/lib/api/auth";
import { Sidebar } from "@/components/sidebar";
import { PageHeader } from "@/components/page-header";
import { Card, CardContent } from "@/components/ui/card";
import { EmptyState } from "@/components/ui/empty-state";
import { CreateProjectDialog } from "./create-dialog";
import { formatRelative, isRedirectError } from "@/lib/utils";

export default async function ProjectsPage() {
  const session = await getSession();
  let projects: Awaited<ReturnType<typeof listProjects>> = [];
  let error: string | null = null;
  try {
    projects = await listProjects(session.token);
  } catch (e) {
    if (isRedirectError(e)) throw e;
    error = (e as Error).message;
  }

  return (
    <div className="flex min-h-screen">
      <Sidebar email={session.user.email} projects={projects} />
      <main className="flex-1 px-8 py-6">
        <div className="mx-auto max-w-5xl space-y-6">
          <PageHeader
            title="Projects"
            description="A project groups your API keys and isolates events from other tenants."
            action={<CreateProjectDialog />}
          />

          {error && (
            <div className="rounded-md border border-destructive/30 bg-destructive/5 p-4 text-sm text-destructive">
              auth-api unavailable: {error}
            </div>
          )}

          {projects.length === 0 ? (
            <EmptyState
              icon={Folder}
              title="No projects yet"
              description="Create your first project to start collecting HTTP traffic."
              action={<CreateProjectDialog />}
            />
          ) : (
            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
              {projects.map((p) => (
                <Link key={p.id} href={`/projects/${p.id}/keys`}>
                  <Card className="group transition-colors hover:border-foreground/40">
                    <CardContent className="flex items-start justify-between p-5">
                      <div className="space-y-1">
                        <div className="flex items-center gap-2">
                          <Folder className="h-4 w-4 text-muted-foreground" />
                          <span className="font-semibold">{p.name}</span>
                        </div>
                        <div className="text-xs text-muted-foreground">
                          Created {formatRelative(p.created_at)} · ID {p.id}
                        </div>
                      </div>
                      <ArrowRight className="h-4 w-4 text-muted-foreground transition-transform group-hover:translate-x-0.5" />
                    </CardContent>
                  </Card>
                </Link>
              ))}
            </div>
          )}
        </div>
      </main>
    </div>
  );
}
