//go:build ignore
// +build ignore

package integration

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestKYCWorkflowFlow tests the complete KYC verification workflow
func TestKYCWorkflowFlow(t *testing.T) {
	setup := NewTestSetup(t)
	defer setup.Cleanup()

	// Step 1: Create user
	user := setup.CreateTestUser(t)

	// Step 2: Submit KYC application
	kycData := map[string]interface{}{
		"first_name":    "Alice",
		"last_name":     "Smith",
		"email":         "alice.smith@example.com",
		"phone":         "+1234567890",
		"country":       "US",
		"address":       "456 Oak Avenue, Portland, OR 97201",
		"date_of_birth": "1985-06-15",
		"id_type":       "drivers_license",
		"id_number":     "D12345678",
		"wallet_address": user.PublicKey,
	}

	resp, err := setup.MakeRequest("POST", "/api/v1/kyc/submit", kycData, user.Token)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Step 3: Verify KYC status is pending
	resp, err = setup.MakeRequest("GET", fmt.Sprintf("/api/v1/kyc/status/%s", user.PublicKey), nil, user.Token)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var kycStatus map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&kycStatus)
	require.NoError(t, err)
	assert.Equal(t, "pending", kycStatus["status"])

	// Step 4: Admin reviews KYC application
	resp, err = setup.MakeRequest("GET", fmt.Sprintf("/api/v1/kyc/applications/%s", user.PublicKey), nil, setup.AdminToken)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var application map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&application)
	require.NoError(t, err)
	assert.Equal(t, kycData["first_name"], application["first_name"])
	assert.Equal(t, kycData["last_name"], application["last_name"])
	assert.Equal(t, "pending", application["status"])

	// Step 5: Admin approves KYC
	approvalData := map[string]interface{}{
		"user_id": user.UserID,
		"status":  "approved",
		"reason":  "All documents verified and complete",
	}

	resp, err = setup.MakeRequest("POST", "/api/v1/kyc/review", approvalData, setup.AdminToken)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Step 6: Verify KYC status is approved
	resp, err = setup.MakeRequest("GET", fmt.Sprintf("/api/v1/kyc/status/%s", user.PublicKey), nil, user.Token)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var approvedStatus map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&approvedStatus)
	require.NoError(t, err)
	assert.Equal(t, "approved", approvedStatus["status"])
	assert.NotNil(t, approvedStatus["approved_at"])

	// Step 7: Verify user can now access restricted features
	// Try to create an asset (should work with approved KYC)
	assetData := map[string]interface{}{
		"name":         "KYC Test Asset",
		"code":         "KYCTEST",
		"description":  "Asset created after KYC approval",
		"total_supply": "1000000",
		"decimals":     7,
	}

	resp, err = setup.MakeRequest("POST", "/api/v1/assets", assetData, user.Token)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	t.Logf("✅ KYC workflow flow completed successfully for user %s", user.PublicKey)
}

// TestKYCRejectionFlow tests KYC rejection workflow
func TestKYCRejectionFlow(t *testing.T) {
	setup := NewTestSetup(t)
	defer setup.Cleanup()

	user := setup.CreateTestUser(t)

	// Submit incomplete KYC data
	kycData := map[string]interface{}{
		"first_name":    "Bob",
		"last_name":     "", // Missing last name
		"email":         "invalid-email", // Invalid email
		"phone":         "+1234567890",
		"country":       "US",
		"address":       "789 Pine Street, Seattle, WA 98101",
		"date_of_birth": "1990-12-01",
		"id_type":       "passport",
		"id_number":     "P87654321",
		"wallet_address": user.PublicKey,
	}

	resp, err := setup.MakeRequest("POST", "/api/v1/kyc/submit", kycData, user.Token)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Admin rejects KYC
	rejectionData := map[string]interface{}{
		"user_id": user.UserID,
		"status":  "rejected",
		"reason":  "Incomplete information: missing last name and invalid email format",
	}

	resp, err = setup.MakeRequest("POST", "/api/v1/kyc/review", rejectionData, setup.AdminToken)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify rejection status
	resp, err = setup.MakeRequest("GET", fmt.Sprintf("/api/v1/kyc/status/%s", user.PublicKey), nil, user.Token)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var rejectedStatus map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&rejectedStatus)
	require.NoError(t, err)
	assert.Equal(t, "rejected", rejectedStatus["status"])
	assert.Equal(t, rejectionData["reason"], rejectedStatus["rejection_reason"])

	// Verify user cannot access restricted features
	assetData := map[string]interface{}{
		"name":         "Should Fail Asset",
		"code":         "FAIL",
		"description":  "This should fail due to rejected KYC",
		"total_supply": "1000000",
		"decimals":     7,
	}

	resp, err = setup.MakeRequest("POST", "/api/v1/assets", assetData, user.Token)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)

	t.Log("✅ KYC rejection flow tests passed")
}

