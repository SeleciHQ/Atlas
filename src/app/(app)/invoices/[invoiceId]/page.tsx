import { notFound } from "next/navigation";
import Link from "next/link";
import { getInvoiceDetail } from "@/lib/queries";
import { db } from "@/lib/db";
import { formatCurrency, formatDate, relativeTime } from "@/lib/format";
import { PageHeader } from "@/components/app-shell/app-shell";
import { InvoiceStatusBadge, CaseStatusBadge, ExceptionBadge, ApprovalStatusBadge } from "@/components/shared/badges";
import { StatusBadge } from "@/components/shared/status-badge";
import { SummaryBlock, CardSection } from "@/components/case/summary-blocks";
import { LineComparisonTable } from "@/components/case/summary-blocks";
import { CaseTimelineCard } from "@/components/case/case-timeline";
import { EvidenceGrid, type EvidenceDoc } from "@/components/evidence/evidence-grid";
import { buildLineComparison, type AgentCaseInput } from "@/lib/agent";
import { EXCEPTION_LABELS, type ExceptionType } from "@/types/ontology";

export const dynamic = "force-dynamic";

export default async function InvoiceDetailPage(props: PageProps<"/invoices/[invoiceId]">) {
  const { invoiceId: number } = await props.params;
  const data = await getInvoiceDetail(number);
  if (!data) notFound();
  const { invoice, shipment } = data;

  const receivedTotal = (shipment?.deliveryEvents ?? []).flatMap((e) => e.lines).reduce((s, l) => s + l.quantity, 0);

  const input: AgentCaseInput = {
    invoiceNumber: invoice.number,
    exceptionType: (invoice.case?.exceptionType as ExceptionType) ?? "QUANTITY_MISMATCH",
    caseStatus: invoice.case?.status ?? "OPEN",
    amount: invoice.amount,
    currency: invoice.currency,
    poNumber: invoice.po?.number ?? null,
    poLines: (invoice.po?.lines ?? []).map((l) => ({ item: l.item, quantity: l.quantity, unitPrice: l.unitPrice })),
    invoiceLines: invoice.lines.map((l) => ({
      item: l.item,
      invoicedQuantity: l.invoicedQuantity,
      unitPrice: l.unitPrice,
    })),
    deliveries: (shipment?.deliveryEvents ?? []).map((d) => ({
      reference: d.reference,
      type: d.type,
      lines: d.lines.map((l) => ({ item: l.item, quantity: l.quantity })),
    })),
    discrepancyTypes: [],
    supplierPriorIssues: 0,
    taxDocMissing: invoice.taxDocMissing,
    duplicateOf: invoice.duplicateOf,
    evidenceCount: 0,
  };
  const comparison = buildLineComparison(input);

  const documents: EvidenceDoc[] = invoice.documents.map((d) => ({
    id: d.id,
    kind: d.kind,
    name: d.name,
    createdAt: d.createdAt.toISOString(),
    extractedFields: d.extractedFields
      ? (() => {
          try {
            return JSON.parse(d.extractedFields) as Record<string, unknown>;
          } catch {
            return null;
          }
        })()
      : null,
    poNumber: d.kind === "PURCHASE_ORDER" ? invoice.po?.number ?? null : null,
  }));

  return (
    <article>
      <PageHeader
        breadcrumbs={[
          { label: "Invoices", href: "/invoices" },
          { label: invoice.number, href: `/invoices/${invoice.number}` },
        ]}
        title={
          <span className="flex flex-wrap items-center gap-x-3 gap-y-2">
            <span className="font-mono">{invoice.number}</span>
            <InvoiceStatusBadge status={invoice.status} />
            {invoice.case && <CaseStatusBadge status={invoice.case.status} />}
          </span>
        }
        description={`${invoice.supplier.name} · ${invoice.po ? `PO ${invoice.po.number}` : "No purchase order"} · Issued ${formatDate(invoice.invoiceDate)}`}
        actions={
          invoice.case ? (
            <Link
              href={`/work/${invoice.number}`}
              className="inline-flex h-9 items-center gap-1.5 rounded-md bg-primary px-3.5 text-[13px] font-medium text-primary-foreground shadow-sm transition-colors hover:bg-brand-hover"
            >
              Open case
            </Link>
          ) : undefined
        }
      />

      <div className="grid items-start gap-6 xl:grid-cols-[minmax(0,1fr)_380px]">
        <div className="flex min-w-0 flex-col gap-5">
          {/* Overview */}
          <section aria-label="Overview" className="flex flex-col gap-4">
            <h2 className="text-[15px] font-semibold text-foreground">Overview</h2>
            <div className="grid gap-4 sm:grid-cols-2">
              <SummaryBlock
                title="Invoice"
                rows={[
                  { label: "Invoice number", value: <span className="font-mono">{invoice.number}</span> },
                  { label: "Supplier", value: invoice.supplier.name },
                  { label: "Amount", value: <span className="num font-semibold">{formatCurrency(invoice.amount, invoice.currency)}</span> },
                  { label: "Currency", value: invoice.currency },
                ]}
              />
              <SummaryBlock
                title="Dates & status"
                rows={[
                  { label: "Issue date", value: formatDate(invoice.invoiceDate) },
                  { label: "Due date", value: invoice.dueDate ? formatDate(invoice.dueDate) : "—" },
                  { label: "Payment status", value: <InvoiceStatusBadge status={invoice.status} /> },
                  {
                    label: "ERP status",
                    value: (
                      <StatusBadge tone={invoice.erpStatus === "PAYMENT_RELEASED" ? "success" : invoice.erpStatus === "BLOCKED" ? "warning" : "neutral"}>
                        {(invoice.erpStatus ?? "NOT_POSTED").toLowerCase().replace(/_/g, " ")}
                      </StatusBadge>
                    ),
                  },
                ]}
              />
            </div>
          </section>

          {/* Reconciliation */}
          <section aria-label="Reconciliation" className="flex flex-col gap-4">
            <h2 className="text-[15px] font-semibold text-foreground">Reconciliation</h2>
            {invoice.case && (
              <div className="flex items-center gap-2.5 rounded-lg border border-warning/20 bg-warning-bg px-4 py-2.5 text-[13px] text-warning">
                <span className="font-medium">{EXCEPTION_LABELS[invoice.case.exceptionType as ExceptionType]}</span>
                <span className="text-warning/70">—</span>
                <Link href={`/work/${invoice.number}`} className="font-medium underline underline-offset-4">
                  review in case workspace
                </Link>
              </div>
            )}
            <CardSection title="Line-level comparison">
              <div className="overflow-x-auto">
                <LineComparisonTable comparison={comparison} />
              </div>
            </CardSection>
            <div className="grid gap-4 sm:grid-cols-2">
              <SummaryBlock
                title="Purchase order"
                rows={
                  invoice.po
                    ? [
                        { label: "PO number", value: <span className="font-mono">{invoice.po.number}</span> },
                        { label: "PO total", value: <span className="num">{formatCurrency(invoice.po.totalAmount, invoice.po.currency)}</span> },
                        ...invoice.po.lines.map((l) => ({
                          label: `${l.item} (${l.sku})`,
                          value: <span className="num">{`${l.quantity.toLocaleString()} × $${l.unitPrice.toFixed(2)}`}</span>,
                        })),
                      ]
                    : [{ label: "PO number", value: <span className="font-medium text-danger">Not matched</span> }]
                }
              />
              <SummaryBlock
                title="Delivery"
                rows={
                  shipment && shipment.deliveryEvents.length > 0
                    ? [
                        { label: "Shipment", value: <span className="font-mono">{shipment.reference}</span> },
                        { label: "Carrier", value: shipment.carrier },
                        ...shipment.deliveryEvents.flatMap((e) => [
                          { label: `Event ${e.reference}`, value: e.type.toLowerCase().replace(/_/g, " ") },
                          ...e.lines.map((l) => ({ label: l.item, value: <span className="num">{`${l.quantity.toLocaleString()} received`}</span> })),
                        ]),
                      ]
                    : [{ label: "Delivery record", value: <span className="font-medium text-warning">None found</span> }]
                }
                footer={
                  shipment && shipment.deliveryEvents.length > 0
                    ? `${receivedTotal.toLocaleString()} units received in total.`
                    : "No goods received note on file."
                }
              />
            </div>
          </section>

          {/* Documents */}
          <section aria-label="Documents" className="flex flex-col gap-4">
            <h2 className="text-[15px] font-semibold text-foreground">Documents</h2>
            {documents.length > 0 ? (
              <EvidenceGrid documents={documents} />
            ) : (
              <p className="rounded-lg border border-dashed px-4 py-6 text-center text-[12.5px] text-muted-foreground">
                No documents attached to this invoice.
              </p>
            )}
          </section>

          {/* Activity */}
          <section aria-label="Activity">
            <h2 className="mb-4 text-[15px] font-semibold text-foreground">Activity</h2>
            <CaseTimelineCard events={await invoiceTimeline(invoice.id)} />
          </section>
        </div>

        <div className="flex flex-col gap-5">
          {/* Approvals */}
          <section aria-label="Approvals" className="flex flex-col gap-3">
            <h2 className="text-[15px] font-semibold text-foreground">Approvals</h2>
            <ApprovalsList invoiceId={invoice.id} />
          </section>

          {/* Metadata */}
          <SummaryBlock
            title="Record"
            rows={[
              { label: "OCR status", value: invoice.ocrStatus.toLowerCase() },
              { label: "Tax document", value: invoice.taxDocMissing ? <span className="font-medium text-warning">Missing</span> : "On file" },
              ...(invoice.duplicateOf ? [{ label: "Duplicate of", value: <span className="font-mono">{invoice.duplicateOf}</span> }] : []),
              ...(invoice.case
                ? [{ label: "Exception", value: <ExceptionBadge type={invoice.case.exceptionType} /> }]
                : []),
              {
                label: "Last updated",
                value: relativeTime(invoice.case?.updatedAt ?? invoice.invoiceDate),
              },
            ]}
          />
        </div>
      </div>
    </article>
  );
}

