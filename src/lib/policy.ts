import { db } from "@/lib/db";
import { DEFAULT_POLICY_CONFIG, POLICY_VERSION, userPermissionsFor } from "@/types/policy";
import type { PolicyConfig } from "@/types/policy";
import type { PolicyEvaluation, PolicyViolation } from "@/types/workflow";
import type { ActionType, Role } from "@/types/ontology";

export interface PolicyContext {
  action: ActionType;
  role: Role;
  amount: number;
  currency: string;
  unresolvedQuantityMismatch: boolean;
  missingDeliveryRecord: boolean;
  priceVariancePct: number | null;
  unresolvedDiscrepancyTypes: string[];
  hasRequiredEvidence: boolean;
}

export async function getPolicyConfig(): Promise<PolicyConfig> {
  try {
    const row = await db.policy.findFirst({ where: { key: DEFAULT_POLICY_CONFIG.key, active: true } });
    if (row) {
      const parsed = JSON.parse(row.config) as Partial<PolicyConfig>;
      return { ...DEFAULT_POLICY_CONFIG, ...parsed, version: row.version };
    }
  } catch {
    // fall through to defaults when the database is not seeded yet
  }
  return DEFAULT_POLICY_CONFIG;
}

export function evaluatePolicy(ctx: PolicyContext, config: PolicyConfig): PolicyEvaluation {
  const violations: PolicyViolation[] = [];
  const perms = userPermissionsFor(ctx.role);

  if (ctx.action === "approve_payment") {
    if (!perms.canPropose.includes("approve_payment") && !perms.canExecute.includes("approve_payment")) {
      violations.push({ rule: "agent_cannot_approve", detail: "Agents may propose payment approval but cannot execute it." });
    }
    if (ctx.unresolvedQuantityMismatch) {
      violations.push({
        rule: "quantity_rule",
        detail: "Invoices with unresolved quantity mismatch cannot be approved.",
      });
    }
    if (ctx.missingDeliveryRecord) {
      violations.push({
        rule: "required_evidence",
        detail: "A delivery record is required before payment can be approved.",
      });
    }
    if (ctx.priceVariancePct !== null && ctx.priceVariancePct > config.priceTolerancePct) {
      violations.push({
        rule: "price_tolerance",
        detail: `Price variance of ${ctx.priceVariancePct.toFixed(1)}% exceeds the ${config.priceTolerancePct}% tolerance. Review is required.`,
      });
    }
    if (!ctx.hasRequiredEvidence) {
      violations.push({
        rule: "required_evidence",
        detail: "Required evidence (invoice, purchase order, delivery record) must be attached.",
      });
    }
  }

  const hardBlocked =
    ctx.action === "approve_payment" &&
    (ctx.unresolvedQuantityMismatch ||
      ctx.missingDeliveryRecord ||
      !ctx.hasRequiredEvidence);

  const requiresApproval = ctx.action === "approve_payment" && ctx.amount > config.approvalThreshold;

  const blockedReason = hardBlocked
    ? violations[0]?.detail ?? "Action blocked by policy."
    : null;

  return {
    allowed: !hardBlocked,
    requiresApproval,
    violations,
    blockedReason,
    reason: hardBlocked ? "blocked" : requiresApproval ? "controller_approval_required" : "allowed",
  };
}

export function policySummaryLines(config: PolicyConfig): string[] {
  return [
    `Human approval required for payments above $${config.approvalThreshold.toLocaleString()}.`,
    "Agents may propose payment approval but cannot execute it.",
    `Agents may request documents and escalate cases.`,
    `Invoice quantity cannot exceed received quantity.`,
    `Price variance above ${config.priceTolerancePct}% requires review.`,
  ];
}

export { POLICY_VERSION };
