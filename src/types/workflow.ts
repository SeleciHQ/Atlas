import type { Role, ActionType, ExceptionType, CaseStatus, ActorType } from "./ontology";

export interface SupplierSummary {
  id: string;
  code: string;
  name: string;
  country: string;
  status: string;
  contactEmail: string;
}

export interface LineQuantityComparison {
  item: string;
  ordered: number;
  invoiced: number;
  received: number;
  difference: number;
  passed: boolean;
}

export interface CaseQueueItem {
  id: string;
  invoiceNumber: string;
  supplierId: string;
  supplierName: string;
  amount: number;
  currency: string;
  exceptionType: ExceptionType;
  status: CaseStatus;
  createdAt: Date;
  ageDays: number;
  recommendation: string;
  highValue: boolean;
  escalated: boolean;
  headline: string;
  detail: string;
  // Master-detail extras
  poNumber: string | null;
  shipmentReference: string | null;
  discrepancySummary: string;
  evidenceCount: number;
  confidence: "HIGH" | "MEDIUM" | "LOW";
  relatedApprovalReference: string | null;
  lastUpdatedAt: Date;
  approveBlockedReason: string | null;
  requiresApproval: boolean;
}

export interface ApprovalQueueItem {
  id: string;
  reference: string;
  caseId: string;
  invoiceNumber: string;
  supplierName: string;
  supplierId: string;
  amount: number;
  currency: string;
  proposedAction: string;
  rationale: string;
  evidenceCount: number;
  createdAt: Date;
}

export interface AuditEventView {
  id: string;
  actor: string;
  actorType: ActorType;
  action: string;
  objectType: string;
  objectId: string;
  caseId: string | null;
  inputs: string;
  outputs: string;
  policyVersion: string;
  createdAt: Date;
}

export interface PolicyRuleView {
  label: string;
  detail: string;
}

export interface PolicyEvaluation {
  allowed: boolean;
  requiresApproval: boolean;
  violations: PolicyViolation[];
  blockedReason: string | null;
  reason: string;
}

export interface PolicyViolation {
  rule: string;
  detail: string;
}

export interface AgentReview {
  recommendedAction: ActionType;
  recommendation: string;
  confidence: "HIGH" | "MEDIUM" | "LOW";
  evidenceCount: number;
  reasoningSteps: string[];
  matchedPoNumber: string | null;
  matchedDeliveryRefs: string[];
  priceVariancePct: number | null;
}

export interface RolePermissions {
  role: Role;
  canExecute: ActionType[];
  canPropose: ActionType[];
}

export interface LineComparison {
  item: string;
  ordered: number;
  invoiced: number;
  received: number;
  difference: number;
  passed: boolean;
}

export interface CaseQueueGroup {
  key: string;
  title: string;
  items: CaseQueueItem[];
}

export interface WorkQueueGroup {
  key: string;
  title: string;
  items: CaseQueueItem[];
}
