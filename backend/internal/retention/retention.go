// Package retention defines Atlas's v0 retention defaults and legal holds.
// Automated deletion sweeps are not enabled yet: a hold must be checked before
// any governed deletion, and releasing a hold is audited.
package retention

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"atlas/internal/audit"
	"atlas/internal/platform/apperr"
	"atlas/internal/tenancy"
)

// Class is a retention class from ADR-007.
type Class string

const (
	ClassDocuments      Class = "documents"
	ClassSourceRecords  Class = "source_records"
	ClassCanonical      Class = "canonical_records"
	ClassReconciliation Class = "reconciliation_history"
	ClassAudit          Class = "audit_events"
	ClassJobs           Class = "jobs_and_results"
	ClassCredentials    Class = "revoked_credentials"
	ClassTelemetry      Class = "telemetry"
)

// Policy is one retention default.
type Policy struct {
	Class       Class         `json:"class"`
	Description string        `json:"description"`
	Duration    time.Duration `json:"-"`
	Days        int           `json:"days"`
	Basis       string        `json:"basis"`
}

const day = 24 * time.Hour

var defaults = []Policy{
	{Class: ClassDocuments, Description: "Original documents and extracted content", Duration: 397 * day, Days: 397, Basis: "13 months after last activity (sandbox/v0)"},
	{Class: ClassSourceRecords, Description: "Raw and normalized source records", Duration: 397 * day, Days: 397, Basis: "13 months after ingestion; follows the document lifecycle"},
	{Class: ClassCanonical, Description: "Canonical invoices, POs, deliveries and links", Duration: 7 * 365 * day, Days: 7 * 365, Basis: "7 years (financial records)"},
	{Class: ClassReconciliation, Description: "Reconciliation runs, matches and discrepancies", Duration: 7 * 365 * day, Days: 7 * 365, Basis: "7 years (evidence for financial decisions)"},
	{Class: ClassAudit, Description: "Append-only audit events", Duration: 7 * 365 * day, Days: 7 * 365, Basis: "7 years; never deleted while a legal hold is active"},
	{Class: ClassJobs, Description: "Jobs, attempts and job results", Duration: 30 * day, Days: 30, Basis: "30 days operational telemetry"},
	{Class: ClassCredentials, Description: "Revoked integration/agent credentials and tokens", Duration: 90 * day, Days: 90, Basis: "90 days after revocation for incident review"},
	{Class: ClassTelemetry, Description: "Logs, traces and metrics", Duration: 30 * day, Days: 30, Basis: "30 days in the sandbox (ADR-006)"},
}

// Defaults returns the v0 retention policy table.
func Defaults() []Policy {
	return append([]Policy(nil), defaults...)
}

// ValidClass reports whether class is a known retention class.
func ValidClass(class Class) bool {
	for _, policy := range defaults {
		if policy.Class == class {
			return true
		}
	}
	return false
}

var validResourceTypes = map[string]struct{}{
	"document": {}, "source_record": {}, "canonical_invoice": {}, "canonical_purchase_order": {},
	"reconciliation_run": {}, "audit_event": {}, "organization_data": {},
}

