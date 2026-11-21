package integration

import (
	"context"
	"testing"

	"atlas/internal/canonical"
)

func TestPartialDeliveriesAndReadModels(t *testing.T) {
	admin, app, _ := setupTenantSchema(t)
	organizationID := seedQueueOrganization(t, admin)
	service := canonical.NewService(app)
	ctx := context.Background()

	supplier, err := service.CreateSupplier(ctx, organizationID, "Delivery Supplier", "EUR")
	if err != nil {
		t.Fatalf("create supplier: %v", err)
	}
	order, err := service.CreatePurchaseOrder(ctx, organizationID, supplier.ID, "PO-D1", "EUR", 10000)
	if err != nil {
		t.Fatalf("create PO: %v", err)
	}
	line, err := service.AddPOLine(ctx, organizationID, order.ID, 1, "Parts", "3.000000", "piece", 3333, "EUR")
	if err != nil {
		t.Fatalf("add PO line: %v", err)
	}
	first, err := service.AddDelivery(ctx, organizationID, supplier.ID, order.ID, "DN-D1", "2026-11-01")
	if err != nil {
		t.Fatalf("add delivery 1: %v", err)
	}
	second, err := service.AddDelivery(ctx, organizationID, supplier.ID, order.ID, "DN-D2", "2026-11-05")
	if err != nil {
		t.Fatalf("add delivery 2: %v", err)
	}
	if err := service.AddDeliveryLine(ctx, organizationID, first.ID, line.ID, "1.500000"); err != nil {
		t.Fatalf("add delivery line 1: %v", err)
	}
	if err := service.AddDeliveryLine(ctx, organizationID, second.ID, line.ID, "0.750000"); err != nil {
		t.Fatalf("add delivery line 2: %v", err)
	}

	poDetail, err := service.GetPurchaseOrderDetail(ctx, organizationID, order.ID)
	if err != nil {
		t.Fatalf("get PO detail: %v", err)
	}
	if len(poDetail.Lines) != 1 || poDetail.Lines[0].Fulfilled != "2.250000" {
		t.Fatalf("fulfilled quantity = %+v", poDetail.Lines)
	}

	_, err = service.CreateInvoice(ctx, organizationID, supplier.ID, "INV-D1", "EUR", 5000)
	if err != nil {
		t.Fatalf("create invoice: %v", err)
	}
	supplierDetail, err := service.GetSupplierDetail(ctx, organizationID, supplier.ID)
	if err != nil {
		t.Fatalf("get supplier detail: %v", err)
	}
	if supplierDetail.InvoiceCount != 1 || supplierDetail.OrderCount != 1 {
		t.Fatalf("supplier counts = %+v", supplierDetail)
	}

	// Ontology guard: deprecated active version blocks writes until restored.
	if _, err := admin.Exec(ctx, `UPDATE ontology_versions SET status = 'deprecated' WHERE version = 1`); err != nil {
		t.Fatalf("deprecate ontology: %v", err)
	}
	if _, err := service.CreateSupplier(ctx, organizationID, "Blocked Supplier", "EUR"); err == nil {
		t.Fatal("write succeeded with no active ontology")
	}
	if _, err := admin.Exec(ctx, `UPDATE ontology_versions SET status = 'active' WHERE version = 1`); err != nil {
		t.Fatalf("restore ontology: %v", err)
	}
}
