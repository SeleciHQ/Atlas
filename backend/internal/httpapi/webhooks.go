package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"atlas/internal/ingestion"
	"atlas/internal/jobs"
	"atlas/internal/platform/apperr"
	"atlas/internal/tenancy"
)

const webhookTolerance = 5 * time.Minute

// webhookHandler verifies an integration webhook (HMAC-SHA256 over
// timestamp.body), rejects stale timestamps, persists the raw payload and
// dedupes by external event id before enqueueing a targeted sync.
func webhookHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Integrations == nil {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnavailable, "integrations_unavailable", "Integrations are not configured."))
			return
		}
		organizationID := chi.URLParam(r, "organizationID")
		integrationID := chi.URLParam(r, "integrationID")
		eventID := r.Header.Get("X-Atlas-Event-ID")
		timestamp := r.Header.Get("X-Atlas-Timestamp")
		signature := r.Header.Get("X-Atlas-Signature")
		if eventID == "" || timestamp == "" || signature == "" {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnauthenticated, "invalid_webhook", "Webhook headers are missing."))
			return
		}
		sent, err := strconv.ParseInt(timestamp, 10, 64)
		if err != nil || time.Since(time.Unix(sent, 0)).Abs() > webhookTolerance {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnauthenticated, "stale_webhook", "Webhook timestamp is outside the tolerance window."))
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindInvalid, "invalid_body", "Webhook body is too large."))
			return
		}

		secret, err := deps.Integrations.WebhookSecret(r.Context(), organizationID, integrationID)
		if err != nil || secret == "" {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindNotFound, "webhook_not_configured", "Webhook is not configured for this integration."))
			return
		}
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(timestamp + "."))
		mac.Write(body)
		expected := hex.EncodeToString(mac.Sum(nil))
		if !hmac.Equal([]byte(expected), []byte(signature)) {
			WriteError(w, r, deps.Logger, apperr.New(apperr.KindUnauthenticated, "invalid_signature", "Webhook signature is invalid."))
			return
		}

		created, err := deps.Integrations.RecordWebhook(r.Context(), organizationID, integrationID,
			eventID, r.Header.Get("X-Atlas-Event-Type"), body)
		if err != nil {
			WriteError(w, r, deps.Logger, err)
			return
		}
		if created && deps.Queue != nil {
			if err := tenancy.WithOrganizationTx(r.Context(), deps.Pool, organizationID, func(tx pgx.Tx) error {
				_, _, err := deps.Queue.Enqueue(r.Context(), tx, jobs.NewJob{
					OrganizationID: organizationID,
					Type:           "connector.sync",
					SchemaVersion:  1,
					Payload:        ingestion.SyncJobPayload(organizationID, integrationID),
					IdempotencyKey: "webhook:" + eventID,
				})
				return err
			}); err != nil {
				WriteError(w, r, deps.Logger, err)
				return
			}
		}
		WriteJSON(w, http.StatusOK, map[string]any{"status": "accepted", "duplicate": !created})
	}
}
