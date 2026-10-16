package services

import (
	"encoding/json"
	"log"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/yourusername/kor-assetforge/models"
	"gorm.io/gorm"
)

type RecommendationService struct {
	db *gorm.DB
}

func NewRecommendationService(db *gorm.DB) *RecommendationService {
	return &RecommendationService{db: db}
}

func (s *RecommendationService) RecordInteraction(userID, assetID uint, interactionType models.InteractionType, metadata map[string]interface{}) error {
	weight := 1.0
	switch interactionType {
	case models.InteractionPurchase:
		weight = 5.0
	case models.InteractionList:
		weight = 3.0
	case models.InteractionView:
		weight = 1.0
	case models.InteractionSearch:
		weight = 0.5
	}

	metaJSON, _ := json.Marshal(metadata)
	interaction := models.UserInteraction{
		UserID:          userID,
		AssetID:         assetID,
		InteractionType: interactionType,
		Weight:          weight,
		Metadata:        string(metaJSON),
	}

	return s.db.Create(&interaction).Error
}

func (s *RecommendationService) GetRecommendations(userID uint, limit int) ([]models.AssetRecommendation, error) {
	var existing []models.AssetRecommendation
	s.db.Where("user_id = ? AND expires_at > ? AND is_viewed = ?", userID, time.Now(), false).
		Preload("Asset").Order("score DESC").Limit(limit).Find(&existing)

	if len(existing) >= limit {
		return existing[:limit], nil
	}

	recommendations, err := s.computeRecommendations(userID, limit)
	if err != nil {
		return nil, err
	}

	if len(recommendations) == 0 {
		recommendations = s.fallbackPopular(limit)
	}

	for i := range recommendations {
		recommendations[i].UserID = userID
		recommendations[i].ExpiresAt = time.Now().Add(24 * time.Hour)
		s.db.Where("user_id = ? AND asset_id = ?", userID, recommendations[i].AssetID).
			Assign(&recommendations[i]).
			FirstOrCreate(&models.AssetRecommendation{})
	}

	return recommendations, nil
}

func (s *RecommendationService) computeRecommendations(userID uint, limit int) ([]models.AssetRecommendation, error) {
	similarUsers := s.findSimilarUsers(userID, 10)
	if len(similarUsers) == 0 {
		return nil, nil
	}

	userAssetIDs := s.getUserAssetIDs(userID)
	excludeMap := make(map[uint]bool)
	for _, id := range userAssetIDs {
		excludeMap[id] = true
	}

	assetScores := make(map[uint]float64)
	for _, similarUserID := range similarUsers {
		var interactions []models.UserInteraction
		s.db.Where("user_id = ?", similarUserID).Find(&interactions)

		for _, interaction := range interactions {
			if excludeMap[interaction.AssetID] {
				continue
			}
			assetScores[interaction.AssetID] += interaction.Weight
		}
	}

	if len(assetScores) == 0 {
		return nil, nil
	}

	type scoredAsset struct {
		AssetID uint
		Score   float64
	}
	var scored []scoredAsset
	for assetID, score := range assetScores {
		scored = append(scored, scoredAsset{AssetID: assetID, Score: score})
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})

	if len(scored) > limit {
		scored = scored[:limit]
	}

	var recommendations []models.AssetRecommendation
	for _, sa := range scored {
		var asset models.Asset
		if err := s.db.First(&asset, sa.AssetID).Error; err != nil {
			continue
		}
		recommendations = append(recommendations, models.AssetRecommendation{
			AssetID: sa.AssetID,
			Asset:   asset,
			Score:   sa.Score,
			Reason:  "Based on users with similar interests",
		})
	}

	return recommendations, nil
}

func (s *RecommendationService) findSimilarUsers(userID uint, maxUsers int) []uint {
	var userInteractions []models.UserInteraction
	s.db.Where("user_id = ?", userID).Find(&userInteractions)

	if len(userInteractions) == 0 {
		return nil
	}

	userAssetSet := make(map[uint]bool)
	for _, ui := range userInteractions {
		userAssetSet[ui.AssetID] = true
	}

	type userSimilarity struct {
		UserID     uint
		CommonCount int
	}

	var searchUserIDs []uint
	for _, ui := range userInteractions {
		var otherInteractions []models.UserInteraction
		s.db.Where("asset_id = ? AND user_id != ?", ui.AssetID, userID).
			Select("DISTINCT user_id").Find(&otherInteractions)
		for _, oi := range otherInteractions {
			searchUserIDs = append(searchUserIDs, oi.UserID)
		}
	}

	seen := make(map[uint]bool)
	var uniqueIDs []uint
	for _, id := range searchUserIDs {
		if !seen[id] {
			seen[id] = true
			uniqueIDs = append(uniqueIDs, id)
		}
	}

	var similarities []userSimilarity
	for _, otherID := range uniqueIDs {
		var otherInteractions []models.UserInteraction
		s.db.Where("user_id = ?", otherID).Find(&otherInteractions)

		common := 0
		for _, oi := range otherInteractions {
			if userAssetSet[oi.AssetID] {
				common++
			}
		}

		if common > 0 {
			similarities = append(similarities, userSimilarity{
				UserID:      otherID,
				CommonCount: common,
			})
		}
	}

	sort.Slice(similarities, func(i, j int) bool {
		return similarities[i].CommonCount > similarities[j].CommonCount
	})

	if len(similarities) > maxUsers {
		similarities = similarities[:maxUsers]
	}

	var result []uint
	for _, sim := range similarities {
		result = append(result, sim.UserID)
	}
	return result
}

