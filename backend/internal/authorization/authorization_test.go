package authorization

import (
	"testing"

	"atlas/internal/identity"
)

func TestRolePermissionMatrix(t *testing.T) {
	cases := []struct {
		role       string
		permission Permission
		allowed    bool
	}{
		{identity.RoleAdmin, PermissionOrganizationManage, true},
		{identity.RoleAdmin, PermissionMembersManage, true},
		{identity.RoleAdmin, PermissionAgentsManage, true},
		{identity.RoleAdmin, PermissionActionsApprove, false},
		{identity.RoleAdmin, PermissionActionsReject, false},
		{identity.RoleMember, PermissionMembersRead, true},
		{identity.RoleMember, PermissionOrganizationRead, true},
		{identity.RoleMember, PermissionOrganizationManage, false},
		{identity.RoleMember, PermissionMembersManage, false},
		{identity.RoleMember, PermissionActionsApprove, false},
		{identity.RoleMember, PermissionActionsReject, false},
		{"unknown_role", PermissionOrganizationRead, false},
	}
	service := NewService()
	for _, tc := range cases {
		principal := identity.Principal{Kind: identity.KindHuman, Role: tc.role}
		if got := service.Allowed(principal, tc.permission); got != tc.allowed {
			t.Errorf("role %s permission %s = %v, want %v", tc.role, tc.permission, got, tc.allowed)
		}
	}
}

func TestNonHumanPrincipalsAreDenied(t *testing.T) {
	service := NewService()
	for _, kind := range []identity.Kind{identity.KindAgent, identity.KindIntegration, identity.KindWorkload} {
		principal := identity.Principal{Kind: kind, Role: identity.RoleAdmin}
		if service.Allowed(principal, PermissionOrganizationRead) {
			t.Errorf("principal kind %s must be denied without scopes", kind)
		}
	}
}

func TestAgentScopeAuthorization(t *testing.T) {
	service := NewService()
	agent := identity.Principal{Kind: identity.KindAgent, Scopes: []string{"invoices:read", "reconciliation:read"}}
	if !service.AllowedScope(agent, "invoices:read") || !service.AllowedScope(agent, "reconciliation:read") {
		t.Fatal("granted agent scopes were denied")
	}
	if service.AllowedScope(agent, "reconciliation:run") || service.AllowedScope(agent, "invoices:write") {
		t.Fatal("ungranted agent scope was allowed")
	}
	if err := service.AuthorizeScope(agent, "reconciliation:read"); err != nil {
		t.Fatalf("authorize scope: %v", err)
	}

	human := identity.Principal{Kind: identity.KindHuman, Role: identity.RoleMember}
	if !service.AllowedScope(human, "reconciliation:read") {
		t.Fatal("member should map to reconciliation:read permission")
	}
	if service.AllowedScope(human, "invoices:read") {
		t.Fatal("human scope without a permission mapping must be denied")
	}
	unknown := identity.Principal{Kind: identity.KindWorkload}
	if service.AllowedScope(unknown, "invoices:read") {
		t.Fatal("workload scope was allowed")
	}
}
