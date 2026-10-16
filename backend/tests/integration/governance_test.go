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

// TestGovernanceVotingFlow tests the complete governance voting workflow
func TestGovernanceVotingFlow(t *testing.T) {
	setup := NewTestSetup(t)
	defer setup.Cleanup()

	// Create multiple users for voting
	proposer := setup.CreateTestUser(t)
	voter1 := setup.CreateTestUser(t)
	voter2 := setup.CreateTestUser(t)
	voter3 := setup.CreateTestUser(t)

	// Approve KYC for all users
	setup.ApproveKYC(t, proposer.PublicKey)
	setup.ApproveKYC(t, voter1.PublicKey)
	setup.ApproveKYC(t, voter2.PublicKey)
	setup.ApproveKYC(t, voter3.PublicKey)

	// Step 1: Create governance proposal
	proposalData := map[string]interface{}{
		"title":       "Update Trading Fees",
		"description": "Reduce marketplace trading fees from 2% to 1%",
		"type":        "parameter_change",
		"parameters": map[string]interface{}{
			"trading_fee": "1.0",
		},
		"voting_period": 7, // 7 days
	}

	resp, err := setup.MakeRequest("POST", "/api/v1/governance/proposals", proposalData, proposer.Token)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var proposal map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&proposal)
	require.NoError(t, err)

	proposalID := fmt.Sprintf("%.0f", proposal["id"])
	require.NotEmpty(t, proposalID)

	// Step 2: Verify proposal creation
	resp, err = setup.MakeRequest("GET", fmt.Sprintf("/api/v1/governance/proposals/%s", proposalID), nil, "")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var retrievedProposal map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&retrievedProposal)
	require.NoError(t, err)
	assert.Equal(t, proposalData["title"], retrievedProposal["title"])
	assert.Equal(t, "active", retrievedProposal["status"])
	assert.Equal(t, float64(0), retrievedProposal["votes_for"])
	assert.Equal(t, float64(0), retrievedProposal["votes_against"])

	// Step 3: Cast votes
	voteData := map[string]interface{}{
		"support": true,
		"reason":  "I support reducing fees for better market liquidity",
	}

	// Voter 1 votes in favor
	resp, err = setup.MakeRequest("POST", fmt.Sprintf("/api/v1/governance/proposals/%s/vote", proposalID), voteData, voter1.Token)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Voter 2 votes in favor
	resp, err = setup.MakeRequest("POST", fmt.Sprintf("/api/v1/governance/proposals/%s/vote", proposalID), voteData, voter2.Token)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Voter 3 votes against
	voteData["support"] = false
	voteData["reason"] = "I think 2% is appropriate for platform sustainability"
	resp, err = setup.MakeRequest("POST", fmt.Sprintf("/api/v1/governance/proposals/%s/vote", proposalID), voteData, voter3.Token)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Step 4: Verify vote counting
	resp, err = setup.MakeRequest("GET", fmt.Sprintf("/api/v1/governance/proposals/%s", proposalID), nil, "")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var votedProposal map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&votedProposal)
	require.NoError(t, err)
	assert.Equal(t, float64(2), votedProposal["votes_for"])
	assert.Equal(t, float64(1), votedProposal["votes_against"])

	// Step 5: Check individual votes
	resp, err = setup.MakeRequest("GET", fmt.Sprintf("/api/v1/governance/proposals/%s/votes", proposalID), nil, "")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var votes map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&votes)
	require.NoError(t, err)

	votesData, ok := votes["data"].([]interface{})
	require.True(t, ok)
	require.Equal(t, 3, len(votesData))

	// Verify votes are recorded correctly
	voteCount := 0
	for _, v := range votesData {
		voteMap := v.(map[string]interface{})
		if voteMap["support"].(bool) {
			voteCount++
		}
	}
	assert.Equal(t, 2, voteCount) // 2 votes for, 1 against

	// Step 6: Execute proposal (admin action in real implementation)
	// For testing, we'll simulate execution
	resp, err = setup.MakeRequest("POST", fmt.Sprintf("/api/v1/governance/proposals/%s/execute", proposalID), nil, setup.AdminToken)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Step 7: Verify proposal execution
	resp, err = setup.MakeRequest("GET", fmt.Sprintf("/api/v1/governance/proposals/%s", proposalID), nil, "")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var executedProposal map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&executedProposal)
	require.NoError(t, err)
	assert.Equal(t, "executed", executedProposal["status"])

	t.Logf("✅ Governance voting flow completed successfully for proposal %s", proposalID)
}

