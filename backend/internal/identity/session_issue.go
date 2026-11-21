package identity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"atlas/internal/audit"
	"atlas/internal/platform/apperr"
	"atlas/internal/tenancy"
)

// IssueSession creates a session for an already-authenticated user (OIDC path).
func (s *SessionService) IssueSession(ctx context.Context, organizationID, userID, action string) (string, Principal, time.Time, error) {
	token, hash, err := NewSessionToken()
	if err != nil {
		return "", Principal{}, time.Time{}, apperr.New(apperr.KindInternal, "session_creation_failed", "Could not create a session.").WithCause(err)
	}
	expiresAt := s.now().UTC().Add(s.ttl)
	var principal Principal
	err = tenancy.WithOrganizationTx(ctx, s.pool, organizationID, func(tx pgx.Tx) error {
		var displayName, email, role string
		err := tx.QueryRow(ctx, `
			SELECT u.display_name, u.email, m.role
			FROM users u JOIN memberships m ON m.user_id = u.id
			WHERE m.organization_id = $1 AND m.user_id = $2 AND u.status = 'active'`,
			organizationID, userID).Scan(&displayName, &email, &role)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.New(apperr.KindForbidden, "membership_required", "The user is not a member of this organization.")
			}
			return fmt.Errorf("resolve membership: %w", err)
		}
		var sessionID string
		if err := tx.QueryRow(ctx, `
			INSERT INTO sessions (organization_id, user_id, token_hash, expires_at)
			VALUES ($1, $2, $3, $4) RETURNING id`,
			organizationID, userID, hash, expiresAt).Scan(&sessionID); err != nil {
			return fmt.Errorf("create session: %w", err)
		}
		if err := audit.Record(ctx, tx, audit.Event{
			OrganizationID: organizationID,
			ActorKind:      string(KindHuman),
			ActorID:        userID,
			ActorDisplay:   displayName,
			Action:         action,
			ResourceType:   "session",
			ResourceID:     sessionID,
			Decision:       "allow",
			Metadata:       map[string]any{"method": "oidc"},
		}); err != nil {
			return err
		}
		principal = Principal{Kind: KindHuman, ID: userID, OrganizationID: organizationID, DisplayName: displayName, Email: email, Role: role}
		return nil
	})
	if err != nil {
		return "", Principal{}, time.Time{}, err
	}
	return token, principal, expiresAt, nil
}
