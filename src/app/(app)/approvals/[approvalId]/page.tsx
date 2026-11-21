import { notFound } from "next/navigation";
import Link from "next/link";
import { getApprovalDetail } from "@/lib/queries";
import { formatCurrency, formatDate } from "@/lib/format";
import { StatusBadge } from "@/components/shared/status-badge";
import { PageHeader } from "@/components/app-shell/app-shell";
import { SummaryBlock } from "@/components/case/summary-blocks";
import { InvoiceStatusBadge } from "@/components/shared/badges";
import { ApprovalDecisionPanel } from "@/components/approvals/decision-panel";
import { EvidenceGrid, type EvidenceDoc } from "@/components/evidence/evidence-grid";

export const dynamic = "force-dynamic";

export default async function ApprovalDetailPage(props: PageProps<"/approvals/[approvalId]">) {
  const { approvalId: reference } = await props.params;
  const data = await getApprovalDetail(reference);
  if (!data || !data.approval) notFound();
  const { approval, user } = data;
  const kase = approval.case;
  const invoice = kase.invoice;
  const amountText = formatCurrency(approval.amount, approval.currency);

  const evidence: EvidenceDoc[] = kase.documents.map((d) => ({
    id: d.id,
    kind: d.kind,
    name: d.name,
    createdAt: d.createdAt.toISOString(),
    extractedFields:
      d.kind === "INVOICE_PDF" || d.kind === "CORRECTED_INVOICE"
        ? {
            supplier: kase.supplier.name,
            invoiceNumber: invoice.number,
            amount: invoice.amount,
            currency: invoice.currency,
          }
        : d.kind === "PURCHASE_ORDER" && invoice.po
          ? { poNumber: invoice.po.number, total: invoice.po.totalAmount }
          : null,
    poNumber: d.kind === "PURCHASE_ORDER" ? invoice.po?.number ?? null : null,
  }));

  return (
    <article>
      <PageHeader
        breadcrumbs={[
          { label: "Approvals", href: "/approvals" },
          { label: approval.reference, href: `/approvals/${approval.reference}` },
        ]}
        title={
          <span className="flex flex-wrap items-center gap-x-3 gap-y-2">
            <span className="font-mono">{invoice.number}</span>
            <span className="text-muted-foreground/50">·</span>
            {kase.supplier.name}
            <StatusBadge
              tone={
                approval.status === "PENDING"
                  ? "warning"
                  : approval.status === "APPROVED"
                    ? "success"
                    : "danger"
              }
            >
              {approval.status.toLowerCase()}
            </StatusBadge>
          </span>
        }
        description={`Approval ${approval.reference} · requested ${formatDate(approval.createdAt)}`}
      />

      <div className="grid items-start gap-6 xl:grid-cols-[minmax(0,1fr)_380px]">
        <div className="flex min-w-0 flex-col gap-5">
          {/* Request */}
          <section className="rounded-lg border border-border bg-surface shadow-xs" aria-label="What is being requested">
            <div className="flex h-9 items-center justify-between border-b px-4">
              <h2 className="text-[12px] font-semibold text-foreground">What is being requested</h2>
              <span className="num text-[13px] font-semibold text-foreground">{amountText}</span>
            </div>
            <div className="px-4 py-3.5">
              <ul className="flex flex-col gap-2 text-[13px] text-foreground">
                <li className="flex gap-2.5">
                  <span className="mt-[7px] h-1 w-1 shrink-0 rounded-full bg-brand" aria-hidden="true" />
                  Mark invoice {invoice.number} as approved
                  <InvoiceStatusBadge status={invoice.status} />
                </li>
                <li className="flex gap-2.5">
                  <span className="mt-[7px] h-1 w-1 shrink-0 rounded-full bg-brand" aria-hidden="true" />
                  Release payment of <span className="font-semibold">{amountText}</span> in the mock ERP
                </li>
                <li className="flex gap-2.5">
                  <span className="mt-[7px] h-1 w-1 shrink-0 rounded-full bg-brand" aria-hidden="true" />
                  Attach an immutable approval record to the case
                </li>
              </ul>
            </div>
          </section>

          {/* Why required */}
          <section className="rounded-lg border border-border bg-surface shadow-xs" aria-label="Why authorization is necessary">
            <div className="flex h-9 items-center border-b px-4">
              <h2 className="text-[12px] font-semibold text-foreground">Why authorization is necessary</h2>
            </div>
            <div className="px-4 py-3.5">
              <p className="text-[13px] leading-relaxed text-muted-foreground">{approval.rationale}</p>
            </div>
          </section>

          {/* Evidence */}
          <section aria-label="Evidence" className="flex flex-col gap-3">
            <h2 className="text-[15px] font-semibold text-foreground">Evidence</h2>
            <EvidenceGrid documents={evidence} />
          </section>

          {/* Case context */}
          <SummaryBlock
            title="Affected operational entity"
            rows={[
              {
                label: "Case",
                value: (
                  <Link href={`/work/${invoice.number}`} className="font-mono text-primary hover:underline underline-offset-4">
                    {invoice.number}
                  </Link>
                ),
              },
              { label: "Supplier", value: kase.supplier.name },
              { label: "Exception", value: kase.exceptionType.toLowerCase().replace(/_/g, " ") },
              { label: "Requested by", value: approval.requestedBy },
              ...(approval.decidedBy
                ? [
                    {
                      label: "Decided by",
                      value: `${approval.decidedBy.name} · ${formatDate(approval.decidedAt ?? approval.createdAt)}`,
                    },
                  ]
                : []),
              ...(approval.comment ? [{ label: "Decision note", value: <span className="text-right">{approval.comment}</span> }] : []),
            ]}
          />
        </div>

        {/* Decision */}
        <div className="flex flex-col gap-5">
          <section className="rounded-lg border border-border bg-surface shadow-xs" aria-label="Decision">
            <div className="flex h-9 items-center border-b px-4">
              <h2 className="text-[12px] font-semibold text-foreground">Decision</h2>
            </div>
            <div className="px-4 py-3.5">
              <ApprovalDecisionPanel
                approvalId={approval.id}
                amountText={amountText}
                supplierName={kase.supplier.name}
                canDecide={user.role === "CONTROLLER"}
                decided={approval.status !== "PENDING"}
              />
            </div>
          </section>

          <Link
            href={`/work/${invoice.number}`}
            className="text-[13px] text-muted-foreground underline-offset-4 hover:text-foreground hover:underline"
          >
            Open full case workspace →
          </Link>
        </div>
      </div>
    </article>
  );
}
