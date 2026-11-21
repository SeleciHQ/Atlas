import { db } from "@/lib/db";
import { analyzeCase, buildLineComparison, computePriceVariance, type AgentCaseInput } from "@/lib/agent";
import { getPolicyConfig, evaluatePolicy, policySummaryLines } from "@/lib/policy";import { getSessionUser, type SessionUser } from "@/lib/session";
import type { CaseQueueItem, ApprovalQueueItem, AgentReview, LineComparison, AuditEventView, PolicyEvaluation, PolicyRuleView, CaseQueueGroup } from "@/types/workflow";
import { EXCEPTION_LABELS, type ExceptionType, type CaseStatus, type ActorType } from "@/types/ontology";
import { DEFAULT_POLICY_CONFIG, POLICY_VERSION } from "@/types/policy";
import type { Case, Invoice, InvoiceLine, Supplier, PurchaseOrder, PurchaseOrderLine, Shipment, DeliveryEvent, DeliveryEventLine, Discrepancy, Document, Approval, Resolution, User, AuditEvent } from "@/generated/prisma/client";

const DAY = 86_400_000;

function exceptionLabel(t: string): string {
  return EXCEPTION_LABELS[t as ExceptionType] ?? t;
}

function priorityScore(item: CaseQueueItem): number {
  let score = 0;
  if (item.escalated) score += 3;
  if (item.highValue) score += 2;
  score += item.amount / 100_000;
  return score;
}

function toQueueItem(c: Case & { invoice: Invoice; supplier: Supplier }): CaseQueueItem {
  const now = new Date();
  return {
    id: c.id,
    invoiceNumber: c.invoice.number,
    supplierId: c.supplier.code,
    supplierName: c.supplier.name,
    amount: c.amount,
    currency: c.currency,
    exceptionType: c.exceptionType as ExceptionType,
    status: c.status as CaseStatus,
    createdAt: c.createdAt,
    ageDays: Math.floor((now.getTime() - c.createdAt.getTime()) / DAY),
    recommendation: "",
    highValue: c.highValue,
    escalated: c.escalated,
    headline: exceptionLabel(c.exceptionType),
    detail: "",
    poNumber: null,
    shipmentReference: null,
    discrepancySummary: "",
    evidenceCount: 0,
    confidence: "MEDIUM",
    relatedApprovalReference: null,
    lastUpdatedAt: c.updatedAt,
    approveBlockedReason: null,
    requiresApproval: false,
  };
}

function detailFor(invoiced: number, received: number, hasDelivery: boolean): string {
  if (!hasDelivery) return "No delivery record on file";
  return `Invoiced ${invoiced.toLocaleString()} · Received ${received.toLocaleString()}`;
}

export interface CaseEvidenceInput {
  po: (PurchaseOrder & { lines: PurchaseOrderLine[] }) | null;
  invoiceLines: InvoiceLine[];
  deliveries: (DeliveryEvent & { lines: DeliveryEventLine[] })[];
  discrepancyTypes: string[];
  priorIssues: number;
  evidenceCount: number;
}

export function agentInputFrom(
  invoice: Invoice,
  evidence: CaseEvidenceInput
): AgentCaseInput {
  return {
    invoiceNumber: invoice.number,
    exceptionType: "QUANTITY_MISMATCH",
    caseStatus: "OPEN",
    amount: invoice.amount,
    currency: invoice.currency,
    poNumber: evidence.po?.number ?? null,
    poLines: (evidence.po?.lines ?? []).map((l) => ({ item: l.item, quantity: l.quantity, unitPrice: l.unitPrice })),
    invoiceLines: evidence.invoiceLines.map((l) => ({ item: l.item, invoicedQuantity: l.invoicedQuantity, unitPrice: l.unitPrice })),
    deliveries: evidence.deliveries.map((d) => ({
      reference: d.reference,
      type: d.type,
      lines: d.lines.map((l) => ({ item: l.item, quantity: l.quantity })),
    })),
    discrepancyTypes: evidence.discrepancyTypes,
    supplierPriorIssues: evidence.priorIssues,
    taxDocMissing: invoice.taxDocMissing,
    duplicateOf: invoice.duplicateOf,
    evidenceCount: evidence.evidenceCount,
  };
}

