import { StatusBadge, type StatusTone } from "./status-badge";
import {
  CASE_STATUS_LABELS,
  INVOICE_STATUS_LABELS,
  EXCEPTION_LABELS,
  type CaseStatus,
  type InvoiceStatus,
  type ExceptionType,
} from "@/types/ontology";

type StatusEntry = { label: string; tone: StatusTone };

const CASE_STATUS_META: Record<CaseStatus, StatusEntry> = {
  OPEN: { label: "Open", tone: "warning" },
  WAITING_SUPPLIER: { label: "Waiting on supplier", tone: "info" },
  READY_FOR_APPROVAL: { label: "Ready for approval", tone: "brand" },
  ESCALATED: { label: "Escalated", tone: "warning" },
  RESOLVED: { label: "Resolved", tone: "success" },
};

const INVOICE_STATUS_META: Record<InvoiceStatus, StatusEntry> = {
  RECEIVED: { label: "Received", tone: "neutral" },
  IN_REVIEW: { label: "In review", tone: "warning" },
  WAITING_SUPPLIER: { label: "Waiting on supplier", tone: "info" },
  HELD: { label: "Held", tone: "warning" },
  APPROVED: { label: "Approved", tone: "brand" },
  PAID: { label: "Paid", tone: "success" },
  REJECTED: { label: "Rejected", tone: "danger" },
};

const APPROVAL_STATUS_META: Record<string, StatusEntry> = {
  PENDING: { label: "Pending", tone: "warning" },
  APPROVED: { label: "Approved", tone: "success" },
  REJECTED: { label: "Rejected", tone: "danger" },
  INFO_REQUESTED: { label: "Info requested", tone: "info" },
};

const ACTOR_META: Record<string, { label: string; tone: StatusTone }> = {
  HUMAN: { label: "Human", tone: "info" },
  AGENT: { label: "Agent", tone: "brand" },
  SYSTEM: { label: "System", tone: "neutral" },
};

const SUPPLIER_STATUS_META: Record<string, StatusEntry> = {
  ACTIVE: { label: "Active", tone: "success" },
  PROBATION: { label: "Probation", tone: "warning" },
  ON_HOLD: { label: "On hold", tone: "danger" },
};

function render(
  meta: { label: string; tone: StatusTone } | undefined,
  fallback: string
) {
  return (
    <StatusBadge tone={meta?.tone ?? "neutral"}>{meta?.label ?? fallback}</StatusBadge>
  );
}

export function CaseStatusBadge({ status, ...rest }: { status: string; children?: null }) {
  void rest;
  return render(CASE_STATUS_META[status as CaseStatus] ?? { label: CASE_STATUS_LABELS[status as CaseStatus] ?? status }, status);
}

export function InvoiceStatusBadge({ status }: { status: string }) {
  return render(INVOICE_STATUS_META[status as InvoiceStatus] ?? { label: INVOICE_STATUS_LABELS[status as InvoiceStatus] ?? status }, status);
}

export function ApprovalStatusBadge({ status }: { status: string }) {
  return render(APPROVAL_STATUS_META[status] ?? { label: status, tone: "neutral" }, status);
}

export function ActorBadge({ type }: { type: string }) {
  const meta = ACTOR_META[type];
  return render(meta, type.toLowerCase());
}

export function SupplierStatusBadge({ status }: { status: string }) {
  return render(SUPPLIER_STATUS_META[status] ?? { label: status.toLowerCase(), tone: "neutral" }, status);
}

export function ExceptionBadge({ type }: { type: string }) {
  return (
    <span className="text-[12.5px] text-muted-foreground">
      {EXCEPTION_LABELS[type as ExceptionType] ?? type}
    </span>
  );
}
