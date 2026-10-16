package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yourusername/kor-assetforge/models"
	"gorm.io/gorm"
)

const (
	minComparisonAssets = 2
	maxComparisonAssets = 10
	comparisonCacheTTL  = 5 * time.Minute
)

// knownComparisonCriteria enumerates the metrics that can be compared. Requests
// may restrict the comparison to a subset via custom criteria.
var knownComparisonCriteria = map[string]bool{
	"valuation":     true,
	"total_supply":  true,
	"fractions":     true,
	"asset_type":    true,
	"verified":      true,
	"active_listings": true,
	"transaction_count": true,
}

// AssetComparison holds the per-asset values gathered for a comparison.
type AssetComparison struct {
	AssetID      uint               `json:"asset_id"`
	Name         string             `json:"name"`
	Symbol       string             `json:"symbol"`
	AssetType    string             `json:"asset_type"`
	Verified     bool               `json:"verified"`
	Metrics      map[string]float64 `json:"metrics"`
	Attributes   map[string]string  `json:"attributes"`
}

// CriterionSummary describes the spread of one numeric criterion across the
// compared assets, including which asset leads.
type CriterionSummary struct {
	Criterion   string  `json:"criterion"`
	Min         float64 `json:"min"`
	Max         float64 `json:"max"`
	Average     float64 `json:"average"`
	BestAssetID uint    `json:"best_asset_id"`
}

// ComparisonResult is the full side-by-side comparison payload.
type ComparisonResult struct {
	Assets      []AssetComparison           `json:"assets"`
	Summary     map[string]CriterionSummary `json:"summary"`
	Criteria    []string                    `json:"criteria"`
	GeneratedAt time.Time                   `json:"generated_at"`
	Cached      bool                        `json:"cached"`
}

type cachedComparison struct {
	result    *ComparisonResult
	expiresAt time.Time
}

// ComparisonService compares key metrics and attributes of multiple assets,
// with in-memory result caching and persistent comparison history (#167).
type ComparisonService struct {
	db    *gorm.DB
	mu    sync.Mutex
	cache map[string]cachedComparison
}

// NewComparisonService creates a ComparisonService.
func NewComparisonService(db *gorm.DB) *ComparisonService {
	return &ComparisonService{
		db:    db,
		cache: make(map[string]cachedComparison),
	}
}

// Compare builds a side-by-side comparison of 2-10 assets. criteria optionally
// restricts which metrics are computed; an empty list compares all known
// criteria. Results are cached for a short TTL keyed on the asset set + criteria.
func (s *ComparisonService) Compare(assetIDs []uint, criteria []string) (*ComparisonResult, error) {
	normalizedIDs, err := normalizeAssetIDs(assetIDs)
	if err != nil {
		return nil, err
	}
	activeCriteria, err := resolveCriteria(criteria)
	if err != nil {
		return nil, err
	}

	cacheKey := comparisonCacheKey(normalizedIDs, activeCriteria)
	if cached := s.fromCache(cacheKey); cached != nil {
		return cached, nil
	}

	var assets []models.Asset
	if err := s.db.Where("id IN ?", normalizedIDs).Find(&assets).Error; err != nil {
		return nil, fmt.Errorf("failed to load assets: %w", err)
	}
	if len(assets) != len(normalizedIDs) {
		return nil, errors.New("one or more assets were not found")
	}

	criterionSet := make(map[string]bool, len(activeCriteria))
	for _, cr := range activeCriteria {
		criterionSet[cr] = true
	}

	result := &ComparisonResult{
		Criteria:    activeCriteria,
		GeneratedAt: time.Now().UTC(),
		Summary:     make(map[string]CriterionSummary),
	}
	for i := range assets {
		result.Assets = append(result.Assets, s.buildAssetComparison(&assets[i], criterionSet))
	}
	result.Summary = summarize(result.Assets, activeCriteria)

	s.store(cacheKey, result)
	return result, nil
}

// buildAssetComparison gathers the requested metrics and attributes for a single
// asset.
func (s *ComparisonService) buildAssetComparison(asset *models.Asset, criteria map[string]bool) AssetComparison {
	ac := AssetComparison{
		AssetID:    asset.ID,
		Name:       asset.Name,
		Symbol:     asset.Symbol,
		AssetType:  asset.AssetType,
		Verified:   asset.Verified,
		Metrics:    make(map[string]float64),
		Attributes: make(map[string]string),
	}

	if criteria["asset_type"] {
		ac.Attributes["asset_type"] = asset.AssetType
	}
	if criteria["verified"] {
		ac.Metrics["verified"] = boolToFloat(asset.Verified)
	}
	if criteria["total_supply"] {
		ac.Metrics["total_supply"] = float64(asset.TotalSupply)
	}
	if criteria["fractions"] {
		ac.Metrics["fractions"] = float64(asset.Fractions)
	}
	if criteria["valuation"] {
		ac.Metrics["valuation"] = s.latestValuation(asset.ID)
	}
	if criteria["active_listings"] {
		ac.Metrics["active_listings"] = float64(s.countActiveListings(asset.ID))
	}
	if criteria["transaction_count"] {
		ac.Metrics["transaction_count"] = float64(s.countTransactions(asset.ID))
	}
	return ac
}

func (s *ComparisonService) latestValuation(assetID uint) float64 {
	var v models.ValuationHistory
	if err := s.db.Where("asset_id = ?", assetID).Order("recorded_at DESC").First(&v).Error; err != nil {
		return 0
	}
	return v.ValuationUSD
}

