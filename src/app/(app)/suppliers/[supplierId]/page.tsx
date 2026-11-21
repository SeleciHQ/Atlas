import { notFound } from "next/navigation";
import Link from "next/link";
import { getSupplierDetail } from "@/lib/queries";
import { formatCurrency, formatDate, relativeTime } from "@/lib/format";
import { PageHeader } from "@/components/app-shell/app-shell";
import { SupplierStatusBadge, InvoiceStatusBadge, CaseStatusBadge } from "@/components/shared/badges";
import { StatusBadge } from "@/components/shared/status-badge";
import { SummaryBlock } from "@/components/case/summary-blocks";
import { ExceptionBadge } from "@/components/shared/badges";

export const dynamic = "force-dynamic";

export default async function SupplierDetailPage(props: PageProps<"/suppliers/[supplierId]">) {
  const { supplierId: code } = await props.params;
  const data = await getSupplierDetail(code);
  if (!data) notFound();
  const { supplier, totalSpend, openDiscrepancies } = data;

  const recentIssues = supplier.cases;
  const paidInvoices = supplier.invoices.filter((i) => i.status === "PAID");
  const outstanding = supplier.invoices
    .filter((i) => !["PAID", "REJECTED"].includes(i.status))
    .reduce((s, i) => s + i.amount, 0);

  return (
    <article>
      <PageHeader
        breadcrumbs={[
          { label: "Suppliers", href: "/suppliers" },
          { label: supplier.code, href: `/suppliers/${supplier.code}` },
        ]}
        title={
          <span className="flex flex-wrap items-center gap-x-3 gap-y-2">
            {supplier.name}
            <span className="font-mono text-[14px] font-medium text-muted-foreground">{supplier.code}</span>
            <SupplierStatusBadge status={supplier.status} />
          </span>
        }
        description={`${supplier.country} · ${supplier.contactEmail}`}
      />

      {/* Stats */}
      <div className="mb-6 grid grid-cols-2 gap-2.5 md:grid-cols-4">
        {[
          { label: "Total spend (paid)", value: formatCurrency(totalSpend) },
          { label: "Outstanding", value: formatCurrency(outstanding) },
          { label: "Invoices", value: String(supplier.invoices.length) },
          { label: "Open exceptions", value: String(openDiscrepancies) },
        ].map((stat) => (
          <div key={stat.label} className="rounded-lg border border-border bg-surface px-4 py-3.5 shadow-xs">
            <p className="text-[11px] font-medium uppercase tracking-[.08em] text-muted-foreground">{stat.label}</p>
            <p className="num mt-1 text-lg font-semibold tabular-nums text-foreground">{stat.value}</p>
          </div>
        ))}
      </div>

      <div className="grid items-start gap-5 xl:grid-cols-2">
        {/* Overview */}
        <section aria-label="Overview" className="rounded-lg border border-border bg-surface px-5 py-4 shadow-xs">
          <h2 className="mb-3 text-[15px] font-semibold text-foreground">Overview</h2>
          <SummaryBlock
            title="Profile"
            rows={[
              { label: "Supplier ID", value: <span className="font-mono">{supplier.code}</span> },
              { label: "Country", value: supplier.country },
              { label: "Contact", value: supplier.contactEmail },
              { label: "Status", value: <SupplierStatusBadge status={supplier.status} /> },
              {
                label: "Data sync",
                value: <StatusBadge tone="success">Connected via mock ERP</StatusBadge>,
              },
            ]}
            footer={supplier.notes ?? undefined}
          />
        </section>

        {/* Recent issues */}
        <section className="rounded-lg border border-border bg-surface px-5 py-4 shadow-xs" aria-label="Recent issues">
          <h2 className="mb-3 text-[15px] font-semibold text-foreground">Recent issues</h2>
          {recentIssues.length === 0 ? (
            <p className="py-4 text-[13px] text-muted-foreground">No discrepancies on record.</p>
          ) : (
            <ul className="divide-y divide-border">
              {recentIssues.map((c) => (
                <li key={c.id} className="flex items-center justify-between gap-3 py-2.5">
                  <div className="flex min-w-0 items-baseline gap-3">
                    <Link href={`/work/${c.invoice.number}`} className="font-mono text-[13px] font-medium text-primary hover:underline underline-offset-4">
                      {c.invoice.number}
                    </Link>
                    <ExceptionBadge type={c.exceptionType} />
                  </div>
                  <div className="flex shrink-0 items-center gap-3">
                    <span className="text-[12px] text-muted-foreground">{relativeTime(c.createdAt)}</span>
                    <CaseStatusBadge status={c.status} />
                  </div>
                </li>
              ))}
            </ul>
          )}
        </section>

        {/* Invoices */}
        <section className="rounded-lg border border-border bg-surface shadow-xs" aria-label="Invoices">
          <div className="flex h-9 items-center border-b px-4">
            <h2 className="text-[12px] font-semibold text-foreground">Invoices</h2>
            <span className="ml-auto">
              <StatusBadge tone="neutral" dot={false}>
                {supplier.invoices.length}
              </StatusBadge>
            </span>
          </div>
          <ul className="divide-y divide-border">
            {supplier.invoices.map((i) => (
              <li key={i.id}>
                <Link href={`/invoices/${i.number}`} className="flex items-center justify-between gap-3 px-4 py-2.5 transition-colors hover:bg-surface-subtle">
                  <div className="flex min-w-0 items-center gap-3">
                    <span className="font-mono text-[13px] font-medium text-foreground">{i.number}</span>
                    {i.po && <span className="font-mono text-[11.5px] text-muted-foreground">{i.po.number}</span>}
                  </div>
                  <div className="flex shrink-0 items-center gap-3">
                    <span className="num text-[13px] text-foreground">{formatCurrency(i.amount, i.currency)}</span>
                    <InvoiceStatusBadge status={i.status} />
                  </div>
                </Link>
              </li>
            ))}
          </ul>
        </section>

        {/* Purchase orders + shipments */}
        <section className="rounded-lg border border-border bg-surface shadow-xs" aria-label="Purchase orders and shipments">
          <div className="flex h-9 items-center border-b px-4">
            <h2 className="text-[12px] font-semibold text-foreground">Purchase orders & shipments</h2>
          </div>
          <ul className="divide-y divide-border">
            {supplier.purchaseOrders.map((po) => {
              const linkedShipment = supplier.shipments.find((s) => s.poId === po.id);
              return (
                <li key={po.id} className="flex items-center justify-between gap-3 px-4 py-2.5">
                  <div className="flex min-w-0 items-baseline gap-3">
                    <span className="font-mono text-[13px] font-medium text-foreground">{po.number}</span>
                    <span className="text-[12px] text-muted-foreground">{po.status.toLowerCase()} · issued {formatDate(po.issuedAt)}</span>
                  </div>
                  <div className="flex shrink-0 items-center gap-3">
                    {linkedShipment && (
                      <span className="font-mono text-[11.5px] text-muted-foreground">
                        {linkedShipment.reference} · {linkedShipment.status.toLowerCase().replace(/_/g, " ")}
                      </span>
                    )}
                    <span className="num text-[13px] text-foreground">{formatCurrency(po.totalAmount, po.currency)}</span>
                  </div>
                </li>
              );
            })}
          </ul>
        </section>
      </div>

      {paidInvoices.length === 0 && (
        <p className="mt-6 text-[13px] text-muted-foreground">No paid invoices on record for this supplier yet.</p>
      )}
    </article>
  );
}
