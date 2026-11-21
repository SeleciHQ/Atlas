"use client";

import * as React from "react";
import Link from "next/link";
import { ChevronDown, ChevronRight, ArrowRight, CircleAlert } from "lucide-react";
import { DetailDrawer } from "@/components/shared/detail-drawer";
import { StatusBadge } from "@/components/shared/status-badge";
import { EmptyState } from "@/components/shared/empty-state";
import { CaseStatusBadge } from "@/components/shared/badges";
import { CaseActionPanel } from "@/components/case/action-panel";
import { formatCurrency, relativeTime } from "@/lib/format";
import { cn } from "@/lib/utils";
import type { CaseQueueGroup, CaseQueueItem } from "@/types/workflow";

const FILTERS = [
  { key: "all", label: "All" },
  { key: "needs-decision", label: "Needs my decision" },
  { key: "waiting-supplier", label: "Waiting on supplier" },
  { key: "ready-for-approval", label: "Ready for approval" },
  { key: "high-value", label: "High value" },
  { key: "escalated", label: "Escalated" },
  { key: "resolved", label: "Resolved" },
] as const;

type FilterKey = (typeof FILTERS)[number]["key"];

function matchesFilter(item: CaseQueueItem, filter: FilterKey): boolean {
  switch (filter) {
    case "all":
      return true;
    case "needs-decision":
      return item.status === "OPEN";
    case "waiting-supplier":
      return item.status === "WAITING_SUPPLIER";
    case "ready-for-approval":
      return item.status === "READY_FOR_APPROVAL";
    case "high-value":
      return item.highValue;
    case "escalated":
      return item.escalated;
    case "resolved":
      return item.status === "RESOLVED";
  }
}

export function WorkView({
  groups,
  stats,
  role,
  name,
}: {
  groups: CaseQueueGroup[];
  stats: { open: number; needsDecision: number };
  role: string;
  name: string;
}) {
  const first = name.split(" ")[0];
  const [filter, setFilter] = React.useState<FilterKey>("all");
  const [collapsed, setCollapsed] = React.useState<Record<string, boolean>>({});
  const [selected, setSelected] = React.useState<CaseQueueItem | null>(null);

  const visible = React.useMemo(
    () =>
      groups
        .map((g) => ({ ...g, items: g.items.filter((i) => matchesFilter(i, filter)) }))
        .filter((g) => g.items.length > 0),
    [groups, filter]
  );

  return (
    <div>
      {/* Header */}
      <header className="mb-6">
        <h1 className="text-[22px] font-semibold leading-7 tracking-[-0.01em] text-foreground">
          Good day, {first}.
        </h1>
        {role !== "CONTROLLER" && (
          <p className="mt-1.5 text-[13px] text-muted-foreground">
            You have{" "}
            <span className="font-semibold text-foreground">{stats.open} open invoice exceptions</span>.{" "}
            <span className="font-semibold text-foreground">{stats.needsDecision} need your decision</span>.
          </p>
        )}
      </header>

      {/* Filter chips */}
      <div className="mb-4 flex flex-wrap items-center gap-1.5" role="tablist" aria-label="Filter cases">
        {FILTERS.map((f) => {
          const count =
            f.key === "all"
              ? groups.reduce((s, g) => s + g.items.length, 0)
              : groups
                  .flatMap((g) => g.items)
                  .filter((i) => matchesFilter(i, f.key)).length;
          if (count === 0 && f.key !== "all") return null;
          return (
            <button
              key={f.key}
              role="tab"
              aria-selected={filter === f.key}
              onClick={() => setFilter(f.key)}
              className={cn(
                "inline-flex h-7 items-center gap-1.5 rounded-md border px-2.5 text-[12.5px] font-medium",
                filter === f.key
                  ? "border-brand/30 bg-brand-soft text-brand"
                  : "border-border bg-surface text-muted-foreground hover:border-foreground/25 hover:text-foreground"
              )}
            >
              {f.label}
              <span className={cn("tabular text-[11px]", filter === f.key ? "text-brand/70" : "text-muted-foreground")}>
                {count}
              </span>
            </button>
          );
        })}
      </div>

      {/* Queue */}
      {visible.length === 0 ? (
        <EmptyState
          title="No cases match this filter"
          description="Try a different filter, or check back later — new exceptions appear here as invoices are ingested."
        />
      ) : (
        <div className="flex flex-col gap-6">
          {visible.map((group) => {
            const isCollapsed = collapsed[group.key] ?? false;
            const isNeedsDecision = group.key === "needs-decision";
            return (
              <section key={group.key} aria-label={group.title}>
                <button
                  type="button"
                  onClick={() => setCollapsed((c) => ({ ...c, [group.key]: !isCollapsed }))}
                  aria-expanded={!isCollapsed}
                  className="mb-2 flex w-full items-center gap-1.5 text-left"
                >
                  <ChevronDown
                    className={cn("h-3.5 w-3.5 text-muted-foreground transition-transform", isCollapsed && "-rotate-90")}
                    aria-hidden="true"
                  />
                  <h2 className="text-[13px] font-semibold text-foreground">{group.title}</h2>
                  <span className="text-[12px] font-medium tabular text-muted-foreground">{group.items.length}</span>
                </button>
                {!isCollapsed && (
                  <div className="overflow-hidden rounded-lg border border-border bg-surface shadow-xs">
                    <ul className="divide-y divide-border">
                      {group.items.map((item, idx) => (
                        <QueueRow
                          key={item.id}
                          item={item}
                          emphasized={isNeedsDecision && filter === "all" && idx === 0}
                          onOpen={() => setSelected(item)}
                        />
                      ))}
                    </ul>
                  </div>
                )}
              </section>
            );
          })}
        </div>
      )}

      {/* Detail drawer */}
      <CaseDrawer item={selected} onClose={() => setSelected(null)} isController={role === "CONTROLLER"} />
    </div>
  );
}

