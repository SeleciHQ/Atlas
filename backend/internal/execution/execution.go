// Package execution runs approved operations from the durable job queue. It is
// the only path that performs an operation's effect: adapters are selected by
// operation type, every terminal outcome is written through governance, and the
// operation ID is the provider-facing idempotency key.
//
// Execution is at-least-once. Re-running an operation that already reached a
// terminal state returns the recorded result instead of repeating the effect.
package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"atlas/internal/connectors"
	"atlas/internal/governance"
	"atlas/internal/tenancy"
)

// TypeConnectorWrite is the operation type executed by a connector's write
// path. Payload shape: {"integration_id":"...","kind":"...","payload":{...}}.
const TypeConnectorWrite = "connector.write"

// Operation is one approved unit of work handed to an adapter.
type Operation struct {
	ID             string
	OrganizationID string
	Type           string
	IdempotencyKey string
	Payload        json.RawMessage
}

// Result is the outcome reported for a successful operation.
type Result struct {
	Status    string `json:"status"`
	Reference string `json:"reference,omitempty"`
	Error     string `json:"error,omitempty"`
}

// Adapter performs the effect for one operation type.
type Adapter interface {
	Execute(ctx context.Context, operation Operation) (Result, error)
}

// WriteExecutor is the connector write boundary used by ConnectorWriteAdapter.
type WriteExecutor interface {
	ExecuteWrite(ctx context.Context, organizationID, integrationID string, operation connectors.WriteOperation) (connectors.WriteResult, error)
}

// ConnectorWriteAdapter executes low-risk external writes through a connector.
type ConnectorWriteAdapter struct{ executor WriteExecutor }

// NewConnectorWriteAdapter constructs the adapter.
func NewConnectorWriteAdapter(executor WriteExecutor) *ConnectorWriteAdapter {
	return &ConnectorWriteAdapter{executor: executor}
}

// Execute decodes the payload, calls the connector write path and maps the
// provider result. The operation ID is propagated as the idempotency key.
func (a *ConnectorWriteAdapter) Execute(ctx context.Context, operation Operation) (Result, error) {
	var request struct {
		IntegrationID string          `json:"integration_id"`
		Kind          string          `json:"kind"`
		Payload       json.RawMessage `json:"payload"`
	}
	if len(operation.Payload) > 0 {
		if err := json.Unmarshal(operation.Payload, &request); err != nil {
			return Result{}, Permanent(fmt.Errorf("decode connector.write payload: %w", err))
		}
	}
	if request.IntegrationID == "" || request.Kind == "" {
		return Result{}, Permanent(errors.New("connector.write payload requires integration_id and kind"))
	}
	payload := request.Payload
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	written, err := a.executor.ExecuteWrite(ctx, operation.OrganizationID, request.IntegrationID, connectors.WriteOperation{
		Kind:           request.Kind,
		Payload:        payload,
		IdempotencyKey: operation.ID,
	})
	if err != nil {
		return Result{}, err
	}
	return Result{Status: written.Status, Reference: written.ExternalID}, nil
}

// Registry resolves operation types to adapters. Unknown types fail closed.
type Registry struct {
	mutex    sync.RWMutex
	adapters map[string]Adapter
}

// NewRegistry constructs an empty registry.
func NewRegistry() *Registry {
	return &Registry{adapters: map[string]Adapter{}}
}

// Register adds an adapter for an operation type.
func (r *Registry) Register(operationType string, adapter Adapter) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.adapters[operationType] = adapter
}

// Adapter returns the adapter for an operation type.
func (r *Registry) Adapter(operationType string) (Adapter, bool) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	adapter, ok := r.adapters[operationType]
	return adapter, ok
}

// Service claims and executes operations.
type Service struct {
	pool       *pgxpool.Pool
	governance *governance.Service
	registry   *Registry
}

// NewService constructs the execution service.
func NewService(pool *pgxpool.Pool, governanceService *governance.Service, registry *Registry) *Service {
	return &Service{pool: pool, governance: governanceService, registry: registry}
}

