package integration

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"atlas/internal/authorization"
	"atlas/internal/httpapi"
	"atlas/internal/identity"
	"atlas/internal/orgs"
	"atlas/internal/platform/apperr"
	"atlas/internal/retention"
)

func TestRetentionLegalHolds(t *testing.T) {
	admin, app, _ := setupTenantSchema(t)
	organizationA := seedQueueOrganization(t, admin)
	organizationB := seedQueueOrganization(t, admin)
	ctx := context.Background()
	service := retention.NewService(app)

	if policies := retention.Defaults(); len(policies) != 8 {
		t.Fatalf("retention policies = %d, want 8", len(policies))
	}
	if !retention.ValidClass(retention.ClassAudit) || retention.ValidClass("unknown") {
		t.Fatal("retention class validation is wrong")
	}

	hold, err := service.PlaceHold(ctx, organizationA, "document", "doc-1", "regulatory review", "")
	if err != nil {
		t.Fatalf("place hold: %v", err)
	}
	if hold.ResourceType != "document" || hold.ResourceID != "doc-1" || hold.ReleasedAt != nil {
		t.Fatalf("hold = %+v", hold)
	}
	if _, err := service.PlaceHold(ctx, organizationA, "document", "doc-1", "duplicate", ""); !isAppErrKind(err, apperr.KindConflict) {
		t.Fatalf("duplicate hold error = %v", err)
	}
	if _, err := service.PlaceHold(ctx, organizationA, "unknown", "x", "bad", ""); !isAppErrKind(err, apperr.KindInvalid) {
		t.Fatalf("invalid hold error = %v", err)
	}
	if _, err := service.PlaceHold(ctx, organizationA, "document", "doc-1", " ", ""); !isAppErrKind(err, apperr.KindInvalid) {
		t.Fatalf("empty reason error = %v", err)
	}

	held, err := service.IsHeld(ctx, organizationA, "document", "doc-1")
	if err != nil || !held {
		t.Fatalf("IsHeld = %v %v", held, err)
	}
	active, err := service.ListHolds(ctx, organizationA, true, 10)
	if err != nil || len(active) != 1 || active[0].ID != hold.ID {
		t.Fatalf("active holds = %+v %v", active, err)
	}
	if other, err := service.ListHolds(ctx, organizationB, true, 10); err != nil || len(other) != 0 {
		t.Fatalf("cross-tenant holds = %+v %v", other, err)
	}
	if held, err := service.IsHeld(ctx, organizationB, "document", "doc-1"); err != nil || held {
		t.Fatalf("cross-tenant IsHeld = %v %v", held, err)
	}
	if err := service.ReleaseHold(ctx, organizationB, hold.ID, ""); !isAppErrKind(err, apperr.KindNotFound) {
		t.Fatalf("cross-tenant release error = %v", err)
	}

	if err := service.ReleaseHold(ctx, organizationA, hold.ID, ""); err != nil {
		t.Fatalf("release hold: %v", err)
	}
	if held, err := service.IsHeld(ctx, organizationA, "document", "doc-1"); err != nil || held {
		t.Fatalf("IsHeld after release = %v %v", held, err)
	}
	if err := service.ReleaseHold(ctx, organizationA, hold.ID, ""); !isAppErrKind(err, apperr.KindNotFound) {
		t.Fatalf("second release error = %v", err)
	}
	all, err := service.ListHolds(ctx, organizationA, false, 10)
	if err != nil || len(all) != 1 || all[0].ReleasedAt == nil {
		t.Fatalf("all holds = %+v %v", all, err)
	}

	var audits int
	if err := admin.QueryRow(ctx, `
		SELECT count(*) FROM audit_events
		WHERE organization_id = $1 AND action IN ('retention.legal_hold_placed', 'retention.legal_hold_released')`,
		organizationA).Scan(&audits); err != nil {
		t.Fatalf("count audit: %v", err)
	}
	if audits != 2 {
		t.Fatalf("retention audit rows = %d, want 2", audits)
	}
}

func TestLegalHoldREST(t *testing.T) {
	admin, app, auth := setupTenantSchema(t)
	tenants := seedTenants(t, admin)
	sessions := identity.NewSessionService(auth, time.Hour)
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
		Retention:      retention.NewService(app),
		DevAuthEnabled: true,
	})
	adminCookie := login(t, router, tenants.OrganizationA, tenants.AdminEmailA)
	memberCookie := login(t, router, tenants.OrganizationA, tenants.MemberEmailA)

	created := request(t, router, http.MethodPost, "/v1/legal-holds", adminCookie, map[string]any{
		"resource_type": "canonical_invoice", "resource_id": "inv-1", "reason": "audit",
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("create hold status = %d, body = %s", created.Code, created.Body.String())
	}
	var createdBody struct {
		Hold retention.Hold `json:"legal_hold"`
	}
	decodeBody(t, created, &createdBody)
	if createdBody.Hold.ID == "" {
		t.Fatalf("created hold = %+v", createdBody)
	}
	if response := request(t, router, http.MethodGet, "/v1/legal-holds", memberCookie, nil); response.Code != http.StatusForbidden {
		t.Fatalf("member list holds status = %d, want 403", response.Code)
	}
	listed := request(t, router, http.MethodGet, "/v1/legal-holds", adminCookie, nil)
	if listed.Code != http.StatusOK {
		t.Fatalf("list holds status = %d, body = %s", listed.Code, listed.Body.String())
	}
	released := request(t, router, http.MethodPost, "/v1/legal-holds/"+createdBody.Hold.ID+"/release", adminCookie, nil)
	if released.Code != http.StatusOK {
		t.Fatalf("release hold status = %d, body = %s", released.Code, released.Body.String())
	}
	if response := request(t, router, http.MethodPost, "/v1/legal-holds", adminCookie, map[string]any{
		"resource_type": "bogus", "resource_id": "x", "reason": "bad",
	}); response.Code != http.StatusBadRequest {
		t.Fatalf("invalid hold status = %d, want 400", response.Code)
	}
}

func isAppErrKind(err error, kind apperr.Kind) bool {
	var appErr *apperr.Error
	return errors.As(err, &appErr) && appErr.Kind == kind
}
