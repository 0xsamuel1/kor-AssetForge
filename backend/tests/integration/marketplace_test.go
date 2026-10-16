//go:build ignore
// +build ignore

package integration

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMarketplaceTradingFlow tests the complete marketplace trading workflow
func TestMarketplaceTradingFlow(t *testing.T) {
	setup := NewTestSetup(t)
	defer setup.Cleanup()

	// Create two users for trading
	seller := setup.CreateTestUser(t)
	buyer := setup.CreateTestUser(t)

	// Approve KYC for both users
	setup.ApproveKYC(t, seller.PublicKey)
	setup.ApproveKYC(t, buyer.PublicKey)

	// Step 1: Create an asset for trading
	assetData := map[string]interface{}{
		"name":         "Trading Test Asset",
		"code":         "TRADE",
		"description":  "Asset for marketplace trading test",
		"total_supply": "1000000",
		"decimals":     7,
	}

	resp, err := setup.MakeRequest("POST", "/api/v1/assets", assetData, seller.Token)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var asset map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&asset)
	require.NoError(t, err)

	assetID := fmt.Sprintf("%.0f", asset["id"])

	// Step 2: Mint tokens to seller
	mintData := map[string]interface{}{
		"amount":    "10000",
		"recipient": seller.PublicKey,
		"asset_id":  assetID,
	}

	resp, err = setup.MakeRequest("POST", "/api/v1/assets/mint", mintData, seller.Token)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Step 3: Create marketplace listing
	listingData := map[string]interface{}{
		"asset_id": assetID,
		"price":    "1000", // 1000 units of base currency
		"amount":   "100",  // 100 tokens for sale
	}

	resp, err = setup.MakeRequest("POST", "/api/v1/marketplace/list", listingData, seller.Token)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var listing map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&listing)
	require.NoError(t, err)

	listingID := fmt.Sprintf("%.0f", listing["id"])
	require.NotEmpty(t, listingID)

	// Step 4: Verify listing appears in marketplace
	resp, err = setup.MakeRequest("GET", "/api/v1/marketplace/listings", nil, "")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var listings map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&listings)
	require.NoError(t, err)

	listingsData, ok := listings["data"].([]interface{})
	require.True(t, ok)
	require.Greater(t, len(listingsData), 0)

	// Find our listing
	found := false
	for _, l := range listingsData {
		listingMap := l.(map[string]interface{})
		if fmt.Sprintf("%.0f", listingMap["id"]) == listingID {
			found = true
			assert.Equal(t, assetID, fmt.Sprintf("%.0f", listingMap["asset_id"]))
			assert.Equal(t, "1000", listingMap["price"])
			assert.Equal(t, "100", listingMap["amount"])
			assert.Equal(t, "active", listingMap["status"])
			break
		}
	}
	assert.True(t, found, "Created listing not found in marketplace")

	// Step 5: Buyer purchases from listing
	purchaseData := map[string]interface{}{
		"listing_id": listingID,
		"amount":     "50", // Buy 50 tokens
	}

	resp, err = setup.MakeRequest("POST", "/api/v1/marketplace/purchase", purchaseData, buyer.Token)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Step 6: Verify purchase completed
	resp, err = setup.MakeRequest("GET", fmt.Sprintf("/api/v1/marketplace/listings/%s", listingID), nil, "")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var updatedListing map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&updatedListing)
	require.NoError(t, err)

	// Check remaining amount
	remainingAmount := updatedListing["remaining_amount"].(float64)
	assert.Equal(t, float64(50), remainingAmount) // 100 - 50 = 50 remaining

	// Step 7: Verify buyer's balance increased
	resp, err = setup.MakeRequest("GET", fmt.Sprintf("/api/v1/users/%s/balance", buyer.PublicKey), nil, buyer.Token)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var buyerBalance map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&buyerBalance)
	require.NoError(t, err)

	balances, ok := buyerBalance["balances"].([]interface{})
	require.True(t, ok)
	foundBuyerBalance := false
	for _, b := range balances {
		balanceMap := b.(map[string]interface{})
		if balanceMap["asset_code"] == assetData["code"] {
			foundBuyerBalance = true
			assert.Equal(t, "50", balanceMap["balance"])
			break
		}
	}
	assert.True(t, foundBuyerBalance, "Purchased tokens not found in buyer balance")

	// Step 8: Complete the listing (sell remaining tokens)
	resp, err = setup.MakeRequest("POST", fmt.Sprintf("/api/v1/marketplace/listings/%s/complete", listingID), nil, seller.Token)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify listing is now closed
	resp, err = setup.MakeRequest("GET", fmt.Sprintf("/api/v1/marketplace/listings/%s", listingID), nil, "")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var closedListing map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&closedListing)
	require.NoError(t, err)
	assert.Equal(t, "completed", closedListing["status"])

	t.Logf("✅ Marketplace trading flow completed successfully for listing %s", listingID)
}