export async function getWorkData(): Promise<{
  user: SessionUser;
  groups: CaseQueueGroup[];
  stats: { open: number; needsDecision: number; waitingSupplier: number; readyForApproval: number; resolvedToday: number };
  approvals: ApprovalQueueItem[];
  totalProposed: number;
}> {
  const user = await getSessionUser();
  const cases = await db.case.findMany({
    include: { invoice: true, supplier: true },
  });
  const policy = await getPolicyConfig();

  const startOfToday = new Date();
  startOfToday.setHours(0, 0, 0, 0);

  const queueItems: (CaseQueueItem & { resolvedAt: Date | null })[] = [];
  for (const c of cases) {
    const evidence = await evidenceForCase(c.id);
    if (!evidence) continue;
    const input = agentInputFrom(evidence.invoice, evidence);
    input.exceptionType = c.exceptionType as ExceptionType;
    input.caseStatus = c.status;
    const agent = analyzeCase(input, policy);
    const invoicedTotal = evidence.invoiceLines.reduce((s, l) => s + l.invoicedQuantity, 0);
    const receivedTotal = evidence.deliveries.flatMap((d) => d.lines).reduce((s, l) => s + l.quantity, 0);
    const approvalRow = await db.approval.findFirst({
      where: { caseId: c.id },
      orderBy: { createdAt: "desc" },
      select: { reference: true },
    });
    const unresolvedQuantity =
      evidence.deliveries.length > 0
        ? evidence.invoiceLines.some((l) => {
            const received = evidence.deliveries
              .flatMap((d) => d.lines)
              .filter((dl) => dl.item === l.item)
              .reduce((s, dl) => s + dl.quantity, 0);
            return l.invoicedQuantity > received;
          }) || evidence.discrepancyTypes.includes("QUANTITY_MISMATCH")
        : evidence.discrepancyTypes.includes("QUANTITY_MISMATCH");
    const missingDelivery = evidence.deliveries.length === 0 && !evidence.invoice.duplicateOf;
    const evaluation = evaluatePolicy(
      {
        action: "approve_payment",
        role: user.role,
        amount: c.amount,
        currency: c.currency,
        unresolvedQuantityMismatch: unresolvedQuantity,
        missingDeliveryRecord: missingDelivery,
        priceVariancePct: null,
        unresolvedDiscrepancyTypes: evidence.discrepancyTypes,
        hasRequiredEvidence:
          evidence.evidenceCount >= 3 &&
          evidence.deliveries.length > 0 &&
          Boolean(evidence.po),
      },
      policy
    );
    queueItems.push({
      ...toQueueItem(c),
      recommendation: agent.recommendation,
      resolvedAt: c.resolvedAt,
      detail: detailFor(invoicedTotal, receivedTotal, evidence.deliveries.length > 0),
      poNumber: evidence.po?.number ?? null,
      shipmentReference: evidence.shipment?.reference ?? null,
      discrepancySummary: evidence.discrepancies[0]?.description ?? "",
      evidenceCount: evidence.evidenceCount,
      confidence: agent.confidence,
      relatedApprovalReference: approvalRow?.reference ?? null,
      lastUpdatedAt: c.updatedAt,
      approveBlockedReason: evaluation.allowed ? null : evaluation.blockedReason,
      requiresApproval: evaluation.requiresApproval,
    });
  }

  const byStatus = (status: CaseStatus) =>
    queueItems
      .filter((i) => i.status === status)
      .sort(
        (a, b) =>
          priorityScore(b) - priorityScore(a) || b.amount - a.amount || a.createdAt.getTime() - b.createdAt.getTime()
      );

  const needsDecision = byStatus("OPEN");
  const waitingSupplier = byStatus("WAITING_SUPPLIER");
  const readyForApproval = byStatus("READY_FOR_APPROVAL");
  const escalated = byStatus("ESCALATED");
  const resolvedToday = queueItems
    .filter((i) => i.status === "RESOLVED" && i.resolvedAt && i.resolvedAt >= startOfToday)
    .sort((a, b) => (b.resolvedAt?.getTime() ?? 0) - (a.resolvedAt?.getTime() ?? 0));

  const groups: CaseQueueGroup[] = [
    { key: "needs-decision", title: "Needs your decision", items: needsDecision },
    { key: "waiting-supplier", title: "Waiting on supplier", items: waitingSupplier },
    { key: "ready-for-approval", title: "Ready for approval", items: readyForApproval },
    ...(escalated.length > 0 ? [{ key: "escalated", title: "Escalated", items: escalated }] : []),
    { key: "resolved-today", title: "Resolved today", items: resolvedToday },
  ];

  const approvals = await getPendingApprovals();

  return {
    user,
    groups,
    stats: {
      open: queueItems.filter((i) => i.status !== "RESOLVED").length,
      needsDecision: needsDecision.length,
      waitingSupplier: waitingSupplier.length,
      readyForApproval: readyForApproval.length,
      resolvedToday: resolvedToday.length,
    },
    approvals,
    totalProposed: approvals.reduce((s, a) => s + a.amount, 0),
  };
}

