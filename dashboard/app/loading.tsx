import { Skeleton } from "@/components/ui/skeleton";

export default function RootLoading() {
  return (
    <div className="flex min-h-screen items-center justify-center px-4">
      <Skeleton className="h-12 w-48" />
    </div>
  );
}
