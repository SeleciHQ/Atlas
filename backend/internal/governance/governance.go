// Package governance implements central policy evaluation, human approvals and
// idempotent operation records. Execution effects are performed by the worker
// behind this boundary; every decision is auditable.
package governance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"atlas/internal/audit"
	"atlas/internal/jobs"
	"atlas/internal/platform/apperr"
)

// ActionExecuteJob is the job type that runs approved operations.
const ActionExecuteJob = "action.execute"

// ErrOperationTerminal is returned when a terminal operation is mutated.
var ErrOperationTerminal = errors.New("operation is already terminal")

// ErrIdempotencyConflict is returned when a key is reused for a different request.
var ErrIdempotencyConflict = errors.New("idempotency key was used with a different request")

// defaultApprovalTTL applies when the active policy does not set a shorter TTL.
const defaultApprovalTTL = 7 * 24 * time.Hour

// Service provides policy, approval and operation operations.
type Service struct {
	pool  *pgxpool.Pool
	queue *jobs.Queue
}

// NewService constructs the governance service.
func NewService(pool *pgxpool.Pool, queue *jobs.Queue) *Service {
	return &Service{pool: pool, queue: queue}
}

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
	ApprovalTTLMinutes   int64    `json:"approval_ttl_minutes"`
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

// Actor identifies who initiated a governance action. Agent, human, integration
// and workload actors are all recorded in audit; only humans have a user row.
type Actor struct {
	Kind    string
	ID      string
	Display string
}

// RequestApproval creates a pending approval for a human requester.
func (s *Service) RequestApproval(ctx context.Context, organizationID, operationType, objectType, objectID, requestedBy string) (string, error) {
	return s.RequestApprovalAs(ctx, organizationID, operationType, objectType, objectID,
		Actor{Kind: "human", ID: requestedBy})
}

