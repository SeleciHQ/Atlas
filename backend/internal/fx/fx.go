// Package fx records dated FX rate snapshots and fetches ECB euro reference
// rates (ADR-008). Rates are always stored per organization; the core converter
// consumes these rows and fails closed when a pair is missing.
package fx

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"atlas/internal/audit"
	"atlas/internal/tenancy"
)

const DefaultECBURL = "https://www.ecb.europa.eu/stats/eurofxref/eurofxref-daily.xml"

var currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

// Rate is one dated FX snapshot. ECB data is EUR-based.
type Rate struct {
	Base        string
	Quote       string
	Rate        float64
	EffectiveAt time.Time
}

// Provider fetches a daily rate set.
type Provider interface {
	DailyRates(ctx context.Context) ([]Rate, error)
}

// ECBProvider fetches the European Central Bank euro reference rates.
type ECBProvider struct {
	URL    string
	Client *http.Client
}

// NewECBProvider constructs the ECB provider.
func NewECBProvider() *ECBProvider {
	return &ECBProvider{URL: DefaultECBURL, Client: &http.Client{Timeout: 15 * time.Second}}
}

type ecbEnvelope struct {
	Outer struct {
		Inner struct {
			Time  string `xml:"time,attr"`
			Rates []struct {
				Currency string `xml:"currency,attr"`
				Rate     string `xml:"rate,attr"`
			} `xml:"Cube"`
		} `xml:"Cube"`
	} `xml:"Cube"`
}

// DailyRates downloads and parses the ECB daily reference set.
func (p *ECBProvider) DailyRates(ctx context.Context) ([]Rate, error) {
	if p.URL == "" {
		return nil, fmt.Errorf("ECB URL is not configured")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.URL, nil)
	if err != nil {
		return nil, err
	}
	client := p.Client
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch ECB rates: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch ECB rates: status %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return nil, fmt.Errorf("read ECB rates: %w", err)
	}
	var envelope ecbEnvelope
	if err := xml.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("parse ECB rates: %w", err)
	}
	day := strings.TrimSpace(envelope.Outer.Inner.Time)
	effectiveAt, err := time.Parse("2006-01-02", day)
	if err != nil {
		return nil, fmt.Errorf("parse ECB date %q: %w", day, err)
	}
	rates := make([]Rate, 0, len(envelope.Outer.Inner.Rates))
	for _, entry := range envelope.Outer.Inner.Rates {
		quote := strings.ToUpper(strings.TrimSpace(entry.Currency))
		value, err := strconv.ParseFloat(strings.TrimSpace(entry.Rate), 64)
		if err != nil || value <= 0 || !currencyPattern.MatchString(quote) {
			continue
		}
		rates = append(rates, Rate{Base: "EUR", Quote: quote, Rate: value, EffectiveAt: effectiveAt.UTC()})
	}
	if len(rates) == 0 {
		return nil, fmt.Errorf("ECB response contained no rates")
	}
	return rates, nil
}

// Service records FX snapshots for an organization.
type Service struct {
	pool     *pgxpool.Pool
	provider Provider
}

// NewService constructs the FX service.
func NewService(pool *pgxpool.Pool, provider Provider) *Service {
	return &Service{pool: pool, provider: provider}
}

// RecordRates stores dated rates, ignoring duplicates. source is manual, ecb or organization.
func (s *Service) RecordRates(ctx context.Context, organizationID, source string, rates []Rate) (int, error) {
	if source != "manual" && source != "ecb" && source != "organization" {
		return 0, fmt.Errorf("invalid FX source %q", source)
	}
	for _, rate := range rates {
		if !currencyPattern.MatchString(rate.Base) || !currencyPattern.MatchString(rate.Quote) ||
			rate.Base == rate.Quote || rate.Rate <= 0 || rate.EffectiveAt.IsZero() {
			return 0, fmt.Errorf("invalid FX rate %s/%s", rate.Base, rate.Quote)
		}
	}
	recorded := 0
	err := tenancy.WithOrganizationTx(ctx, s.pool, organizationID, func(tx pgx.Tx) error {
		for _, rate := range rates {
			tag, err := tx.Exec(ctx, `
				INSERT INTO fx_rates
					(organization_id, base_currency, quote_currency, rate, effective_at, source)
				VALUES ($1, $2, $3, $4::numeric, $5, $6)
				ON CONFLICT (organization_id, base_currency, quote_currency, effective_at) DO NOTHING`,
				organizationID, rate.Base, rate.Quote, strconv.FormatFloat(rate.Rate, 'f', -1, 64),
				rate.EffectiveAt.UTC(), source)
			if err != nil {
				return err
			}
			recorded += int(tag.RowsAffected())
		}
		if recorded == 0 {
			return nil
		}
		return audit.Record(ctx, tx, audit.Event{
			OrganizationID: organizationID, ActorKind: "system", Action: "fx.rates_recorded",
			ResourceType: "fx_rates", ResourceID: source,
			Metadata: map[string]any{"source": source, "recorded": recorded},
		})
	})
	if err != nil {
		return 0, err
	}
	return recorded, nil
}

// RefreshECB fetches and records the latest ECB snapshot.
func (s *Service) RefreshECB(ctx context.Context, organizationID string) (int, error) {
	if s.provider == nil {
		return 0, fmt.Errorf("FX provider is not configured")
	}
	rates, err := s.provider.DailyRates(ctx)
	if err != nil {
		return 0, err
	}
	return s.RecordRates(ctx, organizationID, "ecb", rates)
}
