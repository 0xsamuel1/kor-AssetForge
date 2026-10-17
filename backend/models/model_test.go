package models

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBeneficialOwnerRequiresDeclarationAtThreshold(t *testing.T) {
	bo := &BeneficialOwner{OwnershipPercent: UBOOwnershipThresholdPct}
	assert.True(t, bo.RequiresDeclaration())
}

func TestBeneficialOwnerRequiresDeclarationAboveThreshold(t *testing.T) {
	bo := &BeneficialOwner{OwnershipPercent: 60}
	assert.True(t, bo.RequiresDeclaration())
}

func TestBeneficialOwnerBelowThresholdNotRequired(t *testing.T) {
	bo := &BeneficialOwner{OwnershipPercent: 10}
	assert.False(t, bo.RequiresDeclaration())
}

func TestBeneficialOwnerControllingPartyAlwaysRequired(t *testing.T) {
	bo := &BeneficialOwner{OwnershipPercent: 0, IsControllingParty: true}
	assert.True(t, bo.RequiresDeclaration())
}

func TestBeneficialOwnerJSONHidesSensitiveFields(t *testing.T) {
	bo := BeneficialOwner{
		ID:                 1,
		FullName:           "Jane Doe",
		DocumentNumberHash: "deadbeef",
	}
	data, err := json.Marshal(bo)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "deadbeef")
	assert.Contains(t, string(data), "Jane Doe")
}

func TestFeeTypeConstants(t *testing.T) {
	assert.Equal(t, FeeType("marketplace"), FeeTypeMarketplace)
	assert.Equal(t, FeeType("transfer"), FeeTypeTransfer)
	assert.Equal(t, FeeType("staking"), FeeTypeStaking)
	assert.Equal(t, FeeType("liquidity"), FeeTypeLiquidity)
}
