"use server";

import { revalidatePath } from "next/cache";
import { cookies } from "next/headers";
import { z } from "zod";
import { getSessionUser, ROLE_COOKIE, type DemoRole } from "@/lib/session";
import {
  requestDocument,
  holdPayment,
  escalateCase,
  approvePayment,
  recordCorrectedDocument,
  decideApproval,
  resetDemo,
  type WorkflowResult,
} from "@/lib/workflow";

const roleSchema = z.enum(["amina", "jean"]);
const decisionSchema = z.enum(["APPROVE", "REJECT", "REQUEST_INFO"]);

function refreshAll() {
  revalidatePath("/work");
  revalidatePath("/invoices");
  revalidatePath("/suppliers");
  revalidatePath("/approvals");
  revalidatePath("/activity");
  revalidatePath("/settings");
  revalidatePath("/", "layout");
}

export async function switchRoleAction(rawRole: string): Promise<WorkflowResult> {
  const parsed = roleSchema.safeParse(rawRole);
  if (!parsed.success) return { ok: false, message: "Unknown role." };
  const store = await cookies();
  store.set(ROLE_COOKIE, parsed.data satisfies DemoRole, { path: "/", maxAge: 60 * 60 * 24 * 365 });
  refreshAll();
  return { ok: true, message: `Switched to ${parsed.data === "jean" ? "Jean Baptiste (Controller)" : "Amina Diallo (Finance Ops)"}.` };
}

export async function requestDocumentAction(rawCaseId: string): Promise<WorkflowResult> {
  const user = await getSessionUser();
  const result = await requestDocument(rawCaseId, user.name, user.role);
  if (result.ok) refreshAll();
  return result;
}

export async function holdPaymentAction(rawCaseId: string): Promise<WorkflowResult> {
  const user = await getSessionUser();
  const result = await holdPayment(rawCaseId, user.name, user.role);
  if (result.ok) refreshAll();
  return result;
}

export async function escalateCaseAction(rawCaseId: string): Promise<WorkflowResult> {
  const user = await getSessionUser();
  const result = await escalateCase(rawCaseId, user.name, user.role);
  if (result.ok) refreshAll();
  return result;
}

export async function approvePaymentAction(rawCaseId: string): Promise<WorkflowResult> {
  const user = await getSessionUser();
  const result = await approvePayment(rawCaseId, user.name, user.role);
  if (result.ok) refreshAll();
  return result;
}

export async function recordCorrectedDocumentAction(rawCaseId: string): Promise<WorkflowResult> {
  const user = await getSessionUser();
  const result = await recordCorrectedDocument(rawCaseId, user.name);
  if (result.ok) refreshAll();
  return result;
}

export async function decideApprovalAction(
  rawApprovalId: string,
  rawDecision: string,
  comment?: string
): Promise<WorkflowResult> {
  const parsed = decisionSchema.safeParse(rawDecision);
  if (!parsed.success) return { ok: false, message: "Invalid decision." };
  const user = await getSessionUser();
  const result = await decideApproval(rawApprovalId, parsed.data, user.name, user.role, comment);
  if (result.ok) refreshAll();
  return result;
}

export async function resetDemoAction(): Promise<WorkflowResult> {
  const result = await resetDemo();
  if (result.ok) refreshAll();
  return result;
}
