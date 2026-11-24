package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"atlas/internal/identity"
	"atlas/internal/platform/apperr"
	"atlas/internal/retention"
)

type legalHoldRequest struct {
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	Reason       string `json:"reason"`
}

func retentionUnavailable(deps Deps) *apperr.Error {
	return apperr.New(apperr.KindUnavailable, "retention_unavailable", "Retention services are not configured.")
}

func listLegalHoldsHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := identity.PrincipalFrom(r.Context())
		if !ok || deps.Retention == nil {
			WriteError(w, r, deps.Logger, retentionUnavailable(deps))
			return
		}
		activeOnly := true
		if raw := r.URL.Query().Get("active"); raw != "" {
			parsed, err := strconv.ParseBool(raw)
			if err != nil {
				WriteError(w, r, deps.Logger, apperr.New(apperr.KindInvalid, "invalid_request", "active must be true or false."))
				return
			}
			activeOnly = parsed
		}
		limit, err := listLimit(r)
		if err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		holds, err := deps.Retention.ListHolds(r.Context(), principal.OrganizationID, activeOnly, limit)
		if err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"legal_holds": holds})
	}
}

func placeLegalHoldHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := identity.PrincipalFrom(r.Context())
		if !ok || deps.Retention == nil {
			WriteError(w, r, deps.Logger, retentionUnavailable(deps))
			return
		}
		var request legalHoldRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&request); err != nil {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindInvalid, "invalid_request", "resource_type, resource_id and reason are required."))
			return
		}
		hold, err := deps.Retention.PlaceHold(r.Context(), principal.OrganizationID,
			request.ResourceType, request.ResourceID, request.Reason, principal.ID)
		if err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		WriteJSON(w, http.StatusCreated, map[string]any{"legal_hold": hold})
	}
}

func releaseLegalHoldHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := identity.PrincipalFrom(r.Context())
		if !ok || deps.Retention == nil {
			WriteError(w, r, deps.Logger, retentionUnavailable(deps))
			return
		}
		if err := deps.Retention.ReleaseHold(r.Context(), principal.OrganizationID,
			chi.URLParam(r, "holdID"), principal.ID); err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"status": "released"})
	}
}

func retentionPoliciesHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := identity.PrincipalFrom(r.Context()); !ok {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnauthenticated, "authentication_required", "Authentication is required."))
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"policies": retention.Defaults()})
	}
}
