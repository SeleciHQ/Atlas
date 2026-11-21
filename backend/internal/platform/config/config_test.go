package config

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func envFrom(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := LoadFrom(envFrom(map[string]string{
		"DATABASE_URL": "postgres://atlas:atlas@localhost:5432/atlas",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Environment != "development" {
		t.Errorf("environment = %q, want development", cfg.Environment)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr = %q, want :8080", cfg.HTTPAddr)
	}
	if cfg.DBMaxConns != 10 {
		t.Errorf("DBMaxConns = %d, want 10", cfg.DBMaxConns)
	}
	if cfg.LogLevel != slog.LevelInfo {
		t.Errorf("LogLevel = %v, want info", cfg.LogLevel)
	}
	if cfg.MigrationsDir != "migrations" {
		t.Errorf("MigrationsDir = %q, want migrations", cfg.MigrationsDir)
	}
	if cfg.ServiceName != "atlas-api" {
		t.Errorf("ServiceName = %q, want atlas-api", cfg.ServiceName)
	}
	if cfg.AuthDatabaseURL != "postgres://atlas:atlas@localhost:5432/atlas" {
		t.Errorf("AuthDatabaseURL = %q, want the main database URL in development", cfg.AuthDatabaseURL)
	}
	if !cfg.DevAuthEnabled {
		t.Error("DevAuthEnabled should default to true in development")
	}
	if cfg.SessionTTL != 24*time.Hour {
		t.Errorf("SessionTTL = %v, want 24h", cfg.SessionTTL)
	}
}

func TestProductionDisablesDevAuth(t *testing.T) {
	cfg, err := LoadFrom(envFrom(map[string]string{
		"DATABASE_URL": "postgres://atlas:atlas@localhost:5432/atlas",
		"ATLAS_ENV":    "production",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DevAuthEnabled {
		t.Fatal("DevAuthEnabled must default to false in production")
	}
}

func TestProductionRejectsDevAuth(t *testing.T) {
	_, err := LoadFrom(envFrom(map[string]string{
		"DATABASE_URL":     "postgres://atlas:atlas@localhost:5432/atlas",
		"ATLAS_ENV":        "production",
		"DEV_AUTH_ENABLED": "true",
	}))
	if err == nil {
		t.Fatal("expected production to reject DEV_AUTH_ENABLED=true")
	}
}

func TestProductionRefusesToServeWithoutOIDC(t *testing.T) {
	cfg, err := LoadFrom(envFrom(map[string]string{
		"DATABASE_URL":      "postgres://atlas:atlas@localhost:5432/atlas",
		"AUTH_DATABASE_URL": "postgres://atlas_auth:secret@localhost:5432/atlas",
		"ATLAS_ENV":         "production",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := cfg.ValidateServing(); err == nil {
		t.Fatal("production must refuse to serve until managed OIDC is implemented")
	}
}

func TestDevelopmentMayServe(t *testing.T) {
	cfg, err := LoadFrom(envFrom(map[string]string{
		"DATABASE_URL": "postgres://atlas:atlas@localhost:5432/atlas",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := cfg.ValidateServing(); err != nil {
		t.Fatalf("development should be serveable: %v", err)
	}
}

func TestAuthDatabaseURLOverride(t *testing.T) {
	cfg, err := LoadFrom(envFrom(map[string]string{
		"DATABASE_URL":      "postgres://atlas_app:secret@db.example.com:5432/atlas",
		"AUTH_DATABASE_URL": "postgres://atlas_auth:secret@db.example.com:5432/atlas",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AuthDatabaseURL == cfg.DatabaseURL {
		t.Fatal("AUTH_DATABASE_URL override was not applied")
	}
}

func TestSessionTTLOverride(t *testing.T) {
	cfg, err := LoadFrom(envFrom(map[string]string{
		"DATABASE_URL": "postgres://atlas:atlas@localhost:5432/atlas",
		"SESSION_TTL":  "2h",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SessionTTL != 2*time.Hour {
		t.Errorf("SessionTTL = %v, want 2h", cfg.SessionTTL)
	}
}

func TestLoadOverrides(t *testing.T) {
	cfg, err := LoadFrom(envFrom(map[string]string{
		"DATABASE_URL":     "postgresql://user:pass@db.example.com:5432/atlas?sslmode=require",
		"ATLAS_ENV":        "production",
		"PORT":             "9090",
		"DB_MAX_CONNS":     "25",
		"LOG_LEVEL":        "warn",
		"MIGRATIONS_DIR":   "/srv/atlas/migrations",
		"SHUTDOWN_TIMEOUT": "30s",
		"SERVICE_NAME":     "atlas-mcp",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HTTPAddr != ":9090" {
		t.Errorf("HTTPAddr = %q", cfg.HTTPAddr)
	}
	if cfg.DBMaxConns != 25 {
		t.Errorf("DBMaxConns = %d", cfg.DBMaxConns)
	}
	if cfg.LogLevel != slog.LevelWarn {
		t.Errorf("LogLevel = %v", cfg.LogLevel)
	}
	if cfg.MigrationsDir != "/srv/atlas/migrations" {
		t.Errorf("MigrationsDir = %q", cfg.MigrationsDir)
	}
	if cfg.ServiceName != "atlas-mcp" {
		t.Errorf("ServiceName = %q", cfg.ServiceName)
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	cases := map[string]map[string]string{
		"missing database url": {},
		"bad database url":     {"DATABASE_URL": "not-a-url"},
		"wrong scheme":         {"DATABASE_URL": "mysql://localhost/atlas"},
		"bad port":             {"DATABASE_URL": "postgres://localhost/atlas", "PORT": "70000"},
		"non numeric port":     {"DATABASE_URL": "postgres://localhost/atlas", "PORT": "http"},
		"bad environment":      {"DATABASE_URL": "postgres://localhost/atlas", "ATLAS_ENV": "qa"},
		"bad log level":        {"DATABASE_URL": "postgres://localhost/atlas", "LOG_LEVEL": "verbose"},
		"bad max conns":        {"DATABASE_URL": "postgres://localhost/atlas", "DB_MAX_CONNS": "0"},
		"bad shutdown":         {"DATABASE_URL": "postgres://localhost/atlas", "SHUTDOWN_TIMEOUT": "5"},
	}
	for name, values := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadFrom(envFrom(values)); err == nil {
				t.Fatalf("expected an error for %s", name)
			} else if strings.Contains(err.Error(), "postgres://") {
				t.Fatalf("error must not echo the database URL: %v", err)
			}
		})
	}
}
