import type { ActorType } from "./ontology";

export interface AuditEventInput {
  actor: string;
  actorType: ActorType;
  action: string;
  objectType: string;
  objectId: string;
  caseId?: string | null;
  inputs?: Record<string, unknown>;
  outputs?: Record<string, unknown>;
  policyVersion?: string;
  createdAt?: Date;
}

export interface AuditEventRecord extends AuditEventInput {
  id: string;
}
