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

// Invitation is an organization membership invitation.
type Invitation struct {
	ID             string
	OrganizationID string
	Email          string
	Role           string
	ExpiresAt      time.Time
	CreatedAt      time.Time
}

// InvitationService creates and accepts organization invitations.
type InvitationService struct {
	pool *pgxpool.Pool
	ttl  time.Duration
	now  func() time.Time
}

// NewInvitationService constructs the service.
func NewInvitationService(pool *pgxpool.Pool, ttl time.Duration) *InvitationService {
	return &InvitationService{pool: pool, ttl: ttl, now: time.Now}
}

// Create issues an invitation and returns the raw token once (for a share link).
func (s *InvitationService) Create(ctx context.Context, organizationID, email, role, invitedByUserID, inviterName string) (Invitation, string, error) {
	if _, err := uuid.Parse(organizationID); err != nil {
		return Invitation{}, "", apperr.New(apperr.KindInvalid, "invalid_organization", "A valid organization is required.")
	}
	if role != RoleAdmin && role != RoleMember {
		return Invitation{}, "", apperr.New(apperr.KindInvalid, "invalid_role", "Role must be admin or member.")
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || !strings.Contains(email, "@") {
		return Invitation{}, "", apperr.New(apperr.KindInvalid, "invalid_email", "A valid email address is required.")
	}
	token, hash, err := NewSessionToken()
	if err != nil {
		return Invitation{}, "", apperr.New(apperr.KindInternal, "invitation_failed", "Could not create the invitation.").WithCause(err)
	}
	expiresAt := s.now().UTC().Add(s.ttl)
	invitation := Invitation{OrganizationID: organizationID, Email: email, Role: role, ExpiresAt: expiresAt}

	err = tenancy.WithOrganizationTx(ctx, s.pool, organizationID, func(tx pgx.Tx) error {
		var inviterDisplay string
		if err := tx.QueryRow(ctx, `
			INSERT INTO organization_invitations (organization_id, email, role, token_hash, invited_by_user_id, expires_at)
			VALUES ($1, $2, $3, $4, nullif($5, '')::uuid, $6)
			RETURNING id, created_at`,
			organizationID, email, role, hash, invitedByUserID, expiresAt).
			Scan(&invitation.ID, &invitation.CreatedAt); err != nil {
			return fmt.Errorf("create invitation: %w", err)
		}
		inviterDisplay = inviterName
		return audit.Record(ctx, tx, audit.Event{
			OrganizationID: organizationID,
			ActorKind:      string(KindHuman),
			ActorID:        invitedByUserID,
			ActorDisplay:   inviterDisplay,
			Action:         "invitation.created",
			ResourceType:   "invitation",
			ResourceID:     invitation.ID,
			Decision:       "allow",
			Metadata:       map[string]any{"email": email, "role": role},
		})
	})
	if err != nil {
		return Invitation{}, "", err
	}
	return invitation, token, nil
}

// Accept marks a pending invitation accepted for a matching, verified email and
// invitation token. It must be called inside an organization-scoped transaction.
func (s *InvitationService) Accept(ctx context.Context, tx pgx.Tx, organizationID, email, userID, tokenHash string) (string, bool, error) {
	if tokenHash == "" {
		return "", false, nil
	}
	var invitationID, role string
	err := tx.QueryRow(ctx, `
		SELECT id, role
		FROM organization_invitations
		WHERE organization_id = $1
		  AND lower(email) = lower($2)
		  AND token_hash = $3
		  AND accepted_at IS NULL
		  AND revoked_at IS NULL
		  AND expires_at > now()
		ORDER BY created_at DESC
		LIMIT 1
		FOR UPDATE`, organizationID, email, tokenHash).Scan(&invitationID, &role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("find invitation: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE organization_invitations
		SET accepted_at = now(), accepted_by_user_id = $1
		WHERE id = $2`, userID, invitationID); err != nil {
		return "", false, fmt.Errorf("accept invitation: %w", err)
	}
	return role, true, nil
}
