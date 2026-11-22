// Command atlas-api runs the Atlas REST API process.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"atlas/internal/authorization"
	"atlas/internal/connectors"
	"atlas/internal/documents"
	"atlas/internal/httpapi"
	"atlas/internal/identity"
	"atlas/internal/ingestion"
	"atlas/internal/jobs"
	"atlas/internal/orgs"
	"atlas/internal/platform/config"
	"atlas/internal/platform/db"
	"atlas/internal/platform/logging"
	"atlas/internal/platform/migrate"
	"atlas/internal/reconciliation"
	"atlas/internal/secrets"
	"atlas/internal/storage"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "atlas-api:", err)
		os.Exit(1)
	}
}

func run() error {
	envFile := os.Getenv("ATLAS_ENV_FILE")
	if envFile == "" {
		envFile = ".env.workos"
	}
	if _, err := config.LoadEnvFile(envFile); err != nil {
		return fmt.Errorf("load env file %s: %w", envFile, err)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := cfg.ValidateServing(); err != nil {
		return err
	}
	logger := logging.New(cfg)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL, cfg.DBMaxConns)
	if err != nil {
		return err
	}
	defer pool.Close()
	logger.Info("database_connected")

	// Identity/session operations use a separate, narrowly privileged database
	// role. In development the same URL may be used; staging/production should
	// provide AUTH_DATABASE_URL.
	authPool := pool
	if cfg.AuthDatabaseURL != cfg.DatabaseURL {
		authPool, err = db.Connect(ctx, cfg.AuthDatabaseURL, cfg.DBMaxConns)
		if err != nil {
			return fmt.Errorf("connect auth database: %w", err)
		}
		defer authPool.Close()
		logger.Info("auth_database_connected")
	} else if cfg.Environment != "development" && cfg.Environment != "test" {
		logger.Warn("auth_database_role_not_separated", "detail", "set AUTH_DATABASE_URL to a dedicated role outside development")
	}

	migrationsFS := os.DirFS(cfg.MigrationsDir)
	if pending, err := migrate.Pending(ctx, pool, migrationsFS); err != nil {
		logger.Warn("migration_state_unavailable", "error", err.Error())
	} else if pending > 0 {
		logger.Warn("migrations_pending", "count", pending, "detail", "run atlas-migrate up before serving production traffic")
	} else {
		logger.Info("migrations_current")
	}

	sessions := identity.NewSessionService(authPool, cfg.SessionTTL)
	invitations := identity.NewInvitationService(pool, 7*24*time.Hour)

	var oidcService *identity.OIDCService
	if cfg.OIDCEnabled {
		workos := identity.NewWorkOSClient(identity.WorkOSConfig{
			ClientID:        cfg.WorkOSClientID,
			APIKey:          cfg.WorkOSAPIKey,
			RedirectURI:     cfg.WorkOSRedirectURI,
			AuthorizeURL:    cfg.WorkOSAuthorizeURL,
			AuthenticateURL: cfg.WorkOSAuthenticateURL,
		})
		oidcService = identity.NewOIDCService(authPool, sessions, workos, invitations)
		logger.Info("managed_oidc_enabled", "provider", "workos")
	}

	// Object storage: local filesystem in development; S3-compatible R2 in
	// staging/production.
	var store storage.Store
	var storageHandler http.Handler
	switch cfg.StorageDriver {
	case "s3":
		s3Store, err := storage.NewS3(storage.S3Config{
			Endpoint: cfg.R2Endpoint, Region: cfg.R2Region, Bucket: cfg.R2Bucket,
			AccessKeyID: cfg.R2AccessKeyID, SecretAccessKey: cfg.R2SecretAccessKey,
		})
		if err != nil {
			return fmt.Errorf("configure s3 storage: %w", err)
		}
		store = s3Store
		logger.Info("object_storage_enabled", "driver", "s3")
	default:
		if err := os.MkdirAll(cfg.StorageLocalDir, 0o750); err != nil {
			return fmt.Errorf("create local storage directory: %w", err)
		}
		baseURL := cfg.StoragePublicBaseURL
		if baseURL == "" {
			baseURL = "http://localhost" + cfg.HTTPAddr
		}
		local := storage.NewLocal(cfg.StorageLocalDir, []byte(cfg.StorageLocalSecret), baseURL)
		store = local
		storageHandler = local
		logger.Info("object_storage_enabled", "driver", "local", "dir", cfg.StorageLocalDir)
	}
	documentsService := documents.NewService(pool, store)

	queue := jobs.NewQueue(pool)
	var integrationService *ingestion.Service
	if rawKey := os.Getenv("ATLAS_SECRET_KEY"); rawKey != "" {
		sealer, err := secrets.NewSealer(rawKey)
		if err != nil {
			return fmt.Errorf("configure secret key: %w", err)
		}
		registry := connectors.NewRegistry()
		registry.Register("mock", connectors.NewMock)
		registry.Register("document_inbox", connectors.NewDocumentInbox)
		integrationService = ingestion.NewService(pool, registry, sealer)
		logger.Info("integrations_enabled")
	} else {
		logger.Warn("integrations_disabled", "detail", "set ATLAS_SECRET_KEY to enable integration management")
	}

	router := httpapi.NewRouter(httpapi.Deps{
		Logger:         logger,
		Pool:           pool,
		Migrations:     migrationsFS,
		ServiceName:    cfg.ServiceName,
		Version:        cfg.Version,
		Identity:       sessions,
		DevSessions:    sessions,
		OIDC:           oidcService,
		OIDCEnabled:    cfg.OIDCEnabled,
		Invitations:    invitations,
		Documents:      documentsService,
		Integrations:   integrationService,
		Queue:          queue,
		Reconciliation: reconciliation.NewService(pool),
		StorageHandler: storageHandler,
		Authorizer:     authorization.NewService(),
		Orgs:           orgs.NewService(pool),
		DevAuthEnabled: cfg.DevAuthEnabled,
		SecureCookies:  cfg.Environment == "production",
	})
	if cfg.DevAuthEnabled {
		logger.Warn("development_authentication_enabled", "detail", "dev login is available; never enable this in staging or production")
	}

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("server_started", "addr", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
	}

	logger.Info("shutdown_started")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	logger.Info("shutdown_complete")
	return nil
}
