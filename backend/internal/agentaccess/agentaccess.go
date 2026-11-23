// Package agentaccess manages external agent connections, their scopes and
// bearer tokens. Agents are distinct principals from humans, integrations and
// workloads; they are denied by default unless a connection grants an explicit
// scope. Atlas does not build or host agents.
//
// Token issuance here is the development/v0 issuer permitted by the roadmap
// while ADR-002/ADR-004 production token issuance is finalized.
package agentaccess

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"atlas/internal/audit"
	"atlas/internal/identity"
	"atlas/internal/platform/apperr"
	"atlas/internal/tenancy"
)

// Scopes are the capabilities an agent connection may hold. No wildcard is
// supported; a connection only receives the scopes it is explicitly granted.
const (
	ScopeInvoicesRead       = "invoices:read"
	ScopePurchaseOrdersRead = "purchase_orders:read"
	ScopeDeliveriesRead     = "deliveries:read"
	ScopeSuppliersRead      = "suppliers:read"
	ScopeReconciliationRead = "reconciliation:read"
	ScopeReconciliationRun  = "reconciliation:run"
	ScopeDiscrepanciesRead  = "discrepancies:read"
	ScopeActionsRequest     = "actions:request"
	ScopeApprovalsRequest   = "approvals:request"
	ScopeOperationsRead     = "operations:read"
)

var validScopes = map[string]struct{}{
	ScopeInvoicesRead:       {},
	ScopePurchaseOrdersRead: {},
	ScopeDeliveriesRead:     {},
	ScopeSuppliersRead:      {},
	ScopeReconciliationRead: {},
	ScopeReconciliationRun:  {},
	ScopeDiscrepanciesRead:  {},
	ScopeActionsRequest:     {},
	ScopeApprovalsRequest:   {},
	ScopeOperationsRead:     {},
}

// ValidScope reports whether scope is in the agent scope catalog.
func ValidScope(scope string) bool {
	_, ok := validScopes[scope]
	return ok
}

// DefaultTokenTTL is used when a caller does not request one.
const DefaultTokenTTL = 24 * time.Hour

// MaxTokenTTL bounds issued agent tokens.
const MaxTokenTTL = 30 * 24 * time.Hour

// Connection is an agent connection read model.
type Connection struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Status    string     `json:"status"`
	Scopes    []string   `json:"scopes"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

// Token is an agent token read model. The raw token is never stored.
type Token struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// IssuedToken is returned once, when the raw token is generated.
type IssuedToken struct {
	Token     string    `json:"token"`
	TokenID   string    `json:"token_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Service manages agent connections and tokens.
type Service struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

// NewService constructs the agent access service.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, now: time.Now}
}

