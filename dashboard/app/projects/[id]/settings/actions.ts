"use server";

import { revalidatePath } from "next/cache";
import { updateProjectRetention } from "@/lib/api/auth";
import { getSession } from "@/lib/auth";

export async function setRetention(projectId: number, days: number) {
  if (!Number.isInteger(days) || days < 1 || days > 90) {
    return { ok: false, error: "Retention must be 1-90 days" } as const;
  }
  const session = await getSession();
  try {
    await updateProjectRetention(session.token, projectId, days);
    revalidatePath(`/projects/${projectId}/settings`);
    return { ok: true } as const;
  } catch (err) {
    return { ok: false, error: (err as Error).message } as const;
  }
}
