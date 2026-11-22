package integration

import (
	"context"
	"testing"

	"atlas/internal/canonical"
	"atlas/internal/reconciliation"
)

func TestReconciliationDeterministic(t *testing.T) {
	admin, app, _ := setupTenantSchema(t)
	organizationID := seedQueueOrganization(t, admin)
	ctx := context.Background()
	canon := canonical.NewService(app)
	service := reconciliation.NewService(app)

	supplier, err := canon.CreateSupplier(ctx, organizationID, "Recon Supplier", "EUR")
	if err != nil {
		t.Fatalf("supplier: %v", err)
	}
	po1, err := canon.CreatePurchaseOrder(ctx, organizationID, supplier.ID, "PO-R1", "EUR", 10000)
	if err != nil {
		t.Fatalf("po1: %v", err)
	}
	po2, err := canon.CreatePurchaseOrder(ctx, organizationID, supplier.ID, "PO-R2", "EUR", 5000)
	if err != nil {
		t.Fatalf("po2: %v", err)
	}

	exact, err := canon.CreateInvoice(ctx, organizationID, supplier.ID, "INV-R1", "EUR", 10000)
	if err != nil {
		t.Fatalf("invoice: %v", err)
	}
	if err := canon.LinkInvoicePO(ctx, organizationID, exact.ID, po1.ID, nil); err != nil {
		t.Fatalf("link: %v", err)
	}
	result, err := service.Run(ctx, organizationID, exact.ID)
	if err != nil || result.Matches != 1 || result.Discrepancies != 0 {
		t.Fatalf("exact match = %+v %v", result, err)
	}

	split, err := canon.CreateInvoice(ctx, organizationID, supplier.ID, "INV-R2", "EUR", 15000)
	if err != nil {
		t.Fatalf("split invoice: %v", err)
	}
	if err := canon.LinkInvoicePO(ctx, organizationID, split.ID, po1.ID, nil); err != nil {
		t.Fatalf("link split 1: %v", err)
	}
	if err := canon.LinkInvoicePO(ctx, organizationID, split.ID, po2.ID, nil); err != nil {
		t.Fatalf("link split 2: %v", err)
	}
	result, err = service.Run(ctx, organizationID, split.ID)
	if err != nil || result.Matches != 2 || result.Discrepancies != 0 {
		t.Fatalf("split match = %+v %v", result, err)
	}
	detail, err := service.GetRun(ctx, organizationID, result.RunID)
	if err != nil || len(detail.Matches) != 2 || len(detail.Discrepancies) != 0 {
		t.Fatalf("run detail = %+v %v", detail, err)
	}
	runs, err := service.ListByInvoice(ctx, organizationID, split.ID)
	if err != nil || len(runs) != 1 || runs[0].RunID != result.RunID {
		t.Fatalf("list runs = %+v %v", runs, err)
	}

	mismatch, err := canon.CreateInvoice(ctx, organizationID, supplier.ID, "INV-R3", "EUR", 9999)
	if err != nil {
		t.Fatalf("mismatch invoice: %v", err)
	}
	if err := canon.LinkInvoicePO(ctx, organizationID, mismatch.ID, po1.ID, nil); err != nil {
		t.Fatalf("link mismatch: %v", err)
	}
	result, err = service.Run(ctx, organizationID, mismatch.ID)
	if err != nil || result.Matches != 0 || result.Discrepancies != 1 {
		t.Fatalf("mismatch = %+v %v", result, err)
	}

	unlinked, err := canon.CreateInvoice(ctx, organizationID, supplier.ID, "INV-R4", "EUR", 1000)
	if err != nil {
		t.Fatalf("unlinked invoice: %v", err)
	}
	result, err = service.Run(ctx, organizationID, unlinked.ID)
	if err != nil || result.Discrepancies != 1 {
		t.Fatalf("unlinked = %+v %v", result, err)
	}
}

