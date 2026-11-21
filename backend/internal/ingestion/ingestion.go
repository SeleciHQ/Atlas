// Package ingestion orchestrates connector syncs: raw source records with
// checksums, versioned normalization, cursors, sync history and provenance.
// Connectors never write canonical tables; this package only writes ingestion
// tables.
package ingestion

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"atlas/internal/connectors"
	"atlas/internal/secrets"
	"atlas/internal/tenancy"
)

const mappingVersion = "mock-v1"

// Summary reports one sync run.
type Summary struct {
	RunID          string `json:"run_id"`
	RecordsSeen    int    `json:"records_seen"`
	RecordsWritten int    `json:"records_written"`
	Cursor         string `json:"cursor"`
}

// Service runs connector syncs and persists ingestion records.
type Service struct {
	pool     *pgxpool.Pool
	registry *connectors.Registry
	sealer   *secrets.Sealer
}

// NewService constructs the ingestion service.
func NewService(pool *pgxpool.Pool, registry *connectors.Registry, sealer *secrets.Sealer) *Service {
	return &Service{pool: pool, registry: registry, sealer: sealer}
}

// CreateIntegration registers an integration (metadata only).
func (s *Service) CreateIntegration(ctx context.Context, organizationID, provider, kind, name string, settings map[string]any) (string, error) {
	if settings == nil {
		settings = map[string]any{}
	}
	config, err := json.Marshal(settings)
	if err != nil {
		return "", fmt.Errorf("encode integration config: %w", err)
	}
	integrationID := uuid.NewString()
	err = tenancy.WithOrganizationTx(ctx, s.pool, organizationID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO integrations (organization_id, id, provider, kind, name, config)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			organizationID, integrationID, provider, kind, name, config)
		return err
	})
	if err != nil {
		return "", fmt.Errorf("create integration: %w", err)
	}
	return integrationID, nil
}

// SealCredentials encrypts and stores integration credentials.
func (s *Service) SealCredentials(ctx context.Context, organizationID, integrationID string, credentials []byte) error {
	aad := []byte(organizationID + "|" + integrationID)
	sealed, err := s.sealer.Seal(credentials, aad)
	if err != nil {
		return err
	}
	return tenancy.WithOrganizationTx(ctx, s.pool, organizationID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO integration_credentials
				(organization_id, integration_id, ciphertext, wrapped_key, nonce, wrapped_nonce, key_version)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (organization_id, integration_id) DO UPDATE SET
				ciphertext = EXCLUDED.ciphertext, wrapped_key = EXCLUDED.wrapped_key,
				nonce = EXCLUDED.nonce, wrapped_nonce = EXCLUDED.wrapped_nonce,
				key_version = EXCLUDED.key_version, rotated_at = now()`,
			organizationID, integrationID, sealed.Ciphertext, sealed.WrappedKey,
			sealed.Nonce, sealed.WrappedNonce, sealed.KeyVersion)
		return err
	})
}

type integrationRow struct {
	provider string
	kind     string
	settings map[string]any
}

// RunSync executes a connector sync for all supported objects.
func (s *Service) RunSync(ctx context.Context, organizationID, integrationID string, maxPages int) (*Summary, error) {
	if maxPages <= 0 {
		maxPages = 25
	}
	var integration integrationRow
	var rawSettings []byte
	err := tenancy.WithOrganizationTx(ctx, s.pool, organizationID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT provider, kind, config FROM integrations
			WHERE organization_id = $1 AND id = $2`, organizationID, integrationID).
			Scan(&integration.provider, &integration.kind, &rawSettings)
	})
	if err != nil {
		return nil, fmt.Errorf("load integration: %w", err)
	}
	if len(rawSettings) > 0 {
		_ = json.Unmarshal(rawSettings, &integration.settings)
	}

	credentials, err := s.credentials(ctx, organizationID, integrationID)
	if err != nil {
		return nil, err
	}
	connector, err := s.registry.Open(integration.provider)
	if err != nil {
		return nil, err
	}
	cfg := connectors.ConnectionConfig{
		OrganizationID: organizationID, IntegrationID: integrationID,
		Provider: integration.provider, Settings: integration.settings, Credentials: credentials,
	}
	if err := connector.ValidateConnection(ctx, cfg); err != nil {
		return nil, fmt.Errorf("validate connection: %w", err)
	}
	capabilities, err := connector.DiscoverCapabilities(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("discover capabilities: %w", err)
	}

	runID := uuid.NewString()
	if err := tenancy.WithOrganizationTx(ctx, s.pool, organizationID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO sync_runs (organization_id, id, integration_id, status) VALUES ($1, $2, $3, 'running')`,
			organizationID, runID, integrationID)
		return err
	}); err != nil {
		return nil, fmt.Errorf("start sync run: %w", err)
	}

	summary := &Summary{RunID: runID}
	syncErr := s.syncObjects(ctx, organizationID, integrationID, runID, connector, cfg, capabilities, maxPages, summary)

	status := "completed"
	errorText := ""
	if syncErr != nil {
		status = "failed"
		errorText = syncErr.Error()
	}
	_ = tenancy.WithOrganizationTx(ctx, s.pool, organizationID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE sync_runs SET status = $3, records_seen = $4, records_written = $5,
			       cursor_after = $6, error = nullif($7, ''), finished_at = now()
			WHERE organization_id = $1 AND id = $2`,
			organizationID, runID, status, summary.RecordsSeen, summary.RecordsWritten, summary.Cursor, errorText)
		if err != nil {
			return err
		}
		health := "connected"
		if syncErr != nil {
			health = "degraded"
		}
		_, err = tx.Exec(ctx, `
			UPDATE integrations SET last_synced_at = now(), status = $3, updated_at = now()
			WHERE organization_id = $1 AND id = $2`, organizationID, integrationID, health)
		return err
	})
	if syncErr != nil {
		return summary, syncErr
	}
	return summary, nil
}

