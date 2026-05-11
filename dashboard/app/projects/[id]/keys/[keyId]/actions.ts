"use server";

import { revalidatePath } from "next/cache";
import { getSession } from "@/lib/auth";
import { updateAPIKey, type APIKeyInput } from "@/lib/api/auth";

export async function updateKeyAction(
  projectId: string,
  keyId: string,
  input: APIKeyInput,
) {
  const session = await getSession();
  try {
    const updated = await updateAPIKey(session.token, projectId, keyId, input);
    revalidatePath(`/projects/${projectId}/keys`);
    revalidatePath(`/projects/${projectId}/keys/${keyId}`);
    return { ok: true as const, key: updated };
  } catch (e) {
    return { ok: false as const, error: (e as Error).message };
  }
}
