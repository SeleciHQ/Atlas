package integration

import (
	"context"
	"testing"

	"atlas/internal/extraction"
)

func TestExtractionValidationGate(t *testing.T) {
	admin, app, _ := setupTenantSchema(t)
	organizationID := seedQueueOrganization(t, admin)
	ctx := context.Background()

	var documentID string
	if err := admin.QueryRow(ctx, `
		INSERT INTO documents (organization_id, kind, status) VALUES ($1, 'invoice', 'pending_upload') RETURNING id`,
		organizationID).Scan(&documentID); err != nil {
		t.Fatalf("insert document: %v", err)
	}
	service := extraction.NewService(app, 0.8)

	valid := extraction.MockProvider(documentID)
	draft, err := service.Persist(ctx, organizationID, documentID, valid)
	if err != nil {
		t.Fatalf("persist valid: %v", err)
	}
	if draft.Status != "validated" {
		t.Fatalf("valid draft status = %q", draft.Status)
	}

	lowConfidence := valid
	lowConfidence.Confidence = 0.5
	draft, err = service.Persist(ctx, organizationID, documentID, lowConfidence)
	if err != nil {
		t.Fatalf("persist low confidence: %v", err)
	}
	if draft.Status != "rejected" || len(draft.Reasons) == 0 {
		t.Fatalf("low-confidence draft = %+v", draft)
	}

	invalid := valid
	invalid.Currency = "eur"
	invalid.Lines[0].Quantity = ""
	draft, err = service.Persist(ctx, organizationID, documentID, invalid)
	if err != nil {
		t.Fatalf("persist invalid: %v", err)
	}
	if draft.Status != "rejected" || len(draft.Reasons) < 2 {
		t.Fatalf("invalid draft = %+v", draft)
	}

	count, err := service.CountDrafts(ctx, organizationID, documentID)
	if err != nil || count != 3 {
		t.Fatalf("drafts = %d, %v", count, err)
	}
	var suppliers int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM suppliers`).Scan(&suppliers); err != nil {
		t.Fatalf("count suppliers: %v", err)
	}
	if suppliers != 0 {
		t.Fatal("extraction wrote canonical rows")
	}
}
