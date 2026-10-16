package services

import (
	"context"
	"errors"
	"log"
	"math"
	"time"

	"github.com/yourusername/kor-assetforge/models"
	"gorm.io/gorm"
)

// metricsRecalcInterval is how often the background job recomputes metrics.
const metricsRecalcInterval = 24 * time.Hour

// MetricsCalculatorService computes asset performance indicators (ROI,
// appreciation rate, dividend yield, volatility) from valuation history and
// dividend records, persisting snapshots and supporting benchmark comparison and
// a daily recalculation job (#169).
type MetricsCalculatorService struct {
	db *gorm.DB
}

// NewMetricsCalculatorService creates a MetricsCalculatorService.
func NewMetricsCalculatorService(db *gorm.DB) *MetricsCalculatorService {
	return &MetricsCalculatorService{db: db}
}

// Calculate computes performance metrics for an asset over [from, to]. The result
// is not persisted; use CalculateAndStore for that.
func (s *MetricsCalculatorService) Calculate(assetID uint, from, to time.Time) (*models.PerformanceMetric, error) {
	if !to.After(from) {
		return nil, errors.New("'to' must be after 'from'")
	}

	var history []models.ValuationHistory
	if err := s.db.Where("asset_id = ? AND recorded_at <= ?", assetID, to).
		Order("recorded_at ASC").Find(&history).Error; err != nil {
		return nil, err
	}

	initial := valuationAtOrBefore(history, from)
	current := latestValuationBefore(history, to)

	metric := &models.PerformanceMetric{
		AssetID:             assetID,
		PeriodStart:         from,
		PeriodEnd:           to,
		InitialValuationUSD: initial,
		CurrentValuationUSD: current,
		ComputedAt:          time.Now().UTC(),
	}

	years := to.Sub(from).Hours() / (24 * 365)
	if years <= 0 {
		years = 1.0 / 365
	}

	if initial > 0 {
		metric.ROI = (current - initial) / initial
		// Annualized appreciation: (current/initial)^(1/years) - 1.
		metric.AppreciationRate = math.Pow(current/initial, 1/years) - 1
	}

	metric.TotalDividendsUSD = s.totalDividends(assetID, from, to)
	if current > 0 {
		// Annualize the dividend income relative to current valuation.
		metric.DividendYield = (metric.TotalDividendsUSD / current) / years
	}
	metric.AnnualizedReturn = metric.AppreciationRate + metric.DividendYield
	metric.Volatility = volatility(periodReturns(history, from))

	metric.BenchmarkROI = s.benchmarkROI(from, to, assetID)
	metric.ExcessReturn = metric.ROI - metric.BenchmarkROI

	return metric, nil
}

// CalculateAndStore computes and persists a metrics snapshot for an asset.
func (s *MetricsCalculatorService) CalculateAndStore(assetID uint, from, to time.Time) (*models.PerformanceMetric, error) {
	metric, err := s.Calculate(assetID, from, to)
	if err != nil {
		return nil, err
	}
	if err := s.db.Create(metric).Error; err != nil {
		return nil, err
	}
	return metric, nil
}

// History returns stored performance snapshots for an asset, newest first.
func (s *MetricsCalculatorService) History(assetID uint, limit int) ([]models.PerformanceMetric, error) {
	if limit <= 0 || limit > 365 {
		limit = 90
	}
	var metrics []models.PerformanceMetric
	err := s.db.Where("asset_id = ?", assetID).
		Order("computed_at DESC").Limit(limit).Find(&metrics).Error
	return metrics, err
}

// RecordDividend stores a dividend distribution used for yield calculations.
func (s *MetricsCalculatorService) RecordDividend(assetID uint, amountUSD float64, currency, note string, paidAt time.Time) (*models.AssetDividend, error) {
	if amountUSD <= 0 {
		return nil, errors.New("dividend amount must be positive")
	}
	if currency == "" {
		currency = "USD"
	}
	if paidAt.IsZero() {
		paidAt = time.Now().UTC()
	}
	dividend := &models.AssetDividend{
		AssetID:   assetID,
		AmountUSD: amountUSD,
		Currency:  currency,
		Note:      note,
		PaidAt:    paidAt,
	}
	if err := s.db.Create(dividend).Error; err != nil {
		return nil, err
	}
	return dividend, nil
}

