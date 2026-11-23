package integration

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"atlas/internal/connectors"
	"atlas/internal/execution"
	"atlas/internal/governance"
	"atlas/internal/ingestion"
	"atlas/internal/jobs"
	"atlas/internal/secrets"
)

type countingAdapter struct {
	calls  int
	result execution.Result
	err    error
}

func (a *countingAdapter) Execute(context.Context, execution.Operation) (execution.Result, error) {
	a.calls++
	return a.result, a.err
}

func TestExecutionRunsApprovedOperationIdempotently(t *testing.T) {
	admin, _, _ := setupTenantSchema(t)
	worker := connectRole(t, workerRole)
	organizationID := seedQueueOrganization(t, admin)
	ctx := context.Background()
	queue := jobs.NewQueue(worker)
	governanceService := governance.NewService(worker, queue)

	approvalID, err := governanceService.RequestApproval(ctx, organizationID, "test.action", "invoice", "inv-1", "")
	if err != nil {
		t.Fatalf("request approval: %v", err)
	}
	if err := governanceService.Decide(ctx, organizationID, approvalID, "", true, "ok"); err != nil {
		t.Fatalf("approve: %v", err)
	}

	adapter := &countingAdapter{result: execution.Result{Status: "accepted", Reference: "ext-1"}}
	registry := execution.NewRegistry()
	registry.Register("test.action", adapter)
	service := execution.NewService(worker, governanceService, registry)

	operationID, created, err := governanceService.StartOperation(ctx, organizationID, approvalID,
		"test.action", "exec-key-1", []byte(`{"amount_minor":1000}`))
	if err != nil || !created {
		t.Fatalf("start operation = %s %v %v", operationID, created, err)
	}

	var jobType, jobKey string
	var rawPayload []byte
	if err := admin.QueryRow(ctx, `
		SELECT type, idempotency_key, payload FROM jobs
		WHERE organization_id = $1 AND idempotency_key = $2`,
		organizationID, operationID).Scan(&jobType, &jobKey, &rawPayload); err != nil {
		t.Fatalf("read execution job: %v", err)
	}
	if jobType != governance.ActionExecuteJob || jobKey != operationID {
		t.Fatalf("job = %s/%s, want %s/%s", jobType, jobKey, governance.ActionExecuteJob, operationID)
	}
	var jobPayload map[string]string
	if err := json.Unmarshal(rawPayload, &jobPayload); err != nil {
		t.Fatalf("decode job payload: %v", err)
	}
	if jobPayload["operation_id"] != operationID || jobPayload["organization_id"] != organizationID {
		t.Fatalf("job payload = %v", jobPayload)
	}

	result, err := service.Execute(ctx, organizationID, operationID)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if adapter.calls != 1 {
		t.Fatalf("adapter calls = %d, want 1", adapter.calls)
	}
	var decoded execution.Result
	if err := json.Unmarshal(result, &decoded); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if decoded.Status != "accepted" || decoded.Reference != "ext-1" {
		t.Fatalf("result = %+v", decoded)
	}
	operation, err := governanceService.GetOperation(ctx, organizationID, operationID)
	if err != nil || operation.Status != "succeeded" {
		t.Fatalf("operation = %+v %v", operation, err)
	}

	again, err := service.Execute(ctx, organizationID, operationID)
	if err != nil {
		t.Fatalf("duplicate execution: %v", err)
	}
	var againDecoded execution.Result
	if err := json.Unmarshal(again, &againDecoded); err != nil || againDecoded != decoded {
		t.Fatalf("duplicate execution = %s %v", again, err)
	}
	if adapter.calls != 1 {
		t.Fatalf("adapter ran again: %d calls", adapter.calls)
	}
	var attempts int
	if err := admin.QueryRow(ctx, `
		SELECT count(*) FROM operation_attempts WHERE organization_id = $1 AND operation_id = $2`,
		organizationID, operationID).Scan(&attempts); err != nil {
		t.Fatalf("count attempts: %v", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
	var audited int
	if err := admin.QueryRow(ctx, `
		SELECT count(*) FROM audit_events
		WHERE organization_id = $1 AND action = 'operation.succeeded' AND resource_id = $2`,
		organizationID, operationID).Scan(&audited); err != nil {
		t.Fatalf("count audit: %v", err)
	}
	if audited != 1 {
		t.Fatalf("operation audit rows = %d, want 1", audited)
	}
}

func TestExecutionFailsClosedWithoutAdapter(t *testing.T) {
	admin, _, _ := setupTenantSchema(t)
	worker := connectRole(t, workerRole)
	organizationID := seedQueueOrganization(t, admin)
	ctx := context.Background()
	governanceService := governance.NewService(worker, jobs.NewQueue(worker))
	service := execution.NewService(worker, governanceService, execution.NewRegistry())

	operationID, created, err := governanceService.StartOperation(ctx, organizationID, "",
		"unknown.action", "exec-key-unknown", nil)
	if err != nil || !created {
		t.Fatalf("start operation = %s %v %v", operationID, created, err)
	}
	if _, err := service.Execute(ctx, organizationID, operationID); err != nil {
		t.Fatalf("execute should record a failed operation: %v", err)
	}
	operation, err := governanceService.GetOperation(ctx, organizationID, operationID)
	if err != nil || operation.Status != "failed" || operation.LastError == nil {
		t.Fatalf("operation = %+v %v", operation, err)
	}
}

func TestConnectorWriteExecutionPath(t *testing.T) {
	admin, app, _ := setupTenantSchema(t)
	worker := connectRole(t, workerRole)
	organizationID := seedQueueOrganization(t, admin)
	ctx := context.Background()

	sealer, err := secrets.NewSealer(strings.Repeat("ab", 32))
	if err != nil {
		t.Fatalf("sealer: %v", err)
	}
	connectorRegistry := connectors.NewRegistry()
	connectorRegistry.Register("mock", connectors.NewMock)
	ingestionService := ingestion.NewService(app, connectorRegistry, sealer)
	integrationID, err := ingestionService.CreateIntegration(ctx, organizationID, "mock", "mock", "Mock ERP",
		map[string]any{"allow_writes": true})
	if err != nil {
		t.Fatalf("create integration: %v", err)
	}

	queue := jobs.NewQueue(worker)
	governanceService := governance.NewService(worker, queue)
	registry := execution.NewRegistry()
	registry.Register(execution.TypeConnectorWrite, execution.NewConnectorWriteAdapter(ingestionService))
	service := execution.NewService(worker, governanceService, registry)

	payload, err := json.Marshal(map[string]any{
		"integration_id": integrationID,
		"kind":           "supplier_followup",
		"payload":        map[string]any{"invoice_id": "inv-1"},
	})
	if err != nil {
		t.Fatalf("encode payload: %v", err)
	}
	operationID := approveAndStart(t, ctx, governanceService, organizationID, execution.TypeConnectorWrite,
		"connector-write-1", payload)

	result, err := service.Execute(ctx, organizationID, operationID)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var decoded execution.Result
	if err := json.Unmarshal(result, &decoded); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if decoded.Status != "accepted" || decoded.Reference != "mock-"+operationID {
		t.Fatalf("result = %+v", decoded)
	}

	disabledIntegration, err := ingestionService.CreateIntegration(ctx, organizationID, "mock", "mock",
		"Writes disabled", map[string]any{})
	if err != nil {
		t.Fatalf("create disabled integration: %v", err)
	}
	disabledPayload, err := json.Marshal(map[string]any{
		"integration_id": disabledIntegration,
		"kind":           "supplier_followup",
	})
	if err != nil {
		t.Fatalf("encode disabled payload: %v", err)
	}
	disabledOperation := approveAndStart(t, ctx, governanceService, organizationID, execution.TypeConnectorWrite,
		"connector-write-2", disabledPayload)
	_, execErr := service.Execute(ctx, organizationID, disabledOperation)
	if execErr == nil {
		t.Fatal("disabled connector write succeeded")
	}
	running, err := governanceService.GetOperation(ctx, organizationID, disabledOperation)
	if err != nil || running.Status != "running" {
		t.Fatalf("operation after transient failure = %+v %v", running, err)
	}
	if err := service.Fail(ctx, organizationID, disabledOperation, execErr); err != nil {
		t.Fatalf("fail operation: %v", err)
	}
	failed, err := governanceService.GetOperation(ctx, organizationID, disabledOperation)
	if err != nil || failed.Status != "failed" {
		t.Fatalf("failed operation = %+v %v", failed, err)
	}
}

func approveAndStart(t *testing.T, ctx context.Context, service *governance.Service, organizationID, operationType, key string, payload []byte) string {
	t.Helper()
	approvalID, err := service.RequestApproval(ctx, organizationID, operationType, "invoice", "inv-1", "")
	if err != nil {
		t.Fatalf("request approval: %v", err)
	}
	if err := service.Decide(ctx, organizationID, approvalID, "", true, "ok"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	operationID, created, err := service.StartOperation(ctx, organizationID, approvalID, operationType, key, payload)
	if err != nil || !created {
		t.Fatalf("start operation = %s %v %v", operationID, created, err)
	}
	return operationID
}
