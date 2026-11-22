package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"atlas/internal/identity"
	"atlas/internal/platform/apperr"
)

type approvalRequest struct {
	OperationType string `json:"operation_type"`
	ObjectType    string `json:"object_type"`
	ObjectID      string `json:"object_id"`
}

type approvalDecisionRequest struct {
	Approve bool   `json:"approve"`
	Reason  string `json:"reason"`
}

type policyRequest struct {
	Name   string         `json:"name"`
	Config map[string]any `json:"config"`
}

func requestApprovalHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := identity.PrincipalFrom(r.Context())
		if !ok || deps.Governance == nil {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnavailable, "governance_unavailable", "Governance is not configured."))
			return
		}
		var request approvalRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&request); err != nil ||
			request.OperationType == "" || request.ObjectType == "" || request.ObjectID == "" {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindInvalid, "invalid_request", "operation_type, object_type and object_id are required."))
			return
		}
		id, err := deps.Governance.RequestApproval(r.Context(), principal.OrganizationID,
			request.OperationType, request.ObjectType, request.ObjectID, principal.ID)
		if err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		WriteJSON(w, http.StatusCreated, map[string]any{"approval_id": id})
	}
}

func decideApprovalHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := identity.PrincipalFrom(r.Context())
		if !ok || deps.Governance == nil {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnavailable, "governance_unavailable", "Governance is not configured."))
			return
		}
		var request approvalDecisionRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&request); err != nil {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindInvalid, "invalid_request", "A decision is required."))
			return
		}
		if err := deps.Governance.Decide(r.Context(), principal.OrganizationID,
			chi.URLParam(r, "approvalID"), principal.ID, request.Approve, request.Reason); err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"status": "decided"})
	}
}

func setPolicyHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := identity.PrincipalFrom(r.Context())
		if !ok || deps.Governance == nil {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnavailable, "governance_unavailable", "Governance is not configured."))
			return
		}
		var request policyRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&request); err != nil || request.Name == "" {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindInvalid, "invalid_request", "Policy name and config are required."))
			return
		}
		config, _ := json.Marshal(request.Config)
		version, err := deps.Governance.SetPolicy(r.Context(), principal.OrganizationID, request.Name, config)
		if err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		WriteJSON(w, http.StatusCreated, map[string]any{"version": version})
	}
}

func getPolicyHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := identity.PrincipalFrom(r.Context())
		if !ok || deps.Governance == nil {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnavailable, "governance_unavailable", "Governance is not configured."))
			return
		}
		policy, err := deps.Governance.ActivePolicy(r.Context(), principal.OrganizationID)
		if err != nil {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindNotFound, "policy_not_found", "No active policy."))
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"policy": json.RawMessage(policy)})
	}
}