// RequestApprovalAs creates a pending approval attributed to actor. The expiry
// comes from the active policy's approval_ttl_minutes, or a conservative
// default when unset.
func (s *Service) RequestApprovalAs(ctx context.Context, organizationID, operationType, objectType, objectID string, actor Actor) (string, error) {
	if actor.Kind == "" {
		actor.Kind = "human"
	}
	requestedBy := ""
	if actor.Kind == "human" {
		requestedBy = actor.ID
	}
	var id string
	err := s.withOrg(ctx, organizationID, func(tx pgx.Tx) error {
		ttl := defaultApprovalTTL
		var raw []byte
		err := tx.QueryRow(ctx, `
			SELECT config FROM policies WHERE organization_id = $1 AND status = 'active'
			ORDER BY version DESC LIMIT 1`, organizationID).Scan(&raw)
		switch {
		case err == nil:
			var config policyConfig
			if err := json.Unmarshal(raw, &config); err != nil {
				return fmt.Errorf("decode policy: %w", err)
			}
			if config.ApprovalTTLMinutes > 0 {
				ttl = time.Duration(config.ApprovalTTLMinutes) * time.Minute
			}
		case errors.Is(err, pgx.ErrNoRows):
		default:
			return err
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO approvals (organization_id, operation_type, object_type, object_id, requested_by, expires_at)
			VALUES ($1, $2, $3, $4, nullif($5, '')::uuid,
			        now() + ($6::bigint * interval '1 second')) RETURNING id`,
			organizationID, operationType, objectType, objectID, requestedBy, int64(ttl/time.Second)).Scan(&id); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{
			OrganizationID: organizationID, ActorKind: actor.Kind, ActorID: actor.ID, ActorDisplay: actor.Display,
			Action: "approval.requested", ResourceType: objectType, ResourceID: objectID, Decision: "require_approval",
			Metadata: map[string]any{"operation_type": operationType},
		})
	})
	return id, err
}

// FollowUp is the result of requesting a governed supplier follow-up.
type FollowUp struct {
	ApprovalID   string `json:"approval_id"`
	Status       string `json:"status"`
	PolicyReason string `json:"policy_reason"`
}

// RequestSupplierFollowup evaluates policy and creates an approval for a
// supplier follow-up on an invoice. Execution stays Atlas-controlled.
func (s *Service) RequestSupplierFollowup(ctx context.Context, organizationID, invoiceID string, amountMinor int64, actor Actor) (*FollowUp, error) {
	if invoiceID == "" {
		return nil, apperr.New(apperr.KindInvalid, "invalid_request", "invoice_id is required.")
	}
	decision, err := s.Evaluate(ctx, organizationID, "supplier.followup", amountMinor)
	if err != nil {
		return nil, err
	}
	if !decision.Allowed {
		return nil, apperr.New(apperr.KindForbidden, "policy_denied", "Policy denied supplier follow-up: "+decision.Reason)
	}
	approvalID, err := s.RequestApprovalAs(ctx, organizationID, "supplier.followup", "invoice", invoiceID, actor)
	if err != nil {
		return nil, err
	}
	return &FollowUp{ApprovalID: approvalID, Status: "pending", PolicyReason: decision.Reason}, nil
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

// StartOperation records an idempotent operation and enqueues its execution job
// in the same transaction. A referenced approval must already be approved.
// Duplicate idempotency keys return the existing operation without enqueueing.
func (s *Service) StartOperation(ctx context.Context, organizationID, approvalID, operationType, idempotencyKey string, payload []byte) (string, bool, error) {
	if idempotencyKey == "" {
		return "", false, fmt.Errorf("idempotency key is required")
	}
	if s.queue == nil {
		return "", false, fmt.Errorf("job queue is not configured")
	}
	if len(payload) == 0 {
		payload = []byte(`{}`)
	}
	if !json.Valid(payload) {
		return "", false, fmt.Errorf("operation payload is not valid JSON")
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
		if errors.Is(err, pgx.ErrNoRows) {
			var sameRequest bool
			if err := tx.QueryRow(ctx, `
				SELECT id, (type = $3 AND payload = $4::jsonb)
				FROM operations WHERE organization_id = $1 AND idempotency_key = $2`,
				organizationID, idempotencyKey, operationType, payload).Scan(&id, &sameRequest); err != nil {
				return err
			}
			if !sameRequest {
				return fmt.Errorf("%w: %s", ErrIdempotencyConflict, idempotencyKey)
			}
			return nil
		}
		if err != nil {
			return err
		}
		created = true
		jobPayload, err := json.Marshal(map[string]string{
			"organization_id": organizationID,
			"operation_id":    id,
		})
		if err != nil {
			return fmt.Errorf("encode action.execute payload: %w", err)
		}
		if _, _, err := s.queue.Enqueue(ctx, tx, jobs.NewJob{
			OrganizationID: organizationID,
			Type:           ActionExecuteJob,
			SchemaVersion:  1,
			Payload:        jobPayload,
			IdempotencyKey: id,
		}); err != nil {
			return fmt.Errorf("enqueue operation: %w", err)
		}
		return audit.Record(ctx, tx, audit.Event{
			OrganizationID: organizationID, ActorKind: "system", Action: "operation.started",
			ResourceType: "operation", ResourceID: id, Decision: "allow",
			Metadata: map[string]any{"type": operationType},
		})
	})
	if err != nil {
		return "", false, err
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
			return fmt.Errorf("%w: operation %s", ErrOperationTerminal, operationID)
		}
		if _, err = tx.Exec(ctx, `
			INSERT INTO operation_attempts (organization_id, operation_id, attempt, status, error, finished_at)
			VALUES ($1, $2,
				(SELECT coalesce(max(attempt), 0) + 1 FROM operation_attempts WHERE operation_id = $2),
				$3, nullif($4, ''), now())`, organizationID, operationID, status, errorText); err != nil {
			return err
		}
		metadata := map[string]any{}
		if errorText != "" {
			metadata["error"] = errorText
		}
		return audit.Record(ctx, tx, audit.Event{
			OrganizationID: organizationID, ActorKind: "system", Action: "operation." + status,
			ResourceType: "operation", ResourceID: operationID,
			Decision: map[string]string{"succeeded": "allow", "failed": "deny", "cancelled": "deny"}[status],
			Metadata: metadata,
		})
	})
}

// ExpireApprovals marks every pending approval past its expiry as expired and
// records an audit event per approval. The cross-organization query runs in a
// security-definer function so no tenant context is bypassed by the caller.
func (s *Service) ExpireApprovals(ctx context.Context) (int, error) {
	var expired int
	if err := s.pool.QueryRow(ctx, `SELECT atlas_expire_approvals()`).Scan(&expired); err != nil {
		return 0, fmt.Errorf("expire approvals: %w", err)
	}
	return expired, nil
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

// Approval is the approval read model.
type Approval struct {
	ID             string     `json:"id"`
	OperationType  string     `json:"operation_type"`
	ObjectType     string     `json:"object_type"`
	ObjectID       string     `json:"object_id"`
	Status         string     `json:"status"`
	RequestedBy    *string    `json:"requested_by,omitempty"`
	DecidedBy      *string    `json:"decided_by,omitempty"`
	DecisionReason *string    `json:"decision_reason,omitempty"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	DecidedAt      *time.Time `json:"decided_at,omitempty"`
}

const approvalColumns = `id, operation_type, object_type, object_id, status,
	requested_by::text, decided_by::text, decision_reason, expires_at, created_at, decided_at`

func scanApproval(row pgx.Row) (*Approval, error) {
	var approval Approval
	err := row.Scan(&approval.ID, &approval.OperationType, &approval.ObjectType, &approval.ObjectID,
		&approval.Status, &approval.RequestedBy, &approval.DecidedBy, &approval.DecisionReason,
		&approval.ExpiresAt, &approval.CreatedAt, &approval.DecidedAt)
	return &approval, err
}

// GetApproval returns one approval.
func (s *Service) GetApproval(ctx context.Context, organizationID, approvalID string) (*Approval, error) {
	var approval *Approval
	err := s.withOrg(ctx, organizationID, func(tx pgx.Tx) error {
		var err error
		approval, err = scanApproval(tx.QueryRow(ctx, `
			SELECT `+approvalColumns+` FROM approvals
			WHERE organization_id = $1 AND id = $2`, organizationID, approvalID))
		return err
	})
	if err != nil {
		return nil, err
	}
	return approval, nil
}

// ListApprovals returns approvals newest first. An empty status means all.
func (s *Service) ListApprovals(ctx context.Context, organizationID, status string, limit int) ([]Approval, error) {
	if status != "" && status != "pending" && status != "approved" && status != "rejected" && status != "expired" {
		return nil, fmt.Errorf("invalid approval status %q", status)
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	var approvals []Approval
	err := s.withOrg(ctx, organizationID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT `+approvalColumns+` FROM approvals
			WHERE organization_id = $1 AND ($2 = '' OR status = $2)
			ORDER BY created_at DESC LIMIT $3`, organizationID, status, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			approval, err := scanApproval(rows)
			if err != nil {
				return err
			}
			approvals = append(approvals, *approval)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return approvals, nil
}

// OperationAttempt is one execution attempt record.
type OperationAttempt struct {
	Attempt           int        `json:"attempt"`
	Status            string     `json:"status"`
	ExternalReference *string    `json:"external_reference,omitempty"`
	Error             *string    `json:"error,omitempty"`
	StartedAt         time.Time  `json:"started_at"`
	FinishedAt        *time.Time `json:"finished_at,omitempty"`
}

// Operation is the operation read model.
type Operation struct {
	ID             string             `json:"id"`
	Type           string             `json:"type"`
	Status         string             `json:"status"`
	IdempotencyKey string             `json:"idempotency_key"`
	ApprovalID     *string            `json:"approval_id,omitempty"`
	Result         json.RawMessage    `json:"result,omitempty"`
	LastError      *string            `json:"last_error,omitempty"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
	Attempts       []OperationAttempt `json:"attempts,omitempty"`
}

const operationColumns = `id, type, status, idempotency_key, approval_id::text, result, last_error,
	created_at, updated_at`

func scanOperation(row pgx.Row) (*Operation, []byte, error) {
	var operation Operation
	var result []byte
	err := row.Scan(&operation.ID, &operation.Type, &operation.Status, &operation.IdempotencyKey,
		&operation.ApprovalID, &result, &operation.LastError, &operation.CreatedAt, &operation.UpdatedAt)
	return &operation, result, err
}

// GetOperation returns one operation with its attempt history.
func (s *Service) GetOperation(ctx context.Context, organizationID, operationID string) (*Operation, error) {
	var operation *Operation
	err := s.withOrg(ctx, organizationID, func(tx pgx.Tx) error {
		var result []byte
		var err error
		operation, result, err = scanOperation(tx.QueryRow(ctx, `
			SELECT `+operationColumns+` FROM operations
			WHERE organization_id = $1 AND id = $2`, organizationID, operationID))
		if err != nil {
			return err
		}
		operation.Result = result
		rows, err := tx.Query(ctx, `
			SELECT attempt, status, external_reference, error, started_at, finished_at
			FROM operation_attempts WHERE organization_id = $1 AND operation_id = $2
			ORDER BY attempt`, organizationID, operationID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var attempt OperationAttempt
			if err := rows.Scan(&attempt.Attempt, &attempt.Status, &attempt.ExternalReference,
				&attempt.Error, &attempt.StartedAt, &attempt.FinishedAt); err != nil {
				return err
			}
			operation.Attempts = append(operation.Attempts, attempt)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return operation, nil
}

// ListOperations returns operations newest first. An empty status means all.
func (s *Service) ListOperations(ctx context.Context, organizationID, status string, limit int) ([]Operation, error) {
	if status != "" && status != "pending" && status != "running" && status != "succeeded" &&
		status != "failed" && status != "cancelled" {
		return nil, fmt.Errorf("invalid operation status %q", status)
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	var operations []Operation
	err := s.withOrg(ctx, organizationID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT `+operationColumns+` FROM operations
			WHERE organization_id = $1 AND ($2 = '' OR status = $2)
			ORDER BY created_at DESC LIMIT $3`, organizationID, status, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			operation, result, err := scanOperation(rows)
			if err != nil {
				return err
			}
			operation.Result = result
			operations = append(operations, *operation)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return operations, nil
}
