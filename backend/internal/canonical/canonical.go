// Package canonical implements the org-scoped canonical v0 model and ontology
// validation for the invoice-reconciliation domain.
package canonical

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

// Service provides canonical writes and reads. Every call is organization
// scoped through the RLS-bound transaction.
type Service struct {
	pool *pgxpool.Pool
}

// NewService constructs the canonical service.
func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// Supplier is a canonical supplier.
type Supplier struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Currency string `json:"currency"`
}

// Invoice is a canonical invoice header.
type Invoice struct {
	ID         string `json:"id"`
	SupplierID string `json:"supplier_id"`
	Number     string `json:"number"`
	Currency   string `json:"currency"`
	TotalMinor int64  `json:"total_minor"`
}

// PurchaseOrder is a canonical purchase order header.
type PurchaseOrder struct {
	ID         string `json:"id"`
	SupplierID string `json:"supplier_id"`
	Number     string `json:"number"`
	Currency   string `json:"currency"`
	TotalMinor int64  `json:"total_minor"`
}

func validateCurrency(currency string) error {
	if !currencyPattern.MatchString(currency) {
		return fmt.Errorf("currency must be an ISO-4217 uppercase code")
	}
	return nil
}

// CreateSupplier inserts a canonical supplier.
func (s *Service) CreateSupplier(ctx context.Context, organizationID, name, currency string) (*Supplier, error) {
	if name == "" {
		return nil, fmt.Errorf("supplier name is required")
	}
	if err := validateCurrency(currency); err != nil {
		return nil, err
	}
	supplier := &Supplier{Name: name, Currency: currency}
	err := s.withOrg(ctx, organizationID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			INSERT INTO suppliers (organization_id, name, currency)
			VALUES ($1, $2, $3) RETURNING id`, organizationID, name, currency).Scan(&supplier.ID)
	})
	return supplier, err
}

// CreatePurchaseOrder inserts a canonical purchase order.
func (s *Service) CreatePurchaseOrder(ctx context.Context, organizationID, supplierID, number, currency string, totalMinor int64) (*PurchaseOrder, error) {
	if number == "" || totalMinor < 0 {
		return nil, fmt.Errorf("purchase order requires a number and non-negative total")
	}
	if err := validateCurrency(currency); err != nil {
		return nil, err
	}
	order := &PurchaseOrder{SupplierID: supplierID, Number: number, Currency: currency, TotalMinor: totalMinor}
	err := s.withOrg(ctx, organizationID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			INSERT INTO purchase_orders (organization_id, supplier_id, number, currency, total_minor)
			VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			organizationID, supplierID, number, currency, totalMinor).Scan(&order.ID)
	})
	return order, err
}

// CreateInvoice inserts a canonical invoice.
func (s *Service) CreateInvoice(ctx context.Context, organizationID, supplierID, number, currency string, totalMinor int64) (*Invoice, error) {
	if number == "" || totalMinor < 0 {
		return nil, fmt.Errorf("invoice requires a number and non-negative total")
	}
	if err := validateCurrency(currency); err != nil {
		return nil, err
	}
	invoice := &Invoice{SupplierID: supplierID, Number: number, Currency: currency, TotalMinor: totalMinor}
	err := s.withOrg(ctx, organizationID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			INSERT INTO invoices (organization_id, supplier_id, number, currency, total_minor)
			VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			organizationID, supplierID, number, currency, totalMinor).Scan(&invoice.ID)
	})
	return invoice, err
}

// LinkInvoicePO creates an invoice-to-purchase-order link (many-to-many).
func (s *Service) LinkInvoicePO(ctx context.Context, organizationID, invoiceID, purchaseOrderID string, allocatedMinor *int64) error {
	return s.withOrg(ctx, organizationID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO invoice_po_links (organization_id, invoice_id, purchase_order_id, allocated_minor)
			VALUES ($1, $2, $3, $4)`, organizationID, invoiceID, purchaseOrderID, allocatedMinor)
		return err
	})
}

// LinkSource records evidence-backed provenance from a source record to a
// canonical entity. Entity resolution populates these in Phase 6.
func (s *Service) LinkSource(ctx context.Context, organizationID, sourceRecordID, entityType, entityID, method string, confidence float64, evidence map[string]any) error {
	if confidence < 0 || confidence > 1 {
		return fmt.Errorf("confidence must be between 0 and 1")
	}
	details, err := json.Marshal(evidence)
	if err != nil {
		return err
	}
	return s.withOrg(ctx, organizationID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO source_entity_links
				(organization_id, source_record_id, entity_type, entity_id, method, confidence, evidence)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			organizationID, sourceRecordID, entityType, entityID, method, confidence, details)
		return err
	})
}

// CountInvoicePOLinks returns the number of POs linked to an invoice.
func (s *Service) CountInvoicePOLinks(ctx context.Context, organizationID, invoiceID string) (int, error) {
	var count int
	err := s.withOrg(ctx, organizationID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT count(*) FROM invoice_po_links WHERE organization_id = $1 AND invoice_id = $2`,
			organizationID, invoiceID).Scan(&count)
	})
	return count, err
}

// CountSourceLinks returns the number of provenance links for an entity.
func (s *Service) CountSourceLinks(ctx context.Context, organizationID, entityType, entityID string) (int, error) {
	var count int
	err := s.withOrg(ctx, organizationID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT count(*) FROM source_entity_links
			WHERE organization_id = $1 AND entity_type = $2 AND entity_id = $3`,
			organizationID, entityType, entityID).Scan(&count)
	})
	return count, err
}

// OntologyVersion returns the active ontology version.
func (s *Service) OntologyVersion(ctx context.Context) (int, error) {
	var version int
	err := s.pool.QueryRow(ctx, `SELECT version FROM ontology_versions WHERE status = 'active'`).Scan(&version)
	return version, err
}

func (s *Service) withOrg(ctx context.Context, organizationID string, fn func(pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT set_config('app.organization_id', $1, true)`, organizationID); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
