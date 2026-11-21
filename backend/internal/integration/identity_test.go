package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"atlas/internal/authorization"
	"atlas/internal/httpapi"
	"atlas/internal/identity"
	"atlas/internal/orgs"
	"atlas/internal/tenancy"
)

func identityRouter(t *testing.T, appPool *pgxpool.Pool, authPool *pgxpool.Pool) http.Handler {
	t.Helper()
	return httpapi.NewRouter(httpapi.Deps{
		Logger:         testLogger(),
		Pool:           appPool,
		Migrations:     migrationsFS(t),
		ServiceName:    "atlas-api",
		Version:        "test",
		Identity:       identity.NewSessionService(authPool, time.Hour),
		DevSessions:    identity.NewSessionService(authPool, time.Hour),
		Authorizer:     authorization.NewService(),
		Orgs:           orgs.NewService(appPool),
		DevAuthEnabled: true,
	})
}

func request(t *testing.T, handler http.Handler, method, path string, cookie *http.Cookie, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
		reader = bytes.NewReader(encoded)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}

func login(t *testing.T, handler http.Handler, organizationID, email string) *http.Cookie {
	t.Helper()
	response := request(t, handler, http.MethodPost, "/v1/auth/dev-login", nil, map[string]string{
		"organization_id": organizationID,
		"email":           email,
	})
	if response.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", response.Code, response.Body.String())
	}
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == httpapi.SessionCookieName {
			return cookie
		}
	}
	t.Fatal("login did not set a session cookie")
	return nil
}

func decodeBody(t *testing.T, response *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), target); err != nil {
		t.Fatalf("decode body %q: %v", response.Body.String(), err)
	}
}

func TestRLSContextIsolationUnderConcurrency(t *testing.T) {
	admin, app, _ := setupTenantSchema(t)
	tenants := seedTenants(t, admin)
	ctx := context.Background()

	const workers = 24
	errors := make(chan error, workers)
	var group sync.WaitGroup
	for i := 0; i < workers; i++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			organizationID := tenants.OrganizationA
			expected := 2
			if index%2 == 1 {
				organizationID = tenants.OrganizationB
				expected = 1
			}
			err := tenancy.WithOrganizationTx(ctx, app, organizationID, func(tx pgx.Tx) error {
				var count int
				if err := tx.QueryRow(ctx, `SELECT count(*) FROM memberships`).Scan(&count); err != nil {
					return err
				}
				if count != expected {
					return fmt.Errorf("organization %s saw %d memberships, want %d", organizationID, count, expected)
				}
				return nil
			})
			errors <- err
		}(i)
	}
	group.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("concurrent tenant context leaked: %v", err)
		}
	}
}

func TestAuthenticationAndAuthorization(t *testing.T) {
	admin, app, auth := setupTenantSchema(t)
	tenants := seedTenants(t, admin)
	router := identityRouter(t, app, auth)

	if response := request(t, router, http.MethodGet, "/v1/auth/me", nil, nil); response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated me status = %d", response.Code)
	}
	bogusCookie := &http.Cookie{Name: httpapi.SessionCookieName, Value: "bogus-session-token"}
	if response := request(t, router, http.MethodGet, "/v1/auth/me", bogusCookie, nil); response.Code != http.StatusUnauthorized {
		t.Fatalf("bogus session status = %d, want 401", response.Code)
	}

	adminCookie := login(t, router, tenants.OrganizationA, tenants.AdminEmailA)

	me := request(t, router, http.MethodGet, "/v1/auth/me", adminCookie, nil)
	if me.Code != http.StatusOK {
		t.Fatalf("me status = %d, body = %s", me.Code, me.Body.String())
	}
	var principal identity.Principal
	decodeBody(t, me, &principal)
	if principal.Kind != identity.KindHuman || principal.Role != identity.RoleAdmin || principal.OrganizationID != tenants.OrganizationA {
		t.Fatalf("unexpected principal: %+v", principal)
	}

	members := request(t, router, http.MethodGet, "/v1/members", adminCookie, nil)
	if members.Code != http.StatusOK {
		t.Fatalf("members status = %d, body = %s", members.Code, members.Body.String())
	}
	var membersBody struct {
		Members []orgs.Member `json:"members"`
	}
	decodeBody(t, members, &membersBody)
	if len(membersBody.Members) != 2 {
		t.Fatalf("members = %d, want 2 (organization A only)", len(membersBody.Members))
	}
	for _, member := range membersBody.Members {
		if member.Email == tenants.AdminEmailB {
			t.Fatal("members endpoint leaked another organization's member")
		}
	}

	settings := request(t, router, http.MethodGet, "/v1/organization/settings", adminCookie, nil)
	if settings.Code != http.StatusOK {
		t.Fatalf("admin settings status = %d, body = %s", settings.Code, settings.Body.String())
	}

	memberCookie := login(t, router, tenants.OrganizationA, tenants.MemberEmailA)
	memberMembers := request(t, router, http.MethodGet, "/v1/members", memberCookie, nil)
	if memberMembers.Code != http.StatusOK {
		t.Fatalf("member members status = %d", memberMembers.Code)
	}
	memberSettings := request(t, router, http.MethodGet, "/v1/organization/settings", memberCookie, nil)
	if memberSettings.Code != http.StatusForbidden {
		t.Fatalf("member settings status = %d, want 403", memberSettings.Code)
	}

	logout := request(t, router, http.MethodPost, "/v1/auth/logout", adminCookie, nil)
	if logout.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d", logout.Code)
	}
	if response := request(t, router, http.MethodGet, "/v1/auth/me", adminCookie, nil); response.Code != http.StatusUnauthorized {
		t.Fatalf("me after logout status = %d", response.Code)
	}
}

