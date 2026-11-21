import { db } from "./db";
import { POLICY_VERSION } from "../types/policy";
import type { AuditEventInput } from "../types/audit";

export async function recordAuditEvent(input: AuditEventInput) {
  return db.auditEvent.create({
    data: {
      actor: input.actor,
      actorType: input.actorType,
      action: input.action,
      objectType: input.objectType,
      objectId: input.objectId,
      caseId: input.caseId ?? null,
      inputs: JSON.stringify(input.inputs ?? {}),
      outputs: JSON.stringify(input.outputs ?? {}),
      policyVersion: input.policyVersion ?? POLICY_VERSION,
      createdAt: input.createdAt ?? new Date(),
    },
  });
}
