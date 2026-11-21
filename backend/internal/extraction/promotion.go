package extraction

import (
	"context"
	"encoding/json"
	"fmt"

	"atlas/internal/canonical"
)

// Promotion is the outcome of promoting a validated extraction draft.
type Promotion struct {
	SupplierID     string `json:"supplier_id"`
	InvoiceID      string `json:"invoice_id"`
	SupplierMethod string `json:"supplier_method"`
	ReusedInvoice  bool   `json:"reused_invoice"`
}

// ValidateAndPromote applies the validation gate, persists the draft, and
// promotes it only when validated. Rejected results are recorded without
// canonical writes.
func (s *Service) ValidateAndPromote(ctx context.Context, organizationID, documentID string, result Result) (*Draft, *Promotion, error) {
	draft, err := s.Persist(ctx, organizationID, documentID, result)
	if err != nil {
		return nil, nil, err
	}
	if draft.Status != "validated" {
		return draft, nil, nil
	}
	promotion, err := s.Promote(ctx, organizationID, draft.ID)
	if err != nil {
		return draft, nil, err
	}
	return draft, promotion, nil
}

// Promote turns a validated draft into canonical supplier/invoice rows with
// evidence-backed source links. Resolution is exact-name only; ambiguity never
// silently merges. Re-promotion is idempotent by invoice number.
func (s *Service) Promote(ctx context.Context, organizationID, draftID string) (*Promotion, error) {
	var status, documentID, provider string
	var payload []byte
	var confidence float64
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `SELECT set_config('app.organization_id', $1, true)`, organizationID); err != nil {
		_ = tx.Rollback(ctx)
		return nil, err
	}
	err = tx.QueryRow(ctx, `
		SELECT status, document_id, provider, confidence, payload FROM extraction_drafts
		WHERE organization_id = $1 AND id = $2`, organizationID, draftID).
		Scan(&status, &documentID, &provider, &confidence, &payload)
	_ = tx.Rollback(ctx)
	if err != nil {
		return nil, fmt.Errorf("load extraction draft: %w", err)
	}
	if status != "validated" {
		return nil, fmt.Errorf("only validated drafts can be promoted")
	}
	var result Result
	if err := json.Unmarshal(payload, &result); err != nil {
		return nil, err
	}

	canon := canonical.NewService(s.pool)
	promotion := &Promotion{}
	supplierID, method, err := resolveSupplier(ctx, canon, organizationID, result)
	if err != nil {
		return nil, err
	}
	promotion.SupplierMethod = method
	promotion.SupplierID = supplierID

	invoiceID, err := canon.FindInvoiceIDByNumber(ctx, organizationID, result.InvoiceNumber)
	if err != nil {
		return nil, err
	}
	if invoiceID != "" {
		promotion.InvoiceID = invoiceID
		promotion.ReusedInvoice = true
		return promotion, nil
	}
	invoice, err := canon.CreateInvoice(ctx, organizationID, supplierID, result.InvoiceNumber, result.Currency, result.TotalMinor)
	if err != nil {
		return nil, fmt.Errorf("create invoice: %w", err)
	}
	for index, line := range result.Lines {
		if _, err := canon.AddInvoiceLine(ctx, organizationID, invoice.ID, index+1, line.Description,
			line.Quantity, "unit", line.UnitPriceMinor, line.Currency); err != nil {
			return nil, fmt.Errorf("create invoice line: %w", err)
		}
	}
	promotion.InvoiceID = invoice.ID

	// Traceability: link the document-inbox source record when present.
	var sourceRecordID string
	srcTx, srcErr := s.pool.Begin(ctx)
	if srcErr == nil {
		_, _ = srcTx.Exec(ctx, `SELECT set_config('app.organization_id', $1, true)`, organizationID)
		_ = srcTx.QueryRow(ctx, `
			SELECT id FROM source_records
			WHERE organization_id = $1 AND provider = 'document_inbox'
			  AND object_type = 'document' AND external_id = $2`, organizationID, documentID).Scan(&sourceRecordID)
		_ = srcTx.Rollback(ctx)
	}
	if sourceRecordID != "" {
		evidence := map[string]any{"draft_id": draftID, "document_id": documentID, "provider": provider}
		method := "extraction:" + provider
		if err := canon.LinkSource(ctx, organizationID, sourceRecordID, "supplier", supplierID, method, confidence, evidence); err != nil {
			return nil, err
		}
		if err := canon.LinkSource(ctx, organizationID, sourceRecordID, "invoice", invoice.ID, method, confidence, evidence); err != nil {
			return nil, err
		}
	}
	return promotion, nil
}

func resolveSupplier(ctx context.Context, canon *canonical.Service, organizationID string, result Result) (string, string, error) {
	vatID, err := canon.FindSupplierByVAT(ctx, organizationID, result.SupplierVATID)
	if err != nil {
		return "", "", err
	}
	emailID, err := canon.FindSupplierByEmail(ctx, organizationID, result.SupplierEmail)
	if err != nil {
		return "", "", err
	}
	nameID, err := canon.FindSupplierByName(ctx, organizationID, result.SupplierName)
	if err != nil {
		return "", "", err
	}
	if vatID != "" && emailID != "" && vatID != emailID {
		return "", "", fmt.Errorf("ambiguous supplier: VAT and email match different suppliers")
	}
	if vatID != "" && nameID != "" && vatID != nameID {
		return "", "", fmt.Errorf("ambiguous supplier: VAT and name match different suppliers")
	}
	if emailID != "" && nameID != "" && emailID != nameID {
		return "", "", fmt.Errorf("ambiguous supplier: email and name match different suppliers")
	}
	switch {
	case vatID != "":
		return vatID, "vat-exact", nil
	case emailID != "":
		return emailID, "email-exact", nil
	case nameID != "":
		return nameID, "exact-name", nil
	}
	supplier, err := canon.CreateSupplierWithIdentifiers(ctx, organizationID, result.SupplierName, result.Currency, result.SupplierEmail, result.SupplierVATID)
	if err != nil {
		return "", "", fmt.Errorf("create supplier: %w", err)
	}
	return supplier.ID, "jit-create", nil
}
