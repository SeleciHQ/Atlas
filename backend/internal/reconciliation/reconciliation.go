// Package reconciliation implements deterministic invoice-to-PO matching.
// Results are versioned, explainable and evidence-backed; no model authority.
package reconciliation

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Result summarizes a reconciliation run.
type Result struct {
	RunID         string `json:"run_id"`
	Status        string `json:"status"`
	Matches       int    `json:"matches"`
	Discrepancies int    `json:"discrepancies"`
}

type linkedPO struct {
	ID         string
	Currency   string
	TotalMinor int64
	Allocated  *int64
}

// Service runs deterministic two-way header reconciliation.
type Service struct{ pool *pgxpool.Pool }

// NewService constructs the reconciliation service.
func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// Run reconciles an invoice against its linked purchase orders.
func (s *Service) Run(ctx context.Context, organizationID, invoiceID string) (*Result, error) {
	runID := uuid.NewString()
	result := &Result{RunID: runID, Status: "completed"}
	err := s.withOrg(ctx, organizationID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO reconciliation_runs (organization_id, id, invoice_id, status)
			VALUES ($1, $2, $3, 'running')`, organizationID, runID, invoiceID); err != nil {
			return err
		}
		var currency string
		var total int64
		if err := tx.QueryRow(ctx, `
			SELECT currency, total_minor FROM invoices WHERE organization_id = $1 AND id = $2`,
			organizationID, invoiceID).Scan(&currency, &total); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT po.id, po.currency, po.total_minor, l.allocated_minor
			FROM invoice_po_links l JOIN purchase_orders po
			  ON po.organization_id = l.organization_id AND po.id = l.purchase_order_id
			WHERE l.organization_id = $1 AND l.invoice_id = $2`, organizationID, invoiceID)
		if err != nil {
			return err
		}
		defer rows.Close()
		var orders []linkedPO
		for rows.Next() {
			var po linkedPO
			if err := rows.Scan(&po.ID, &po.Currency, &po.TotalMinor, &po.Allocated); err != nil {
				return err
			}
			orders = append(orders, po)
		}
		if err := rows.Err(); err != nil {
			return err
		}

		discrepancies := 0
		matches := 0
		addDiscrepancy := func(kind, severity, description string, evidence map[string]any) error {
			raw, _ := json.Marshal(evidence)
			discrepancies++
			_, err := tx.Exec(ctx, `
				INSERT INTO reconciliation_discrepancies (organization_id, run_id, kind, severity, description, evidence)
				VALUES ($1, $2, $3, $4, $5, $6)`, organizationID, runID, kind, severity, description, raw)
			return err
		}

		if len(orders) == 0 {
			if err := addDiscrepancy("no_linked_po", "error", "Invoice has no linked purchase order.", map[string]any{"invoice_total_minor": total}); err != nil {
				return err
			}
		} else {
			poSum := int64(0)
			for _, po := range orders {
				poSum += po.TotalMinor
				if po.Currency != currency {
					if err := addDiscrepancy("currency_mismatch", "error", "Invoice and purchase order currencies differ.",
						map[string]any{"purchase_order_id": po.ID, "invoice_currency": currency, "po_currency": po.Currency}); err != nil {
						return err
					}
				}
			}
			if poSum == total {
				for _, po := range orders {
					evidence, _ := json.Marshal(map[string]any{"invoice_total_minor": total, "po_total_minor": po.TotalMinor})
					matches++
					if _, err := tx.Exec(ctx, `
						INSERT INTO reconciliation_matches
							(organization_id, run_id, match_type, invoice_id, purchase_order_id, confidence, evidence)
						VALUES ($1, $2, 'header', $3, $4, 1.0, $5)`,
						organizationID, runID, invoiceID, po.ID, evidence); err != nil {
						return err
					}
				}
			} else {
				if err := addDiscrepancy("total_mismatch", "error", "Invoice total does not equal linked purchase order totals.",
					map[string]any{"invoice_total_minor": total, "po_total_minor": poSum, "difference_minor": total - poSum}); err != nil {
					return err
				}
			}
		}
		lineMatches, lineDiscrepancies, err := matchLines(ctx, tx, organizationID, runID, invoiceID, currency, orders)
		if err != nil {
			return err
		}
		matches += lineMatches
		discrepancies += lineDiscrepancies

		summary, _ := json.Marshal(map[string]any{"matches": matches, "discrepancies": discrepancies})
		if _, err := tx.Exec(ctx, `
			UPDATE reconciliation_runs SET status = 'completed', summary = $3, completed_at = now()
			WHERE organization_id = $1 AND id = $2`, organizationID, runID, summary); err != nil {
			return err
		}
		result.Matches = matches
		result.Discrepancies = discrepancies
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("reconcile invoice: %w", err)
	}
	return result, nil
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

