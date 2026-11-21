import { z } from "zod";
import { db } from "@/lib/db";
import { recordAuditEvent } from "@/lib/audit";
import { evaluatePolicy, getPolicyConfig } from "@/lib/policy";
import { userPermissionsFor } from "@/types/policy";
import { resetAndSeed } from "@/lib/demo-data";
import type { Role } from "@/types/ontology";

export interface WorkflowResult {
  ok: boolean;
  message: string;
  redirect?: string;
}

const caseIdSchema = z.string().min(1, "Case id is required");
const approvalIdSchema = z.string().min(1, "Approval id is required");
const decisionSchema = z.enum(["APPROVE", "REJECT", "REQUEST_INFO"]);

function fail(message: string): WorkflowResult {
  return { ok: false, message };
}

async function loadCase(caseId: string) {
  return db.case.findUnique({
    where: { id: caseId },
    include: {
      invoice: { include: { lines: true, po: { include: { lines: true } } } },
      supplier: true,
      discrepancies: true,
      documents: true,
    },
  });
}

async function shipmentDeliveriesFor(poId: string | null) {
  if (!poId) return [];
  const shipment = await db.shipment.findFirst({
    where: { poId },
    include: { deliveryEvents: { include: { lines: true } } },
  });
  return shipment?.deliveryEvents ?? [];
}

function fmtMoney(amount: number, currency = "USD"): string {
  return new Intl.NumberFormat("en-US", { style: "currency", currency, maximumFractionDigits: 0 }).format(amount);
}

export async function requestDocument(rawCaseId: string, actorName: string, role: Role): Promise<WorkflowResult> {
  const parsed = caseIdSchema.safeParse(rawCaseId);
  if (!parsed.success) return fail("Invalid case.");
  const kase = await loadCase(parsed.data);
  if (!kase) return fail("Case not found.");

  const perms = userPermissionsFor(role);
  if (!perms.canExecute.includes("request_document")) {
    await recordAuditEvent({
      actor: actorName,
      actorType: "HUMAN",
      action: "policy.action_blocked",
      objectType: "case",
      objectId: kase.id,
      caseId: kase.id,
      inputs: { action: "request_document" },
      outputs: { allowed: false, reason: "insufficient_role" },
    });
    return fail("Your role cannot request documents from suppliers.");
  }

  await recordAuditEvent({
    actor: actorName,
    actorType: "HUMAN",
    action: "action.approved",
    objectType: "case",
    objectId: kase.id,
    caseId: kase.id,
    inputs: { action: "request_document" },
    outputs: { status: "WAITING_SUPPLIER" },
  });
  await recordAuditEvent({
    actor: "Atlas Outbound",
    actorType: "SYSTEM",
    action: "supplier.document_request_sent",
    objectType: "supplier",
    objectId: kase.supplier.code,
    caseId: kase.id,
    inputs: { method: "email", to: kase.supplier.contactEmail, requested: "corrected_invoice" },
    outputs: { simulated: true },
  });

  await db.case.update({
    where: { id: kase.id },
    data: { status: "WAITING_SUPPLIER", updatedAt: new Date() },
  });
  await db.invoice.update({
    where: { id: kase.invoiceId },
    data: { status: "WAITING_SUPPLIER" },
  });
  await db.resolution.create({
    data: {
      caseId: kase.id,
      type: "DOCUMENT_REQUESTED",
      note: `Requested corrected invoice from ${kase.supplier.name}.`,
      actorId: role === "AGENT" ? null : await actorIdByName(actorName),
      createdAt: new Date(),
    },
  });

  return { ok: true, message: `Document request sent to ${kase.supplier.name}.`, redirect: "/work" };
}