// TestMarketplacePermissions tests marketplace permission requirements
func TestMarketplacePermissions(t *testing.T) {
	setup := NewTestSetup(t)
	defer setup.Cleanup()

	unverifiedUser := setup.CreateTestUser(t) // No KYC approval
	verifiedUser := setup.CreateTestUser(t)
	setup.ApproveKYC(t, verifiedUser.PublicKey)

	// Create asset
	assetData := map[string]interface{}{
		"name":         "Permission Test Asset",
		"code":         "PERM",
		"description":  "Testing permissions",
		"total_supply": "1000000",
		"decimals":     7,
	}

	resp, err := setup.MakeRequest("POST", "/api/v1/assets", assetData, verifiedUser.Token)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var asset map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&asset)
	require.NoError(t, err)
	assetID := fmt.Sprintf("%.0f", asset["id"])

	// Test 1: Unverified user cannot create listing
	listingData := map[string]interface{}{
		"asset_id": assetID,
		"price":    "1000",
		"amount":   "100",
	}

	resp, err = setup.MakeRequest("POST", "/api/v1/marketplace/list", listingData, unverifiedUser.Token)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)

	// Test 2: Verified user can create listing
	resp, err = setup.MakeRequest("POST", "/api/v1/marketplace/list", listingData, verifiedUser.Token)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var listing map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&listing)
	require.NoError(t, err)
	listingID := fmt.Sprintf("%.0f", listing["id"])

	// Test 3: Unverified user cannot purchase
	purchaseData := map[string]interface{}{
		"listing_id": listingID,
		"amount":     "10",
	}

	resp, err = setup.MakeRequest("POST", "/api/v1/marketplace/purchase", purchaseData, unverifiedUser.Token)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)

	// Test 4: Verified user can purchase
	resp, err = setup.MakeRequest("POST", "/api/v1/marketplace/purchase", purchaseData, verifiedUser.Token)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusOK)

	t.Log("✅ Marketplace permission tests passed")
}

// TestMarketplaceValidations tests marketplace input validation
func TestMarketplaceValidations(t *testing.T) {
	setup := NewTestSetup(t)
	defer setup.Cleanup()

	user := setup.CreateTestUser(t)
	setup.ApproveKYC(t, user.PublicKey)

	// Test invalid listing data
	testCases := []struct {
		name   string
		data   map[string]interface{}
		status int
	}{
		{
			name: "Missing asset_id",
			data: map[string]interface{}{
				"price":  "1000",
				"amount": "100",
			},
			status: http.StatusBadRequest,
		},
		{
			name: "Invalid price",
			data: map[string]interface{}{
				"asset_id": "1",
				"price":    "-100", // Negative price
				"amount":   "100",
			},
			status: http.StatusBadRequest,
		},
		{
			name: "Invalid amount",
			data: map[string]interface{}{
				"asset_id": "1",
				"price":    "1000",
				"amount":   "0", // Zero amount
			},
			status: http.StatusBadRequest,
		},
		{
			name: "Non-existent asset",
			data: map[string]interface{}{
				"asset_id": "999999", // Non-existent asset
				"price":    "1000",
				"amount":   "100",
			},
			status: http.StatusNotFound,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := setup.MakeRequest("POST", "/api/v1/marketplace/list", tc.data, user.Token)
			require.NoError(t, err)
			assert.Equal(t, tc.status, resp.StatusCode)
		})
	}

	t.Log("✅ Marketplace validation tests passed")
}

// TestMarketplaceSearchAndFilters tests marketplace search functionality
func TestMarketplaceSearchAndFilters(t *testing.T) {
	setup := NewTestSetup(t)
	defer setup.Cleanup()

	user := setup.CreateTestUser(t)
	setup.ApproveKYC(t, user.PublicKey)

	// Create multiple assets and listings
	assets := []map[string]interface{}{
		{
			"name":         "Real Estate Property",
			"code":         "REAL",
			"description":  "Luxury apartment in downtown",
			"total_supply": "1000000",
			"decimals":     7,
		},
		{
			"name":         "Digital Art NFT",
			"code":         "ART",
			"description":  "Unique digital artwork",
			"total_supply": "100",
			"decimals":     0,
		},
	}

	createdAssets := []string{}
	for _, assetData := range assets {
		resp, err := setup.MakeRequest("POST", "/api/v1/assets", assetData, user.Token)
		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, resp.StatusCode)

		var asset map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&asset)
		require.NoError(t, err)
		createdAssets = append(createdAssets, fmt.Sprintf("%.0f", asset["id"]))
	}

	// Create listings for each asset
	for _, assetID := range createdAssets {
		listingData := map[string]interface{}{
			"asset_id": assetID,
			"price":    "1000",
			"amount":   "100",
		}
		resp, err := setup.MakeRequest("POST", "/api/v1/marketplace/list", listingData, user.Token)
		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	}

	// Test search by asset code
	resp, err := setup.MakeRequest("GET", "/api/v1/marketplace/listings?search=REAL", nil, "")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var searchResults map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&searchResults)
	require.NoError(t, err)

	searchData, ok := searchResults["data"].([]interface{})
	require.True(t, ok)
	require.Equal(t, 1, len(searchData)) // Should find only REAL asset

	// Test filter by price range
	resp, err = setup.MakeRequest("GET", "/api/v1/marketplace/listings?min_price=500&max_price=1500", nil, "")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var priceResults map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&priceResults)
	require.NoError(t, err)

	priceData, ok := priceResults["data"].([]interface{})
	require.True(t, ok)
	require.Greater(t, len(priceData), 0) // Should find listings in price range

	t.Log("✅ Marketplace search and filter tests passed")
}
