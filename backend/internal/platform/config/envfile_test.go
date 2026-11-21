package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnvFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env.test")
	content := "# comment\nWORKOS_CLIENT_ID=client_abc\nQUOTED=\"quoted value\"\nSINGLE='single value'\nIGNORED\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	t.Setenv("IGNORED", "keep") // ensure we do not touch unrelated keys
	os.Unsetenv("WORKOS_CLIENT_ID")
	os.Unsetenv("QUOTED")
	os.Unsetenv("SINGLE")

	loaded, err := LoadEnvFile(path)
	if err != nil || !loaded {
		t.Fatalf("LoadEnvFile = %v, %v", loaded, err)
	}
	if got := os.Getenv("WORKOS_CLIENT_ID"); got != "client_abc" {
		t.Fatalf("WORKOS_CLIENT_ID = %q", got)
	}
	if got := os.Getenv("QUOTED"); got != "quoted value" {
		t.Fatalf("QUOTED = %q", got)
	}
	if got := os.Getenv("SINGLE"); got != "single value" {
		t.Fatalf("SINGLE = %q", got)
	}
}

func TestLoadEnvFileDoesNotOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env.test")
	if err := os.WriteFile(path, []byte("DATABASE_URL=from-file\n"), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	t.Setenv("DATABASE_URL", "from-environment")
	if _, err := LoadEnvFile(path); err != nil {
		t.Fatalf("LoadEnvFile: %v", err)
	}
	if got := os.Getenv("DATABASE_URL"); got != "from-environment" {
		t.Fatalf("DATABASE_URL = %q, environment must win", got)
	}
}

func TestLoadEnvFileMissingIsNotAnError(t *testing.T) {
	loaded, err := LoadEnvFile(filepath.Join(t.TempDir(), "missing.env"))
	if err != nil || loaded {
		t.Fatalf("missing file: loaded=%v err=%v", loaded, err)
	}
}
