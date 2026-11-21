package canonical

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Delivery is a canonical delivery (goods-received) record.
type Delivery struct {
	ID              string  `json:"id"`
	SupplierID      string  `json:"supplier_id"`
	PurchaseOrderID *string `json:"purchase_order_id"`
	Number          string  `json:"number"`
	DeliveredOn     *string `json:"delivered_on"`
}

// POLineView is a PO line with the quantity fulfilled across deliveries.
type POLineView struct {
	Line      Line   `json:"line"`
	Fulfilled string `json:"fulfilled_quantity"`
}

// PurchaseOrderDetail is the PO read model.
type PurchaseOrderDetail struct {
	Order PurchaseOrder `json:"order"`
	Lines []POLineView  `json:"lines"`
}

// SupplierDetail is the supplier read model.
type SupplierDetail struct {
	Supplier     Supplier `json:"supplier"`
	InvoiceCount int      `json:"invoice_count"`
	OrderCount   int      `json:"order_count"`
}

// AddDelivery inserts a delivery record, optionally against a PO.
func (s *Service) AddDelivery(ctx context.Context, organizationID, supplierID, purchaseOrderID, number, deliveredOn string) (*Delivery, error) {
	if number == "" {
		return nil, fmt.Errorf("delivery number is required")
	}
	if err := s.ensureOntology(ctx); err != nil {
		return nil, err
	}
	var poID any
	if purchaseOrderID != "" {
		poID = purchaseOrderID
	}
	var delivered any
	if deliveredOn != "" {
		delivered = deliveredOn
	}
	delivery := &Delivery{SupplierID: supplierID, Number: number}
	if purchaseOrderID != "" {
		delivery.PurchaseOrderID = &purchaseOrderID
	}
	if deliveredOn != "" {
		delivery.DeliveredOn = &deliveredOn
	}
	err := s.withOrg(ctx, organizationID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			INSERT INTO delivery_records (organization_id, supplier_id, purchase_order_id, number, delivered_on)
			VALUES ($1, $2, $3, $4, $5::date) RETURNING id`,
			organizationID, supplierID, poID, number, delivered).Scan(&delivery.ID)
	})
	return delivery, err
}

// AddDeliveryLine records received quantity against a PO line. Multiple
// deliveries accumulate against the same line (partial deliveries).
func (s *Service) AddDeliveryLine(ctx context.Context, organizationID, deliveryRecordID, poLineID, quantity string) error {
	if err := s.ensureOntology(ctx); err != nil {
		return err
	}
	return s.withOrg(ctx, organizationID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO delivery_lines (organization_id, delivery_record_id, po_line_id, quantity)
			VALUES ($1, $2, $3, $4::numeric)`, organizationID, deliveryRecordID, poLineID, quantity)
		return err
	})
}

// GetPurchaseOrderDetail returns the PO, lines and fulfilled quantities.
func (s *Service) GetPurchaseOrderDetail(ctx context.Context, organizationID, purchaseOrderID string) (*PurchaseOrderDetail, error) {
	detail := &PurchaseOrderDetail{Lines: []POLineView{}}
	err := s.withOrg(ctx, organizationID, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			SELECT id, supplier_id, number, currency, total_minor FROM purchase_orders
			WHERE organization_id = $1 AND id = $2`, organizationID, purchaseOrderID).
			Scan(&detail.Order.ID, &detail.Order.SupplierID, &detail.Order.Number,
				&detail.Order.Currency, &detail.Order.TotalMinor); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT l.id, l.line_number, l.description, l.quantity::text, l.unit, l.unit_price_minor, l.currency,
			       coalesce((SELECT sum(dl.quantity) FROM delivery_lines dl
			                 WHERE dl.organization_id = l.organization_id AND dl.po_line_id = l.id), 0)::text
			FROM po_lines l
			WHERE l.organization_id = $1 AND l.purchase_order_id = $2
			ORDER BY l.line_number`, organizationID, purchaseOrderID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var view POLineView
			if err := rows.Scan(&view.Line.ID, &view.Line.LineNumber, &view.Line.Description,
				&view.Line.Quantity, &view.Line.Unit, &view.Line.UnitPriceMinor, &view.Line.Currency,
				&view.Fulfilled); err != nil {
				return err
			}
			detail.Lines = append(detail.Lines, view)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return detail, nil
}

// GetSupplierDetail returns the supplier with invoice/order counts.
func (s *Service) GetSupplierDetail(ctx context.Context, organizationID, supplierID string) (*SupplierDetail, error) {
	detail := &SupplierDetail{}
	err := s.withOrg(ctx, organizationID, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			SELECT id, name, currency FROM suppliers WHERE organization_id = $1 AND id = $2`,
			organizationID, supplierID).Scan(&detail.Supplier.ID, &detail.Supplier.Name, &detail.Supplier.Currency); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `
			SELECT (SELECT count(*) FROM invoices WHERE organization_id = $1 AND supplier_id = $2),
			       (SELECT count(*) FROM purchase_orders WHERE organization_id = $1 AND supplier_id = $2)`,
			organizationID, supplierID).Scan(&detail.InvoiceCount, &detail.OrderCount); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return detail, nil
}
