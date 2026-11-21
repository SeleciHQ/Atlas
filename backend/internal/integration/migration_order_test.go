package integration

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	"atlas/internal/platform/migrate"
)

// TestPostgresMajorVersionParity ensures local and CI databases use the same
// PostgreSQL major version. Embedded locally: 16.x. CI: postgres:16.
func TestPostgresMajorVersionParity(t *testing.T) {
	pool := newPool(t)
	var version string
	if err := pool.QueryRow(context.Background(), `SHOW server_version`).Scan(&version); err != nil {
		t.Fatalf("read server version: %v", err)
	}
	if !strings.HasPrefix(version, "16.") {
		t.Fatalf("PostgreSQL server_version = %q, want 16.x (embedded and CI parity)", version)
	}
}

// TestMigrationsApplyInOrder proves numeric ordering: the migration named "10"
// must run after the migration named "2", even though "10" sorts first
// lexicographically.
func TestMigrationsApplyInOrder(t *testing.T) {
	pool := newPool(t)
	resetSchema(t, pool)
	ctx := context.Background()

	fsys := fstest.MapFS{
		"2_second.up.sql": {
			Data: []byte("CREATE TABLE migration_order (seq integer NOT NULL); INSERT INTO migration_order (seq) VALUES (2);"),
		},
		"2_second.down.sql": {Data: []byte("DROP TABLE IF EXISTS migration_order;")},
		"10_tenth.up.sql":   {Data: []byte("INSERT INTO migration_order (seq) VALUES (10);")},
		"10_tenth.down.sql": {Data: []byte("DELETE FROM migration_order WHERE seq = 10;")},
	}

	applied, err := migrate.Up(ctx, pool, fsys)
	if err != nil {
		t.Fatalf("up: %v", err)
	}
	if applied != 2 {
		t.Fatalf("applied = %d, want 2", applied)
	}

	var low, high int
	if err := pool.QueryRow(ctx, `SELECT min(seq), max(seq) FROM migration_order`).Scan(&low, &high); err != nil {
		t.Fatalf("query order: %v", err)
	}
	if low != 2 || high != 10 {
		t.Fatalf("migration contents low=%d high=%d, want 2 and 10", low, high)
	}
}
