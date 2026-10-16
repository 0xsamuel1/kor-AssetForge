package services

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func newTestFeed(price float64, age time.Duration) *PriceFeed {
	return &PriceFeed{
		Symbol:    "TEST",
		Price:     price,
		Timestamp: time.Now().Add(-age).Unix(),
		Source:    "test",
		Decimals:  8,
	}
}

func TestConfiguredMaxAgeDefault(t *testing.T) {
	t.Setenv("ORACLE_MAX_PRICE_AGE", "")

	if got := configuredMaxAge(); got != DefaultMaxPriceAge {
		t.Fatalf("expected default max age of %s, got %s", DefaultMaxPriceAge, got)
	}
	if DefaultMaxPriceAge != 5*time.Minute {
		t.Fatalf("default max price age must be five minutes, got %s", DefaultMaxPriceAge)
	}
}

func TestConfiguredMaxAgeFromEnv(t *testing.T) {
	t.Setenv("ORACLE_MAX_PRICE_AGE", "90s")
	if got := configuredMaxAge(); got != 90*time.Second {
		t.Fatalf("expected 90s, got %s", got)
	}

	t.Setenv("ORACLE_MAX_PRICE_AGE", "45")
	if got := configuredMaxAge(); got != 45*time.Second {
		t.Fatalf("expected 45s from bare seconds, got %s", got)
	}
}

func TestConfiguredMaxAgeRejectsInvalidValues(t *testing.T) {
	for _, value := range []string{"0", "-10s", "not-a-duration"} {
		t.Setenv("ORACLE_MAX_PRICE_AGE", value)
		if got := configuredMaxAge(); got != DefaultMaxPriceAge {
			t.Fatalf("expected fallback for %q, got %s", value, got)
		}
	}
}

func TestFeedIsStale(t *testing.T) {
	maxAge := 5 * time.Minute

	if fresh := newTestFeed(100, time.Minute); fresh.IsStale(maxAge) {
		t.Fatal("a one minute old price must not be stale")
	}

	if boundary := newTestFeed(100, 5*time.Minute-time.Second); boundary.IsStale(maxAge) {
		t.Fatal("a price just inside the threshold must not be stale")
	}

	if old := newTestFeed(100, 5*time.Minute+time.Second); !old.IsStale(maxAge) {
		t.Fatal("a price older than the threshold must be stale")
	}

	var missing *PriceFeed
	if !missing.IsStale(maxAge) {
		t.Fatal("a missing feed must be stale")
	}

	future := newTestFeed(100, -time.Hour)
	if !future.IsStale(maxAge) {
		t.Fatal("a feed timestamped in the future must be treated as stale")
	}
}

func TestIsStaleUsesConfiguredThreshold(t *testing.T) {
	svc := NewOraclePriceService()
	svc.UpdateFeed(7, newTestFeed(100, 10*time.Minute))

	if !svc.IsStale(7, 0) {
		t.Fatal("feed older than the default five minutes must be stale")
	}
	if svc.IsStale(7, 30*time.Minute) {
		t.Fatal("feed must be fresh when the caller allows a longer window")
	}
	if !svc.IsStale(99, 0) {
		t.Fatal("an unknown asset must be considered stale")
	}
}

func TestSetMaxPriceAge(t *testing.T) {
	svc := NewOraclePriceService()
	svc.SetMaxPriceAge(30 * time.Second)
	if svc.MaxPriceAge() != 30*time.Second {
		t.Fatalf("expected 30s, got %s", svc.MaxPriceAge())
	}

	svc.SetMaxPriceAge(0)
	if svc.MaxPriceAge() != 30*time.Second {
		t.Fatal("a non-positive override must be ignored")
	}
}