// CreateConnection registers an agent connection with explicit scopes.
func (s *Service) CreateConnection(ctx context.Context, organizationID, name string, scopes []string, createdBy string) (*Connection, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 120 {
		return nil, apperr.New(apperr.KindInvalid, "invalid_agent_name", "Agent name is required and must be 120 characters or fewer.")
	}
	if len(scopes) == 0 {
		return nil, apperr.New(apperr.KindInvalid, "scopes_required", "At least one scope is required.")
	}
	seen := map[string]struct{}{}
	for _, scope := range scopes {
		if !ValidScope(scope) {
			return nil, apperr.New(apperr.KindInvalid, "invalid_scope", "Unknown agent scope.").WithField("scope", scope)
		}
		if _, ok := seen[scope]; ok {
			return nil, apperr.New(apperr.KindInvalid, "duplicate_scope", "Scopes must be unique.").WithField("scope", scope)
		}
		seen[scope] = struct{}{}
	}

	connection := Connection{Name: name, Status: "active", Scopes: append([]string(nil), scopes...)}
	err := tenancy.WithOrganizationTx(ctx, s.pool, organizationID, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			INSERT INTO agent_connections (organization_id, name) VALUES ($1, $2) RETURNING id, created_at`,
			organizationID, name).Scan(&connection.ID, &connection.CreatedAt); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return apperr.New(apperr.KindConflict, "agent_name_conflict", "An agent connection with that name already exists.")
			}
			return err
		}
		for _, scope := range connection.Scopes {
			if _, err := tx.Exec(ctx, `
				INSERT INTO agent_scopes (agent_connection_id, organization_id, scope) VALUES ($1, $2, $3)`,
				connection.ID, organizationID, scope); err != nil {
				return err
			}
		}
		return audit.Record(ctx, tx, audit.Event{
			OrganizationID: organizationID, ActorKind: string(identity.KindHuman), ActorID: createdBy,
			Action: "agent.connection_created", ResourceType: "agent_connection", ResourceID: connection.ID,
			Metadata: map[string]any{"name": name, "scopes": connection.Scopes},
		})
	})
	if err != nil {
		return nil, err
	}
	return &connection, nil
}

// ListConnections returns agent connections newest first.
func (s *Service) ListConnections(ctx context.Context, organizationID string) ([]Connection, error) {
	var connections []Connection
	err := tenancy.WithOrganizationTx(ctx, s.pool, organizationID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT c.id, c.name, c.status, c.created_at, c.revoked_at,
			       coalesce(array_agg(s.scope ORDER BY s.scope)
			                FILTER (WHERE s.scope IS NOT NULL), ARRAY[]::text[])
			FROM agent_connections c
			LEFT JOIN agent_scopes s
			  ON s.organization_id = c.organization_id AND s.agent_connection_id = c.id
			WHERE c.organization_id = $1
			GROUP BY c.id
			ORDER BY c.created_at DESC`, organizationID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var connection Connection
			if err := rows.Scan(&connection.ID, &connection.Name, &connection.Status,
				&connection.CreatedAt, &connection.RevokedAt, &connection.Scopes); err != nil {
				return err
			}
			connections = append(connections, connection)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return connections, nil
}

// RevokeConnection revokes a connection and all of its tokens.
func (s *Service) RevokeConnection(ctx context.Context, organizationID, connectionID, revokedBy string) error {
	return tenancy.WithOrganizationTx(ctx, s.pool, organizationID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE agent_connections SET status = 'revoked', revoked_at = now()
			WHERE organization_id = $1 AND id = $2 AND status = 'active'`, organizationID, connectionID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return apperr.New(apperr.KindNotFound, "agent_not_found", "Active agent connection not found.")
		}
		if _, err := tx.Exec(ctx, `
			UPDATE agent_tokens SET revoked_at = now()
			WHERE organization_id = $1 AND agent_connection_id = $2 AND revoked_at IS NULL`,
			organizationID, connectionID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{
			OrganizationID: organizationID, ActorKind: string(identity.KindHuman), ActorID: revokedBy,
			Action: "agent.connection_revoked", ResourceType: "agent_connection", ResourceID: connectionID,
		})
	})
}

// IssueToken creates a bearer token for an active connection. The raw token is
// returned once and only its hash is persisted.
func (s *Service) IssueToken(ctx context.Context, organizationID, connectionID, createdBy string, ttl time.Duration) (*IssuedToken, error) {
	if ttl <= 0 {
		ttl = DefaultTokenTTL
	}
	if ttl > MaxTokenTTL {
		ttl = MaxTokenTTL
	}
	expiresAt := s.now().UTC().Add(ttl)
	token, hash, err := identity.NewSessionToken()
	if err != nil {
		return nil, apperr.New(apperr.KindInternal, "token_issue_failed", "Could not issue the agent token.").WithCause(err)
	}
	issued := IssuedToken{Token: token, ExpiresAt: expiresAt}
	err = tenancy.WithOrganizationTx(ctx, s.pool, organizationID, func(tx pgx.Tx) error {
		var status string
		if err := tx.QueryRow(ctx, `
			SELECT status FROM agent_connections WHERE organization_id = $1 AND id = $2`,
			organizationID, connectionID).Scan(&status); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.New(apperr.KindNotFound, "agent_not_found", "Agent connection not found.")
			}
			return err
		}
		if status != "active" {
			return apperr.New(apperr.KindConflict, "agent_revoked", "Revoked agent connections cannot receive tokens.")
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO agent_tokens (organization_id, agent_connection_id, token_hash, issued_by, expires_at)
			VALUES ($1, $2, $3, nullif($4, '')::uuid, $5) RETURNING id`,
			organizationID, connectionID, hash, createdBy, expiresAt).Scan(&issued.TokenID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{
			OrganizationID: organizationID, ActorKind: string(identity.KindHuman), ActorID: createdBy,
			Action: "agent.token_issued", ResourceType: "agent_token", ResourceID: issued.TokenID,
			Metadata: map[string]any{"agent_connection_id": connectionID, "expires_at": expiresAt},
		})
	})
	if err != nil {
		return nil, err
	}
	return &issued, nil
}

