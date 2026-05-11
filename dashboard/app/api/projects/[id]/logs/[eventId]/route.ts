import { NextRequest, NextResponse } from "next/server";
import { isRedirectError } from "@/lib/utils";
import { getSession } from "@/lib/auth";
import { getLog } from "@/lib/api/stats";

export async function GET(
  _req: NextRequest,
  { params }: { params: { id: string; eventId: string } },
) {
  const session = await getSession();
  try {
    const event = await getLog(session.token, params.id, params.eventId);
    return NextResponse.json(event);
  } catch (e) {
    if (isRedirectError(e)) throw e;
    return NextResponse.json({ error: (e as Error).message }, { status: 500 });
  }
}
