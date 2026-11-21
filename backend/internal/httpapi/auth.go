package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"atlas/internal/authorization"
	"atlas/internal/identity"
	"atlas/internal/platform/apperr"
)

// SessionCookieName is the development session cookie. Production OIDC will
// use an equivalent secure server-side session.
const SessionCookieName = "atlas_session"

type devLoginRequest struct {
	Email          string `json:"email"`
	OrganizationID string `json:"organization_id"`
}

func sessionToken(r *http.Request) string {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func setSessionCookie(w http.ResponseWriter, deps Deps, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   deps.SecureCookies,
		SameSite: http.SameSiteLaxMode,
		Expires:  expiresAt,
		MaxAge:   int(time.Until(expiresAt).Seconds()),
	})
}

func clearSessionCookie(w http.ResponseWriter, deps Deps) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   deps.SecureCookies,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
}

// authenticate resolves the session cookie into a principal. Every protected
// route passes through this middleware; there is no alternate permission path.
func authenticate(deps Deps) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if deps.Identity == nil {
				WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnavailable, "authentication_unavailable", "Authentication is not configured."))
				return
			}
			principal, err := deps.Identity.Authenticate(r.Context(), sessionToken(r))
			if err != nil {
				WriteError(w, r, deps.Logger, err)
				return
			}
			next.ServeHTTP(w, r.WithContext(identity.WithPrincipal(r.Context(), principal)))
		})
	}
}

func requirePermission(deps Deps, permission authorization.Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, ok := identity.PrincipalFrom(r.Context())
			if !ok {
				WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnauthenticated, "authentication_required", "Authentication is required."))
				return
			}
			if err := deps.Authorizer.Authorize(principal, permission); err != nil {
				WriteError(w, r, deps.Logger, err)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// devLoginHandler issues a development session. The route is only registered
// when development authentication is enabled, and configuration prevents it in
// staging/production.
func devLoginHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !deps.DevAuthEnabled || deps.DevSessions == nil {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindNotFound, "not_found", "The requested resource was not found."))
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		var request devLoginRequest
		if err := decoder.Decode(&request); err != nil {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindInvalid, "invalid_request", "The request body is invalid."))
			return
		}

		token, principal, expiresAt, err := deps.DevSessions.DevLogin(r.Context(), request.OrganizationID, request.Email)
		if err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		setSessionCookie(w, deps, token, expiresAt)
		WriteJSON(w, http.StatusOK, map[string]any{
			"principal":  principal,
			"expires_at": expiresAt.UTC(),
		})
	}
}

func meHandler(w http.ResponseWriter, r *http.Request) {
	principal, ok := identity.PrincipalFrom(r.Context())
	if !ok {
		WriteError(w, r, nil, apperr.New(apperr.KindUnauthenticated, "authentication_required", "Authentication is required."))
		return
	}
	WriteJSON(w, http.StatusOK, principal)
}

func logoutHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := deps.Identity.Logout(r.Context(), sessionToken(r)); err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		clearSessionCookie(w, deps)
		w.WriteHeader(http.StatusNoContent)
	}
}

func membersHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := identity.PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnauthenticated, "authentication_required", "Authentication is required."))
			return
		}
		members, err := deps.Orgs.ListMembers(r.Context(), principal.OrganizationID)
		if err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"members": members})
	}
}

func organizationSettingsHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := identity.PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnauthenticated, "authentication_required", "Authentication is required."))
			return
		}
		organization, err := deps.Orgs.GetOrganization(r.Context(), principal.OrganizationID)
		if err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"organization": organization})
	}
}
