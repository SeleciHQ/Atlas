package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"atlas/internal/agentaccess"
	"atlas/internal/authorization"
	"atlas/internal/canonical"
	"atlas/internal/governance"
	"atlas/internal/jobs"
	"atlas/internal/mcpapi"
	"atlas/internal/query"
	"atlas/internal/reconciliation"
)

type mcpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func mcpPost(t *testing.T, handler http.Handler, token, version string, payload any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode MCP request: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if version != "" {
		request.Header.Set("MCP-Protocol-Version", version)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func TestMCPToolsAndAuthorization(t *testing.T) {
	admin, app, _ := setupTenantSchema(t)
	tenants := seedTenants(t, admin)
	ctx := context.Background()
	canon := canonical.NewService(app)
	recon := reconciliation.NewService(app)
	queue := jobs.NewQueue(app)
	governanceService := governance.NewService(app, queue)
	agents := agentaccess.NewService(app)
	handler := mcpapi.NewHandler(mcpapi.Deps{
		Logger:     testLogger(),
		Pool:       app,
		Agents:     agents,
		Authorizer: authorization.NewService(),
		Query:      query.NewService(app, canon, recon),
		Governance: governanceService,
		Queue:      queue,
	})

	supplier, err := canon.CreateSupplier(ctx, tenants.OrganizationA, "MCP Supplies", "EUR")
	if err != nil {
		t.Fatalf("create supplier: %v", err)
	}
	invoice, err := canon.CreateInvoice(ctx, tenants.OrganizationA, supplier.ID, "INV-MCP-1", "EUR", 750)
	if err != nil {
		t.Fatalf("create invoice: %v", err)
	}
	order, err := canon.CreatePurchaseOrder(ctx, tenants.OrganizationA, supplier.ID, "PO-MCP-1", "EUR", 750)
	if err != nil {
		t.Fatalf("create purchase order: %v", err)
	}
	if err := canon.LinkInvoicePO(ctx, tenants.OrganizationA, invoice.ID, order.ID, nil); err != nil {
		t.Fatalf("link invoice: %v", err)
	}
	if _, err := recon.Run(ctx, tenants.OrganizationA, invoice.ID); err != nil {
		t.Fatalf("reconciliation run: %v", err)
	}
	if _, err := governanceService.SetPolicy(ctx, tenants.OrganizationA, "default",
		[]byte(`{"allow_actions":["supplier.followup"]}`)); err != nil {
		t.Fatalf("set policy: %v", err)
	}

	allScopes := []string{
		agentaccess.ScopeInvoicesRead, agentaccess.ScopePurchaseOrdersRead,
		agentaccess.ScopeDeliveriesRead, agentaccess.ScopeSuppliersRead,
		agentaccess.ScopeReconciliationRead, agentaccess.ScopeReconciliationRun,
		agentaccess.ScopeDiscrepanciesRead, agentaccess.ScopeActionsRequest,
		agentaccess.ScopeApprovalsRequest, agentaccess.ScopeOperationsRead,
	}
	connection, err := agents.CreateConnection(ctx, tenants.OrganizationA, "mcp-agent", allScopes, tenants.AdminUserA)
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	issued, err := agents.IssueToken(ctx, tenants.OrganizationA, connection.ID, tenants.AdminUserA, time.Hour)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	limited, err := agents.CreateConnection(ctx, tenants.OrganizationA, "limited-agent",
		[]string{agentaccess.ScopeSuppliersRead}, tenants.AdminUserA)
	if err != nil {
		t.Fatalf("create limited agent: %v", err)
	}
	limitedToken, err := agents.IssueToken(ctx, tenants.OrganizationA, limited.ID, tenants.AdminUserA, time.Hour)
	if err != nil {
		t.Fatalf("issue limited token: %v", err)
	}

	if response := mcpPost(t, handler, "", "", map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
	}); response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated initialize status = %d", response.Code)
	}

	initialize := mcpPost(t, handler, issued.Token, "", map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": "2025-03-26"},
	})
	if initialize.Code != http.StatusOK {
		t.Fatalf("initialize status = %d, body = %s", initialize.Code, initialize.Body.String())
	}
	var initializeBody mcpResponse
	decodeBody(t, initialize, &initializeBody)
	var initResult struct {
		ProtocolVersion string `json:"protocolVersion"`
		ServerInfo      struct {
			Name string `json:"name"`
		} `json:"serverInfo"`
	}
	if err := json.Unmarshal(initializeBody.Result, &initResult); err != nil {
		t.Fatalf("decode initialize: %v", err)
	}
	if initResult.ProtocolVersion != "2025-03-26" || initResult.ServerInfo.Name != "atlas-mcp" {
		t.Fatalf("initialize result = %+v", initResult)
	}

	notification := mcpPost(t, handler, issued.Token, "", map[string]any{
		"jsonrpc": "2.0", "method": "notifications/initialized",
	})
	if notification.Code != http.StatusAccepted {
		t.Fatalf("notification status = %d", notification.Code)
	}

	tools := mcpPost(t, handler, issued.Token, "2025-03-26", map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "tools/list",
	})
	if tools.Code != http.StatusOK {
		t.Fatalf("tools/list status = %d, body = %s", tools.Code, tools.Body.String())
	}
	var toolsBody mcpResponse
	decodeBody(t, tools, &toolsBody)
	var toolsResult struct {
		Tools []struct {
			Name        string         `json:"name"`
			InputSchema map[string]any `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(toolsBody.Result, &toolsResult); err != nil {
		t.Fatalf("decode tools: %v", err)
	}
	if len(toolsResult.Tools) != 11 || toolsResult.Tools[0].InputSchema == nil {
		t.Fatalf("tools = %+v", toolsResult.Tools)
	}

	search := mcpPost(t, handler, issued.Token, "2025-03-26", map[string]any{
		"jsonrpc": "2.0", "id": 3, "method": "tools/call",
		"params": map[string]any{"name": "search_invoices", "arguments": map[string]any{}},
	})
	var searchBody mcpResponse
	decodeBody(t, search, &searchBody)
	var searchResult struct {
		StructuredContent struct {
			Invoices []query.InvoiceSummary `json:"invoices"`
		} `json:"structuredContent"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(searchBody.Result, &searchResult); err != nil {
		t.Fatalf("decode search result: %v", err)
	}
	if searchResult.IsError || len(searchResult.StructuredContent.Invoices) != 1 ||
		searchResult.StructuredContent.Invoices[0].ID != invoice.ID {
		t.Fatalf("search result = %s", searchBody.Result)
	}

	denied := mcpPost(t, handler, limitedToken.Token, "2025-03-26", map[string]any{
		"jsonrpc": "2.0", "id": 4, "method": "tools/call",
		"params": map[string]any{"name": "search_invoices", "arguments": map[string]any{}},
	})
	var deniedBody mcpResponse
	decodeBody(t, denied, &deniedBody)
	if deniedBody.Error == nil || deniedBody.Error.Code != -32003 {
		t.Fatalf("scope denial = %s", denied.Body.String())
	}

	unknown := mcpPost(t, handler, issued.Token, "", map[string]any{
		"jsonrpc": "2.0", "id": 5, "method": "tools/call",
		"params": map[string]any{"name": "nope", "arguments": map[string]any{}},
	})
	var unknownBody mcpResponse
	decodeBody(t, unknown, &unknownBody)
	if unknownBody.Error == nil || unknownBody.Error.Code != -32602 {
		t.Fatalf("unknown tool = %s", unknown.Body.String())
	}

	if response := mcpPost(t, handler, issued.Token, "1999-01-01", map[string]any{
		"jsonrpc": "2.0", "id": 6, "method": "tools/list",
	}); response.Code != http.StatusBadRequest {
		t.Fatalf("unsupported version status = %d", response.Code)
	}

	reconcile := mcpPost(t, handler, issued.Token, "", map[string]any{
		"jsonrpc": "2.0", "id": 7, "method": "tools/call",
		"params": map[string]any{"name": "reconcile_invoice", "arguments": map[string]any{"invoice_id": invoice.ID}},
	})
	var reconcileBody mcpResponse
	decodeBody(t, reconcile, &reconcileBody)
	var reconcileEnvelope struct {
		StructuredContent struct {
			JobID string `json:"job_id"`
		} `json:"structuredContent"`
	}
	if err := json.Unmarshal(reconcileBody.Result, &reconcileEnvelope); err != nil || reconcileEnvelope.StructuredContent.JobID == "" {
		t.Fatalf("reconcile result = %s %v", reconcileBody.Result, err)
	}
	reconcileResult := reconcileEnvelope.StructuredContent
	var jobType string
	if err := admin.QueryRow(ctx, `SELECT type FROM jobs WHERE organization_id = $1 AND id = $2`,
		tenants.OrganizationA, reconcileResult.JobID).Scan(&jobType); err != nil {
		t.Fatalf("read reconciliation job: %v", err)
	}
	if jobType != "reconciliation.run" {
		t.Fatalf("job type = %s", jobType)
	}

	followup := mcpPost(t, handler, issued.Token, "", map[string]any{
		"jsonrpc": "2.0", "id": 8, "method": "tools/call",
		"params": map[string]any{"name": "request_supplier_followup",
			"arguments": map[string]any{"invoice_id": invoice.ID, "amount_minor": 750, "note": "missing document"}},
	})
	var followupBody mcpResponse
	decodeBody(t, followup, &followupBody)
	var followupEnvelope struct {
		StructuredContent struct {
			ApprovalID string `json:"approval_id"`
			Status     string `json:"status"`
		} `json:"structuredContent"`
	}
	if err := json.Unmarshal(followupBody.Result, &followupEnvelope); err != nil ||
		followupEnvelope.StructuredContent.ApprovalID == "" || followupEnvelope.StructuredContent.Status != "pending" {
		t.Fatalf("followup result = %s %v", followupBody.Result, err)
	}
	var actorKind, actorID string
	if err := admin.QueryRow(ctx, `
		SELECT actor_kind, actor_id FROM audit_events
		WHERE organization_id = $1 AND action = 'approval.requested' AND resource_id = $2`,
		tenants.OrganizationA, invoice.ID).Scan(&actorKind, &actorID); err != nil {
		t.Fatalf("read approval audit: %v", err)
	}
	if actorKind != "agent" || actorID != connection.ID {
		t.Fatalf("approval audit = %s/%s", actorKind, actorID)
	}
}
