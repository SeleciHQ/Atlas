package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"atlas/internal/canonical"
	"atlas/internal/fx"
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

func TestECBRateSnapshot(t *testing.T) {
	admin, app, _ := setupTenantSchema(t)
	organizationID := seedQueueOrganization(t, admin)
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<gesmes:Envelope xmlns:gesmes="http://www.gesmes.org/xml/2002-08-01">
<Cube><Cube time="2026-11-21"><Cube currency="USD" rate="1.1000"/></Cube></Cube></gesmes:Envelope>`))
	}))
	defer server.Close()

	service := fx.NewService(app, &fx.ECBProvider{URL: server.URL, Client: server.Client()})
	recorded, err := service.RefreshECB(ctx, organizationID)
	if err != nil || recorded != 1 {
		t.Fatalf("refresh = %d %v", recorded, err)
	}
	if again, err := service.RefreshECB(ctx, organizationID); err != nil || again != 0 {
		t.Fatalf("duplicate refresh = %d %v", again, err)
	}
	var source string
	var rate string
	if err := admin.QueryRow(ctx, `
		SELECT source, rate::text FROM fx_rates
		WHERE organization_id = $1 AND base_currency = 'EUR' AND quote_currency = 'USD'`,
		organizationID).Scan(&source, &rate); err != nil {
		t.Fatalf("read fx rate: %v", err)
	}
	if source != "ecb" {
		t.Fatalf("source = %s", source)
	}
	converted, err := canonical.NewDatabaseConverter(app).ConvertMoney(ctx, organizationID, 1000, "EUR", "USD")
	if err != nil || converted != 1100 {
		t.Fatalf("converted = %d %v", converted, err)
	}
	organizationB := seedQueueOrganization(t, admin)
	if _, err := canonical.NewDatabaseConverter(app).ConvertMoney(ctx, organizationB, 1000, "EUR", "USD"); err == nil {
		t.Fatal("cross-tenant FX rate was visible")
	}
}
