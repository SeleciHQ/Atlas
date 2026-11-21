"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { Search } from "lucide-react";
import { Input } from "@/components/ui/input";
import { SupplierStatusBadge } from "@/components/shared/badges";
import { StatusBadge } from "@/components/shared/status-badge";
import { EmptyState } from "@/components/shared/empty-state";
import { formatCurrency, relativeTime } from "@/lib/format";
import { cn } from "@/lib/utils";

export interface SupplierRow {
  code: string;
  name: string;
  country: string;
  contactEmail: string;
  status: string;
  invoiceCount: number;
  openCases: number;
  totalSpend: number;
  lastActivity: string;
}

function supplierSort(a: SupplierRow, b: SupplierRow) {
  return a.name.localeCompare(b.name);
}

export function SuppliersTable({ rows }: { rows: SupplierRow[] }) {
  const [query, setQuery] = React.useState("");
  const router = useRouter();

  const visible = React.useMemo(() => {
    const q = query.trim().toLowerCase();
    let list = [...rows].sort(supplierSort);
    if (q) list = list.filter((r) => `${r.name} ${r.code} ${r.country} ${r.contactEmail}`.toLowerCase().includes(q));
    return list;
  }, [rows, query]);

  return (
    <div>
      <div className="mb-3 flex items-center gap-2">
        <div className="relative w-72">
          <Search className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" aria-hidden="true" />
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search suppliers…"
            aria-label="Search suppliers"
            className="h-8 pl-8 text-[12.5px]"
          />
        </div>
        <span className="ml-auto text-[12.5px] text-muted-foreground">
          <span className="num font-semibold text-foreground">{rows.length}</span> suppliers
        </span>
      </div>

      {visible.length === 0 ? (
        <EmptyState title="No suppliers found" description={query ? `No suppliers match “${query}”.` : undefined} />
      ) : (
        <div className="overflow-hidden rounded-lg border border-border bg-surface shadow-xs">
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="border-b border-border bg-surface-subtle/60 text-left">
                  <th className="h-9 px-4 text-[11px] font-medium uppercase tracking-[.08em] text-muted-foreground">Supplier</th>
                  <th className="h-9 px-4 text-[11px] font-medium uppercase tracking-[.08em] text-muted-foreground">Contact</th>
                  <th className="h-9 px-4 text-right text-[11px] font-medium uppercase tracking-[.08em] text-muted-foreground">Invoices</th>
                  <th className="h-9 px-4 text-right text-[11px] font-medium uppercase tracking-[.08em] text-muted-foreground">Open exceptions</th>
                  <th className="h-9 px-4 text-right text-[11px] font-medium uppercase tracking-[.08em] text-muted-foreground">Total spend</th>
                  <th className="h-9 px-4 text-[11px] font-medium uppercase tracking-[.08em] text-muted-foreground">Status</th>
                  <th className="h-9 px-4 text-right text-[11px] font-medium uppercase tracking-[.08em] text-muted-foreground">Last activity</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {visible.map((row) => (
                  <tr
                    key={row.code}
                    tabIndex={0}
                    onClick={() => router.push(`/suppliers/${row.code}`)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter") router.push(`/suppliers/${row.code}`);
                    }}
                    aria-label={`Open supplier ${row.name}`}
                    className="cursor-pointer transition-colors hover:bg-surface-subtle focus-visible:bg-brand-soft focus-visible:outline-none"
                  >
                    <td className="px-4 py-2.5">
                      <span className="text-[13px] font-medium text-foreground">{row.name}</span>
                      <span className="ml-2 font-mono text-[11.5px] text-muted-foreground">{row.code}</span>
                    </td>
                    <td className="px-4 py-2.5 text-[12.5px] text-muted-foreground">{row.contactEmail}</td>
                    <td className="num px-4 py-2.5 text-right text-[13px] text-foreground">{row.invoiceCount}</td>
                    <td className="px-4 py-2.5 text-right">
                      {row.openCases > 0 ? (
                        <StatusBadge tone="warning">{row.openCases}</StatusBadge>
                      ) : (
                        <span className="num text-[13px] text-muted-foreground">0</span>
                      )}
                    </td>
                    <td className="num px-4 py-2.5 text-right text-[13px] font-medium text-foreground">
                      {formatCurrency(row.totalSpend)}
                    </td>
                    <td className="px-4 py-2.5">
                      <SupplierStatusBadge status={row.status} />
                    </td>
                    <td className="px-4 py-2.5 text-right text-[12px] text-muted-foreground">{relativeTime(row.lastActivity)}</td>
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

export function SupplierEmptyHint() {
  return (
    <p className={cn("mt-6 text-[13px] text-muted-foreground")}>
      No paid invoices on record for this supplier yet.
    </p>
  );
}
