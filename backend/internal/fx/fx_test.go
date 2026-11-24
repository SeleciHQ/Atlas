package fx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const ecbFixture = `<?xml version="1.0" encoding="UTF-8"?>
<gesmes:Envelope xmlns:gesmes="http://www.gesmes.org/xml/2002-08-01">
  <Cube>
    <Cube time="2026-11-21">
      <Cube currency="USD" rate="1.0850"/>
      <Cube currency="GBP" rate="0.8450"/>
    </Cube>
  </Cube>
</gesmes:Envelope>`

func TestECBProviderParsesRates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(ecbFixture))
	}))
	defer server.Close()

	provider := &ECBProvider{URL: server.URL, Client: &http.Client{Timeout: time.Second}}
	rates, err := provider.DailyRates(context.Background())
	if err != nil {
		t.Fatalf("daily rates: %v", err)
	}
	if len(rates) != 2 {
		t.Fatalf("rates = %+v", rates)
	}
	if rates[0].Base != "EUR" || rates[0].Quote != "USD" || rates[0].Rate != 1.085 {
		t.Fatalf("first rate = %+v", rates[0])
	}
	if got := rates[0].EffectiveAt.Format("2006-01-02"); got != "2026-11-21" {
		t.Fatalf("effective date = %s", got)
	}
}

func TestECBProviderRejectsBadResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()
	provider := &ECBProvider{URL: server.URL}
	if _, err := provider.DailyRates(context.Background()); err == nil {
		t.Fatal("non-200 response was accepted")
	}
}
