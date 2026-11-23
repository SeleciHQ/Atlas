package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"atlas/internal/agentaccess"
	"atlas/internal/authorization"
	"atlas/internal/httpapi"
	"atlas/internal/identity"
	"atlas/internal/orgs"
)

func TestAgentConnectionsAndTokens(t *testing.T) {
	admin, app, _ := setupTenantSchema(t)
	organizationA := seedQueueOrganization(t, admin)
	organizationB := seedQueueOrganization(t, admin)
	ctx := context.Background()
	service := agentaccess.NewService(app)

	connection, err := service.CreateConnection(ctx, organizationA, "research-agent",
		[]string{agentaccess.ScopeInvoicesRead, agentaccess.ScopeReconciliationRead}, "")
	if err != nil {
		t.Fatalf("create connection: %v", err)
	}
	if connection.Status != "active" || len(connection.Scopes) != 2 {
		t.Fatalf("connection = %+v", connection)
	}
	if _, err := service.CreateConnection(ctx, organizationA, "research-agent",
		[]string{agentaccess.ScopeInvoicesRead}, ""); err == nil {
		t.Fatal("duplicate connection name was accepted")
	}
	if _, err := service.CreateConnection(ctx, organizationA, "invalid-agent",
		[]string{"*"}, ""); err == nil {
		t.Fatal("wildcard scope was accepted")
	}
	connectionsA, err := service.ListConnections(ctx, organizationA)
	if err != nil || len(connectionsA) != 1 || connectionsA[0].ID != connection.ID {
		t.Fatalf("connections A = %+v %v", connectionsA, err)
	}
	connectionsB, err := service.ListConnections(ctx, organizationB)
	if err != nil || len(connectionsB) != 0 {
		t.Fatalf("connections B = %+v %v", connectionsB, err)
	}

	expired, err := service.IssueToken(ctx, organizationA, connection.ID, "", time.Hour)
	if err != nil {
		t.Fatalf("issue expired token: %v", err)
	}
	if _, err := admin.Exec(ctx,
		`UPDATE agent_tokens SET expires_at = now() - interval '1 hour' WHERE id = $1`, expired.TokenID); err != nil {
		t.Fatalf("expire token: %v", err)
	}
	if _, err := service.AuthenticateToken(ctx, expired.Token); err == nil {
		t.Fatal("expired token authenticated")
	}

	first, err := service.IssueToken(ctx, organizationA, connection.ID, "", time.Hour)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	principal, err := service.AuthenticateToken(ctx, first.Token)
	if err != nil {
		t.Fatalf("authenticate token: %v", err)
	}
	if principal.Kind != identity.KindAgent || principal.OrganizationID != organizationA ||
		principal.ID != connection.ID || len(principal.Scopes) != 2 {
		t.Fatalf("principal = %+v", principal)
	}
	if _, err := service.AuthenticateToken(ctx, "not-a-token"); err == nil {
		t.Fatal("invalid token authenticated")
	}

	if err := service.RevokeToken(ctx, organizationA, connection.ID, first.TokenID, ""); err != nil {
		t.Fatalf("revoke token: %v", err)
	}
	if _, err := service.AuthenticateToken(ctx, first.Token); err == nil {
		t.Fatal("revoked token authenticated")
	}

	last, err := service.IssueToken(ctx, organizationA, connection.ID, "", time.Hour)
	if err != nil {
		t.Fatalf("issue last token: %v", err)
	}
	if err := service.RevokeConnection(ctx, organizationA, connection.ID, ""); err != nil {
		t.Fatalf("revoke connection: %v", err)
	}
	if _, err := service.AuthenticateToken(ctx, last.Token); err == nil {
		t.Fatal("token for revoked connection authenticated")
	}
	if _, err := service.IssueToken(ctx, organizationA, connection.ID, "", time.Hour); err == nil {
		t.Fatal("token issued for revoked connection")
	}
	revoked, err := service.ListConnections(ctx, organizationA)
	if err != nil || len(revoked) != 1 || revoked[0].Status != "revoked" || revoked[0].RevokedAt == nil {
		t.Fatalf("revoked connection = %+v %v", revoked, err)
	}

	var audited int
	if err := admin.QueryRow(ctx, `
		SELECT count(*) FROM audit_events
		WHERE organization_id = $1 AND action LIKE 'agent.%'`, organizationA).Scan(&audited); err != nil {
		t.Fatalf("count audit: %v", err)
	}
	if audited < 6 {
		t.Fatalf("agent audit rows = %d, want at least 6", audited)
	}
}

