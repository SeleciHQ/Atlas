import type { ActionType, Role } from "./ontology";

export const POLICY_VERSION = "payment-policy-v3";

export interface PolicyConfig {
  key: string;
  name: string;
  version: string;
  priceTolerancePct: number;
  quantityRule: "invoice_quantity <= received_quantity";
  approvalThreshold: number;
  requiredEvidence: string[];
  currency: string;
}

export const DEFAULT_POLICY_CONFIG: PolicyConfig = {
  key: "invoice_reconciliation",
  name: "Payment policy v3",
  version: POLICY_VERSION,
  priceTolerancePct: 2,
  quantityRule: "invoice_quantity <= received_quantity",
  approvalThreshold: 5000,
  requiredEvidence: ["invoice", "purchase_order", "delivery_event"],
  currency: "USD",
};

export function userPermissionsFor(role: Role): { canExecute: ActionType[]; canPropose: ActionType[] } {
  switch (role) {
    case "FINANCE_OPS":
      return {
        canExecute: ["request_document", "hold_payment", "escalate_case", "reject_invoice", "create_discrepancy"],
        canPropose: ["approve_payment"],
      };
    case "CONTROLLER":
      return {
        canExecute: [
          "request_document",
          "hold_payment",
          "escalate_case",
          "reject_invoice",
          "create_discrepancy",
          "approve_payment",
        ],
        canPropose: ["approve_payment"],
      };
    case "AGENT":
      return {
        canExecute: ["request_document", "escalate_case", "create_discrepancy"],
        canPropose: ["approve_payment", "hold_payment"],
      };
  }
}

export function canRead(role: Role, objectType: string): boolean {
  // Both demo roles are internal finance staff with read access to all objects.
  void objectType;
  void role;
  return true;
}
