package config

import "testing"

func TestAuthorizeURLUsesAuthKitDomain(t *testing.T) {
	cfg, err := LoadFrom(envFrom(map[string]string{
		"DATABASE_URL":          "postgres://atlas:atlas@localhost:5432/atlas",
		"WORKOS_CLIENT_ID":      "client_test",
		"WORKOS_API_KEY":        "sk_test",
		"WORKOS_REDIRECT_URI":   "http://localhost:8080/v1/auth/oidc/callback",
		"WORKOS_AUTHKIT_DOMAIN": "https://example-staging.authkit.app/",
	}))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.WorkOSAuthorizeURL != "https://example-staging.authkit.app/" {
		t.Fatalf("authorize URL = %q, want the hosted AuthKit domain root", cfg.WorkOSAuthorizeURL)
	}
	if cfg.WorkOSAuthenticateURL != "https://api.workos.com/user_management/authenticate" {
		t.Fatalf("authenticate URL should remain the WorkOS API: %q", cfg.WorkOSAuthenticateURL)
	}
}

func TestAuthorizeURLOverrideWins(t *testing.T) {
	cfg, err := LoadFrom(envFrom(map[string]string{
		"DATABASE_URL":          "postgres://atlas:atlas@localhost:5432/atlas",
		"WORKOS_CLIENT_ID":      "client_test",
		"WORKOS_API_KEY":        "sk_test",
		"WORKOS_REDIRECT_URI":   "http://localhost:8080/v1/auth/oidc/callback",
		"WORKOS_AUTHKIT_DOMAIN": "https://example-staging.authkit.app",
		"WORKOS_AUTHORIZE_URL":  "http://127.0.0.1:9999/authorize",
	}))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.WorkOSAuthorizeURL != "http://127.0.0.1:9999/authorize" {
		t.Fatalf("explicit override must win: %q", cfg.WorkOSAuthorizeURL)
	}
}

func TestAuthorizeURLFallsBackWithoutDomain(t *testing.T) {
	cfg, err := LoadFrom(envFrom(map[string]string{
		"DATABASE_URL":        "postgres://atlas:atlas@localhost:5432/atlas",
		"WORKOS_CLIENT_ID":    "client_test",
		"WORKOS_API_KEY":      "sk_test",
		"WORKOS_REDIRECT_URI": "http://localhost:8080/v1/auth/oidc/callback",
	}))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.WorkOSAuthorizeURL != "https://api.workos.com/user_management/authorize" {
		t.Fatalf("fallback authorize URL = %q", cfg.WorkOSAuthorizeURL)
	}
}
