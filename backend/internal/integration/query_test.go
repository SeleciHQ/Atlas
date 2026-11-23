package integration

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"atlas/internal/agentaccess"
	"atlas/internal/authorization"
	"atlas/internal/canonical"
	"atlas/internal/httpapi"
	"atlas/internal/identity"
	"atlas/internal/orgs"
	"atlas/internal/query"
	"atlas/internal/reconciliation"
)

func seedInvoiceScenario(t *testing.T, appPool, adminPool *pgxpool.Pool, organizationID string) (string, string) {
	t.Helper()
	ctx := context.Background()
	canon := canonical.NewService(appPool)
	supplier, err := canon.CreateSupplier(ctx, organizationID, "Alpha Supplies", "EUR")
	if err != nil {
		t.Fatalf("create supplier: %v", err)
	}
	order, err := canon.CreatePurchaseOrder(ctx, organizationID, supplier.ID, "PO-Q1", "EUR", 10000)
	if err != nil {
		t.Fatalf("create purchase order: %v", err)
	}
	if _, err := canon.AddPOLine(ctx, organizationID, order.ID, 1, "Widget", "10", "unit", 1000, "EUR"); err != nil {
		t.Fatalf("add PO line: %v", err)
	}
	invoice, err := canon.CreateInvoice(ctx, organizationID, supplier.ID, "INV-Q1", "EUR", 9999)
	if err != nil {
		t.Fatalf("create invoice: %v", err)
	}
	if _, err := canon.AddInvoiceLine(ctx, organizationID, invoice.ID, 1, "Widget", "10", "unit", 1000, "EUR"); err != nil {
		t.Fatalf("add invoice line: %v", err)
	}
	if err := canon.LinkInvoicePO(ctx, organizationID, invoice.ID, order.ID, nil); err != nil {
		t.Fatalf("link invoice PO: %v", err)
	}
	delivery, err := canon.AddDelivery(ctx, organizationID, supplier.ID, order.ID, "DN-Q1", "2026-11-10")
	if err != nil {
		t.Fatalf("add delivery: %v", err)
	}
	if err := canon.AddDeliveryLine(ctx, organizationID, delivery.ID, mustPOLineID(t, adminPool, organizationID, order.ID), "5"); err != nil {
		t.Fatalf("add delivery line: %v", err)
	}
	return supplier.ID, invoice.ID
}

func mustPOLineID(t *testing.T, pool *pgxpool.Pool, organizationID, purchaseOrderID string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(), `
		SELECT id FROM po_lines WHERE organization_id = $1 AND purchase_order_id = $2 AND line_number = 1`,
		organizationID, purchaseOrderID).Scan(&id); err != nil {
		t.Fatalf("read PO line: %v", err)
	}
	return id
}

func TestQueryReadModels(t *testing.T) {
	admin, app, _ := setupTenantSchema(t)
	organizationID := seedQueueOrganization(t, admin)
	organizationB := seedQueueOrganization(t, admin)
	ctx := context.Background()
	supplierID, invoiceID := seedInvoiceScenario(t, app, admin, organizationID)

	canon := canonical.NewService(app)
	recon := reconciliation.NewService(app)
	service := query.NewService(app, canon, recon)
	run, err := recon.Run(ctx, organizationID, invoiceID)
	if err != nil {
		t.Fatalf("reconciliation run: %v", err)
	}
	if run == nil {
		t.Fatal("reconciliation run returned nil")
	}

	invoices, err := service.SearchInvoices(ctx, organizationID, query.InvoiceFilters{})
	if err != nil || len(invoices) != 1 || invoices[0].ID != invoiceID || invoices[0].SupplierName != "Alpha Supplies" {
		t.Fatalf("invoices = %+v %v", invoices, err)
	}
	filtered, err := service.SearchInvoices(ctx, organizationID, query.InvoiceFilters{SupplierID: supplierID, Number: "INV-Q"})
	if err != nil || len(filtered) != 1 {
		t.Fatalf("filtered invoices = %+v %v", filtered, err)
	}
	if missing, err := service.SearchInvoices(ctx, organizationID, query.InvoiceFilters{Currency: "USD"}); err != nil || len(missing) != 0 {
		t.Fatalf("currency filter = %+v %v", missing, err)
	}
	if _, err := service.SearchInvoices(ctx, organizationID, query.InvoiceFilters{SupplierID: "not-a-uuid"}); err == nil {
		t.Fatal("invalid supplier filter was accepted")
	}
	if other, err := service.SearchInvoices(ctx, organizationB, query.InvoiceFilters{}); err != nil || len(other) != 0 {
		t.Fatalf("cross-tenant invoices = %+v %v", other, err)
	}

	orders, err := service.SearchPurchaseOrders(ctx, organizationID, query.PurchaseOrderFilters{})
	if err != nil || len(orders) != 1 || orders[0].LineCount != 1 {
		t.Fatalf("purchase orders = %+v %v", orders, err)
	}
	suppliers, err := service.SearchSuppliers(ctx, organizationID, "alpha", 10)
	if err != nil || len(suppliers) != 1 || suppliers[0].InvoiceCount != 1 {
		t.Fatalf("suppliers = %+v %v", suppliers, err)
	}
	deliveries, err := service.ListDeliveries(ctx, organizationID, invoiceID, "", 10)
	if err != nil || len(deliveries) != 1 || deliveries[0].Number != "DN-Q1" {
		t.Fatalf("deliveries by invoice = %+v %v", deliveries, err)
	}
	byOrder, err := service.ListDeliveries(ctx, organizationID, "", orders[0].ID, 10)
	if err != nil || len(byOrder) != 1 {
		t.Fatalf("deliveries by order = %+v %v", byOrder, err)
	}
	if _, err := service.ListDeliveries(ctx, organizationID, "", "", 10); err == nil {
		t.Fatal("deliveries without a filter were accepted")
	}

	findings, err := service.SearchDiscrepancies(ctx, organizationID, query.DiscrepancyFilters{InvoiceID: invoiceID})
	if err != nil || len(findings) == 0 || findings[0].InvoiceID != invoiceID {
		t.Fatalf("discrepancies = %+v %v", findings, err)
	}
	if _, err := service.SearchDiscrepancies(ctx, organizationID, query.DiscrepancyFilters{Severity: "critical"}); err == nil {
		t.Fatal("invalid severity was accepted")
	}

	if detail, err := service.GetInvoice(ctx, organizationID, invoiceID); err != nil || detail.Invoice.ID != invoiceID {
		t.Fatalf("invoice detail = %+v %v", detail, err)
	}
	if detail, err := service.GetSupplier(ctx, organizationID, supplierID); err != nil || detail.Supplier.Name != "Alpha Supplies" {
		t.Fatalf("supplier detail = %+v %v", detail, err)
	}
	if detail, err := service.GetPurchaseOrder(ctx, organizationID, orders[0].ID); err != nil || detail.Order.Number != "PO-Q1" {
		t.Fatalf("purchase order detail = %+v %v", detail, err)
	}
	if detail, err := service.GetReconciliation(ctx, organizationID, run.RunID); err != nil || detail.RunID != run.RunID {
		t.Fatalf("reconciliation detail = %+v %v", detail, err)
	}
}

