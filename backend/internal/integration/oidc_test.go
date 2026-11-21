package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"atlas/internal/identity"
)

type mockWorkOS struct {
	server   *httptest.Server
	userID   string
	email    string
	verified bool
}

func newMockWorkOS(t *testing.T, userID, email string, verified bool) *mockWorkOS {
	t.Helper()
	mock := &mockWorkOS{userID: userID, email: email, verified: verified}
	mock.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/authenticate" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user": map[string]any{
				"id": mock.userID, "email": mock.email,
				"first_name": "Test", "last_name": "User", "email_verified": mock.verified,
			},
			"organization_id": "",
		})
	}))
	t.Cleanup(mock.server.Close)
	return mock
}

func (m *mockWorkOS) config() identity.WorkOSConfig {
	return identity.WorkOSConfig{
		ClientID:        "client_test",
		APIKey:          "sk_test",
		RedirectURI:     "http://localhost:8080/v1/auth/oidc/callback",
		AuthorizeURL:    m.server.URL + "/authorize",
		AuthenticateURL: m.server.URL + "/authenticate",
	}
}

func oidcService(t *testing.T, auth *pgxpool.Pool, app *pgxpool.Pool, mock *mockWorkOS) *identity.OIDCService {
	t.Helper()
	sessions := identity.NewSessionService(auth, time.Hour)
	invitations := identity.NewInvitationService(app, 24*time.Hour)
	return identity.NewOIDCService(auth, sessions, identity.NewWorkOSClient(mock.config()), invitations)
}

func startOIDC(t *testing.T, service *identity.OIDCService, organizationID, email, linkUserID, invitationToken string) identity.OIDCStartResult {
	t.Helper()
	result, err := service.Start(organizationID, email, linkUserID, invitationToken)
	if err != nil {
		t.Fatalf("oidc start: %v", err)
	}
	if result.StateCookie == "" || !strings.Contains(result.AuthorizationURL, "state=") {
		t.Fatalf("start result incomplete: %+v", result)
	}
	return result
}

func TestOIDCInvitedUserLogin(t *testing.T) {
	admin, app, auth := setupTenantSchema(t)
	tenants := seedTenants(t, admin)
	ctx := context.Background()

	email := fmt.Sprintf("invited-%d@alpha.test", time.Now().UnixNano())
	invitations := identity.NewInvitationService(app, 24*time.Hour)
	_, invitationToken, err := invitations.Create(ctx, tenants.OrganizationA, email, "member", tenants.AdminUserA, "Ada Alpha")
	if err != nil {
		t.Fatalf("create invitation: %v", err)
	}

	mock := newMockWorkOS(t, "user_workos_invited", email, true)
	service := oidcService(t, auth, app, mock)
	start := startOIDC(t, service, tenants.OrganizationA, email, "", invitationToken)

	token, principal, _, err := service.Callback(ctx, "code_invited", start.StateCookie, "127.0.0.1", "test")
	if err != nil {
		t.Fatalf("callback: %v", err)
	}
	if principal.Role != "member" || principal.OrganizationID != tenants.OrganizationA {
		t.Fatalf("unexpected principal: %+v", principal)
	}

	sessions := identity.NewSessionService(auth, time.Hour)
	restored, err := sessions.Authenticate(ctx, token)
	if err != nil || restored.ID != principal.ID {
		t.Fatalf("session restore failed: %v (%+v)", err, restored)
	}

	var mapped int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM oidc_identities WHERE subject = 'user_workos_invited'`).Scan(&mapped); err != nil || mapped != 1 {
		t.Fatalf("identity mapping count = %d, err=%v", mapped, err)
	}
	var accepted int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM organization_invitations WHERE lower(email) = lower($1) AND accepted_at IS NOT NULL`, email).Scan(&accepted); err != nil || accepted != 1 {
		t.Fatalf("invitation not accepted: count=%d err=%v", accepted, err)
	}
}

