// Package canonical: FX and unit-of-measure conversion interfaces (ADR-008).
// v0 ships deterministic defaults only: identity conversions are allowed and
// everything else fails closed. No exchange rates or unit factors are invented.
package canonical

import (
	"context"
	"fmt"
)

// Converter converts money across currencies and quantities across units.
type Converter interface {
	ConvertMoney(ctx context.Context, organizationID string, amountMinor int64, fromCurrency, toCurrency string) (int64, error)
	ConvertQuantity(ctx context.Context, organizationID, quantity, fromUnit, toUnit string) (string, error)
}

// DefaultConverter allows identity conversions only.
type DefaultConverter struct{}

// ConvertMoney returns the amount when currencies match, otherwise fails.
func (DefaultConverter) ConvertMoney(ctx context.Context, organizationID string, amountMinor int64, fromCurrency, toCurrency string) (int64, error) {
	if fromCurrency == toCurrency {
		return amountMinor, nil
	}
	return 0, fmt.Errorf("no FX rate available for %s->%s", fromCurrency, toCurrency)
}

// ConvertQuantity returns the quantity when units match, otherwise fails.
func (DefaultConverter) ConvertQuantity(ctx context.Context, organizationID, quantity, fromUnit, toUnit string) (string, error) {
	if fromUnit == toUnit {
		return quantity, nil
	}
	return "", fmt.Errorf("no unit-of-measure factor available for %s->%s", fromUnit, toUnit)
}
