// Package version exposes build metadata injected at build time.
package version

// These values are overridden with -ldflags at build time.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)
