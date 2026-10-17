package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yourusername/kor-assetforge/models"
)

func TestParseVolumeTiers(t *testing.T) {
	tiers, err := parseVolumeTiers(`[{"min_volume_stroops":100,"discount_bps":5}]`)
	require.NoError(t, err)
	require.Len(t, tiers, 1)
	assert.Equal(t, int64(100), tiers[0].MinVolumeStroops)
	assert.Equal(t, 5, tiers[0].DiscountBps)
}

func TestParseVolumeTiersEmpty(t *testing.T) {
	tiers, err := parseVolumeTiers("")
	require.NoError(t, err)
	assert.Nil(t, tiers)

	tiers, err = parseVolumeTiers("[]")
	require.NoError(t, err)
	assert.Nil(t, tiers)
}

func TestParseVolumeTiersInvalidJSON(t *testing.T) {
	_, err := parseVolumeTiers("not json")
	assert.Error(t, err)
}

func TestApplyTierDiscountNoTiers(t *testing.T) {
	applied, tier := applyTierDiscount(100, 10, nil, 1_000_000)
	assert.Equal(t, 100, applied)
	assert.Nil(t, tier)
}

func TestApplyTierDiscountMatchesHighestThreshold(t *testing.T) {
	tiers := []models.VolumeTier{
		{MinVolumeStroops: 100, DiscountBps: 5},
		{MinVolumeStroops: 1000, DiscountBps: 15},
	}
	applied, tier := applyTierDiscount(100, 10, tiers, 5000)
	assert.Equal(t, 85, applied)
	require.NotNil(t, tier)
	assert.Equal(t, int64(1000), tier.MinVolumeStroops)
}

func TestApplyTierDiscountBelowAllThresholds(t *testing.T) {
	tiers := []models.VolumeTier{{MinVolumeStroops: 1000, DiscountBps: 5}}
	applied, tier := applyTierDiscount(100, 10, tiers, 10)
	assert.Equal(t, 100, applied)
	assert.Nil(t, tier)
}

func TestApplyTierDiscountRespectsFloor(t *testing.T) {
	tiers := []models.VolumeTier{{MinVolumeStroops: 0, DiscountBps: 95}}
	applied, tier := applyTierDiscount(100, 10, tiers, 1000)
	assert.Equal(t, 10, applied)
	require.NotNil(t, tier)
}

func TestValidateFeeConfigValid(t *testing.T) {
	s := &FeeService{}
	cfg := &models.FeeConfig{
		Name:            "Standard",
		FeeType:         models.FeeTypeMarketplace,
		BaseBasisPoints: 100,
		MinBasisPoints:  10,
		MaxBasisPoints:  500,
	}
	require.NoError(t, s.validateConfig(cfg))
	assert.False(t, cfg.EffectiveFrom.IsZero())
}

func TestValidateFeeConfigRejectsInvalidType(t *testing.T) {
	s := &FeeService{}
	cfg := &models.FeeConfig{
		Name:            "Bad",
		FeeType:         models.FeeType("bogus"),
		BaseBasisPoints: 100,
		MaxBasisPoints:  500,
	}
	assert.Error(t, s.validateConfig(cfg))
}

func TestValidateFeeConfigRejectsNegativeBase(t *testing.T) {
	s := &FeeService{}
	cfg := &models.FeeConfig{
		Name:            "Bad",
		FeeType:         models.FeeTypeTransfer,
		BaseBasisPoints: -1,
		MaxBasisPoints:  500,
	}
	assert.Error(t, s.validateConfig(cfg))
}

func TestValidateFeeConfigRejectsMaxBelowBase(t *testing.T) {
	s := &FeeService{}
	cfg := &models.FeeConfig{
		Name:            "Bad",
		FeeType:         models.FeeTypeTransfer,
		BaseBasisPoints: 100,
		MaxBasisPoints:  50,
	}
	assert.Error(t, s.validateConfig(cfg))
}

func TestValidateFeeConfigRejectsEmptyName(t *testing.T) {
	s := &FeeService{}
	cfg := &models.FeeConfig{
		FeeType:         models.FeeTypeTransfer,
		BaseBasisPoints: 100,
		MaxBasisPoints:  500,
	}
	assert.Error(t, s.validateConfig(cfg))
}

func TestValidateFeeConfigRejectsInvalidTiers(t *testing.T) {
	s := &FeeService{}
	cfg := &models.FeeConfig{
		Name:            "Bad",
		FeeType:         models.FeeTypeTransfer,
		BaseBasisPoints: 100,
		MaxBasisPoints:  500,
		VolumeTiers:     "not json",
	}
	assert.Error(t, s.validateConfig(cfg))
}
