package canonical

import (
	"context"
	"encoding/json"
	"fmt"
)

// Ontology is the active canonical ontology definition.
type Ontology struct {
	Version       int
	Entities      []string
	Relationships []string
}

var requiredEntities = []string{"supplier", "invoice", "invoice_line", "purchase_order", "po_line", "delivery_record", "delivery_line", "document"}
var requiredRelationships = []string{"invoice_po_link", "line_match", "delivery_fulfillment", "source_entity_link"}

// ActiveOntology loads and validates the single active ontology version. It
// fails closed on missing, duplicate or structurally incomplete definitions.
func (s *Service) ActiveOntology(ctx context.Context) (*Ontology, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT version, definitions FROM ontology_versions WHERE status = 'active'`)
	if err != nil {
		return nil, fmt.Errorf("load active ontology: %w", err)
	}
	defer rows.Close()

	var ontology *Ontology
	var definitions []byte
	count := 0
	for rows.Next() {
		count++
		var version int
		if err := rows.Scan(&version, &definitions); err != nil {
			return nil, err
		}
		ontology = &Ontology{Version: version}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if count != 1 {
		return nil, fmt.Errorf("expected exactly one active ontology version, found %d", count)
	}
	if ontology.Version != 1 {
		return nil, fmt.Errorf("unsupported active ontology version %d", ontology.Version)
	}
	var body struct {
		Entities      []string `json:"entities"`
		Relationships []string `json:"relationships"`
	}
	if err := json.Unmarshal(definitions, &body); err != nil {
		return nil, fmt.Errorf("decode ontology definitions: %w", err)
	}
	ontology.Entities = body.Entities
	ontology.Relationships = body.Relationships
	for _, entity := range requiredEntities {
		if !contains(ontology.Entities, entity) {
			return nil, fmt.Errorf("ontology v1 is missing entity %q", entity)
		}
	}
	for _, relationship := range requiredRelationships {
		if !contains(ontology.Relationships, relationship) {
			return nil, fmt.Errorf("ontology v1 is missing relationship %q", relationship)
		}
	}
	return ontology, nil
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// ValidateOntology is the write-path hook.
func (s *Service) ValidateOntology(ctx context.Context) error {
	_, err := s.ActiveOntology(ctx)
	return err
}
