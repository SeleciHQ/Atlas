package ingestion

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"atlas/internal/audit"
	"atlas/internal/tenancy"
	"github.com/jackc/pgx/v5"
)

// Integration is integration metadata for management endpoints.
type Integration struct {
	ID           string          `json:"id"`
	Provider     string          `json:"provider"`
	Kind         string          `json:"kind"`
	Name         string          `json:"name"`
	Status       string          `json:"status"`
	Config       json.RawMessage `json:"config"`
	LastSyncedAt *time.Time      `json:"last_synced_at"`
}

// GetIntegration returns one integration.
func (s *Service) GetIntegration(ctx context.Context, organizationID, integrationID string) (*Integration, error) {
	var integration Integration
	var config []byte
	var lastSynced *time.Time
	if err := tenancy.WithOrganizationTx(ctx, s.pool, organizationID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT id, provider, kind, name, status, config, last_synced_at
			FROM integrations WHERE organization_id = $1 AND id = $2`, organizationID, integrationID).
			Scan(&integration.ID, &integration.Provider, &integration.Kind, &integration.Name,
				&integration.Status, &config, &lastSynced)
	}); err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("integration not found")
		}
		return nil, err
	}
	if len(config) > 0 {
		integration.Config = config
	}
	integration.LastSyncedAt = lastSynced
	return &integration, nil
}

// ListIntegrations returns all integrations for the organization.
func (s *Service) ListIntegrations(ctx context.Context, organizationID string) ([]Integration, error) {
	var integrations []Integration
	err := tenancy.WithOrganizationTx(ctx, s.pool, organizationID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, provider, kind, name, status, config, last_synced_at
			FROM integrations WHERE organization_id = $1 ORDER BY created_at ASC`, organizationID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var integration Integration
			var config []byte
			if err := rows.Scan(&integration.ID, &integration.Provider, &integration.Kind,
				&integration.Name, &integration.Status, &config, &integration.LastSyncedAt); err != nil {
				return err
			}
			integration.Config = config
			integrations = append(integrations, integration)
		}
		return rows.Err()
	})
	return integrations, err
}

// WebhookSecret reads the integration's webhook signing secret from config.
func (s *Service) WebhookSecret(ctx context.Context, organizationID, integrationID string) (string, error) {
	var secret string
	err := tenancy.WithOrganizationTx(ctx, s.pool, organizationID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT coalesce(config->>'webhook_secret', '') FROM integrations
			WHERE organization_id = $1 AND id = $2`, organizationID, integrationID).Scan(&secret)
	})
	if err != nil {
		return "", fmt.Errorf("load webhook secret: %w", err)
	}
	return secret, nil
}

// RecordWebhook persists a verified webhook event and reports whether it is new
// (dedupe by external event id).
func (s *Service) RecordWebhook(ctx context.Context, organizationID, integrationID, eventID, eventType string, payload []byte) (bool, error) {
	created := false
	err := tenancy.WithOrganizationTx(ctx, s.pool, organizationID, func(tx pgx.Tx) error {
		var id string
		err := tx.QueryRow(ctx, `
			INSERT INTO webhook_events (organization_id, integration_id, external_event_id, event_type, raw_payload)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (organization_id, integration_id, external_event_id) DO NOTHING
			RETURNING id`, organizationID, integrationID, eventID, eventType, payload).Scan(&id)
		if err == pgx.ErrNoRows {
			return nil
		}
		if err != nil {
			return err
		}
		created = true
		return audit.Record(ctx, tx, audit.Event{
			OrganizationID: organizationID, ActorKind: "integration", ActorID: integrationID,
			Action: "integration.webhook.received", ResourceType: "integration", ResourceID: integrationID,
			Decision: "recorded", Metadata: map[string]any{"event_id": eventID, "event_type": eventType},
		})
	})
	return created, err
}

// IngestDocument records a finalized document-inbox upload as a source record
// with a normalized projection and provenance. It is idempotent per document.
func (s *Service) IngestDocument(ctx context.Context, organizationID, documentID, kind, contentType string, size int64, sha256Hex string) error {
	payload, err := json.Marshal(map[string]any{
		"document_id": documentID, "kind": kind, "content_type": contentType,
		"size_bytes": size, "sha256": sha256Hex,
	})
	if err != nil {
		return err
	}
	sum := sha256.Sum256(payload)
	return tenancy.WithOrganizationTx(ctx, s.pool, organizationID, func(tx pgx.Tx) error {
		var integrationID string
		err := tx.QueryRow(ctx, `
			SELECT id FROM integrations WHERE organization_id = $1 AND kind = 'document_inbox' ORDER BY created_at LIMIT 1`,
			organizationID).Scan(&integrationID)
		if err == pgx.ErrNoRows {
			var config []byte = []byte(`{}`)
			if err := tx.QueryRow(ctx, `
				INSERT INTO integrations (organization_id, provider, kind, name, config)
				VALUES ($1, 'document_inbox', 'document_inbox', 'Document inbox', $2)
				RETURNING id`, organizationID, config).Scan(&integrationID); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		var sourceRecordID string
		err = tx.QueryRow(ctx, `
			INSERT INTO source_records
				(organization_id, integration_id, provider, object_type, external_id, raw_payload, checksum, batch_ref)
			VALUES ($1, $2, 'document_inbox', 'document', $3, $4, $5, 'inbox')
			ON CONFLICT (organization_id, integration_id, object_type, external_id) DO NOTHING
			RETURNING id`,
			organizationID, integrationID, documentID, payload, hex.EncodeToString(sum[:])).Scan(&sourceRecordID)
		if err == pgx.ErrNoRows {
			return nil
		}
		if err != nil {
			return err
		}
		normalized, _ := json.Marshal(map[string]any{
			"document_id": documentID, "kind": kind, "content_type": contentType,
			"size_bytes": size, "sha256": sha256Hex,
		})
		if _, err := tx.Exec(ctx, `
			INSERT INTO normalized_records
				(organization_id, source_record_id, record_type, mapping_version, payload)
			VALUES ($1, $2, 'document.v1', 'inbox-v1', $3)`, organizationID, sourceRecordID, normalized); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO provenance_records (organization_id, source_record_id, transformation, mapping_version, details)
			VALUES ($1, $2, 'normalize', 'inbox-v1', '{}'::jsonb)`, organizationID, sourceRecordID)
		return err
	})
}

// SyncJobPayload builds the connector.sync payload.
func SyncJobPayload(organizationID, integrationID string) []byte {
	payload, _ := json.Marshal(map[string]string{"organization_id": organizationID, "integration_id": integrationID})
	return payload
}
