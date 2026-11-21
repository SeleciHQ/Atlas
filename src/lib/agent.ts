import { DEFAULT_POLICY_CONFIG } from "../types/policy";
import type { AgentReview, LineComparison } from "../types/workflow";
import type { ActionType, ExceptionType } from "../types/ontology";
import type { PolicyConfig } from "../types/policy";

export interface AgentCaseInput {
  invoiceNumber: string;
  exceptionType: ExceptionType;
  caseStatus: string;
  amount: number;
  currency: string;
  poNumber: string | null;
  poLines: { item: string; quantity: number; unitPrice: number }[];
  invoiceLines: { item: string; invoicedQuantity: number; unitPrice: number }[];
  deliveries: { reference: string; type: string; lines: { item: string; quantity: number }[] }[];
  discrepancyTypes: string[];
  supplierPriorIssues: number;
  taxDocMissing: boolean;
  duplicateOf: string | null;
  evidenceCount: number;
}

export function buildLineComparison(input: AgentCaseInput): LineComparison[] {
  const items = new Map<string, { ordered: number; invoiced: number; received: number }>();
  for (const line of input.poLines) {
    const entry = items.get(line.item) ?? { ordered: 0, invoiced: 0, received: 0 };
    entry.ordered += line.quantity;
    items.set(line.item, entry);
  }
  for (const line of input.invoiceLines) {
    const entry = items.get(line.item) ?? { ordered: 0, invoiced: 0, received: 0 };
    entry.invoiced += line.invoicedQuantity;
    items.set(line.item, entry);
  }
  for (const delivery of input.deliveries) {
    for (const line of delivery.lines) {
      const entry = items.get(line.item) ?? { ordered: 0, invoiced: 0, received: 0 };
      entry.received += line.quantity;
      items.set(line.item, entry);
    }
  }
  const rows: LineComparison[] = [];
  for (const [item, q] of items) {
    rows.push({
      item,
      ordered: q.ordered,
      invoiced: q.invoiced,
      received: q.received,
      difference: q.invoiced - q.received,
      passed: q.invoiced <= q.received,
    });
  }
  return rows;
}

export function computePriceVariance(input: AgentCaseInput): number | null {
  const poPrices = new Map<string, number>();
  for (const line of input.poLines) poPrices.set(line.item, line.unitPrice);
  let max = 0;
  let found = false;
  for (const line of input.invoiceLines) {
    const base = poPrices.get(line.item);
    if (base !== undefined && base > 0) {
      const pct = Math.abs(line.unitPrice - base) / base * 100;
      if (pct > max) max = pct;
      found = true;
    }
  }
  return found ? Math.round(max * 100) / 100 : null;
}

const CONFIDENCE_BY_TYPE: Record<string, AgentReview["confidence"]> = {
  QUANTITY_MISMATCH: "HIGH",
  PRICE_VARIANCE: "HIGH",
  MISSING_DELIVERY: "HIGH",
  DUPLICATE_INVOICE: "HIGH",
  LATE_DELIVERY: "MEDIUM",
  MISSING_TAX_DOC: "HIGH",
  UNAPPROVED: "MEDIUM",
  PARTIAL_DELIVERY: "MEDIUM",
};

