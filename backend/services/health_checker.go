package services

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/yourusername/kor-assetforge/utils"
	"gorm.io/gorm"
)

type DependencyStatus string

const (
	DependencyStatusUp         DependencyStatus = "up"
	DependencyStatusDown       DependencyStatus = "down"
	DependencyStatusDegraded   DependencyStatus = "degraded"
	DependencyStatusNotEnabled DependencyStatus = "not_enabled"
)

type DependencyCheckResult struct {
	Name      string           `json:"name"`
	Status    DependencyStatus `json:"status"`
	Message   string           `json:"message,omitempty"`
	LatencyMS int64            `json:"latency_ms"`
	CheckedAt time.Time        `json:"checked_at"`
}

type OverallHealthStatus string

const (
	OverallHealthUp       OverallHealthStatus = "up"
	OverallHealthDegraded OverallHealthStatus = "degraded"
	OverallHealthDown     OverallHealthStatus = "down"
)

type HealthReport struct {
	Status       OverallHealthStatus              `json:"status"`
	GeneratedAt  time.Time                        `json:"generated_at"`
	Dependencies map[string]DependencyCheckResult `json:"dependencies"`
}

type HealthChecker struct {
	db               *gorm.DB
	redisClient      *redis.Client
	stellarClient    *utils.StellarClient
	elasticsearchURL string
	emailConfigured  bool
	httpClient       *http.Client
	breakers         map[string]*dependencyCircuitBreaker
	mu               sync.Mutex
}

type dependencyCircuitBreaker struct {
	failures int
	openedAt time.Time
}

func NewHealthChecker(db *gorm.DB, redisClient *redis.Client, stellarClient *utils.StellarClient) *HealthChecker {
	return &HealthChecker{
		db:               db,
		redisClient:      redisClient,
		stellarClient:    stellarClient,
		elasticsearchURL: os.Getenv("ELASTICSEARCH_URL"),
		emailConfigured:  emailProviderConfigured(),
		httpClient:       &http.Client{Timeout: 5 * time.Second},
		breakers:         make(map[string]*dependencyCircuitBreaker),
	}
}

func (h *HealthChecker) Check(ctx context.Context) HealthReport {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	checks := map[string]func(context.Context) DependencyCheckResult{
		"database":      h.checkDatabase,
		"redis":         h.checkRedis,
		"stellar":       h.checkStellar,
		"elasticsearch": h.checkElasticsearch,
		"email":         h.checkEmail,
	}

	results := make(map[string]DependencyCheckResult, len(checks))
	for name, check := range checks {
		if h.circuitOpen(name) {
			results[name] = DependencyCheckResult{
				Name:      name,
				Status:    DependencyStatusDegraded,
				Message:   "circuit breaker open after repeated failures",
				CheckedAt: time.Now().UTC(),
			}
			continue
		}
		result := check(ctx)
		h.recordCircuitState(name, result.Status)
		results[name] = result
	}

	status := OverallHealthUp
	for _, result := range results {
		switch result.Status {
		case DependencyStatusDown:
			if result.Name == "database" {
				status = OverallHealthDown
			} else if status != OverallHealthDown {
				status = OverallHealthDegraded
			}
		case DependencyStatusDegraded:
			if status == OverallHealthUp {
				status = OverallHealthDegraded
			}
		}
	}

	return HealthReport{
		Status:       status,
		GeneratedAt:  time.Now().UTC(),
		Dependencies: results,
	}
}

