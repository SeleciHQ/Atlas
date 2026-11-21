import { getApprovalsList } from "@/lib/queries";
import { PageHeader } from "@/components/app-shell/app-shell";
import { ApprovalsView, type ApprovalRecord } from "@/components/approvals/approvals-view";

export const dynamic = "force-dynamic";

export default async function ApprovalsPage() {
  const { user, approvals } = await getApprovalsList();
  const records: ApprovalRecord[] = approvals.map((a) => ({
    id: a.id,
    reference: a.reference,
    invoiceNumber: a.case.invoice.number,
    supplierName: a.case.supplier.name,
    supplierCode: a.case.supplier.code,
    amount: a.amount,
    currency: a.currency,
    action: a.action,
    status: a.status,
    rationale: a.rationale,
    requestedBy: a.requestedBy,
    decidedByName: a.decidedBy?.name ?? null,
    decidedAt: a.decidedAt?.toISOString() ?? null,
    evidenceCount: a.case.documents.length,
    createdAt: a.createdAt.toISOString(),
  }));

  return (
    <div>
      <PageHeader
        title="Approvals"
        description="Authorization decisions for high-value payment actions."
      />
      <ApprovalsView approvals={records} canDecide={user.role === "CONTROLLER"} />
    </div>
  );
}
