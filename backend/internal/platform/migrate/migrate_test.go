package migrate

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestLoadSortsAndComputesChecksums(t *testing.T) {
	fsys := fstest.MapFS{
		"0002_second.up.sql":   {Data: []byte("SELECT 2;")},
		"0002_second.down.sql": {Data: []byte("SELECT 2;")},
		"0001_first.up.sql":    {Data: []byte("SELECT 1;")},
		"0001_first.down.sql":  {Data: []byte("SELECT 1;")},
		"README.md":            {Data: []byte("ignored")},
	}
	migrations, err := Load(fsys)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(migrations) != 2 {
		t.Fatalf("len = %d, want 2", len(migrations))
	}
	if migrations[0].Version != "0001" || migrations[1].Version != "0002" {
		t.Fatalf("unexpected order: %+v", migrations)
	}
	if len(migrations[0].Checksum) != 64 {
		t.Fatalf("checksum = %q", migrations[0].Checksum)
	}
}

func TestLoadOrdersVersionsNumerically(t *testing.T) {
	// Lexicographic order would put "10" before "2"; numeric ordering must not.
	fsys := fstest.MapFS{
		"10_tenth.up.sql":   {Data: []byte("SELECT 10;")},
		"10_tenth.down.sql": {Data: []byte("SELECT 10;")},
		"2_second.up.sql":   {Data: []byte("SELECT 2;")},
		"2_second.down.sql": {Data: []byte("SELECT 2;")},
	}
	migrations, err := Load(fsys)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(migrations) != 2 {
		t.Fatalf("len = %d, want 2", len(migrations))
	}
	if migrations[0].Version != "2" || migrations[1].Version != "10" {
		t.Fatalf("unexpected order: %+v", migrations)
	}
}

func TestLoadRequiresDownMigration(t *testing.T) {
	fsys := fstest.MapFS{
		"0001_one.up.sql": {Data: []byte("SELECT 1;")},
	}
	_, err := Load(fsys)
	if err == nil || !strings.Contains(err.Error(), "missing its .down.sql") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRejectsDuplicateDirection(t *testing.T) {
	fsys := fstest.MapFS{
		"0001_one.up.sql":     {Data: []byte("SELECT 1;")},
		"0001_one.down.sql":   {Data: []byte("SELECT 1;")},
		"0001_other.up.sql":   {Data: []byte("SELECT 2;")},
		"0001_other.down.sql": {Data: []byte("SELECT 2;")},
	}
	_, err := Load(fsys)
	if err == nil {
		t.Fatal("expected an error for conflicting migration names")
	}
}

func TestLoadRejectsMissingUp(t *testing.T) {
	fsys := fstest.MapFS{
		"0001_one.down.sql": {Data: []byte("SELECT 1;")},
	}
	_, err := Load(fsys)
	if err == nil || !strings.Contains(err.Error(), "missing its .up.sql") {
		t.Fatalf("unexpected error: %v", err)
	}
}
