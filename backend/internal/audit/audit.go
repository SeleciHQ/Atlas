// Package audit writes append-only audit attribution records.
package audit

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
)

// Executor is satisfied by both pgxpool.Pool and pgx.Tx.
type Executor interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// Event is an audit attribution record. OrganizationID must match the active
// RLS organization context; the INSERT policy rejects mismatches.
type Event struct {
	OrganizationID string
	ActorKind      string // human | agent | integration | workload | system
	ActorID        string
	ActorDisplay   string
	Action         string
	ResourceType   string
	ResourceID     string
	Decision       string // allow | deny | require_approval | recorded
	Metadata       map[string]any
}

// Record appends an audit event using exec (a tenant-scoped transaction is
// expected for tenant actions).
func Record(ctx context.Context, exec Executor, event Event) error {
	if event.OrganizationID == "" {
		return fmt.Errorf("audit event requires an organization")
	}
	if event.ActorKind == "" || event.Action == "" || event.ResourceType == "" {
		return fmt.Errorf("audit event requires actor kind, action and resource type")
	}
	if event.Decision == "" {
		event.Decision = "recorded"
	}
	metadata := event.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	payload, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal audit metadata: %w", err)
	}
	_, err = exec.Exec(ctx, `
		INSERT INTO audit_events
			(organization_id, actor_kind, actor_id, actor_display, action,
			 resource_type, resource_id, decision, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		event.OrganizationID,
		event.ActorKind,
		nullIfEmpty(event.ActorID),
		nullIfEmpty(event.ActorDisplay),
		event.Action,
		event.ResourceType,
		nullIfEmpty(event.ResourceID),
		event.Decision,
		payload,
	)
	if err != nil {
		return fmt.Errorf("append audit event: %w", err)
	}
	return nil
}

func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}
