package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
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

// TestMCPReconciliationCorpusRegression drives the fixture corpus through the
// MCP entrypoint: reconcile via tool call, worker execution, then status and
// reconciliation reads over MCP.
func TestMCPReconciliationCorpusRegression(t *testing.T) {
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

	for _, scenario := range corpus.Scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			admin, app, _ := setupTenantSchema(t)
			organizationID := seedQueueOrganization(t, admin)
			seeded := seedScenario(t, app, organizationID, scenario)
			if len(seeded.Invoices) == 0 {
				t.Skip("scenario has no invoice")
			}
			invoiceID := seeded.Invoices[0]
			ctx := context.Background()
			canon := canonical.NewService(app)
			recon := reconciliation.NewService(app)
			queue := jobs.NewQueue(app)
			governanceService := governance.NewService(app, queue)
			agents := agentaccess.NewService(app)
			handler := mcpapi.NewHandler(mcpapi.Deps{
				Logger:     testLogger(),
				Agents:     agents,
				Authorizer: authorization.NewService(),
				Query:      query.NewService(app, canon, recon),
				Governance: governanceService,
			})
			connection, err := agents.CreateConnection(ctx, organizationID, "corpus-agent",
				[]string{agentaccess.ScopeReconciliationRun, agentaccess.ScopeReconciliationRead, agentaccess.ScopeOperationsRead}, "")
			if err != nil {
				t.Fatalf("create agent: %v", err)
			}
			issued, err := agents.IssueToken(ctx, organizationID, connection.ID, "", time.Hour)
			if err != nil {
				t.Fatalf("issue token: %v", err)
			}

			reconcileResponse := mcpPost(t, handler, issued.Token, "", map[string]any{
				"jsonrpc": "2.0", "id": 1, "method": "tools/call",
				"params": map[string]any{"name": "reconcile_invoice",
					"arguments": map[string]any{"invoice_id": invoiceID, "idempotency_key": "corpus-" + scenario.Name}},
			})
			var reconcileBody mcpResponse
			decodeBody(t, reconcileResponse, &reconcileBody)
			var reconcileEnvelope struct {
				StructuredContent struct {
					OperationID string `json:"operation_id"`
				} `json:"structuredContent"`
				IsError bool `json:"isError"`
			}
			if err := json.Unmarshal(reconcileBody.Result, &reconcileEnvelope); err != nil ||
				reconcileEnvelope.IsError || reconcileEnvelope.StructuredContent.OperationID == "" {
				t.Fatalf("reconcile result = %s %v", reconcileBody.Result, err)
			}
			operationID := reconcileEnvelope.StructuredContent.OperationID

			worker := connectRole(t, workerRole)
			workerQueue := jobs.NewQueue(worker)
			job, err := workerQueue.Claim(ctx, "corpus-worker", 30*time.Second)
			if err != nil || job == nil {
				t.Fatalf("claim job = %+v %v", job, err)
			}
			var jobPayload struct {
				OperationID string `json:"operation_id"`
				InvoiceID   string `json:"invoice_id"`
			}
			if err := json.Unmarshal(job.Payload, &jobPayload); err != nil {
				t.Fatalf("decode job payload: %v", err)
			}
			if jobPayload.OperationID != operationID || jobPayload.InvoiceID != invoiceID {
				t.Fatalf("job payload = %+v", jobPayload)
			}

			result, err := recon.Run(ctx, organizationID, invoiceID)
			if err != nil {
				t.Fatalf("run reconciliation: %v", err)
			}
			encoded, err := json.Marshal(result)
			if err != nil {
				t.Fatalf("encode reconciliation result: %v", err)
			}
			if err := governanceService.FinishOperation(ctx, organizationID, operationID, "succeeded", string(encoded), ""); err != nil {
				t.Fatalf("finish operation: %v", err)
			}
			if err := workerQueue.Complete(ctx, job, encoded); err != nil {
				t.Fatalf("complete job: %v", err)
			}

			statusResponse := mcpPost(t, handler, issued.Token, "", map[string]any{
				"jsonrpc": "2.0", "id": 2, "method": "tools/call",
				"params": map[string]any{"name": "get_operation_status",
					"arguments": map[string]any{"operation_id": operationID}},
			})
			var statusBody mcpResponse
			decodeBody(t, statusResponse, &statusBody)
			var statusEnvelope struct {
				StructuredContent struct {
					Operation struct {
						Status string `json:"status"`
					} `json:"operation"`
				} `json:"structuredContent"`
			}
			if err := json.Unmarshal(statusBody.Result, &statusEnvelope); err != nil ||
				statusEnvelope.StructuredContent.Operation.Status != "succeeded" {
				t.Fatalf("operation status = %s %v", statusBody.Result, err)
			}

			runsResponse := mcpPost(t, handler, issued.Token, "", map[string]any{
				"jsonrpc": "2.0", "id": 3, "method": "tools/call",
				"params": map[string]any{"name": "get_reconciliation",
					"arguments": map[string]any{"invoice_id": invoiceID}},
			})
			var runsBody mcpResponse
			decodeBody(t, runsResponse, &runsBody)
			var runsEnvelope struct {
				StructuredContent struct {
					Runs []reconciliation.RunSummary `json:"reconciliation_runs"`
				} `json:"structuredContent"`
			}
			if err := json.Unmarshal(runsBody.Result, &runsEnvelope); err != nil ||
				len(runsEnvelope.StructuredContent.Runs) != 1 ||
				runsEnvelope.StructuredContent.Runs[0].Status != "completed" {
				t.Fatalf("reconciliation runs = %s %v", runsBody.Result, err)
			}
			if code := reconcileResponse.Code; code != http.StatusOK {
				t.Fatalf("reconcile HTTP status = %d", code)
			}
		})
	}
}