export async function holdPayment(rawCaseId: string, actorName: string, role: Role): Promise<WorkflowResult> {
  const parsed = caseIdSchema.safeParse(rawCaseId);
  if (!parsed.success) return fail("Invalid case.");
  const kase = await loadCase(parsed.data);
  if (!kase) return fail("Case not found.");

  const perms = userPermissionsFor(role);
  if (!perms.canExecute.includes("hold_payment")) {
    await recordAuditEvent({
      actor: actorName,
      actorType: "HUMAN",
      action: "policy.action_blocked",
      objectType: "case",
      objectId: kase.id,
      caseId: kase.id,
      inputs: { action: "hold_payment" },
      outputs: { allowed: false, reason: "insufficient_role" },
    });
    return fail("Your role cannot hold payments.");
  }

  await recordAuditEvent({
    actor: actorName,
    actorType: "HUMAN",
    action: "action.executed",
    objectType: "case",
    objectId: kase.id,
    caseId: kase.id,
    inputs: { action: "hold_payment" },
    outputs: { status: "HELD" },
  });
  await recordAuditEvent({
    actor: "Atlas ERP Adapter",
    actorType: "SYSTEM",
    action: "erp.payment_blocked",
    objectType: "invoice",
    objectId: kase.invoice.number,
    caseId: kase.id,
    inputs: { reason: "hold_payment" },
    outputs: { erpStatus: "BLOCKED" },
  });

  await db.case.update({
    where: { id: kase.id },
    data: { status: "RESOLVED", resolvedAt: new Date(), updatedAt: new Date() },
  });
  await db.invoice.update({
    where: { id: kase.invoiceId },
    data: { status: "HELD", erpStatus: "BLOCKED" },
  });
  await db.resolution.create({
    data: {
      caseId: kase.id,
      type: "HELD",
      note: "Payment held pending resolution with supplier.",
      actorId: await actorIdByName(actorName),
      createdAt: new Date(),
    },
  });

  return { ok: true, message: `Payment held for ${kase.invoice.number}.`, redirect: "/work" };
}

export async function escalateCase(rawCaseId: string, actorName: string, role: Role): Promise<WorkflowResult> {
  const parsed = caseIdSchema.safeParse(rawCaseId);
  if (!parsed.success) return fail("Invalid case.");
  const kase = await loadCase(parsed.data);
  if (!kase) return fail("Case not found.");

  const perms = userPermissionsFor(role);
  if (!perms.canExecute.includes("escalate_case")) {
    await recordAuditEvent({
      actor: actorName,
      actorType: "HUMAN",
      action: "policy.action_blocked",
      objectType: "case",
      objectId: kase.id,
      caseId: kase.id,
      inputs: { action: "escalate_case" },
      outputs: { allowed: false, reason: "insufficient_role" },
    });
    return fail("Your role cannot escalate cases.");
  }

  await recordAuditEvent({
    actor: actorName,
    actorType: "HUMAN",
    action: "case.escalated",
    objectType: "case",
    objectId: kase.id,
    caseId: kase.id,
    inputs: { team: "procurement" },
    outputs: { status: "ESCALATED" },
  });
  await recordAuditEvent({
    actor: "Atlas Outbound",
    actorType: "SYSTEM",
    action: "procurement.notified",
    objectType: "case",
    objectId: kase.id,
    caseId: kase.id,
    inputs: { channel: "email" },
    outputs: { simulated: true },
  });

  await db.case.update({
    where: { id: kase.id },
    data: { status: "ESCALATED", escalated: true, updatedAt: new Date() },
  });
  await db.resolution.create({
    data: {
      caseId: kase.id,
      type: "ESCALATED",
      note: "Escalated to procurement for follow-up.",
      actorId: await actorIdByName(actorName),
      createdAt: new Date(),
    },
  });

  return { ok: true, message: `Case escalated to procurement.`, redirect: "/work" };
}