func TestAgentBearerAuthenticationEndpoints(t *testing.T) {
	admin, app, auth := setupTenantSchema(t)
	tenants := seedTenants(t, admin)
	sessions := identity.NewSessionService(auth, time.Hour)
	agents := agentaccess.NewService(app)
	router := httpapi.NewRouter(httpapi.Deps{
		Logger:         testLogger(),
		Pool:           app,
		Migrations:     migrationsFS(t),
		ServiceName:    "atlas-api",
		Version:        "test",
		Identity:       sessions,
		DevSessions:    sessions,
		Authorizer:     authorization.NewService(),
		Orgs:           orgs.NewService(app),
		Agents:         agents,
		DevAuthEnabled: true,
	})

	adminCookie := login(t, router, tenants.OrganizationA, tenants.AdminEmailA)
	created := request(t, router, http.MethodPost, "/v1/agents", adminCookie, map[string]any{
		"name":   "ops-assistant",
		"scopes": []string{agentaccess.ScopeInvoicesRead, agentaccess.ScopeApprovalsRequest},
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("create agent status = %d, body = %s", created.Code, created.Body.String())
	}
	var createdBody struct {
		Agent agentaccess.Connection `json:"agent"`
	}
	decodeBody(t, created, &createdBody)
	if createdBody.Agent.Status != "active" {
		t.Fatalf("created agent = %+v", createdBody.Agent)
	}

	memberCookie := login(t, router, tenants.OrganizationA, tenants.MemberEmailA)
	if response := request(t, router, http.MethodGet, "/v1/agents", memberCookie, nil); response.Code != http.StatusForbidden {
		t.Fatalf("member list agents status = %d, want 403", response.Code)
	}

	issuedResponse := request(t, router, http.MethodPost,
		"/v1/agents/"+createdBody.Agent.ID+"/tokens", adminCookie, map[string]any{"expires_in_seconds": 3600})
	if issuedResponse.Code != http.StatusCreated {
		t.Fatalf("issue token status = %d, body = %s", issuedResponse.Code, issuedResponse.Body.String())
	}
	var issued agentaccess.IssuedToken
	decodeBody(t, issuedResponse, &issued)
	if issued.Token == "" || issued.TokenID == "" {
		t.Fatalf("issued token = %+v", issued)
	}

	me := bearerRequest(t, router, http.MethodGet, "/v1/agents/me", issued.Token)
	if me.Code != http.StatusOK {
		t.Fatalf("agent me status = %d, body = %s", me.Code, me.Body.String())
	}
	var meBody struct {
		Principal identity.Principal `json:"principal"`
	}
	decodeBody(t, me, &meBody)
	if meBody.Principal.Kind != identity.KindAgent || meBody.Principal.ID != createdBody.Agent.ID ||
		len(meBody.Principal.Scopes) != 2 {
		t.Fatalf("agent principal = %+v", meBody.Principal)
	}
	if response := bearerRequest(t, router, http.MethodGet, "/v1/agents", issued.Token); response.Code != http.StatusForbidden {
		t.Fatalf("agent list agents status = %d, want 403", response.Code)
	}

	revoked := request(t, router, http.MethodPost, "/v1/agents/"+createdBody.Agent.ID+"/revoke", adminCookie, nil)
	if revoked.Code != http.StatusOK {
		t.Fatalf("revoke status = %d, body = %s", revoked.Code, revoked.Body.String())
	}
	if response := bearerRequest(t, router, http.MethodGet, "/v1/agents/me", issued.Token); response.Code != http.StatusUnauthorized {
		t.Fatalf("revoked token status = %d, want 401", response.Code)
	}
}

func bearerRequest(t *testing.T, handler http.Handler, method, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, nil)
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}
