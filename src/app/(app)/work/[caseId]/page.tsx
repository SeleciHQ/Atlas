import { notFound } from "next/navigation";
import Link from "next/link";
import { getCaseDetail } from "@/lib/queries";
import { formatCurrency, ageLabel, formatDate } from "@/lib/format";
import { CaseStatusBadge } from "@/components/shared/badges";
import { PageHeader } from "@/components/app-shell/app-shell";
import { EvidenceGrid, type EvidenceDoc } from "@/components/evidence/evidence-grid";
import { RecommendationCard } from "@/components/agent/recommendation-card";
import { PolicyCard } from "@/components/policy/policy-card";
import { CaseActionPanel } from "@/components/case/action-panel";
import { LineComparisonTable, SummaryBlock, CardSection } from "@/components/case/summary-blocks";
import { CaseTimelineCard } from "@/components/case/case-timeline";
import { EXCEPTION_LABELS } from "@/types/ontology";
import { DEFAULT_POLICY_CONFIG } from "@/types/policy";
import { supplierPath } from "@/lib/ontology";

export const dynamic = "force-dynamic";

export default async function CasePage(props: PageProps<"/work/[caseId]">) {
  const { caseId: invoiceNumber } = await props.params;
  const detail = await getCaseDetail(invoiceNumber);
  if (!detail) notFound();

  const { invoice, supplier, po, shipment, agent, comparison } = detail;
  const ageDays = detail.ageDays;
  const amountText = formatCurrency(invoice.amount, invoice.currency);
  const fmtDate = (d: Date) => formatDate(d);

  const evidence: EvidenceDoc[] = detail.documents.map((d) => {
    let extracted: Record<string, unknown> | null = null;
    if (d.extractedFields) {
      try {
        extracted = JSON.parse(d.extractedFields) as Record<string, unknown>;
      } catch {
        extracted = null;
      }
    }
    const delivery = shipment?.deliveryEvents.find((ev) => ev.reference && d.name.startsWith(ev.reference));
    const poMatch = d.kind === "PURCHASE_ORDER" ? po : null;
    return {
      id: d.id,
      kind: d.kind,
      name: d.name,
      createdAt: d.createdAt.toISOString(),
      extractedFields: extracted ?? (d.kind === "INVOICE_PDF" || d.kind === "CORRECTED_INVOICE"
        ? {
            supplier: supplier.name,
            invoiceNumber: invoice.number,
            amount: invoice.amount,
            currency: invoice.currency,
            quantity: invoice.lines.reduce((s, l) => s + l.invoicedQuantity, 0),
            unitPrice: invoice.lines[0]?.unitPrice ?? 0,
            invoiceDate: invoice.invoiceDate.toISOString().slice(0, 10),
          }
        : poMatch
          ? {
              poNumber: poMatch.number,
              supplier: supplier.name,
              total: poMatch.totalAmount,
              issuedAt: poMatch.issuedAt.toISOString().slice(0, 10),
            }
          : delivery
            ? {
                receivedNote: delivery.receivedNote ?? `${delivery.lines.reduce((s, l) => s + l.quantity, 0)} units received`,
              }
            : {}),
      deliveryRef: delivery?.reference ?? null,
      deliveryType: delivery?.type ?? null,
      deliveredAt: delivery?.deliveredAt?.toISOString() ?? null,
      deliveryLines: delivery?.lines.map((l) => ({ item: l.item, quantity: l.quantity })) ?? null,
      photoNote: delivery?.photoNote ?? null,
      poNumber: d.kind === "PURCHASE_ORDER" ? po?.number ?? null : null,
    };
  });

  const whySteps = [
    po ? `Matched invoice ${invoice.number} to ${po.number}.` : `No purchase order matched invoice ${invoice.number}.`,
    shipment && shipment.deliveryEvents.length > 0
      ? `Retrieved delivery event ${shipment.deliveryEvents[0].reference}.`
      : "No delivery event found for this purchase order.",
    ...comparison
      .filter((r) => !r.passed)
      .map(
        (r) =>
          `Found ${Math.abs(r.difference)}-unit difference for ${r.item} (invoiced ${r.invoiced}, received ${r.received}).`
      ),
    agent.priceVariancePct !== null
      ? `Checked price tolerance: ${agent.priceVariancePct.toFixed(1)}% variance versus ${DEFAULT_POLICY_CONFIG.priceTolerancePct}% allowed.`
      : `Checked price tolerance: within ${DEFAULT_POLICY_CONFIG.priceTolerancePct}% allowed.`,
    `Checked supplier history: ${detail.supplierHistory.length} prior ${detail.supplierHistory.length === 1 ? "discrepancy" : "discrepancies"}.`,
  ];

  const violations = detail.approveEvaluation.violations.filter(
    (v) => v.rule === "quantity_rule" || v.rule === "price_tolerance"
  );

  return (
    <article>
      <PageHeader
        breadcrumbs={[
          { label: "Work", href: "/work" },
          { label: invoice.number, href: `/work/${invoice.number}` },
        ]}
        title={
          <span className="flex flex-wrap items-center gap-x-3 gap-y-2">
            <span className="font-mono">{invoice.number}</span>
            <span className="text-muted-foreground/50">·</span>
            <Link href={supplierPath(supplier.code)} className="text-primary hover:underline underline-offset-4">
              {supplier.name}
            </Link>
            <CaseStatusBadge status={detail.status} />
          </span>
        }
        description={
          <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <span className="font-semibold text-foreground">{amountText}</span>
            <span>·</span>
            {EXCEPTION_LABELS[detail.exceptionType]}
            <span>·</span>
            {ageDays === 0 ? "Created today" : `Created ${ageLabel(detail.createdAt)}`}
          </span>
        }
      />

      <div className="grid items-start gap-6 xl:grid-cols-[minmax(0,1fr)_380px]">
        <div className="flex min-w-0 flex-col gap-5">
          <section aria-label="What happened" className="flex flex-col gap-4">
            <h2 className="text-[15px] font-semibold text-foreground">What happened</h2>
            <SummaryBlock
              title="Invoice"
              rows={[
                { label: "Invoice number", value: <span className="font-mono">{invoice.number}</span> },
                { label: "Supplier", value: supplier.name },
                { label: "Amount", value: <span className="num">{amountText}</span> },
                { label: "Currency", value: invoice.currency },
                {
                  label: "Quantity",
                  value: <span className="num">{invoice.lines.reduce((s, l) => s + l.invoicedQuantity, 0).toLocaleString()} units</span>,
                },
                { label: "Unit price", value: <span className="num">${(invoice.lines[0]?.unitPrice ?? 0).toFixed(2)}</span> },
                { label: "Invoice date", value: fmtDate(invoice.invoiceDate) },
              ]}
              footer={detail.summary}
            />

            <div>
              <h3 className="label-caps mb-2.5">Evidence</h3>
              <EvidenceGrid documents={evidence} />
            </div>

            <CardSection title="Line-level comparison">
              <div className="overflow-x-auto">
                <LineComparisonTable comparison={comparison} />
              </div>
            </CardSection>

            <SummaryBlock
              title="Purchase order"
              rows={
                po
                  ? [
                      { label: "PO number", value: <span className="font-mono">{po.number}</span> },
                      { label: "Status", value: po.status.toLowerCase() },
                      { label: "Issued", value: fmtDate(po.issuedAt) },
                      { label: "PO total", value: <span className="num">{formatCurrency(po.totalAmount, po.currency)}</span> },
                      ...po.lines.map((l) => ({
                        label: `${l.item} (${l.sku})`,
                        value: <span className="num">{`${l.quantity.toLocaleString()} × $${l.unitPrice.toFixed(2)}`}</span>,
                      })),
                    ]
                  : [{ label: "PO number", value: <span className="font-medium text-danger">Not matched</span> }]
              }
              footer={
                po
                  ? undefined
                  : "This invoice could not be tied to an approved purchase order. Procurement must confirm the requisition."
              }
            />

            <SummaryBlock
              title="Delivery"
              tone={shipment && shipment.deliveryEvents.length === 0 ? "warning" : "default"}
              rows={
                shipment && shipment.deliveryEvents.length > 0
                  ? [
                      { label: "Shipment", value: <span className="font-mono">{shipment.reference}</span> },
                      { label: "Carrier", value: shipment.carrier },
                      { label: "Status", value: shipment.status.toLowerCase().replace(/_/g, " ") },
                      ...shipment.deliveryEvents.flatMap((ev) => [
                        { label: `Event ${ev.reference}`, value: <span className="font-mono">{ev.type.toLowerCase().replace(/_/g, " ")}</span> },
                        ...ev.lines.map((l) => ({ label: l.item, value: <span className="num">{`${l.quantity.toLocaleString()} received`}</span> })),
                      ]),
                    ]
                  : [{ label: "Delivery record", value: <span className="font-medium text-warning">None found</span> }]
              }
              footer={
                shipment && shipment.deliveryEvents.length === 0
                  ? "No goods received note is on file. Payment cannot be approved until delivery is confirmed."
                  : undefined
              }
            />

            <SummaryBlock
              title="Supplier history"
              rows={[
                { label: "Status", value: supplier.status.toLowerCase().replace(/_/g, " ") },
                { label: "Country", value: supplier.country },
                { label: "Contact", value: supplier.contactEmail },
                { label: "Prior cases", value: <span className="num">{detail.supplierHistory.length}</span> },
              ]}
              footer={
                detail.supplierHistory.length > 0 ? (
                  <ul className="flex flex-col gap-1">
                    {detail.supplierHistory.slice(0, 4).map((h) => (
                      <li key={h.invoiceNumber} className="flex items-baseline justify-between gap-3">
                        <Link href={`/work/${h.invoiceNumber}`} className="font-mono text-[12.5px] text-primary hover:underline underline-offset-4">
                          {h.invoiceNumber}
                        </Link>
                        <span className="text-[12px] text-muted-foreground">
                          {EXCEPTION_LABELS[h.exceptionType as keyof typeof EXCEPTION_LABELS] ?? h.exceptionType} ·{" "}
                          {h.status.toLowerCase()}
                        </span>
                      </li>
                    ))}
                  </ul>
                ) : (
                  "No prior discrepancies on record for this supplier."
                )
              }
            />
          </section>

          <section aria-label="Case timeline">
            <CaseTimelineCard events={detail.auditEvents} />
          </section>
        </div>

        <div className="flex flex-col gap-5">
          <section aria-label="What should we do" className="flex flex-col gap-4">
            <h2 className="text-[15px] font-semibold text-foreground">What should we do?</h2>
            <RecommendationCard
              recommendation={agent.recommendation}
              confidence={agent.confidence}
              evidenceCount={agent.evidenceCount}
              policyName={DEFAULT_POLICY_CONFIG.name}
              whySteps={whySteps}
              violations={violations}
            />
            <PolicyCard
              policyVersion={DEFAULT_POLICY_CONFIG.version}
              rules={[
                `Human approval required for payments above $${DEFAULT_POLICY_CONFIG.approvalThreshold.toLocaleString()}.`,
                "Agent cannot approve payment directly.",
                "Agent may request documents and escalate cases.",
                "Invoice quantity cannot exceed received quantity.",
                `Price variance above ${DEFAULT_POLICY_CONFIG.priceTolerancePct}% requires review.`,
              ]}
              approveEvaluation={detail.approveEvaluation}
              amountText={amountText}
            />
            <CaseActionPanel
              caseId={detail.caseId}
              status={detail.status}
              supplierName={supplier.name}
              amountText={amountText}
              approveBlockedReason={detail.approveEvaluation.allowed ? null : detail.approveEvaluation.blockedReason}
              requiresApproval={detail.approveEvaluation.requiresApproval}
              isController={detail.role === "CONTROLLER"}
            />
          </section>
        </div>
      </div>
    </article>
  );
}
