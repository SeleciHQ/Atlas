package identity

import (
	"context"
	"time"
)

// Authenticator resolves a session token into a principal and revokes sessions.
// The development implementation is SessionService; production human
// authentication will provide a managed-OIDC-backed implementation behind this
// same interface (ADR-002).
type Authenticator interface {
	Authenticate(ctx context.Context, token string) (Principal, error)
	Logout(ctx context.Context, token string) error
}

// DevSessionIssuer issues development-only sessions. It is deliberately a
// separate interface so production authentication never exposes a dev login
// path.
type DevSessionIssuer interface {
	DevLogin(ctx context.Context, organizationID, email string) (token string, principal Principal, expiresAt time.Time, err error)
}
