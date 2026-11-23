package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"atlas/internal/identity"
	"atlas/internal/platform/apperr"
)

func triggerReconciliationHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := identity.PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnauthenticated, "authentication_required", "Authentication is required."))
			return
		}
		if deps.Governance == nil {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnavailable, "reconciliation_unavailable", "Reconciliation is not configured."))
			return
		}
		invoiceID := chi.URLParam(r, "invoiceID")
		idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if idempotencyKey == "" {
			idempotencyKey = uuid.NewString()
		}
		payload, _ := json.Marshal(map[string]string{"invoice_id": invoiceID})
		operationID, created, err := deps.Governance.StartJobOperation(r.Context(), principal.OrganizationID,
			"reconciliation.run", "reconciliation.run", idempotencyKey, payload)
		if err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		WriteJSON(w, http.StatusAccepted, map[string]any{
			"operation_id": operationID, "status": "queued", "created": created,
		})
	}
}

func getReconciliationRunHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := identity.PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnauthenticated, "authentication_required", "Authentication is required."))
			return
		}
		if deps.Reconciliation == nil {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnavailable, "reconciliation_unavailable", "Reconciliation is not configured."))
			return
		}
		detail, err := deps.Reconciliation.GetRun(r.Context(), principal.OrganizationID, chi.URLParam(r, "runID"))
		if err != nil {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindNotFound, "reconciliation_run_not_found", "Reconciliation run not found."))
			return
		}
		WriteJSON(w, http.StatusOK, detail)
	}
}

func listReconciliationRunsHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := identity.PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnauthenticated, "authentication_required", "Authentication is required."))
			return
		}
		if deps.Reconciliation == nil {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnavailable, "reconciliation_unavailable", "Reconciliation is not configured."))
			return
		}
		runs, err := deps.Reconciliation.ListByInvoice(r.Context(), principal.OrganizationID, chi.URLParam(r, "invoiceID"))
		if err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"reconciliation_runs": runs})
	}
}
