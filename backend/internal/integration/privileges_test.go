package integration

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"atlas/internal/tenancy"
)

// TestProvisioningSQLRolePrivileges verifies the actual provisioning SQL:
// role attributes, ownership, and the tenant/identity privilege split.
func TestProvisioningSQLRolePrivileges(t *testing.T) {
	admin, app, auth := setupTenantSchema(t)
	tenants := seedTenants(t, admin)
	ctx := context.Background()

	for _, role := range []string{appRole, authRole, workerRole} {
		var superuser, bypassRLS bool
		if err := admin.QueryRow(ctx, `SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname = $1`, role).
			Scan(&superuser, &bypassRLS); err != nil {
			t.Fatalf("read role %s: %v", role, err)
		}
		if superuser || bypassRLS {
			t.Fatalf("role %s must be NOSUPERUSER and NOBYPASSRLS", role)
		}
	}

	var nonMigratorOwners int
	if err := admin.QueryRow(ctx,
		`SELECT count(*) FROM pg_tables WHERE schemaname = 'public' AND tableowner <> 'atlas_migrator'`).
		Scan(&nonMigratorOwners); err != nil {
		t.Fatalf("table ownership: %v", err)
	}
	if nonMigratorOwners != 0 {
		t.Fatalf("%d tables are not owned by atlas_migrator", nonMigratorOwners)
	}

	var nonMigratorFunctions int
	if err := admin.QueryRow(ctx, `
		SELECT count(*) FROM pg_proc p
		JOIN pg_namespace n ON n.oid = p.pronamespace
		WHERE n.nspname = 'public' AND p.proname LIKE 'atlas_%'
		  AND pg_get_userbyid(p.proowner) <> 'atlas_migrator'`).Scan(&nonMigratorFunctions); err != nil {
		t.Fatalf("function ownership: %v", err)
	}
	if nonMigratorFunctions != 0 {
		t.Fatalf("%d Atlas functions are not owned by atlas_migrator", nonMigratorFunctions)
	}

	// The runtime role must not read global identities or sessions directly.
	if _, err := app.Exec(ctx, `SELECT count(*) FROM users`); err == nil {
		t.Fatal("atlas_app must not have direct access to users")
	}
	if _, err := app.Exec(ctx, `SELECT count(*) FROM sessions`); err == nil {
		t.Fatal("atlas_app must not have direct access to sessions")
	}

	// Member visibility goes through the security-definer function and is
	// scoped to the current organization context.
	var memberCount int
	if err := tenancy.WithOrganizationTx(ctx, app, tenants.OrganizationA, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM atlas_list_members()`).Scan(&memberCount)
	}); err != nil {
		t.Fatalf("list members as atlas_app: %v", err)
	}
	if memberCount != 2 {
		t.Fatalf("member count = %d, want 2", memberCount)
	}
	if err := app.QueryRow(ctx, `SELECT count(*) FROM atlas_list_members()`).Scan(&memberCount); err != nil {
		t.Fatalf("list members without context: %v", err)
	}
	if memberCount != 0 {
		t.Fatalf("members visible without context = %d, want 0", memberCount)
	}

	// The auth role is the narrow identity path: it may read identity data but
	// may not mutate tenant membership.
	var userCount int
	if err := auth.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&userCount); err != nil {
		t.Fatalf("auth role should read users: %v", err)
	}
	if userCount != 3 {
		t.Fatalf("auth role user count = %d, want 3", userCount)
	}
	if _, err := auth.Exec(ctx,
		`INSERT INTO memberships (organization_id, user_id, role) VALUES ($1, $2, 'member')`,
		tenants.OrganizationB, tenants.AdminUserA); err == nil {
		t.Fatal("auth role must not mutate memberships")
	}
	if err := tenancy.WithOrganizationTx(ctx, auth, tenants.OrganizationA, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE audit_events SET decision = 'deny'`)
		return err
	}); err == nil {
		t.Fatal("auth role must not update audit events")
	}

	worker := connectRole(t, workerRole)
	if _, err := worker.Exec(ctx, `SELECT count(*) FROM users`); err == nil {
		t.Fatal("atlas_worker must not have direct access to users")
	}
	if _, err := worker.Exec(ctx, `SELECT count(*) FROM sessions`); err == nil {
		t.Fatal("atlas_worker must not have direct access to sessions")
	}
	err := tenancy.WithOrganizationTx(ctx, worker, tenants.OrganizationA, func(tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM memberships`).Scan(&count); err != nil {
			return err
		}
		if count != 2 {
			t.Fatalf("worker membership count = %d, want 2", count)
		}
		_, err := tx.Exec(ctx, `UPDATE audit_events SET decision = 'deny'`)
		return err
	})
	if err == nil {
		t.Fatal("atlas_worker must not update audit events")
	}
}
