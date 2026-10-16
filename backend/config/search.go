package config

import (
	"os"
	"strconv"
)

// SearchConfig contains relevance knobs that can be changed without code
// changes. Elasticsearch's BM25 similarity supplies TF/IDF relevance; these
// values control the relative importance of each indexed field.
type SearchConfig struct {
	FieldBoosts             map[string]float64
	PersonalizationBoost    float64
	VerifiedBoost           float64
	TreatmentTrafficPercent int
}

func LoadSearchConfig() SearchConfig {
	percentage := envInt("SEARCH_AB_TREATMENT_PERCENT", 50)
	if percentage > 100 {
		percentage = 100
	}
	return SearchConfig{
		FieldBoosts: map[string]float64{
			"name":        envFloat("SEARCH_BOOST_NAME", 4),
			"symbol":      envFloat("SEARCH_BOOST_SYMBOL", 3),
			"description": envFloat("SEARCH_BOOST_DESCRIPTION", 1.5),
			"asset_type":  envFloat("SEARCH_BOOST_ASSET_TYPE", 1.25),
			"categories":  envFloat("SEARCH_BOOST_CATEGORIES", 1.5),
			"tags":        envFloat("SEARCH_BOOST_TAGS", 1.25),
			"search_text": envFloat("SEARCH_BOOST_SEARCH_TEXT", 1),
		},
		PersonalizationBoost:    envFloat("SEARCH_PERSONALIZATION_BOOST", 1.75),
		VerifiedBoost:           envFloat("SEARCH_VERIFIED_BOOST", 1.1),
		TreatmentTrafficPercent: percentage,
	}
}

func envFloat(key string, fallback float64) float64 {
	value, err := strconv.ParseFloat(os.Getenv(key), 64)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
