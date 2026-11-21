// Package tenancy establishes and propagates the organization context used by
// PostgreSQL Row-Level Security.
package tenancy

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type contextKey struct{}

// WithOrganization stores the trusted organization identifier in the context.
func WithOrganization(ctx context.Context, organizationID string) context.Context {
	return context.WithValue(ctx, contextKey{}, organizationID)
}

// OrganizationFrom returns the organization identifier from the context.
func OrganizationFrom(ctx context.Context) (string, bool) {
	value, ok := ctx.Value(contextKey{}).(string)
	return value, ok && value != ""
}

// BeginOrganizationTx starts a transaction and binds the RLS organization
// setting for its duration. Callers must commit or roll back the transaction.
func BeginOrganizationTx(ctx context.Context, pool *pgxpool.Pool, organizationID string) (pgx.Tx, error) {
	if _, err := uuid.Parse(organizationID); err != nil {
		return nil, fmt.Errorf("invalid organization id: %w", err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin organization transaction: %w", err)
	}
	if _, err := tx.Exec(ctx, `SELECT set_config('app.organization_id', $1, true)`, organizationID); err != nil {
		_ = tx.Rollback(ctx)
		return nil, fmt.Errorf("bind organization context: %w", err)
	}
	return tx, nil
}

// WithOrganizationTx runs fn inside a transaction scoped to organizationID.
func WithOrganizationTx(ctx context.Context, pool *pgxpool.Pool, organizationID string, fn func(pgx.Tx) error) error {
	tx, err := BeginOrganizationTx(ctx, pool, organizationID)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
