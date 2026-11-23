// Package governance implements central policy evaluation, human approvals and
// idempotent operation records. Execution effects are performed by the worker
// behind this boundary; every decision is auditable.
package governance

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"atlas/internal/audit"
)

// Service provides policy, approval and operation operations.
type Service struct{ pool *pgxpool.Pool }

// NewService constructs the governance service.
func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// Decision is the policy outcome for one requested action.
type Decision struct {
	Allowed          bool   `json:"allowed"`
	RequiresApproval bool   `json:"requires_approval"`
	Reason           string `json:"reason"`
}

type policyConfig struct {
	AllowActions         []string `json:"allow_actions"`
	ApprovalActions      []string `json:"approval_actions"`
	AmountThresholdMinor int64    `json:"amount_threshold_minor"`
}

// Evaluate applies the latest active policy. No active policy means deny.
func (s *Service) Evaluate(ctx context.Context, organizationID, action string, amountMinor int64) (*Decision, error) {
	var raw []byte
	err := s.withOrg(ctx, organizationID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT config FROM policies WHERE organization_id = $1 AND status = 'active'
			ORDER BY version DESC LIMIT 1`, organizationID).Scan(&raw)
	})
	if err == pgx.ErrNoRows {
		return &Decision{Allowed: false, Reason: "no active policy"}, nil
	}
	if err != nil {
		return nil, err
	}
	var config policyConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		return nil, fmt.Errorf("decode policy: %w", err)
	}
	for _, allowed := range config.AllowActions {
		if allowed == action {
			return &Decision{Allowed: true, Reason: "allowed by policy"}, nil
		}
	}
	for _, needsApproval := range config.ApprovalActions {
		if needsApproval == action {
			if config.AmountThresholdMinor > 0 && amountMinor > config.AmountThresholdMinor {
				return &Decision{Allowed: false, Reason: "amount exceeds approval-free threshold"}, nil
			}
			return &Decision{Allowed: true, RequiresApproval: true, Reason: "requires human approval"}, nil
		}
	}
	return &Decision{Allowed: false, Reason: "action not permitted by policy"}, nil
}

// RequestApproval creates a pending approval.
func (s *Service) RequestApproval(ctx context.Context, organizationID, operationType, objectType, objectID, requestedBy string) (string, error) {
	var id string
	err := s.withOrg(ctx, organizationID, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			INSERT INTO approvals (organization_id, operation_type, object_type, object_id, requested_by)
			VALUES ($1, $2, $3, $4, nullif($5, '')::uuid) RETURNING id`,
			organizationID, operationType, objectType, objectID, requestedBy).Scan(&id); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{
			OrganizationID: organizationID, ActorKind: "human", ActorID: requestedBy,
			Action: "approval.requested", ResourceType: objectType, ResourceID: objectID, Decision: "require_approval",
			Metadata: map[string]any{"operation_type": operationType},
		})
	})
	return id, err
}

