import { getSuppliersList } from "@/lib/queries";
import { PageHeader } from "@/components/app-shell/app-shell";
import { SuppliersTable, type SupplierRow } from "@/components/suppliers/suppliers-table";

export const dynamic = "force-dynamic";

export default async function SuppliersPage() {
  const suppliers = await getSuppliersList();
  const rows: SupplierRow[] = suppliers.map((s) => ({
    code: s.code,
    name: s.name,
    country: s.country,
    contactEmail: s.contactEmail,
    status: s.status,
    invoiceCount: s.invoiceCount,
    openCases: s.openDiscrepancies,
    totalSpend: s.totalSpend,
    lastActivity: s.invoices
      .map((i) => (i.case?.updatedAt ?? i.invoiceDate).toISOString())
      .sort()
      .at(-1) ?? new Date(0).toISOString(),
  }));

  return (
    <div>
      <PageHeader
        title="Suppliers"
        description="Persistent supplier profiles with invoices, exceptions, and activity."
      />
      <SuppliersTable rows={rows} />
    </div>
  );
}
