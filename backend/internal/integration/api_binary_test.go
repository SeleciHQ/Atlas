package integration

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"atlas/internal/platform/migrate"
)

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate port: %v", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

// TestAPIBinaryStartsAndServes builds the real atlas-api binary, starts it
// against the test database and verifies liveness and readiness over HTTP.
func TestAPIBinaryStartsAndServes(t *testing.T) {
	root := backendRoot(t)
	pool := newPool(t)
	resetSchema(t, pool)
	if _, err := migrate.Up(context.Background(), pool, migrationsFS(t)); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	binary := filepath.Join(t.TempDir(), "atlas-api")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, "./cmd/atlas-api")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build atlas-api: %v\n%s", err, output)
	}

	port := freePort(t)
	process := exec.Command(binary)
	process.Env = append(os.Environ(),
		"DATABASE_URL="+testDatabaseURL,
		"PORT="+strconv.Itoa(port),
		"ATLAS_ENV=test",
		"MIGRATIONS_DIR="+filepath.Join(root, "migrations"),
		"LOG_LEVEL=error",
	)
	var logs bytes.Buffer
	process.Stdout = &logs
	process.Stderr = &logs
	if err := process.Start(); err != nil {
		t.Fatalf("start atlas-api: %v", err)
	}
	t.Cleanup(func() {
		_ = process.Process.Kill()
		_, _ = process.Process.Wait()
	})

	client := &http.Client{Timeout: 2 * time.Second}
	baseURL := "http://127.0.0.1:" + strconv.Itoa(port)
	deadline := time.Now().Add(20 * time.Second)
	for {
		response, err := client.Get(baseURL + "/healthz")
		if err == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("atlas-api did not become healthy: %v\nlogs: %s", err, logs.String())
		}
		time.Sleep(200 * time.Millisecond)
	}

	response, err := client.Get(baseURL + "/readyz")
	if err != nil {
		t.Fatalf("readyz: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("readyz status = %d\nlogs: %s", response.StatusCode, logs.String())
	}
}
