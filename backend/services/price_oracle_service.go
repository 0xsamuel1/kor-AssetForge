package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	metrics "github.com/yourusername/kor-assetforge/metrics"
)

type PriceProvider string

const (
	ProviderChainlink PriceProvider = "chainlink"
	ProviderBand      PriceProvider = "band"
	ProviderCustom    PriceProvider = "custom"
)

// DefaultMaxPriceAge is the maximum age a price feed may have before it is
// rejected as stale. Five minutes matches the on-chain staleness threshold
// (Oracle::DEFAULT_STALENESS_THRESHOLD) so the backend never quotes a price the
// contracts would refuse to settle against.
const DefaultMaxPriceAge = 5 * time.Minute

// ErrStalePrice is returned when every configured provider returned data that
// is older than the accepted maximum age.
var ErrStalePrice = errors.New("oracle: price data is stale")

type PriceFeed struct {
	AssetID   uint    `json:"asset_id"`
	Symbol    string  `json:"symbol"`
	Price     float64 `json:"price"`
	Timestamp int64   `json:"timestamp"`
	Source    string  `json:"source"`
	Decimals  int     `json:"decimals"`
}

// Age reports how old the feed is relative to now.
func (f *PriceFeed) Age() time.Duration {
	if f == nil {
		return time.Duration(1<<63 - 1)
	}
	return time.Since(time.Unix(f.Timestamp, 0))
}

// IsStale reports whether the feed is older than maxAge.
func (f *PriceFeed) IsStale(maxAge time.Duration) bool {
	if f == nil {
		return true
	}
	// A timestamp in the future is never trusted either: it usually means a
	// misconfigured provider clock.
	if f.Timestamp > time.Now().Unix() {
		return true
	}
	return f.Age() > maxAge
}

type OraclePriceService struct {
	mu        sync.RWMutex
	feeds     map[uint]*PriceFeed
	providers map[string]PriceProvider
	client    *http.Client
	maxAge    time.Duration
}

func NewOraclePriceService() *OraclePriceService {
	return &OraclePriceService{
		feeds: make(map[uint]*PriceFeed),
		providers: map[string]PriceProvider{
			"chainlink": ProviderChainlink,
			"band":      ProviderBand,
		},
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		maxAge: configuredMaxAge(),
	}
}

// envOrDefault returns the value of key, or fallback when unset.
func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// configuredMaxAge reads ORACLE_MAX_PRICE_AGE (in seconds, e.g. "300s" or
// "300"). It falls back to DefaultMaxPriceAge when unset or malformed.
func configuredMaxAge() time.Duration {
	raw := os.Getenv("ORACLE_MAX_PRICE_AGE")
	if raw == "" {
		return DefaultMaxPriceAge
	}
	if d, err := time.ParseDuration(raw); err == nil && d > 0 {
		return d
	}
	if secs, err := strconv.Atoi(raw); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	return DefaultMaxPriceAge
}

// MaxPriceAge returns the staleness threshold enforced by this service.
func (s *OraclePriceService) MaxPriceAge() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.maxAge
}

// SetMaxPriceAge overrides the staleness threshold.
func (s *OraclePriceService) SetMaxPriceAge(d time.Duration) {
	if d <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.maxAge = d
}

// fallbackOrder returns the providers to try, primary first. The primary
// provider comes from PRICE_ORACLE_PROVIDER; the rest act as fallbacks so a
// single dead feed does not take pricing down.
func (s *OraclePriceService) fallbackOrder() []PriceProvider {
	primary := os.Getenv("PRICE_ORACLE_PROVIDER")
	if primary == "" {
		primary = string(ProviderChainlink)
	}

	order := []PriceProvider{PriceProvider(primary)}
	for _, candidate := range []PriceProvider{ProviderChainlink, ProviderBand, ProviderCustom} {
		if candidate != PriceProvider(primary) {
			order = append(order, candidate)
		}
	}
	return order
}

func (s *OraclePriceService) fetchFromProvider(provider PriceProvider, symbol string) (*PriceFeed, error) {
	switch provider {
	case ProviderChainlink:
		return s.fetchChainlink(symbol)
	case ProviderBand:
		return s.fetchBand(symbol)
	case ProviderCustom:
		return s.fetchCustom(symbol)
	default:
		return nil, fmt.Errorf("oracle: unknown provider %q", provider)
	}
}

