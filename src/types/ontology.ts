export type Role = "FINANCE_OPS" | "CONTROLLER" | "AGENT";

export type ActorType = "HUMAN" | "AGENT" | "SYSTEM";

export type ExceptionType =
  | "QUANTITY_MISMATCH"
  | "PRICE_VARIANCE"
  | "MISSING_DELIVERY"
  | "DUPLICATE_INVOICE"
  | "LATE_DELIVERY"
  | "MISSING_TAX_DOC"
  | "UNAPPROVED"
  | "PARTIAL_DELIVERY";

export type CaseStatus =
  | "OPEN"
  | "WAITING_SUPPLIER"
  | "READY_FOR_APPROVAL"
  | "ESCALATED"
  | "RESOLVED";

export type InvoiceStatus =
  | "RECEIVED"
  | "IN_REVIEW"
  | "WAITING_SUPPLIER"
  | "HELD"
  | "APPROVED"
  | "PAID"
  | "REJECTED";

export type DiscrepancyType = ExceptionType;

export type ActionType =
  | "create_discrepancy"
  | "request_document"
  | "hold_payment"
  | "approve_payment"
  | "reject_invoice"
  | "escalate_case";

export type ApprovalStatus = "PENDING" | "APPROVED" | "REJECTED" | "INFO_REQUESTED";

export type DocumentKind =
  | "INVOICE_PDF"
  | "PURCHASE_ORDER"
  | "DELIVERY_NOTE"
  | "DELIVERY_PHOTO"
  | "TAX_DOC"
  | "CORRECTED_INVOICE";

export const EXCEPTION_LABELS: Record<ExceptionType, string> = {
  QUANTITY_MISMATCH: "Quantity mismatch",
  PRICE_VARIANCE: "Price variance",
  MISSING_DELIVERY: "Missing delivery record",
  DUPLICATE_INVOICE: "Duplicate invoice",
  LATE_DELIVERY: "Late delivery",
  MISSING_TAX_DOC: "Missing tax document",
  UNAPPROVED: "Unapproved invoice",
  PARTIAL_DELIVERY: "Partial delivery",
};

export const CASE_STATUS_LABELS: Record<CaseStatus, string> = {
  OPEN: "Open",
  WAITING_SUPPLIER: "Waiting on supplier",
  READY_FOR_APPROVAL: "Ready for approval",
  ESCALATED: "Escalated",
  RESOLVED: "Resolved",
};

export const INVOICE_STATUS_LABELS: Record<InvoiceStatus, string> = {
  RECEIVED: "Received",
  IN_REVIEW: "In review",
  WAITING_SUPPLIER: "Waiting on supplier",
  HELD: "Held",
  APPROVED: "Approved",
  PAID: "Paid",
  REJECTED: "Rejected",
};

export const ACTION_LABELS: Record<ActionType, string> = {
  create_discrepancy: "Create discrepancy",
  request_document: "Request corrected document",
  hold_payment: "Hold payment",
  approve_payment: "Approve payment",
  reject_invoice: "Reject invoice",
  escalate_case: "Escalate to procurement",
};

export function isExceptionType(v: string): v is ExceptionType {
  return v in EXCEPTION_LABELS;
}

export function isCaseStatus(v: string): v is CaseStatus {
  return v in CASE_STATUS_LABELS;
}

export function isInvoiceStatus(v: string): v is InvoiceStatus {
  return v in INVOICE_STATUS_LABELS;
}