func TestOIDCRequiresInvitation(t *testing.T) {
	admin, app, auth := setupTenantSchema(t)
	tenants := seedTenants(t, admin)
	ctx := context.Background()

	email := fmt.Sprintf("noinvite-%d@alpha.test", time.Now().UnixNano())
	mock := newMockWorkOS(t, "user_workos_noinvite", email, true)
	service := oidcService(t, auth, app, mock)
	start := startOIDC(t, service, tenants.OrganizationA, email, "", "")

	if _, _, _, err := service.Callback(ctx, "code", start.StateCookie, "127.0.0.1", "test"); err == nil {
		t.Fatal("expected invitation_required")
	}
	var users int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM users WHERE lower(email) = lower($1)`, email).Scan(&users); err != nil || users != 0 {
		t.Fatalf("user should not be created without an invitation: count=%d err=%v", users, err)
	}
}

func TestOIDCRejectsUnverifiedInvitationEmail(t *testing.T) {
	admin, app, auth := setupTenantSchema(t)
	tenants := seedTenants(t, admin)
	ctx := context.Background()

	email := fmt.Sprintf("unverified-%d@alpha.test", time.Now().UnixNano())
	invitations := identity.NewInvitationService(app, 24*time.Hour)
	if _, invitationToken, err := invitations.Create(ctx, tenants.OrganizationA, email, "member", tenants.AdminUserA, "Ada Alpha"); err != nil {
		t.Fatalf("create invitation: %v", err)
	} else {
		_ = invitationToken
	}
	mock := newMockWorkOS(t, "user_workos_unverified", email, false)
	service := oidcService(t, auth, app, mock)
	start := startOIDC(t, service, tenants.OrganizationA, email, "", "")

	if _, _, _, err := service.Callback(ctx, "code", start.StateCookie, "127.0.0.1", "test"); err == nil {
		t.Fatal("expected email_not_verified")
	}
}

func TestOIDCExistingEmailRequiresExplicitLinking(t *testing.T) {
	admin, app, auth := setupTenantSchema(t)
	tenants := seedTenants(t, admin)
	ctx := context.Background()

	mock := newMockWorkOS(t, "user_workos_same_email", tenants.AdminEmailA, true)
	service := oidcService(t, auth, app, mock)
	start := startOIDC(t, service, tenants.OrganizationA, tenants.AdminEmailA, "", "")

	if _, _, _, err := service.Callback(ctx, "code", start.StateCookie, "127.0.0.1", "test"); err == nil {
		t.Fatal("expected account_linking_required for an existing Atlas email")
	}
	var identities int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM oidc_identities WHERE subject = 'user_workos_same_email'`).Scan(&identities); err != nil || identities != 0 {
		t.Fatalf("identity must not be auto-linked: count=%d err=%v", identities, err)
	}
}

func TestOIDCExplicitLinkingFlow(t *testing.T) {
	admin, app, auth := setupTenantSchema(t)
	tenants := seedTenants(t, admin)
	ctx := context.Background()

	mock := newMockWorkOS(t, "user_workos_link", "linked@alpha.test", true)
	service := oidcService(t, auth, app, mock)
	start := startOIDC(t, service, tenants.OrganizationA, "linked@alpha.test", tenants.AdminUserA, "")

	_, principal, _, err := service.Callback(ctx, "code_link", start.StateCookie, "127.0.0.1", "test")
	if err != nil {
		t.Fatalf("link callback: %v", err)
	}
	if principal.ID != tenants.AdminUserA {
		t.Fatalf("linked principal = %s, want %s", principal.ID, tenants.AdminUserA)
	}
	var mappedUser string
	if err := admin.QueryRow(ctx, `SELECT user_id FROM oidc_identities WHERE subject = 'user_workos_link'`).Scan(&mappedUser); err != nil || mappedUser != tenants.AdminUserA {
		t.Fatalf("identity link = %q, err=%v", mappedUser, err)
	}
}

func TestOIDCRejectsTamperedState(t *testing.T) {
	admin, app, auth := setupTenantSchema(t)
	tenants := seedTenants(t, admin)
	mock := newMockWorkOS(t, "user_workos_tamper", "tamper@alpha.test", true)
	service := oidcService(t, auth, app, mock)
	start := startOIDC(t, service, tenants.OrganizationA, "tamper@alpha.test", "", "")

	tampered := start.StateCookie + "x"
	if _, _, _, err := service.Callback(context.Background(), "code", tampered, "127.0.0.1", "test"); err == nil {
		t.Fatal("tampered state must be rejected")
	}
}

func TestOIDCStartDisabledWhenNotConfigured(t *testing.T) {
	admin, app, auth := setupTenantSchema(t)
	_ = admin
	router := identityRouter(t, app, auth)
	response := request(t, router, http.MethodGet, "/v1/auth/oidc/start", nil, nil)
	if response.Code != http.StatusNotFound {
		t.Fatalf("oidc start without configuration = %d, want 404", response.Code)
	}
}
