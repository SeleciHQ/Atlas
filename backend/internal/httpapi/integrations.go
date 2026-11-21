package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"atlas/internal/identity"
	"atlas/internal/ingestion"
	"atlas/internal/jobs"
	"atlas/internal/platform/apperr"
	"atlas/internal/tenancy"
)

type createIntegrationRequest struct {
	Provider string         `json:"provider"`
	Kind     string         `json:"kind"`
	Name     string         `json:"name"`
	Config   map[string]any `json:"config"`
}

type integrationCredentialsRequest struct {
	Credentials string `json:"credentials"`
}

func integrationsConfigured(deps Deps) bool { return deps.Integrations != nil }

func createIntegrationHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := identity.PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnauthenticated, "authentication_required", "Authentication is required."))
			return
		}
		if !integrationsConfigured(deps) {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnavailable, "integrations_unavailable", "Integration management is not configured."))
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		var request createIntegrationRequest
		if err := decoder.Decode(&request); err != nil || request.Provider == "" || request.Name == "" {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindInvalid, "invalid_request", "provider and name are required."))
			return
		}
		if request.Kind == "" {
			request.Kind = request.Provider
		}
		id, err := deps.Integrations.CreateIntegration(r.Context(), principal.OrganizationID,
			request.Provider, request.Kind, request.Name, request.Config)
		if err != nil {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindInvalid, "integration_create_failed", err.Error()))
			return
		}
		WriteJSON(w, http.StatusCreated, map[string]any{"integration_id": id})
	}
}

func setIntegrationCredentialsHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := identity.PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnauthenticated, "authentication_required", "Authentication is required."))
			return
		}
		if !integrationsConfigured(deps) {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnavailable, "integrations_unavailable", "Integration management is not configured."))
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		var request integrationCredentialsRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Credentials == "" {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindInvalid, "invalid_request", "credentials are required."))
			return
		}
		if err := deps.Integrations.SealCredentials(r.Context(), principal.OrganizationID,
			chi.URLParam(r, "integrationID"), []byte(request.Credentials)); err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func triggerIntegrationSyncHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := identity.PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnauthenticated, "authentication_required", "Authentication is required."))
			return
		}
		if deps.Queue == nil {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnavailable, "jobs_unavailable", "The job queue is not configured."))
			return
		}
		integrationID := chi.URLParam(r, "integrationID")
		var job *jobs.Job
		err := tenancy.WithOrganizationTx(r.Context(), deps.Pool, principal.OrganizationID, func(tx pgx.Tx) error {
			var err error
			job, _, err = deps.Queue.Enqueue(r.Context(), tx, jobs.NewJob{
				OrganizationID: principal.OrganizationID,
				Type:           "connector.sync",
				SchemaVersion:  1,
				Payload:        ingestion.SyncJobPayload(principal.OrganizationID, integrationID),
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

func getIntegrationHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := identity.PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnauthenticated, "authentication_required", "Authentication is required."))
			return
		}
		if !integrationsConfigured(deps) {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnavailable, "integrations_unavailable", "Integration management is not configured."))
			return
		}
		integration, err := deps.Integrations.GetIntegration(r.Context(), principal.OrganizationID, chi.URLParam(r, "integrationID"))
		if err != nil {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindNotFound, "integration_not_found", "Integration not found."))
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"integration": integration})
	}
}

func listIntegrationsHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := identity.PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnauthenticated, "authentication_required", "Authentication is required."))
			return
		}
		if !integrationsConfigured(deps) {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnavailable, "integrations_unavailable", "Integration management is not configured."))
			return
		}
		integrations, err := deps.Integrations.ListIntegrations(r.Context(), principal.OrganizationID)
		if err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"integrations": integrations})
	}
}
