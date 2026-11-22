package integration

import (
	"context"
	"testing"

	"atlas/internal/governance"
)

func TestGovernanceApprovalsAndIdempotency(t *testing.T) {
	admin, app, _ := setupTenantSchema(t)
	organizationID := seedQueueOrganization(t, admin)
	ctx := context.Background()
	service := governance.NewService(app)

	decision, err := service.Evaluate(ctx, organizationID, "payment.execute", 1000)
	if err != nil || decision.Allowed {
		t.Fatalf("default decision = %+v %v", decision, err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO policies (organization_id, name, version, status, config)
		VALUES ($1, 'default', 1, 'active', '{"approval_actions":["payment.execute"],"amount_threshold_minor":5000}'::jsonb)`,
		organizationID); err != nil {
		t.Fatalf("insert policy: %v", err)
	}
	decision, err = service.Evaluate(ctx, organizationID, "payment.execute", 1000)
	if err != nil || !decision.Allowed || !decision.RequiresApproval {
		t.Fatalf("approval decision = %+v %v", decision, err)
	}
	decision, err = service.Evaluate(ctx, organizationID, "payment.execute", 6000)
	if err != nil || decision.Allowed {
		t.Fatalf("threshold decision = %+v %v", decision, err)
	}

	approvalID, err := service.RequestApproval(ctx, organizationID, "payment.execute", "invoice", "inv-1", "")
	if err != nil {
		t.Fatalf("request approval: %v", err)
	}
	if _, _, err := service.StartOperation(ctx, organizationID, approvalID, "payment.execute", "op-key-1", []byte(`{}`)); err == nil {
		t.Fatal("operation started with a pending approval")
	}
	if err := service.Decide(ctx, organizationID, approvalID, "", true, "ok"); err != nil {
		t.Fatalf("decide: %v", err)
	}
	operationID, created, err := service.StartOperation(ctx, organizationID, approvalID, "payment.execute", "op-key-1", []byte(`{"amount_minor":1000}`))
	if err != nil || !created {
		t.Fatalf("start operation = %s %v %v", operationID, created, err)
	}
	again, created, err := service.StartOperation(ctx, organizationID, approvalID, "payment.execute", "op-key-1", []byte(`{}`))
	if err != nil || created || again != operationID {
		t.Fatalf("idempotency = %s %v %v", again, created, err)
	}
	if err := service.FinishOperation(ctx, organizationID, operationID, "succeeded", `{"reference":"ext-1"}`, ""); err != nil {
		t.Fatalf("finish: %v", err)
	}
	if err := service.FinishOperation(ctx, organizationID, operationID, "succeeded", `{}`, ""); err == nil {
		t.Fatal("terminal operation was re-finished")
	}
}
