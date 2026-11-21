package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"atlas/internal/platform/migrate"
)

type healthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
	Version string `json:"version"`
	Time    string `json:"time"`
}

type checkResult struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type readinessResponse struct {
	Status string        `json:"status"`
	Checks []checkResult `json:"checks"`
}

func healthHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		WriteJSON(w, http.StatusOK, healthResponse{
			Status:  "ok",
			Service: deps.ServiceName,
			Version: deps.Version,
			Time:    time.Now().UTC().Format(time.RFC3339),
		})
	}
}

func readinessHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		ready := true
		checks := make([]checkResult, 0, 2)

		databaseCheck := checkResult{Name: "database", Status: "pass"}
		if deps.Pool == nil {
			databaseCheck.Status = "fail"
			databaseCheck.Detail = "connection pool is not configured"
			ready = false
		} else if err := deps.Pool.Ping(ctx); err != nil {
			databaseCheck.Status = "fail"
			databaseCheck.Detail = "database unreachable"
			ready = false
		}
		checks = append(checks, databaseCheck)

		migrationCheck := checkResult{Name: "migrations", Status: "pass"}
		switch {
		case deps.Migrations == nil:
			migrationCheck.Status = "fail"
			migrationCheck.Detail = "migrations are not configured"
			ready = false
		case deps.Pool == nil:
			migrationCheck.Status = "fail"
			migrationCheck.Detail = "database is not configured"
			ready = false
		default:
			pending, err := migrate.Pending(ctx, deps.Pool, deps.Migrations)
			switch {
			case err != nil:
				migrationCheck.Status = "fail"
				migrationCheck.Detail = "migration state unavailable"
				ready = false
			case pending > 0:
				migrationCheck.Status = "fail"
				migrationCheck.Detail = fmt.Sprintf("%d migration(s) pending", pending)
				ready = false
			default:
				migrationCheck.Detail = "schema up to date"
			}
		}
		checks = append(checks, migrationCheck)

		status := http.StatusOK
		label := "ready"
		if !ready {
			status = http.StatusServiceUnavailable
			label = "not_ready"
		}
		WriteJSON(w, status, readinessResponse{Status: label, Checks: checks})
	}
}
