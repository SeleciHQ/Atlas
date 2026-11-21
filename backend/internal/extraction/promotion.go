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
	supplierID, err := canon.FindSupplierByName(ctx, organizationID, result.SupplierName)
	if err != nil {
		return nil, err
	}
	if supplierID != "" {
		promotion.SupplierMethod = "exact-name"
	} else {
		supplier, err := canon.CreateSupplier(ctx, organizationID, result.SupplierName, result.Currency)
		if err != nil {
			return nil, fmt.Errorf("create supplier: %w", err)
		}
		supplierID = supplier.ID
		promotion.SupplierMethod = "jit-create"
	}
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