func TestQueryHTTPAgentScopes(t *testing.T) {
	admin, app, auth := setupTenantSchema(t)
	tenants := seedTenants(t, admin)
	canon := canonical.NewService(app)
	supplier, err := canon.CreateSupplier(context.Background(), tenants.OrganizationA, "Beta Parts", "EUR")
	if err != nil {
		t.Fatalf("create supplier: %v", err)
	}
	invoice, err := canon.CreateInvoice(context.Background(), tenants.OrganizationA, supplier.ID, "INV-HTTP-1", "EUR", 500)
	if err != nil {
		t.Fatalf("create invoice: %v", err)
	}

	agents := agentaccess.NewService(app)
	connection, err := agents.CreateConnection(context.Background(), tenants.OrganizationA, "read-agent",
		[]string{agentaccess.ScopeInvoicesRead}, tenants.AdminUserA)
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	issued, err := agents.IssueToken(context.Background(), tenants.OrganizationA, connection.ID, tenants.AdminUserA, time.Hour)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	otherConnection, err := agents.CreateConnection(context.Background(), tenants.OrganizationA, "approval-agent",
		[]string{agentaccess.ScopeApprovalsRequest}, tenants.AdminUserA)
	if err != nil {
		t.Fatalf("create other agent: %v", err)
	}
	otherToken, err := agents.IssueToken(context.Background(), tenants.OrganizationA, otherConnection.ID, tenants.AdminUserA, time.Hour)
	if err != nil {
		t.Fatalf("issue other token: %v", err)
	}

	sessions := identity.NewSessionService(auth, time.Hour)
	router := httpapi.NewRouter(httpapi.Deps{
		Logger:         testLogger(),
		Pool:           app,
		Migrations:     migrationsFS(t),
		ServiceName:    "atlas-api",
		Version:        "test",
		Identity:       sessions,
		DevSessions:    sessions,
		Authorizer:     authorization.NewService(),
		Orgs:           orgs.NewService(app),
		Agents:         agents,
		Query:          query.NewService(app, canon, reconciliation.NewService(app)),
		DevAuthEnabled: true,
	})

	response := bearerRequest(t, router, http.MethodGet, "/v1/invoices", issued.Token)
	if response.Code != http.StatusOK {
		t.Fatalf("agent invoices status = %d, body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Invoices []query.InvoiceSummary `json:"invoices"`
	}
	decodeBody(t, response, &body)
	if len(body.Invoices) != 1 || body.Invoices[0].ID != invoice.ID {
		t.Fatalf("agent invoices = %+v", body)
	}
	if response := bearerRequest(t, router, http.MethodGet, "/v1/invoices", otherToken.Token); response.Code != http.StatusForbidden {
		t.Fatalf("wrong-scope status = %d, want 403", response.Code)
	}
	if response := bearerRequest(t, router, http.MethodGet, "/v1/invoices?supplier_id=nope", issued.Token); response.Code != http.StatusBadRequest {
		t.Fatalf("invalid filter status = %d, want 400", response.Code)
	}

	memberCookie := login(t, router, tenants.OrganizationA, tenants.MemberEmailA)
	if response := request(t, router, http.MethodGet, "/v1/invoices", memberCookie, nil); response.Code != http.StatusOK {
		t.Fatalf("member invoices status = %d", response.Code)
	}
}