func (s *ComparisonService) countActiveListings(assetID uint) int64 {
	var count int64
	s.db.Model(&models.Listing{}).Where("asset_id = ? AND active = ? AND deleted_at IS NULL", assetID, true).Count(&count)
	return count
}

func (s *ComparisonService) countTransactions(assetID uint) int64 {
	var count int64
	s.db.Model(&models.Transaction{}).Where("asset_id = ?", assetID).Count(&count)
	return count
}

// SaveHistory persists a comparison to the user's history.
func (s *ComparisonService) SaveHistory(userID uint, assetIDs []uint, criteria []string) (*models.ComparisonHistory, error) {
	idsJSON, _ := json.Marshal(assetIDs)
	critJSON, _ := json.Marshal(criteria)
	record := &models.ComparisonHistory{
		UserID:      userID,
		AssetIDs:    string(idsJSON),
		Criteria:    string(critJSON),
		AssetsCount: len(assetIDs),
	}
	if err := s.db.Create(record).Error; err != nil {
		return nil, err
	}
	return record, nil
}

// ListHistory returns a user's saved comparisons, most recent first.
func (s *ComparisonService) ListHistory(userID uint, limit int) ([]models.ComparisonHistory, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var records []models.ComparisonHistory
	err := s.db.Where("user_id = ?", userID).Order("created_at DESC").Limit(limit).Find(&records).Error
	return records, err
}

// GetHistory loads a single history entry (scoped to its owner) and re-runs the
// comparison so the caller gets fresh data.
func (s *ComparisonService) GetHistory(userID, historyID uint) (*models.ComparisonHistory, *ComparisonResult, error) {
	var record models.ComparisonHistory
	if err := s.db.Where("id = ? AND user_id = ?", historyID, userID).First(&record).Error; err != nil {
		return nil, nil, err
	}
	var assetIDs []uint
	var criteria []string
	_ = json.Unmarshal([]byte(record.AssetIDs), &assetIDs)
	_ = json.Unmarshal([]byte(record.Criteria), &criteria)
	result, err := s.Compare(assetIDs, criteria)
	if err != nil {
		return &record, nil, err
	}
	return &record, result, nil
}

func (s *ComparisonService) fromCache(key string) *ComparisonResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.cache[key]
	if !ok || time.Now().After(entry.expiresAt) {
		if ok {
			delete(s.cache, key)
		}
		return nil
	}
	clone := *entry.result
	clone.Cached = true
	return &clone
}

func (s *ComparisonService) store(key string, result *ComparisonResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache[key] = cachedComparison{result: result, expiresAt: time.Now().Add(comparisonCacheTTL)}
}

// normalizeAssetIDs validates the count and de-duplicates while preserving order.
func normalizeAssetIDs(assetIDs []uint) ([]uint, error) {
	seen := make(map[uint]bool)
	var unique []uint
	for _, id := range assetIDs {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		unique = append(unique, id)
	}
	if len(unique) < minComparisonAssets {
		return nil, fmt.Errorf("at least %d distinct assets are required for comparison", minComparisonAssets)
	}
	if len(unique) > maxComparisonAssets {
		return nil, fmt.Errorf("at most %d assets can be compared at once", maxComparisonAssets)
	}
	return unique, nil
}

// resolveCriteria validates requested criteria, defaulting to all known criteria.
func resolveCriteria(criteria []string) ([]string, error) {
	if len(criteria) == 0 {
		all := make([]string, 0, len(knownComparisonCriteria))
		for cr := range knownComparisonCriteria {
			all = append(all, cr)
		}
		sort.Strings(all)
		return all, nil
	}
	seen := make(map[string]bool)
	var resolved []string
	for _, cr := range criteria {
		cr = strings.ToLower(strings.TrimSpace(cr))
		if cr == "" || seen[cr] {
			continue
		}
		if !knownComparisonCriteria[cr] {
			return nil, fmt.Errorf("unknown comparison criterion: %s", cr)
		}
		seen[cr] = true
		resolved = append(resolved, cr)
	}
	sort.Strings(resolved)
	return resolved, nil
}

// summarize computes min/max/average and the leading asset for each numeric
// criterion across the compared assets.
func summarize(assets []AssetComparison, criteria []string) map[string]CriterionSummary {
	summary := make(map[string]CriterionSummary)
	for _, cr := range criteria {
		var (
			values   []float64
			best     float64
			bestID   uint
			haveData bool
			sum      float64
		)
		for _, a := range assets {
			val, ok := a.Metrics[cr]
			if !ok {
				continue
			}
			values = append(values, val)
			sum += val
			if !haveData || val > best {
				best = val
				bestID = a.AssetID
				haveData = true
			}
		}
		if !haveData {
			continue
		}
		min, max := values[0], values[0]
		for _, v := range values {
			if v < min {
				min = v
			}
			if v > max {
				max = v
			}
		}
		summary[cr] = CriterionSummary{
			Criterion:   cr,
			Min:         min,
			Max:         max,
			Average:     sum / float64(len(values)),
			BestAssetID: bestID,
		}
	}
	return summary
}

func comparisonCacheKey(assetIDs []uint, criteria []string) string {
	sorted := make([]uint, len(assetIDs))
	copy(sorted, assetIDs)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	parts := make([]string, len(sorted))
	for i, id := range sorted {
		parts[i] = fmt.Sprintf("%d", id)
	}
	return strings.Join(parts, ",") + "|" + strings.Join(criteria, ",")
}

func boolToFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
