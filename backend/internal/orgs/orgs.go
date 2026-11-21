// Package orgs provides organization-scoped read services.
package orgs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"atlas/internal/platform/apperr"
	"atlas/internal/tenancy"
)

// Member is an organization membership joined with its user identity.
type Member struct {
	MembershipID string    `json:"membership_id"`
	UserID       string    `json:"user_id"`
	DisplayName  string    `json:"display_name"`
	Email        string    `json:"email"`
	Role         string    `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
}

// Organization is the tenant record visible to its members.
type Organization struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// Service reads organization-scoped data through RLS-bound transactions.
type Service struct {
	pool *pgxpool.Pool
}

// NewService constructs an organization service.
func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// ListMembers returns members of the organization.
func (s *Service) ListMembers(ctx context.Context, organizationID string) ([]Member, error) {
	if _, err := uuid.Parse(organizationID); err != nil {
		return nil, apperr.New(apperr.KindInvalid, "invalid_organization", "A valid organization is required.")
	}
	var members []Member
	err := tenancy.WithOrganizationTx(ctx, s.pool, organizationID, func(tx pgx.Tx) error {
		// The function is security-definer and returns only the current
		// organization's members, so the runtime role does not need direct
		// access to the global users table.
		rows, err := tx.Query(ctx, `
			SELECT membership_id, user_id, display_name, email, role, created_at
			FROM atlas_list_members()`)
		if err != nil {
			return fmt.Errorf("list members: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var member Member
			if err := rows.Scan(&member.MembershipID, &member.UserID, &member.DisplayName, &member.Email, &member.Role, &member.CreatedAt); err != nil {
				return fmt.Errorf("scan member: %w", err)
			}
			members = append(members, member)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	if members == nil {
		members = []Member{}
	}
	return members, nil
}

// GetOrganization returns the organization when visible under RLS.
func (s *Service) GetOrganization(ctx context.Context, organizationID string) (Organization, error) {
	if _, err := uuid.Parse(organizationID); err != nil {
		return Organization{}, apperr.New(apperr.KindInvalid, "invalid_organization", "A valid organization is required.")
	}
	var organization Organization
	err := tenancy.WithOrganizationTx(ctx, s.pool, organizationID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT id, name, status, created_at
			FROM organizations WHERE id = $1`, organizationID).
			Scan(&organization.ID, &organization.Name, &organization.Status, &organization.CreatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.New(apperr.KindNotFound, "organization_not_found", "The organization was not found.")
		}
		if err != nil {
			return fmt.Errorf("get organization: %w", err)
		}
		return nil
	})
	return organization, err
}
