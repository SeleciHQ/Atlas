// Package identity defines Atlas principals, roles and session handling.
package identity

import "context"

// Kind identifies the category of an authenticated principal.
type Kind string

const (
	KindHuman       Kind = "human"
	KindAgent       Kind = "agent"
	KindIntegration Kind = "integration"
	KindWorkload    Kind = "workload"
)

// Organization roles for human memberships. Job titles are not built-in
// roles; permissions are the authorization mechanism.
const (
	RoleAdmin  = "admin"
	RoleMember = "member"
)

// Principal is the authenticated subject for a request. Human principals carry
// an organization membership role; agent principals will carry scopes when
// agent authentication is implemented (Phase 9).
type Principal struct {
	Kind           Kind     `json:"kind"`
	ID             string   `json:"id"`
	OrganizationID string   `json:"organization_id"`
	DisplayName    string   `json:"display_name"`
	Email          string   `json:"email,omitempty"`
	Role           string   `json:"role,omitempty"`
	Scopes         []string `json:"scopes,omitempty"`
}

// ValidRole reports whether role is a known membership role.
func ValidRole(role string) bool {
	switch role {
	case RoleAdmin, RoleMember:
		return true
	default:
		return false
	}
}

type principalContextKey struct{}

// WithPrincipal stores the authenticated principal in the context.
func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}

// PrincipalFrom returns the authenticated principal.
func PrincipalFrom(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(Principal)
	return principal, ok
}

// WorkloadIdentity is reserved for process-level authentication (ADR-004).
// Concrete issuance/verification is deferred until production workload
// identity is selected; this interface fixes the boundary.
type WorkloadIdentity interface {
	ID() string
	Verify(ctx context.Context, token string) (Principal, error)
}

// Workload names used for attribution once workload identities are issued.
const (
	WorkloadAPI       = "atlas-api"
	WorkloadMCP       = "atlas-mcp"
	WorkloadWorker    = "atlas-worker"
	WorkloadScheduler = "atlas-scheduler"
	WorkloadPython    = "atlas-python-worker"
)
