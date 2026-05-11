import { NextRequest, NextResponse } from "next/server";
import { isRedirectError } from "@/lib/utils";
import { getSession } from "@/lib/auth";
import { getTimeSeries } from "@/lib/api/stats";
import { parseRange } from "@/lib/api/range";

export async function GET(req: NextRequest, { params }: { params: { id: string } }) {
  const session = await getSession();
  try {
    return NextResponse.json(
      await getTimeSeries(session.token, params.id, parseRange(req.nextUrl.searchParams)),
    );
  } catch (e) {
    if (isRedirectError(e)) throw e;
    return NextResponse.json({ error: (e as Error).message }, { status: 500 });
  }
}