export function analyzeCase(input: AgentCaseInput, config: PolicyConfig = DEFAULT_POLICY_CONFIG): AgentReview {
  const comparison = buildLineComparison(input);
  const failedLines = comparison.filter((r) => !r.passed);
  const variance = computePriceVariance(input);
  const unresolved = new Set(input.discrepancyTypes);
  const allResolved = input.discrepancyTypes.length === 0 || (unresolved.size === 0);

  const reasoningSteps: string[] = [];
  let action: ActionType = "hold_payment";
  let recommendation = "Hold payment and request corrected invoice.";

  if (input.poNumber) {
    reasoningSteps.push(`Matched invoice ${input.invoiceNumber} to ${input.poNumber}.`);
  } else {
    reasoningSteps.push(`No purchase order matched invoice ${input.invoiceNumber}.`);
  }
  if (input.deliveries.length > 0) {
    reasoningSteps.push(`Retrieved delivery event ${input.deliveries[0].reference}.`);
  } else {
    reasoningSteps.push("No delivery event found for this purchase order.");
  }

  const comparedItems = comparison.filter((r) => r.received > 0 || r.invoiced > 0);
  if (comparedItems.length > 0) {
    reasoningSteps.push(`Compared line quantities for ${comparedItems.map((r) => r.item).join(", ")}.`);
  }
  for (const line of failedLines) {
    reasoningSteps.push(
      `Found ${Math.abs(line.difference)}-unit difference for ${line.item} (invoiced ${line.invoiced}, received ${line.received}).`
    );
  }
  if (variance !== null) {
    reasoningSteps.push(
      `Checked price tolerance: ${variance.toFixed(1)}% variance versus ${config.priceTolerancePct}% allowed.`
    );
  }

  const policyChecks: string[] = [];
  if (failedLines.length > 0) {
    policyChecks.push("invoice quantity cannot exceed received quantity");
  }
  if (variance !== null && variance > config.priceTolerancePct) {
    policyChecks.push(`price variance above ${config.priceTolerancePct}% requires review`);
  }
  if (input.duplicateOf) policyChecks.push("duplicate invoice detected");
  if (input.taxDocMissing) policyChecks.push("tax document required");
  if (policyChecks.length > 0) {
    reasoningSteps.push(`Checked policy: ${policyChecks.join("; ")}.`);
  }

  reasoningSteps.push(
    `Checked supplier history: ${input.supplierPriorIssues} prior ${input.supplierPriorIssues === 1 ? "discrepancy" : "discrepancies"}.`
  );

  if (allResolved) {
    action = "approve_payment";
    recommendation = `Approve payment of ${formatUsd(input.amount)}.`;
    reasoningSteps.push(`Discrepancies resolved. Proposed approve_payment.`);
  } else if (input.duplicateOf) {
    action = "reject_invoice";
    recommendation = "Reject invoice as a duplicate of an already recorded invoice.";
    reasoningSteps.push("Proposed reject_invoice.");
  } else if (input.discrepancyTypes.includes("UNAPPROVED")) {
    action = "escalate_case";
    recommendation = "Escalate to procurement to confirm the purchase order.";
    reasoningSteps.push("Proposed escalate_case.");
  } else if (input.discrepancyTypes.includes("MISSING_DELIVERY")) {
    action = "hold_payment";
    recommendation = "Hold payment until a delivery record is located.";
    reasoningSteps.push("Proposed hold_payment.");
  } else if (input.discrepancyTypes.includes("QUANTITY_MISMATCH") || input.discrepancyTypes.includes("PARTIAL_DELIVERY")) {
    action = "hold_payment";
    recommendation = "Hold payment and request corrected invoice.";
    reasoningSteps.push("Proposed hold_payment and request_document.");
  } else if (input.discrepancyTypes.includes("MISSING_TAX_DOC")) {
    action = "request_document";
    recommendation = "Request the missing tax document before payment.";
    reasoningSteps.push("Proposed request_document.");
  } else if (input.discrepancyTypes.includes("PRICE_VARIANCE")) {
    action = "request_document";
    recommendation = "Request price clarification from the supplier before payment.";
    reasoningSteps.push("Proposed request_document.");
  } else {
    action = "approve_payment";
    recommendation = `Approve payment of ${formatUsd(input.amount)}.`;
    reasoningSteps.push("Proposed approve_payment.");
  }

  return {
    recommendedAction: action,
    recommendation,
    confidence: CONFIDENCE_BY_TYPE[input.exceptionType] ?? "MEDIUM",
    evidenceCount: input.evidenceCount,
    reasoningSteps,
    matchedPoNumber: input.poNumber,
    matchedDeliveryRefs: input.deliveries.map((d) => d.reference),
    priceVariancePct: variance,
  };
}

function formatUsd(amount: number): string {
  return new Intl.NumberFormat("en-US", { style: "currency", currency: "USD", maximumFractionDigits: 0 }).format(amount);
}
