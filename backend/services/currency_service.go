// Package services provides core business logic for the kor-AssetForge platform.
//
// # Multi-Currency Support
//
// CurrencyService handles foreign exchange (FX) rate retrieval and amount conversion
// between the platform's supported fiat and cryptocurrency denominations.
//
// Supported currencies: USD, EUR, GBP, XLM (Stellar Lumens), BTC, ETH.
//
// Exchange rates are sourced from open.er-api.com (configurable via EXCHANGE_RATE_API_URL),
// cached in Redis for 15 minutes to reduce upstream API calls, and fall back to
// hardcoded snapshot rates when both Redis and the upstream API are unavailable.
//
// All conversions use USD as the pivot currency:
//
//	converted = (amount / rateFrom) * rateTo
//
// See docs/MULTI_CURRENCY_SUPPORT.md for full documentation.
package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// currencyCachePrefix is the Redis key namespace for FX rate entries.
	currencyCachePrefix = "kor:fx:"

	// currencyCacheTTL controls how long exchange rates are held in Redis before
	// a fresh fetch is attempted. 15 minutes balances freshness with API quota usage.
	currencyCacheTTL = 15 * time.Minute
)

// SupportedCurrencies lists every fiat and crypto denomination the platform accepts.
// When adding a new currency: append the code here, add a fallback rate in
// fallbackRates(), and update docs/MULTI_CURRENCY_SUPPORT.md.
var SupportedCurrencies = []string{"USD", "EUR", "GBP", "XLM", "BTC", "ETH"}

// ExchangeRates holds conversion rates relative to USD returned by GetRates.
// All rate values express how many units of the named currency equal 1 USD.
type ExchangeRates struct {
	// Base is always "USD" — all rates are expressed relative to USD.
	Base string `json:"base"`
	// Rates maps currency code → units-per-USD.
	Rates     map[string]float64 `json:"rates"`
	FetchedAt time.Time          `json:"fetched_at"`
}

// CurrencyService converts amounts between supported currencies.
//
// It pulls live rates from a configurable exchange-rate API, caches them in
// Redis, and degrades gracefully to hardcoded fallback rates when external
// dependencies are unavailable.
type CurrencyService struct {
	httpClient *http.Client
	redis      *redis.Client
	// apiBaseURL is the base URL of the exchange-rate API.
	// The service appends "/USD" to fetch USD-based rates.
	// Example: https://open.er-api.com/v6/latest
	// Configure via the EXCHANGE_RATE_API_URL environment variable.
	apiBaseURL string
}

// NewCurrencyService creates a CurrencyService wired to the given Redis client
// and exchange-rate API base URL (e.g. "https://open.er-api.com/v6/latest").
// Pass nil for rdb to skip caching (useful in tests or environments without Redis).
func NewCurrencyService(rdb *redis.Client, apiBaseURL string) *CurrencyService {
	return &CurrencyService{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		redis:      rdb,
		apiBaseURL: apiBaseURL,
	}
}

// GetRates returns exchange rates relative to USD.
//
// Resolution order:
//  1. Redis cache (key: kor:fx:rates, TTL 15 min) — fastest path, shared across instances
//  2. Live upstream API (EXCHANGE_RATE_API_URL/USD)
//  3. Hardcoded fallback snapshot — used when both Redis and the API are unavailable
//
// The function never returns an error; degraded fallback rates are returned instead
// so callers can continue operating even during external outages.
func (s *CurrencyService) GetRates(ctx context.Context) (*ExchangeRates, error) {
	cacheKey := currencyCachePrefix + "rates"

	if s.redis != nil {
		if raw, err := s.redis.Get(ctx, cacheKey).Bytes(); err == nil {
			var rates ExchangeRates
			if json.Unmarshal(raw, &rates) == nil {
				return &rates, nil
			}
		}
	}

	rates, err := s.fetchFromAPI(ctx)
	if err != nil {
		// Return a static fallback so the service degrades gracefully.
		return s.fallbackRates(), nil
	}

	if s.redis != nil {
		if data, err := json.Marshal(rates); err == nil {
			s.redis.Set(ctx, cacheKey, data, currencyCacheTTL)
		}
	}

	return rates, nil
}

// Convert converts amount from the from currency to the to currency.
//
// Both currency codes must appear in SupportedCurrencies; an error is returned
// for unknown codes. The conversion uses USD as an intermediate pivot:
//
//	result = (amount / rateFrom) * rateTo
//
// Example: converting 100 EUR → GBP
//
//	rateEUR=0.92, rateGBP=0.79
//	100/0.92 = 108.70 USD → 108.70*0.79 = 85.87 GBP
func (s *CurrencyService) Convert(ctx context.Context, amount float64, from, to string) (float64, error) {
	if from == to {
		return amount, nil
	}

	rates, err := s.GetRates(ctx)
	if err != nil {
		return 0, fmt.Errorf("currency service unavailable: %w", err)
	}

	fromRate, ok := rates.Rates[from]
	if !ok {
		return 0, fmt.Errorf("unsupported currency: %s", from)
	}
	toRate, ok := rates.Rates[to]
	if !ok {
		return 0, fmt.Errorf("unsupported currency: %s", to)
	}

	// Convert via USD as the common denominator.
	amountInUSD := amount / fromRate
	return amountInUSD * toRate, nil
}

// IsSupportedCurrency reports whether code is in SupportedCurrencies.
// Use this to validate user-supplied currency parameters before calling Convert.
func IsSupportedCurrency(code string) bool {
	for _, c := range SupportedCurrencies {
		if c == code {
			return true
		}
	}
	return false
}

// fetchFromAPI fetches live USD-based exchange rates from the configured API.
// The response must match the Open Exchange Rates v6 JSON shape:
//
//	{ "base_code": "USD", "rates": { "EUR": 0.92, ... } }
func (s *CurrencyService) fetchFromAPI(ctx context.Context) (*ExchangeRates, error) {
	url := s.apiBaseURL + "/USD"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("exchange rate API returned status %d", resp.StatusCode)
	}

	var payload struct {
		BaseCode string             `json:"base_code"`
		Rates    map[string]float64 `json:"rates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	return &ExchangeRates{
		Base:      payload.BaseCode,
		Rates:     payload.Rates,
		FetchedAt: time.Now().UTC(),
	}, nil
}

// fallbackRates returns a hardcoded snapshot of approximate exchange rates.
// These are used only when both the Redis cache and the upstream API are unavailable.
// Values should be kept roughly current but are not guaranteed to be accurate.
// See docs/MULTI_CURRENCY_SUPPORT.md for the fallback rate table.
func (s *CurrencyService) fallbackRates() *ExchangeRates {
	return &ExchangeRates{
		Base: "USD",
		Rates: map[string]float64{
			"USD": 1.0,
			"EUR": 0.92,
			"GBP": 0.79,
			"XLM": 8.93,  // approximate; updated by live API when available
			"BTC": 0.000016,
			"ETH": 0.00031,
		},
		FetchedAt: time.Now().UTC(),
	}
}
