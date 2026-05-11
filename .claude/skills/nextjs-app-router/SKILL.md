---
name: nextjs-app-router
description: Standard pattern for the echoproxy dashboard (Next.js App Router) — Server vs Client Components, Server Actions, httpOnly cookie auth, react-query, route groups. Apply when creating a new page in dashboard/; adding a form/mutation; calling auth-api or stats-api from the dashboard; setting up auth; debugging rendering (server vs client); or adding real-time streams.
---

# Next.js App Router for the echoproxy dashboard

The echoproxy dashboard uses **Next.js App Router** (Next 14+). Goal: render on the server by default, fetch data close to the render, and use the client only for interactivity.

## 1. Directory structure

```
dashboard/
├── app/
│   ├── (auth)/                   ← route group: login, signup
│   │   ├── login/page.tsx
│   │   └── signup/page.tsx
│   ├── (app)/                    ← route group: protected, with sidebar layout
│   │   ├── layout.tsx            ← sidebar, project switcher, user menu
│   │   ├── projects/
│   │   │   ├── page.tsx          ← list projects
│   │   │   └── [id]/
│   │   │       ├── layout.tsx    ← project tabs (keys/logs/analytics/live)
│   │   │       ├── keys/page.tsx
│   │   │       ├── logs/page.tsx
│   │   │       ├── analytics/page.tsx
│   │   │       └── live/page.tsx
│   ├── api/                      ← route handlers (BFF endpoints, NO business logic)
│   │   └── auth/[...nextauth]/route.ts
│   ├── layout.tsx                ← root layout
│   └── page.tsx                  ← landing / redirect
├── lib/
│   ├── api/                      ← typed clients calling auth-api, stats-api
│   │   ├── auth.ts
│   │   ├── stats.ts
│   │   └── types.ts
│   ├── auth.ts                   ← getSession() helper, server-only
│   └── react-query.ts            ← QueryClient provider
├── components/
│   ├── ui/                       ← shadcn/ui primitives
│   └── ...
├── middleware.ts                 ← redirect unauthenticated → /login
└── next.config.mjs
```

## 2. Server vs Client Components — rules

**Default to Server Component.** Add `"use client"` only when needed:

- Hooks (`useState`, `useEffect`, `useQuery`).
- Event handlers (`onClick`, `onChange`).
- Browser-only APIs (`localStorage`, `window`).
- Libraries that require the client (Framer Motion, animated charts).

**Interleave pattern:** Server Component fetches data → renders a Client Component, passing the data as props.

```tsx
// app/(app)/projects/[id]/keys/page.tsx  — Server Component
import { listAPIKeys } from "@/lib/api/auth";
import { getSession } from "@/lib/auth";
import { KeysTable } from "./keys-table";  // client

export default async function Page({ params }: { params: { id: string } }) {
  const session = await getSession();
  const keys = await listAPIKeys(session.token, params.id);
  return <KeysTable initialData={keys} projectId={params.id} />;
}
```

```tsx
// app/(app)/projects/[id]/keys/keys-table.tsx  — Client Component
"use client";
import { useQuery } from "@tanstack/react-query";
import { listAPIKeys } from "@/lib/api/auth";

export function KeysTable({ initialData, projectId }) {
  const { data } = useQuery({
    queryKey: ["api-keys", projectId],
    queryFn: () => listAPIKeys(undefined, projectId),
    initialData,
  });
  // ... table with refresh
}
```

## 3. Data-fetching pattern

### Read (Server Component)

- Call typed clients from `lib/api/*` directly inside Server Components.
- Use React's `cache()` if you need dedupe across the same render tree.
- Pass JWT from the cookie into the API client.

```ts
// lib/api/stats.ts
export async function getMinuteMetrics(token: string, projectId: string, range: string) {
  const res = await fetch(`${process.env.STATS_API_URL}/v1/metrics/minute?project_id=${projectId}&range=${range}`, {
    headers: { Authorization: `Bearer ${token}` },
    cache: "no-store",  // always fresh; or next: { revalidate: 30 } for a 30s cache
  });
  if (!res.ok) throw new Error("stats fetch failed");
  return res.json() as Promise<MinuteMetric[]>;
}
```

### Write (Server Action)

Form submit → Server Action → call API → `revalidatePath` / `revalidateTag`.

```tsx
// app/(app)/projects/[id]/keys/actions.ts
"use server";
import { getSession } from "@/lib/auth";
import { revalidatePath } from "next/cache";

export async function createAPIKey(projectId: string, formData: FormData) {
  const session = await getSession();
  const name = formData.get("name") as string;
  const allowlist = (formData.get("allowlist") as string).split(",").map(s => s.trim());

  const res = await fetch(`${process.env.AUTH_API_URL}/v1/projects/${projectId}/keys`, {
    method: "POST",
    headers: { "Content-Type": "application/json", Authorization: `Bearer ${session.token}` },
    body: JSON.stringify({ name, allowlist }),
  });
  if (!res.ok) return { ok: false, error: await res.text() };
  revalidatePath(`/projects/${projectId}/keys`);
  return { ok: true, key: await res.json() };
}
```

