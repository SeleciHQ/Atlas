import { Suspense } from "react";
import { getInvoicesList } from "@/lib/queries";
import { PageHeader } from "@/components/app-shell/app-shell";
import { InvoicesTable, type InvoiceRow } from "@/components/invoices/invoices-table";

export const dynamic = "force-dynamic";

export default async function InvoicesPage() {
  const invoices = await getInvoicesList();
  const rows: InvoiceRow[] = invoices.map((i) => ({
    number: i.number,
    supplierName: i.supplier.name,
    supplierCode: i.supplier.code,
    amount: i.amount,
    currency: i.currency,
    invoiceDate: i.invoiceDate.toISOString(),
    dueDate: i.dueDate?.toISOString() ?? null,
    poNumber: i.po?.number ?? null,
    status: i.status,
    exceptionType: i.case?.exceptionType ?? null,
    caseStatus: i.case?.status ?? null,
    updatedAt: i.case?.updatedAt.toISOString() ?? i.invoiceDate.toISOString(),
  }));

  return (
    <div>
      <PageHeader
        title="Invoices"
        description="All invoice records across suppliers, with reconciliation and payment status."
      />
      <Suspense>
        <InvoicesTable rows={rows} />
      </Suspense>
    </div>
  );
}
