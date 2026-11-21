package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestMigrateCLI builds the atlas-migrate binary and exercises status, up and down.
func TestMigrateCLI(t *testing.T) {
	root := backendRoot(t)
	pool := newPool(t)
	resetSchema(t, pool)

	binary := filepath.Join(t.TempDir(), "atlas-migrate")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, "./cmd/atlas-migrate")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build atlas-migrate: %v\n%s", err, output)
	}

	run := func(args ...string) string {
		t.Helper()
		command := exec.Command(binary, args...)
		command.Env = append(os.Environ(),
			"DATABASE_URL="+testDatabaseURL,
			"ATLAS_ENV=test",
			"MIGRATIONS_DIR="+filepath.Join(root, "migrations"),
			"LOG_LEVEL=error",
		)
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("atlas-migrate %v: %v\n%s", args, err, output)
		}
		return string(output)
	}

	if output := run("status"); !strings.Contains(output, "pending: 11") {
		t.Fatalf("status before up = %q", output)
	}
	if output := run("up"); !strings.Contains(output, "applied 11 migration(s)") {
		t.Fatalf("up = %q", output)
	}
	if output := run("status"); !strings.Contains(output, "applied: 11") || !strings.Contains(output, "pending: 0") {
		t.Fatalf("status after up = %q", output)
	}
	if output := run("down", "-steps", "1"); !strings.Contains(output, "reverted 1 migration(s)") {
		t.Fatalf("down = %q", output)
	}
}