/* ------------------------------------------------------------------ */
/* Queue row                                                           */
/* ------------------------------------------------------------------ */

function QueueRow({
  item,
  emphasized,
  onOpen,
}: {
  item: CaseQueueItem;
  emphasized?: boolean;
  onOpen: () => void;
}) {
  return (
    <li>
      <button
        type="button"
        onClick={onOpen}
        className={cn(
          "group flex w-full items-center gap-4 px-4 py-3 text-left transition-colors hover:bg-surface-subtle focus-visible:outline-none focus-visible:bg-brand-soft",
          emphasized && "bg-brand-soft/60"
        )}
      >
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-x-2.5 gap-y-1">
            {emphasized && (
              <span className="inline-flex items-center gap-1 rounded bg-brand px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wider text-white">
                <CircleAlert className="h-3 w-3" aria-hidden="true" />
                Highest priority
              </span>
            )}
            <span className="font-mono text-[13px] font-semibold text-foreground">{item.invoiceNumber}</span>
            <span className="text-[13px] text-muted-foreground">·</span>
            <span className="text-[13px] font-medium text-foreground">{item.supplierName}</span>
            <span className="text-[13px] text-muted-foreground">·</span>
            <span className="text-[12.5px] text-muted-foreground">{item.headline}</span>
          </div>
          {item.recommendation && (
            <p className="mt-1 truncate text-[12.5px] text-muted-foreground">
              <span className="font-medium text-secondary">Agent:</span> {item.recommendation}
            </p>
          )}
        </div>

        <div className="flex shrink-0 items-center gap-4">
          <span className="num text-[13px] font-semibold text-foreground">
            {formatCurrency(item.amount, item.currency)}
          </span>
          {item.status !== "OPEN" ? (
            <CaseStatusBadge status={item.status} />
          ) : (
            <StatusBadge tone="warning">Needs decision</StatusBadge>
          )}
          <span className="w-16 text-right text-[12px] text-muted-foreground">
            {relativeTime(item.lastUpdatedAt)}
          </span>
          <ChevronRight
            className="h-4 w-4 text-[#c1c7d0] transition-colors group-hover:text-muted-foreground"
            aria-hidden="true"
          />
        </div>
      </button>
    </li>
  );
}

