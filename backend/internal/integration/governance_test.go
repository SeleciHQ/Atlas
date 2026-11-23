package integration

import (
	"context"
	"testing"

	"atlas/internal/governance"
	"atlas/internal/jobs"
)

func TestGovernanceApprovalsAndIdempotency(t *testing.T) {
	admin, app, _ := setupTenantSchema(t)
	organizationID := seedQueueOrganization(t, admin)
	ctx := context.Background()
	service := governance.NewService(app, jobs.NewQueue(app))

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
	if _, err := admin.Exec(ctx, `UPDATE approvals SET expires_at = now() - interval '1 hour' WHERE id = $1`, approvalID); err != nil {
		t.Fatalf("expire approval: %v", err)
	}
	if err := service.Decide(ctx, organizationID, approvalID, "", true, "late"); err == nil {
		t.Fatal("expired approval was decided")
	}
	if _, err := admin.Exec(ctx, `UPDATE approvals SET expires_at = NULL WHERE id = $1`, approvalID); err != nil {
		t.Fatalf("unexpire approval: %v", err)
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
	operation, err := service.GetOperation(ctx, organizationID, operationID)
	if err != nil || operation.Status != "succeeded" || string(operation.Result) != `{"reference": "ext-1"}` {
		t.Fatalf("operation read = %+v %v", operation, err)
	}
	if err := service.FinishOperation(ctx, organizationID, operationID, "succeeded", `{}`, ""); err == nil {
		t.Fatal("terminal operation was re-finished")
	}
}

func TestApprovalExpirySweep(t *testing.T) {
	admin, _, _ := setupTenantSchema(t)
	worker := connectRole(t, workerRole)
	organizationA := seedQueueOrganization(t, admin)
	organizationB := seedQueueOrganization(t, admin)
	ctx := context.Background()
	service := governance.NewService(worker, jobs.NewQueue(worker))

	insert := func(organizationID, objectID, offset string) string {
		t.Helper()
		var id string
		if err := admin.QueryRow(ctx, `
			INSERT INTO approvals (organization_id, operation_type, object_type, object_id, expires_at)
			VALUES ($1, 'supplier.followup', 'invoice', $2, now() + $3::interval)
			RETURNING id`, organizationID, objectID, offset).Scan(&id); err != nil {
			t.Fatalf("insert approval: %v", err)
		}
		return id
	}
	expiredA := insert(organizationA, "inv-a", "-1 hour")
	expiredB := insert(organizationB, "inv-b", "-1 minute")
	liveA := insert(organizationA, "inv-c", "+1 hour")

	expired, err := service.ExpireApprovals(ctx)
	if err != nil || expired != 2 {
		t.Fatalf("expire approvals = %d, %v", expired, err)
	}
	status := func(id string) string {
		t.Helper()
		var value string
		if err := admin.QueryRow(ctx, `SELECT status FROM approvals WHERE id = $1`, id).Scan(&value); err != nil {
			t.Fatalf("read approval %s: %v", id, err)
		}
		return value
	}
	if status(expiredA) != "expired" || status(expiredB) != "expired" || status(liveA) != "pending" {
		t.Fatalf("statuses = %s/%s/%s", status(expiredA), status(expiredB), status(liveA))
	}
	var audited int
	if err := admin.QueryRow(ctx, `
		SELECT count(*) FROM audit_events WHERE action = 'approval.expired'`).Scan(&audited); err != nil {
		t.Fatalf("count audit: %v", err)
	}
	if audited != 2 {
		t.Fatalf("approval.expired audit rows = %d, want 2", audited)
	}
	if again, err := service.ExpireApprovals(ctx); err != nil || again != 0 {
		t.Fatalf("second sweep = %d, %v", again, err)
	}
}
