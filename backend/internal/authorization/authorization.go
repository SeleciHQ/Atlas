// Package authorization centralizes permission decisions. Default deny.
package authorization

import (
	"atlas/internal/identity"
	"atlas/internal/platform/apperr"
)

// Permission is a coarse capability checked by handlers.
type Permission string

const (
	PermissionOrganizationRead   Permission = "organization:read"
	PermissionOrganizationManage Permission = "organization:manage"
	PermissionMembersRead        Permission = "members:read"
	PermissionMembersManage      Permission = "members:manage"
	PermissionAgentsManage       Permission = "agents:manage"
	PermissionDocumentsRead      Permission = "documents:read"
	PermissionDocumentsWrite     Permission = "documents:write"
	PermissionIntegrationsRead   Permission = "integrations:read"
	PermissionIntegrationsWrite  Permission = "integrations:write"
	PermissionReconciliationRead Permission = "reconciliation:read"
	PermissionReconciliationRun  Permission = "reconciliation:run"
	PermissionApprovalsRequest   Permission = "approvals:request"
	PermissionApprovalsDecide    Permission = "approvals:decide"
	// Approval permissions may be granted explicitly to eligible members once
	// approval workflows exist (Phase 8). They are deliberately not implied by
	// a job title. There is intentionally no human actions.execute permission:
	// execution remains Atlas-controlled.
	PermissionActionsApprove Permission = "actions.approve"
	PermissionActionsReject  Permission = "actions.reject"
)

var rolePermissions = map[string]map[Permission]struct{}{
	identity.RoleAdmin: {
		PermissionOrganizationRead:   {},
		PermissionOrganizationManage: {},
		PermissionMembersRead:        {},
		PermissionMembersManage:      {},
		PermissionAgentsManage:       {},
		PermissionDocumentsRead:      {},
		PermissionDocumentsWrite:     {},
		PermissionIntegrationsRead:   {},
		PermissionIntegrationsWrite:  {},
		PermissionReconciliationRead: {},
		PermissionApprovalsRequest:   {},
		PermissionReconciliationRun:  {},
	},
	identity.RoleMember: {
		PermissionOrganizationRead:   {},
		PermissionMembersRead:        {},
		PermissionDocumentsRead:      {},
		PermissionDocumentsWrite:     {},
		PermissionIntegrationsRead:   {},
		PermissionReconciliationRead: {},
		PermissionApprovalsRequest:   {},
	},
}

// Service evaluates permissions for principals.
type Service struct{}

// NewService constructs the authorization service.
func NewService() Service { return Service{} }

// Authorize returns nil when the principal holds permission.
//
// Human principals are evaluated through their membership role. Agent and
// integration principals will be evaluated through scopes when those
// authentication paths exist (Phase 9); until then they are denied.
func (Service) Authorize(principal identity.Principal, permission Permission) error {
	if principal.Kind != identity.KindHuman {
		return apperr.New(apperr.KindForbidden, "permission_denied", "This principal may not perform that action.")
	}
	permissions, ok := rolePermissions[principal.Role]
	if !ok {
		return apperr.New(apperr.KindForbidden, "permission_denied", "This principal may not perform that action.")
	}
	if _, ok := permissions[permission]; !ok {
		return apperr.New(apperr.KindForbidden, "permission_denied", "This principal may not perform that action.")
	}
	return nil
}

// Allowed is the boolean form used in tests and internal checks.
func (s Service) Allowed(principal identity.Principal, permission Permission) bool {
	return s.Authorize(principal, permission) == nil
}