async function evidenceForCase(caseId: string) {
  const c = await db.case.findUnique({
    where: { id: caseId },
    include: {
      invoice: { include: { lines: true, po: { include: { lines: true } }, documents: true } },
      supplier: true,
      discrepancies: true,
    },
  });
  if (!c) return null;
  const shipment = c.invoice.poId
    ? await db.shipment.findFirst({
        where: { poId: c.invoice.poId },
        include: { deliveryEvents: { include: { lines: true } } },
      })
    : null;
  const priorIssues = await db.case.count({
    where: { supplierId: c.supplierId, id: { not: c.id } },
  });
  const docCount = await db.document.count({ where: { caseId: c.id } });
  return {
    invoice: c.invoice,
    po: c.invoice.po ?? null,
    invoiceLines: c.invoice.lines,
    discrepancies: c.discrepancies,
    deliveries: shipment?.deliveryEvents ?? [],
    discrepancyTypes: c.discrepancies.filter((d) => d.status === "OPEN").map((d) => d.type),
    allDiscrepancyTypes: c.discrepancies.map((d) => d.type),
    priorIssues,
    evidenceCount: docCount,
    supplier: c.supplier,
    shipment,
  };
}

export interface CaseDetail {
  caseId: string;
  status: CaseStatus;
  exceptionType: ExceptionType;
  summary: string;
  ageDays: number;
  createdAt: Date;
  updatedAt: Date;
  invoice: Invoice & { lines: InvoiceLine[] };
  supplier: Supplier;
  po: (PurchaseOrder & { lines: PurchaseOrderLine[] }) | null;
  shipment: (Shipment & { deliveryEvents: (DeliveryEvent & { lines: DeliveryEventLine[] })[] }) | null;
  documents: Document[];
  discrepancies: Discrepancy[];
  approvals: (Approval & { decidedBy: User | null })[];
  resolutions: (Resolution & { actor: User | null })[];
  auditEvents: AuditEvent[];
  supplierHistory: { invoiceNumber: string; exceptionType: string; status: string; amount: number; createdAt: Date }[];
  agent: AgentReview;
  comparison: LineComparison[];
  priceVariancePct: number | null;
  policyRules: PolicyRuleView[];
  approveEvaluation: PolicyEvaluation;
  role: SessionUser["role"];
  currentUserName: string;
  canDecideApprovals: boolean;
}

