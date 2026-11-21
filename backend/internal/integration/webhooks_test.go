package integration

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"atlas/internal/connectors"
	"atlas/internal/httpapi"
	"atlas/internal/ingestion"
	"atlas/internal/jobs"
	"atlas/internal/secrets"
)

func TestWebhookVerificationAndDedupe(t *testing.T) {
	admin, app, _ := setupTenantSchema(t)
	organizationID := seedQueueOrganization(t, admin)
	sealer, err := secrets.NewSealer(strings.Repeat("cd", 32))
	if err != nil {
		t.Fatalf("sealer: %v", err)
	}
	registry := connectors.NewRegistry()
	registry.Register("mock", connectors.NewMock)
	service := ingestion.NewService(app, registry, sealer)
	ctx := context.Background()

	integrationID, err := service.CreateIntegration(ctx, organizationID, "mock", "mock", "Webhook ERP", map[string]any{})
	if err != nil {
		t.Fatalf("create integration: %v", err)
	}
	if _, err := admin.Exec(ctx, `UPDATE integrations SET config = '{"webhook_secret":"whsec"}' WHERE id = $1`, integrationID); err != nil {
		t.Fatalf("set webhook secret: %v", err)
	}

	router := httpapi.NewRouter(httpapi.Deps{
		Logger: testLogger(), Pool: app, Integrations: service, Queue: jobs.NewQueue(app),
	})
	body := []byte(`{"object":"invoice","id":"evt-payload"}`)
	post := func(eventID, timestamp, signature string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost,
			"/v1/webhooks/"+organizationID+"/"+integrationID, bytes.NewReader(body))
		request.Header.Set("X-Atlas-Event-ID", eventID)
		request.Header.Set("X-Atlas-Timestamp", timestamp)
		request.Header.Set("X-Atlas-Signature", signature)
		request.Header.Set("X-Atlas-Event-Type", "invoice.updated")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		return recorder
	}
	sign := func(timestamp string, payload []byte) string {
		mac := hmac.New(sha256.New, []byte("whsec"))
		mac.Write([]byte(timestamp + "."))
		mac.Write(payload)
		return hex.EncodeToString(mac.Sum(nil))
	}

	now := strconv.FormatInt(time.Now().Unix(), 10)
	if response := post("evt-1", now, "deadbeef"); response.Code != http.StatusUnauthorized {
		t.Fatalf("bad signature status = %d", response.Code)
	}
	stale := strconv.FormatInt(time.Now().Add(-time.Hour).Unix(), 10)
	if response := post("evt-1", stale, sign(stale, body)); response.Code != http.StatusUnauthorized {
		t.Fatalf("stale timestamp status = %d", response.Code)
	}
	if response := post("evt-1", now, sign(now, body)); response.Code != http.StatusOK {
		t.Fatalf("valid webhook status = %d body=%s", response.Code, response.Body.String())
	}
	if response := post("evt-1", now, sign(now, body)); response.Code != http.StatusOK ||
		!strings.Contains(response.Body.String(), `"duplicate":true`) {
		t.Fatalf("duplicate webhook = %d %s", response.Code, response.Body.String())
	}

	var webhookCount, jobCount int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM webhook_events`).Scan(&webhookCount); err != nil {
		t.Fatalf("count webhook events: %v", err)
	}
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE type = 'connector.sync'`).Scan(&jobCount); err != nil {
		t.Fatalf("count sync jobs: %v", err)
	}
	if webhookCount != 1 || jobCount != 1 {
		t.Fatalf("webhooks=%d jobs=%d, want 1/1 (%s)", webhookCount, jobCount, fmt.Sprint(webhookCount, jobCount))
	}
}
