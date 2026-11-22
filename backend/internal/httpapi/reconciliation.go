package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"atlas/internal/identity"
	"atlas/internal/jobs"
	"atlas/internal/platform/apperr"
	"atlas/internal/tenancy"
	"encoding/json"
)

func triggerReconciliationHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := identity.PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnauthenticated, "authentication_required", "Authentication is required."))
			return
		}
		if deps.Queue == nil || deps.Reconciliation == nil {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnavailable, "reconciliation_unavailable", "Reconciliation is not configured."))
			return
		}
		invoiceID := chi.URLParam(r, "invoiceID")
		var job *jobs.Job
		payload, _ := json.Marshal(map[string]string{"organization_id": principal.OrganizationID, "invoice_id": invoiceID})
		err := tenancy.WithOrganizationTx(r.Context(), deps.Pool, principal.OrganizationID, func(tx pgx.Tx) error {
			var err error
			job, _, err = deps.Queue.Enqueue(r.Context(), tx, jobs.NewJob{
				OrganizationID: principal.OrganizationID,
				Type:           "reconciliation.run",
				SchemaVersion:  1,
				Payload:        payload,
			})
			return err
		})
		if err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		WriteJSON(w, http.StatusAccepted, map[string]any{"job_id": job.ID})
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
