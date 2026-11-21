package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"atlas/internal/identity"
	"atlas/internal/platform/apperr"
)

type createDocumentRequest struct {
	Kind           string `json:"kind"`
	ContentType    string `json:"content_type"`
	SizeBytes      int64  `json:"size_bytes"`
	SHA256         string `json:"sha256"`
	RetentionClass string `json:"retention_class"`
}

func createDocumentHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := identity.PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnauthenticated, "authentication_required", "Authentication is required."))
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		var request createDocumentRequest
		if err := decoder.Decode(&request); err != nil {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindInvalid, "invalid_request", "The request body is invalid."))
			return
		}
		authorization, err := deps.Documents.CreateUpload(r.Context(), principal.OrganizationID, principal.ID,
			request.Kind, request.ContentType, request.SizeBytes, request.SHA256, request.RetentionClass)
		if err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		WriteJSON(w, http.StatusCreated, map[string]any{
			"document": authorization.Document,
			"upload":   authorization.Upload,
		})
	}
}

func finalizeDocumentHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := identity.PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnauthenticated, "authentication_required", "Authentication is required."))
			return
		}
		document, err := deps.Documents.Finalize(r.Context(), principal.OrganizationID, chi.URLParam(r, "documentID"))
		if err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		if deps.Integrations != nil {
			if err := deps.Integrations.IngestDocument(r.Context(), principal.OrganizationID, document.ID,
				document.Kind, document.ContentType, document.SizeBytes, document.SHA256); err != nil {
				deps.Logger.Warn("document_inbox_ingestion_failed", "document_id", document.ID, "error", err.Error())
			}
		}
		WriteJSON(w, http.StatusOK, map[string]any{"document": document})
	}
}

func getDocumentHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := identity.PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnauthenticated, "authentication_required", "Authentication is required."))
			return
		}
		document, err := deps.Documents.Get(r.Context(), principal.OrganizationID, chi.URLParam(r, "documentID"))
		if err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"document": document})
	}
}

func downloadDocumentHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := identity.PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnauthenticated, "authentication_required", "Authentication is required."))
			return
		}
		document, request, err := deps.Documents.Download(r.Context(), principal.OrganizationID, chi.URLParam(r, "documentID"), principal.ID)
		if err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"document": document, "download": request})
	}
}
