// Package query is the shared read layer used by REST and the MCP server. It
// returns organization-scoped projections over canonical and reconciliation
// data; it never mutates state. Handlers must not reimplement filtering.
package query

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"atlas/internal/canonical"
	"atlas/internal/platform/apperr"
	"atlas/internal/reconciliation"
	"atlas/internal/tenancy"
)

var currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

// Service provides read-only projections.
type Service struct {
	pool           *pgxpool.Pool
	canonical      *canonical.Service
	reconciliation *reconciliation.Service
}

// NewService constructs the query service.
func NewService(pool *pgxpool.Pool, canonicalService *canonical.Service, reconciliationService *reconciliation.Service) *Service {
	return &Service{pool: pool, canonical: canonicalService, reconciliation: reconciliationService}
}

// InvoiceFilters selects invoices.
type InvoiceFilters struct {
	SupplierID    string
	Number        string
	Currency      string
	MinTotalMinor *int64
	MaxTotalMinor *int64
	Limit         int
}

// InvoiceSummary is one invoice row.
type InvoiceSummary struct {
	ID              string    `json:"id"`
	SupplierID      string    `json:"supplier_id"`
	SupplierName    string    `json:"supplier_name"`
	Number          string    `json:"number"`
	Currency        string    `json:"currency"`
	TotalMinor      int64     `json:"total_minor"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
	LatestRunID     *string   `json:"latest_reconciliation_run_id,omitempty"`
	LatestRunStatus *string   `json:"latest_reconciliation_status,omitempty"`
}

// SearchInvoices returns invoices newest first.
func (s *Service) SearchInvoices(ctx context.Context, organizationID string, filters InvoiceFilters) ([]InvoiceSummary, error) {
	if err := validateUUIDFilter(filters.SupplierID, "supplier_id"); err != nil {
		return nil, err
	}
	if err := validateCurrencyFilter(filters.Currency); err != nil {
		return nil, err
	}
	limit := clampLimit(filters.Limit)
	var invoices []InvoiceSummary
	err := tenancy.WithOrganizationTx(ctx, s.pool, organizationID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT i.id, i.supplier_id, s.name, i.number, i.currency, i.total_minor, i.status, i.created_at,
			       r.id, r.status
			FROM invoices i
			JOIN suppliers s ON s.organization_id = i.organization_id AND s.id = i.supplier_id
			LEFT JOIN LATERAL (
				SELECT id, status FROM reconciliation_runs
				WHERE organization_id = i.organization_id AND invoice_id = i.id
				ORDER BY created_at DESC LIMIT 1
			) r ON true
			WHERE i.organization_id = $1
			  AND ($2 = '' OR i.supplier_id = $2::uuid)
			  AND ($3 = '' OR i.number ILIKE $3 ESCAPE '\')
			  AND ($4 = '' OR i.currency = $4)
			  AND ($5::bigint IS NULL OR i.total_minor >= $5)
			  AND ($6::bigint IS NULL OR i.total_minor <= $6)
			ORDER BY i.created_at DESC, i.id DESC
			LIMIT $7`, organizationID, filters.SupplierID, likePattern(filters.Number),
			filters.Currency, filters.MinTotalMinor, filters.MaxTotalMinor, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var invoice InvoiceSummary
			if err := rows.Scan(&invoice.ID, &invoice.SupplierID, &invoice.SupplierName, &invoice.Number,
				&invoice.Currency, &invoice.TotalMinor, &invoice.Status, &invoice.CreatedAt,
				&invoice.LatestRunID, &invoice.LatestRunStatus); err != nil {
				return err
			}
			invoices = append(invoices, invoice)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return invoices, nil
}

// PurchaseOrderFilters selects purchase orders.
type PurchaseOrderFilters struct {
	SupplierID    string
	Number        string
	Currency      string
	MinTotalMinor *int64
	MaxTotalMinor *int64
	Limit         int
}

// PurchaseOrderSummary is one purchase order row.
type PurchaseOrderSummary struct {
	ID           string    `json:"id"`
	SupplierID   string    `json:"supplier_id"`
	SupplierName string    `json:"supplier_name"`
	Number       string    `json:"number"`
	Currency     string    `json:"currency"`
	TotalMinor   int64     `json:"total_minor"`
	OrderedOn    *string   `json:"ordered_on,omitempty"`
	LineCount    int       `json:"line_count"`
	CreatedAt    time.Time `json:"created_at"`
}