func TestFetchPriceRejectsStaleProvider(t *testing.T) {
	stale := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"price":42,"timestamp":` + strconv.FormatInt(time.Now().Add(-time.Hour).Unix(), 10) + `}`))
	}))
	defer stale.Close()

	fresh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"price":100,"timestamp":` + strconv.FormatInt(time.Now().Unix(), 10) + `}`))
	}))
	defer fresh.Close()

	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	dead.Close()

	t.Setenv("PRICE_ORACLE_PROVIDER", "custom")
	t.Setenv("CUSTOM_PRICE_ORACLE_URL", stale.URL)
	t.Setenv("ORACLE_BAND_URL", fresh.URL)
	t.Setenv("ORACLE_CHAINLINK_URL", dead.URL)

	svc := NewOraclePriceService()
	feed, err := svc.FetchPrice("TEST")
	if err != nil {
		t.Fatalf("expected fallback to a fresh provider, got %v", err)
	}
	if feed.Source != "band" {
		t.Fatalf("expected the band fallback to be used, got %q", feed.Source)
	}
	if feed.Price != 100 {
		t.Fatalf("expected the fallback price 100, got %v", feed.Price)
	}
}

func TestFetchPriceFailsWhenEverythingIsStale(t *testing.T) {
	staleTimestamp := strconv.FormatInt(time.Now().Add(-time.Hour).Unix(), 10)

	custom := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"price":42,"timestamp":` + staleTimestamp + `}`))
	}))
	defer custom.Close()

	band := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"price":42,"timestamp":` + staleTimestamp + `}`))
	}))
	defer band.Close()

	// Chainlink always timestamps "now", so point it at a dead endpoint to force
	// an error instead of a network call.
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	dead.Close()

	t.Setenv("PRICE_ORACLE_PROVIDER", "custom")
	t.Setenv("CUSTOM_PRICE_ORACLE_URL", custom.URL)
	t.Setenv("ORACLE_BAND_URL", band.URL)
	t.Setenv("ORACLE_CHAINLINK_URL", dead.URL)

	svc := NewOraclePriceService()
	if _, err := svc.FetchPrice("TEST"); !errors.Is(err, ErrStalePrice) {
		t.Fatalf("expected ErrStalePrice, got %v", err)
	}
	if _, err := svc.FetchPriceStrict("TEST"); !errors.Is(err, ErrStalePrice) {
		t.Fatalf("FetchPriceStrict must fail on stale data, got %v", err)
	}
}

func TestFetchPriceRejectsNonPositivePrice(t *testing.T) {
	zero := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"price":0,"timestamp":` + strconv.FormatInt(time.Now().Unix(), 10) + `}`))
	}))
	defer zero.Close()

	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	dead.Close()

	band := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"price":0}`))
	}))
	defer band.Close()

	t.Setenv("PRICE_ORACLE_PROVIDER", "custom")
	t.Setenv("CUSTOM_PRICE_ORACLE_URL", zero.URL)
	t.Setenv("ORACLE_BAND_URL", band.URL)
	t.Setenv("ORACLE_CHAINLINK_URL", dead.URL)

	svc := NewOraclePriceService()
	if _, err := svc.FetchPrice("TEST"); err == nil {
		t.Fatal("a zero price must be rejected")
	}
}

func TestFetchPriceAcceptsFreshProvider(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"USD":1234.5}`))
	}))
	defer primary.Close()

	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	dead.Close()

	t.Setenv("PRICE_ORACLE_PROVIDER", "chainlink")
	t.Setenv("ORACLE_CHAINLINK_URL", primary.URL)
	t.Setenv("ORACLE_BAND_URL", dead.URL)

	svc := NewOraclePriceService()
	feed, err := svc.FetchPriceStrict("TEST")
	if err != nil {
		t.Fatalf("expected a fresh price, got %v", err)
	}
	if feed.Price != 1234.5 {
		t.Fatalf("expected 1234.5, got %v", feed.Price)
	}
	if feed.Source != "chainlink" {
		t.Fatalf("expected the primary provider, got %q", feed.Source)
	}
	if feed.Age() > time.Minute {
		t.Fatalf("a fresh feed should not report a large age, got %s", feed.Age())
	}
}

func TestFallbackOrderStartsWithConfiguredProvider(t *testing.T) {
	t.Setenv("PRICE_ORACLE_PROVIDER", "band")

	svc := NewOraclePriceService()
	order := svc.fallbackOrder()
	if len(order) == 0 {
		t.Fatal("expected at least one provider")
	}
	if order[0] != ProviderBand {
		t.Fatalf("expected band to be primary, got %q", order[0])
	}

	seen := make(map[PriceProvider]bool, len(order))
	for _, p := range order {
		if seen[p] {
			t.Fatalf("provider %q appears twice", p)
		}
		seen[p] = true
	}
}
