package canonical

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ProjectNormalized creates (or reuses) a canonical entity from a normalized
// record and records evidence-backed provenance. Idempotent per source record,
// entity type and projection method.
func (s *Service) ProjectNormalized(ctx context.Context, organizationID, normalizedRecordID string) (string, error) {
	var sourceRecordID, recordType, mappingVersion string
	var payload []byte
	if err := s.withOrg(ctx, organizationID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT source_record_id, record_type, mapping_version, payload
			FROM normalized_records WHERE organization_id = $1 AND id = $2`,
			organizationID, normalizedRecordID).
			Scan(&sourceRecordID, &recordType, &mappingVersion, &payload)
	}); err != nil {
		return "", fmt.Errorf("load normalized record: %w", err)
	}

	method := "projection:" + mappingVersion
	var existing string
	if err := s.withOrg(ctx, organizationID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT entity_id FROM source_entity_links
			WHERE organization_id = $1 AND source_record_id = $2 AND method = $3 LIMIT 1`,
			organizationID, sourceRecordID, method).Scan(&existing)
	}); err == nil && existing != "" {
		return existing, nil
	}

	var fields map[string]any
	if err := json.Unmarshal(payload, &fields); err != nil {
		return "", fmt.Errorf("decode normalized payload: %w", err)
	}
	text := func(key string) string { value, _ := fields[key].(string); return value }
	number := func(key string) int64 {
		switch value := fields[key].(type) {
		case float64:
			return int64(value)
		case int:
			return int64(value)
		case json.Number:
			parsed, _ := value.Int64()
			return parsed
		}
		return 0
	}

	var entityID string
	switch recordType {
	case "supplier.v1":
		supplier, err := s.CreateSupplier(ctx, organizationID, text("name"), text("currency"))
		if err != nil {
			return "", err
		}
		entityID = supplier.ID
	case "purchase_order.v1", "invoice.v1":
		supplierID, err := s.ensureSupplier(ctx, organizationID, text("supplier_name"), text("currency"))
		if err != nil {
			return "", err
		}
		if recordType == "purchase_order.v1" {
			order, err := s.CreatePurchaseOrder(ctx, organizationID, supplierID, text("number"), text("currency"), number("total_minor"))
			if err != nil {
				return "", err
			}
			entityID = order.ID
		} else {
			invoice, err := s.CreateInvoice(ctx, organizationID, supplierID, text("number"), text("currency"), number("total_minor"))
			if err != nil {
				return "", err
			}
			entityID = invoice.ID
		}
	default:
		return "", fmt.Errorf("no canonical projection for record type %q", recordType)
	}

	entityType := "supplier"
	if recordType == "purchase_order.v1" {
		entityType = "purchase_order"
	} else if recordType == "invoice.v1" {
		entityType = "invoice"
	}
	if err := s.LinkSource(ctx, organizationID, sourceRecordID, entityType, entityID, method, 1.0,
		map[string]any{"normalized_record_id": normalizedRecordID, "mapping_version": mappingVersion}); err != nil {
		return "", err
	}
	return entityID, nil
}

func (s *Service) ensureSupplier(ctx context.Context, organizationID, name, currency string) (string, error) {
	var id string
	err := s.withOrg(ctx, organizationID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT id FROM suppliers WHERE organization_id = $1 AND name = $2`,
			organizationID, name).Scan(&id)
	})
	if err == nil {
		return id, nil
	}
	supplier, err := s.CreateSupplier(ctx, organizationID, name, currency)
	if err != nil {
		return "", err
	}
	return supplier.ID, nil
}