// Decide approves or rejects a pending approval. Only pending approvals move.
func (s *Service) Decide(ctx context.Context, organizationID, approvalID, decidedBy string, approve bool, reason string) error {
	status := "rejected"
	if approve {
		status = "approved"
	}
	return s.withOrg(ctx, organizationID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE approvals SET status = $3, decided_by = nullif($4, '')::uuid, decision_reason = $5, decided_at = now()
			WHERE organization_id = $1 AND id = $2 AND status = 'pending'
			  AND (expires_at IS NULL OR expires_at > now())`,
			organizationID, approvalID, status, decidedBy, reason)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return fmt.Errorf("approval is not pending")
		}
		return audit.Record(ctx, tx, audit.Event{
			OrganizationID: organizationID, ActorKind: "human", ActorID: decidedBy,
			Action: "approval.decided", ResourceType: "approval", ResourceID: approvalID, Decision: map[bool]string{true: "allow", false: "deny"}[approve],
			Metadata: map[string]any{"reason": reason},
		})
	})
}

// StartOperation records an idempotent operation. A referenced approval must
// already be approved. Duplicate idempotency keys return the existing operation.
func (s *Service) StartOperation(ctx context.Context, organizationID, approvalID, operationType, idempotencyKey string, payload []byte) (string, bool, error) {
	if idempotencyKey == "" {
		return "", false, fmt.Errorf("idempotency key is required")
	}
	var id string
	created := false
	err := s.withOrg(ctx, organizationID, func(tx pgx.Tx) error {
		if approvalID != "" {
			var status string
			if err := tx.QueryRow(ctx, `
				SELECT status FROM approvals WHERE organization_id = $1 AND id = $2`,
				organizationID, approvalID).Scan(&status); err != nil {
				return err
			}
			if status != "approved" {
				return fmt.Errorf("operation requires an approved approval")
			}
		}
		err := tx.QueryRow(ctx, `
			INSERT INTO operations (organization_id, approval_id, type, idempotency_key, payload)
			VALUES ($1, nullif($2, '')::uuid, $3, $4, $5)
			ON CONFLICT (organization_id, idempotency_key) DO NOTHING RETURNING id`,
			organizationID, approvalID, operationType, idempotencyKey, payload).Scan(&id)
		if err == pgx.ErrNoRows {
			return tx.QueryRow(ctx, `
				SELECT id FROM operations WHERE organization_id = $1 AND idempotency_key = $2`,
				organizationID, idempotencyKey).Scan(&id)
		}
		if err != nil {
			return err
		}
		created = true
		return nil
	})
	if err != nil {
		return "", false, err
	}
	if created {
		_ = s.withOrg(ctx, organizationID, func(tx pgx.Tx) error {
			return audit.Record(ctx, tx, audit.Event{
				OrganizationID: organizationID, ActorKind: "system", Action: "operation.started",
				ResourceType: "operation", ResourceID: id, Decision: "allow",
				Metadata: map[string]any{"type": operationType},
			})
		})
	}
	return id, created, nil
}

// FinishOperation records a terminal outcome and attempt row.
func (s *Service) FinishOperation(ctx context.Context, organizationID, operationID, status, result, errorText string) error {
	if status != "succeeded" && status != "failed" && status != "cancelled" {
		return fmt.Errorf("invalid terminal status %q", status)
	}
	return s.withOrg(ctx, organizationID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE operations SET status = $3, result = nullif($4, '')::jsonb, last_error = nullif($5, ''),
			       updated_at = now()
			WHERE organization_id = $1 AND id = $2 AND status IN ('pending', 'running')`,
			organizationID, operationID, status, result, errorText)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return fmt.Errorf("operation is already terminal")
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO operation_attempts (organization_id, operation_id, attempt, status, error, finished_at)
			VALUES ($1, $2,
				(SELECT coalesce(max(attempt), 0) + 1 FROM operation_attempts WHERE operation_id = $2),
				$3, nullif($4, ''), now())`, organizationID, operationID, status, errorText)
		return err
	})
}

func (s *Service) withOrg(ctx context.Context, organizationID string, fn func(pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT set_config('app.organization_id', $1, true)`, organizationID); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SetPolicy creates the next active policy version and retires older ones.
func (s *Service) SetPolicy(ctx context.Context, organizationID, name string, config []byte) (int, error) {
	if name == "" || len(config) == 0 {
		return 0, fmt.Errorf("policy name and config are required")
	}
	var version int
	err := s.withOrg(ctx, organizationID, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			SELECT coalesce(max(version), 0) + 1 FROM policies
			WHERE organization_id = $1 AND name = $2`, organizationID, name).Scan(&version); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE policies SET status = 'retired' WHERE organization_id = $1 AND name = $2 AND status = 'active'`,
			organizationID, name); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO policies (organization_id, name, version, status, config)
			VALUES ($1, $2, $3, 'active', $4)`, organizationID, name, version, config)
		return err
	})
	return version, err
}

// ActivePolicy returns the latest active policy config.
func (s *Service) ActivePolicy(ctx context.Context, organizationID string) (json.RawMessage, error) {
	var raw []byte
	err := s.withOrg(ctx, organizationID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT config FROM policies WHERE organization_id = $1 AND status = 'active'
			ORDER BY version DESC LIMIT 1`, organizationID).Scan(&raw)
	})
	if err != nil {
		return nil, err
	}
	return raw, nil
}

// Operation is the operation read model.
type Operation struct {
	ID             string          `json:"id"`
	Type           string          `json:"type"`
	Status         string          `json:"status"`
	IdempotencyKey string          `json:"idempotency_key"`
	Result         json.RawMessage `json:"result,omitempty"`
	LastError      *string         `json:"last_error,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
}

// GetOperation returns one operation.
func (s *Service) GetOperation(ctx context.Context, organizationID, operationID string) (*Operation, error) {
	var operation Operation
	var result []byte
	err := s.withOrg(ctx, organizationID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT id, type, status, idempotency_key, result, last_error, created_at
			FROM operations WHERE organization_id = $1 AND id = $2`,
			organizationID, operationID).Scan(&operation.ID, &operation.Type, &operation.Status,
			&operation.IdempotencyKey, &result, &operation.LastError, &operation.CreatedAt)
	})
	if err != nil {
		return nil, err
	}
	operation.Result = result
	return &operation, nil
}