func matchLines(ctx context.Context, tx pgx.Tx, organizationID, runID, invoiceID, currency string, orders []linkedPO) (int, int, error) {
	poIDs := make([]string, 0, len(orders))
	for _, order := range orders {
		poIDs = append(poIDs, order.ID)
	}
	if len(poIDs) == 0 {
		return 0, 0, nil
	}

	type invoiceLine struct {
		ID          string
		Description string
		Price       int64
		Currency    string
		Quantity    float64
	}
	type poLine struct {
		ID              string
		PurchaseOrderID string
		Description     string
		Price           int64
		Currency        string
		Quantity        float64
		Delivered       float64
		Remaining       float64
	}

	invoiceRows, err := tx.Query(ctx, `
		SELECT id, description, unit_price_minor, currency, quantity::float8
		FROM invoice_lines WHERE organization_id = $1 AND invoice_id = $2 ORDER BY line_number`,
		organizationID, invoiceID)
	if err != nil {
		return 0, 0, err
	}
	var invoiceLines []invoiceLine
	for invoiceRows.Next() {
		var line invoiceLine
		if err := invoiceRows.Scan(&line.ID, &line.Description, &line.Price, &line.Currency, &line.Quantity); err != nil {
			invoiceRows.Close()
			return 0, 0, err
		}
		invoiceLines = append(invoiceLines, line)
	}
	invoiceRows.Close()
	if err := invoiceRows.Err(); err != nil {
		return 0, 0, err
	}

	poRows, err := tx.Query(ctx, `
		SELECT pl.id, pl.purchase_order_id, pl.description, pl.unit_price_minor, pl.currency, pl.quantity::float8,
		       coalesce((SELECT sum(dl.quantity) FROM delivery_lines dl
		                 WHERE dl.organization_id = pl.organization_id AND dl.po_line_id = pl.id), 0)::float8
		FROM po_lines pl
		WHERE pl.organization_id = $1 AND pl.purchase_order_id = ANY($2)
		ORDER BY pl.purchase_order_id, pl.line_number, pl.id`, organizationID, poIDs)
	if err != nil {
		return 0, 0, err
	}
	var candidates []*poLine
	for poRows.Next() {
		line := &poLine{}
		if err := poRows.Scan(&line.ID, &line.PurchaseOrderID, &line.Description, &line.Price,
			&line.Currency, &line.Quantity, &line.Delivered); err != nil {
			poRows.Close()
			return 0, 0, err
		}
		line.Remaining = line.Quantity
		candidates = append(candidates, line)
	}
	poRows.Close()
	if err := poRows.Err(); err != nil {
		return 0, 0, err
	}

	key := func(description string, price int64, currency string) string {
		return strings.ToLower(strings.TrimSpace(description)) + "|" + strconv.FormatInt(price, 10) + "|" + currency
	}
	groups := map[string][]*poLine{}
	for _, candidate := range candidates {
		k := key(candidate.Description, candidate.Price, candidate.Currency)
		groups[k] = append(groups[k], candidate)
	}

	matches, discrepancies := 0, 0
	insertMatch := func(invoiceLineID, poLineID, purchaseOrderID string, confidence float64, allocated, invoiced, ordered, delivered float64) error {
		evidence, _ := json.Marshal(map[string]any{
			"invoice_line_id": invoiceLineID, "po_line_id": poLineID,
			"allocated_quantity": formatQuantity(allocated),
			"invoice_quantity":   formatQuantity(invoiced),
			"po_quantity":        formatQuantity(ordered),
		})
		matches++
		_, err := tx.Exec(ctx, `
			INSERT INTO reconciliation_matches
				(organization_id, run_id, match_type, invoice_id, purchase_order_id, confidence, evidence)
			VALUES ($1, $2, 'line', $3, $4, $5, $6)`,
			organizationID, runID, invoiceID, purchaseOrderID, confidence, evidence)
		return err
	}
	insertFinding := func(kind, severity, description string, evidence map[string]any) error {
		raw, _ := json.Marshal(evidence)
		discrepancies++
		_, err := tx.Exec(ctx, `
			INSERT INTO reconciliation_discrepancies (organization_id, run_id, kind, severity, description, evidence)
			VALUES ($1, $2, $3, $4, $5, $6)`, organizationID, runID, kind, severity, description, raw)
		return err
	}

	const epsilon = 0.0000005
	for _, invoiceLine := range invoiceLines {
		remaining := invoiceLine.Quantity
		allocatedAny := false
		for _, candidate := range groups[key(invoiceLine.Description, invoiceLine.Price, invoiceLine.Currency)] {
			if candidate.Remaining <= epsilon || remaining <= epsilon {
				continue
			}
			allocated := math.Min(remaining, candidate.Remaining)
			confidence := 1.0
			if allocated < invoiceLine.Quantity-epsilon {
				confidence = 0.9
			}
			if err := insertMatch(invoiceLine.ID, candidate.ID, candidate.PurchaseOrderID, confidence,
				allocated, invoiceLine.Quantity, candidate.Quantity, candidate.Delivered); err != nil {
				return 0, 0, err
			}
			allocatedAny = true
			candidate.Remaining -= allocated
			remaining -= allocated
			if candidate.Delivered+epsilon < allocated {
				if err := insertFinding("delivery_shortfall", "warning",
					"Delivered quantity is below allocated invoice quantity.", map[string]any{
						"invoice_line_id": invoiceLine.ID, "po_line_id": candidate.ID,
						"allocated_quantity": formatQuantity(allocated),
						"delivered_quantity": formatQuantity(candidate.Delivered),
					}); err != nil {
					return 0, 0, err
				}
			}
		}
		if remaining > epsilon {
			if allocatedAny {
				if err := insertFinding("invoice_line_partial", "warning",
					"Invoice line is only partially allocated to purchase order lines.", map[string]any{
						"invoice_line_id":      invoiceLine.ID,
						"invoice_quantity":     formatQuantity(invoiceLine.Quantity),
						"unallocated_quantity": formatQuantity(remaining),
					}); err != nil {
					return 0, 0, err
				}
			} else if err := insertFinding("invoice_line_unmatched", "warning",
				"Invoice line has no matching purchase order line.", map[string]any{
					"invoice_line_id": invoiceLine.ID, "description": invoiceLine.Description,
				}); err != nil {
				return 0, 0, err
			}
		}
	}
	return matches, discrepancies, nil
}