/* ------------------------------------------------------------------ */
/* Case drawer                                                         */
/* ------------------------------------------------------------------ */

function CaseDrawer({
  item,
  onClose,
  isController,
}: {
  item: CaseQueueItem | null;
  onClose: () => void;
  isController: boolean;
}) {
  if (!item) return null;
  const amountText = formatCurrency(item.amount, item.currency);
  const confidenceTone = item.confidence === "HIGH" ? "success" : item.confidence === "MEDIUM" ? "warning" : "danger";

  return (
    <DetailDrawer
      open
      onOpenChange={(o) => !o && onClose()}
      width="lg"
      eyebrow={
        <span className="flex items-center gap-2">
          <CaseStatusBadge status={item.status} />
        </span>
      }
      title={
        <span className="flex items-baseline gap-2.5">
          <span className="font-mono">{item.invoiceNumber}</span>
          <span className="text-[14px] font-medium text-muted-foreground">{item.supplierName}</span>
        </span>
      }
      description={
        <span className="flex items-center gap-2">
          <span className="font-semibold text-foreground">{amountText}</span>
          <span>·</span>
          {item.headline}
          <span>·</span>
          {item.ageDays === 0 ? "Created today" : `${item.ageDays}d old`}
        </span>
      }
      footer={
        <Link
          href={`/work/${item.invoiceNumber}`}
          className="flex h-9 w-full items-center justify-center gap-1.5 rounded-md bg-primary text-[13px] font-medium text-primary-foreground transition-colors hover:bg-brand-hover"
        >
          Open full case workspace
          <ArrowRight className="h-4 w-4" aria-hidden="true" />
        </Link>
      }
    >
      <div className="flex flex-col gap-5">
        {/* Summary */}
        {item.discrepancySummary && (
          <div className="rounded-lg border border-border bg-surface-subtle px-3.5 py-3">
            <p className="text-[12px] font-semibold uppercase tracking-wider text-muted-foreground">Discrepancy</p>
            <p className="mt-1 text-[13px] leading-relaxed text-foreground">{item.discrepancySummary}</p>
          </div>
        )}

        {/* Facts */}
        <section>
          <h3 className="label-caps mb-2">Record</h3>
          <dl className="divide-y divide-border rounded-lg border border-border bg-surface px-3.5">
            <Fact label="Invoice" value={<span className="font-mono">{item.invoiceNumber}</span>} />
            <Fact
              label="Supplier"
              value={
                <Link href={`/suppliers/${item.supplierId}`} className="text-primary hover:underline underline-offset-4">
                  {item.supplierName}
                </Link>
              }
            />
            <Fact label="Amount" value={<span className="num font-semibold">{amountText}</span>} />
            <Fact label="Purchase order" value={item.poNumber ? <span className="font-mono">{item.poNumber}</span> : <span className="text-danger font-medium">Not matched</span>} />
            <Fact
              label="Delivery"
              value={item.shipmentReference ? <span className="font-mono">{item.shipmentReference}</span> : <span className="text-warning font-medium">None on file</span>}
            />
            <Fact label="Evidence" value={`${item.evidenceCount} documents`} />
            {item.relatedApprovalReference && (
              <Fact
                label="Approval"
                value={
                  <Link href={`/approvals/${item.relatedApprovalReference}`} className="font-mono text-primary hover:underline underline-offset-4">
                    {item.relatedApprovalReference}
                  </Link>
                }
              />
            )}
          </dl>
        </section>

        {/* Agent */}
        <section>
          <h3 className="label-caps mb-2">Agent recommendation</h3>
          <div className="rounded-lg border border-border bg-surface px-3.5 py-3">
            <div className="flex items-center justify-between gap-3">
              <p className="text-[13px] font-medium leading-snug text-foreground">{item.recommendation || "—"}</p>
              <StatusBadge tone={confidenceTone}>{item.confidence.toLowerCase()} confidence</StatusBadge>
            </div>
          </div>
        </section>

        {/* Policy */}
        <section>
          <h3 className="label-caps mb-2">Policy</h3>
          <div
            className={cn(
              "rounded-lg border px-3.5 py-2.5 text-[12.5px]",
              item.approveBlockedReason
                ? "border-danger/20 bg-danger-bg text-danger"
                : "border-success/20 bg-success-bg text-success"
            )}
          >
            {item.approveBlockedReason
              ? `Approve payment blocked by policy — ${item.approveBlockedReason}`
              : item.requiresApproval
                ? "Approve payment allowed — requires controller approval (above $5,000)."
                : "Approve payment allowed within policy."}
          </div>
        </section>

        {/* Actions */}
        <section>
          <h3 className="label-caps mb-2">Actions</h3>
          <CaseActionPanel
            caseId={item.id}
            status={item.status}
            supplierName={item.supplierName}
            amountText={amountText}
            approveBlockedReason={item.approveBlockedReason}
            requiresApproval={item.requiresApproval}
            isController={isController}
            onActionSuccess={onClose}
          />
        </section>
      </div>
    </DetailDrawer>
  );
}

