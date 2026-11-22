package canonical

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DatabaseConverter resolves FX rates and unit factors from organization-scoped
// tables. Identity conversions short-circuit; missing data fails closed.
type DatabaseConverter struct {
	pool *pgxpool.Pool
}

// NewDatabaseConverter constructs the table-backed converter.
func NewDatabaseConverter(pool *pgxpool.Pool) *DatabaseConverter {
	return &DatabaseConverter{pool: pool}
}

// ConvertMoney converts using the latest applicable rate for the pair.
func (c *DatabaseConverter) ConvertMoney(ctx context.Context, organizationID string, amountMinor int64, fromCurrency, toCurrency string) (int64, error) {
	if fromCurrency == toCurrency {
		return amountMinor, nil
	}
	var converted sql.NullFloat64
	err := c.withOrg(ctx, organizationID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT round($1::numeric * rate)::bigint
			FROM fx_rates WHERE organization_id = $2 AND base_currency = $3 AND quote_currency = $4
			ORDER BY effective_at DESC LIMIT 1`, amountMinor, organizationID, fromCurrency, toCurrency).
			Scan(&converted)
	})
	if err != nil || !converted.Valid {
		return 0, fmt.Errorf("no FX rate available for %s->%s", fromCurrency, toCurrency)
	}
	return int64(converted.Float64), nil
}

// ConvertQuantity applies the configured unit factor.
func (c *DatabaseConverter) ConvertQuantity(ctx context.Context, organizationID, quantity, fromUnit, toUnit string) (string, error) {
	if fromUnit == toUnit {
		return quantity, nil
	}
	var converted sql.NullString
	err := c.withOrg(ctx, organizationID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT ($1::numeric * factor)::text FROM uom_conversions
			WHERE organization_id = $2 AND from_unit = $3 AND to_unit = $4`,
			quantity, organizationID, fromUnit, toUnit).Scan(&converted)
	})
	if err != nil || !converted.Valid {
		return "", fmt.Errorf("no unit-of-measure factor available for %s->%s", fromUnit, toUnit)
	}
	return converted.String, nil
}

func (c *DatabaseConverter) withOrg(ctx context.Context, organizationID string, fn func(pgx.Tx) error) error {
	tx, err := c.pool.Begin(ctx)
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
