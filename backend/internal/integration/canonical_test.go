package integration

import (
	"context"
	"testing"

	"atlas/internal/canonical"
)

func TestCanonicalManyToManyAndConstraints(t *testing.T) {
	admin, app, _ := setupTenantSchema(t)
	organizationID := seedQueueOrganization(t, admin)
	service := canonical.NewService(app)
	ctx := context.Background()

	supplier, err := service.CreateSupplier(ctx, organizationID, "Canonical Supplier", "EUR")
	if err != nil {
		t.Fatalf("create supplier: %v", err)
	}
	if _, err := service.CreateSupplier(ctx, organizationID, "Bad Currency", "eur"); err == nil {
		t.Fatal("lowercase currency accepted")
	}
	if _, err := service.CreateSupplier(ctx, organizationID, "Bad Money", "EUR"); err != nil {
		t.Fatalf("valid supplier: %v", err)
	}
	if _, err := service.CreatePurchaseOrder(ctx, organizationID, supplier.ID, "bad", "EUR", -1); err == nil {
		t.Fatal("negative total accepted")
	}
	po1, err := service.CreatePurchaseOrder(ctx, organizationID, supplier.ID, "PO-C1", "EUR", 10000)
	if err != nil {
		t.Fatalf("create PO 1: %v", err)
	}
	po2, err := service.CreatePurchaseOrder(ctx, organizationID, supplier.ID, "PO-C2", "EUR", 20000)
	if err != nil {
		t.Fatalf("create PO 2: %v", err)
	}
	invoice, err := service.CreateInvoice(ctx, organizationID, supplier.ID, "INV-C1", "EUR", 30000)
	if err != nil {
		t.Fatalf("create invoice: %v", err)
	}
	if err := service.LinkInvoicePO(ctx, organizationID, invoice.ID, po1.ID, nil); err != nil {
		t.Fatalf("link PO 1: %v", err)
	}
	if err := service.LinkInvoicePO(ctx, organizationID, invoice.ID, po2.ID, nil); err != nil {
		t.Fatalf("link PO 2: %v", err)
	}
	links, err := service.CountInvoicePOLinks(ctx, organizationID, invoice.ID)
	if err != nil || links != 2 {
		t.Fatalf("invoice PO links = %d, %v", links, err)
	}

	version, err := service.OntologyVersion(ctx)
	if err != nil || version != 1 {
		t.Fatalf("ontology version = %d, %v", version, err)
	}

	var sourceRecordID string
	if err := admin.QueryRow(ctx, `
		INSERT INTO source_records (organization_id, integration_id, provider, object_type, external_id, raw_payload, checksum)
		VALUES ($1,
			(SELECT id FROM integrations WHERE organization_id = $1 LIMIT 1),
			'mock', 'supplier', 'sup-canonical', '{}'::bytea,
			repeat('0', 64)) RETURNING id`, organizationID).Scan(&sourceRecordID); err != nil {
		// No integration exists in this fixture; create one through the ingestion service path.
		if _, err := admin.Exec(ctx, `
			INSERT INTO integrations (organization_id, provider, kind, name)
			VALUES ($1, 'mock', 'mock', 'Canonical Fixture')`, organizationID); err != nil {
			t.Fatalf("insert integration: %v", err)
		}
		if err := admin.QueryRow(ctx, `
			INSERT INTO source_records (organization_id, integration_id, provider, object_type, external_id, raw_payload, checksum)
			VALUES ($1, (SELECT id FROM integrations WHERE organization_id = $1 LIMIT 1),
				'mock', 'supplier', 'sup-canonical', '{}'::bytea, repeat('0', 64)) RETURNING id`,
			organizationID).Scan(&sourceRecordID); err != nil {
			t.Fatalf("insert source record: %v", err)
		}
	}
	if err := service.LinkSource(ctx, organizationID, sourceRecordID, "supplier", supplier.ID, "exact-email", 1.0, map[string]any{"email": "x@y.test"}); err != nil {
		t.Fatalf("link source: %v", err)
	}
	if err := service.LinkSource(ctx, organizationID, sourceRecordID, "supplier", supplier.ID, "bad", 1.5, nil); err == nil {
		t.Fatal("invalid confidence accepted")
	}
	links, err = service.CountSourceLinks(ctx, organizationID, "supplier", supplier.ID)
	if err != nil || links != 1 {
		t.Fatalf("source links = %d, %v", links, err)
	}

	// RLS: a different organization cannot see these canonical rows.
	otherOrg := seedQueueOrganization(t, admin)
	otherService := canonical.NewService(app)
	if _, err := otherService.CreateSupplier(ctx, otherOrg, "Other Org Supplier", "EUR"); err != nil {
		t.Fatalf("create other supplier: %v", err)
	}
	var visible int
	if err := app.QueryRow(ctx, `SELECT count(*) FROM suppliers`).Scan(&visible); err != nil {
		t.Fatalf("count suppliers without context: %v", err)
	}
	if visible != 0 {
		t.Fatalf("RLS leaked %d suppliers without organization context", visible)
	}
}
