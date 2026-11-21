import { NextResponse } from "next/server";
import { z } from "zod";
import { getSessionUser } from "@/lib/session";
import { decideApproval } from "@/lib/workflow";

const bodySchema = z.object({
  decision: z.enum(["APPROVE", "REJECT", "REQUEST_INFO"]),
  comment: z.string().max(2000).optional(),
});

export async function POST(request: Request, context: { params: Promise<{ approvalId: string }> }) {
  const { approvalId } = await context.params;
  let body: unknown;
  try {
    body = await request.json();
  } catch {
    return NextResponse.json({ ok: false, message: "Invalid JSON body." }, { status: 400 });
  }
  const parsed = bodySchema.safeParse(body);
  if (!parsed.success) {
    return NextResponse.json({ ok: false, message: "Invalid decision payload.", issues: parsed.error.issues }, { status: 400 });
  }

  const user = await getSessionUser();
  const result = await decideApproval(approvalId, parsed.data.decision, user.name, user.role, parsed.data.comment);
  return NextResponse.json(result, { status: result.ok ? 200 : 403 });
}
