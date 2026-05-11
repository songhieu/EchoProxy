"use server";

import { signup as apiSignup } from "@/lib/api/auth";

export async function signupAction(email: string, password: string) {
  try {
    await apiSignup(email, password);
    return { ok: true as const };
  } catch (e) {
    return { ok: false as const, error: (e as Error).message };
  }
}
