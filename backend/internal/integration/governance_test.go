package integration

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"atlas/internal/agentaccess"
	"atlas/internal/authorization"
	"atlas/internal/governance"
	"atlas/internal/httpapi"
	"atlas/internal/identity"
	"atlas/internal/jobs"
	"atlas/internal/orgs"
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
	again, created, err := service.StartOperation(ctx, organizationID, approvalID, "payment.execute", "op-key-1", []byte(`{"amount_minor":1000}`))
	if err != nil || created || again != operationID {
		t.Fatalf("idempotency = %s %v %v", again, created, err)
	}
	if _, _, err := service.StartOperation(ctx, organizationID, approvalID, "payment.execute", "op-key-1", []byte(`{"amount_minor":2000}`)); !errors.Is(err, governance.ErrIdempotencyConflict) {
		t.Fatalf("conflicting idempotency payload error = %v", err)
	}
	if err := service.FinishOperation(ctx, organizationID, operationID, "succeeded", `{"reference":"ext-1"}`, ""); err != nil {
		t.Fatalf("finish: %v", err)
	}
	operation, err := service.GetOperation(ctx, organizationID, operationID)
	if err != nil || operation.Status != "succeeded" || string(operation.Result) != `{"reference": "ext-1"}` {
		t.Fatalf("operation read = %+v %v", operation, err)
	}
	if operation.ApprovalID == nil || *operation.ApprovalID != approvalID {
		t.Fatalf("operation approval = %v", operation.ApprovalID)
	}
	if len(operation.Attempts) != 1 || operation.Attempts[0].Status != "succeeded" {
		t.Fatalf("operation attempts = %+v", operation.Attempts)
	}
	operations, err := service.ListOperations(ctx, organizationID, "succeeded", 10)
	if err != nil || len(operations) != 1 || operations[0].ID != operationID {
		t.Fatalf("list operations = %+v %v", operations, err)
	}
	approval, err := service.GetApproval(ctx, organizationID, approvalID)
	if err != nil || approval.Status != "approved" || approval.DecidedAt == nil {
		t.Fatalf("approval read = %+v %v", approval, err)
	}
	approvals, err := service.ListApprovals(ctx, organizationID, "approved", 10)
	if err != nil || len(approvals) != 1 || approvals[0].ID != approvalID {
		t.Fatalf("list approvals = %+v %v", approvals, err)
	}
	if empty, err := service.ListApprovals(ctx, organizationID, "rejected", 10); err != nil || len(empty) != 0 {
		t.Fatalf("filtered approvals = %+v %v", empty, err)
	}
	if _, err := service.ListApprovals(ctx, organizationID, "unknown", 10); err == nil {
		t.Fatal("invalid approval status was accepted")
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

func TestSupplierFollowupREST(t *testing.T) {
	admin, app, auth := setupTenantSchema(t)
	tenants := seedTenants(t, admin)
	ctx := context.Background()
	governanceService := governance.NewService(app, jobs.NewQueue(app))
	if _, err := governanceService.SetPolicy(ctx, tenants.OrganizationA, "default",
		[]byte(`{"allow_actions":["supplier.followup"]}`)); err != nil {
		t.Fatalf("set policy: %v", err)
	}
	agents := agentaccess.NewService(app)
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
		Governance:     governanceService,
		DevAuthEnabled: true,
	})
	invoiceID := "3f2504e0-4f89-41d3-9a0c-0305e82c3301"

	memberCookie := login(t, router, tenants.OrganizationA, tenants.MemberEmailA)
	followup := request(t, router, http.MethodPost,
		"/v1/invoices/"+invoiceID+"/supplier-followups", memberCookie, map[string]any{"amount_minor": 900})
	if followup.Code != http.StatusCreated {
		t.Fatalf("member follow-up status = %d, body = %s", followup.Code, followup.Body.String())
	}
	var followupBody governance.FollowUp
	decodeBody(t, followup, &followupBody)
	if followupBody.ApprovalID == "" || followupBody.Status != "pending" || followupBody.PolicyReason == "" {
		t.Fatalf("follow-up = %+v", followupBody)
	}
	var actorKind, actorID, operationType string
	if err := admin.QueryRow(ctx, `
		SELECT actor_kind, actor_id FROM audit_events
		WHERE organization_id = $1 AND action = 'approval.requested' AND resource_id = $2
		ORDER BY created_at DESC LIMIT 1`, tenants.OrganizationA, invoiceID).Scan(&actorKind, &actorID); err != nil {
		t.Fatalf("read member audit: %v", err)
	}
	if actorKind != "human" || actorID != tenants.MemberUserA {
		t.Fatalf("member audit = %s/%s", actorKind, actorID)
	}
	if err := admin.QueryRow(ctx, `
		SELECT operation_type FROM approvals WHERE organization_id = $1 AND id = $2`,
		tenants.OrganizationA, followupBody.ApprovalID).Scan(&operationType); err != nil {
		t.Fatalf("read approval: %v", err)
	}
	if operationType != "supplier.followup" {
		t.Fatalf("approval operation type = %s", operationType)
	}

	connection, err := agents.CreateConnection(ctx, tenants.OrganizationA, "followup-agent",
		[]string{agentaccess.ScopeActionsRequest}, tenants.AdminUserA)
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	issued, err := agents.IssueToken(ctx, tenants.OrganizationA, connection.ID, tenants.AdminUserA, time.Hour)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	agentFollowup := bearerRequest(t, router, http.MethodPost,
		"/v1/invoices/"+invoiceID+"/supplier-followups", issued.Token)
	if agentFollowup.Code != http.StatusCreated {
		t.Fatalf("agent follow-up status = %d, body = %s", agentFollowup.Code, agentFollowup.Body.String())
	}
	if err := admin.QueryRow(ctx, `
		SELECT actor_kind, actor_id FROM audit_events
		WHERE organization_id = $1 AND action = 'approval.requested' AND resource_id = $2
		ORDER BY created_at DESC LIMIT 1`, tenants.OrganizationA, invoiceID).Scan(&actorKind, &actorID); err != nil {
		t.Fatalf("read agent audit: %v", err)
	}
	if actorKind != "agent" || actorID != connection.ID {
		t.Fatalf("agent audit = %s/%s", actorKind, actorID)
	}

	limited, err := agents.CreateConnection(ctx, tenants.OrganizationA, "limited-followup",
		[]string{agentaccess.ScopeSuppliersRead}, tenants.AdminUserA)
	if err != nil {
		t.Fatalf("create limited agent: %v", err)
	}
	limitedToken, err := agents.IssueToken(ctx, tenants.OrganizationA, limited.ID, tenants.AdminUserA, time.Hour)
	if err != nil {
		t.Fatalf("issue limited token: %v", err)
	}
	if response := bearerRequest(t, router, http.MethodPost,
		"/v1/invoices/"+invoiceID+"/supplier-followups", limitedToken.Token); response.Code != http.StatusForbidden {
		t.Fatalf("limited agent follow-up status = %d, want 403", response.Code)
	}
}

