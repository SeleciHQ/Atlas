// Package extraction defines the extraction result contract and the Go
// validation gate between model output and canonical candidates. Extraction
// results are drafts; they are never written to canonical tables here.
package extraction

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

// Result is a provider extraction result (schema_version 1).
type Result struct {
	SchemaVersion int      `json:"schema_version"`
	Provider      string   `json:"provider"`
	SupplierName  string   `json:"supplier_name"`
	SupplierEmail string   `json:"supplier_email"`
	SupplierVATID string   `json:"supplier_vat_id"`
	InvoiceNumber string   `json:"invoice_number"`
	Currency      string   `json:"currency"`
	TotalMinor    int64    `json:"total_minor"`
	Confidence    float64  `json:"confidence"`
	Lines         []Line   `json:"lines"`
	Warnings      []string `json:"warnings"`
}

// Line is one extracted invoice line.
type Line struct {
	Description    string  `json:"description"`
	Quantity       string  `json:"quantity"`
	UnitPriceMinor int64   `json:"unit_price_minor"`
	Currency       string  `json:"currency"`
	Confidence     float64 `json:"confidence"`
}

// Reason is a structured validation failure.
type Reason struct {
	Field  string `json:"field"`
	Detail string `json:"detail"`
}

// Validate applies schema, type, money/quantity and confidence rules. Results
// below the confidence threshold are rejected with structured reasons.
func Validate(result Result, threshold float64) []Reason {
	var reasons []Reason
	if result.SchemaVersion <= 0 {
		reasons = append(reasons, Reason{"schema_version", "must be positive"})
	}
	if result.Provider == "" {
		reasons = append(reasons, Reason{"provider", "required"})
	}
	if result.SupplierName == "" {
		reasons = append(reasons, Reason{"supplier_name", "required"})
	}
	if result.InvoiceNumber == "" {
		reasons = append(reasons, Reason{"invoice_number", "required"})
	}
	if !currencyPattern.MatchString(result.Currency) {
		reasons = append(reasons, Reason{"currency", "must be an ISO-4217 uppercase code"})
	}
	if result.TotalMinor < 0 {
		reasons = append(reasons, Reason{"total_minor", "must be non-negative"})
	}
	if result.Confidence < 0 || result.Confidence > 1 {
		reasons = append(reasons, Reason{"confidence", "must be between 0 and 1"})
	} else if result.Confidence < threshold {
		reasons = append(reasons, Reason{"confidence", "below required threshold"})
	}
	for index, line := range result.Lines {
		field := fmt.Sprintf("lines[%d]", index)
		if line.Quantity == "" {
			reasons = append(reasons, Reason{field + ".quantity", "required"})
		}
		if line.UnitPriceMinor < 0 {
			reasons = append(reasons, Reason{field + ".unit_price_minor", "must be non-negative"})
		}
		if !currencyPattern.MatchString(line.Currency) {
			reasons = append(reasons, Reason{field + ".currency", "must be an ISO-4217 uppercase code"})
		}
		if line.Confidence < threshold {
			reasons = append(reasons, Reason{field + ".confidence", "below required threshold"})
		}
	}
	return reasons
}

// Draft is a persisted extraction draft.
type Draft struct {
	ID      string   `json:"id"`
	Status  string   `json:"status"`
	Reasons []Reason `json:"reasons"`
}

// Service persists validated/rejected extraction drafts.
type Service struct {
	pool      *pgxpool.Pool
	threshold float64
}

// NewService constructs the extraction service with a confidence threshold.
func NewService(pool *pgxpool.Pool, threshold float64) *Service {
	return &Service{pool: pool, threshold: threshold}
}

// Persist validates a result and records a draft. Rejected drafts keep the
// reasons; neither path writes canonical rows.
func (s *Service) Persist(ctx context.Context, organizationID, documentID string, result Result) (*Draft, error) {
	reasons := Validate(result, s.threshold)
	status := "validated"
	if len(reasons) > 0 {
		status = "rejected"
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	warnings, _ := json.Marshal(result.Warnings)
	reasonJSON, _ := json.Marshal(reasons)
	var id string
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT set_config('app.organization_id', $1, true)`, organizationID); err != nil {
		return nil, err
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO extraction_drafts
			(organization_id, document_id, provider, schema_version, status, confidence, payload, warnings, reasons)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`,
		organizationID, documentID, result.Provider, result.SchemaVersion, status,
		result.Confidence, payload, warnings, reasonJSON).Scan(&id); err != nil {
		return nil, fmt.Errorf("persist extraction draft: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &Draft{ID: id, Status: status, Reasons: reasons}, nil
}

// MockProvider returns a deterministic extraction result for a document.
func MockProvider(documentID string) Result {
	return Result{
		SchemaVersion: 1, Provider: "mock", SupplierName: "Mock Supplier",
		InvoiceNumber: "INV-MOCK-1", Currency: "EUR", TotalMinor: 12000, Confidence: 0.95,
		Lines: []Line{{Description: "Mock line", Quantity: "1.000000", UnitPriceMinor: 12000, Currency: "EUR", Confidence: 0.95}},
	}
}

// CountDrafts returns the number of drafts for a document (tests/visibility).
func (s *Service) CountDrafts(ctx context.Context, organizationID, documentID string) (int, error) {
	var count int
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT set_config('app.organization_id', $1, true)`, organizationID); err != nil {
		return 0, err
	}
	err = tx.QueryRow(ctx, `SELECT count(*) FROM extraction_drafts WHERE organization_id = $1 AND document_id = $2`,
		organizationID, documentID).Scan(&count)
	if err != nil && err != pgx.ErrNoRows {
		return 0, err
	}
	return count, nil
}