func (s *Service) syncObjects(ctx context.Context, organizationID, integrationID, runID string, connector connectors.Connector, cfg connectors.ConnectionConfig, capabilities connectors.Capabilities, maxPages int, summary *Summary) error {
	for _, objectType := range capabilities.Objects {
		cursor := connectors.Cursor("")
		_ = tenancy.WithOrganizationTx(ctx, s.pool, organizationID, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `
				SELECT cursor FROM sync_cursors WHERE organization_id = $1 AND integration_id = $2 AND object_type = $3`,
				organizationID, integrationID, string(objectType)).Scan(&cursor)
		})
		for page := 0; page < maxPages; page++ {
			pageResult, err := connector.Sync(ctx, cfg, objectType, cursor)
			if err != nil {
				return fmt.Errorf("sync %s: %w", objectType, err)
			}
			summary.RecordsSeen += len(pageResult.Records)
			if err := s.persistPage(ctx, organizationID, integrationID, runID, cfg.Provider, objectType, pageResult.Records, pageResult.NextCursor, summary); err != nil {
				return err
			}
			summary.Cursor = string(pageResult.NextCursor)
			cursor = pageResult.NextCursor
			if !pageResult.HasMore {
				break
			}
		}
	}
	return nil
}

func (s *Service) persistPage(ctx context.Context, organizationID, integrationID, runID, provider string, objectType connectors.ObjectType, records []connectors.Record, next connectors.Cursor, summary *Summary) error {
	return tenancy.WithOrganizationTx(ctx, s.pool, organizationID, func(tx pgx.Tx) error {
		for _, record := range records {
			sum := sha256.Sum256(record.Payload)
			checksum := hex.EncodeToString(sum[:])
			var sourceRecordID string
			err := tx.QueryRow(ctx, `
				INSERT INTO source_records
					(organization_id, integration_id, provider, object_type, external_id, raw_payload, checksum, batch_ref)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
				ON CONFLICT (organization_id, integration_id, object_type, external_id) DO NOTHING
				RETURNING id`,
				organizationID, integrationID, provider, string(record.ObjectType), record.ExternalID,
				[]byte(record.Payload), checksum, runID).Scan(&sourceRecordID)
			if err == pgx.ErrNoRows {
				continue
			}
			if err != nil {
				return fmt.Errorf("insert source record: %w", err)
			}
			recordType, normalized, err := normalize(record.ObjectType, record.Payload)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO normalized_records
					(organization_id, source_record_id, record_type, mapping_version, payload)
				VALUES ($1, $2, $3, $4, $5)`,
				organizationID, sourceRecordID, recordType, mappingVersion, normalized); err != nil {
				return fmt.Errorf("insert normalized record: %w", err)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO provenance_records
					(organization_id, source_record_id, transformation, mapping_version, details)
				VALUES ($1, $2, 'normalize', $3, $4)`,
				organizationID, sourceRecordID, mappingVersion,
				map[string]any{"object_type": string(record.ObjectType)}); err != nil {
				return fmt.Errorf("insert provenance: %w", err)
			}
			summary.RecordsWritten++
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO sync_cursors (organization_id, integration_id, object_type, cursor)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (organization_id, integration_id, object_type)
			DO UPDATE SET cursor = EXCLUDED.cursor, updated_at = now()`,
			organizationID, integrationID, string(objectType), string(next)); err != nil {
			return fmt.Errorf("update cursor: %w", err)
		}
		return nil
	})
}

func (s *Service) credentials(ctx context.Context, organizationID, integrationID string) ([]byte, error) {
	var sealed secrets.Sealed
	err := tenancy.WithOrganizationTx(ctx, s.pool, organizationID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT ciphertext, wrapped_key, nonce, wrapped_nonce, key_version
			FROM integration_credentials WHERE organization_id = $1 AND integration_id = $2`,
			organizationID, integrationID).
			Scan(&sealed.Ciphertext, &sealed.WrappedKey, &sealed.Nonce, &sealed.WrappedNonce, &sealed.KeyVersion)
	})
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load integration credentials: %w", err)
	}
	return s.sealer.Open(sealed, []byte(organizationID+"|"+integrationID))
}