// Execute claims an operation and performs its effect. A terminal operation
// returns its recorded result without re-executing. Transient failures are
// returned to the job queue for retry; permanent failures close the operation.
func (s *Service) Execute(ctx context.Context, organizationID, operationID string) (json.RawMessage, error) {
	operation, terminal, err := s.claim(ctx, organizationID, operationID)
	if err != nil {
		return nil, err
	}
	if terminal != nil {
		return *terminal, nil
	}
	adapter, ok := s.registry.Adapter(operation.Type)
	if !ok {
		return s.markFailed(ctx, organizationID, operationID,
			fmt.Errorf("no execution adapter for operation type %q", operation.Type))
	}
	result, err := adapter.Execute(ctx, *operation)
	if err != nil {
		if isPermanent(err) {
			return s.markFailed(ctx, organizationID, operationID, err)
		}
		return nil, err
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("encode operation result: %w", err)
	}
	if err := s.governance.FinishOperation(ctx, organizationID, operationID, "succeeded", string(payload), ""); err != nil {
		if errors.Is(err, governance.ErrOperationTerminal) {
			return s.terminalResult(ctx, organizationID, operationID)
		}
		return nil, err
	}
	return payload, nil
}

// Fail closes an operation after the job queue exhausted its retries.
func (s *Service) Fail(ctx context.Context, organizationID, operationID string, cause error) error {
	if cause == nil {
		cause = errors.New("operation failed")
	}
	err := s.governance.FinishOperation(ctx, organizationID, operationID, "failed", "", cause.Error())
	if errors.Is(err, governance.ErrOperationTerminal) {
		return nil
	}
	return err
}

func (s *Service) markFailed(ctx context.Context, organizationID, operationID string, cause error) (json.RawMessage, error) {
	err := s.governance.FinishOperation(ctx, organizationID, operationID, "failed", "", cause.Error())
	if err != nil && !errors.Is(err, governance.ErrOperationTerminal) {
		return nil, err
	}
	payload, marshalErr := json.Marshal(Result{Status: "failed", Error: cause.Error()})
	if marshalErr != nil {
		return nil, marshalErr
	}
	return payload, nil
}

// claim moves a pending/running operation to running and returns it. When the
// operation is already terminal, its recorded result is returned instead.
func (s *Service) claim(ctx context.Context, organizationID, operationID string) (*Operation, *json.RawMessage, error) {
	var operation Operation
	var terminal json.RawMessage
	found := false
	err := tenancy.WithOrganizationTx(ctx, s.pool, organizationID, func(tx pgx.Tx) error {
		var payload []byte
		err := tx.QueryRow(ctx, `
			UPDATE operations SET status = 'running', updated_at = now()
			WHERE organization_id = $1 AND id = $2 AND status IN ('pending', 'running')
			RETURNING type, idempotency_key, payload`, organizationID, operationID).
			Scan(&operation.Type, &operation.IdempotencyKey, &payload)
		if err == nil {
			operation.ID = operationID
			operation.OrganizationID = organizationID
			operation.Payload = payload
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("claim operation: %w", err)
		}

		var status string
		var result []byte
		err = tx.QueryRow(ctx, `
			SELECT status, result FROM operations WHERE organization_id = $1 AND id = $2`,
			organizationID, operationID).Scan(&status, &result)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("operation %s not found", operationID)
		}
		if err != nil {
			return fmt.Errorf("load operation: %w", err)
		}
		if status != "succeeded" && status != "failed" && status != "cancelled" {
			return fmt.Errorf("operation %s is %s", operationID, status)
		}
		if len(result) > 0 {
			terminal = result
		} else {
			terminal, err = json.Marshal(Result{Status: status})
			if err != nil {
				return err
			}
		}
		found = true
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	if found {
		return nil, &terminal, nil
	}
	return &operation, nil, nil
}

func (s *Service) terminalResult(ctx context.Context, organizationID, operationID string) (json.RawMessage, error) {
	var status string
	var result []byte
	err := tenancy.WithOrganizationTx(ctx, s.pool, organizationID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT status, result FROM operations WHERE organization_id = $1 AND id = $2`,
			organizationID, operationID).Scan(&status, &result)
	})
	if err != nil {
		return nil, err
	}
	if len(result) > 0 {
		return result, nil
	}
	return json.Marshal(Result{Status: status})
}

type permanentError struct{ err error }

func (e *permanentError) Error() string { return e.err.Error() }
func (e *permanentError) Unwrap() error { return e.err }

// Permanent marks an adapter error as non-retryable.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return &permanentError{err: err}
}

func isPermanent(err error) bool {
	var target *permanentError
	return errors.As(err, &target)
}
