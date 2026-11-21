"use client";

import * as React from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { Search, ArrowUpDown, ChevronDown } from "lucide-react";
import { Input } from "@/components/ui/input";
import { StatusBadge } from "@/components/shared/status-badge";
import { InvoiceStatusBadge } from "@/components/shared/badges";
import { EmptyState } from "@/components/shared/empty-state";
import { formatCurrency, formatDate, relativeTime } from "@/lib/format";
import { EXCEPTION_LABELS, type ExceptionType } from "@/types/ontology";
import { cn } from "@/lib/utils";

export interface InvoiceRow {
  number: string;
  supplierName: string;
  supplierCode: string;
  amount: number;
  currency: string;
  invoiceDate: string;
  dueDate: string | null;
  poNumber: string | null;
  status: string;
  exceptionType: string | null;
  caseStatus: string | null;
  updatedAt: string;
}

const FILTERS = [
  { key: "all", label: "All invoices" },
  { key: "matched", label: "Matched" },
  { key: "exceptions", label: "Exceptions" },
  { key: "awaiting-supplier", label: "Awaiting supplier" },
  { key: "awaiting-approval", label: "Awaiting approval" },
  { key: "paid", label: "Paid" },
  { key: "rejected", label: "Rejected" },
] as const;

type SortKey = "invoiceDate" | "amount" | "supplier" | "updatedAt";
type SortDir = "asc" | "desc";

function matchesFilter(row: InvoiceRow, filter: string): boolean {
  switch (filter) {
    case "all":
      return true;
    case "matched":
      return !row.exceptionType && row.status !== "RECEIVED";
    case "exceptions":
      return Boolean(row.exceptionType);
    case "awaiting-supplier":
      return row.caseStatus === "WAITING_SUPPLIER";
    case "awaiting-approval":
      return row.caseStatus === "READY_FOR_APPROVAL" || row.status === "HELD";
    case "paid":
      return row.status === "PAID";
    case "rejected":
      return row.status === "REJECTED";
    default:
      return true;
  }
}