export async function approvePayment(rawCaseId: string, actorName: string, role: Role): Promise<WorkflowResult> {
  const parsed = caseIdSchema.safeParse(rawCaseId);
  if (!parsed.success) return fail("Invalid case.");
  const kase = await loadCase(parsed.data);
  if (!kase) return fail("Case not found.");

  const policy = await getPolicyConfig();
  const deliveries = await shipmentDeliveriesFor(kase.invoice.poId);
  const comparison = kase.invoice.lines.map((l) => {
    const received = deliveries.flatMap((d) => d.lines).filter((dl) => dl.item === l.item).reduce((s, dl) => s + dl.quantity, 0);
    return { item: l.item, invoiced: l.invoicedQuantity, received };
  });
  const unresolvedQuantity = comparison.some((c) => c.invoiced > c.received) || kase.discrepancies.some((d) => d.type === "QUANTITY_MISMATCH" && d.status === "OPEN");
  const missingDelivery = deliveries.length === 0;
  const hasRequiredEvidence =
    kase.documents.some((d) => d.kind === "INVOICE_PDF") &&
    kase.documents.some((d) => d.kind === "PURCHASE_ORDER") &&
    kase.documents.some((d) => d.kind === "DELIVERY_NOTE");

  const evaluation = evaluatePolicy(
    {
      action: "approve_payment",
      role,
      amount: kase.amount,
      currency: kase.currency,
      unresolvedQuantityMismatch: unresolvedQuantity,
      missingDeliveryRecord: missingDelivery,
      priceVariancePct: null,
      unresolvedDiscrepancyTypes: kase.discrepancies.filter((d) => d.status === "OPEN").map((d) => d.type),
      hasRequiredEvidence,
    },
    policy
  );

  if (!evaluation.allowed) {
    await recordAuditEvent({
      actor: actorName,
      actorType: "HUMAN",
      action: "policy.action_blocked",
      objectType: "case",
      objectId: kase.id,
      caseId: kase.id,
      inputs: { action: "approve_payment" },
      outputs: { allowed: false, reason: evaluation.blockedReason },
    });
    return fail(evaluation.blockedReason ?? "Blocked by policy.");
  }

  if (evaluation.requiresApproval) {
    if (!userPermissionsFor(role).canPropose.includes("approve_payment")) {
      return fail("Your role cannot propose payment approvals.");
    }
    const count = await db.approval.count();
    const reference = `APR-${200 + count}`;
    await db.approval.create({
      data: {
        reference,
        caseId: kase.id,
        action: "approve_payment",
        amount: kase.amount,
        currency: kase.currency,
        status: "PENDING",
        rationale: `Amount ${fmtMoney(kase.amount)} exceeds the $${policy.approvalThreshold.toLocaleString()} approval threshold. Proposed by ${actorName}.`,
        requestedBy: actorName,
        createdAt: new Date(),
      },
    });
    await db.case.update({ where: { id: kase.id }, data: { status: "READY_FOR_APPROVAL", updatedAt: new Date() } });
    await recordAuditEvent({
      actor: actorName,
      actorType: "HUMAN",
      action: "payment.proposed_for_approval",
      objectType: "approval",
      objectId: reference,
      caseId: kase.id,
      inputs: { action: "approve_payment", amount: kase.amount },
      outputs: { approval: reference },
    });
    await recordAuditEvent({
      actor: "Atlas Engine",
      actorType: "SYSTEM",
      action: "policy.controller_approval_required",
      objectType: "approval",
      objectId: reference,
      caseId: kase.id,
      inputs: { threshold: policy.approvalThreshold },
      outputs: { result: "controller_approval_required" },
    });
    return { ok: true, message: `Payment routed to the controller for approval (${reference}).`, redirect: "/work" };
  }

  // Direct release (low value, policy allows)
  return releasePayment(kase.id, actorName);
}

async function releasePayment(caseId: string, actorName: string): Promise<WorkflowResult> {
  const kase = await loadCase(caseId);
  if (!kase) return fail("Case not found.");
  await db.case.update({ where: { id: caseId }, data: { status: "RESOLVED", resolvedAt: new Date(), updatedAt: new Date() } });
  await db.invoice.update({
    where: { id: kase.invoiceId },
    data: { status: "PAID", erpStatus: "PAYMENT_RELEASED" },
  });
  await recordAuditEvent({
    actor: actorName,
    actorType: "HUMAN",
    action: "action.executed",
    objectType: "case",
    objectId: caseId,
    caseId,
    inputs: { action: "approve_payment" },
    outputs: { status: "PAID" },
  });
  await recordAuditEvent({
    actor: "Atlas ERP Adapter",
    actorType: "SYSTEM",
    action: "erp.payment_released",
    objectType: "invoice",
    objectId: kase.invoice.number,
    caseId,
    inputs: { erpDocument: `ERP-PAY-${kase.invoice.number.slice(-4)}` },
    outputs: { erpStatus: "PAYMENT_RELEASED" },
  });
  await recordAuditEvent({
    actor: "Atlas Engine",
    actorType: "SYSTEM",
    action: "invoice.marked_paid",
    objectType: "invoice",
    objectId: kase.invoice.number,
    caseId,
    inputs: {},
    outputs: { status: "PAID" },
  });
  await db.resolution.create({
    data: {
      caseId,
      type: "PAYMENT_RELEASED",
      note: `Payment of ${fmtMoney(kase.amount)} approved by ${actorName} and released in the mock ERP.`,
      actorId: await actorIdByName(actorName),
      createdAt: new Date(),
    },
  });
  return { ok: true, message: `Payment of ${fmtMoney(kase.amount)} released.`, redirect: "/work" };
}

