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
	PermissionInvoicesRead       Permission = "invoices:read"
	PermissionPurchaseOrdersRead Permission = "purchase_orders:read"
	PermissionDeliveriesRead     Permission = "deliveries:read"
	PermissionSuppliersRead      Permission = "suppliers:read"
	PermissionDiscrepanciesRead  Permission = "discrepancies:read"
	PermissionOperationsRead     Permission = "operations:read"
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
		PermissionInvoicesRead:       {},
		PermissionPurchaseOrdersRead: {},
		PermissionDeliveriesRead:     {},
		PermissionSuppliersRead:      {},
		PermissionDiscrepanciesRead:  {},
		PermissionOperationsRead:     {},
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
		PermissionInvoicesRead:       {},
		PermissionPurchaseOrdersRead: {},
		PermissionDeliveriesRead:     {},
		PermissionSuppliersRead:      {},
		PermissionDiscrepanciesRead:  {},
		PermissionOperationsRead:     {},
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

// scopePermissions maps agent scopes to the equivalent human permission where
// one exists. A human calling an agent-capable REST endpoint is evaluated by
// role; scopes without a human equivalent are agent-only.
var scopePermissions = map[string]Permission{
	"invoices:read":        PermissionInvoicesRead,
	"purchase_orders:read": PermissionPurchaseOrdersRead,
	"deliveries:read":      PermissionDeliveriesRead,
	"suppliers:read":       PermissionSuppliersRead,
	"discrepancies:read":   PermissionDiscrepanciesRead,
	"reconciliation:read":  PermissionReconciliationRead,
	"reconciliation:run":   PermissionReconciliationRun,
	"approvals:request":    PermissionApprovalsRequest,
	"operations:read":      PermissionOperationsRead,
}

// AuthorizeScope returns nil when an agent connection holds scope, or when a
// human principal's role implies the equivalent permission. Unknown scopes and
// non-agent/non-human principals are denied.
func (s Service) AuthorizeScope(principal identity.Principal, scope string) error {
	switch principal.Kind {
	case identity.KindAgent:
		for _, held := range principal.Scopes {
			if held == scope {
				return nil
			}
		}
	case identity.KindHuman:
		if permission, ok := scopePermissions[scope]; ok {
			return s.Authorize(principal, permission)
		}
	}
	return apperr.New(apperr.KindForbidden, "permission_denied", "This principal may not perform that action.")
}

// AllowedScope is the boolean form of AuthorizeScope.
func (s Service) AllowedScope(principal identity.Principal, scope string) bool {
	return s.AuthorizeScope(principal, scope) == nil
}
