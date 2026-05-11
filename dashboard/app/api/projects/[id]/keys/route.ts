import { NextResponse } from "next/server";
import { isRedirectError } from "@/lib/utils";
import { getSession } from "@/lib/auth";
import { listAPIKeys } from "@/lib/api/auth";

export async function GET(_: Request, { params }: { params: { id: string } }) {
  const session = await getSession();
  try {
    return NextResponse.json(await listAPIKeys(session.token, params.id));
  } catch (e) {
    if (isRedirectError(e)) throw e;
    return NextResponse.json({ error: (e as Error).message }, { status: 500 });
  }
}
