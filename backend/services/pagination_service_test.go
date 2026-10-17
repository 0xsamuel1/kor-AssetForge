package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeDecodeCursorRoundTrip(t *testing.T) {
	cursor := EncodeCursor(42)
	got, err := DecodeCursor(cursor)
	require.NoError(t, err)
	assert.Equal(t, uint(42), got)
}

func TestDecodeCursorEmpty(t *testing.T) {
	got, err := DecodeCursor("")
	require.NoError(t, err)
	assert.Equal(t, uint(0), got)
}

func TestDecodeCursorInvalid(t *testing.T) {
	_, err := DecodeCursor("!!!not-base64!!!")
	assert.Error(t, err)
}

func TestDecodeCursorBadPrefix(t *testing.T) {
	// base64 of "wrong:1"
	_, err := DecodeCursor("d3Jvbmc6MQ==")
	assert.Error(t, err)
}

func TestNormalizePaginationDefaults(t *testing.T) {
	limit, afterID, beforeID, forward, err := NormalizePaginationArgs(nil, nil, "", "")
	require.NoError(t, err)
	assert.Equal(t, defaultPageSize, limit)
	assert.Equal(t, uint(0), afterID)
	assert.Equal(t, uint(0), beforeID)
	assert.True(t, forward)
}

func TestNormalizePaginationForwardFirst(t *testing.T) {
	first := 5
	limit, _, _, forward, err := NormalizePaginationArgs(&first, nil, EncodeCursor(10), "")
	require.NoError(t, err)
	assert.Equal(t, 5, limit)
	assert.True(t, forward)
}

func TestNormalizePaginationBackward(t *testing.T) {
	last := 3
	limit, _, beforeID, forward, err := NormalizePaginationArgs(nil, &last, "", EncodeCursor(20))
	require.NoError(t, err)
	assert.Equal(t, 3, limit)
	assert.Equal(t, uint(20), beforeID)
	assert.False(t, forward)
}

func TestNormalizePaginationClampsLimits(t *testing.T) {
	tooBig := 1000
	limit, _, _, _, err := NormalizePaginationArgs(&tooBig, nil, "", "")
	require.NoError(t, err)
	assert.Equal(t, maxPageSize, limit)

	negative := -5
	limit, _, _, _, err = NormalizePaginationArgs(&negative, nil, "", "")
	require.NoError(t, err)
	assert.Equal(t, defaultPageSize, limit)
}

func TestNormalizePaginationInvalidCursor(t *testing.T) {
	_, _, _, _, err := NormalizePaginationArgs(nil, nil, "bad-cursor", "")
	assert.Error(t, err)
}

func TestBuildPageInfo(t *testing.T) {
	pi := BuildPageInfo(true, false, 1, 10)
	assert.True(t, pi.HasNextPage)
	assert.False(t, pi.HasPreviousPage)
	require.NotNil(t, pi.StartCursor)
	require.NotNil(t, pi.EndCursor)

	start, err := DecodeCursor(*pi.StartCursor)
	require.NoError(t, err)
	assert.Equal(t, uint(1), start)

	end, err := DecodeCursor(*pi.EndCursor)
	require.NoError(t, err)
	assert.Equal(t, uint(10), end)
}

func TestBuildPageInfoZeroIDs(t *testing.T) {
	pi := BuildPageInfo(false, false, 0, 0)
	assert.Nil(t, pi.StartCursor)
	assert.Nil(t, pi.EndCursor)
}
