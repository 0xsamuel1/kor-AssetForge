package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCalculateCapitalGainGain(t *testing.T) {
	ts := &TaxService{}
	assert.Equal(t, int64(500), ts.CalculateCapitalGain(100, 150, 10))
}

func TestCalculateCapitalGainLoss(t *testing.T) {
	ts := &TaxService{}
	assert.Equal(t, int64(-250), ts.CalculateCapitalGain(200, 150, 5))
}

func TestCalculateCapitalGainZeroQuantity(t *testing.T) {
	ts := &TaxService{}
	assert.Equal(t, int64(0), ts.CalculateCapitalGain(100, 150, 0))
}