// TestKYCValidations tests KYC input validation
func TestKYCValidations(t *testing.T) {
	setup := NewTestSetup(t)
	defer setup.Cleanup()

	user := setup.CreateTestUser(t)

	// Test invalid KYC data
	testCases := []struct {
		name   string
		data   map[string]interface{}
		status int
	}{
		{
			name: "Missing required fields",
			data: map[string]interface{}{
				"first_name": "Test",
				// Missing last_name, email, etc.
			},
			status: http.StatusBadRequest,
		},
		{
			name: "Invalid email format",
			data: map[string]interface{}{
				"first_name":    "Test",
				"last_name":     "User",
				"email":         "not-an-email",
				"phone":         "+1234567890",
				"country":       "US",
				"address":       "123 Test St",
				"date_of_birth": "1990-01-01",
				"id_type":       "drivers_license",
				"id_number":     "D123456",
				"wallet_address": user.PublicKey,
			},
			status: http.StatusBadRequest,
		},
		{
			name: "Invalid date format",
			data: map[string]interface{}{
				"first_name":    "Test",
				"last_name":     "User",
				"email":         "test@example.com",
				"phone":         "+1234567890",
				"country":       "US",
				"address":       "123 Test St",
				"date_of_birth": "01-01-1990", // Wrong format
				"id_type":       "drivers_license",
				"id_number":     "D123456",
				"wallet_address": user.PublicKey,
			},
			status: http.StatusBadRequest,
		},
		{
			name: "Invalid ID type",
			data: map[string]interface{}{
				"first_name":    "Test",
				"last_name":     "User",
				"email":         "test@example.com",
				"phone":         "+1234567890",
				"country":       "US",
				"address":       "123 Test St",
				"date_of_birth": "1990-01-01",
				"id_type":       "invalid_id", // Invalid type
				"id_number":     "D123456",
				"wallet_address": user.PublicKey,
			},
			status: http.StatusBadRequest,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := setup.MakeRequest("POST", "/api/v1/kyc/submit", tc.data, user.Token)
			require.NoError(t, err)
			assert.Equal(t, tc.status, resp.StatusCode)
		})
	}

	t.Log("✅ KYC validation tests passed")
}

// TestKYCDuplicateSubmission tests duplicate KYC submission handling
func TestKYCDuplicateSubmission(t *testing.T) {
	setup := NewTestSetup(t)
	defer setup.Cleanup()

	user := setup.CreateTestUser(t)

	// Submit first KYC application
	kycData := map[string]interface{}{
		"first_name":    "Charlie",
		"last_name":     "Brown",
		"email":         "charlie.brown@example.com",
		"phone":         "+1234567890",
		"country":       "US",
		"address":       "321 Elm Street, Boston, MA 02101",
		"date_of_birth": "1975-03-20",
		"id_type":       "passport",
		"id_number":     "P11223344",
		"wallet_address": user.PublicKey,
	}

	resp, err := setup.MakeRequest("POST", "/api/v1/kyc/submit", kycData, user.Token)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Try to submit duplicate KYC application
	resp, err = setup.MakeRequest("POST", "/api/v1/kyc/submit", kycData, user.Token)
	require.NoError(t, err)
	require.Equal(t, http.StatusConflict, resp.StatusCode)

	// Verify original application is still pending
	resp, err = setup.MakeRequest("GET", fmt.Sprintf("/api/v1/kyc/status/%s", user.PublicKey), nil, user.Token)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var status map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&status)
	require.NoError(t, err)
	assert.Equal(t, "pending", status["status"])

	t.Log("✅ KYC duplicate submission tests passed")
}

// TestKYCAdminPermissions tests KYC admin permission requirements
func TestKYCAdminPermissions(t *testing.T) {
	setup := NewTestSetup(t)
	defer setup.Cleanup()

	user := setup.CreateTestUser(t)
	setup.ApproveKYC(t, user.PublicKey)

	// Test 1: Regular user cannot review KYC applications
	reviewData := map[string]interface{}{
		"user_id": user.UserID,
		"status":  "approved",
		"reason":  "Trying to approve without admin rights",
	}

	resp, err := setup.MakeRequest("POST", "/api/v1/kyc/review", reviewData, user.Token)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)

	// Test 2: Regular user cannot view KYC applications
	resp, err = setup.MakeRequest("GET", fmt.Sprintf("/api/v1/kyc/applications/%s", user.PublicKey), nil, user.Token)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)

	// Test 3: Admin can review KYC applications
	resp, err = setup.MakeRequest("POST", "/api/v1/kyc/review", reviewData, setup.AdminToken)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Test 4: Admin can view KYC applications
	resp, err = setup.MakeRequest("GET", fmt.Sprintf("/api/v1/kyc/applications/%s", user.PublicKey), nil, setup.AdminToken)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	t.Log("✅ KYC admin permission tests passed")
}
