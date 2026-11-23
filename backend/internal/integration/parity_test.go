package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
	"time"

	"atlas/internal/agentaccess"
	"atlas/internal/authorization"
	"atlas/internal/canonical"
	"atlas/internal/governance"
	"atlas/internal/httpapi"
	"atlas/internal/identity"
	"atlas/internal/jobs"
	"atlas/internal/mcpapi"
	"atlas/internal/orgs"
	"atlas/internal/query"
	"atlas/internal/reconciliation"
)

// TestRESTAndMCPParity proves both transports return identical payloads for
// shared read capabilities: the same agent, services and organization are used.
func TestRESTAndMCPParity(t *testing.T) {
	admin, app, auth := setupTenantSchema(t)
	tenants := seedTenants(t, admin)
	ctx := context.Background()

	canon := canonical.NewService(app)
	recon := reconciliation.NewService(app)
	queryService := query.NewService(app, canon, recon)
	governanceService := governance.NewService(app, jobs.NewQueue(app))
	agents := agentaccess.NewService(app)
	sessions := identity.NewSessionService(auth, time.Hour)

	supplier, err := canon.CreateSupplier(ctx, tenants.OrganizationA, "Parity Supplies", "EUR")
	if err != nil {
		t.Fatalf("create supplier: %v", err)
	}
	invoice, err := canon.CreateInvoice(ctx, tenants.OrganizationA, supplier.ID, "INV-PARITY-1", "EUR", 2500)
	if err != nil {
		t.Fatalf("create invoice: %v", err)
	}
	if _, err := recon.Run(ctx, tenants.OrganizationA, invoice.ID); err != nil {
		t.Fatalf("reconciliation run: %v", err)
	}

	connection, err := agents.CreateConnection(ctx, tenants.OrganizationA, "parity-agent",
		[]string{agentaccess.ScopeInvoicesRead, agentaccess.ScopeDiscrepanciesRead}, tenants.AdminUserA)
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	issued, err := agents.IssueToken(ctx, tenants.OrganizationA, connection.ID, tenants.AdminUserA, time.Hour)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	mcpHandler := mcpapi.NewHandler(mcpapi.Deps{
		Logger:     testLogger(),
		Agents:     agents,
		Authorizer: authorization.NewService(),
		Query:      queryService,
		Governance: governanceService,
	})
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
		Query:          queryService,
		Governance:     governanceService,
		Reconciliation: recon,
		DevAuthEnabled: true,
	})

	restGet := func(path string) map[string]any {
		t.Helper()
		response := bearerRequest(t, router, http.MethodGet, path, issued.Token)
		if response.Code != http.StatusOK {
			t.Fatalf("REST %s status = %d, body = %s", path, response.Code, response.Body.String())
		}
		var body map[string]any
		decodeBody(t, response, &body)
		return body
	}
	mcpCall := func(name string, arguments map[string]any) map[string]any {
		t.Helper()
		response := mcpPost(t, mcpHandler, issued.Token, "", map[string]any{
			"jsonrpc": "2.0", "id": 1, "method": "tools/call",
			"params": map[string]any{"name": name, "arguments": arguments},
		})
		if response.Code != http.StatusOK {
			t.Fatalf("MCP %s status = %d, body = %s", name, response.Code, response.Body.String())
		}
		var body mcpResponse
		decodeBody(t, response, &body)
		if body.Error != nil {
			t.Fatalf("MCP %s error = %+v", name, body.Error)
		}
		var envelope struct {
			StructuredContent map[string]any `json:"structuredContent"`
			IsError           bool           `json:"isError"`
		}
		if err := json.Unmarshal(body.Result, &envelope); err != nil || envelope.IsError {
			t.Fatalf("MCP %s result = %s %v", name, body.Result, err)
		}
		return envelope.StructuredContent
	}

	restInvoices := restGet("/v1/invoices")["invoices"]
	mcpInvoices := mcpCall("search_invoices", map[string]any{})["invoices"]
	if !reflect.DeepEqual(restInvoices, mcpInvoices) {
		t.Fatalf("invoice search parity mismatch:\nrest=%v\nmcp=%v", restInvoices, mcpInvoices)
	}

	restInvoice := restGet("/v1/invoices/" + invoice.ID)["invoice"]
	mcpInvoice := mcpCall("get_invoice", map[string]any{"invoice_id": invoice.ID})["invoice"]
	if !reflect.DeepEqual(restInvoice, mcpInvoice) {
		t.Fatalf("invoice detail parity mismatch:\nrest=%v\nmcp=%v", restInvoice, mcpInvoice)
	}

	restDiscrepancies := restGet("/v1/discrepancies?invoice_id=" + invoice.ID)["discrepancies"]
	mcpDiscrepancies := mcpCall("search_discrepancies", map[string]any{"invoice_id": invoice.ID})["discrepancies"]
	if !reflect.DeepEqual(restDiscrepancies, mcpDiscrepancies) {
		t.Fatalf("discrepancy parity mismatch:\nrest=%v\nmcp=%v", restDiscrepancies, mcpDiscrepancies)
	}
}
