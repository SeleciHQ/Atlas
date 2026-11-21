package integration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"atlas/internal/canonical"
)

type fixtureLine struct {
	Line           int    `json:"line"`
	Description    string `json:"description"`
	Quantity       string `json:"quantity"`
	Unit           string `json:"unit"`
	UnitPriceMinor int64  `json:"unit_price_minor"`
	Currency       string `json:"currency"`
}

type fixtureOrder struct {
	Number     string        `json:"number"`
	TotalMinor int64         `json:"total_minor"`
	Currency   string        `json:"currency"`
	Lines      []fixtureLine `json:"lines"`
}

type fixtureInvoice struct {
	Number     string        `json:"number"`
	TotalMinor int64         `json:"total_minor"`
	Currency   string        `json:"currency"`
	Lines      []fixtureLine `json:"lines"`
	PONumbers  []string      `json:"po_numbers"`
}

type fixtureDelivery struct {
	Number      string `json:"number"`
	PONumber    string `json:"po_number"`
	DeliveredOn string `json:"delivered_on"`
	Lines       []struct {
		PONumber string `json:"po_number"`
		Line     int    `json:"line"`
		Quantity string `json:"quantity"`
	} `json:"lines"`
}

type fixtureScenario struct {
	Name     string `json:"name"`
	Supplier struct {
		Name     string `json:"name"`
		Currency string `json:"currency"`
	} `json:"supplier"`
	Orders     []fixtureOrder    `json:"orders"`
	Invoices   []fixtureInvoice  `json:"invoices"`
	Deliveries []fixtureDelivery `json:"deliveries"`
	Expect     struct {
		InvoicePOLinks int    `json:"invoice_po_links"`
		Fulfilled      string `json:"fulfilled"`
	} `json:"expect"`
}

func TestCanonicalFixtureCorpus(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(backendRoot(t), "internal", "canonical", "testdata", "scenarios.json"))
	if err != nil {
		t.Fatalf("read fixture corpus: %v", err)
	}
	var corpus struct {
		Scenarios []fixtureScenario `json:"scenarios"`
	}
	if err := json.Unmarshal(data, &corpus); err != nil {
		t.Fatalf("decode fixture corpus: %v", err)
	}
	if len(corpus.Scenarios) == 0 {
		t.Fatal("fixture corpus is empty")
	}

	admin, app, _ := setupTenantSchema(t)
	service := canonical.NewService(app)
	ctx := context.Background()

	for _, scenario := range corpus.Scenarios {
		organizationID := seedQueueOrganization(t, admin)
		supplier, err := service.CreateSupplier(ctx, organizationID, scenario.Supplier.Name, scenario.Supplier.Currency)
		if err != nil {
			t.Fatalf("%s: create supplier: %v", scenario.Name, err)
		}
		orders := map[string]string{}
		poLines := map[string]map[int]string{}
		for _, order := range scenario.Orders {
			created, err := service.CreatePurchaseOrder(ctx, organizationID, supplier.ID, order.Number, order.Currency, order.TotalMinor)
			if err != nil {
				t.Fatalf("%s: create order %s: %v", scenario.Name, order.Number, err)
			}
			orders[order.Number] = created.ID
			poLines[order.Number] = map[int]string{}
			for _, line := range order.Lines {
				createdLine, err := service.AddPOLine(ctx, organizationID, created.ID, line.Line, line.Description, line.Quantity, line.Unit, line.UnitPriceMinor, line.Currency)
				if err != nil {
					t.Fatalf("%s: add PO line: %v", scenario.Name, err)
				}
				poLines[order.Number][line.Line] = createdLine.ID
			}
		}
		for _, delivery := range scenario.Deliveries {
			created, err := service.AddDelivery(ctx, organizationID, supplier.ID, orders[delivery.PONumber], delivery.Number, delivery.DeliveredOn)
			if err != nil {
				t.Fatalf("%s: add delivery: %v", scenario.Name, err)
			}
			for _, line := range delivery.Lines {
				if err := service.AddDeliveryLine(ctx, organizationID, created.ID, poLines[line.PONumber][line.Line], line.Quantity); err != nil {
					t.Fatalf("%s: add delivery line: %v", scenario.Name, err)
				}
			}
		}
		var firstInvoice string
		for _, invoice := range scenario.Invoices {
			created, err := service.CreateInvoice(ctx, organizationID, supplier.ID, invoice.Number, invoice.Currency, invoice.TotalMinor)
			if err != nil {
				t.Fatalf("%s: create invoice: %v", scenario.Name, err)
			}
			if firstInvoice == "" {
				firstInvoice = created.ID
			}
			for _, line := range invoice.Lines {
				if _, err := service.AddInvoiceLine(ctx, organizationID, created.ID, line.Line, line.Description, line.Quantity, line.Unit, line.UnitPriceMinor, line.Currency); err != nil {
					t.Fatalf("%s: add invoice line: %v", scenario.Name, err)
				}
			}
			for _, number := range invoice.PONumbers {
				if err := service.LinkInvoicePO(ctx, organizationID, created.ID, orders[number], nil); err != nil {
					t.Fatalf("%s: link invoice PO: %v", scenario.Name, err)
				}
			}
		}
		if firstInvoice != "" {
			links, err := service.CountInvoicePOLinks(ctx, organizationID, firstInvoice)
			if err != nil || links != scenario.Expect.InvoicePOLinks {
				t.Fatalf("%s: invoice PO links = %d, %v; want %d", scenario.Name, links, err, scenario.Expect.InvoicePOLinks)
			}
		}
		if scenario.Expect.Fulfilled != "" {
			detail, err := service.GetPurchaseOrderDetail(ctx, organizationID, orders[scenario.Deliveries[0].PONumber])
			if err != nil {
				t.Fatalf("%s: PO detail: %v", scenario.Name, err)
			}
			if len(detail.Lines) != 1 || detail.Lines[0].Fulfilled != scenario.Expect.Fulfilled {
				t.Fatalf("%s: fulfilled = %+v, want %s", scenario.Name, detail.Lines, scenario.Expect.Fulfilled)
			}
		}
	}
}

func TestOntologyValidationFailsClosed(t *testing.T) {
	admin, app, _ := setupTenantSchema(t)
	organizationID := seedQueueOrganization(t, admin)
	service := canonical.NewService(app)
	ctx := context.Background()

	if _, err := admin.Exec(ctx, `UPDATE ontology_versions SET definitions = '{"entities":[],"relationships":[]}'::jsonb WHERE version = 1`); err != nil {
		t.Fatalf("corrupt ontology: %v", err)
	}
	if _, err := service.CreateSupplier(ctx, organizationID, "Should Fail", "EUR"); err == nil {
		t.Fatal("write succeeded with incomplete ontology definitions")
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO ontology_versions (version, status, definitions) VALUES (2, 'active', '{}'::jsonb)`); err != nil {
		t.Fatalf("insert duplicate active ontology: %v", err)
	}
	if _, err := service.ActiveOntology(ctx); err == nil {
		t.Fatal("multiple active ontology versions accepted")
	}
}