func TestCrossTenantLoginRejected(t *testing.T) {
	admin, app, auth := setupTenantSchema(t)
	tenants := seedTenants(t, admin)
	router := identityRouter(t, app, auth)

	// A user from organization A must not obtain a session for organization B.
	response := request(t, router, http.MethodPost, "/v1/auth/dev-login", nil, map[string]string{
		"organization_id": tenants.OrganizationB,
		"email":           tenants.AdminEmailA,
	})
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("cross-tenant login status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestRLSTenantIsolation(t *testing.T) {
	admin, app, _ := setupTenantSchema(t)
	tenants := seedTenants(t, admin)
	ctx := context.Background()

	var count int
	if err := app.QueryRow(ctx, `SELECT count(*) FROM memberships`).Scan(&count); err != nil {
		t.Fatalf("count memberships without context: %v", err)
	}
	if count != 0 {
		t.Fatalf("memberships visible without organization context = %d, want 0", count)
	}

	err := tenancy.WithOrganizationTx(ctx, app, tenants.OrganizationA, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM memberships`).Scan(&count); err != nil {
			return err
		}
		if count != 2 {
			t.Fatalf("organization A memberships = %d, want 2", count)
		}
		var otherOrganizationCount int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM memberships WHERE organization_id = $1`, tenants.OrganizationB).Scan(&otherOrganizationCount); err != nil {
			return err
		}
		if otherOrganizationCount != 0 {
			t.Fatalf("organization B memberships visible under A context = %d, want 0", otherOrganizationCount)
		}
		var organizationCount int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM organizations`).Scan(&organizationCount); err != nil {
			return err
		}
		if organizationCount != 1 {
			t.Fatalf("organizations visible = %d, want 1", organizationCount)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("organization scoped transaction: %v", err)
	}

	// Cross-tenant writes must be rejected by the RLS WITH CHECK clause.
	err = tenancy.WithOrganizationTx(ctx, app, tenants.OrganizationA, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO memberships (organization_id, user_id, role) VALUES ($1, $2, 'member')`,
			tenants.OrganizationB, tenants.AdminUserA)
		return err
	})
	if err == nil {
		t.Fatal("cross-organization membership insert should be rejected")
	}

	// Agent connections and integrations are tenant-scoped as well.
	if _, err := admin.Exec(ctx, `INSERT INTO agent_connections (organization_id, name) VALUES ($1, 'Claude Cowork'), ($2, 'Custom agent')`,
		tenants.OrganizationA, tenants.OrganizationB); err != nil {
		t.Fatalf("insert agent connections: %v", err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO integrations (organization_id, provider, name) VALUES ($1, 'mock-erp', 'Mock ERP'), ($2, 'mock-erp', 'Mock ERP')`,
		tenants.OrganizationA, tenants.OrganizationB); err != nil {
		t.Fatalf("insert integrations: %v", err)
	}
	err = tenancy.WithOrganizationTx(ctx, app, tenants.OrganizationA, func(tx pgx.Tx) error {
		var connections int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM agent_connections`).Scan(&connections); err != nil {
			return err
		}
		if connections != 1 {
			t.Fatalf("agent connections visible = %d, want 1", connections)
		}
		var integrations int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM integrations`).Scan(&integrations); err != nil {
			return err
		}
		if integrations != 1 {
			t.Fatalf("integrations visible = %d, want 1", integrations)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tenant-scoped agent/integration read: %v", err)
	}
}

func TestAuditAttributionAndImmutability(t *testing.T) {
	admin, app, auth := setupTenantSchema(t)
	tenants := seedTenants(t, admin)
	router := identityRouter(t, app, auth)

	cookie := login(t, router, tenants.OrganizationA, tenants.AdminEmailA)
	if cookie == nil {
		t.Fatal("login failed")
	}

	ctx := context.Background()
	var actorKind, actorID, organizationID, action, decision string
	if err := admin.QueryRow(ctx, `
		SELECT actor_kind, actor_id, organization_id, action, decision
		FROM audit_events ORDER BY created_at DESC LIMIT 1`).
		Scan(&actorKind, &actorID, &organizationID, &action, &decision); err != nil {
		t.Fatalf("read audit event: %v", err)
	}
	if actorKind != string(identity.KindHuman) || actorID != tenants.AdminUserA ||
		organizationID != tenants.OrganizationA || action != "auth.dev_login" || decision != "allow" {
		t.Fatalf("unexpected audit attribution: kind=%s id=%s org=%s action=%s decision=%s",
			actorKind, actorID, organizationID, action, decision)
	}

	var visibleWithoutContext int
	if err := app.QueryRow(ctx, `SELECT count(*) FROM audit_events`).Scan(&visibleWithoutContext); err != nil {
		t.Fatalf("audit count without context: %v", err)
	}
	if visibleWithoutContext != 0 {
		t.Fatalf("audit events visible without organization context = %d, want 0", visibleWithoutContext)
	}

	// The application role must not be able to modify or delete audit rows.
	err := tenancy.WithOrganizationTx(ctx, app, tenants.OrganizationA, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE audit_events SET decision = 'deny'`)
		return err
	})
	if err == nil {
		t.Fatal("audit UPDATE should be denied for the application role")
	}
	err = tenancy.WithOrganizationTx(ctx, app, tenants.OrganizationA, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM audit_events`)
		return err
	})
	if err == nil {
		t.Fatal("audit DELETE should be denied for the application role")
	}
}

func TestExpiredSessionRejected(t *testing.T) {
	admin, app, auth := setupTenantSchema(t)
	tenants := seedTenants(t, admin)
	router := identityRouter(t, app, auth)

	cookie := login(t, router, tenants.OrganizationA, tenants.AdminEmailA)
	hash := identity.HashSessionToken(cookie.Value)
	if _, err := admin.Exec(context.Background(),
		`UPDATE sessions SET expires_at = now() - interval '1 hour' WHERE token_hash = $1`, hash); err != nil {
		t.Fatalf("expire session: %v", err)
	}

	response := request(t, router, http.MethodGet, "/v1/auth/me", cookie, nil)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expired session status = %d, want 401", response.Code)
	}
}

func TestDevelopmentLoginDisabled(t *testing.T) {
	admin, app, auth := setupTenantSchema(t)
	tenants := seedTenants(t, admin)

	router := httpapi.NewRouter(httpapi.Deps{
		Logger:         testLogger(),
		Pool:           app,
		Migrations:     migrationsFS(t),
		ServiceName:    "atlas-api",
		Version:        "test",
		Identity:       identity.NewSessionService(auth, time.Hour),
		DevSessions:    identity.NewSessionService(auth, time.Hour),
		Authorizer:     authorization.NewService(),
		Orgs:           orgs.NewService(app),
		DevAuthEnabled: false,
	})

	response := request(t, router, http.MethodPost, "/v1/auth/dev-login", nil, map[string]string{
		"organization_id": tenants.OrganizationA,
		"email":           tenants.AdminEmailA,
	})
	if response.Code != http.StatusNotFound {
		t.Fatalf("disabled dev login status = %d, want 404", response.Code)
	}
}
