package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"atlas/internal/httpapi"
	"atlas/internal/platform/db"
	dbgen "atlas/internal/platform/db/gen"
	"atlas/internal/platform/migrate"
	"atlas/internal/testutil"
)

var testDatabaseURL string

func TestMain(m *testing.M) {
	databaseURL, err := testutil.DatabaseURL()
	if err != nil {
		_, _ = os.Stderr.WriteString("postgres unavailable: " + err.Error() + "\n")
		os.Exit(1)
	}
	testDatabaseURL = databaseURL
	code := m.Run()
	testutil.Stop()
	os.Exit(code)
}

func backendRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file location")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func migrationsFS(t *testing.T) fstest.MapFS {
	t.Helper()
	root := backendRoot(t)
	directory := filepath.Join(root, "migrations")
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read migrations directory: %v", err)
	}
	fsys := fstest.MapFS{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		fsys[entry.Name()] = &fstest.MapFile{Data: content}
	}
	return fsys
}

func newPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := db.Connect(ctx, testDatabaseURL, 5)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func resetSchema(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, `DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatalf("reset schema: %v", err)
	}
}

func TestMigrationLifecycle(t *testing.T) {
	pool := newPool(t)
	resetSchema(t, pool)
	fsys := migrationsFS(t)
	ctx := context.Background()

	pending, err := migrate.Pending(ctx, pool, fsys)
	if err != nil {
		t.Fatalf("pending: %v", err)
	}
	if pending != 15 {
		t.Fatalf("pending = %d, want 15", pending)
	}

	applied, err := migrate.Up(ctx, pool, fsys)
	if err != nil {
		t.Fatalf("up: %v", err)
	}
	if applied != 15 {
		t.Fatalf("applied = %d, want 15", applied)
	}

	status, err := migrate.Status(ctx, pool, fsys)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if len(status.Applied) != 15 || status.PendingCount != 0 || status.LatestApplied != "0015" {
		t.Fatalf("unexpected status: %+v", status)
	}

	var tablePresent bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('platform_metadata') IS NOT NULL`).Scan(&tablePresent); err != nil {
		t.Fatalf("table check: %v", err)
	}
	if !tablePresent {
		t.Fatal("platform_metadata table was not created")
	}

	reverted, err := migrate.Down(ctx, pool, fsys, 12)
	if err != nil {
		t.Fatalf("down: %v", err)
	}
	if reverted != 12 {
		t.Fatalf("reverted = %d, want 12", reverted)
	}
	if err := pool.QueryRow(ctx, `SELECT to_regclass('platform_metadata') IS NOT NULL`).Scan(&tablePresent); err != nil {
		t.Fatalf("table check after down: %v", err)
	}
	if !tablePresent {
		t.Fatal("platform_metadata table should remain after reverting 0002")
	}
	var organizationsPresent bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('organizations') IS NOT NULL`).Scan(&organizationsPresent); err != nil {
		t.Fatalf("organizations check after down: %v", err)
	}
	if !organizationsPresent {
		t.Fatal("organizations table should remain after reverting 0003")
	}
	var memberFunctionPresent bool
	if err := pool.QueryRow(ctx, `SELECT to_regprocedure('atlas_list_members()') IS NOT NULL`).Scan(&memberFunctionPresent); err != nil {
		t.Fatalf("member function check after down: %v", err)
	}
	if !memberFunctionPresent {
		t.Fatal("atlas_list_members should remain after reverting only 0004")
	}
	var invitationsPresent bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('organization_invitations') IS NOT NULL`).Scan(&invitationsPresent); err != nil {
		t.Fatalf("invitations check after down: %v", err)
	}
	if invitationsPresent {
		t.Fatal("organization_invitations should be removed by the 0004 down migration")
	}
	var membershipsFunctionPresent bool
	if err := pool.QueryRow(ctx, `SELECT to_regprocedure('atlas_memberships_for_user(uuid)') IS NOT NULL`).Scan(&membershipsFunctionPresent); err != nil {
		t.Fatalf("memberships function check after down: %v", err)
	}
	if membershipsFunctionPresent {
		t.Fatal("atlas_memberships_for_user should be removed by the 0004 down migration")
	}
	var jobsPresent, documentsPresent bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('jobs') IS NOT NULL`).Scan(&jobsPresent); err != nil {
		t.Fatalf("jobs check after down: %v", err)
	}
	if jobsPresent {
		t.Fatal("jobs should be removed by the 0005 down migration")
	}
	if err := pool.QueryRow(ctx, `SELECT to_regclass('documents') IS NOT NULL`).Scan(&documentsPresent); err != nil {
		t.Fatalf("documents check after down: %v", err)
	}
	if documentsPresent {
		t.Fatal("documents should be removed by the 0006 down migration")
	}

	if _, err := migrate.Up(ctx, pool, fsys); err != nil {
		t.Fatalf("re-apply: %v", err)
	}

	// Editing an applied migration must be detected.
	if _, err := pool.Exec(ctx, `UPDATE schema_migrations SET checksum = 'tampered' WHERE version = '0001'`); err != nil {
		t.Fatalf("tamper checksum: %v", err)
	}
	if _, err := migrate.Up(ctx, pool, fsys); err == nil {
		t.Fatal("expected a checksum mismatch error")
	}
}

func TestPlatformMetadataQueries(t *testing.T) {
	pool := newPool(t)
	resetSchema(t, pool)
	fsys := migrationsFS(t)
	if _, err := migrate.Up(context.Background(), pool, fsys); err != nil {
		t.Fatalf("up: %v", err)
	}

	queries := dbgen.New(pool)
	ctx := context.Background()
	if _, err := queries.UpsertPlatformMetadata(ctx, dbgen.UpsertPlatformMetadataParams{
		Key:   "schema_ready",
		Value: "true",
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	record, err := queries.GetPlatformMetadata(ctx, "schema_ready")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if record.Value != "true" {
		t.Fatalf("value = %q", record.Value)
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestHealthAndReadiness(t *testing.T) {
	pool := newPool(t)
	resetSchema(t, pool)
	fsys := migrationsFS(t)
	if _, err := migrate.Up(context.Background(), pool, fsys); err != nil {
		t.Fatalf("up: %v", err)
	}

	router := httpapi.NewRouter(httpapi.Deps{
		Logger:      testLogger(),
		Pool:        pool,
		Migrations:  fsys,
		ServiceName: "atlas-api",
		Version:     "test",
	})

	health := httptest.NewRecorder()
	router.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("health status = %d", health.Code)
	}
	if health.Header().Get("X-Request-Id") == "" {
		t.Fatal("missing X-Request-Id header")
	}
	var healthBody map[string]any
	if err := json.Unmarshal(health.Body.Bytes(), &healthBody); err != nil {
		t.Fatalf("decode health: %v", err)
	}
	if healthBody["status"] != "ok" {
		t.Fatalf("health body = %v", healthBody)
	}

	ready := httptest.NewRecorder()
	router.ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if ready.Code != http.StatusOK {
		t.Fatalf("ready status = %d, body = %s", ready.Code, ready.Body.String())
	}

	// A schema with an extra unapplied migration must fail readiness.
	pendingFS := migrationsFS(t)
	pendingFS["0016_extra.up.sql"] = &fstest.MapFile{Data: []byte("SELECT 1;")}
	pendingFS["0016_extra.down.sql"] = &fstest.MapFile{Data: []byte("SELECT 1;")}
	pendingRouter := httpapi.NewRouter(httpapi.Deps{
		Logger: testLogger(), Pool: pool, Migrations: pendingFS,
		ServiceName: "atlas-api", Version: "test",
	})
	pendingReady := httptest.NewRecorder()
	pendingRouter.ServeHTTP(pendingReady, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if pendingReady.Code != http.StatusServiceUnavailable {
		t.Fatalf("pending ready status = %d", pendingReady.Code)
	}
	if !bytes.Contains(pendingReady.Body.Bytes(), []byte("1 migration(s) pending")) {
		t.Fatalf("pending body = %s", pendingReady.Body.String())
	}
}

func TestReadinessFailsWhenDatabaseIsUnavailable(t *testing.T) {
	closedPool := newPool(t)
	closedPool.Close()

	router := httpapi.NewRouter(httpapi.Deps{
		Logger:      testLogger(),
		Pool:        closedPool,
		Migrations:  migrationsFS(t),
		ServiceName: "atlas-api",
		Version:     "test",
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte("database unreachable")) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestProblemResponses(t *testing.T) {
	router := httpapi.NewRouter(httpapi.Deps{
		Logger:      testLogger(),
		Migrations:  migrationsFS(t),
		ServiceName: "atlas-api",
		Version:     "test",
	})

	notFound := httptest.NewRecorder()
	router.ServeHTTP(notFound, httptest.NewRequest(http.MethodGet, "/missing", nil))
	if notFound.Code != http.StatusNotFound {
		t.Fatalf("404 status = %d", notFound.Code)
	}
	if contentType := notFound.Header().Get("Content-Type"); contentType != "application/problem+json; charset=utf-8" {
		t.Fatalf("content type = %q", contentType)
	}
	var problem map[string]any
	if err := json.Unmarshal(notFound.Body.Bytes(), &problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem["code"] != "not_found" {
		t.Fatalf("problem = %v", problem)
	}

	methodNotAllowed := httptest.NewRecorder()
	router.ServeHTTP(methodNotAllowed, httptest.NewRequest(http.MethodPost, "/healthz", nil))
	if methodNotAllowed.Code != http.StatusMethodNotAllowed {
		t.Fatalf("405 status = %d", methodNotAllowed.Code)
	}
}
