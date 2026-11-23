package mcpapi

import (
	"context"
	"encoding/json"
	"strings"

	"atlas/internal/identity"
)

const resourcePrefix = "atlas://organizations/"

type resourceContent struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType"`
	Text     string `json:"text"`
}

func resourceTemplates() any {
	return map[string]any{
		"resourceTemplates": []map[string]any{
			{
				"uriTemplate": "atlas://organizations/{organization_id}/invoices/{invoice_id}",
				"name":        "invoice",
				"description": "Invoice header, lines and linked purchase orders",
				"mimeType":    "application/json",
			},
			{
				"uriTemplate": "atlas://organizations/{organization_id}/reconciliations/{run_id}",
				"name":        "reconciliation",
				"description": "Reconciliation run with matches and discrepancies",
				"mimeType":    "application/json",
			},
		},
	}
}

// parseResourceURI extracts the organization, resource kind and id from an
// Atlas resource URI. It returns ok=false for unsupported shapes.
func parseResourceURI(uri string) (organizationID, kind, id string, ok bool) {
	rest, found := strings.CutPrefix(uri, resourcePrefix)
	if !found {
		return "", "", "", false
	}
	parts := strings.Split(rest, "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[2], true
}

func (h *handler) readResource(ctx context.Context, principal identity.Principal, uri string) (any, *rpcError) {
	organizationID, kind, id, ok := parseResourceURI(uri)
	if !ok {
		return nil, &rpcError{Code: -32602, Message: "unsupported resource uri"}
	}
	if organizationID != principal.OrganizationID {
		return nil, &rpcError{Code: -32003, Message: "resource belongs to another organization"}
	}
	var payload any
	switch kind {
	case "invoices":
		if err := h.deps.Authorizer.AuthorizeScope(principal, scopeInvoices); err != nil {
			return nil, &rpcError{Code: -32003, Message: "insufficient scope for invoice resources"}
		}
		detail, err := h.deps.Query.GetInvoice(ctx, organizationID, id)
		if err != nil {
			return nil, &rpcError{Code: -32602, Message: "invoice resource not found"}
		}
		payload = map[string]any{"invoice": detail}
	case "reconciliations":
		if err := h.deps.Authorizer.AuthorizeScope(principal, scopeReconciliationR); err != nil {
			return nil, &rpcError{Code: -32003, Message: "insufficient scope for reconciliation resources"}
		}
		detail, err := h.deps.Query.GetReconciliation(ctx, organizationID, id)
		if err != nil {
			return nil, &rpcError{Code: -32602, Message: "reconciliation resource not found"}
		}
		payload = map[string]any{"reconciliation": detail}
	default:
		return nil, &rpcError{Code: -32602, Message: "unsupported resource kind"}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, &rpcError{Code: -32603, Message: "encode resource"}
	}
	return map[string]any{
		"contents": []resourceContent{{URI: uri, MimeType: "application/json", Text: string(encoded)}},
	}, nil
}
