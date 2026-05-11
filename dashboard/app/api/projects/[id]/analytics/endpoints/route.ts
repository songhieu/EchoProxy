import { NextRequest, NextResponse } from "next/server";
import { isRedirectError } from "@/lib/utils";
import { getSession } from "@/lib/auth";
import { getEndpoints } from "@/lib/api/stats";
import { parseRange } from "@/lib/api/range";

export async function GET(req: NextRequest, { params }: { params: { id: string } }) {
  const session = await getSession();
  const sp = req.nextUrl.searchParams;
  const limit = Number(sp.get("limit") ?? 50);
  try {
    return NextResponse.json(await getEndpoints(session.token, params.id, parseRange(sp), limit));
  } catch (e) {
    if (isRedirectError(e)) throw e;
    return NextResponse.json({ error: (e as Error).message }, { status: 500 });
  }
}
