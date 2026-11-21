package integration

import (
	"context"
	"testing"

	"atlas/internal/canonical"
	"atlas/internal/extraction"
)

func TestExtractionPromotionIsIdempotent(t *testing.T) {
	admin, app, _ := setupTenantSchema(t)
	organizationID := seedQueueOrganization(t, admin)
	ctx := context.Background()

	var integrationID, documentID string
	if err := admin.QueryRow(ctx, `
		INSERT INTO integrations (organization_id, provider, kind, name)
		VALUES ($1, 'document_inbox', 'document_inbox', 'Inbox') RETURNING id`, organizationID).Scan(&integrationID); err != nil {
		t.Fatalf("insert integration: %v", err)
	}
	if err := admin.QueryRow(ctx, `
		INSERT INTO documents (organization_id, kind, status) VALUES ($1, 'invoice', 'pending_upload') RETURNING id`,
		organizationID).Scan(&documentID); err != nil {
		t.Fatalf("insert document: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO source_records (organization_id, integration_id, provider, object_type, external_id, raw_payload, checksum)
		VALUES ($1, $2, 'document_inbox', 'document', $3, '{}'::bytea, repeat('0', 64))`,
		organizationID, integrationID, documentID); err != nil {
		t.Fatalf("insert source record: %v", err)
	}

	service := extraction.NewService(app, 0.8)
	draft, err := service.Persist(ctx, organizationID, documentID, extraction.MockProvider(documentID))
	if err != nil || draft.Status != "validated" {
		t.Fatalf("persist draft: %+v %v", draft, err)
	}
	promotion, err := service.Promote(ctx, organizationID, draft.ID)
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if promotion.SupplierMethod != "jit-create" || promotion.ReusedInvoice {
		t.Fatalf("promotion = %+v", promotion)
	}
	canon := canonical.NewService(app)
	if links, err := canon.CountSourceLinks(ctx, organizationID, "invoice", promotion.InvoiceID); err != nil || links != 1 {
		t.Fatalf("invoice links = %d, %v", links, err)
	}
	if links, err := canon.CountSourceLinks(ctx, organizationID, "supplier", promotion.SupplierID); err != nil || links != 1 {
		t.Fatalf("supplier links = %d, %v", links, err)
	}

	again, err := service.Promote(ctx, organizationID, draft.ID)
	if err != nil || !again.ReusedInvoice || again.InvoiceID != promotion.InvoiceID {
		t.Fatalf("second promotion = %+v %v", again, err)
	}
	var invoices int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM invoices`).Scan(&invoices); err != nil || invoices != 1 {
		t.Fatalf("invoices = %d, %v", invoices, err)
	}

	low := extraction.MockProvider(documentID)
	low.Confidence = 0.3
	rejected, err := service.Persist(ctx, organizationID, documentID, low)
	if err != nil {
		t.Fatalf("persist rejected: %v", err)
	}
	if _, err := service.Promote(ctx, organizationID, rejected.ID); err == nil {
		t.Fatal("rejected draft was promoted")
	}
}
