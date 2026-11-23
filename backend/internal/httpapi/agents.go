package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"atlas/internal/identity"
	"atlas/internal/platform/apperr"
)

type createAgentRequest struct {
	Name   string   `json:"name"`
	Scopes []string `json:"scopes"`
}

type issueAgentTokenRequest struct {
	ExpiresInSeconds int64 `json:"expires_in_seconds"`
}

func agentUnavailable(deps Deps) *apperr.Error {
	return apperr.New(apperr.KindUnavailable, "agents_unavailable", "Agent access is not configured.")
}

func createAgentHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := identity.PrincipalFrom(r.Context())
		if !ok || deps.Agents == nil {
			WriteError(w, r, deps.Logger, agentUnavailable(deps))
			return
		}
		var request createAgentRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&request); err != nil {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindInvalid, "invalid_request", "A name and scopes are required."))
			return
		}
		connection, err := deps.Agents.CreateConnection(r.Context(), principal.OrganizationID,
			request.Name, request.Scopes, principal.ID)
		if err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		WriteJSON(w, http.StatusCreated, map[string]any{"agent": connection})
	}
}

func listAgentsHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := identity.PrincipalFrom(r.Context())
		if !ok || deps.Agents == nil {
			WriteError(w, r, deps.Logger, agentUnavailable(deps))
			return
		}
		connections, err := deps.Agents.ListConnections(r.Context(), principal.OrganizationID)
		if err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"agents": connections})
	}
}

func revokeAgentHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := identity.PrincipalFrom(r.Context())
		if !ok || deps.Agents == nil {
			WriteError(w, r, deps.Logger, agentUnavailable(deps))
			return
		}
		if err := deps.Agents.RevokeConnection(r.Context(), principal.OrganizationID,
			chi.URLParam(r, "agentID"), principal.ID); err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"status": "revoked"})
	}
}

func issueAgentTokenHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := identity.PrincipalFrom(r.Context())
		if !ok || deps.Agents == nil {
			WriteError(w, r, deps.Logger, agentUnavailable(deps))
			return
		}
		var request issueAgentTokenRequest
		if r.ContentLength > 0 {
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&request); err != nil {
				WriteError(w, r, deps.Logger, apperr.New(apperr.KindInvalid, "invalid_request", "Invalid token request."))
				return
			}
		}
		if request.ExpiresInSeconds < 0 {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindInvalid, "invalid_request", "expires_in_seconds must be positive."))
			return
		}
		ttl := time.Duration(request.ExpiresInSeconds) * time.Second
		issued, err := deps.Agents.IssueToken(r.Context(), principal.OrganizationID,
			chi.URLParam(r, "agentID"), principal.ID, ttl)
		if err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		WriteJSON(w, http.StatusCreated, issued)
	}
}

func listAgentTokensHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := identity.PrincipalFrom(r.Context())
		if !ok || deps.Agents == nil {
			WriteError(w, r, deps.Logger, agentUnavailable(deps))
			return
		}
		tokens, err := deps.Agents.ListTokens(r.Context(), principal.OrganizationID, chi.URLParam(r, "agentID"))
		if err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"tokens": tokens})
	}
}

func revokeAgentTokenHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := identity.PrincipalFrom(r.Context())
		if !ok || deps.Agents == nil {
			WriteError(w, r, deps.Logger, agentUnavailable(deps))
			return
		}
		if err := deps.Agents.RevokeToken(r.Context(), principal.OrganizationID,
			chi.URLParam(r, "agentID"), chi.URLParam(r, "tokenID"), principal.ID); err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"status": "revoked"})
	}
}

// agentMeHandler lets any authenticated principal (human or agent) confirm the
// identity and scopes resolved for the request.
func agentMeHandler(w http.ResponseWriter, r *http.Request) {
	principal, ok := identity.PrincipalFrom(r.Context())
	if !ok {
		WriteError(w, r, nil, apperr.New(apperr.KindUnauthenticated, "authentication_required", "Authentication is required."))
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"principal": principal})
}
