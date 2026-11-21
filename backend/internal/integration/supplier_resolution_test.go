package integration

import (
	"context"
	"testing"

	"atlas/internal/canonical"
	"atlas/internal/extraction"
)

func TestSupplierCandidateStrengthAndAmbiguity(t *testing.T) {
	admin, app, _ := setupTenantSchema(t)
	organizationID := seedQueueOrganization(t, admin)
	ctx := context.Background()
	canon := canonical.NewService(app)
	service := extraction.NewService(app, 0.8)

	var documentID string
	if err := admin.QueryRow(ctx, `INSERT INTO documents (organization_id, kind, status) VALUES ($1, 'invoice', 'pending_upload') RETURNING id`, organizationID).Scan(&documentID); err != nil {
		t.Fatalf("insert document: %v", err)
	}
	vatSupplier, err := canon.CreateSupplierWithIdentifiers(ctx, organizationID, "VAT Supplier", "EUR", "va@example.test", "DE111")
	if err != nil {
		t.Fatalf("create VAT supplier: %v", err)
	}
	if _, err := canon.CreateSupplierWithIdentifiers(ctx, organizationID, "Email Supplier", "EUR", "em@example.test", "DE222"); err != nil {
		t.Fatalf("create email supplier: %v", err)
	}

	matched := extraction.MockProvider(documentID)
	matched.SupplierName = "Unrelated Name"
	matched.SupplierVATID = "DE111"
	matched.SupplierEmail = "va@example.test"
	draft, err := service.Persist(ctx, organizationID, documentID, matched)
	if err != nil {
		t.Fatalf("persist matched: %v", err)
	}
	promotion, err := service.Promote(ctx, organizationID, draft.ID)
	if err != nil || promotion.SupplierID != vatSupplier.ID || promotion.SupplierMethod != "vat-exact" {
		t.Fatalf("VAT match = %+v %v", promotion, err)
	}

	conflict := extraction.MockProvider(documentID)
	conflict.InvoiceNumber = "INV-CONFLICT"
	conflict.SupplierName = "Conflict"
	conflict.SupplierVATID = "DE111"
	conflict.SupplierEmail = "em@example.test"
	draft, err = service.Persist(ctx, organizationID, documentID, conflict)
	if err != nil {
		t.Fatalf("persist conflict: %v", err)
	}
	if _, err := service.Promote(ctx, organizationID, draft.ID); err == nil {
		t.Fatal("conflicting VAT/email evidence was merged")
	}

	created := extraction.MockProvider(documentID)
	created.InvoiceNumber = "INV-NEW-VAT"
	created.SupplierName = "New Identified Supplier"
	created.SupplierVATID = "DE999"
	created.SupplierEmail = "new@example.test"
	draft, err = service.Persist(ctx, organizationID, documentID, created)
	if err != nil {
		t.Fatalf("persist created: %v", err)
	}
	promotion, err = service.Promote(ctx, organizationID, draft.ID)
	if err != nil || promotion.SupplierMethod != "jit-create" {
		t.Fatalf("jit create = %+v %v", promotion, err)
	}
	var email, vat string
	if err := admin.QueryRow(ctx, `SELECT email, vat_id FROM suppliers WHERE id = $1`, promotion.SupplierID).Scan(&email, &vat); err != nil {
		t.Fatalf("read identifiers: %v", err)
	}
	if email != "new@example.test" || vat != "DE999" {
		t.Fatalf("identifiers = %q/%q", email, vat)
	}
}