func formatQuantity(value float64) string {
	formatted := strconv.FormatFloat(value, 'f', 6, 64)
	formatted = strings.TrimRight(formatted, "0")
	formatted = strings.TrimRight(formatted, ".")
	if formatted == "" || formatted == "-" {
		return "0"
	}
	return formatted
}

// RunSummary is a list item for invoice screens.
type RunSummary struct {
	RunID     string          `json:"run_id"`
	Status    string          `json:"status"`
	Summary   json.RawMessage `json:"summary"`
	CreatedAt time.Time       `json:"created_at"`
}

// ListByInvoice returns reconciliation runs for an invoice, newest first.
func (s *Service) ListByInvoice(ctx context.Context, organizationID, invoiceID string) ([]RunSummary, error) {
	runs := []RunSummary{}
	err := s.withOrg(ctx, organizationID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, status, summary, created_at FROM reconciliation_runs
			WHERE organization_id = $1 AND invoice_id = $2 ORDER BY created_at DESC`,
			organizationID, invoiceID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var run RunSummary
			if err := rows.Scan(&run.RunID, &run.Status, &run.Summary, &run.CreatedAt); err != nil {
				return err
			}
			runs = append(runs, run)
		}
		return rows.Err()
	})
	return runs, err
}

// Match is one reconciliation match.
type Match struct {
	ID              string          `json:"id"`
	MatchType       string          `json:"match_type"`
	InvoiceID       string          `json:"invoice_id"`
	PurchaseOrderID string          `json:"purchase_order_id"`
	Confidence      string          `json:"confidence"`
	Evidence        json.RawMessage `json:"evidence"`
}

// Discrepancy is one reconciliation finding.
type Discrepancy struct {
	ID          string          `json:"id"`
	Kind        string          `json:"kind"`
	Severity    string          `json:"severity"`
	Description string          `json:"description"`
	Evidence    json.RawMessage `json:"evidence"`
}

// RunDetail is the reconciliation read model.
type RunDetail struct {
	RunID         string          `json:"run_id"`
	InvoiceID     string          `json:"invoice_id"`
	Status        string          `json:"status"`
	EngineVersion int             `json:"engine_version"`
	Summary       json.RawMessage `json:"summary"`
	Matches       []Match         `json:"matches"`
	Discrepancies []Discrepancy   `json:"discrepancies"`
}

// GetRun returns a run with its matches and discrepancies.
func (s *Service) GetRun(ctx context.Context, organizationID, runID string) (*RunDetail, error) {
	detail := &RunDetail{Matches: []Match{}, Discrepancies: []Discrepancy{}}
	err := s.withOrg(ctx, organizationID, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			SELECT id, invoice_id, status, engine_version, summary
			FROM reconciliation_runs WHERE organization_id = $1 AND id = $2`,
			organizationID, runID).Scan(&detail.RunID, &detail.InvoiceID, &detail.Status,
			&detail.EngineVersion, &detail.Summary); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT id, match_type, invoice_id, purchase_order_id, confidence::text, evidence
			FROM reconciliation_matches WHERE organization_id = $1 AND run_id = $2 ORDER BY created_at`,
			organizationID, runID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var match Match
			if err := rows.Scan(&match.ID, &match.MatchType, &match.InvoiceID, &match.PurchaseOrderID,
				&match.Confidence, &match.Evidence); err != nil {
				return err
			}
			detail.Matches = append(detail.Matches, match)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		findings, err := tx.Query(ctx, `
			SELECT id, kind, severity, description, evidence
			FROM reconciliation_discrepancies WHERE organization_id = $1 AND run_id = $2 ORDER BY created_at`,
			organizationID, runID)
		if err != nil {
			return err
		}
		defer findings.Close()
		for findings.Next() {
			var discrepancy Discrepancy
			if err := findings.Scan(&discrepancy.ID, &discrepancy.Kind, &discrepancy.Severity,
				&discrepancy.Description, &discrepancy.Evidence); err != nil {
				return err
			}
			detail.Discrepancies = append(detail.Discrepancies, discrepancy)
		}
		return findings.Err()
	})
	if err != nil {
		return nil, err
	}
	return detail, nil
}
