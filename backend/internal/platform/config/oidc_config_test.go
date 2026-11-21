package config

import "testing"

func TestProductionOIDCRequiresSandboxVerification(t *testing.T) {
	base := map[string]string{
		"DATABASE_URL":        "postgres://atlas:atlas@localhost:5432/atlas",
		"AUTH_DATABASE_URL":   "postgres://atlas_auth:secret@localhost:5432/atlas",
		"ATLAS_ENV":           "production",
		"WORKOS_CLIENT_ID":    "client_test",
		"WORKOS_API_KEY":      "sk_test",
		"WORKOS_REDIRECT_URI": "https://atlas.example.com/v1/auth/oidc/callback",
	}
	cfg, err := LoadFrom(envFrom(base))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !cfg.OIDCEnabled {
		t.Fatal("OIDC should be enabled when WorkOS values are complete")
	}
	if err := cfg.ValidateServing(); err == nil {
		t.Fatal("production must refuse to serve before sandbox verification")
	}

	base["OIDC_PRODUCTION_VERIFIED"] = "true"
	cfg, err = LoadFrom(envFrom(base))
	if err != nil {
		t.Fatalf("load verified: %v", err)
	}
	if err := cfg.ValidateServing(); err != nil {
		t.Fatalf("verified production should serve: %v", err)
	}
}

func TestWorkOSConfigRequiresAllValues(t *testing.T) {
	_, err := LoadFrom(envFrom(map[string]string{
		"DATABASE_URL":     "postgres://atlas:atlas@localhost:5432/atlas",
		"WORKOS_CLIENT_ID": "client_test",
	}))
	if err == nil {
		t.Fatal("partial WorkOS configuration must be rejected")
	}
}
