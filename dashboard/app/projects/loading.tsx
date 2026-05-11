import { Skeleton } from "@/components/ui/skeleton";

export default function ProjectsLoading() {
  return (
    <div className="flex min-h-screen">
      <aside className="hidden w-60 border-r bg-card md:block" />
      <main className="flex-1 px-8 py-6">
        <div className="mx-auto max-w-5xl space-y-6">
          <Skeleton className="h-9 w-40" />
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {Array.from({ length: 6 }).map((_, i) => (
              <Skeleton key={i} className="h-24 w-full" />
            ))}
          </div>
        </div>
      </main>
    </div>
  );
}
