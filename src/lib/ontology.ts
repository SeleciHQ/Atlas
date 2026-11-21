import type { DocumentKind } from "@/types/ontology";

export const EVIDENCE_META: Record<
  DocumentKind,
  { label: string; description: string }
> = {
  INVOICE_PDF: { label: "Invoice PDF", description: "Supplier invoice document with extracted fields" },
  PURCHASE_ORDER: { label: "Purchase order", description: "Approved order document from the ERP" },
  DELIVERY_NOTE: { label: "Delivery note", description: "Goods received note from the warehouse" },
  DELIVERY_PHOTO: { label: "Delivery photo", description: "Photo taken at goods receipt" },
  TAX_DOC: { label: "Tax document", description: "VAT certificate or tax exemption form" },
  CORRECTED_INVOICE: { label: "Corrected invoice", description: "Reissued invoice from the supplier" },
};

export function evidenceLabel(kind: string): string {
  return EVIDENCE_META[kind as DocumentKind]?.label ?? kind;
}

export function invoicePath(invoiceNumber: string): string {
  return `/invoices/${invoiceNumber}`;
}

export function supplierPath(code: string): string {
  return `/suppliers/${code}`;
}

export function casePathFor(invoiceNumber: string): string {
  return `/work/${invoiceNumber}`;
}