export async function recordCorrectedDocument(rawCaseId: string, actorName: string): Promise<WorkflowResult> {
  const parsed = caseIdSchema.safeParse(rawCaseId);
  if (!parsed.success) return fail("Invalid case.");
  const kase = await loadCase(parsed.data);
  if (!kase) return fail("Case not found.");
  if (kase.status !== "WAITING_SUPPLIER") return fail("This case is not waiting on the supplier.");

  const policy = await getPolicyConfig();
  await recordAuditEvent({
    actor: "Atlas Ingest",
    actorType: "SYSTEM",
    action: "corrected_invoice.received",
    objectType: "invoice",
    objectId: kase.invoice.number,
    caseId: kase.id,
    inputs: { source: "email", supplier: kase.supplier.name },
    outputs: { documents: 1 },
  });
  await db.document.create({
    data: {
      kind: "CORRECTED_INVOICE",
      name: `${kase.invoice.number}_corrected.pdf`,
      caseId: kase.id,
      invoiceId: kase.invoiceId,
      supplierId: kase.supplierId,
      createdAt: new Date(),
    },
  });
  for (const d of kase.discrepancies.filter((x) => x.status === "OPEN")) {
    await db.discrepancy.update({ where: { id: d.id }, data: { status: "RESOLVED" } });
    await recordAuditEvent({
      actor: "Atlas Engine",
      actorType: "SYSTEM",
      action: "discrepancy.resolved",
      objectType: "discrepancy",
      objectId: d.reference,
      caseId: kase.id,
      inputs: { resolvedBy: "corrected_document" },
      outputs: { status: "RESOLVED" },
    });
  }

  if (kase.amount > policy.approvalThreshold) {
    const count = await db.approval.count();
    const reference = `APR-${200 + count}`;
    await db.approval.create({
      data: {
        reference,
        caseId: kase.id,
        action: "approve_payment",
        amount: kase.amount,
        currency: kase.currency,
        status: "PENDING",
        rationale: `Quantity variance was resolved with corrected invoice. Amount exceeds the $${policy.approvalThreshold.toLocaleString()} approval threshold.`,
        requestedBy: actorName,
        createdAt: new Date(),
      },
    });
    await db.case.update({ where: { id: kase.id }, data: { status: "READY_FOR_APPROVAL", updatedAt: new Date() } });
    await db.invoice.update({ where: { id: kase.invoiceId }, data: { status: "HELD" } });
    await recordAuditEvent({
      actor: actorName,
      actorType: "HUMAN",
      action: "payment.proposed_for_approval",
      objectType: "approval",
      objectId: reference,
      caseId: kase.id,
      inputs: { action: "approve_payment", amount: kase.amount },
      outputs: { approval: reference },
    });
    await recordAuditEvent({
      actor: "Atlas Engine",
      actorType: "SYSTEM",
      action: "policy.controller_approval_required",
      objectType: "approval",
      objectId: reference,
      caseId: kase.id,
      inputs: { threshold: policy.approvalThreshold },
      outputs: { result: "controller_approval_required" },
    });
    return { ok: true, message: `Corrected invoice recorded. Routed to the controller for approval (${reference}).`, redirect: "/work" };
  }

  return releasePayment(kase.id, actorName);
}

