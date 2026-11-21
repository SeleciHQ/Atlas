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
	"atlas/internal/httpapi"
	"atlas/internal/identity"
	"atlas/internal/orgs"
	"atlas/internal/platform/config"
	"atlas/internal/platform/db"
	"atlas/internal/platform/logging"
	"atlas/internal/platform/migrate"
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
