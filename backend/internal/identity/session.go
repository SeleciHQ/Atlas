package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"atlas/internal/audit"
	"atlas/internal/platform/apperr"
	"atlas/internal/tenancy"
)

// SessionService issues and resolves opaque development sessions.
//
// This is the development authentication path only. Production human
// authentication is managed OIDC (ADR-002); agent authentication is deferred to
// Phase 9. Organization context is always explicit and required.
type SessionService struct {
	pool *pgxpool.Pool
	ttl  time.Duration
	now  func() time.Time
}

// NewSessionService constructs a session service.
func NewSessionService(pool *pgxpool.Pool, ttl time.Duration) *SessionService {
	return &SessionService{pool: pool, ttl: ttl, now: time.Now}
}

// DevLogin creates a session for an existing user membership. It is only
// reachable when development authentication is enabled by the router.
func (s *SessionService) DevLogin(ctx context.Context, organizationID, email string) (string, Principal, time.Time, error) {
	organizationID = strings.TrimSpace(organizationID)
	if _, err := uuid.Parse(organizationID); err != nil {
		return "", Principal{}, time.Time{}, apperr.New(apperr.KindInvalid, "invalid_organization", "A valid organization id is required.").
			WithField("organization_id", "must be a UUID")
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || !strings.Contains(email, "@") {
		return "", Principal{}, time.Time{}, apperr.New(apperr.KindInvalid, "invalid_email", "A valid email address is required.").
			WithField("email", "must be an email address")
	}

	token, hash, err := NewSessionToken()
	if err != nil {
		return "", Principal{}, time.Time{}, apperr.New(apperr.KindInternal, "session_creation_failed", "Could not create a session.").WithCause(err)
	}
	expiresAt := s.now().UTC().Add(s.ttl)

	var principal Principal
	err = tenancy.WithOrganizationTx(ctx, s.pool, organizationID, func(tx pgx.Tx) error {
		var userID, displayName, role string
		err := tx.QueryRow(ctx, `
			SELECT u.id, u.display_name, m.role
			FROM users u
			JOIN memberships m ON m.user_id = u.id
			JOIN organizations o ON o.id = m.organization_id
			WHERE m.organization_id = $1
			  AND lower(u.email) = $2
			  AND u.status = 'active'
			  AND o.status = 'active'
			LIMIT 1`, organizationID, email).Scan(&userID, &displayName, &role)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.New(apperr.KindUnauthenticated, "invalid_credentials", "No active membership was found for that email in this organization.")
			}
			return fmt.Errorf("look up membership: %w", err)
		}

		var sessionID string
		if err := tx.QueryRow(ctx, `
			INSERT INTO sessions (organization_id, user_id, token_hash, expires_at)
			VALUES ($1, $2, $3, $4)
			RETURNING id`, organizationID, userID, hash, expiresAt).Scan(&sessionID); err != nil {
			return fmt.Errorf("create session: %w", err)
		}

		if err := audit.Record(ctx, tx, audit.Event{
			OrganizationID: organizationID,
			ActorKind:      string(KindHuman),
			ActorID:        userID,
			ActorDisplay:   displayName,
			Action:         "auth.dev_login",
			ResourceType:   "session",
			ResourceID:     sessionID,
			Decision:       "allow",
			Metadata:       map[string]any{"method": "development"},
		}); err != nil {
			return err
		}

		principal = Principal{
			Kind:           KindHuman,
			ID:             userID,
			OrganizationID: organizationID,
			DisplayName:    displayName,
			Email:          email,
			Role:           role,
		}
		return nil
	})
	if err != nil {
		return "", Principal{}, time.Time{}, err
	}
	return token, principal, expiresAt, nil
}

// Authenticate resolves a raw session token into a principal.
func (s *SessionService) Authenticate(ctx context.Context, token string) (Principal, error) {
	if strings.TrimSpace(token) == "" {
		return Principal{}, apperr.New(apperr.KindUnauthenticated, "authentication_required", "Authentication is required.")
	}

	hash := HashSessionToken(token)
	var (
		sessionID      string
		userID         string
		organizationID string
		expiresAt      time.Time
		displayName    string
		email          string
	)
	err := s.pool.QueryRow(ctx, `
		SELECT s.id, s.user_id, s.organization_id, s.expires_at, u.display_name, u.email
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1
		  AND s.expires_at > now()
		  AND u.status = 'active'`,
		hash).Scan(&sessionID, &userID, &organizationID, &expiresAt, &displayName, &email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Principal{}, apperr.New(apperr.KindUnauthenticated, "invalid_session", "The session is invalid or has expired.")
		}
		return Principal{}, fmt.Errorf("resolve session: %w", err)
	}

	var role string
	err = tenancy.WithOrganizationTx(ctx, s.pool, organizationID, func(tx pgx.Tx) error {
		var organizationStatus string
		err := tx.QueryRow(ctx, `
			SELECT m.role, o.status
			FROM memberships m
			JOIN organizations o ON o.id = m.organization_id
			WHERE m.organization_id = $1 AND m.user_id = $2`,
			organizationID, userID).Scan(&role, &organizationStatus)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.New(apperr.KindUnauthenticated, "invalid_session", "The session is no longer valid.")
			}
			return fmt.Errorf("resolve membership: %w", err)
		}
		if organizationStatus != "active" {
			return apperr.New(apperr.KindUnauthenticated, "invalid_session", "The organization is not active.")
		}
		if _, err := tx.Exec(ctx, `UPDATE sessions SET last_seen_at = now() WHERE id = $1`, sessionID); err != nil {
			return fmt.Errorf("touch session: %w", err)
		}
		return nil
	})
	if err != nil {
		return Principal{}, err
	}

	return Principal{
		Kind:           KindHuman,
		ID:             userID,
		OrganizationID: organizationID,
		DisplayName:    displayName,
		Email:          email,
		Role:           role,
	}, nil
}

// Logout revokes a session. Unknown tokens are ignored.
func (s *SessionService) Logout(ctx context.Context, token string) error {
	if strings.TrimSpace(token) == "" {
		return nil
	}
	hash := HashSessionToken(token)

	var organizationID, userID, displayName string
	err := s.pool.QueryRow(ctx, `
		SELECT s.organization_id, s.user_id, u.display_name
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1`, hash).Scan(&organizationID, &userID, &displayName)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("look up session for logout: %w", err)
	}

	return tenancy.WithOrganizationTx(ctx, s.pool, organizationID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, hash); err != nil {
			return fmt.Errorf("revoke session: %w", err)
		}
		return audit.Record(ctx, tx, audit.Event{
			OrganizationID: organizationID,
			ActorKind:      string(KindHuman),
			ActorID:        userID,
			ActorDisplay:   displayName,
			Action:         "auth.logout",
			ResourceType:   "session",
			Decision:       "recorded",
			Metadata:       map[string]any{"method": "development"},
		})
	})
}
