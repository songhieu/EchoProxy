import { Skeleton } from "@/components/ui/skeleton";

export default function ProjectLoading() {
  return (
    <div className="flex min-h-screen">
      <aside className="w-60 border-r bg-card" />
      <main className="flex-1 px-8 py-6">
        <div className="space-y-6">
          <Skeleton className="h-12 w-64" />
          <Skeleton className="h-72 w-full" />
        </div>
      </main>
    </div>
  );
}
