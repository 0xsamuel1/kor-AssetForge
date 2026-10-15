package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yourusername/kor-assetforge/constants"
)

func TestLoadBatchConfig_Defaults(t *testing.T) {
	os.Unsetenv("MAX_BATCH_SIZE")
	os.Unsetenv("BATCH_MAX_RETRIES")
	os.Unsetenv("BATCH_INITIAL_BACKOFF")
	os.Unsetenv("BATCH_MAX_BACKOFF")

	cfg := LoadBatchConfig()
	assert.Equal(t, constants.MAX_BATCH_SIZE, cfg.MaxBatchSize)
	assert.Equal(t, 3, cfg.MaxRetries)
	assert.Equal(t, 2*time.Second, cfg.InitialBackoff)
	assert.Equal(t, 60*time.Second, cfg.MaxBackoff)
	assert.Equal(t, constants.MAX_BATCH_SIZE, GetMaxBatchSize())
}

func TestLoadBatchConfig_CustomEnv(t *testing.T) {
	os.Setenv("MAX_BATCH_SIZE", "25")
	os.Setenv("BATCH_MAX_RETRIES", "5")
	os.Setenv("BATCH_INITIAL_BACKOFF", "2s")
	os.Setenv("BATCH_MAX_BACKOFF", "30s")
	defer func() {
		os.Unsetenv("MAX_BATCH_SIZE")
		os.Unsetenv("BATCH_MAX_RETRIES")
		os.Unsetenv("BATCH_INITIAL_BACKOFF")
		os.Unsetenv("BATCH_MAX_BACKOFF")
	}()

	cfg := LoadBatchConfig()
	assert.Equal(t, 25, cfg.MaxBatchSize)
	assert.Equal(t, 5, cfg.MaxRetries)
	assert.Equal(t, 2*time.Second, cfg.InitialBackoff)
	assert.Equal(t, 30*time.Second, cfg.MaxBackoff)
	assert.Equal(t, 25, GetMaxBatchSize())
}
