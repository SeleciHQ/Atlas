// Package reconciliation implements deterministic invoice-to-PO matching.
// Results are versioned, explainable and evidence-backed; no model authority.
package reconciliation

import (
	"context"
	"encoding/json"
	"fmt"

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
	exact := `
		pl.organization_id = il.organization_id
		AND lower(pl.description) = lower(il.description)
		AND pl.unit_price_minor = il.unit_price_minor
		AND pl.currency = il.currency
		AND pl.quantity = il.quantity`

	matched, err := tx.Exec(ctx, `
		INSERT INTO reconciliation_matches
			(organization_id, run_id, match_type, invoice_id, purchase_order_id, confidence, evidence)
		SELECT $1, $2, 'line', $3, pl.purchase_order_id, 1.0,
		       jsonb_build_object('invoice_line_id', il.id, 'po_line_id', pl.id)
		FROM invoice_lines il
		JOIN po_lines pl ON `+exact+`
		WHERE il.organization_id = $1 AND il.invoice_id = $3 AND pl.purchase_order_id = ANY($4)`,
		organizationID, runID, invoiceID, poIDs)
	if err != nil {
		return 0, 0, err
	}
	unmatched, err := tx.Exec(ctx, `
		INSERT INTO reconciliation_discrepancies (organization_id, run_id, kind, severity, description, evidence)
		SELECT $1, $2, 'invoice_line_unmatched', 'warning', 'Invoice line has no exact purchase order line.',
		       jsonb_build_object('invoice_line_id', il.id, 'description', il.description)
		FROM invoice_lines il
		WHERE il.organization_id = $1 AND il.invoice_id = $3
		  AND NOT EXISTS (SELECT 1 FROM po_lines pl WHERE `+exact+` AND pl.purchase_order_id = ANY($4))`,
		organizationID, runID, invoiceID, poIDs)
	if err != nil {
		return 0, 0, err
	}
	shortfall, err := tx.Exec(ctx, `
		INSERT INTO reconciliation_discrepancies (organization_id, run_id, kind, severity, description, evidence)
		SELECT $1, $2, 'delivery_shortfall', 'warning', 'Delivered quantity is below invoiced quantity.',
		       jsonb_build_object('invoice_line_id', il.id, 'po_line_id', pl.id,
		                          'invoice_quantity', il.quantity::text, 'delivered_quantity', coalesce(d.delivered, 0)::text)
		FROM invoice_lines il
		JOIN po_lines pl ON `+exact+`
		LEFT JOIN (SELECT po_line_id, sum(quantity) AS delivered FROM delivery_lines
		           WHERE organization_id = $1 GROUP BY po_line_id) d ON d.po_line_id = pl.id
		WHERE il.organization_id = $1 AND il.invoice_id = $3 AND pl.purchase_order_id = ANY($4)
		  AND coalesce(d.delivered, 0) < il.quantity`,
		organizationID, runID, invoiceID, poIDs)
	if err != nil {
		return 0, 0, err
	}
	return int(matched.RowsAffected()), int(unmatched.RowsAffected() + shortfall.RowsAffected()), nil
}
