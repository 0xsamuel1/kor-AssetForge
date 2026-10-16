//go:build ignore
// +build ignore

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAssetTokenizationFlow tests the complete asset tokenization workflow
func TestAssetTokenizationFlow(t *testing.T) {
	// Setup test environment
	setup := NewTestSetup(t)
	defer setup.Cleanup()

	// Test user data
	testUser := setup.CreateTestUser(t)
	require.NotNil(t, testUser)

	// Step 1: Complete KYC verification
	kycData := map[string]interface{}{
		"first_name":    "John",
		"last_name":     "Doe",
		"email":         "john.doe@example.com",
		"phone":         "+1234567890",
		"country":       "US",
		"address":       "123 Main St, City, State",
		"date_of_birth": "1990-01-01",
		"id_type":       "passport",
		"id_number":     "P12345678",
		"wallet_address": testUser.PublicKey,
	}

	// Submit KYC
	resp, err := setup.MakeRequest("POST", "/api/v1/kyc/submit", kycData, testUser.Token)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Approve KYC (admin action)
	setup.ApproveKYC(t, testUser.PublicKey)

	// Verify KYC status
	resp, err = setup.MakeRequest("GET", fmt.Sprintf("/api/v1/kyc/status/%s", testUser.PublicKey), nil, testUser.Token)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var kycStatus map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&kycStatus)
	require.NoError(t, err)
	assert.Equal(t, "approved", kycStatus["status"])

	// Step 2: Create new asset
	assetData := map[string]interface{}{
		"name":         "Test Real Estate Property",
		"code":         "TREPROP",
		"description":  "A test property for integration testing",
		"total_supply": "1000000",
		"decimals":     7,
		"metadata": map[string]interface{}{
			"property_type":  "residential",
			"location":      "Test City, Test State",
			"square_feet":   2000,
			"bedrooms":      3,
			"bathrooms":     2,
		},
	}

	resp, err = setup.MakeRequest("POST", "/api/v1/assets", assetData, testUser.Token)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var asset map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&asset)
	require.NoError(t, err)

	assetID := fmt.Sprintf("%.0f", asset["id"])
	require.NotEmpty(t, assetID)

	// Step 3: Verify asset creation
	resp, err = setup.MakeRequest("GET", fmt.Sprintf("/api/v1/assets/%s", assetID), nil, testUser.Token)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var retrievedAsset map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&retrievedAsset)
	require.NoError(t, err)
	assert.Equal(t, assetData["name"], retrievedAsset["name"])
	assert.Equal(t, assetData["code"], retrievedAsset["code"])

	// Step 4: Mint tokens (if applicable)
	mintData := map[string]interface{}{
		"amount":       "1000",
		"recipient":    testUser.PublicKey,
		"asset_id":     assetID,
	}

	resp, err = setup.MakeRequest("POST", "/api/v1/assets/mint", mintData, testUser.Token)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Step 5: Check user balance
	resp, err = setup.MakeRequest("GET", fmt.Sprintf("/api/v1/users/%s/balance", testUser.PublicKey), nil, testUser.Token)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var balance map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&balance)
	require.NoError(t, err)

	// Verify balance includes the minted tokens
	balances, ok := balance["balances"].([]interface{})
	require.True(t, ok)
	found := false
	for _, b := range balances {
		balanceMap := b.(map[string]interface{})
		if balanceMap["asset_code"] == assetData["code"] {
			found = true
			assert.Equal(t, "1000", balanceMap["balance"])
			break
		}
	}
	assert.True(t, found, "Minted tokens not found in user balance")

	t.Logf("✅ Asset tokenization flow completed successfully for asset %s", assetID)
}

// TestAssetTokenizationWithInvalidData tests tokenization with invalid data
func TestAssetTokenizationWithInvalidData(t *testing.T) {
	setup := NewTestSetup(t)
	defer setup.Cleanup()

	testUser := setup.CreateTestUser(t)

	// Test with missing required fields
	invalidAssetData := map[string]interface{}{
		"name": "", // Empty name should fail
		"code": "INVALID",
	}

	resp, err := setup.MakeRequest("POST", "/api/v1/assets", invalidAssetData, testUser.Token)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	// Test with duplicate asset code
	assetData := map[string]interface{}{
		"name":         "Test Asset",
		"code":         "DUPLICATE",
		"description":  "Test description",
		"total_supply": "1000000",
		"decimals":     7,
	}

	// Create first asset
	resp, err = setup.MakeRequest("POST", "/api/v1/assets", assetData, testUser.Token)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	// Try to create duplicate
	resp, err = setup.MakeRequest("POST", "/api/v1/assets", assetData, testUser.Token)
	require.NoError(t, err)
	require.Equal(t, http.StatusConflict, resp.StatusCode)

	t.Log("✅ Asset tokenization validation tests passed")
}

// TestAssetTokenizationPermissions tests permission requirements
func TestAssetTokenizationPermissions(t *testing.T) {
	setup := NewTestSetup(t)
	defer setup.Cleanup()

	// Test without authentication
	assetData := map[string]interface{}{
		"name":         "Unauthorized Asset",
		"code":         "UNAUTH",
		"description":  "Should fail without auth",
		"total_supply": "1000000",
		"decimals":     7,
	}

	resp, err := setup.MakeRequest("POST", "/api/v1/assets", assetData, "")
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// Test without KYC approval
	unverifiedUser := setup.CreateTestUser(t)
	// Don't approve KYC

	resp, err = setup.MakeRequest("POST", "/api/v1/assets", assetData, unverifiedUser.Token)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)

	t.Log("✅ Asset tokenization permission tests passed")
}