async function invoiceTimeline(invoiceId: string) {
  const kase = await db.case.findUnique({ where: { invoiceId } });
  if (!kase) return [];
  return db.auditEvent.findMany({ where: { caseId: kase.id }, orderBy: { createdAt: "asc" } });
}

async function ApprovalsList({ invoiceId }: { invoiceId: string }) {
  const kase = await db.case.findUnique({ where: { invoiceId }, include: { approvals: true } });
  if (!kase || kase.approvals.length === 0) {
    return (
      <p className="rounded-lg border border-dashed px-4 py-5 text-center text-[12.5px] text-muted-foreground">
        No approval requests for this invoice.
      </p>
    );
  }
  return (
    <ul className="divide-y divide-border rounded-lg border border-border bg-surface shadow-xs">
      {kase.approvals.map((a) => (
        <li key={a.id}>
          <Link
            href={`/approvals/${a.reference}`}
            className="flex items-center justify-between gap-3 px-4 py-3 transition-colors hover:bg-surface-subtle"
          >
            <div className="min-w-0">
              <p className="font-mono text-[12.5px] font-medium text-foreground">{a.reference}</p>
              <p className="text-[12px] text-muted-foreground">
                {a.action.replace(/_/g, " ")} · {a.requestedBy}
                {a.decidedAt ? ` · decided ${formatDate(a.decidedAt)}` : ""}
              </p>
            </div>
            <ApprovalStatusBadge status={a.status} />
          </Link>
        </li>
      ))}
    </ul>
  );
}