func (h *HealthChecker) checkDatabase(ctx context.Context) DependencyCheckResult {
	start := time.Now()
	sqlDB, err := h.db.DB()
	if err != nil {
		return failedCheck("database", start, err.Error())
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(pingCtx); err != nil && err != sql.ErrConnDone {
		return failedCheck("database", start, err.Error())
	}
	return okCheck("database", start, "database connection is ready")
}

func (h *HealthChecker) checkRedis(ctx context.Context) DependencyCheckResult {
	start := time.Now()
	if h.redisClient == nil {
		return skippedCheck("redis", start, "redis is not configured")
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := h.redisClient.Ping(pingCtx).Err(); err != nil {
		return degradedCheck("redis", start, err.Error())
	}
	return okCheck("redis", start, "redis ping succeeded")
}

func (h *HealthChecker) checkStellar(ctx context.Context) DependencyCheckResult {
	start := time.Now()
	if h.stellarClient == nil || h.stellarClient.HorizonClient == nil {
		return skippedCheck("stellar", start, "stellar horizon is not configured")
	}
	errCh := make(chan error, 1)
	go func() {
		_, err := h.stellarClient.HorizonClient.Root()
		errCh <- err
	}()
	select {
	case <-ctx.Done():
		return degradedCheck("stellar", start, ctx.Err().Error())
	case err := <-errCh:
		if err != nil {
			return degradedCheck("stellar", start, err.Error())
		}
		return okCheck("stellar", start, "stellar horizon is reachable")
	}
}

func (h *HealthChecker) checkElasticsearch(ctx context.Context) DependencyCheckResult {
	start := time.Now()
	if h.elasticsearchURL == "" {
		return skippedCheck("elasticsearch", start, "elasticsearch is not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.elasticsearchURL, nil)
	if err != nil {
		return degradedCheck("elasticsearch", start, err.Error())
	}
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return degradedCheck("elasticsearch", start, err.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return degradedCheck("elasticsearch", start, resp.Status)
	}
	return okCheck("elasticsearch", start, "elasticsearch endpoint responded")
}

func (h *HealthChecker) checkEmail(ctx context.Context) DependencyCheckResult {
	start := time.Now()
	select {
	case <-ctx.Done():
		return degradedCheck("email", start, ctx.Err().Error())
	default:
	}
	if !h.emailConfigured {
		return skippedCheck("email", start, "email provider is not configured")
	}
	return okCheck("email", start, "email provider configuration is present")
}

func (h *HealthChecker) circuitOpen(name string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	breaker := h.breakers[name]
	if breaker == nil || breaker.failures < 3 {
		return false
	}
	return time.Since(breaker.openedAt) < time.Minute
}

func (h *HealthChecker) recordCircuitState(name string, status DependencyStatus) {
	h.mu.Lock()
	defer h.mu.Unlock()
	breaker := h.breakers[name]
	if breaker == nil {
		breaker = &dependencyCircuitBreaker{}
		h.breakers[name] = breaker
	}
	if status == DependencyStatusDown || status == DependencyStatusDegraded {
		breaker.failures++
		if breaker.failures == 3 {
			breaker.openedAt = time.Now().UTC()
		}
		return
	}
	breaker.failures = 0
	breaker.openedAt = time.Time{}
}

func okCheck(name string, start time.Time, message string) DependencyCheckResult {
	return checkResult(name, DependencyStatusUp, message, start)
}

func failedCheck(name string, start time.Time, message string) DependencyCheckResult {
	return checkResult(name, DependencyStatusDown, message, start)
}

func degradedCheck(name string, start time.Time, message string) DependencyCheckResult {
	return checkResult(name, DependencyStatusDegraded, message, start)
}

func skippedCheck(name string, start time.Time, message string) DependencyCheckResult {
	return checkResult(name, DependencyStatusNotEnabled, message, start)
}

func checkResult(name string, status DependencyStatus, message string, start time.Time) DependencyCheckResult {
	return DependencyCheckResult{
		Name:      name,
		Status:    status,
		Message:   message,
		LatencyMS: time.Since(start).Milliseconds(),
		CheckedAt: time.Now().UTC(),
	}
}

func emailProviderConfigured() bool {
	if os.Getenv("SENDGRID_API_KEY") != "" {
		return true
	}
	return os.Getenv("SES_SMTP_USERNAME") != "" && os.Getenv("SES_SMTP_PASSWORD") != ""
}