// normalize converts a provider record into a typed normalized payload.
func normalize(objectType connectors.ObjectType, raw []byte) (string, []byte, error) {
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		return "", nil, fmt.Errorf("decode source payload: %w", err)
	}
	text := func(key string) string { value, _ := fields[key].(string); return value }
	number := func(key string) int64 {
		switch value := fields[key].(type) {
		case float64:
			return int64(value)
		case int:
			return int64(value)
		default:
			return 0
		}
	}
	var typed any
	switch objectType {
	case connectors.ObjectSupplier:
		typed = map[string]any{"external_id": text("external_id"), "name": text("name"), "email": text("email"), "currency": text("currency")}
	case connectors.ObjectPurchaseOrder:
		typed = map[string]any{"external_id": text("external_id"), "supplier_external_id": text("supplier_external_id"), "number": text("number"), "total_minor": number("total_minor"), "currency": text("currency")}
	case connectors.ObjectDelivery:
		typed = map[string]any{"external_id": text("external_id"), "purchase_order_external_id": text("purchase_order_external_id"), "number": text("number"), "delivered_on": text("delivered_on")}
	case connectors.ObjectInvoice:
		typed = map[string]any{"external_id": text("external_id"), "supplier_external_id": text("supplier_external_id"), "number": text("number"), "total_minor": number("total_minor"), "currency": text("currency")}
	default:
		return "", nil, fmt.Errorf("no normalizer for object type %q", objectType)
	}
	encoded, err := json.Marshal(typed)
	if err != nil {
		return "", nil, err
	}
	return string(objectType) + ".v1", encoded, nil
}

var _ = time.Now
