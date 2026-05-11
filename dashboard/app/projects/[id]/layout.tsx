import { notFound } from "next/navigation";
import { ReactNode } from "react";
import { getSession } from "@/lib/auth";
import { listProjects } from "@/lib/api/auth";
import { isRedirectError } from "@/lib/utils";
import { Sidebar } from "@/components/sidebar";

export default async function ProjectLayout({
  children,
  params,
}: {
  children: ReactNode;
  params: { id: string };
}) {
  const session = await getSession();
  let projects: Awaited<ReturnType<typeof listProjects>> = [];
  try {
    projects = await listProjects(session.token);
  } catch (e) {
    if (isRedirectError(e)) throw e;
    /* surface inside page */
  }
  const current = projects.find((p) => String(p.id) === params.id);
  if (!current && projects.length > 0) notFound();

  return (
    <div className="flex min-h-screen">
      <Sidebar email={session.user.email} projects={projects} currentProjectId={params.id} />
      <main className="flex-1 overflow-x-hidden px-8 py-6">{children}</main>
    </div>
  );
}
