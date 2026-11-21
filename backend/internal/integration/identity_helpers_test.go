package integration

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"atlas/internal/platform/db"
	"atlas/internal/platform/migrate"
)

const (
	appRole      = "atlas_app"
	authRole     = "atlas_auth"
	workerRole   = "atlas_worker"
	rolePassword = "atlas_role_password"
)

// setupTenantSchema resets the database, applies all migrations, executes the
// real provisioning SQL, and returns privileged, runtime, and identity pools.
func setupTenantSchema(t *testing.T) (admin *pgxpool.Pool, app *pgxpool.Pool, auth *pgxpool.Pool) {
	t.Helper()
	admin = newPool(t)
	resetSchema(t, admin)
	if _, err := migrate.Up(context.Background(), admin, migrationsFS(t)); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	executeProvisioningSQL(t, admin)
	return admin, connectRole(t, appRole), connectRole(t, authRole)
}

// executeProvisioningSQL runs scripts/provision-app-role.sql exactly as
// shipped, with passwords supplied through session settings.
func executeProvisioningSQL(t *testing.T, admin *pgxpool.Pool) {
	t.Helper()
	scriptPath := filepath.Join(backendRoot(t), "scripts", "provision-app-role.sql")
	script, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read provisioning script: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	conn, err := admin.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire admin connection: %v", err)
	}
	defer conn.Release()

	settings := map[string]string{
		"atlas.app_password":      rolePassword,
		"atlas.auth_password":     rolePassword,
		"atlas.migrator_password": rolePassword,
		"atlas.worker_password":   rolePassword,
	}
	for setting, value := range settings {
		if _, err := conn.Exec(ctx, `SELECT set_config($1, $2, false)`, setting, value); err != nil {
			t.Fatalf("set %s: %v", setting, err)
		}
	}
	if _, err := conn.Exec(ctx, string(script), pgx.QueryExecModeSimpleProtocol); err != nil {
		t.Fatalf("execute provisioning SQL: %v", err)
	}
}

func roleDatabaseURL(t *testing.T, adminURL, role string) string {
	t.Helper()
	parsed, err := url.Parse(adminURL)
	if err != nil {
		t.Fatalf("parse database url: %v", err)
	}
	parsed.User = url.UserPassword(role, rolePassword)
	return parsed.String()
}

func connectRole(t *testing.T, role string) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := db.Connect(ctx, roleDatabaseURL(t, testDatabaseURL, role), 5)
	if err != nil {
		t.Fatalf("connect as %s: %v", role, err)
	}
	t.Cleanup(pool.Close)
	return pool
}

type seededTenants struct {
	OrganizationA string
	OrganizationB string
	AdminEmailA   string
	MemberEmailA  string
	AdminEmailB   string
	AdminUserA    string
	MemberUserA   string
	AdminUserB    string
}

func seedTenants(t *testing.T, admin *pgxpool.Pool) seededTenants {
	t.Helper()
	ctx := context.Background()

	insertOrganization := func(name string) string {
		var id string
		if err := admin.QueryRow(ctx, `INSERT INTO organizations (name) VALUES ($1) RETURNING id`, name).Scan(&id); err != nil {
			t.Fatalf("insert organization: %v", err)
		}
		return id
	}
	insertUser := func(email, displayName string) string {
		var id string
		if err := admin.QueryRow(ctx, `INSERT INTO users (email, display_name) VALUES ($1, $2) RETURNING id`, email, displayName).Scan(&id); err != nil {
			t.Fatalf("insert user: %v", err)
		}
		return id
	}
	insertMembership := func(organizationID, userID, role string) {
		if _, err := admin.Exec(ctx, `INSERT INTO memberships (organization_id, user_id, role) VALUES ($1, $2, $3)`, organizationID, userID, role); err != nil {
			t.Fatalf("insert membership: %v", err)
		}
	}

	orgA := insertOrganization("Alpha Logistics")
	orgB := insertOrganization("Beta Manufacturing")

	adminEmailA := fmt.Sprintf("admin-%d@alpha.test", time.Now().UnixNano())
	memberEmailA := fmt.Sprintf("member-%d@alpha.test", time.Now().UnixNano())
	adminEmailB := fmt.Sprintf("admin-%d@beta.test", time.Now().UnixNano())

	adminUserA := insertUser(adminEmailA, "Ada Alpha")
	memberUserA := insertUser(memberEmailA, "Mia Member")
	adminUserB := insertUser(adminEmailB, "Bea Beta")

	insertMembership(orgA, adminUserA, "admin")
	insertMembership(orgA, memberUserA, "member")
	insertMembership(orgB, adminUserB, "admin")

	return seededTenants{
		OrganizationA: orgA,
		OrganizationB: orgB,
		AdminEmailA:   adminEmailA,
		MemberEmailA:  memberEmailA,
		AdminEmailB:   adminEmailB,
		AdminUserA:    adminUserA,
		MemberUserA:   memberUserA,
		AdminUserB:    adminUserB,
	}
}
