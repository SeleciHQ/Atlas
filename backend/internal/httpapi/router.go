// Package httpapi wires Atlas's HTTP surface. Handlers stay thin: parsing,
// authentication and authorization middleware will be added in Phase 2 before
// any business handler exists.
package httpapi

import (
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"atlas/internal/agentaccess"
	"atlas/internal/authorization"
	"atlas/internal/documents"
	"atlas/internal/governance"
	"atlas/internal/identity"
	"atlas/internal/ingestion"
	"atlas/internal/jobs"
	"atlas/internal/orgs"
	"atlas/internal/platform/apperr"
	"atlas/internal/query"
	"atlas/internal/reconciliation"
	"atlas/internal/retention"
)

// Deps are the dependencies required to build the HTTP router.
type Deps struct {
	Logger         *slog.Logger
	Pool           *pgxpool.Pool
	Migrations     fs.FS
	ServiceName    string
	Version        string
	Identity       identity.Authenticator
	DevSessions    identity.DevSessionIssuer
	OIDC           *identity.OIDCService
	OIDCEnabled    bool
	Invitations    *identity.InvitationService
	Documents      *documents.Service
	Integrations   *ingestion.Service
	Queue          *jobs.Queue
	Reconciliation *reconciliation.Service
	Query          *query.Service
	Governance     *governance.Service
	Agents         *agentaccess.Service
	Retention      *retention.Service
	StorageHandler http.Handler
	Authorizer     authorization.Service
	Orgs           *orgs.Service
	DevAuthEnabled bool
	SecureCookies  bool
}

