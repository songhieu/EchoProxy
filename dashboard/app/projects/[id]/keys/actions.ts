"use server";

import { revalidatePath } from "next/cache";
import { getSession } from "@/lib/auth";
import { createAPIKey, revokeAPIKey } from "@/lib/api/auth";

export async function createKeyAction(projectId: string, _prev: unknown, formData: FormData) {
  const session = await getSession();
  const allowlist = String(formData.get("allowlist") ?? "")
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);
  const description = String(formData.get("description") ?? "");
  const body_cap = Number(formData.get("body_cap") || 0);

  try {
    const key = await createAPIKey(session.token, projectId, { allowlist, body_cap, description });
    revalidatePath(`/projects/${projectId}/keys`);
    return { ok: true as const, raw: key.raw ?? null };
  } catch (err) {
    return { ok: false as const, error: (err as Error).message };
  }
}

export async function revokeKeyAction(projectId: string, keyId: number) {
  const session = await getSession();
  await revokeAPIKey(session.token, projectId, keyId);
  revalidatePath(`/projects/${projectId}/keys`);
}
