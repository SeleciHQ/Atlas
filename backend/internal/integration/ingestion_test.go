package integration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"atlas/internal/connectors"
	"atlas/internal/ingestion"
	"atlas/internal/secrets"
)

func TestConnectorSyncIsIdempotent(t *testing.T) {
	admin, app, _ := setupTenantSchema(t)
	organizationID := seedQueueOrganization(t, admin)
	sealer, err := secrets.NewSealer(strings.Repeat("ab", 32))
	if err != nil {
		t.Fatalf("sealer: %v", err)
	}
	registry := connectors.NewRegistry()
	registry.Register("mock", connectors.NewMock)
	service := ingestion.NewService(app, registry, sealer)
	ctx := context.Background()

	integrationID, err := service.CreateIntegration(ctx, organizationID, "mock", "mock", "Mock ERP", map[string]any{"page_size": 2})
	if err != nil {
		t.Fatalf("create integration: %v", err)
	}
	if err := service.SealCredentials(ctx, organizationID, integrationID, []byte("super-secret-token")); err != nil {
		t.Fatalf("seal credentials: %v", err)
	}

	summary, err := service.RunSync(ctx, organizationID, integrationID, 10)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if summary.RecordsSeen != 10 || summary.RecordsWritten != 10 {
		t.Fatalf("summary = %+v, want 10/10", summary)
	}

	var sourceCount, normalizedCount int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM source_records`).Scan(&sourceCount); err != nil {
		t.Fatalf("count source records: %v", err)
	}
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM normalized_records`).Scan(&normalizedCount); err != nil {
		t.Fatalf("count normalized records: %v", err)
	}
	if sourceCount != 10 || normalizedCount != 10 {
		t.Fatalf("source=%d normalized=%d, want 10/10", sourceCount, normalizedCount)
	}

	var mappingVersion, checksum string
	var raw []byte
	if err := admin.QueryRow(ctx, `
		SELECT n.mapping_version, s.checksum, s.raw_payload
		FROM source_records s JOIN normalized_records n ON n.source_record_id = s.id
		WHERE s.external_id = 'sup-1'`).Scan(&mappingVersion, &checksum, &raw); err != nil {
		t.Fatalf("read source record: %v", err)
	}
	sum := sha256.Sum256(raw)
	if checksum != hex.EncodeToString(sum[:]) {
		t.Fatal("checksum does not match raw payload")
	}
	if mappingVersion != "mock-v1" {
		t.Fatalf("mapping version = %q", mappingVersion)
	}
	if !strings.Contains(string(raw), "billing@alpha.test") {
		t.Fatalf("raw payload not preserved: %s", raw)
	}

	var ciphertext []byte
	if err := admin.QueryRow(ctx, `SELECT ciphertext FROM integration_credentials`).Scan(&ciphertext); err != nil {
		t.Fatalf("read credentials: %v", err)
	}
	if strings.Contains(string(ciphertext), "super-secret-token") {
		t.Fatal("credential stored in plaintext")
	}

	second, err := service.RunSync(ctx, organizationID, integrationID, 10)
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if second.RecordsWritten != 0 {
		t.Fatalf("second sync wrote %d records, want 0", second.RecordsWritten)
	}
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM source_records`).Scan(&sourceCount); err != nil {
		t.Fatalf("count after second sync: %v", err)
	}
	if sourceCount != 10 {
		t.Fatalf("source records after second sync = %d", sourceCount)
	}

	var cursor string
	if err := admin.QueryRow(ctx, `
		SELECT cursor FROM sync_cursors WHERE integration_id = $1 AND object_type = 'invoice'`, integrationID).Scan(&cursor); err != nil {
		t.Fatalf("read cursor: %v", err)
	}
	if cursor == "" {
		t.Fatal("cursor did not advance")
	}
}
