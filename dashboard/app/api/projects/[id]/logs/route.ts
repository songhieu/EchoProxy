import { NextRequest, NextResponse } from "next/server";
import { isRedirectError } from "@/lib/utils";
import { getSession } from "@/lib/auth";
import { listLogs } from "@/lib/api/stats";

export async function GET(req: NextRequest, { params }: { params: { id: string } }) {
  const session = await getSession();
  const sp = req.nextUrl.searchParams;
  try {
    const logs = await listLogs(session.token, params.id, {
      method: sp.get("method") ?? undefined,
      status: sp.get("status") ? Number(sp.get("status")) : undefined,
      path: sp.get("path") ?? undefined,
      from: sp.get("from") ?? undefined,
      to: sp.get("to") ?? undefined,
      limit: Number(sp.get("limit") ?? 100),
    });
    return NextResponse.json(logs);
  } catch (e) {
    if (isRedirectError(e)) throw e;
    return NextResponse.json({ error: (e as Error).message }, { status: 500 });
  }
}
