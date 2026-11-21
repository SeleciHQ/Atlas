"use client";

import * as React from "react";
import Link from "next/link";
import { ChevronRight } from "lucide-react";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ApprovalStatusBadge } from "@/components/shared/badges";
import { EmptyState } from "@/components/shared/empty-state";
import { formatCurrency, relativeTime } from "@/lib/format";

export interface ApprovalRecord {
  id: string;
  reference: string;
  invoiceNumber: string;
  supplierName: string;
  supplierCode: string;
  amount: number;
  currency: string;
  action: string;
  status: string;
  rationale: string;
  requestedBy: string;
  decidedByName: string | null;
  decidedAt: string | null;
  evidenceCount: number;
  createdAt: string;
}

const TABS = [
  { key: "PENDING", label: "Pending" },
  { key: "APPROVED", label: "Approved" },
  { key: "REJECTED", label: "Rejected" },
  { key: "ALL", label: "All" },
] as const;

export function ApprovalsView({
  approvals,
  canDecide,
}: {
  approvals: ApprovalRecord[];
  canDecide: boolean;
}) {
  const [tab, setTab] = React.useState<string>("PENDING");

  const pending = approvals.filter((a) => a.status === "PENDING");
  const pendingTotal = pending.reduce((s, a) => s + a.amount, 0);

  const visible =
    tab === "ALL" ? approvals : approvals.filter((a) => a.status === tab);

  return (
    <div>
      {/* Summary */}
      <div className="mb-4 rounded-lg border border-border bg-surface px-4 py-3 shadow-xs">
        <div className="flex flex-wrap items-center gap-x-6 gap-y-1.5 text-[12.5px] text-muted-foreground">
          <span>
            <span className="num font-semibold text-foreground">{pending.length}</span> pending decisions
          </span>
          <span>
            <span className="num font-semibold text-foreground">{formatCurrency(pendingTotal)}</span> in proposed payments
          </span>
          <span className="hidden md:inline">
            Authorization belongs to the organization — agent proposals require a human decision.
          </span>
          {!canDecide && (
            <span className="rounded bg-warning-bg px-2 py-0.5 text-[11.5px] font-medium text-warning">
              View only — controller role required to decide
            </span>
          )}
        </div>
      </div>

      <Tabs value={tab} onValueChange={setTab} className="gap-3">
        <TabsList>
          {TABS.map((t) => {
            const count = t.key === "ALL" ? approvals.length : approvals.filter((a) => a.status === t.key).length;
            return (
              <TabsTrigger key={t.key} value={t.key}>
                {t.label}
                <span className="num text-[11px] opacity-70">{count}</span>
              </TabsTrigger>
            );
          })}
        </TabsList>
      </Tabs>

      <div className="mt-3">
        {visible.length === 0 ? (
          <EmptyState
            title={tab === "PENDING" ? "No pending approvals" : `No ${tab.toLowerCase()} approvals`}
            description={
              tab === "PENDING"
                ? "High-value payment proposals above the $5,000 threshold will appear here."
                : undefined
            }
          />
        ) : (
          <div className="overflow-hidden rounded-lg border border-border bg-surface shadow-xs">
            <ul className="divide-y divide-border">
              {visible.map((a) => (
                <li key={a.id}>
                  <Link
                    href={`/approvals/${a.reference}`}
                    className="group flex w-full items-center gap-4 px-4 py-3.5 transition-colors hover:bg-surface-subtle focus-visible:bg-brand-soft focus-visible:outline-none"
                  >
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-x-2.5 gap-y-1">
                        <span className="font-mono text-[11.5px] text-muted-foreground">{a.reference}</span>
                        <span className="font-mono text-[13px] font-semibold text-foreground">{a.invoiceNumber}</span>
                        <span className="text-[13px] text-muted-foreground">·</span>
                        <span className="text-[13px] font-medium text-foreground">{a.supplierName}</span>
                        <ApprovalStatusBadge status={a.status} />
                      </div>
                      <p className="mt-1 truncate text-[12.5px] text-muted-foreground">
                        {a.action.replace(/_/g, " ")} · requested by {a.requestedBy} · {relativeTime(a.createdAt)}
                        {a.decidedByName ? ` · decided by ${a.decidedByName}` : ""}
                      </p>
                    </div>
                    <div className="flex shrink-0 items-center gap-4">
                      <span className="num text-[13px] font-semibold text-foreground">
                        {formatCurrency(a.amount, a.currency)}
                      </span>
                      <ChevronRight
                        className="h-4 w-4 text-[#c1c7d0] transition-colors group-hover:text-muted-foreground"
                        aria-hidden="true"
                      />
                    </div>
                  </Link>
                </li>
              ))}
            </ul>
          </div>
        )}
      </div>
    </div>
  );
}