```tsx
// keys-form.tsx (Client)
"use client";
import { useFormState } from "react-dom";
import { createAPIKey } from "./actions";

export function CreateKeyForm({ projectId }) {
  const [state, action] = useFormState(createAPIKey.bind(null, projectId), { ok: false });
  return <form action={action}>...</form>;
}
```

### Real-time (SSE)

The live tail page uses an `EventSource` in a Client Component, connecting to `stats-api`.

```tsx
"use client";
useEffect(() => {
  const es = new EventSource(`/api/stream?project_id=${projectId}`);
  es.onmessage = (e) => setEvents(prev => [JSON.parse(e.data), ...prev].slice(0, 200));
  return () => es.close();
}, [projectId]);
```

`/app/api/stream/route.ts` proxies to stats-api with the JWT (BFF pattern, do not expose stats-api tokens to the browser).

## 4. Auth — httpOnly cookie + JWT

NextAuth Credentials provider:

```ts
// app/api/auth/[...nextauth]/route.ts
import NextAuth from "next-auth";
import Credentials from "next-auth/providers/credentials";

const handler = NextAuth({
  providers: [
    Credentials({
      credentials: { email: {}, password: {} },
      async authorize(creds) {
        const res = await fetch(`${process.env.AUTH_API_URL}/v1/login`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(creds),
        });
        if (!res.ok) return null;
        const { user, token } = await res.json();
        return { id: user.id, email: user.email, token };
      },
    }),
  ],
  callbacks: {
    async jwt({ token, user }) { if (user) token.apiToken = user.token; return token; },
    async session({ session, token }) { session.token = token.apiToken; return session; },
  },
  session: { strategy: "jwt" },
});
export { handler as GET, handler as POST };
```

```ts
// lib/auth.ts (server-only)
import { getServerSession } from "next-auth/next";
export async function getSession() {
  const s = await getServerSession();
  if (!s) throw new Error("unauthenticated");
  return s as { token: string; user: { email: string } };
}
```

```ts
// middleware.ts — redirect unauthenticated requests
import { withAuth } from "next-auth/middleware";
export default withAuth({ pages: { signIn: "/login" } });
export const config = { matcher: ["/projects/:path*"] };
```

**Never** store JWTs in `localStorage` or non-httpOnly cookies. NextAuth handles httpOnly + signed cookies for you.

## 5. React Query setup

```tsx
// lib/react-query.ts
"use client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useState } from "react";

export function Providers({ children }: { children: React.ReactNode }) {
  const [client] = useState(() => new QueryClient({
    defaultOptions: { queries: { staleTime: 30_000, retry: 1 } },
  }));
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}
```

Wrap it in `app/layout.tsx`. Use react-query MAINLY for client-side refresh, polling, and optimistic updates. Initial data should always come from the Server Component.

## 6. Env management

- `.env.local` for dev: `AUTH_API_URL`, `STATS_API_URL`, `NEXTAUTH_SECRET`, `NEXTAUTH_URL`.
- DON'T use `NEXT_PUBLIC_*` for API URLs when the dashboard goes through the BFF (route handlers `/api/*` proxy). The browser hits `/api/*`, the server knows the real URL.
- Type-check env via `lib/env.ts` with `zod`:

```ts
import { z } from "zod";
export const env = z.object({
  AUTH_API_URL: z.string().url(),
  STATS_API_URL: z.string().url(),
  NEXTAUTH_SECRET: z.string().min(32),
}).parse(process.env);
```

## 7. UI library

- `shadcn/ui` for primitives (Button, Dialog, Table). Copy into `components/ui/`.
- `tremor` for charts/dashboards (rich pre-built analytics components).
- `tailwindcss` for styling.
- `lucide-react` for icons.

## 8. Adding a new page — checklist

1. Place the page in the correct route group (`(auth)` or `(app)`).
2. Default Server Component; Client Component only for interactive parts.
3. Fetch data in the Server Component, pass it as props.
4. Mutations → Server Action in `actions.ts` next to the page, plus `revalidatePath`.
5. Loading state → `loading.tsx` next to the page; error → `error.tsx`.
6. Type API responses in `lib/api/types.ts` (single source of truth, in sync with backend OpenAPI).
7. Don't call `auth-api` / `stats-api` directly from a Client Component — go through a Server Component (read) or Server Action / route handler (write).

## 9. Anti-patterns

- ❌ `"use client"` at the root layout. Kills server rendering everywhere.
- ❌ Fetching in `useEffect` when the data could come from a Server Component.
- ❌ Calling internal API URLs from the browser (leaks network topology).
- ❌ Storing tokens in `localStorage`.
- ❌ Putting business logic in route handlers under `app/api/*`. That's the Go service's job. Route handlers only proxy / format.
- ❌ Passing non-serializable props (functions, class instances) from Server → Client Component.
- ❌ `useState` for data that comes from the server. Use react-query `initialData` or re-render the Server Component.

## 10. When extending

Every new dashboard feature must:
- Follow the data-fetching pattern (Server Component reads, Server Action writes).
- Be type-safe end-to-end: backend OpenAPI → `lib/api/types.ts` → component props.
- Reuse primitives from `components/ui/`. Create new ones only if missing.
- Have a loading skeleton + error state.