// ListTokens returns tokens for a connection (hashes are never exposed).
func (s *Service) ListTokens(ctx context.Context, organizationID, connectionID string) ([]Token, error) {
	var tokens []Token
	err := tenancy.WithOrganizationTx(ctx, s.pool, organizationID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, name, expires_at, revoked_at, last_used_at, created_at
			FROM agent_tokens WHERE organization_id = $1 AND agent_connection_id = $2
			ORDER BY created_at DESC`, organizationID, connectionID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var token Token
			if err := rows.Scan(&token.ID, &token.Name, &token.ExpiresAt, &token.RevokedAt,
				&token.LastUsedAt, &token.CreatedAt); err != nil {
				return err
			}
			tokens = append(tokens, token)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return tokens, nil
}

// RevokeToken revokes one token without touching the connection.
func (s *Service) RevokeToken(ctx context.Context, organizationID, connectionID, tokenID, revokedBy string) error {
	return tenancy.WithOrganizationTx(ctx, s.pool, organizationID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE agent_tokens SET revoked_at = now()
			WHERE organization_id = $1 AND agent_connection_id = $2 AND id = $3 AND revoked_at IS NULL`,
			organizationID, connectionID, tokenID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return apperr.New(apperr.KindNotFound, "agent_token_not_found", "Active agent token not found.")
		}
		return audit.Record(ctx, tx, audit.Event{
			OrganizationID: organizationID, ActorKind: string(identity.KindHuman), ActorID: revokedBy,
			Action: "agent.token_revoked", ResourceType: "agent_token", ResourceID: tokenID,
			Metadata: map[string]any{"agent_connection_id": connectionID},
		})
	})
}

// AuthenticateToken resolves a raw bearer token into an agent principal. It is
// the pre-tenant path; the security-definer function enforces connection,
// organization, expiry and revocation state and returns the scopes.
func (s *Service) AuthenticateToken(ctx context.Context, token string) (identity.Principal, error) {
	if strings.TrimSpace(token) == "" {
		return identity.Principal{}, apperr.New(apperr.KindUnauthenticated, "authentication_required", "Authentication is required.")
	}
	hash := identity.HashSessionToken(token)
	var principal identity.Principal
	err := s.pool.QueryRow(ctx, `
		SELECT organization_id, agent_connection_id, name, scopes
		FROM atlas_resolve_agent_token($1)`, hash).
		Scan(&principal.OrganizationID, &principal.ID, &principal.DisplayName, &principal.Scopes)
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.Principal{}, apperr.New(apperr.KindUnauthenticated, "invalid_agent_token", "The agent token is invalid, expired or revoked.")
	}
	if err != nil {
		return identity.Principal{}, fmt.Errorf("resolve agent token: %w", err)
	}
	principal.Kind = identity.KindAgent
	return principal, nil
}
