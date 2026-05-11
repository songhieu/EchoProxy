import { NextRequest, NextResponse } from "next/server";
import { isRedirectError } from "@/lib/utils";
import { getSession } from "@/lib/auth";
import { getDistribution } from "@/lib/api/stats";
import { parseRange } from "@/lib/api/range";

export async function GET(req: NextRequest, { params }: { params: { id: string } }) {
  const session = await getSession();
  const sp = req.nextUrl.searchParams;
  const k = sp.get("kind");
  const kind = (k === "method" || k === "host" ? k : "status") as "status" | "method" | "host";
  try {
    return NextResponse.json(
      await getDistribution(session.token, params.id, kind, parseRange(sp)),
    );
  } catch (e) {
    if (isRedirectError(e)) throw e;
    return NextResponse.json({ error: (e as Error).message }, { status: 500 });
  }
}
