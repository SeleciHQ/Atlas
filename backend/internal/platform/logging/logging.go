// Package logging constructs Atlas's structured logger.
package logging

import (
	"log/slog"
	"os"

	"atlas/internal/platform/config"
)

// New returns a JSON logger with service-level attributes.
func New(cfg config.Config) *slog.Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel})
	return slog.New(handler).With(
		"service", cfg.ServiceName,
		"env", cfg.Environment,
		"version", cfg.Version,
	)
}
