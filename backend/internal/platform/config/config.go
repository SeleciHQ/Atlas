// Package config loads and validates Atlas process configuration from the environment.
package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"atlas/internal/platform/version"
)

// Config is the validated configuration shared by Atlas processes.
type Config struct {
	Environment     string
	ServiceName     string
	Version         string
	HTTPAddr        string
	DatabaseURL     string
	AuthDatabaseURL string
	DBMaxConns      int32
	LogLevel        slog.Level
	MigrationsDir   string
	ShutdownTimeout time.Duration
	DevAuthEnabled  bool
	SessionTTL      time.Duration

	// Managed OIDC (WorkOS AuthKit). All three WorkOS values must be provided
	// together. When absent, only development authentication is available.
	WorkOSClientID         string
	WorkOSAPIKey           string
	WorkOSRedirectURI      string
	WorkOSAuthKitDomain    string
	WorkOSAuthorizeURL     string
	WorkOSAuthenticateURL  string
	OIDCEnabled            bool
	OIDCProductionVerified bool

	// Object storage (Phase 3). Local is for development; production requires
	// the S3-compatible R2 adapter.
	StorageDriver        string
	StorageLocalDir      string
	StorageLocalSecret   string
	StoragePublicBaseURL string
	R2Endpoint           string
	R2Region             string
	R2Bucket             string
	R2AccessKeyID        string
	R2SecretAccessKey    string
}

const (
	defaultPort            = 8080
	defaultDBMaxConns      = 10
	defaultShutdownTimeout = 10 * time.Second
	defaultMigrationsDir   = "migrations"
	defaultServiceName     = "atlas-api"
	defaultSessionTTL      = 24 * time.Hour
)

var allowedEnvironments = map[string]struct{}{
	"development": {},
	"test":        {},
	"staging":     {},
	"production":  {},
}

// Load reads configuration from the process environment.
func Load() (Config, error) {
	return LoadFrom(os.Getenv)
}

