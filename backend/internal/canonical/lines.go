package canonical

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Line is a canonical invoice or PO line.
type Line struct {
	ID             string `json:"id"`
	LineNumber     int    `json:"line_number"`
	Description    string `json:"description"`
	Quantity       string `json:"quantity"`
	Unit           string `json:"unit"`
	UnitPriceMinor int64  `json:"unit_price_minor"`
	Currency       string `json:"currency"`
}

// InvoiceDetail is the read model used by invoice screens.
type InvoiceDetail struct {
	Invoice          Invoice  `json:"invoice"`
	Lines            []Line   `json:"lines"`
	PurchaseOrderIDs []string `json:"purchase_order_ids"`
}

func (s *Service) AddPOLine(ctx context.Context, organizationID, purchaseOrderID string, lineNumber int, description, quantity, unit string, unitPriceMinor int64, currency string) (*Line, error) {
	if lineNumber <= 0 || unitPriceMinor < 0 {
		return nil, fmt.Errorf("line requires a positive number and non-negative unit price")
	}
	if err := validateCurrency(currency); err != nil {
		return nil, err
	}
	if err := s.ensureOntology(ctx); err != nil {
		return nil, err
	}
	line := &Line{LineNumber: lineNumber, Description: description, Quantity: quantity, Unit: unit, UnitPriceMinor: unitPriceMinor, Currency: currency}
	err := s.withOrg(ctx, organizationID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			INSERT INTO po_lines
				(organization_id, purchase_order_id, line_number, description, quantity, unit, unit_price_minor, currency)
			VALUES ($1, $2, $3, $4, $5::numeric, $6, $7, $8) RETURNING id`,
			organizationID, purchaseOrderID, lineNumber, description, quantity, unit, unitPriceMinor, currency).Scan(&line.ID)
	})
	return line, err
}

func (s *Service) AddInvoiceLine(ctx context.Context, organizationID, invoiceID string, lineNumber int, description, quantity, unit string, unitPriceMinor int64, currency string) (*Line, error) {
	if lineNumber <= 0 || unitPriceMinor < 0 {
		return nil, fmt.Errorf("line requires a positive number and non-negative unit price")
	}
	if err := validateCurrency(currency); err != nil {
		return nil, err
	}
	if err := s.ensureOntology(ctx); err != nil {
		return nil, err
	}
	line := &Line{LineNumber: lineNumber, Description: description, Quantity: quantity, Unit: unit, UnitPriceMinor: unitPriceMinor, Currency: currency}
	err := s.withOrg(ctx, organizationID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			INSERT INTO invoice_lines
				(organization_id, invoice_id, line_number, description, quantity, unit, unit_price_minor, currency)
			VALUES ($1, $2, $3, $4, $5::numeric, $6, $7, $8) RETURNING id`,
			organizationID, invoiceID, lineNumber, description, quantity, unit, unitPriceMinor, currency).Scan(&line.ID)
	})
	return line, err
}

// GetInvoiceDetail returns the invoice, its lines and linked PO ids.
func (s *Service) GetInvoiceDetail(ctx context.Context, organizationID, invoiceID string) (*InvoiceDetail, error) {
	detail := &InvoiceDetail{Lines: []Line{}, PurchaseOrderIDs: []string{}}
	err := s.withOrg(ctx, organizationID, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			SELECT id, supplier_id, number, currency, total_minor FROM invoices
			WHERE organization_id = $1 AND id = $2`, organizationID, invoiceID).
			Scan(&detail.Invoice.ID, &detail.Invoice.SupplierID, &detail.Invoice.Number,
				&detail.Invoice.Currency, &detail.Invoice.TotalMinor); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT id, line_number, description, quantity::text, unit, unit_price_minor, currency
			FROM invoice_lines WHERE organization_id = $1 AND invoice_id = $2 ORDER BY line_number`,
			organizationID, invoiceID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var line Line
			if err := rows.Scan(&line.ID, &line.LineNumber, &line.Description, &line.Quantity,
				&line.Unit, &line.UnitPriceMinor, &line.Currency); err != nil {
				return err
			}
			detail.Lines = append(detail.Lines, line)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		idRows, err := tx.Query(ctx, `
			SELECT purchase_order_id FROM invoice_po_links
			WHERE organization_id = $1 AND invoice_id = $2 ORDER BY purchase_order_id`, organizationID, invoiceID)
		if err != nil {
			return err
		}
		defer idRows.Close()
		for idRows.Next() {
			var id string
			if err := idRows.Scan(&id); err != nil {
				return err
			}
			detail.PurchaseOrderIDs = append(detail.PurchaseOrderIDs, id)
		}
		return idRows.Err()
	})
	if err != nil {
		return nil, err
	}
	return detail, nil
}

func (s *Service) ensureOntology(ctx context.Context) error {
	var version int
	if err := s.pool.QueryRow(ctx, `SELECT version FROM ontology_versions WHERE status = 'active'`).Scan(&version); err != nil {
		return fmt.Errorf("load active ontology: %w", err)
	}
	if version != 1 {
		return fmt.Errorf("unsupported active ontology version %d", version)
	}
	return nil
}
