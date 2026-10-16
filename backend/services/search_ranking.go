package services

import (
	"context"
	"fmt"
	"hash/fnv"
	"sort"

	searchconfig "github.com/yourusername/kor-assetforge/config"
	"github.com/yourusername/kor-assetforge/models"
	"gorm.io/gorm"
)

const (
	RankingControl   = "control"
	RankingTreatment = "treatment"
)

// SearchRanker builds the scored portion of an Elasticsearch query. ES uses
// BM25 (term frequency and inverse document frequency) for each multi-match;
// this layer adds field weighting, history-based personalization, and stable
// A/B assignment.
type SearchRanker struct {
	db     *gorm.DB
	config searchconfig.SearchConfig
}

func NewSearchRanker(db *gorm.DB, cfg searchconfig.SearchConfig) *SearchRanker {
	return &SearchRanker{db: db, config: cfg}
}

// Variant deterministically assigns the same user (or anonymous query) to an
// experiment cohort, avoiding cross-request ranking changes.
func (r *SearchRanker) Variant(key string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	if int(h.Sum32()%100) < r.config.TreatmentTrafficPercent {
		return RankingTreatment
	}
	return RankingControl
}

func (r *SearchRanker) RankedQuery(ctx context.Context, req *SearchRequest) (map[string]interface{}, string) {
	key := req.Query
	if req.UserID != 0 {
		key = fmt.Sprintf("user:%d", req.UserID)
	}
	variant := r.Variant(key)
	fields := make([]string, 0, len(r.config.FieldBoosts))
	keys := make([]string, 0, len(r.config.FieldBoosts))
	for field := range r.config.FieldBoosts {
		keys = append(keys, field)
	}
	sort.Strings(keys)
	for _, field := range keys {
		boost := r.config.FieldBoosts[field]
		if variant == RankingTreatment && (field == "name" || field == "categories" || field == "tags") {
			boost *= 1.15
		}
		fields = append(fields, fmt.Sprintf("%s^%g", field, boost))
	}

	base := map[string]interface{}{
		"multi_match": map[string]interface{}{
			"query": req.Query, "fields": fields, "type": "best_fields",
			"fuzziness": "AUTO", "prefix_length": 1,
		},
	}
	functions := []map[string]interface{}{
		{"filter": map[string]interface{}{"term": map[string]interface{}{"verified": true}}, "weight": r.config.VerifiedBoost},
		{"field_value_factor": map[string]interface{}{"field": "popularity_score", "modifier": "log1p", "missing": 0, "factor": 0.1}},
	}
	if req.UserID != 0 {
		for _, assetType := range r.preferredAssetTypes(ctx, req.UserID) {
			functions = append(functions, map[string]interface{}{
				"filter": map[string]interface{}{"term": map[string]interface{}{"asset_type": assetType}},
				"weight": r.config.PersonalizationBoost,
			})
		}
	}
	return map[string]interface{}{
		"function_score": map[string]interface{}{
			"query": base, "functions": functions, "score_mode": "sum", "boost_mode": "multiply",
		},
	}, variant
}

func (r *SearchRanker) preferredAssetTypes(ctx context.Context, userID uint) []string {
	if r.db == nil {
		return nil
	}
	type preference struct{ AssetType string }
	var rows []preference
	err := r.db.WithContext(ctx).Model(&models.UserInteraction{}).
		Select("assets.asset_type").
		Joins("JOIN assets ON assets.id = user_interactions.asset_id").
		Where("user_interactions.user_id = ?", userID).
		Group("assets.asset_type").
		Order("SUM(user_interactions.weight) DESC").Limit(3).Scan(&rows).Error
	if err != nil {
		return nil
	}
	result := make([]string, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.AssetType)
	}
	return result
}