// FetchPrice returns a fresh price from the primary provider. When the primary
// feed is unavailable or stale it falls back to the remaining providers. It
// returns ErrStalePrice when no provider returned usable data.
func (s *OraclePriceService) FetchPrice(symbol string) (*PriceFeed, error) {
	maxAge := s.MaxPriceAge()

	var lastErr error
	for _, provider := range s.fallbackOrder() {
		feed, err := s.fetchFromProvider(provider, symbol)
		if err != nil {
			lastErr = err
			metrics.OracleRequestsTotal.WithLabelValues(string(provider), "error").Inc()
			continue
		}

		if feed.Price <= 0 {
			lastErr = fmt.Errorf("oracle: %s returned a non-positive price for %s", provider, symbol)
			metrics.OracleRequestsTotal.WithLabelValues(string(provider), "invalid_price").Inc()
			continue
		}

		if feed.IsStale(maxAge) {
			lastErr = fmt.Errorf("oracle: %s price for %s is %s old (max %s): %w", provider, symbol, feed.Age(), maxAge, ErrStalePrice)
			metrics.OracleRequestsTotal.WithLabelValues(string(provider), "stale").Inc()
			metrics.OracleStalenessSeconds.WithLabelValues(symbol).Set(feed.Age().Seconds())
			s.reportStale(symbol, feed.Age())
			continue
		}

		metrics.OracleRequestsTotal.WithLabelValues(string(provider), "ok").Inc()
		metrics.OracleStalenessSeconds.WithLabelValues(symbol).Set(feed.Age().Seconds())
		return feed, nil
	}

	metrics.OracleStaleFeeds.WithLabelValues(symbol).Set(1)
	metrics.OracleRequestsTotal.WithLabelValues("all", "stale").Inc()

	if lastErr == nil {
		lastErr = ErrStalePrice
	}
	return nil, lastErr
}

// FetchPriceStrict returns a fresh price or fails. It is the entry point for
// anything that settles value (listings, purchases, valuations).
func (s *OraclePriceService) FetchPriceStrict(symbol string) (*PriceFeed, error) {
	feed, err := s.FetchPrice(symbol)
	if err != nil {
		metrics.OracleStaleFeeds.WithLabelValues(symbol).Set(1)
		return nil, err
	}
	metrics.OracleStaleFeeds.WithLabelValues(symbol).Set(0)
	return feed, nil
}

func (s *OraclePriceService) fetchChainlink(symbol string) (*PriceFeed, error) {
	base := envOrDefault("ORACLE_CHAINLINK_URL", "https://min-api.cryptocompare.com/data/price")
	url := fmt.Sprintf("%s?fsym=%s&tsyms=USD", base, symbol)
	resp, err := s.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("oracle: chainlink fetch failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("oracle: failed to read response: %w", err)
	}

	var result map[string]float64
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("oracle: failed to parse response: %w", err)
	}

	price, ok := result["USD"]
	if !ok {
		return nil, fmt.Errorf("oracle: no USD price for %s", symbol)
	}

	feed := &PriceFeed{
		Symbol:    symbol,
		Price:     price,
		Timestamp: time.Now().Unix(),
		Source:    "chainlink",
		Decimals:  8,
	}

	return feed, nil
}

func (s *OraclePriceService) fetchBand(symbol string) (*PriceFeed, error) {
	base := envOrDefault("ORACLE_BAND_URL", "https://api.bandprotocol.com/bandchain/price")
	url := fmt.Sprintf("%s?symbol=%s", base, symbol)
	resp, err := s.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("oracle: Band Protocol fetch failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("oracle: failed to read Band response: %w", err)
	}

	var result struct {
		Price     float64 `json:"price"`
		Timestamp int64   `json:"timestamp"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("oracle: failed to parse Band response: %w", err)
	}

	if result.Timestamp == 0 {
		result.Timestamp = time.Now().Unix()
	}

	return &PriceFeed{
		Symbol:    symbol,
		Price:     result.Price,
		Timestamp: result.Timestamp,
		Source:    "band",
		Decimals:  9,
	}, nil
}

func (s *OraclePriceService) fetchCustom(symbol string) (*PriceFeed, error) {
	customURL := os.Getenv("CUSTOM_PRICE_ORACLE_URL")
	if customURL == "" {
		return nil, fmt.Errorf("oracle: CUSTOM_PRICE_ORACLE_URL not set")
	}

	url := fmt.Sprintf("%s?symbol=%s", customURL, symbol)
	resp, err := s.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("oracle: custom fetch failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("oracle: failed to read custom response: %w", err)
	}

	var feed PriceFeed
	if err := json.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("oracle: failed to parse custom response: %w", err)
	}

	feed.Source = "custom"
	if feed.Symbol == "" {
		feed.Symbol = symbol
	}
	if feed.Timestamp == 0 {
		feed.Timestamp = time.Now().Unix()
	}
	return &feed, nil
}

func (s *OraclePriceService) UpdateFeed(assetID uint, feed *PriceFeed) {
	s.mu.Lock()
	defer s.mu.Unlock()
	feed.AssetID = assetID
	s.feeds[assetID] = feed
}

func (s *OraclePriceService) GetFeed(assetID uint) *PriceFeed {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.feeds[assetID]
}

// IsStale reports whether the cached feed for assetID is older than maxAge.
// A zero or negative maxAge uses the configured threshold.
func (s *OraclePriceService) IsStale(assetID uint, maxAge time.Duration) bool {
	if maxAge <= 0 {
		maxAge = s.MaxPriceAge()
	}
	feed := s.GetFeed(assetID)
	if feed == nil {
		return true
	}
	return feed.IsStale(maxAge)
}

// reportStale records a stale feed in metrics.
func (s *OraclePriceService) reportStale(symbol string, age time.Duration) {
	metrics.OracleStaleFeeds.WithLabelValues(symbol).Set(1)
	metrics.OracleStalenessSeconds.WithLabelValues(symbol).Set(age.Seconds())
}