// LoadFrom reads configuration through getenv, which makes the loader testable.
func LoadFrom(getenv func(string) string) (Config, error) {
	cfg := Config{
		Environment:     strings.ToLower(strings.TrimSpace(getenv("ATLAS_ENV"))),
		ServiceName:     strings.TrimSpace(getenv("SERVICE_NAME")),
		Version:         version.Version,
		MigrationsDir:   strings.TrimSpace(getenv("MIGRATIONS_DIR")),
		ShutdownTimeout: defaultShutdownTimeout,
		SessionTTL:      defaultSessionTTL,
	}

	if cfg.Environment == "" {
		cfg.Environment = "development"
	}
	if _, ok := allowedEnvironments[cfg.Environment]; !ok {
		return Config{}, fmt.Errorf("invalid ATLAS_ENV %q: must be one of development, test, staging, production", cfg.Environment)
	}

	if cfg.ServiceName == "" {
		cfg.ServiceName = defaultServiceName
	}

	port := defaultPort
	if raw := strings.TrimSpace(getenv("PORT")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 65535 {
			return Config{}, fmt.Errorf("invalid PORT %q: must be an integer between 1 and 65535", raw)
		}
		port = parsed
	}
	cfg.HTTPAddr = fmt.Sprintf(":%d", port)

	cfg.DatabaseURL = strings.TrimSpace(getenv("DATABASE_URL"))
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if err := validateDatabaseURL(cfg.DatabaseURL); err != nil {
		return Config{}, err
	}

	// Identity/session access uses a separate database role. Defaults to the
	// main URL for development; staging and production must provide a
	// dedicated AUTH_DATABASE_URL.
	cfg.AuthDatabaseURL = strings.TrimSpace(getenv("AUTH_DATABASE_URL"))
	if cfg.AuthDatabaseURL == "" {
		cfg.AuthDatabaseURL = cfg.DatabaseURL
	} else if err := validateDatabaseURL(cfg.AuthDatabaseURL); err != nil {
		return Config{}, fmt.Errorf("invalid AUTH_DATABASE_URL: %w", err)
	}

	cfg.DBMaxConns = defaultDBMaxConns
	if raw := strings.TrimSpace(getenv("DB_MAX_CONNS")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			return Config{}, fmt.Errorf("invalid DB_MAX_CONNS %q: must be an integer between 1 and 100", raw)
		}
		cfg.DBMaxConns = int32(parsed)
	}

	cfg.LogLevel = slog.LevelInfo
	if raw := strings.ToLower(strings.TrimSpace(getenv("LOG_LEVEL"))); raw != "" {
		switch raw {
		case "debug":
			cfg.LogLevel = slog.LevelDebug
		case "info":
			cfg.LogLevel = slog.LevelInfo
		case "warn":
			cfg.LogLevel = slog.LevelWarn
		case "error":
			cfg.LogLevel = slog.LevelError
		default:
			return Config{}, fmt.Errorf("invalid LOG_LEVEL %q: must be one of debug, info, warn, error", raw)
		}
	}

	if cfg.MigrationsDir == "" {
		cfg.MigrationsDir = defaultMigrationsDir
	}

	if raw := strings.TrimSpace(getenv("SHUTDOWN_TIMEOUT")); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed < time.Second || parsed > 2*time.Minute {
			return Config{}, fmt.Errorf("invalid SHUTDOWN_TIMEOUT %q: must be a duration between 1s and 2m", raw)
		}
		cfg.ShutdownTimeout = parsed
	}

	// Development authentication (opaque sessions) is enabled by default only
	// in development and test environments. It can never be enabled in staging
	// or production; the production path is managed OIDC (ADR-002).
	cfg.DevAuthEnabled = cfg.Environment == "development" || cfg.Environment == "test"
	if raw := strings.TrimSpace(getenv("DEV_AUTH_ENABLED")); raw != "" {
		enabled, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("invalid DEV_AUTH_ENABLED %q: must be true or false", raw)
		}
		if enabled && (cfg.Environment == "staging" || cfg.Environment == "production") {
			return Config{}, fmt.Errorf("DEV_AUTH_ENABLED must be false in %s", cfg.Environment)
		}
		cfg.DevAuthEnabled = enabled
	}

	if raw := strings.TrimSpace(getenv("SESSION_TTL")); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed < time.Hour || parsed > 30*24*time.Hour {
			return Config{}, fmt.Errorf("invalid SESSION_TTL %q: must be a duration between 1h and 720h", raw)
		}
		cfg.SessionTTL = parsed
	}

	cfg.WorkOSClientID = strings.TrimSpace(getenv("WORKOS_CLIENT_ID"))
	cfg.WorkOSAPIKey = strings.TrimSpace(getenv("WORKOS_API_KEY"))
	cfg.WorkOSRedirectURI = strings.TrimSpace(getenv("WORKOS_REDIRECT_URI"))
	cfg.WorkOSAuthKitDomain = strings.TrimSpace(getenv("WORKOS_AUTHKIT_DOMAIN"))
	cfg.WorkOSAuthorizeURL = strings.TrimSpace(getenv("WORKOS_AUTHORIZE_URL"))
	cfg.WorkOSAuthenticateURL = strings.TrimSpace(getenv("WORKOS_AUTHENTICATE_URL"))
	// Hosted AuthKit sign-in starts at the environment's AuthKit domain root.
	// The api.workos.com authorize endpoint is only a fallback for environments
	// without a hosted domain; an explicit WORKOS_AUTHORIZE_URL always wins
	// (used by tests and mock providers).
	if cfg.WorkOSAuthorizeURL == "" {
		if cfg.WorkOSAuthKitDomain != "" {
			cfg.WorkOSAuthorizeURL = strings.TrimSuffix(cfg.WorkOSAuthKitDomain, "/") + "/"
		} else {
			cfg.WorkOSAuthorizeURL = "https://api.workos.com/user_management/authorize"
		}
	}
	if cfg.WorkOSAuthenticateURL == "" {
		cfg.WorkOSAuthenticateURL = "https://api.workos.com/user_management/authenticate"
	}
	workosProvided := cfg.WorkOSClientID != "" || cfg.WorkOSAPIKey != "" || cfg.WorkOSRedirectURI != ""
	if workosProvided {
		if cfg.WorkOSClientID == "" || cfg.WorkOSAPIKey == "" || cfg.WorkOSRedirectURI == "" {
			return Config{}, fmt.Errorf("managed OIDC requires WORKOS_CLIENT_ID, WORKOS_API_KEY and WORKOS_REDIRECT_URI together")
		}
		cfg.OIDCEnabled = true
	}
	if raw := strings.TrimSpace(getenv("OIDC_PRODUCTION_VERIFIED")); raw != "" {
		verified, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("invalid OIDC_PRODUCTION_VERIFIED %q: must be true or false", raw)
		}
		cfg.OIDCProductionVerified = verified
	}

	cfg.StorageDriver = strings.ToLower(strings.TrimSpace(getenv("STORAGE_DRIVER")))
	if cfg.StorageDriver == "" {
		cfg.StorageDriver = "local"
	}
	if cfg.StorageDriver != "local" && cfg.StorageDriver != "s3" {
		return Config{}, fmt.Errorf("invalid STORAGE_DRIVER %q: must be local or s3", cfg.StorageDriver)
	}
	cfg.StorageLocalDir = strings.TrimSpace(getenv("STORAGE_LOCAL_DIR"))
	if cfg.StorageLocalDir == "" {
		cfg.StorageLocalDir = ".atlas-storage"
	}
	cfg.StorageLocalSecret = strings.TrimSpace(getenv("STORAGE_LOCAL_SECRET"))
	if cfg.StorageLocalSecret == "" && (cfg.Environment == "development" || cfg.Environment == "test") {
		cfg.StorageLocalSecret = "atlas-development-storage-secret"
	}
	cfg.StoragePublicBaseURL = strings.TrimSpace(getenv("STORAGE_PUBLIC_BASE_URL"))
	cfg.R2Endpoint = strings.TrimSpace(getenv("R2_ENDPOINT"))
	cfg.R2Region = strings.TrimSpace(getenv("R2_REGION"))
	cfg.R2Bucket = strings.TrimSpace(getenv("R2_BUCKET"))
	cfg.R2AccessKeyID = strings.TrimSpace(getenv("R2_ACCESS_KEY_ID"))
	cfg.R2SecretAccessKey = strings.TrimSpace(getenv("R2_SECRET_ACCESS_KEY"))

	return cfg, nil
}

func validateDatabaseURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid DATABASE_URL: %w", err)
	}
	switch parsed.Scheme {
	case "postgres", "postgresql":
		return nil
	default:
		return fmt.Errorf("invalid DATABASE_URL: scheme must be postgres or postgresql")
	}
}

// ValidateServing reports whether this process may serve traffic in the
// configured environment. Production requires managed OIDC (ADR-002) and must
// never enable the development authentication path.
func (c Config) ValidateServing() error {
	if c.Environment == "production" {
		if c.DevAuthEnabled {
			return fmt.Errorf("development authentication cannot be enabled in production")
		}
		if !c.OIDCEnabled {
			return fmt.Errorf("production authentication is not configured (WorkOS OIDC env vars missing); refusing to serve")
		}
		if !c.OIDCProductionVerified {
			return fmt.Errorf("production OIDC has not completed sandbox verification (set OIDC_PRODUCTION_VERIFIED=true only after the E2E passes)")
		}
		if c.StorageDriver != "s3" {
			return fmt.Errorf("production storage must use the S3-compatible adapter (STORAGE_DRIVER=s3)")
		}
		if c.R2Endpoint == "" || c.R2Bucket == "" || c.R2AccessKeyID == "" || c.R2SecretAccessKey == "" {
			return fmt.Errorf("production storage requires R2_ENDPOINT, R2_BUCKET, R2_ACCESS_KEY_ID and R2_SECRET_ACCESS_KEY")
		}
	}
	if c.StorageDriver == "local" && c.StorageLocalSecret == "" {
		return fmt.Errorf("local storage requires STORAGE_LOCAL_SECRET")
	}
	return nil
}