func TestGovernanceReadEndpoints(t *testing.T) {
	admin, app, auth := setupTenantSchema(t)
	tenants := seedTenants(t, admin)
	ctx := context.Background()
	queue := jobs.NewQueue(app)
	governanceService := governance.NewService(app, queue)
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
		Governance:     governanceService,
		DevAuthEnabled: true,
	})

	if response := request(t, router, http.MethodGet, "/v1/approvals", nil, nil); response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated approvals status = %d", response.Code)
	}
	cookie := login(t, router, tenants.OrganizationA, tenants.AdminEmailA)

	approvalID, err := governanceService.RequestApproval(ctx, tenants.OrganizationA,
		"connector.write", "invoice", "inv-1", tenants.AdminUserA)
	if err != nil {
		t.Fatalf("request approval: %v", err)
	}
	if err := governanceService.Decide(ctx, tenants.OrganizationA, approvalID, tenants.AdminUserA, true, "ok"); err != nil {
		t.Fatalf("decide: %v", err)
	}
	operationID, _, err := governanceService.StartOperation(ctx, tenants.OrganizationA, approvalID,
		"connector.write", "http-key-1", []byte(`{}`))
	if err != nil {
		t.Fatalf("start operation: %v", err)
	}

	approvalsResponse := request(t, router, http.MethodGet, "/v1/approvals?status=approved&limit=10", cookie, nil)
	if approvalsResponse.Code != http.StatusOK {
		t.Fatalf("approvals status = %d, body = %s", approvalsResponse.Code, approvalsResponse.Body.String())
	}
	var approvalsBody struct {
		Approvals []governance.Approval `json:"approvals"`
	}
	decodeBody(t, approvalsResponse, &approvalsBody)
	if len(approvalsBody.Approvals) != 1 || approvalsBody.Approvals[0].ID != approvalID {
		t.Fatalf("approvals body = %+v", approvalsBody)
	}

	approvalResponse := request(t, router, http.MethodGet, "/v1/approvals/"+approvalID, cookie, nil)
	if approvalResponse.Code != http.StatusOK {
		t.Fatalf("approval status = %d, body = %s", approvalResponse.Code, approvalResponse.Body.String())
	}

	operationsResponse := request(t, router, http.MethodGet, "/v1/operations?status=pending", cookie, nil)
	if operationsResponse.Code != http.StatusOK {
		t.Fatalf("operations status = %d, body = %s", operationsResponse.Code, operationsResponse.Body.String())
	}
	var operationsBody struct {
		Operations []governance.Operation `json:"operations"`
	}
	decodeBody(t, operationsResponse, &operationsBody)
	if len(operationsBody.Operations) != 1 || operationsBody.Operations[0].ID != operationID {
		t.Fatalf("operations body = %+v", operationsBody)
	}

	operationResponse := request(t, router, http.MethodGet, "/v1/operations/"+operationID, cookie, nil)
	if operationResponse.Code != http.StatusOK {
		t.Fatalf("operation status = %d, body = %s", operationResponse.Code, operationResponse.Body.String())
	}
	var operationBody struct {
		Operation governance.Operation `json:"operation"`
	}
	decodeBody(t, operationResponse, &operationBody)
	if operationBody.Operation.ApprovalID == nil || *operationBody.Operation.ApprovalID != approvalID {
		t.Fatalf("operation body = %+v", operationBody)
	}

	if response := request(t, router, http.MethodGet, "/v1/approvals?status=bogus", cookie, nil); response.Code != http.StatusBadRequest {
		t.Fatalf("invalid approval status = %d", response.Code)
	}
	if response := request(t, router, http.MethodGet, "/v1/operations?limit=999", cookie, nil); response.Code != http.StatusBadRequest {
		t.Fatalf("invalid limit = %d", response.Code)
	}
}