// SearchPurchaseOrders returns purchase orders newest first.
func (s *Service) SearchPurchaseOrders(ctx context.Context, organizationID string, filters PurchaseOrderFilters) ([]PurchaseOrderSummary, error) {
	if err := validateUUIDFilter(filters.SupplierID, "supplier_id"); err != nil {
		return nil, err
	}
	if err := validateCurrencyFilter(filters.Currency); err != nil {
		return nil, err
	}
	limit := clampLimit(filters.Limit)
	var orders []PurchaseOrderSummary
	err := tenancy.WithOrganizationTx(ctx, s.pool, organizationID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT o.id, o.supplier_id, s.name, o.number, o.currency, o.total_minor, o.ordered_on::text,
			       (SELECT count(*) FROM po_lines l
			        WHERE l.organization_id = o.organization_id AND l.purchase_order_id = o.id),
			       o.created_at
			FROM purchase_orders o
			JOIN suppliers s ON s.organization_id = o.organization_id AND s.id = o.supplier_id
			WHERE o.organization_id = $1
			  AND ($2 = '' OR o.supplier_id = $2::uuid)
			  AND ($3 = '' OR o.number ILIKE $3 ESCAPE '\')
			  AND ($4 = '' OR o.currency = $4)
			  AND ($5::bigint IS NULL OR o.total_minor >= $5)
			  AND ($6::bigint IS NULL OR o.total_minor <= $6)
			ORDER BY o.created_at DESC, o.id DESC
			LIMIT $7`, organizationID, filters.SupplierID, likePattern(filters.Number),
			filters.Currency, filters.MinTotalMinor, filters.MaxTotalMinor, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var order PurchaseOrderSummary
			if err := rows.Scan(&order.ID, &order.SupplierID, &order.SupplierName, &order.Number,
				&order.Currency, &order.TotalMinor, &order.OrderedOn, &order.LineCount, &order.CreatedAt); err != nil {
				return err
			}
			orders = append(orders, order)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return orders, nil
}

// SupplierSummary is one supplier row.
type SupplierSummary struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Currency     string  `json:"currency"`
	Email        *string `json:"email,omitempty"`
	VATID        *string `json:"vat_id,omitempty"`
	InvoiceCount int     `json:"invoice_count"`
	OrderCount   int     `json:"order_count"`
}

// SearchSuppliers returns suppliers matching an optional name/email/VAT query.
func (s *Service) SearchSuppliers(ctx context.Context, organizationID, text string, limit int) ([]SupplierSummary, error) {
	limit = clampLimit(limit)
	pattern := likePattern(text)
	var suppliers []SupplierSummary
	err := tenancy.WithOrganizationTx(ctx, s.pool, organizationID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, name, currency, email, vat_id,
			       (SELECT count(*) FROM invoices i
			        WHERE i.organization_id = suppliers.organization_id AND i.supplier_id = suppliers.id),
			       (SELECT count(*) FROM purchase_orders o
			        WHERE o.organization_id = suppliers.organization_id AND o.supplier_id = suppliers.id)
			FROM suppliers
			WHERE organization_id = $1
			  AND ($2 = '' OR name ILIKE $2 ESCAPE '\' OR email ILIKE $2 ESCAPE '\' OR vat_id ILIKE $2 ESCAPE '\')
			ORDER BY name ASC
			LIMIT $3`, organizationID, pattern, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var supplier SupplierSummary
			if err := rows.Scan(&supplier.ID, &supplier.Name, &supplier.Currency, &supplier.Email,
				&supplier.VATID, &supplier.InvoiceCount, &supplier.OrderCount); err != nil {
				return err
			}
			suppliers = append(suppliers, supplier)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return suppliers, nil
}

// DeliverySummary is one delivery record row.
type DeliverySummary struct {
	ID              string    `json:"id"`
	SupplierID      string    `json:"supplier_id"`
	PurchaseOrderID *string   `json:"purchase_order_id,omitempty"`
	Number          string    `json:"number"`
	DeliveredOn     *string   `json:"delivered_on,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// ListDeliveries returns delivery records for an invoice and/or purchase order.
func (s *Service) ListDeliveries(ctx context.Context, organizationID, invoiceID, purchaseOrderID string, limit int) ([]DeliverySummary, error) {
	if invoiceID == "" && purchaseOrderID == "" {
		return nil, apperr.New(apperr.KindInvalid, "invalid_request", "invoice_id or purchase_order_id is required.")
	}
	if err := validateUUIDFilter(invoiceID, "invoice_id"); err != nil {
		return nil, err
	}
	if err := validateUUIDFilter(purchaseOrderID, "purchase_order_id"); err != nil {
		return nil, err
	}
	limit = clampLimit(limit)
	var deliveries []DeliverySummary
	err := tenancy.WithOrganizationTx(ctx, s.pool, organizationID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT d.id, d.supplier_id, d.purchase_order_id, d.number, d.delivered_on::text, d.created_at
			FROM delivery_records d
			WHERE d.organization_id = $1
			  AND ($2 = '' OR d.purchase_order_id IN (
			        SELECT purchase_order_id FROM invoice_po_links
			        WHERE organization_id = $1 AND invoice_id = $2::uuid))
			  AND ($3 = '' OR d.purchase_order_id = $3::uuid)
			ORDER BY d.created_at DESC, d.id DESC
			LIMIT $4`, organizationID, invoiceID, purchaseOrderID, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var delivery DeliverySummary
			if err := rows.Scan(&delivery.ID, &delivery.SupplierID, &delivery.PurchaseOrderID,
				&delivery.Number, &delivery.DeliveredOn, &delivery.CreatedAt); err != nil {
				return err
			}
			deliveries = append(deliveries, delivery)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return deliveries, nil
}

// DiscrepancyFilters selects reconciliation findings.
type DiscrepancyFilters struct {
	Kind       string
	Severity   string
	InvoiceID  string
	RunID      string
	SupplierID string
	Limit      int
}

// DiscrepancySummary is one reconciliation finding row.
type DiscrepancySummary struct {
	ID          string          `json:"id"`
	RunID       string          `json:"run_id"`
	InvoiceID   string          `json:"invoice_id"`
	Kind        string          `json:"kind"`
	Severity    string          `json:"severity"`
	Description string          `json:"description"`
	Evidence    json.RawMessage `json:"evidence"`
	CreatedAt   time.Time       `json:"created_at"`
}

// SearchDiscrepancies returns findings newest first.
func (s *Service) SearchDiscrepancies(ctx context.Context, organizationID string, filters DiscrepancyFilters) ([]DiscrepancySummary, error) {
	if filters.Severity != "" && filters.Severity != "info" && filters.Severity != "warning" && filters.Severity != "error" {
		return nil, apperr.New(apperr.KindInvalid, "invalid_request", "severity must be info, warning or error.")
	}
	for name, value := range map[string]string{
		"invoice_id": filters.InvoiceID, "run_id": filters.RunID, "supplier_id": filters.SupplierID,
	} {
		if err := validateUUIDFilter(value, name); err != nil {
			return nil, err
		}
	}
	limit := clampLimit(filters.Limit)
	var findings []DiscrepancySummary
	err := tenancy.WithOrganizationTx(ctx, s.pool, organizationID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT d.id, d.run_id, r.invoice_id, d.kind, d.severity, d.description, d.evidence, d.created_at
			FROM reconciliation_discrepancies d
			JOIN reconciliation_runs r ON r.organization_id = d.organization_id AND r.id = d.run_id
			JOIN invoices i ON i.organization_id = r.organization_id AND i.id = r.invoice_id
			WHERE d.organization_id = $1
			  AND ($2 = '' OR d.kind = $2)
			  AND ($3 = '' OR d.severity = $3)
			  AND ($4 = '' OR r.invoice_id = $4::uuid)
			  AND ($5 = '' OR d.run_id = $5::uuid)
			  AND ($6 = '' OR i.supplier_id = $6::uuid)
			ORDER BY d.created_at DESC, d.id DESC
			LIMIT $7`, organizationID, filters.Kind, filters.Severity, filters.InvoiceID,
			filters.RunID, filters.SupplierID, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var finding DiscrepancySummary
			if err := rows.Scan(&finding.ID, &finding.RunID, &finding.InvoiceID, &finding.Kind,
				&finding.Severity, &finding.Description, &finding.Evidence, &finding.CreatedAt); err != nil {
				return err
			}
			findings = append(findings, finding)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return findings, nil
}

// GetInvoice returns the canonical invoice projection.
func (s *Service) GetInvoice(ctx context.Context, organizationID, invoiceID string) (*canonical.InvoiceDetail, error) {
	return s.canonical.GetInvoiceDetail(ctx, organizationID, invoiceID)
}

// GetPurchaseOrder returns the canonical purchase-order projection.
func (s *Service) GetPurchaseOrder(ctx context.Context, organizationID, purchaseOrderID string) (*canonical.PurchaseOrderDetail, error) {
	return s.canonical.GetPurchaseOrderDetail(ctx, organizationID, purchaseOrderID)
}

// GetSupplier returns the supplier projection.
func (s *Service) GetSupplier(ctx context.Context, organizationID, supplierID string) (*canonical.SupplierDetail, error) {
	return s.canonical.GetSupplierDetail(ctx, organizationID, supplierID)
}

// GetReconciliation returns a reconciliation run with evidence.
func (s *Service) GetReconciliation(ctx context.Context, organizationID, runID string) (*reconciliation.RunDetail, error) {
	return s.reconciliation.GetRun(ctx, organizationID, runID)
}

// ListReconciliationRuns returns runs for an invoice, newest first.
func (s *Service) ListReconciliationRuns(ctx context.Context, organizationID, invoiceID string) ([]reconciliation.RunSummary, error) {
	return s.reconciliation.ListByInvoice(ctx, organizationID, invoiceID)
}

func clampLimit(limit int) int {
	if limit <= 0 {
		return 25
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func validateUUIDFilter(value, field string) error {
	if value == "" {
		return nil
	}
	if _, err := uuid.Parse(value); err != nil {
		return apperr.New(apperr.KindInvalid, "invalid_request", "Invalid filter value.").WithField(field, "must be a UUID")
	}
	return nil
}

func validateCurrencyFilter(currency string) error {
	if currency == "" {
		return nil
	}
	if !currencyPattern.MatchString(currency) {
		return apperr.New(apperr.KindInvalid, "invalid_request", "Invalid filter value.").WithField("currency", "must be an ISO-4217 uppercase code")
	}
	return nil
}

func likePattern(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	value = strings.ReplaceAll(value, `_`, `\_`)
	return "%" + value + "%"
}