function Fact({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex items-baseline justify-between gap-4 py-2">
      <dt className="text-[12.5px] text-muted-foreground">{label}</dt>
      <dd className="text-right text-[13px] text-foreground">{value}</dd>
    </div>
  );
}

/* ------------------------------------------------------------------ */
/* Jean (controller) home                                              */
/* ------------------------------------------------------------------ */

export function JeanApprovalView({
  approvals,
  totalProposed,
  name,
}: {
  approvals: {
    id: string;
    reference: string;
    invoiceNumber: string;
    supplierName: string;
    proposedAction: string;
    amount: number;
    currency: string;
    evidenceCount: number;
    createdAt: Date;
  }[];
  totalProposed: number;
  name: string;
}) {
  const first = name.split(" ")[0];
  return (
    <div>
      <header className="mb-6">
        <h1 className="text-[22px] font-semibold leading-7 tracking-[-0.01em] text-foreground">Good day, {first}.</h1>
        <p className="mt-1.5 text-[13px] text-muted-foreground">
          You have{" "}
          <span className="font-semibold text-foreground">
            {approvals.length} approval{approvals.length === 1 ? "" : "s"} waiting
          </span>
          . <span className="font-semibold text-foreground">{formatCurrency(totalProposed)}</span> in proposed payments.
        </p>
      </header>
      {approvals.length === 0 ? (
        <EmptyState
          title="Approval queue is clear"
          description="High-value payment approvals above the $5,000 threshold will appear here for your decision."
        />
      ) : (
        <div className="overflow-hidden rounded-lg border border-border bg-surface shadow-xs">
          <ul className="divide-y divide-border">
            {approvals.map((a) => (
              <li key={a.id}>
                <Link
                  href={`/approvals/${a.reference}`}
                  className="group flex w-full items-center gap-4 px-4 py-3.5 transition-colors hover:bg-surface-subtle"
                >
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-x-2.5 gap-y-1">
                      <span className="font-mono text-[13px] font-semibold text-foreground">{a.invoiceNumber}</span>
                      <span className="text-[13px] text-muted-foreground">·</span>
                      <span className="text-[13px] font-medium text-foreground">{a.supplierName}</span>
                      <StatusBadge tone="warning">Awaiting your decision</StatusBadge>
                    </div>
                    <p className="mt-1 text-[12.5px] text-muted-foreground">
                      {a.proposedAction} · {a.evidenceCount} evidence sources · requested {relativeTime(a.createdAt)}
                    </p>
                  </div>
                  <span className="num shrink-0 text-[13px] font-semibold text-foreground">
                    {formatCurrency(a.amount, a.currency)}
                  </span>
                  <ChevronRight className="h-4 w-4 shrink-0 text-[#c1c7d0] group-hover:text-muted-foreground" aria-hidden="true" />
                </Link>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
