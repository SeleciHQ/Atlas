// Package testutil starts a disposable PostgreSQL instance for integration
// tests. Set TEST_DATABASE_URL to use an existing dedicated database instead.
package testutil

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
)

var (
	once     sync.Once
	instance *embeddedpostgres.EmbeddedPostgres
	url      string
	startErr error
	dataDir  string
)

// DatabaseURL returns a database URL for integration tests.
func DatabaseURL() (string, error) {
	if fromEnv := os.Getenv("TEST_DATABASE_URL"); fromEnv != "" {
		return fromEnv, nil
	}

	once.Do(func() {
		port, err := freePort()
		if err != nil {
			startErr = fmt.Errorf("allocate postgres port: %w", err)
			return
		}
		dataDir, startErr = os.MkdirTemp("", "atlas-embedded-pg-*")
		if startErr != nil {
			return
		}

		config := embeddedpostgres.DefaultConfig().
			Username("atlas").
			Password("atlas").
			Database("atlas_test").
			Version(embeddedpostgres.V16).
			Port(uint32(port)).
			DataPath(filepath.Join(dataDir, "data")).
			RuntimePath(filepath.Join(dataDir, "runtime")).
			Logger(io.Discard).
			StartTimeout(120 * time.Second)

		instance = embeddedpostgres.NewDatabase(config)
		if err := instance.Start(); err != nil {
			startErr = fmt.Errorf("start embedded postgres: %w", err)
			return
		}
		url = fmt.Sprintf("postgres://atlas:atlas@127.0.0.1:%d/atlas_test?sslmode=disable", port)
	})

	return url, startErr
}

// Stop shuts down the embedded instance, if one was started.
func Stop() {
	if instance != nil {
		_ = instance.Stop()
		instance = nil
	}
	if dataDir != "" {
		_ = os.RemoveAll(dataDir)
		dataDir = ""
	}
}

func freePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}