func (s *RecommendationService) getUserAssetIDs(userID uint) []uint {
	var ids []uint
	s.db.Model(&models.UserInteraction{}).
		Where("user_id = ?", userID).
		Select("DISTINCT asset_id").
		Pluck("asset_id", &ids)
	return ids
}

func (s *RecommendationService) fallbackPopular(limit int) []models.AssetRecommendation {
	var assets []models.Asset
	s.db.Order("created_at DESC").Limit(limit).Find(&assets)

	var recommendations []models.AssetRecommendation
	for _, asset := range assets {
		recommendations = append(recommendations, models.AssetRecommendation{
			AssetID: asset.ID,
			Asset:   asset,
			Score:   0,
			Reason:  "Popular asset",
		})
	}
	return recommendations
}

func (s *RecommendationService) MarkAsViewed(userID, recommendationID uint) error {
	return s.db.Model(&models.AssetRecommendation{}).
		Where("id = ? AND user_id = ?", recommendationID, userID).
		Update("is_viewed", true).Error
}

func (s *RecommendationService) GetContentBasedRecommendations(userID uint, limit int) ([]models.AssetRecommendation, error) {
	var pref models.UserPreference
	if err := s.db.Where("user_id = ?", userID).First(&pref).Error; err != nil {
		return s.fallbackPopular(limit), nil
	}

	categories := strings.Split(pref.PreferredCategories, ",")
	tags := strings.Split(pref.PreferredTags, ",")

	var assets []models.Asset
	query := s.db.Limit(limit)

	if len(categories) > 0 && categories[0] != "" {
		query = query.Where("asset_type IN ?", categories)
	}

	if len(tags) > 0 && tags[0] != "" {
		query = query.Where("id IN (SELECT asset_id FROM asset_tags WHERE tag_id IN (SELECT id FROM tags WHERE name IN ?))", tags)
	}

	if pref.PriceRangeMin > 0 {
		query = query.Where("CAST(metadata->>'price' AS FLOAT) >= ?", pref.PriceRangeMin)
	}
	if pref.PriceRangeMax > 0 {
		query = query.Where("CAST(metadata->>'price' AS FLOAT) <= ?", pref.PriceRangeMax)
	}

	query.Find(&assets)

	var recommendations []models.AssetRecommendation
	existingIDs := s.getUserAssetIDs(userID)
	existingMap := make(map[uint]bool)
	for _, id := range existingIDs {
		existingMap[id] = true
	}

	for _, asset := range assets {
		if existingMap[asset.ID] {
			continue
		}
		recommendations = append(recommendations, models.AssetRecommendation{
			AssetID: asset.ID,
			Asset:   asset,
			Score:   0,
			Reason:  "Based on your preferences",
		})
	}

	return recommendations, nil
}

func (s *RecommendationService) UpdateUserPreferences(userID uint, categories, tags []string, priceMin, priceMax float64, riskTolerance string) error {
	pref := models.UserPreference{
		UserID:             userID,
		PreferredCategories: strings.Join(categories, ","),
		PreferredTags:      strings.Join(tags, ","),
		PriceRangeMin:      priceMin,
		PriceRangeMax:      priceMax,
		RiskTolerance:      riskTolerance,
	}

	return s.db.Where("user_id = ?", userID).Assign(&pref).FirstOrCreate(&models.UserPreference{}).Error
}

func (s *RecommendationService) explainRecommendation(assetID, userID uint) string {
	var interactions []models.UserInteraction
	s.db.Where("asset_id = ? AND user_id != ?", assetID, userID).Limit(5).Find(&interactions)

	if len(interactions) > 3 {
		return "Highly popular among similar investors"
	}

	var purchaseCount int64
	s.db.Model(&models.UserInteraction{}).
		Where("asset_id = ? AND interaction_type = ?", assetID, models.InteractionPurchase).
		Count(&purchaseCount)

	if purchaseCount > 10 {
		return "Frequently purchased by users like you"
	}

	return "Recommended based on your interests"
}

func (s *RecommendationService) RefreshRecommendations() {
	log.Println("[recommendations] Starting daily refresh")

	var userIDs []uint
	s.db.Model(&models.UserInteraction{}).
		Select("DISTINCT user_id").
		Pluck("user_id", &userIDs)

	batchSize := 50
	for i := 0; i < len(userIDs); i += batchSize {
		end := int(math.Min(float64(i+batchSize), float64(len(userIDs))))
		batch := userIDs[i:end]

		for _, userID := range batch {
			recs, err := s.computeRecommendations(userID, 20)
			if err != nil {
				log.Printf("[recommendations] Failed for user %d: %v", userID, err)
				continue
			}

			s.db.Where("user_id = ?", userID).Delete(&models.AssetRecommendation{})

			for _, rec := range recs {
				rec.UserID = userID
				rec.ExpiresAt = time.Now().Add(24 * time.Hour)
				s.db.Create(&rec)
			}
		}
	}

	log.Printf("[recommendations] Refresh complete for %d users", len(userIDs))
}