export async function getCaseDetail(invoiceNumber: string): Promise<CaseDetail | null> {
  const user = await getSessionUser();
  const invoice = await db.invoice.findUnique({
    where: { number: invoiceNumber },
    include: { lines: true, case: true, supplier: true, po: { include: { lines: true } } },
  });
  if (!invoice || !invoice.case) return null;
  const kase = invoice.case;

  const shipment = invoice.poId
    ? await db.shipment.findFirst({
        where: { poId: invoice.poId },
        include: { deliveryEvents: { include: { lines: true }, orderBy: { reference: "asc" } } },
      })
    : null;

  const [documents, discrepancies, approvals, resolutions, auditEvents] = await Promise.all([
    db.document.findMany({ where: { caseId: kase.id }, orderBy: { createdAt: "asc" } }),
    db.discrepancy.findMany({ where: { caseId: kase.id }, orderBy: { reference: "asc" } }),
    db.approval.findMany({ where: { caseId: kase.id }, include: { decidedBy: true }, orderBy: { createdAt: "desc" } }),
    db.resolution.findMany({ where: { caseId: kase.id }, include: { actor: true }, orderBy: { createdAt: "desc" } }),
    db.auditEvent.findMany({ where: { caseId: kase.id }, orderBy: { createdAt: "asc" } }),
  ]);

  const supplierHistory = await db.case.findMany({
    where: { supplierId: invoice.supplierId, id: { not: kase.id } },
    include: { invoice: true },
    orderBy: { createdAt: "desc" },
    take: 6,
  });

  const policy = await getPolicyConfig();
  const openDiscrepancyTypes = discrepancies.filter((d) => d.status === "OPEN").map((d) => d.type);
  const input: AgentCaseInput = {
    invoiceNumber: invoice.number,
    exceptionType: kase.exceptionType as ExceptionType,
    caseStatus: kase.status,
    amount: invoice.amount,
    currency: invoice.currency,
    poNumber: invoice.po?.number ?? null,
    poLines: (invoice.po?.lines ?? []).map((l) => ({ item: l.item, quantity: l.quantity, unitPrice: l.unitPrice })),
    invoiceLines: invoice.lines.map((l) => ({ item: l.item, invoicedQuantity: l.invoicedQuantity, unitPrice: l.unitPrice })),
    deliveries: (shipment?.deliveryEvents ?? []).map((d) => ({
      reference: d.reference,
      type: d.type,
      lines: d.lines.map((l) => ({ item: l.item, quantity: l.quantity })),
    })),
    discrepancyTypes: openDiscrepancyTypes,
    supplierPriorIssues: supplierHistory.length,
    taxDocMissing: invoice.taxDocMissing,
    duplicateOf: invoice.duplicateOf,
    evidenceCount: documents.length,
  };
  const agent = analyzeCase(input, policy);
  const comparison = buildLineComparison(input);
  const priceVariancePct = computePriceVariance(input);

  const missingDelivery = (shipment?.deliveryEvents.length ?? 0) === 0 && !invoice.duplicateOf;
  const unresolvedQuantity = comparison.some((r) => !r.passed) || openDiscrepancyTypes.includes("QUANTITY_MISMATCH");
  const hasRequiredEvidence =
    documents.some((d) => d.kind === "INVOICE_PDF") &&
    documents.some((d) => d.kind === "PURCHASE_ORDER") &&
    documents.some((d) => d.kind === "DELIVERY_NOTE");

  const approveEvaluation = evaluatePolicy(
    {
      action: "approve_payment",
      role: user.role,
      amount: invoice.amount,
      currency: invoice.currency,
      unresolvedQuantityMismatch: unresolvedQuantity,
      missingDeliveryRecord: missingDelivery,
      priceVariancePct,
      unresolvedDiscrepancyTypes: openDiscrepancyTypes,
      hasRequiredEvidence,
    },
    policy
  );

  const policyRules = policySummaryLines(policy).map((line) => ({
    label: line.split(".")[0] + ".",
    detail: line.slice(line.indexOf(".") + 1).trim(),
  }));

  return {
    caseId: kase.id,
    status: kase.status as CaseStatus,
    exceptionType: kase.exceptionType as ExceptionType,
    summary: kase.summary,
    ageDays: Math.floor((Date.now() - kase.createdAt.getTime()) / DAY),
    createdAt: kase.createdAt,
    updatedAt: kase.updatedAt,
    invoice,
    supplier: invoice.supplier,
    po: invoice.po ?? null,
    shipment: shipment ?? null,
    documents,
    discrepancies,
    approvals,
    resolutions,
    auditEvents,
    supplierHistory: supplierHistory.map((h) => ({
      invoiceNumber: h.invoice.number,
      exceptionType: h.exceptionType,
      status: h.status,
      amount: h.amount,
      createdAt: h.createdAt,
    })),
    agent,
    comparison,
    priceVariancePct,
    policyRules,
    approveEvaluation,
    role: user.role,
    currentUserName: user.name,
    canDecideApprovals: user.role === "CONTROLLER",
  };
}

export async function getInvoicesList() {
  const invoices = await db.invoice.findMany({
    include: { supplier: true, case: true, po: true },
    orderBy: { invoiceDate: "desc" },
  });
  return invoices;
}

export async function getInvoiceDetail(invoiceNumber: string) {
  const invoice = await db.invoice.findUnique({
    where: { number: invoiceNumber },
    include: {
      supplier: true,
      po: { include: { lines: true } },
      lines: true,
      case: true,
      documents: true,
    },
  });
  if (!invoice) return null;
  const shipment = invoice.poId
    ? await db.shipment.findFirst({
        where: { poId: invoice.poId },
        include: { deliveryEvents: { include: { lines: true } } },
      })
    : null;
  return { invoice, shipment };
}

export async function getSuppliersList() {
  const suppliers = await db.supplier.findMany({
    include: { invoices: { include: { case: true } }, cases: { where: { status: { not: "RESOLVED" } } }, purchaseOrders: true, shipments: true },
    orderBy: { name: "asc" },
  });
  return suppliers.map((s) => ({
    ...s,
    totalSpend: s.invoices.filter((i) => i.status === "PAID").reduce((sum, i) => sum + i.amount, 0),
    invoiceCount: s.invoices.length,
    openDiscrepancies: s.cases.length,
  }));
}

