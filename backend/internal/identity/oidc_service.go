package identity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"atlas/internal/audit"
	"atlas/internal/platform/apperr"
	"atlas/internal/tenancy"
)

// OIDCStartResult carries the authorization URL and sealed state cookie value.
type OIDCStartResult struct {
	AuthorizationURL string
	StateCookie      string
	StateMaxAge      int
}

// OIDCService implements the managed-OIDC login flow (WorkOS AuthKit).
type OIDCService struct {
	pool        *pgxpool.Pool // identity/session role
	sessions    *SessionService
	workos      *WorkOSClient
	invitations *InvitationService
	now         func() time.Time
}

// NewOIDCService constructs the service.
func NewOIDCService(pool *pgxpool.Pool, sessions *SessionService, workos *WorkOSClient, invitations *InvitationService) *OIDCService {
	return &OIDCService{pool: pool, sessions: sessions, workos: workos, invitations: invitations, now: time.Now}
}

// Start creates the authorization URL and sealed CSRF state. invitationToken is
// required for new-identity sign-up and is sealed into the state.
func (s *OIDCService) Start(organizationID, loginHint, linkUserID, invitationToken string) (OIDCStartResult, error) {
	if organizationID != "" {
		if _, err := uuid.Parse(organizationID); err != nil {
			return OIDCStartResult{}, apperr.New(apperr.KindInvalid, "invalid_organization", "A valid organization is required.")
		}
	}
	invitationTokenHash := ""
	if invitationToken != "" {
		invitationTokenHash = HashSessionToken(invitationToken)
	}
	state, err := NewOIDCState(organizationID, linkUserID, invitationTokenHash, s.now())
	if err != nil {
		return OIDCStartResult{}, err
	}
	sealed, err := SealOIDCState(state, s.workos.config.APIKey)
	if err != nil {
		return OIDCStartResult{}, fmt.Errorf("seal oidc state: %w", err)
	}
	return OIDCStartResult{
		// WorkOS organization_id expects a WorkOS organization, not an Atlas
		// organization UUID. The Atlas organization is carried in the sealed
		// state; per-customer WorkOS SSO connections are mapped later.
		AuthorizationURL: s.workos.AuthorizationURL(state.State, "", loginHint),
		StateCookie:      sealed,
		StateMaxAge:      600,
	}, nil
}

