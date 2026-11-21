package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"atlas/internal/authorization"
	"atlas/internal/identity"
	"atlas/internal/platform/apperr"
)

const oidcStateCookieName = "atlas_oidc_state"

type invitationRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

func setOIDCStateCookie(w http.ResponseWriter, deps Deps, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     oidcStateCookieName,
		Value:    value,
		Path:     "/v1/auth/oidc",
		HttpOnly: true,
		Secure:   deps.SecureCookies,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
		Expires:  time.Now().Add(time.Duration(maxAge) * time.Second),
	})
}

func clearOIDCStateCookie(w http.ResponseWriter, deps Deps) {
	http.SetCookie(w, &http.Cookie{
		Name:     oidcStateCookieName,
		Value:    "",
		Path:     "/v1/auth/oidc",
		HttpOnly: true,
		Secure:   deps.SecureCookies,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
}

// oidcStartHandler begins the managed-OIDC login. With ?link=1 it requires an
// authenticated Atlas session and records an explicit account-linking intent.
func oidcStartHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !deps.OIDCEnabled || deps.OIDC == nil {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindNotFound, "not_found", "Managed sign-in is not configured."))
			return
		}
		query := r.URL.Query()
		linkUserID := ""
		if query.Get("link") == "1" {
			principal, ok := identity.PrincipalFrom(r.Context())
			if !ok {
				WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnauthenticated, "authentication_required", "Sign in to the existing account before linking an identity."))
				return
			}
			linkUserID = principal.ID
		}
		result, err := deps.OIDC.Start(query.Get("organization_id"), query.Get("login_hint"), linkUserID, query.Get("invitation_token"))
		if err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		setOIDCStateCookie(w, deps, result.StateCookie, result.StateMaxAge)
		http.Redirect(w, r, result.AuthorizationURL, http.StatusFound)
	}
}

// oidcCallbackHandler completes the code exchange, creates the Atlas session,
// and returns the browser to the work surface.
func oidcCallbackHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !deps.OIDCEnabled || deps.OIDC == nil {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindNotFound, "not_found", "Managed sign-in is not configured."))
			return
		}
		query := r.URL.Query()
		if providerError := query.Get("error"); providerError != "" {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnauthenticated, "oidc_denied", "Sign-in was not completed."))
			return
		}
		stateCookie, err := r.Cookie(oidcStateCookieName)
		if err != nil {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnauthenticated, "invalid_oidc_state", "The sign-in attempt expired or could not be verified."))
			return
		}
		token, principal, expiresAt, err := deps.OIDC.Callback(
			r.Context(), query.Get("code"), stateCookie.Value, r.RemoteAddr, r.UserAgent(),
		)
		clearOIDCStateCookie(w, deps)
		if err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		setSessionCookie(w, deps, token, expiresAt)
		if query.Get("format") == "json" {
			WriteJSON(w, http.StatusOK, map[string]any{"principal": principal, "expires_at": expiresAt.UTC()})
			return
		}
		http.Redirect(w, r, "/work", http.StatusSeeOther)
	}
}

// createInvitationHandler issues an organization invitation (admin only).
func createInvitationHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := identity.PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnauthenticated, "authentication_required", "Authentication is required."))
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		var request invitationRequest
		if err := decoder.Decode(&request); err != nil {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindInvalid, "invalid_request", "The request body is invalid."))
			return
		}
		invitation, token, err := deps.Invitations.Create(r.Context(), principal.OrganizationID, request.Email, request.Role, principal.ID, principal.DisplayName)
		if err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		WriteJSON(w, http.StatusCreated, map[string]any{
			"invitation": map[string]any{
				"id":              invitation.ID,
				"organization_id": invitation.OrganizationID,
				"email":           invitation.Email,
				"role":            invitation.Role,
				"expires_at":      invitation.ExpiresAt.UTC(),
			},
			"accept_token": token,
		})
	}
}

var _ = authorization.PermissionMembersManage