// TestGovernancePermissions tests governance permission requirements
func TestGovernancePermissions(t *testing.T) {
	setup := NewTestSetup(t)
	defer setup.Cleanup()

	unverifiedUser := setup.CreateTestUser(t) // No KYC approval
	verifiedUser := setup.CreateTestUser(t)
	setup.ApproveKYC(t, verifiedUser.PublicKey)

	// Test 1: Unverified user cannot create proposal
	proposalData := map[string]interface{}{
		"title":       "Test Proposal",
		"description":  "Should fail without KYC",
		"type":        "parameter_change",
		"voting_period": 7,
	}

	resp, err := setup.MakeRequest("POST", "/api/v1/governance/proposals", proposalData, unverifiedUser.Token)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)

	// Test 2: Verified user can create proposal
	resp, err = setup.MakeRequest("POST", "/api/v1/governance/proposals", proposalData, verifiedUser.Token)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var proposal map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&proposal)
	require.NoError(t, err)
	proposalID := fmt.Sprintf("%.0f", proposal["id"])

	// Test 3: Unverified user cannot vote
	voteData := map[string]interface{}{
		"support": true,
		"reason":  "Test vote",
	}

	resp, err = setup.MakeRequest("POST", fmt.Sprintf("/api/v1/governance/proposals/%s/vote", proposalID), voteData, unverifiedUser.Token)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)

	// Test 4: Verified user can vote
	resp, err = setup.MakeRequest("POST", fmt.Sprintf("/api/v1/governance/proposals/%s/vote", proposalID), voteData, verifiedUser.Token)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Test 5: Cannot vote twice
	resp, err = setup.MakeRequest("POST", fmt.Sprintf("/api/v1/governance/proposals/%s/vote", proposalID), voteData, verifiedUser.Token)
	require.NoError(t, err)
	require.Equal(t, http.StatusConflict, resp.StatusCode)

	t.Log("✅ Governance permission tests passed")
}

// TestGovernanceValidations tests governance input validation
func TestGovernanceValidations(t *testing.T) {
	setup := NewTestSetup(t)
	defer setup.Cleanup()

	user := setup.CreateTestUser(t)
	setup.ApproveKYC(t, user.PublicKey)

	// Test invalid proposal data
	testCases := []struct {
		name   string
		data   map[string]interface{}
		status int
	}{
		{
			name: "Missing title",
			data: map[string]interface{}{
				"description":  "Test proposal",
				"type":        "parameter_change",
				"voting_period": 7,
			},
			status: http.StatusBadRequest,
		},
		{
			name: "Invalid voting period",
			data: map[string]interface{}{
				"title":       "Test Proposal",
				"description":  "Test proposal",
				"type":        "parameter_change",
				"voting_period": -1, // Negative voting period
			},
			status: http.StatusBadRequest,
		},
		{
			name: "Invalid proposal type",
			data: map[string]interface{}{
				"title":       "Test Proposal",
				"description":  "Test proposal",
				"type":        "invalid_type",
				"voting_period": 7,
			},
			status: http.StatusBadRequest,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := setup.MakeRequest("POST", "/api/v1/governance/proposals", tc.data, user.Token)
			require.NoError(t, err)
			assert.Equal(t, tc.status, resp.StatusCode)
		})
	}

	t.Log("✅ Governance validation tests passed")
}

// TestGovernanceQuorum tests governance quorum requirements
func TestGovernanceQuorum(t *testing.T) {
	setup := NewTestSetup(t)
	defer setup.Cleanup()

	// Create users
	proposer := setup.CreateTestUser(t)
	voters := []*TestUser{}
	for i := 0; i < 5; i++ {
		voter := setup.CreateTestUser(t)
		setup.ApproveKYC(t, voter.PublicKey)
		voters = append(voters, voter)
	}
	setup.ApproveKYC(t, proposer.PublicKey)

	// Create proposal with high quorum requirement
	proposalData := map[string]interface{}{
		"title":       "High Quorum Test",
		"description":  "Test proposal with high quorum requirement",
		"type":        "parameter_change",
		"voting_period": 7,
		"quorum":      80, // 80% quorum required
	}

	resp, err := setup.MakeRequest("POST", "/api/v1/governance/proposals", proposalData, proposer.Token)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var proposal map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&proposal)
	require.NoError(t, err)
	proposalID := fmt.Sprintf("%.0f", proposal["id"])

	// Only 3 out of 5 users vote (60% participation)
	for i := 0; i < 3; i++ {
		voteData := map[string]interface{}{
			"support": true,
			"reason":  "Test vote",
		}
		resp, err = setup.MakeRequest("POST", fmt.Sprintf("/api/v1/governance/proposals/%s/vote", proposalID), voteData, voters[i].Token)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
	}

	// Try to execute - should fail due to insufficient quorum
	resp, err = setup.MakeRequest("POST", fmt.Sprintf("/api/v1/governance/proposals/%s/execute", proposalID), nil, setup.AdminToken)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	// Add remaining votes to reach quorum
	for i := 3; i < 5; i++ {
		voteData := map[string]interface{}{
			"support": true,
			"reason":  "Additional vote for quorum",
		}
		resp, err = setup.MakeRequest("POST", fmt.Sprintf("/api/v1/governance/proposals/%s/vote", proposalID), voteData, voters[i].Token)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
	}

	// Now execution should succeed
	resp, err = setup.MakeRequest("POST", fmt.Sprintf("/api/v1/governance/proposals/%s/execute", proposalID), nil, setup.AdminToken)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	t.Log("✅ Governance quorum tests passed")
}
