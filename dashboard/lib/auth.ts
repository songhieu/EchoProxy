import "server-only";
import { getServerSession } from "next-auth";
import { authOptions } from "@/app/api/auth/[...nextauth]/options";

export type Session = { token: string; user: { id: number; email: string } };

export async function getSession(): Promise<Session> {
  const s = await getServerSession(authOptions);
  if (!s || !(s as any).token) {
    throw new Error("unauthenticated");
  }
  return s as unknown as Session;
}
