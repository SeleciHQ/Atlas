import { NextResponse } from "next/server";
import { z } from "zod";
import { getSessionUser } from "@/lib/session";
import { requestDocument, holdPayment, escalateCase, approvePayment, recordCorrectedDocument } from "@/lib/workflow";

const bodySchema = z.object({
  action: z.enum(["request_document", "hold_payment", "escalate_case", "approve_payment", "record_corrected_document"]),
});

export async function POST(request: Request, context: { params: Promise<{ caseId: string }> }) {
  const { caseId } = await context.params;
  let body: unknown;
  try {
    body = await request.json();
  } catch {
    return NextResponse.json({ ok: false, message: "Invalid JSON body." }, { status: 400 });
  }
  const parsed = bodySchema.safeParse(body);
  if (!parsed.success) {
    return NextResponse.json({ ok: false, message: "Invalid action.", issues: parsed.error.issues }, { status: 400 });
  }

  const user = await getSessionUser();
  const { action } = parsed.data;
  let result;
  switch (action) {
    case "request_document":
      result = await requestDocument(caseId, user.name, user.role);
      break;
    case "hold_payment":
      result = await holdPayment(caseId, user.name, user.role);
      break;
    case "escalate_case":
      result = await escalateCase(caseId, user.name, user.role);
      break;
    case "approve_payment":
      result = await approvePayment(caseId, user.name, user.role);
      break;
    case "record_corrected_document":
      result = await recordCorrectedDocument(caseId, user.name);
      break;
  }
  return NextResponse.json(result, { status: result.ok ? 200 : 403 });
}