export function InvoicesTable({ rows }: { rows: InvoiceRow[] }) {
  const router = useRouter();
  const searchParams = useSearchParams();
  const [query, setQuery] = React.useState("");
  const [filter, setFilter] = React.useState<string>(() => {
    const f = searchParams.get("filter");
    return f && FILTERS.some((x) => x.key === f) ? f : "all";
  });
  const [sort, setSort] = React.useState<{ key: SortKey; dir: SortDir }>({ key: "invoiceDate", dir: "desc" });

  const counts = React.useMemo(() => {
    const c: Record<string, number> = {};
    for (const f of FILTERS) c[f.key] = rows.filter((r) => matchesFilter(r, f.key)).length;
    return c;
  }, [rows]);

  const visible = React.useMemo(() => {
    const q = query.trim().toLowerCase();
    let list = rows.filter((r) => matchesFilter(r, filter));
    if (q) {
      list = list.filter((r) =>
        `${r.number} ${r.supplierName} ${r.poNumber ?? ""}`.toLowerCase().includes(q)
      );
    }
    const dir = sort.dir === "asc" ? 1 : -1;
    return [...list].sort((a, b) => {
      switch (sort.key) {
        case "amount":
          return (a.amount - b.amount) * dir;
        case "supplier":
          return a.supplierName.localeCompare(b.supplierName) * dir;
        case "updatedAt":
          return (new Date(a.updatedAt).getTime() - new Date(b.updatedAt).getTime()) * dir;
        default:
          return (new Date(a.invoiceDate).getTime() - new Date(b.invoiceDate).getTime()) * dir;
      }
    });
  }, [rows, filter, query, sort]);

  function toggleSort(key: SortKey) {
    setSort((s) => (s.key === key ? { key, dir: s.dir === "asc" ? "desc" : "asc" } : { key, dir: key === "supplier" ? "asc" : "desc" }));
  }

  const exceptionCount = counts["exceptions"] ?? 0;

  return (
    <div>
      {/* Summary strip */}
      <div className="mb-4 flex flex-wrap items-center gap-x-5 gap-y-1 text-[12.5px] text-muted-foreground">
        <span>
          <span className="num font-semibold text-foreground">{rows.length}</span> invoices
        </span>
        <span>
          <span className="num font-semibold text-foreground">{rows.length - exceptionCount}</span> matched
        </span>
        <span>
          <span className="num font-semibold text-foreground">{exceptionCount}</span> exceptions
        </span>
        <span>
          <span className="num font-semibold text-foreground">{counts["paid"]}</span> paid
        </span>
      </div>

      {/* Filters + search */}
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <div className="flex flex-wrap items-center gap-1.5">
          {FILTERS.map((f) => (
            <button
              key={f.key}
              role="tab"
              aria-selected={filter === f.key}
              onClick={() => {
                setFilter(f.key);
                const params = new URLSearchParams(searchParams.toString());
                if (f.key === "all") params.delete("filter");
                else params.set("filter", f.key);
                router.replace(`/invoices${params.size ? `?${params}` : ""}`, { scroll: false });
              }}
              className={cn(
                "inline-flex h-7 items-center gap-1.5 rounded-md border px-2.5 text-[12.5px] font-medium",
                filter === f.key
                  ? "border-brand/30 bg-brand-soft text-brand"
                  : "border-border bg-surface text-muted-foreground hover:border-foreground/25 hover:text-foreground"
              )}
            >
              {f.label}
              <span className={cn("num text-[11px]", filter === f.key ? "text-brand/70" : "text-muted-foreground")}>
                {counts[f.key]}
              </span>
            </button>
          ))}
        </div>
        <div className="relative ml-auto w-64">
          <Search className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" aria-hidden="true" />
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search invoices, suppliers, POs…"
            aria-label="Search invoices"
            className="h-8 pl-8 text-[12.5px]"
          />
        </div>
      </div>

      {/* Table */}
      {visible.length === 0 ? (
        <EmptyState
          title="No invoices found"
          description={query ? `No invoices match “${query}”.` : "No invoices match this filter."}
        />
      ) : (
        <div className="overflow-hidden rounded-lg border border-border bg-surface shadow-xs">
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="border-b border-border bg-surface-subtle/60 text-left">
                  <Th sortable label="Invoice" active={sort.key === "invoiceDate"} dir={sort.dir} onClick={() => toggleSort("invoiceDate")} className="w-32" />
                  <Th label="Supplier" />
                  <Th
                    sortable
                    label="Amount"
                    align="right"
                    active={sort.key === "amount"}
                    dir={sort.dir}
                    onClick={() => toggleSort("amount")}
                  />
                  <Th label="Invoice date" />
                  <Th label="Due date" />
                  <Th label="PO" />
                  <Th label="Reconciliation" />
                  <Th label="Status" />
                  <Th
                    sortable
                    label="Updated"
                    active={sort.key === "updatedAt"}
                    dir={sort.dir}
                    onClick={() => toggleSort("updatedAt")}
                  />
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {visible.map((row) => (
                  <tr
                    key={row.number}
                    className="cursor-pointer transition-colors hover:bg-surface-subtle focus-visible:bg-brand-soft focus-visible:outline-none"
                    tabIndex={0}
                    onClick={() => router.push(`/invoices/${row.number}`)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter") router.push(`/invoices/${row.number}`);
                    }}
                    aria-label={`Open invoice ${row.number}`}
                  >
                    <td className="px-4 py-2.5">
                      <span className="font-mono text-[13px] font-semibold text-foreground">{row.number}</span>
                    </td>
                    <td className="px-4 py-2.5 text-[13px] text-foreground">{row.supplierName}</td>
                    <td className="num px-4 py-2.5 text-right text-[13px] font-semibold text-foreground">
                      {formatCurrency(row.amount, row.currency)}
                    </td>
                    <td className="px-4 py-2.5 text-[12.5px] text-muted-foreground">{formatDate(row.invoiceDate)}</td>
                    <td className="px-4 py-2.5 text-[12.5px] text-muted-foreground">{row.dueDate ? formatDate(row.dueDate) : "—"}</td>
                    <td className="px-4 py-2.5 font-mono text-[12px] text-muted-foreground">{row.poNumber ?? "—"}</td>
                    <td className="px-4 py-2.5">
                      {row.exceptionType ? (
                        <StatusBadge tone="warning">{EXCEPTION_LABELS[row.exceptionType as ExceptionType] ?? row.exceptionType}</StatusBadge>
                      ) : (
                        <StatusBadge tone="success">Matched</StatusBadge>
                      )}
                    </td>
                    <td className="px-4 py-2.5">
                      <InvoiceStatusBadge status={row.status} />
                    </td>
                    <td className="px-4 py-2.5 text-right text-[12px] text-muted-foreground">{relativeTime(row.updatedAt)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
}

function Th({
  label,
  sortable,
  active,
  dir,
  onClick,
  align,
  className,
}: {
  label: string;
  sortable?: boolean;
  active?: boolean;
  dir?: SortDir;
  onClick?: () => void;
  align?: "right";
  className?: string;
}) {
  return (
    <th
      className={cn(
        "h-9 px-4 text-[11px] font-medium uppercase tracking-[.08em] text-muted-foreground",
        align === "right" && "text-right",
        className
      )}
      aria-sort={active ? (dir === "asc" ? "ascending" : "descending") : undefined}
    >
      {sortable ? (
        <button
          type="button"
          onClick={onClick}
          className={cn(
            "inline-flex items-center gap-1 hover:text-foreground",
            active && "text-foreground"
          )}
        >
          {label}
          {active ? (
            <ChevronDown className={cn("h-3 w-3", dir === "asc" && "rotate-180")} aria-hidden="true" />
          ) : (
            <ArrowUpDown className="h-3 w-3 opacity-40" aria-hidden="true" />
          )}
        </button>
      ) : (
        label
      )}
    </th>
  );
}
