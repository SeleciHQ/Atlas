package identity

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"atlas/internal/platform/apperr"
)

// WorkOSConfig configures the managed OIDC (WorkOS AuthKit) adapter.
type WorkOSConfig struct {
	ClientID        string
	APIKey          string
	RedirectURI     string
	AuthorizeURL    string
	AuthenticateURL string
}

// WorkOSSUser is the subset of the AuthKit user we consume.
type WorkOSUser struct {
	ID             string
	Email          string
	FirstName      string
	LastName       string
	EmailVerified  bool
	OrganizationID string
}

// WorkOSClient implements the AuthKit server-side authorization-code exchange.
type WorkOSClient struct {
	config WorkOSConfig
	http   *http.Client
}

// NewWorkOSClient constructs the client.
func NewWorkOSClient(config WorkOSConfig) *WorkOSClient {
	return &WorkOSClient{config: config, http: &http.Client{Timeout: 15 * time.Second}}
}

// Issuer is the stable identity issuer identifier for (issuer, subject) keys.
func (c *WorkOSClient) Issuer() string { return "workos:" + c.config.ClientID }

// AuthorizationURL builds the AuthKit authorization URL.
func (c *WorkOSClient) AuthorizationURL(state, organizationID, loginHint string) string {
	values := url.Values{}
	values.Set("client_id", c.config.ClientID)
	values.Set("redirect_uri", c.config.RedirectURI)
	values.Set("response_type", "code")
	values.Set("state", state)
	if organizationID != "" {
		values.Set("organization_id", organizationID)
	}
	if loginHint != "" {
		values.Set("login_hint", loginHint)
	}
	return c.config.AuthorizeURL + "?" + values.Encode()
}

type workosAuthenticateResponse struct {
	User struct {
		ID            string `json:"id"`
		Email         string `json:"email"`
		FirstName     string `json:"first_name"`
		LastName      string `json:"last_name"`
		EmailVerified bool   `json:"email_verified"`
	} `json:"user"`
	OrganizationID string `json:"organization_id"`
}

// AuthenticateCode exchanges an authorization code server-side.
func (c *WorkOSClient) AuthenticateCode(ctx context.Context, code, ipAddress, userAgent string) (WorkOSUser, error) {
	body, err := json.Marshal(map[string]string{
		"client_id":     c.config.ClientID,
		"client_secret": c.config.APIKey,
		"grant_type":    "authorization_code",
		"code":          code,
		"ip_address":    ipAddress,
		"user_agent":    userAgent,
	})
	if err != nil {
		return WorkOSUser{}, fmt.Errorf("encode authenticate request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.AuthenticateURL, bytes.NewReader(body))
	if err != nil {
		return WorkOSUser{}, fmt.Errorf("build authenticate request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return WorkOSUser{}, apperr.New(apperr.KindUnavailable, "oidc_unavailable", "The identity provider could not be reached.").WithCause(err)
	}
	defer response.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return WorkOSUser{}, apperr.New(apperr.KindUnauthenticated, "oidc_exchange_failed", "The sign-in could not be completed.").
			WithCause(fmt.Errorf("workos authenticate status %d", response.StatusCode))
	}
	var parsed workosAuthenticateResponse
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return WorkOSUser{}, fmt.Errorf("decode authenticate response: %w", err)
	}
	if parsed.User.ID == "" || !strings.Contains(parsed.User.Email, "@") {
		return WorkOSUser{}, fmt.Errorf("identity provider returned an incomplete user")
	}
	displayName := strings.TrimSpace(parsed.User.FirstName + " " + parsed.User.LastName)
	if displayName == "" {
		displayName = parsed.User.Email
	}
	return WorkOSUser{
		ID:             parsed.User.ID,
		Email:          strings.ToLower(strings.TrimSpace(parsed.User.Email)),
		FirstName:      parsed.User.FirstName,
		LastName:       parsed.User.LastName,
		EmailVerified:  parsed.User.EmailVerified,
		OrganizationID: parsed.OrganizationID,
	}, nil
}