// Callback completes the authorization-code exchange and returns an Atlas
// session. The (issuer, subject) pair is the only external identity key.
func (s *OIDCService) Callback(ctx context.Context, code, sealedState, ipAddress, userAgent string) (string, Principal, time.Time, error) {
	if code == "" {
		return "", Principal{}, time.Time{}, apperr.New(apperr.KindInvalid, "missing_code", "The authorization code is missing.")
	}
	state, err := OpenOIDCState(sealedState, s.workos.config.APIKey, s.now())
	if err != nil {
		return "", Principal{}, time.Time{}, apperr.New(apperr.KindUnauthenticated, "invalid_oidc_state", "The sign-in attempt expired or could not be verified.")
	}
	user, err := s.workos.AuthenticateCode(ctx, code, ipAddress, userAgent)
	if err != nil {
		return "", Principal{}, time.Time{}, err
	}
	issuer := s.workos.Issuer()

	identityUserID, found, err := s.findIdentity(ctx, issuer, user.ID)
	if err != nil {
		return "", Principal{}, time.Time{}, err
	}

	if state.LinkUserID != "" {
		return s.linkExisting(ctx, state, issuer, user, identityUserID, found)
	}

	if found {
		organizationID, err := s.resolveOrganization(ctx, identityUserID, state.OrganizationID)
		if err != nil {
			return "", Principal{}, time.Time{}, err
		}
		s.touchIdentity(ctx, issuer, user.ID)
		return s.sessions.IssueSession(ctx, organizationID, identityUserID, "auth.oidc_login")
	}

	// Unknown (issuer, subject): never auto-link to an existing account by email.
	existingUserID, err := s.findUserByEmail(ctx, user.Email)
	if err != nil {
		return "", Principal{}, time.Time{}, err
	}
	if existingUserID != "" {
		return "", Principal{}, time.Time{}, apperr.New(apperr.KindConflict, "account_linking_required",
			"An Atlas account with this email already exists. Sign in to that account to link this identity explicitly.")
	}
	if state.OrganizationID == "" {
		return "", Principal{}, time.Time{}, apperr.New(apperr.KindInvalid, "organization_required",
			"Sign-in for a new identity requires an organization invitation link.")
	}
	if !user.EmailVerified {
		return "", Principal{}, time.Time{}, apperr.New(apperr.KindForbidden, "email_not_verified",
			"The identity provider has not verified this email address.")
	}

	var createdUserID string
	err = tenancy.WithOrganizationTx(ctx, s.pool, state.OrganizationID, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			INSERT INTO users (email, display_name) VALUES ($1, $2)
			RETURNING id`, user.Email, displayName(user)).Scan(&createdUserID); err != nil {
			return fmt.Errorf("create user: %w", err)
		}
		role, accepted, err := s.invitations.Accept(ctx, tx, state.OrganizationID, user.Email, createdUserID, state.InvitationTokenHash)
		if err != nil {
			return err
		}
		if !accepted {
			return apperr.New(apperr.KindForbidden, "invitation_required", "No valid organization invitation exists for this email.")
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO memberships (organization_id, user_id, role) VALUES ($1, $2, $3)
			RETURNING id`, state.OrganizationID, createdUserID, role).Scan(new(string)); err != nil {
			return fmt.Errorf("create membership: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO oidc_identities (issuer, subject, user_id, email_at_link, email_verified, last_login_at)
			VALUES ($1, $2, $3, $4, true, now())`, issuer, user.ID, createdUserID, user.Email); err != nil {
			return fmt.Errorf("link identity: %w", err)
		}
		if err := audit.Record(ctx, tx, audit.Event{
			OrganizationID: state.OrganizationID,
			ActorKind:      string(KindHuman),
			ActorID:        createdUserID,
			ActorDisplay:   displayName(user),
			Action:         "identity.created",
			ResourceType:   "oidc_identity",
			ResourceID:     issuer + "#" + user.ID,
			Decision:       "allow",
			Metadata:       map[string]any{"email": user.Email},
		}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return "", Principal{}, time.Time{}, err
	}
	return s.sessions.IssueSession(ctx, state.OrganizationID, createdUserID, "auth.oidc_login")
}

func (s *OIDCService) linkExisting(ctx context.Context, state OIDCState, issuer string, user WorkOSUser, mappedUserID string, mapped bool) (string, Principal, time.Time, error) {
	if mapped && mappedUserID != state.LinkUserID {
		return "", Principal{}, time.Time{}, apperr.New(apperr.KindConflict, "identity_already_linked",
			"This identity is already linked to a different Atlas account.")
	}
	if !user.EmailVerified {
		return "", Principal{}, time.Time{}, apperr.New(apperr.KindForbidden, "email_not_verified",
			"The identity provider has not verified this email address.")
	}
	organizationID, err := s.resolveOrganization(ctx, state.LinkUserID, state.OrganizationID)
	if err != nil {
		return "", Principal{}, time.Time{}, err
	}
	err = tenancy.WithOrganizationTx(ctx, s.pool, organizationID, func(tx pgx.Tx) error {
		if !mapped {
			if _, err := tx.Exec(ctx, `
				INSERT INTO oidc_identities (issuer, subject, user_id, email_at_link, email_verified, last_login_at)
				VALUES ($1, $2, $3, $4, true, now())`, issuer, user.ID, state.LinkUserID, user.Email); err != nil {
				return fmt.Errorf("link identity: %w", err)
			}
		}
		return audit.Record(ctx, tx, audit.Event{
			OrganizationID: organizationID,
			ActorKind:      string(KindHuman),
			ActorID:        state.LinkUserID,
			Action:         "identity.linked",
			ResourceType:   "oidc_identity",
			ResourceID:     issuer + "#" + user.ID,
			Decision:       "allow",
			Metadata:       map[string]any{"email": user.Email},
		})
	})
	if err != nil {
		return "", Principal{}, time.Time{}, err
	}
	return s.sessions.IssueSession(ctx, organizationID, state.LinkUserID, "auth.oidc_login")
}

func (s *OIDCService) findIdentity(ctx context.Context, issuer, subject string) (string, bool, error) {
	var userID string
	err := s.pool.QueryRow(ctx, `SELECT user_id FROM oidc_identities WHERE issuer = $1 AND subject = $2`, issuer, subject).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("find identity: %w", err)
	}
	return userID, true, nil
}

func (s *OIDCService) findUserByEmail(ctx context.Context, email string) (string, error) {
	var userID string
	err := s.pool.QueryRow(ctx, `SELECT id FROM users WHERE lower(email) = lower($1) LIMIT 1`, email).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("find user by email: %w", err)
	}
	return userID, nil
}

func (s *OIDCService) touchIdentity(ctx context.Context, issuer, subject string) {
	_, _ = s.pool.Exec(ctx, `UPDATE oidc_identities SET last_login_at = now() WHERE issuer = $1 AND subject = $2`, issuer, subject)
}

// resolveOrganization determines the Atlas organization for a session. The
// organization must come from the sealed state or from exactly one membership;
// multiple memberships require an explicit organization selection.
func (s *OIDCService) resolveOrganization(ctx context.Context, userID, requestedOrganizationID string) (string, error) {
	if requestedOrganizationID != "" {
		var status string
		err := tenancy.WithOrganizationTx(ctx, s.pool, requestedOrganizationID, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `
				SELECT o.status FROM memberships m JOIN organizations o ON o.id = m.organization_id
				WHERE m.organization_id = $1 AND m.user_id = $2`, requestedOrganizationID, userID).Scan(&status)
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return "", apperr.New(apperr.KindForbidden, "membership_required", "The user is not a member of this organization.")
		}
		if err != nil {
			return "", err
		}
		if status != "active" {
			return "", apperr.New(apperr.KindForbidden, "organization_inactive", "The organization is not active.")
		}
		return requestedOrganizationID, nil
	}

	rows, err := s.pool.Query(ctx, `SELECT organization_id FROM atlas_memberships_for_user($1)`, userID)
	if err != nil {
		return "", fmt.Errorf("list memberships: %w", err)
	}
	defer rows.Close()
	var organizations []string
	for rows.Next() {
		var organizationID string
		if err := rows.Scan(&organizationID); err != nil {
			return "", fmt.Errorf("scan membership: %w", err)
		}
		organizations = append(organizations, organizationID)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	switch len(organizations) {
	case 0:
		return "", apperr.New(apperr.KindForbidden, "membership_required", "No active organization membership was found.")
	case 1:
		return organizations[0], nil
	default:
		return "", apperr.New(apperr.KindConflict, "organization_selection_required",
			"This account belongs to multiple organizations; start sign-in with an organization id.")
	}
}

func displayName(user WorkOSUser) string {
	if user.FirstName != "" || user.LastName != "" {
		return user.FirstName + " " + user.LastName
	}
	return user.Email
}