// RecalculateAll computes a trailing-12-month snapshot for every asset. It is the
// unit of work for the daily background job.
func (s *MetricsCalculatorService) RecalculateAll(ctx context.Context) (int, error) {
	var assetIDs []uint
	if err := s.db.Model(&models.Asset{}).Where("deleted_at IS NULL").Pluck("id", &assetIDs).Error; err != nil {
		return 0, err
	}
	to := time.Now().UTC()
	from := to.AddDate(-1, 0, 0)
	count := 0
	for _, id := range assetIDs {
		select {
		case <-ctx.Done():
			return count, ctx.Err()
		default:
		}
		if _, err := s.CalculateAndStore(id, from, to); err != nil {
			log.Printf("metrics: failed to compute for asset %d: %v", id, err)
			continue
		}
		count++
	}
	return count, nil
}

// Start launches the daily background recalculation job. It returns immediately;
// the job runs until ctx is cancelled.
func (s *MetricsCalculatorService) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(metricsRecalcInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if n, err := s.RecalculateAll(ctx); err != nil {
					log.Printf("metrics: daily recalculation aborted after %d assets: %v", n, err)
				} else {
					log.Printf("metrics: daily recalculation completed for %d assets", n)
				}
			}
		}
	}()
}

func (s *MetricsCalculatorService) totalDividends(assetID uint, from, to time.Time) float64 {
	var total float64
	s.db.Model(&models.AssetDividend{}).
		Where("asset_id = ? AND paid_at >= ? AND paid_at <= ?", assetID, from, to).
		Select("COALESCE(SUM(amount_usd), 0)").Scan(&total)
	return total
}

// benchmarkROI computes the average ROI across all other assets over the period,
// providing a platform benchmark to compare an individual asset against.
func (s *MetricsCalculatorService) benchmarkROI(from, to time.Time, excludeAssetID uint) float64 {
	var assetIDs []uint
	s.db.Model(&models.Asset{}).Where("deleted_at IS NULL AND id <> ?", excludeAssetID).Pluck("id", &assetIDs)
	var sum float64
	var n int
	for _, id := range assetIDs {
		var history []models.ValuationHistory
		s.db.Where("asset_id = ? AND recorded_at <= ?", id, to).Order("recorded_at ASC").Find(&history)
		initial := valuationAtOrBefore(history, from)
		current := latestValuationBefore(history, to)
		if initial > 0 {
			sum += (current - initial) / initial
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

// valuationAtOrBefore returns the most recent valuation recorded at or before t,
// or the earliest available valuation if none precede t.
func valuationAtOrBefore(history []models.ValuationHistory, t time.Time) float64 {
	if len(history) == 0 {
		return 0
	}
	val := history[0].ValuationUSD
	for _, h := range history {
		if h.RecordedAt.After(t) {
			break
		}
		val = h.ValuationUSD
	}
	return val
}

// latestValuationBefore returns the most recent valuation recorded at or before t.
func latestValuationBefore(history []models.ValuationHistory, t time.Time) float64 {
	var val float64
	for _, h := range history {
		if h.RecordedAt.After(t) {
			break
		}
		val = h.ValuationUSD
	}
	if val == 0 && len(history) > 0 {
		val = history[len(history)-1].ValuationUSD
	}
	return val
}

// periodReturns returns the sequence of point-over-point returns from valuations
// recorded on or after `from`.
func periodReturns(history []models.ValuationHistory, from time.Time) []float64 {
	var returns []float64
	var prev float64
	for _, h := range history {
		if h.RecordedAt.Before(from) {
			prev = h.ValuationUSD
			continue
		}
		if prev > 0 {
			returns = append(returns, (h.ValuationUSD-prev)/prev)
		}
		prev = h.ValuationUSD
	}
	return returns
}

// volatility returns the population standard deviation of the returns.
func volatility(returns []float64) float64 {
	if len(returns) < 2 {
		return 0
	}
	var mean float64
	for _, r := range returns {
		mean += r
	}
	mean /= float64(len(returns))
	var variance float64
	for _, r := range returns {
		variance += (r - mean) * (r - mean)
	}
	variance /= float64(len(returns))
	return math.Sqrt(variance)
}
