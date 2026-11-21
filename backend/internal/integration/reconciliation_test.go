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
