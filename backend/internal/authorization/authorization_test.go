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
