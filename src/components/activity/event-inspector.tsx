"use client";

import * as React from "react";
import Link from "next/link";
import { ActorBadge } from "@/components/shared/badges";
import { StatusBadge } from "@/components/shared/status-badge";
import { JsonBlock } from "@/components/shared/json-block";
import { formatDate, formatTime } from "@/lib/format";
import type { AuditEventView } from "@/types/workflow";
import { cn } from "@/lib/utils";

function objectHref(objectType: string, objectId: string, caseId: string | null): string | null {
  switch (objectType) {
    case "case":
      return `/work/${objectId}`;
    case "invoice":
      return `/invoices/${objectId}`;
    case "supplier":
      return `/suppliers/${objectId}`;
    case "approval":
      return `/approvals/${objectId}`;
    case "purchase_order":
      return `/work/${objectId}`;
    case "discrepancy":
    case "delivery_event":
      return `/work/${objectId}`;
    default:
      return caseId ? `/work/${objectId}` : null;
  }
}

function resultTone(action: string): { text: string; tone: "success" | "danger" | "neutral" } {
  if (action.includes("blocked") || action.includes("failed")) return { text: "Blocked", tone: "danger" };
  if (action.includes("proposed")) return { text: "Proposed", tone: "neutral" };
  if (action.includes("created") || action.includes("executed") || action.includes("approved") || action.includes("decided")) {
    return { text: "Executed", tone: "success" };
  }
  return { text: "Recorded", tone: "neutral" };
}

export function EventInspector({ event }: { event: AuditEventView }) {
  const href = objectHref(event.objectType, event.objectId, event.caseId);
  const result = resultTone(event.action);

  return (
    <div className="flex flex-col gap-5">
      {/* Result */}
      <div className="flex flex-wrap items-center gap-2">
        <StatusBadge tone={result.tone}>{result.text}</StatusBadge>
        <ActorBadge type={event.actorType} />
      </div>

      {/* Fields */}
      <dl className="flex flex-col divide-y divide-border">
        <Field label="Action">
          <span className="font-mono text-[12.5px]">{event.action}</span>
        </Field>
        <Field label="Actor">{event.actor}</Field>
        <Field label="Object">
          {href ? (
            <Link
              href={href}
              className="inline-flex items-center gap-1 font-mono text-[12.5px] text-primary underline-offset-4 hover:underline"
            >
              {event.objectType} · {event.objectId}
            </Link>
          ) : (
            <span className="font-mono text-[12.5px]">
              {event.objectType} · {event.objectId}
            </span>
          )}
        </Field>
        <Field label="Policy version">
          <span className="font-mono text-[12.5px]">{event.policyVersion}</span>
        </Field>
        <Field label="Timestamp">
          {formatDate(event.createdAt)} · {formatTime(event.createdAt)}
        </Field>
      </dl>

      {/* Inputs */}
      <div>
        <h4 className="mb-2 text-[11px] font-semibold uppercase tracking-widest text-muted-foreground">
          Inputs
        </h4>
        <JsonBlock value={event.inputs} maxHeight={220} />
      </div>

      {/* Outputs */}
      <div>
        <h4 className="mb-2 text-[11px] font-semibold uppercase tracking-widest text-muted-foreground">
          Outputs
        </h4>
        <JsonBlock value={event.outputs} maxHeight={280} />
      </div>

      {/* Open related object (CTA) */}
      {href && (
        <div className="border-t pt-3">
          <Link
            href={href}
            className="inline-flex h-8 items-center justify-center rounded-md border border-border bg-surface px-3 text-[12.5px] font-medium text-foreground shadow-xs hover:bg-surface-subtle"
          >
            Open {event.objectType === "case" ? "case workspace" : event.objectType}
            <span className="ml-1.5 text-muted-foreground">→</span>
          </Link>
        </div>
      )}
    </div>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-baseline justify-between gap-4 py-2">
      <dt className="text-[12.5px] text-muted-foreground">{label}</dt>
      <dd className={cn("text-right text-[13px] font-medium text-foreground")}>{children}</dd>
    </div>
  );
}