export async function decideApproval(
  rawApprovalId: string,
  rawDecision: string,
  actorName: string,
  role: Role,
  comment?: string
): Promise<WorkflowResult> {
  const idParsed = approvalIdSchema.safeParse(rawApprovalId);
  const decParsed = decisionSchema.safeParse(rawDecision);
  if (!idParsed.success) return fail("Invalid approval.");
  if (!decParsed.success) return fail("Invalid decision.");
  if (role !== "CONTROLLER") {
    await recordAuditEvent({
      actor: actorName,
      actorType: "HUMAN",
      action: "policy.action_blocked",
      objectType: "approval",
      objectId: idParsed.data,
      inputs: { decision: decParsed.data },
      outputs: { allowed: false, reason: "controller_role_required" },
    });
    return fail("Policy requires controller approval for this decision.");
  }

  const approval = await db.approval.findUnique({
    where: { id: idParsed.data },
    include: { case: { include: { invoice: true, supplier: true } } },
  });
  if (!approval) return fail("Approval not found.");
  if (approval.status !== "PENDING") return fail("This approval has already been decided.");

  const decidedById = await actorIdByName(actorName);
  const caseId = approval.caseId;

  if (decParsed.data === "APPROVE") {
    await db.approval.update({
      where: { id: approval.id },
      data: { status: "APPROVED", decidedById, decidedAt: new Date(), comment: comment ?? null },
    });
    await db.case.update({ where: { id: caseId }, data: { status: "RESOLVED", resolvedAt: new Date(), updatedAt: new Date() } });
    await db.invoice.update({
      where: { id: approval.case.invoiceId },
      data: { status: "PAID", erpStatus: "PAYMENT_RELEASED" },
    });
    await db.resolution.create({
      data: {
        caseId,
        type: "PAYMENT_RELEASED",
        note: `Payment of ${fmtMoney(approval.amount)} approved by ${actorName} and released in the mock ERP.`,
        actorId: decidedById,
        createdAt: new Date(),
      },
    });
    await recordAuditEvent({
      actor: actorName,
      actorType: "HUMAN",
      action: "approval.decided",
      objectType: "approval",
      objectId: approval.reference,
      caseId,
      inputs: { decision: "APPROVED", comment: comment ?? null },
      outputs: { status: "APPROVED" },
    });
    await recordAuditEvent({
      actor: "Atlas ERP Adapter",
      actorType: "SYSTEM",
      action: "erp.payment_released",
      objectType: "invoice",
      objectId: approval.case.invoice.number,
      caseId,
      inputs: { erpDocument: `ERP-PAY-${approval.case.invoice.number.slice(-4)}` },
      outputs: { erpStatus: "PAYMENT_RELEASED" },
    });
    await recordAuditEvent({
      actor: "Atlas Engine",
      actorType: "SYSTEM",
      action: "invoice.marked_paid",
      objectType: "invoice",
      objectId: approval.case.invoice.number,
      caseId,
      inputs: {},
      outputs: { status: "PAID" },
    });
    return { ok: true, message: "Payment approval recorded.", redirect: "/approvals" };
  }

  if (decParsed.data === "REJECT") {
    await db.approval.update({
      where: { id: approval.id },
      data: { status: "REJECTED", decidedById, decidedAt: new Date(), comment: comment ?? null },
    });
    await db.case.update({ where: { id: caseId }, data: { status: "RESOLVED", resolvedAt: new Date(), updatedAt: new Date() } });
    await db.invoice.update({
      where: { id: approval.case.invoiceId },
      data: { status: "REJECTED", erpStatus: "NOT_POSTED" },
    });
    await db.resolution.create({
      data: {
        caseId,
        type: "INVOICE_REJECTED",
        note: `Payment rejected by ${actorName}.`,
        actorId: decidedById,
        createdAt: new Date(),
      },
    });
    await recordAuditEvent({
      actor: actorName,
      actorType: "HUMAN",
      action: "approval.decided",
      objectType: "approval",
      objectId: approval.reference,
      caseId,
      inputs: { decision: "REJECTED", comment: comment ?? null },
      outputs: { status: "REJECTED" },
    });
    return { ok: true, message: "Payment rejected. Case resolved.", redirect: "/approvals" };
  }

  // REQUEST_INFO
  await db.approval.update({
    where: { id: approval.id },
    data: { status: "INFO_REQUESTED", decidedById, decidedAt: new Date(), comment: comment ?? null },
  });
  await db.case.update({ where: { id: caseId }, data: { status: "WAITING_SUPPLIER", updatedAt: new Date() } });
  await db.resolution.create({
    data: {
      caseId,
      type: "DOCUMENT_REQUESTED",
      note: `More information requested by ${actorName}.`,
      actorId: decidedById,
      createdAt: new Date(),
    },
  });
  await recordAuditEvent({
    actor: actorName,
    actorType: "HUMAN",
    action: "approval.info_requested",
    objectType: "approval",
    objectId: approval.reference,
    caseId,
    inputs: { decision: "INFO_REQUESTED" },
    outputs: { status: "INFO_REQUESTED" },
  });
  await recordAuditEvent({
    actor: "Atlas Outbound",
    actorType: "SYSTEM",
    action: "supplier.document_request_sent",
    objectType: "supplier",
    objectId: approval.case.supplier.code,
    caseId,
    inputs: { method: "email", to: approval.case.supplier.contactEmail },
    outputs: { simulated: true },
  });
  return { ok: true, message: "Information request sent to the supplier.", redirect: "/approvals" };
}

export async function resetDemo(): Promise<WorkflowResult> {
  await resetAndSeed();
  return { ok: true, message: "Demo data reset to the original seed.", redirect: "/work" };
}

async function actorIdByName(name: string): Promise<string | null> {
  const user = await db.user.findFirst({ where: { name } });
  return user?.id ?? null;
}
