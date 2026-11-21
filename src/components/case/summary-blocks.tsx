import { StatusBadge } from "@/components/shared/status-badge";
import type { LineComparison } from "@/types/workflow";
import { cn } from "@/lib/utils";

export function LineComparisonTable({ comparison }: { comparison: LineComparison[] }) {
  if (comparison.length === 0) {
    return (
      <p className="rounded-md border border-dashed px-4 py-6 text-center text-[12.5px] text-muted-foreground">
        No comparable line items found between the invoice, purchase order, and delivery records.
      </p>
    );
  }
  return (
    <table className="w-full text-[12.5px]">
      <thead>
        <tr className="border-b text-left text-[11px] font-medium uppercase tracking-[.08em] text-muted-foreground">
          <th className="px-4 py-2 font-medium">Item</th>
          <th className="px-4 py-2 text-right font-medium">Ordered</th>
          <th className="px-4 py-2 text-right font-medium">Invoiced</th>
          <th className="px-4 py-2 text-right font-medium">Received</th>
          <th className="px-4 py-2 text-right font-medium">Difference</th>
          <th className="px-4 py-2 pr-4 text-right font-medium">Status</th>
        </tr>
      </thead>
      <tbody className="divide-y divide-border">
        {comparison.map((row) => (
          <tr key={row.item} className="hover:bg-surface-subtle">
            <td className="px-4 py-2 font-medium text-foreground">{row.item}</td>
            <td className="num px-4 py-2 text-right text-muted-foreground">{row.ordered.toLocaleString()}</td>
            <td className="num px-4 py-2 text-right">{row.invoiced.toLocaleString()}</td>
            <td className="num px-4 py-2 text-right">{row.received.toLocaleString()}</td>
            <td
              className={cn(
                "num px-4 py-2 text-right font-medium",
                row.difference !== 0 ? "text-danger" : "text-muted-foreground"
              )}
            >
              {row.difference > 0 ? "+" : ""}
              {row.difference.toLocaleString()}
            </td>
            <td className="px-4 py-2 pr-4 text-right">
              {row.passed ? (
                <StatusBadge tone="success">Passed</StatusBadge>
              ) : (
                <StatusBadge tone="danger">Failed</StatusBadge>
              )}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

export function SummaryBlock({
  title,
  rows,
  footer,
  tone,
}: {
  title: string;
  rows: { label: string; value: React.ReactNode }[];
  footer?: React.ReactNode;
  tone?: "default" | "warning";
}) {
  return (
    <div className="rounded-lg border border-border bg-surface shadow-xs">
      <div
        className={cn(
          "flex h-9 items-center justify-between border-b px-4",
          tone === "warning" && "bg-warning-bg"
        )}
      >
        <h3 className="text-[12px] font-semibold text-foreground">{title}</h3>
        {tone === "warning" && <StatusBadge tone="warning">Attention</StatusBadge>}
      </div>
      <dl className="divide-y divide-border px-4">
        {rows.map((row) => (
          <div key={row.label} className="flex items-baseline justify-between gap-4 py-2">
            <dt className="text-[12.5px] text-muted-foreground">{row.label}</dt>
            <dd className="text-right text-[13px] font-medium text-foreground">{row.value}</dd>
          </div>
        ))}
      </dl>
      {footer && <div className="border-t bg-surface-subtle px-4 py-2.5 text-[12.5px] leading-relaxed text-muted-foreground">{footer}</div>}
    </div>
  );
}

export function CardSection({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="rounded-lg border border-border bg-surface shadow-xs">
      <div className="flex h-9 items-center border-b px-4">
        <h3 className="text-[12px] font-semibold text-foreground">{title}</h3>
      </div>
      {children}
    </div>
  );
}
