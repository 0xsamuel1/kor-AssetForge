package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourusername/kor-assetforge/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestLiquidityHandler_AnalyticsAndComparison(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Set up memory DB
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(
		&models.Asset{},
		&models.LiquidityPool{},
		&models.LiquidityPosition{},
		&models.PoolSwap{},
	)
	require.NoError(t, err)

	// Seed assets
	assetA := models.Asset{Name: "Asset A", Symbol: "ASST-A", TotalSupply: 1000000, ContractID: "ASSET-A"}
	assetB := models.Asset{Name: "Asset B", Symbol: "ASST-B", TotalSupply: 2000000, ContractID: "ASSET-B"}
	db.Create(&assetA)
	db.Create(&assetB)

	// Seed pools
	pool1 := models.LiquidityPool{
		AssetAID:       assetA.ID,
		AssetBID:       assetB.ID,
		ReserveA:       100000,
		ReserveB:       200000,
		TotalLPTokens:  150000,
		FeeBasisPoints: 30,
		CreatorAddress: "G-CREATOR-1",
		Active:         true,
	}
	pool2 := models.LiquidityPool{
		AssetAID:       assetA.ID,
		AssetBID:       assetB.ID,
		ReserveA:       50000,
		ReserveB:       100000,
		TotalLPTokens:  75000,
		FeeBasisPoints: 30,
		CreatorAddress: "G-CREATOR-2",
		Active:         true,
	}
	db.Create(&pool1)
	db.Create(&pool2)

	// Seed position
	position := models.LiquidityPosition{
		PoolID:          pool1.ID,
		ProviderAddress: "G-PROVIDER-1",
		LPTokens:        50000,
		DepositedA:      30000,
		DepositedB:      50000,
	}
	db.Create(&position)

	// Seed swap (7d volume)
	swap := models.PoolSwap{
		PoolID:        pool1.ID,
		TraderAddress: "G-TRADER-1",
		InputAssetID:  assetA.ID,
		OutputAssetID: assetB.ID,
		InputAmount:   1000,
		OutputAmount:  2000,
		FeeAmount:     3,
		CreatedAt:     time.Now().Add(-10 * time.Minute),
	}
	db.Create(&swap)

	handler := NewLiquidityHandler(db)

	t.Run("GetPoolAnalytics", func(t *testing.T) {
		r := gin.New()
		r.GET("/pools/:id/analytics", handler.GetPoolAnalytics)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/pools/1/analytics", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var metrics models.PoolMetrics
		err = json.Unmarshal(w.Body.Bytes(), &metrics)
		require.NoError(t, err)

		assert.Equal(t, pool1.ID, metrics.PoolID)
		assert.Equal(t, int64(1000), metrics.Volume24h)
		assert.Equal(t, int64(3), metrics.Fees24h)
		assert.Equal(t, pool1.ReserveB*2, metrics.TVL)
		assert.True(t, metrics.APY > 0)
		assert.True(t, metrics.ImpermanentLoss > 0)
	})

	t.Run("ComparePools", func(t *testing.T) {
		r := gin.New()
		r.GET("/pools/compare", handler.ComparePools)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/pools/compare?pool_a=1&pool_b=2", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var comp models.PoolComparison
		err = json.Unmarshal(w.Body.Bytes(), &comp)
		require.NoError(t, err)

		assert.Equal(t, pool1.ID, comp.PoolA.ID)
		assert.Equal(t, pool2.ID, comp.PoolB.ID)
		assert.True(t, comp.TVLDifference > 0)
	})
}
