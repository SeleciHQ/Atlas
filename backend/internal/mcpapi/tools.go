package mcpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"atlas/internal/agentaccess"
	"atlas/internal/governance"
	"atlas/internal/identity"
	"atlas/internal/query"
)

const (
	scopeInvoices        = agentaccess.ScopeInvoicesRead
	scopePurchaseOrders  = agentaccess.ScopePurchaseOrdersRead
	scopeDeliveries      = agentaccess.ScopeDeliveriesRead
	scopeSuppliers       = agentaccess.ScopeSuppliersRead
	scopeReconciliationR = agentaccess.ScopeReconciliationRead
	scopeReconciliationX = agentaccess.ScopeReconciliationRun
	scopeDiscrepancies   = agentaccess.ScopeDiscrepanciesRead
	scopeActionsRequest  = agentaccess.ScopeActionsRequest
	scopeApprovals       = agentaccess.ScopeApprovalsRequest
	scopeOperations      = agentaccess.ScopeOperationsRead
)

type tool struct {
	name        string
	description string
	scope       string
	inputSchema map[string]any
	handler     func(context.Context, identity.Principal, json.RawMessage) (any, error)
}

func newTools(deps Deps) []tool {
	return []tool{
		{
			name:        "search_invoices",
			description: "Find invoices by supplier, number, currency, amount range or limit.",
			scope:       scopeInvoices,
			inputSchema: objectSchema(map[string]any{
				"supplier_id":     stringProperty("Supplier UUID"),
				"number":          stringProperty("Invoice number (partial match)"),
				"currency":        stringProperty("ISO-4217 currency"),
				"min_total_minor": integerProperty("Minimum total in minor units"),
				"max_total_minor": integerProperty("Maximum total in minor units"),
				"limit":           limitProperty(),
			}),
			handler: func(ctx context.Context, principal identity.Principal, raw json.RawMessage) (any, error) {
				var args struct {
					SupplierID    string `json:"supplier_id"`
					Number        string `json:"number"`
					Currency      string `json:"currency"`
					MinTotalMinor *int64 `json:"min_total_minor"`
					MaxTotalMinor *int64 `json:"max_total_minor"`
					Limit         int    `json:"limit"`
				}
				if err := decodeArgs(raw, &args); err != nil {
					return nil, err
				}
				invoices, err := deps.Query.SearchInvoices(ctx, principal.OrganizationID, query.InvoiceFilters{
					SupplierID: args.SupplierID, Number: args.Number, Currency: args.Currency,
					MinTotalMinor: args.MinTotalMinor, MaxTotalMinor: args.MaxTotalMinor, Limit: args.Limit,
				})
				if err != nil {
					return nil, err
				}
				return map[string]any{"invoices": invoices}, nil
			},
		},
		{
			name:        "get_invoice",
			description: "Retrieve one invoice with lines and linked purchase order ids.",
			scope:       scopeInvoices,
			inputSchema: objectSchema(map[string]any{
				"invoice_id": stringProperty("Invoice UUID"),
			}, "invoice_id"),
			handler: func(ctx context.Context, principal identity.Principal, raw json.RawMessage) (any, error) {
				var args struct {
					InvoiceID string `json:"invoice_id"`
				}
				if err := decodeArgs(raw, &args); err != nil {
					return nil, err
				}
				if args.InvoiceID == "" {
					return nil, fmt.Errorf("invoice_id is required")
				}
				invoice, err := deps.Query.GetInvoice(ctx, principal.OrganizationID, args.InvoiceID)
				if err != nil {
					return nil, fmt.Errorf("invoice not found")
				}
				return map[string]any{"invoice": invoice}, nil
			},
		},
		{
			name:        "search_purchase_orders",
			description: "Find purchase orders by supplier, number, currency, amount range or limit.",
			scope:       scopePurchaseOrders,
			inputSchema: objectSchema(map[string]any{
				"supplier_id":     stringProperty("Supplier UUID"),
				"number":          stringProperty("Purchase order number (partial match)"),
				"currency":        stringProperty("ISO-4217 currency"),
				"min_total_minor": integerProperty("Minimum total in minor units"),
				"max_total_minor": integerProperty("Maximum total in minor units"),
				"limit":           limitProperty(),
			}),
			handler: func(ctx context.Context, principal identity.Principal, raw json.RawMessage) (any, error) {
				var args struct {
					SupplierID    string `json:"supplier_id"`
					Number        string `json:"number"`
					Currency      string `json:"currency"`
					MinTotalMinor *int64 `json:"min_total_minor"`
					MaxTotalMinor *int64 `json:"max_total_minor"`
					Limit         int    `json:"limit"`
				}
				if err := decodeArgs(raw, &args); err != nil {
					return nil, err
				}
				orders, err := deps.Query.SearchPurchaseOrders(ctx, principal.OrganizationID, query.PurchaseOrderFilters{
					SupplierID: args.SupplierID, Number: args.Number, Currency: args.Currency,
					MinTotalMinor: args.MinTotalMinor, MaxTotalMinor: args.MaxTotalMinor, Limit: args.Limit,
				})
				if err != nil {
					return nil, err
				}
				return map[string]any{"purchase_orders": orders}, nil
			},
		},
		{
			name:        "get_delivery_records",
			description: "List delivery records for an invoice or purchase order.",
			scope:       scopeDeliveries,
			inputSchema: objectSchema(map[string]any{
				"invoice_id":        stringProperty("Invoice UUID"),
				"purchase_order_id": stringProperty("Purchase order UUID"),
				"limit":             limitProperty(),
			}),
			handler: func(ctx context.Context, principal identity.Principal, raw json.RawMessage) (any, error) {
				var args struct {
					InvoiceID       string `json:"invoice_id"`
					PurchaseOrderID string `json:"purchase_order_id"`
					Limit           int    `json:"limit"`
				}
				if err := decodeArgs(raw, &args); err != nil {
					return nil, err
				}
				deliveries, err := deps.Query.ListDeliveries(ctx, principal.OrganizationID,
					args.InvoiceID, args.PurchaseOrderID, args.Limit)
				if err != nil {
					return nil, err
				}
				return map[string]any{"deliveries": deliveries}, nil
			},
		},
		{
			name:        "get_supplier",
			description: "Retrieve a supplier profile with invoice and order counts.",
			scope:       scopeSuppliers,
			inputSchema: objectSchema(map[string]any{
				"supplier_id": stringProperty("Supplier UUID"),
			}, "supplier_id"),
			handler: func(ctx context.Context, principal identity.Principal, raw json.RawMessage) (any, error) {
				var args struct {
					SupplierID string `json:"supplier_id"`
				}
				if err := decodeArgs(raw, &args); err != nil {
					return nil, err
				}
				if args.SupplierID == "" {
					return nil, fmt.Errorf("supplier_id is required")
				}
				supplier, err := deps.Query.GetSupplier(ctx, principal.OrganizationID, args.SupplierID)
				if err != nil {
					return nil, fmt.Errorf("supplier not found")
				}
				return map[string]any{"supplier": supplier}, nil
			},
		},
		{
			name:        "reconcile_invoice",
			description: "Queue a deterministic reconciliation run for an invoice and return a job id.",
			scope:       scopeReconciliationX,
			inputSchema: objectSchema(map[string]any{
				"invoice_id":      stringProperty("Invoice UUID"),
				"idempotency_key": stringProperty("Optional client idempotency key"),
			}, "invoice_id"),
			handler: func(ctx context.Context, principal identity.Principal, raw json.RawMessage) (any, error) {
				var args struct {
					InvoiceID      string `json:"invoice_id"`
					IdempotencyKey string `json:"idempotency_key"`
				}
				if err := decodeArgs(raw, &args); err != nil {
					return nil, err
				}
				if args.InvoiceID == "" {
					return nil, fmt.Errorf("invoice_id is required")
				}
				idempotencyKey := strings.TrimSpace(args.IdempotencyKey)
				if idempotencyKey == "" {
					idempotencyKey = uuid.NewString()
				}
				payload, err := json.Marshal(map[string]string{"invoice_id": args.InvoiceID})
				if err != nil {
					return nil, err
				}
				operationID, created, err := deps.Governance.StartJobOperation(ctx, principal.OrganizationID,
					"reconciliation.run", "reconciliation.run", idempotencyKey, payload)
				if err != nil {
					return nil, err
				}
				return map[string]any{
					"operation_id": operationID, "status": "queued", "created": created,
				}, nil
			},
		},
		{
			name:        "get_reconciliation",
			description: "Retrieve a reconciliation run by id, or list runs for an invoice.",
			scope:       scopeReconciliationR,
			inputSchema: objectSchema(map[string]any{
				"run_id":     stringProperty("Reconciliation run UUID"),
				"invoice_id": stringProperty("Invoice UUID"),
			}),
			handler: func(ctx context.Context, principal identity.Principal, raw json.RawMessage) (any, error) {
				var args struct {
					RunID     string `json:"run_id"`
					InvoiceID string `json:"invoice_id"`
				}
				if err := decodeArgs(raw, &args); err != nil {
					return nil, err
				}
				switch {
				case args.RunID != "":
					detail, err := deps.Query.GetReconciliation(ctx, principal.OrganizationID, args.RunID)
					if err != nil {
						return nil, fmt.Errorf("reconciliation run not found")
					}
					return map[string]any{"reconciliation": detail}, nil
				case args.InvoiceID != "":
					runs, err := deps.Query.ListReconciliationRuns(ctx, principal.OrganizationID, args.InvoiceID)
					if err != nil {
						return nil, err
					}
					return map[string]any{"reconciliation_runs": runs}, nil
				default:
					return nil, fmt.Errorf("run_id or invoice_id is required")
				}
			},
		},
		{
			name:        "search_discrepancies",
			description: "Find reconciliation discrepancies by kind, severity, invoice, run or supplier.",
			scope:       scopeDiscrepancies,
			inputSchema: objectSchema(map[string]any{
				"kind":        stringProperty("Discrepancy kind"),
				"severity":    stringProperty("info, warning or error"),
				"invoice_id":  stringProperty("Invoice UUID"),
				"run_id":      stringProperty("Reconciliation run UUID"),
				"supplier_id": stringProperty("Supplier UUID"),
				"limit":       limitProperty(),
			}),
			handler: func(ctx context.Context, principal identity.Principal, raw json.RawMessage) (any, error) {
				var args struct {
					Kind       string `json:"kind"`
					Severity   string `json:"severity"`
					InvoiceID  string `json:"invoice_id"`
					RunID      string `json:"run_id"`
					SupplierID string `json:"supplier_id"`
					Limit      int    `json:"limit"`
				}
				if err := decodeArgs(raw, &args); err != nil {
					return nil, err
				}
				findings, err := deps.Query.SearchDiscrepancies(ctx, principal.OrganizationID, query.DiscrepancyFilters{
					Kind: args.Kind, Severity: args.Severity, InvoiceID: args.InvoiceID,
					RunID: args.RunID, SupplierID: args.SupplierID, Limit: args.Limit,
				})
				if err != nil {
					return nil, err
				}
				return map[string]any{"discrepancies": findings}, nil
			},
		},
		{
			name:        "request_supplier_followup",
			description: "Request a governed supplier follow-up for an invoice. Returns an approval id.",
			scope:       scopeActionsRequest,
			inputSchema: objectSchema(map[string]any{
				"invoice_id":   stringProperty("Invoice UUID"),
				"amount_minor": integerProperty("Optional amount for policy thresholds"),
				"note":         stringProperty("Optional note for the reviewer"),
			}, "invoice_id"),
			handler: func(ctx context.Context, principal identity.Principal, raw json.RawMessage) (any, error) {
				var args struct {
					InvoiceID   string `json:"invoice_id"`
					AmountMinor int64  `json:"amount_minor"`
					Note        string `json:"note"`
				}
				if err := decodeArgs(raw, &args); err != nil {
					return nil, err
				}
				if args.InvoiceID == "" {
					return nil, fmt.Errorf("invoice_id is required")
				}
				followUp, err := deps.Governance.RequestSupplierFollowup(ctx, principal.OrganizationID, args.InvoiceID,
					args.AmountMinor, governance.Actor{
						Kind: string(principal.Kind), ID: principal.ID, Display: principal.DisplayName,
					})
				if err != nil {
					return nil, err
				}
				return followUp, nil
			},
		},
		{
			name:        "request_approval",
			description: "Request human approval for a specific proposed action.",
			scope:       scopeApprovals,
			inputSchema: objectSchema(map[string]any{
				"operation_type": stringProperty("Action/operation type"),
				"object_type":    stringProperty("Resource type, for example invoice"),
				"object_id":      stringProperty("Resource id"),
			}, "operation_type", "object_type", "object_id"),
			handler: func(ctx context.Context, principal identity.Principal, raw json.RawMessage) (any, error) {
				var args struct {
					OperationType string `json:"operation_type"`
					ObjectType    string `json:"object_type"`
					ObjectID      string `json:"object_id"`
				}
				if err := decodeArgs(raw, &args); err != nil {
					return nil, err
				}
				if args.OperationType == "" || args.ObjectType == "" || args.ObjectID == "" {
					return nil, fmt.Errorf("operation_type, object_type and object_id are required")
				}
				approvalID, err := deps.Governance.RequestApprovalAs(ctx, principal.OrganizationID,
					args.OperationType, args.ObjectType, args.ObjectID,
					governance.Actor{Kind: string(identity.KindAgent), ID: principal.ID, Display: principal.DisplayName})
				if err != nil {
					return nil, err
				}
				return map[string]any{"approval_id": approvalID, "status": "pending"}, nil
			},
		},
		{
			name:        "get_operation_status",
			description: "Poll the status, attempts and result of a governed operation.",
			scope:       scopeOperations,
			inputSchema: objectSchema(map[string]any{
				"operation_id": stringProperty("Operation UUID"),
			}, "operation_id"),
			handler: func(ctx context.Context, principal identity.Principal, raw json.RawMessage) (any, error) {
				var args struct {
					OperationID string `json:"operation_id"`
				}
				if err := decodeArgs(raw, &args); err != nil {
					return nil, err
				}
				if args.OperationID == "" {
					return nil, fmt.Errorf("operation_id is required")
				}
				operation, err := deps.Governance.GetOperation(ctx, principal.OrganizationID, args.OperationID)
				if err != nil {
					return nil, fmt.Errorf("operation not found")
				}
				return map[string]any{"operation": operation}, nil
			},
		},
	}
}

func decodeArgs(raw json.RawMessage, target any) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = json.RawMessage(`{}`)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	return nil
}

func objectSchema(properties map[string]any, required ...string) map[string]any {
	schema := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func stringProperty(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func integerProperty(description string) map[string]any {
	return map[string]any{"type": "integer", "description": description, "minimum": 0}
}

func limitProperty() map[string]any {
	return map[string]any{
		"type": "integer", "description": "Maximum rows to return (1-100, default 25)",
		"minimum": 1, "maximum": 100,
	}
}