func TestReconciliationLineAndDeliveryMatching(t *testing.T) {
	admin, app, _ := setupTenantSchema(t)
	organizationID := seedQueueOrganization(t, admin)
	ctx := context.Background()
	canon := canonical.NewService(app)
	service := reconciliation.NewService(app)
	supplier, err := canon.CreateSupplier(ctx, organizationID, "Line Supplier", "EUR")
	if err != nil {
		t.Fatalf("supplier: %v", err)
	}
	po, err := canon.CreatePurchaseOrder(ctx, organizationID, supplier.ID, "PO-L1", "EUR", 10000)
	if err != nil {
		t.Fatalf("po: %v", err)
	}
	poLine, err := canon.AddPOLine(ctx, organizationID, po.ID, 1, "Widgets", "2.000000", "piece", 5000, "EUR")
	if err != nil {
		t.Fatalf("po line: %v", err)
	}
	invoice, err := canon.CreateInvoice(ctx, organizationID, supplier.ID, "INV-L1", "EUR", 10000)
	if err != nil {
		t.Fatalf("invoice: %v", err)
	}
	if _, err := canon.AddInvoiceLine(ctx, organizationID, invoice.ID, 1, "Widgets", "2.000000", "piece", 5000, "EUR"); err != nil {
		t.Fatalf("invoice line: %v", err)
	}
	if err := canon.LinkInvoicePO(ctx, organizationID, invoice.ID, po.ID, nil); err != nil {
		t.Fatalf("link: %v", err)
	}
	delivery, err := canon.AddDelivery(ctx, organizationID, supplier.ID, po.ID, "DN-L1", "2026-11-10")
	if err != nil {
		t.Fatalf("delivery: %v", err)
	}
	if err := canon.AddDeliveryLine(ctx, organizationID, delivery.ID, poLine.ID, "1.000000"); err != nil {
		t.Fatalf("delivery line: %v", err)
	}
	result, err := service.Run(ctx, organizationID, invoice.ID)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Matches != 2 || result.Discrepancies != 1 {
		t.Fatalf("line/shortfall = %+v", result)
	}
	var kind string
	if err := admin.QueryRow(ctx, `SELECT kind FROM reconciliation_discrepancies WHERE run_id = $1`, result.RunID).Scan(&kind); err != nil || kind != "delivery_shortfall" {
		t.Fatalf("discrepancy = %q %v", kind, err)
	}

	unmatchedInvoice, err := canon.CreateInvoice(ctx, organizationID, supplier.ID, "INV-L2", "EUR", 10000)
	if err != nil {
		t.Fatalf("invoice 2: %v", err)
	}
	if _, err := canon.AddInvoiceLine(ctx, organizationID, unmatchedInvoice.ID, 1, "Different", "2.000000", "piece", 5000, "EUR"); err != nil {
		t.Fatalf("invoice line 2: %v", err)
	}
	if err := canon.LinkInvoicePO(ctx, organizationID, unmatchedInvoice.ID, po.ID, nil); err != nil {
		t.Fatalf("link 2: %v", err)
	}
	result, err = service.Run(ctx, organizationID, unmatchedInvoice.ID)
	if err != nil || result.Matches != 1 || result.Discrepancies != 1 {
		t.Fatalf("unmatched line = %+v %v", result, err)
	}
}

func TestReconciliationPreventsDuplicateLineConsumption(t *testing.T) {
	admin, app, _ := setupTenantSchema(t)
	organizationID := seedQueueOrganization(t, admin)
	ctx := context.Background()
	canon := canonical.NewService(app)
	service := reconciliation.NewService(app)
	supplier, err := canon.CreateSupplier(ctx, organizationID, "Dup Supplier", "EUR")
	if err != nil {
		t.Fatalf("supplier: %v", err)
	}
	po, err := canon.CreatePurchaseOrder(ctx, organizationID, supplier.ID, "PO-DUP", "EUR", 10000)
	if err != nil {
		t.Fatalf("po: %v", err)
	}
	if _, err := canon.AddPOLine(ctx, organizationID, po.ID, 1, "Widgets", "2.000000", "piece", 5000, "EUR"); err != nil {
		t.Fatalf("po line: %v", err)
	}
	invoice, err := canon.CreateInvoice(ctx, organizationID, supplier.ID, "INV-DUP", "EUR", 10000)
	if err != nil {
		t.Fatalf("invoice: %v", err)
	}
	for line := 1; line <= 2; line++ {
		if _, err := canon.AddInvoiceLine(ctx, organizationID, invoice.ID, line, "Widgets", "2.000000", "piece", 5000, "EUR"); err != nil {
			t.Fatalf("invoice line: %v", err)
		}
	}
	if err := canon.LinkInvoicePO(ctx, organizationID, invoice.ID, po.ID, nil); err != nil {
		t.Fatalf("link: %v", err)
	}
	result, err := service.Run(ctx, organizationID, invoice.ID)
	if err != nil || result.Matches != 2 || result.Discrepancies != 2 {
		t.Fatalf("duplicate consumption = %+v %v", result, err)
	}
	detail, err := service.GetRun(ctx, organizationID, result.RunID)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	lineMatches := 0
	for _, match := range detail.Matches {
		if match.MatchType == "line" {
			lineMatches++
		}
	}
	if lineMatches != 1 {
		t.Fatalf("PO line consumed %d times", lineMatches)
	}
}
