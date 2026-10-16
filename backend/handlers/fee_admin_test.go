package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourusername/kor-assetforge/models"
	"github.com/yourusername/kor-assetforge/services"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupFeeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&mode=memory"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.FeeConfig{},
		&models.FeeAuditLog{},
	))
	return db
}

func setupFeeRouter(db *gorm.DB, adminID uint) *gin.Engine {
	gin.SetMode(gin.TestMode)
	svc := services.NewFeeService(db)
	h := NewFeeAdminHandler(db, svc)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", adminID)
		c.Next()
	})

	admin := r.Group("/admin/fees")
	{
		admin.POST("", h.CreateFeeConfig)
		admin.GET("", h.ListFeeConfigs)
		admin.GET("/:id", h.GetFeeConfig)
		admin.PUT("/:id", h.UpdateFeeConfig)
		admin.DELETE("/:id", h.DeactivateFeeConfig)
		admin.GET("/:id/audit", h.GetFeeAuditLog)
	}
	r.GET("/fees/preview", h.PreviewFee)
	r.GET("/fees/active", h.GetActiveFee)

	return r
}

func TestFeeAdminHandler_CreateAndList(t *testing.T) {
	db := setupFeeTestDB(t)
	r := setupFeeRouter(db, 1)

	body := map[string]interface{}{
		"name":              "Standard Marketplace Fee",
		"fee_type":          "marketplace",
		"base_basis_points": 30,
		"min_basis_points":  5,
		"max_basis_points":  100,
		"volume_tiers":      json.RawMessage(`[{"min_volume_stroops":1000000,"discount_bps":5}]`),
		"reason":            "initial setup",
	}
	bodyBytes, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/admin/fees", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	var created models.FeeConfig
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	assert.Equal(t, "Standard Marketplace Fee", created.Name)
	assert.Equal(t, models.FeeTypeMarketplace, created.FeeType)
	assert.Equal(t, 30, created.BaseBasisPoints)
	assert.True(t, created.Active)

	// List
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/admin/fees?fee_type=marketplace", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var list map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	assert.Equal(t, float64(1), list["total"])
}

func TestFeeAdminHandler_UpdateFeeConfig(t *testing.T) {
	db := setupFeeTestDB(t)
	r := setupFeeRouter(db, 1)

	// Create
	createBody := map[string]interface{}{
		"name":              "Transfer Fee",
		"fee_type":          "transfer",
		"base_basis_points": 20,
		"min_basis_points":  2,
		"max_basis_points":  50,
	}
	bodyBytes, _ := json.Marshal(createBody)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/admin/fees", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var created models.FeeConfig
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))

	// Update
	updateBody := map[string]interface{}{
		"name":              "Transfer Fee",
		"fee_type":          "transfer",
		"base_basis_points": 25,
		"min_basis_points":  2,
		"max_basis_points":  50,
		"reason":            "raised due to network costs",
	}
	updateBytes, _ := json.Marshal(updateBody)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("PUT", fmt.Sprintf("/admin/fees/%d", created.ID), bytes.NewBuffer(updateBytes))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var updated models.FeeConfig
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &updated))
	assert.Equal(t, 25, updated.BaseBasisPoints)

	// Audit log should have 2 entries
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", fmt.Sprintf("/admin/fees/%d/audit", created.ID), nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var auditRes map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &auditRes))
	assert.Equal(t, float64(2), auditRes["total"])
}

func TestFeeAdminHandler_DeactivateFeeConfig(t *testing.T) {
	db := setupFeeTestDB(t)
	r := setupFeeRouter(db, 1)

	// Create
	createBody := map[string]interface{}{
		"name":              "Staking Fee",
		"fee_type":          "staking",
		"base_basis_points": 15,
		"min_basis_points":  1,
		"max_basis_points":  30,
	}
	bodyBytes, _ := json.Marshal(createBody)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/admin/fees", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var created models.FeeConfig
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))

	// Deactivate
	deactivateBody := map[string]interface{}{"reason": "deprecated"}
	deactivateBytes, _ := json.Marshal(deactivateBody)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("DELETE", fmt.Sprintf("/admin/fees/%d", created.ID), bytes.NewBuffer(deactivateBytes))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	// Verify it is deactivated
	var cfg models.FeeConfig
	db.First(&cfg, created.ID)
	assert.False(t, cfg.Active)
}

func TestFeeAdminHandler_PreviewFee(t *testing.T) {
	db := setupFeeTestDB(t)
	r := setupFeeRouter(db, 1)

	// Create a liquidity fee with a tier
	effectiveFrom := time.Now().Add(-1 * time.Hour)
	tiersJSON := `[{"min_volume_stroops":500000,"discount_bps":10}]`
	svc := services.NewFeeService(db)
	cfg := &models.FeeConfig{
		Name:            "Liquidity Fee",
		FeeType:         models.FeeTypeLiquidity,
		BaseBasisPoints: 50,
		MinBasisPoints:  5,
		MaxBasisPoints:  200,
		VolumeTiers:     tiersJSON,
		Active:          true,
		EffectiveFrom:   effectiveFrom,
	}
	require.NoError(t, svc.CreateFeeConfig(cfg, 1, "test"))

	t.Run("No tier - base fee applied", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/fees/preview?fee_type=liquidity&amount=10000&user_volume=100", nil)
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		var result models.EffectiveFee
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
		assert.Equal(t, 50, result.AppliedBps)
		assert.Nil(t, result.TierApplied)
		// fee = 10000 * 50 / 10000 = 50
		assert.Equal(t, int64(50), result.FeeAmount)
		assert.Equal(t, int64(9950), result.NetAmount)
	})

	t.Run("Tier discount applied", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/fees/preview?fee_type=liquidity&amount=10000&user_volume=600000", nil)
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		var result models.EffectiveFee
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
		// 50 - 10 = 40 bps
		assert.Equal(t, 40, result.AppliedBps)
		assert.Equal(t, 10, result.DiscountBps)
		require.NotNil(t, result.TierApplied)
		assert.Equal(t, int64(500000), result.TierApplied.MinVolumeStroops)
	})

	t.Run("Missing fee_type returns 400", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/fees/preview?amount=1000", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Unknown fee_type returns 400", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/fees/preview?fee_type=unknown&amount=1000", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestFeeAdminHandler_ValidationErrors(t *testing.T) {
	db := setupFeeTestDB(t)
	r := setupFeeRouter(db, 1)

	t.Run("Invalid fee_type rejected", func(t *testing.T) {
		body := map[string]interface{}{
			"name":              "Bad Fee",
			"fee_type":          "invalid_type",
			"base_basis_points": 30,
			"max_basis_points":  100,
		}
		bodyBytes, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/admin/fees", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Max below base rejected", func(t *testing.T) {
		body := map[string]interface{}{
			"name":              "Bad Fee2",
			"fee_type":          "marketplace",
			"base_basis_points": 100,
			"max_basis_points":  50,
		}
		bodyBytes, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/admin/fees", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Get nonexistent config returns 404", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/admin/fees/99999", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}
