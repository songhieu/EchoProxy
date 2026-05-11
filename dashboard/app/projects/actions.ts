"use server";

import { revalidatePath } from "next/cache";
import { createProject as apiCreateProject } from "@/lib/api/auth";
import { getSession } from "@/lib/auth";

export async function createProject(_prev: unknown, formData: FormData) {
  const session = await getSession();
  const name = String(formData.get("name") ?? "").trim();
  if (!name) return { ok: false, error: "Name required" };
  try {
    await apiCreateProject(session.token, name);
    revalidatePath("/projects");
    return { ok: true };
  } catch (err) {
    return { ok: false, error: (err as Error).message };
  }
}