// NewRouter builds the Atlas HTTP router.
func NewRouter(deps Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(
		middleware.RequestID,
		middleware.RealIP,
		observe(deps.Logger),
		middleware.Timeout(30*time.Second),
	)

	r.Get("/healthz", healthHandler(deps))
	r.Get("/readyz", readinessHandler(deps))

	r.Route("/v1", func(r chi.Router) {
		if deps.DevAuthEnabled && deps.DevSessions != nil {
			r.Post("/auth/dev-login", devLoginHandler(deps))
		}
		if deps.OIDCEnabled && deps.OIDC != nil {
			r.Get("/auth/oidc/start", oidcStartHandler(deps))
			r.Get("/auth/oidc/callback", oidcCallbackHandler(deps))
		}
		r.Post("/webhooks/{organizationID}/{integrationID}", webhookHandler(deps))
		if deps.StorageHandler != nil {
			r.Handle("/storage/local/object", deps.StorageHandler)
		}
		r.Group(func(r chi.Router) {
			r.Use(authenticate(deps))
			r.Get("/auth/me", meHandler)
			r.Post("/auth/logout", logoutHandler(deps))
			r.Get("/agents/me", agentMeHandler)
			r.With(requirePermission(deps, authorization.PermissionAgentsManage)).
				Get("/agents", listAgentsHandler(deps))
			r.With(requirePermission(deps, authorization.PermissionAgentsManage)).
				Post("/agents", createAgentHandler(deps))
			r.With(requirePermission(deps, authorization.PermissionAgentsManage)).
				Post("/agents/{agentID}/revoke", revokeAgentHandler(deps))
			r.With(requirePermission(deps, authorization.PermissionAgentsManage)).
				Get("/agents/{agentID}/tokens", listAgentTokensHandler(deps))
			r.With(requirePermission(deps, authorization.PermissionAgentsManage)).
				Post("/agents/{agentID}/tokens", issueAgentTokenHandler(deps))
			r.With(requirePermission(deps, authorization.PermissionAgentsManage)).
				Post("/agents/{agentID}/tokens/{tokenID}/revoke", revokeAgentTokenHandler(deps))
			r.With(requirePermission(deps, authorization.PermissionMembersRead)).
				Get("/members", membersHandler(deps))
			r.With(requirePermission(deps, authorization.PermissionOrganizationManage)).
				Get("/organization/settings", organizationSettingsHandler(deps))
			r.With(requirePermission(deps, authorization.PermissionMembersManage)).
				Post("/invitations", createInvitationHandler(deps))
			r.With(requirePermission(deps, authorization.PermissionDocumentsWrite)).
				Post("/documents", createDocumentHandler(deps))
			r.With(requirePermission(deps, authorization.PermissionDocumentsWrite)).
				Post("/documents/{documentID}/finalize", finalizeDocumentHandler(deps))
			r.With(requirePermission(deps, authorization.PermissionDocumentsRead)).
				Get("/documents/{documentID}", getDocumentHandler(deps))
			r.With(requirePermission(deps, authorization.PermissionDocumentsRead)).
				Get("/documents/{documentID}/download", downloadDocumentHandler(deps))
			r.With(requirePermission(deps, authorization.PermissionIntegrationsRead)).
				Get("/integrations", listIntegrationsHandler(deps))
			r.With(requirePermission(deps, authorization.PermissionIntegrationsRead)).
				Get("/integrations/{integrationID}", getIntegrationHandler(deps))
			r.With(requirePermission(deps, authorization.PermissionIntegrationsWrite)).
				Post("/integrations", createIntegrationHandler(deps))
			r.With(requirePermission(deps, authorization.PermissionIntegrationsWrite)).
				Post("/integrations/{integrationID}/credentials", setIntegrationCredentialsHandler(deps))
			r.With(requirePermission(deps, authorization.PermissionIntegrationsWrite)).
				Post("/integrations/{integrationID}/sync", triggerIntegrationSyncHandler(deps))
			r.With(requireScope(deps, "invoices:read")).
				Get("/invoices", searchInvoicesHandler(deps))
			r.With(requireScope(deps, "invoices:read")).
				Get("/invoices/{invoiceID}", getInvoiceHandler(deps))
			r.With(requireScope(deps, "actions:request")).
				Post("/invoices/{invoiceID}/supplier-followups", requestSupplierFollowupHandler(deps))
			r.With(requireScope(deps, "purchase_orders:read")).
				Get("/purchase-orders", searchPurchaseOrdersHandler(deps))
			r.With(requireScope(deps, "purchase_orders:read")).
				Get("/purchase-orders/{purchaseOrderID}", getPurchaseOrderHandler(deps))
			r.With(requireScope(deps, "suppliers:read")).
				Get("/suppliers", searchSuppliersHandler(deps))
			r.With(requireScope(deps, "suppliers:read")).
				Get("/suppliers/{supplierID}", getSupplierHandler(deps))
			r.With(requireScope(deps, "deliveries:read")).
				Get("/deliveries", listDeliveriesHandler(deps))
			r.With(requireScope(deps, "discrepancies:read")).
				Get("/discrepancies", searchDiscrepanciesHandler(deps))
			r.With(requireScope(deps, "reconciliation:run")).
				Post("/invoices/{invoiceID}/reconcile", triggerReconciliationHandler(deps))
			r.With(requireScope(deps, "reconciliation:read")).
				Get("/reconciliation-runs/{runID}", getReconciliationRunHandler(deps))
			r.With(requireScope(deps, "reconciliation:read")).
				Get("/invoices/{invoiceID}/reconciliation-runs", listReconciliationRunsHandler(deps))
			r.With(requireScope(deps, "approvals:request")).
				Get("/approvals", listApprovalsHandler(deps))
			r.With(requireScope(deps, "approvals:request")).
				Get("/approvals/{approvalID}", getApprovalHandler(deps))
			r.With(requireScope(deps, "approvals:request")).
				Post("/approvals", requestApprovalHandler(deps))
			r.With(requirePermission(deps, authorization.PermissionApprovalsDecide)).
				Post("/approvals/{approvalID}/decision", decideApprovalHandler(deps))
			r.With(requirePermission(deps, authorization.PermissionOrganizationManage)).
				Put("/policies", setPolicyHandler(deps))
			r.With(requirePermission(deps, authorization.PermissionOrganizationRead)).
				Get("/policies", getPolicyHandler(deps))
			r.With(requireScope(deps, "operations:read")).
				Get("/operations", listOperationsHandler(deps))
			r.With(requireScope(deps, "operations:read")).
				Get("/operations/{operationID}", getOperationHandler(deps))
			r.With(requirePermission(deps, authorization.PermissionOrganizationManage)).
				Get("/legal-holds", listLegalHoldsHandler(deps))
			r.With(requirePermission(deps, authorization.PermissionOrganizationManage)).
				Post("/legal-holds", placeLegalHoldHandler(deps))
			r.With(requirePermission(deps, authorization.PermissionOrganizationManage)).
				Post("/legal-holds/{holdID}/release", releaseLegalHoldHandler(deps))
			r.With(requirePermission(deps, authorization.PermissionOrganizationRead)).
				Get("/retention/policies", retentionPoliciesHandler(deps))
		})
	})

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, deps.Logger, apperr.New(apperr.KindNotFound, "not_found", "The requested resource was not found."))
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		WriteProblem(w, apperr.Problem{
			Type:      "about:blank",
			Title:     "Method Not Allowed",
			Status:    http.StatusMethodNotAllowed,
			Code:      "method_not_allowed",
			Detail:    "The HTTP method is not allowed for this resource.",
			RequestID: middleware.GetReqID(r.Context()),
		})
	})

	return r
}
