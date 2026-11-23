// Command atlas-mcp serves the Atlas remote MCP endpoint over Streamable HTTP.
// Agents authenticate with Atlas-issued bearer tokens and receive the same
// governed data as the REST API.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"atlas/internal/agentaccess"
	"atlas/internal/authorization"
	"atlas/internal/canonical"
	"atlas/internal/governance"
	"atlas/internal/jobs"
	"atlas/internal/mcpapi"
	"atlas/internal/platform/config"
	"atlas/internal/platform/db"
	"atlas/internal/query"
	"atlas/internal/reconciliation"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "atlas-mcp:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.Environment == "production" {
		return fmt.Errorf("agent token issuance is not production-ready (ADR-002/ADR-004); refusing to serve production")
	}

	addr, err := mcpAddr()
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel})).
		With("service", "atlas-mcp")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL, 8)
	if err != nil {
		return err
	}
	defer pool.Close()

	queue := jobs.NewQueue(pool)
	canonicalService := canonical.NewService(pool)
	reconciliationService := reconciliation.NewService(pool)
	handler := mcpapi.NewHandler(mcpapi.Deps{
		Logger:     logger,
		Pool:       pool,
		Agents:     agentaccess.NewService(pool),
		Authorizer: authorization.NewService(),
		Query:      query.NewService(pool, canonicalService, reconciliationService),
		Governance: governance.NewService(pool, queue),
		Queue:      queue,
	})

	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("mcp_started", "addr", addr)
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
	logger.Info("mcp_stopped")
	return nil
}

func mcpAddr() (string, error) {
	raw := os.Getenv("MCP_PORT")
	if raw == "" {
		raw = os.Getenv("PORT")
	}
	if raw == "" {
		return ":8081", nil
	}
	port, err := strconv.Atoi(raw)
	if err != nil || port < 1 || port > 65535 {
		return "", fmt.Errorf("invalid MCP_PORT/PORT %q: must be an integer between 1 and 65535", raw)
	}
	return fmt.Sprintf(":%d", port), nil
}
