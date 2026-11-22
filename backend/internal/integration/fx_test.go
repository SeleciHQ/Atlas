package integration

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"atlas/internal/canonical"
)

func TestDatabaseConverter(t *testing.T) {
	admin, app, _ := setupTenantSchema(t)
	organizationID := seedQueueOrganization(t, admin)
	ctx := context.Background()
	if _, err := admin.Exec(ctx, `INSERT INTO fx_rates (organization_id, base_currency, quote_currency, rate) VALUES ($1, 'EUR', 'USD', 0.92000000)`, organizationID); err != nil {
		t.Fatalf("insert fx: %v", err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO uom_conversions (organization_id, from_unit, to_unit, factor) VALUES ($1, 'piece', 'kg', 2.50000000)`, organizationID); err != nil {
		t.Fatalf("insert uom: %v", err)
	}
	converter := canonical.NewDatabaseConverter(app)
	amount, err := converter.ConvertMoney(ctx, organizationID, 1000, "EUR", "USD")
	if err != nil || amount != 920 {
		t.Fatalf("money = %d %v", amount, err)
	}
	quantity, err := converter.ConvertQuantity(ctx, organizationID, "2.250000", "piece", "kg")
	if err != nil {
		t.Fatalf("quantity: %v", err)
	}
	parsed, err := strconv.ParseFloat(strings.TrimSpace(quantity), 64)
	if err != nil || parsed != 5.625 {
		t.Fatalf("quantity = %q %v", quantity, err)
	}
	if _, err := converter.ConvertMoney(ctx, organizationID, 1000, "USD", "EUR"); err == nil {
		t.Fatal("missing reverse rate should fail closed")
	}
}
