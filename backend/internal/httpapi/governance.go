package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"atlas/internal/governance"
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

type supplierFollowupRequest struct {
	AmountMinor int64  `json:"amount_minor"`
	Note        string `json:"note"`
}

// requestSupplierFollowupHandler creates the governed approval for a supplier
// follow-up on an invoice. Execution remains Atlas-controlled.
func requestSupplierFollowupHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := identity.PrincipalFrom(r.Context())
		if !ok || deps.Governance == nil {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnavailable, "governance_unavailable", "Governance is not configured."))
			return
		}
		var request supplierFollowupRequest
		if r.ContentLength > 0 {
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&request); err != nil {
				WriteError(w, r, deps.Logger, apperr.New(apperr.KindInvalid, "invalid_request", "Invalid follow-up request."))
				return
			}
		}
		if request.AmountMinor < 0 {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindInvalid, "invalid_request", "amount_minor must be non-negative."))
			return
		}
		followUp, err := deps.Governance.RequestSupplierFollowup(r.Context(), principal.OrganizationID,
			chi.URLParam(r, "invoiceID"), request.AmountMinor, governance.Actor{
				Kind: string(principal.Kind), ID: principal.ID, Display: principal.DisplayName,
			})
		if err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		WriteJSON(w, http.StatusCreated, followUp)
	}
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
		id, err := deps.Governance.RequestApprovalAs(r.Context(), principal.OrganizationID,
			request.OperationType, request.ObjectType, request.ObjectID, governance.Actor{
				Kind: string(principal.Kind), ID: principal.ID, Display: principal.DisplayName,
			})
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

func getOperationHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := identity.PrincipalFrom(r.Context())
		if !ok || deps.Governance == nil {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnavailable, "governance_unavailable", "Governance is not configured."))
			return
		}
		operation, err := deps.Governance.GetOperation(r.Context(), principal.OrganizationID, chi.URLParam(r, "operationID"))
		if err != nil {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindNotFound, "operation_not_found", "Operation not found."))
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"operation": operation})
	}
}

func listOperationsHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := identity.PrincipalFrom(r.Context())
		if !ok || deps.Governance == nil {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnavailable, "governance_unavailable", "Governance is not configured."))
			return
		}
		status := r.URL.Query().Get("status")
		switch status {
		case "", "pending", "running", "succeeded", "failed", "cancelled":
		default:
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindInvalid, "invalid_request", "Unknown operation status."))
			return
		}
		limit, err := listLimit(r)
		if err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		operations, err := deps.Governance.ListOperations(r.Context(), principal.OrganizationID, status, limit)
		if err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"operations": operations})
	}
}

func getApprovalHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := identity.PrincipalFrom(r.Context())
		if !ok || deps.Governance == nil {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnavailable, "governance_unavailable", "Governance is not configured."))
			return
		}
		approval, err := deps.Governance.GetApproval(r.Context(), principal.OrganizationID, chi.URLParam(r, "approvalID"))
		if err != nil {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindNotFound, "approval_not_found", "Approval not found."))
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"approval": approval})
	}
}

func listApprovalsHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := identity.PrincipalFrom(r.Context())
		if !ok || deps.Governance == nil {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnavailable, "governance_unavailable", "Governance is not configured."))
			return
		}
		status := r.URL.Query().Get("status")
		switch status {
		case "", "pending", "approved", "rejected", "expired":
		default:
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindInvalid, "invalid_request", "Unknown approval status."))
			return
		}
		limit, err := listLimit(r)
		if err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		approvals, err := deps.Governance.ListApprovals(r.Context(), principal.OrganizationID, status, limit)
		if err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"approvals": approvals})
	}
}

func listLimit(r *http.Request) (int, error) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return 50, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > 200 {
		return 0, apperr.New(apperr.KindInvalid, "invalid_request", "limit must be between 1 and 200.")
	}
	return limit, nil
}