// Hold is a legal hold that blocks deletion.
type Hold struct {
	ID           string     `json:"id"`
	ResourceType string     `json:"resource_type"`
	ResourceID   string     `json:"resource_id"`
	Reason       string     `json:"reason"`
	CreatedBy    *string    `json:"created_by,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	ReleasedBy   *string    `json:"released_by,omitempty"`
	ReleasedAt   *time.Time `json:"released_at,omitempty"`
}

// Service manages legal holds.
type Service struct{ pool *pgxpool.Pool }

// NewService constructs the retention service.
func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

const holdColumns = `id, resource_type, resource_id, reason, created_by::text, created_at,
	released_by::text, released_at`

func scanHold(row pgx.Row) (*Hold, error) {
	var hold Hold
	err := row.Scan(&hold.ID, &hold.ResourceType, &hold.ResourceID, &hold.Reason, &hold.CreatedBy,
		&hold.CreatedAt, &hold.ReleasedBy, &hold.ReleasedAt)
	return &hold, err
}

// PlaceHold creates an active legal hold. Only one active hold may exist per resource.
func (s *Service) PlaceHold(ctx context.Context, organizationID, resourceType, resourceID, reason, createdBy string) (*Hold, error) {
	resourceType = strings.TrimSpace(resourceType)
	resourceID = strings.TrimSpace(resourceID)
	reason = strings.TrimSpace(reason)
	if _, ok := validResourceTypes[resourceType]; !ok {
		return nil, apperr.New(apperr.KindInvalid, "invalid_resource_type", "Unknown legal-hold resource type.")
	}
	if resourceID == "" {
		return nil, apperr.New(apperr.KindInvalid, "invalid_request", "resource_id is required.")
	}
	if reason == "" || len(reason) > 1000 {
		return nil, apperr.New(apperr.KindInvalid, "invalid_request", "A reason of 1000 characters or fewer is required.")
	}
	var hold *Hold
	err := tenancy.WithOrganizationTx(ctx, s.pool, organizationID, func(tx pgx.Tx) error {
		var err error
		hold, err = scanHold(tx.QueryRow(ctx, `
			INSERT INTO legal_holds (organization_id, resource_type, resource_id, reason, created_by)
			VALUES ($1, $2, $3, $4, nullif($5, '')::uuid) RETURNING `+holdColumns,
			organizationID, resourceType, resourceID, reason, createdBy))
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return apperr.New(apperr.KindConflict, "active_hold_exists", "An active legal hold already exists for this resource.")
			}
			return err
		}
		return audit.Record(ctx, tx, audit.Event{
			OrganizationID: organizationID, ActorKind: "human", ActorID: createdBy,
			Action: "retention.legal_hold_placed", ResourceType: resourceType, ResourceID: resourceID,
			Metadata: map[string]any{"hold_id": hold.ID, "reason": reason},
		})
	})
	if err != nil {
		return nil, err
	}
	return hold, nil
}

// ReleaseHold releases an active hold and records who did it.
func (s *Service) ReleaseHold(ctx context.Context, organizationID, holdID, releasedBy string) error {
	if holdID == "" {
		return apperr.New(apperr.KindInvalid, "invalid_request", "hold id is required.")
	}
	return tenancy.WithOrganizationTx(ctx, s.pool, organizationID, func(tx pgx.Tx) error {
		var resourceType, resourceID string
		err := tx.QueryRow(ctx, `
			UPDATE legal_holds SET released_at = now(), released_by = nullif($3, '')::uuid
			WHERE organization_id = $1 AND id = $2 AND released_at IS NULL
			RETURNING resource_type, resource_id`, organizationID, holdID, releasedBy).
			Scan(&resourceType, &resourceID)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.New(apperr.KindNotFound, "hold_not_found", "Active legal hold not found.")
		}
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{
			OrganizationID: organizationID, ActorKind: "human", ActorID: releasedBy,
			Action: "retention.legal_hold_released", ResourceType: resourceType, ResourceID: resourceID,
			Metadata: map[string]any{"hold_id": holdID},
		})
	})
}

// ListHolds returns holds newest first.
func (s *Service) ListHolds(ctx context.Context, organizationID string, activeOnly bool, limit int) ([]Hold, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	var holds []Hold
	err := tenancy.WithOrganizationTx(ctx, s.pool, organizationID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT `+holdColumns+` FROM legal_holds
			WHERE organization_id = $1 AND (NOT $2 OR released_at IS NULL)
			ORDER BY created_at DESC LIMIT $3`, organizationID, activeOnly, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			hold, err := scanHold(rows)
			if err != nil {
				return err
			}
			holds = append(holds, *hold)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return holds, nil
}

// IsHeld reports whether an active legal hold exists for a resource.
func (s *Service) IsHeld(ctx context.Context, organizationID, resourceType, resourceID string) (bool, error) {
	var held bool
	err := tenancy.WithOrganizationTx(ctx, s.pool, organizationID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM legal_holds
				WHERE organization_id = $1 AND resource_type = $2 AND resource_id = $3 AND released_at IS NULL
			)`, organizationID, resourceType, resourceID).Scan(&held)
	})
	return held, err
}
