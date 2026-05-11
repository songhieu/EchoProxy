import { type ClassValue, clsx } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

export function formatBytes(bytes: number): string {
  if (!bytes) return "0 B";
  const units = ["B", "KB", "MB", "GB"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return `${(bytes / Math.pow(1024, i)).toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

export function formatRelative(iso: string | Date): string {
  const d = typeof iso === "string" ? new Date(iso) : iso;
  const diff = Date.now() - d.getTime();
  const sec = Math.floor(diff / 1000);
  if (sec < 60) return `${sec}s ago`;
  if (sec < 3600) return `${Math.floor(sec / 60)}m ago`;
  if (sec < 86400) return `${Math.floor(sec / 3600)}h ago`;
  return `${Math.floor(sec / 86400)}d ago`;
}

export function statusVariant(status: number): "success" | "warning" | "destructive" | "secondary" {
  if (status === 0) return "secondary";
  if (status >= 500) return "destructive";
  if (status >= 400) return "warning";
  if (status >= 300) return "secondary";
  return "success";
}

// Next.js redirect() throws an Error whose digest starts with NEXT_REDIRECT.
// The framework intercepts it at the Server Component boundary to perform the
// actual redirect — but only if it propagates out. Any try/catch that wraps a
// call which may invoke redirect() must rethrow this error or the redirect
// silently turns into "Error: NEXT_REDIRECT" rendered to the page.
//
// Use:
//   } catch (e) {
//     if (isRedirectError(e)) throw e;
//     error = (e as Error).message;
//   }
export function isRedirectError(e: unknown): boolean {
  if (!e || typeof e !== "object") return false;
  const digest = (e as { digest?: unknown }).digest;
  return typeof digest === "string" && digest.startsWith("NEXT_REDIRECT");
}
