package identity

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// OIDCState is the sealed per-login state carried in a short-lived cookie.
// It protects the authorization-code flow against CSRF (state) and carries the
// optional organization scope and explicit-linking user id.
type OIDCState struct {
	State               string `json:"state"`
	OrganizationID      string `json:"organization_id,omitempty"`
	LinkUserID          string `json:"link_user_id,omitempty"`
	InvitationTokenHash string `json:"invitation_token_hash,omitempty"`
	ExpiresAt           int64  `json:"expires_at"`
}

func stateKey(apiKey string) []byte {
	sum := sha256.Sum256([]byte("atlas-oidc-state:" + apiKey))
	return sum[:]
}

// SealOIDCState signs the state with HMAC-SHA256 (temporary until a dedicated
// secret-management decision exists, ADR-001).
func SealOIDCState(state OIDCState, apiKey string) (string, error) {
	payload, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, stateKey(apiKey))
	mac.Write(payload)
	signature := mac.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

// OpenOIDCState verifies and decodes the sealed state.
func OpenOIDCState(value, apiKey string, now time.Time) (OIDCState, error) {
	var state OIDCState
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return state, errors.New("malformed oidc state")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return state, errors.New("malformed oidc state payload")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return state, errors.New("malformed oidc state signature")
	}
	mac := hmac.New(sha256.New, stateKey(apiKey))
	mac.Write(payload)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return state, errors.New("invalid oidc state signature")
	}
	if err := json.Unmarshal(payload, &state); err != nil {
		return state, errors.New("invalid oidc state contents")
	}
	if state.State == "" || state.ExpiresAt == 0 {
		return state, errors.New("incomplete oidc state")
	}
	if now.Unix() > state.ExpiresAt {
		return state, errors.New("oidc state expired")
	}
	return state, nil
}

// NewOIDCState builds state with a random nonce and a ten-minute lifetime.
func NewOIDCState(organizationID, linkUserID, invitationTokenHash string, now time.Time) (OIDCState, error) {
	nonce, _, err := NewSessionToken()
	if err != nil {
		return OIDCState{}, fmt.Errorf("generate oidc state: %w", err)
	}
	return OIDCState{
		State:               nonce,
		OrganizationID:      organizationID,
		LinkUserID:          linkUserID,
		InvitationTokenHash: invitationTokenHash,
		ExpiresAt:           now.Add(10 * time.Minute).Unix(),
	}, nil
}
