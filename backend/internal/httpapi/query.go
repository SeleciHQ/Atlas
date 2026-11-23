package httpapi

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"atlas/internal/identity"
	"atlas/internal/platform/apperr"
	"atlas/internal/query"
)

func queryUnavailable(deps Deps) *apperr.Error {
	return apperr.New(apperr.KindUnavailable, "query_unavailable", "Query services are not configured.")
}

func queryPrincipal(deps Deps, r *http.Request) (identity.Principal, bool) {
	principal, ok := identity.PrincipalFrom(r.Context())
	if !ok || deps.Query == nil {
		return identity.Principal{}, false
	}
	return principal, true
}

func queryMinor(r *http.Request, name string) (*int64, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return nil, apperr.New(apperr.KindInvalid, "invalid_request", name+" must be a non-negative integer.")
	}
	return &value, nil
}

func searchInvoicesHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := queryPrincipal(deps, r)
		if !ok {
			WriteError(w, r, deps.Logger, queryUnavailable(deps))
			return
		}
		limit, err := listLimit(r)
		if err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		minTotal, err := queryMinor(r, "min_total_minor")
		if err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		maxTotal, err := queryMinor(r, "max_total_minor")
		if err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		invoices, err := deps.Query.SearchInvoices(r.Context(), principal.OrganizationID, query.InvoiceFilters{
			SupplierID:    r.URL.Query().Get("supplier_id"),
			Number:        r.URL.Query().Get("number"),
			Currency:      r.URL.Query().Get("currency"),
			MinTotalMinor: minTotal,
			MaxTotalMinor: maxTotal,
			Limit:         limit,
		})
		if err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"invoices": invoices})
	}
}

func getInvoiceHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := queryPrincipal(deps, r)
		if !ok {
			WriteError(w, r, deps.Logger, queryUnavailable(deps))
			return
		}
		invoice, err := deps.Query.GetInvoice(r.Context(), principal.OrganizationID, chi.URLParam(r, "invoiceID"))
		if err != nil {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindNotFound, "invoice_not_found", "Invoice not found."))
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"invoice": invoice})
	}
}

func searchPurchaseOrdersHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := queryPrincipal(deps, r)
		if !ok {
			WriteError(w, r, deps.Logger, queryUnavailable(deps))
			return
		}
		limit, err := listLimit(r)
		if err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		minTotal, err := queryMinor(r, "min_total_minor")
		if err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		maxTotal, err := queryMinor(r, "max_total_minor")
		if err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		orders, err := deps.Query.SearchPurchaseOrders(r.Context(), principal.OrganizationID, query.PurchaseOrderFilters{
			SupplierID:    r.URL.Query().Get("supplier_id"),
			Number:        r.URL.Query().Get("number"),
			Currency:      r.URL.Query().Get("currency"),
			MinTotalMinor: minTotal,
			MaxTotalMinor: maxTotal,
			Limit:         limit,
		})
		if err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"purchase_orders": orders})
	}
}

func getPurchaseOrderHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := queryPrincipal(deps, r)
		if !ok {
			WriteError(w, r, deps.Logger, queryUnavailable(deps))
			return
		}
		order, err := deps.Query.GetPurchaseOrder(r.Context(), principal.OrganizationID, chi.URLParam(r, "purchaseOrderID"))
		if err != nil {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindNotFound, "purchase_order_not_found", "Purchase order not found."))
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"purchase_order": order})
	}
}

func searchSuppliersHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := queryPrincipal(deps, r)
		if !ok {
			WriteError(w, r, deps.Logger, queryUnavailable(deps))
			return
		}
		limit, err := listLimit(r)
		if err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		suppliers, err := deps.Query.SearchSuppliers(r.Context(), principal.OrganizationID,
			r.URL.Query().Get("q"), limit)
		if err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"suppliers": suppliers})
	}
}

func getSupplierHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := queryPrincipal(deps, r)
		if !ok {
			WriteError(w, r, deps.Logger, queryUnavailable(deps))
			return
		}
		supplier, err := deps.Query.GetSupplier(r.Context(), principal.OrganizationID, chi.URLParam(r, "supplierID"))
		if err != nil {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindNotFound, "supplier_not_found", "Supplier not found."))
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"supplier": supplier})
	}
}

func listDeliveriesHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := queryPrincipal(deps, r)
		if !ok {
			WriteError(w, r, deps.Logger, queryUnavailable(deps))
			return
		}
		limit, err := listLimit(r)
		if err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		deliveries, err := deps.Query.ListDeliveries(r.Context(), principal.OrganizationID,
			r.URL.Query().Get("invoice_id"), r.URL.Query().Get("purchase_order_id"), limit)
		if err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"deliveries": deliveries})
	}
}

func searchDiscrepanciesHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := queryPrincipal(deps, r)
		if !ok {
			WriteError(w, r, deps.Logger, queryUnavailable(deps))
			return
		}
		limit, err := listLimit(r)
		if err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		findings, err := deps.Query.SearchDiscrepancies(r.Context(), principal.OrganizationID, query.DiscrepancyFilters{
			Kind:       r.URL.Query().Get("kind"),
			Severity:   r.URL.Query().Get("severity"),
			InvoiceID:  r.URL.Query().Get("invoice_id"),
			RunID:      r.URL.Query().Get("run_id"),
			SupplierID: r.URL.Query().Get("supplier_id"),
			Limit:      limit,
		})
		if err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"discrepancies": findings})
	}
}