export async function getSupplierDetail(code: string) {
  const supplier = await db.supplier.findUnique({
    where: { code },
    include: {
      purchaseOrders: true,
      shipments: { include: { po: true, deliveryEvents: true } },
      invoices: { include: { case: true, po: true }, orderBy: { invoiceDate: "desc" } },
      cases: { include: { invoice: true, discrepancies: true }, orderBy: { createdAt: "desc" } },
    },
  });
  if (!supplier) return null;
  const totalSpend = supplier.invoices.filter((i) => i.status === "PAID").reduce((sum, i) => sum + i.amount, 0);
  const openDiscrepancies = supplier.cases.filter((c) => c.status !== "RESOLVED").length;
  return { supplier, totalSpend, openDiscrepancies };
}

export async function getApprovalsList() {
  const user = await getSessionUser();
  const approvals = await db.approval.findMany({
    include: { case: { include: { invoice: true, supplier: true, documents: true } }, decidedBy: true },
    orderBy: { createdAt: "desc" },
  });
  return { user, approvals };
}

export async function getApprovalDetail(reference: string) {
  const user = await getSessionUser();
  const approval = await db.approval.findUnique({
    where: { reference },
    include: {
      case: {
        include: {
          invoice: { include: { po: { include: { lines: true } }, lines: true } },
          supplier: true,
          documents: true,
          discrepancies: true,
        },
      },
      decidedBy: true,
    },
  });
  if (!approval) return null;
  return { user, approval };
}

export async function getPendingApprovals(): Promise<ApprovalQueueItem[]> {
  const approvals = await db.approval.findMany({
    where: { status: "PENDING" },
    include: { case: { include: { invoice: true, supplier: true, documents: true } } },
    orderBy: { createdAt: "asc" },
  });
  return approvals.map((a) => ({
    id: a.id,
    reference: a.reference,
    caseId: a.caseId,
    invoiceNumber: a.case.invoice.number,
    supplierName: a.case.supplier.name,
    supplierId: a.case.supplier.code,
    amount: a.amount,
    currency: a.currency,
    proposedAction: `Approve payment of ${new Intl.NumberFormat("en-US", { style: "currency", currency: a.currency, maximumFractionDigits: 0 }).format(a.amount)}`,
    rationale: a.rationale,
    evidenceCount: a.case.documents.length,
    createdAt: a.createdAt,
  }));
}

export async function getActivityEvents(): Promise<AuditEventView[]> {
  const events = await db.auditEvent.findMany({ orderBy: { createdAt: "desc" }, take: 300 });
  return events.map((e) => ({
    id: e.id,
    actor: e.actor,
    actorType: e.actorType as ActorType,
    action: e.action,
    objectType: e.objectType,
    objectId: e.objectId,
    caseId: e.caseId,
    inputs: e.inputs,
    outputs: e.outputs,
    policyVersion: e.policyVersion,
    createdAt: e.createdAt,
  }));
}

export async function getSearchIndex(): Promise<{ label: string; sub: string; href: string; kind: string }[]> {
  const [cases, suppliers, approvals] = await Promise.all([
    db.case.findMany({ include: { invoice: true, supplier: true } }),
    db.supplier.findMany({ orderBy: { name: "asc" } }),
    db.approval.findMany({ where: { status: "PENDING" }, include: { case: { include: { invoice: true, supplier: true } } } }),
  ]);
  return [
    ...cases.map((c) => ({
      label: c.invoice.number,
      sub: `${c.supplier.name} · ${exceptionLabel(c.exceptionType)}`,
      href: `/work/${c.invoice.number}`,
      kind: "Case",
    })),
    ...suppliers.map((s) => ({ label: s.name, sub: `${s.code} · ${s.country}`, href: `/suppliers/${s.code}`, kind: "Supplier" })),
    ...approvals.map((a) => ({
      label: a.reference,
      sub: `${a.case.invoice.number} · ${a.case.supplier.name}`,
      href: `/approvals/${a.reference}`,
      kind: "Approval",
    })),
  ];
}

export async function getSettingsData() {
  const policy = await getPolicyConfig();
  const users = await db.user.findMany({ orderBy: { name: "asc" } });
  return {
    policy,
    rules: policySummaryLines(policy),
    users: users.map((u) => ({ name: u.name, email: u.email, role: u.role })),
  };
}

export async function getShellCounts(): Promise<{ work: number; approvals: number }> {
  const [work, approvals] = await Promise.all([
    db.case.count({ where: { status: "OPEN" } }),
    db.approval.count({ where: { status: "PENDING" } }),
  ]);
  return { work, approvals };
}

export { POLICY_VERSION, DEFAULT_POLICY_CONFIG };
