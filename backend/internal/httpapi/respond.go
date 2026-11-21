package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"

	"atlas/internal/platform/apperr"
)

// WriteJSON writes a JSON response.
func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(payload)
}

// WriteError maps an error to a problem document and writes it.
func WriteError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	problem := apperr.From(err)
	if problem.RequestID == "" {
		problem.RequestID = middleware.GetReqID(r.Context())
	}
	if problem.Status >= http.StatusInternalServerError && logger != nil {
		logger.ErrorContext(r.Context(), "request_failed",
			"code", problem.Code,
			"request_id", problem.RequestID,
			"error", err.Error(),
		)
	}
	WriteProblem(w, problem)
}

// WriteProblem writes a fully specified problem document.
func WriteProblem(w http.ResponseWriter, problem apperr.Problem) {
	w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
	w.WriteHeader(problem.Status)
	_ = json.NewEncoder(w).Encode(problem)
}
