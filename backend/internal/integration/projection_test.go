package integration

import (
	"context"
	"testing"

	"atlas/internal/canonical"
)

func TestCanonicalProjectionAndReadModel(t *testing.T) {
	admin, app, _ := setupTenantSchema(t)
	organizationID := seedQueueOrganization(t, admin)
	service := canonical.NewService(app)
	ctx := context.Background()

	var integrationID string
	if err := admin.QueryRow(ctx, `
		INSERT INTO integrations (organization_id, provider, kind, name)
		VALUES ($1, 'mock', 'mock', 'Projection Fixture') RETURNING id`, organizationID).Scan(&integrationID); err != nil {
		t.Fatalf("insert integration: %v", err)
	}
	insertNormalized := func(objectType, externalID, recordType string, payload string) string {
		var sourceID string
		if err := admin.QueryRow(ctx, `
			INSERT INTO source_records (organization_id, integration_id, provider, object_type, external_id, raw_payload, checksum)
			VALUES ($1, $2, 'mock', $3, $4, convert_to($5, 'UTF8'), repeat('0', 64)) RETURNING id`,
			organizationID, integrationID, objectType, externalID, payload).Scan(&sourceID); err != nil {
			t.Fatalf("insert source record: %v", err)
		}
		var normalizedID string
		if err := admin.QueryRow(ctx, `
			INSERT INTO normalized_records (organization_id, source_record_id, record_type, mapping_version, payload)
			VALUES ($1, $2, $3, 'mock-v1', $4::jsonb) RETURNING id`,
			organizationID, sourceID, recordType, payload).Scan(&normalizedID); err != nil {
			t.Fatalf("insert normalized record: %v", err)
		}
		return normalizedID
	}

	supplierNormalized := insertNormalized("supplier", "sup-proj", "supplier.v1",
		`{"name":"Projection Supplier","currency":"EUR"}`)
	supplierID, err := service.ProjectNormalized(ctx, organizationID, supplierNormalized)
	if err != nil {
		t.Fatalf("project supplier: %v", err)
	}
	again, err := service.ProjectNormalized(ctx, organizationID, supplierNormalized)
	if err != nil || again != supplierID {
		t.Fatalf("projection not idempotent: %s %v", again, err)
	}

	invoiceNormalized := insertNormalized("invoice", "inv-proj", "invoice.v1",
		`{"supplier_name":"Projection Supplier","number":"INV-PROJ","total_minor":5000,"currency":"EUR"}`)
	invoiceID, err := service.ProjectNormalized(ctx, organizationID, invoiceNormalized)
	if err != nil {
		t.Fatalf("project invoice: %v", err)
	}
	if _, err := service.AddInvoiceLine(ctx, organizationID, invoiceID, 1, "Widgets", "2.250000", "piece", 2200, "EUR"); err != nil {
		t.Fatalf("add invoice line: %v", err)
	}
	detail, err := service.GetInvoiceDetail(ctx, organizationID, invoiceID)
	if err != nil {
		t.Fatalf("get invoice detail: %v", err)
	}
	if len(detail.Lines) != 1 || detail.Lines[0].Quantity != "2.250000" {
		t.Fatalf("invoice lines = %+v", detail.Lines)
	}
	links, err := service.CountSourceLinks(ctx, organizationID, "supplier", supplierID)
	if err != nil || links != 1 {
		t.Fatalf("supplier source links = %d, %v", links, err)
	}
}
