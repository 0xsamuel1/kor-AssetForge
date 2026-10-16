package config

import (
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/yourusername/kor-assetforge/constants"
)

// BatchConfig holds configuration for batch processing, validation, and retry logic.
type BatchConfig struct {
	MaxBatchSize      int           `json:"max_batch_size"`
	MaxRetries        int           `json:"max_retries"`
	InitialBackoff    time.Duration `json:"initial_backoff"`
	MaxBackoff        time.Duration `json:"max_backoff"`
	BackoffMultiplier float64       `json:"backoff_multiplier"`
}

var (
	batchConfigMu      sync.RWMutex
	currentBatchConfig BatchConfig
	batchConfigOnce    sync.Once
)

func initBatchConfig() {
	maxBatchSize := constants.MAX_BATCH_SIZE
	if val := os.Getenv("MAX_BATCH_SIZE"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
			maxBatchSize = parsed
		}
	}

	maxRetries := 3
	if val := os.Getenv("BATCH_MAX_RETRIES"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil && parsed >= 0 {
			maxRetries = parsed
		}
	}

	initialBackoff := 2 * time.Second
	if val := os.Getenv("BATCH_INITIAL_BACKOFF"); val != "" {
		if parsed, err := time.ParseDuration(val); err == nil && parsed > 0 {
			initialBackoff = parsed
		}
	}

	maxBackoff := 60 * time.Second
	if val := os.Getenv("BATCH_MAX_BACKOFF"); val != "" {
		if parsed, err := time.ParseDuration(val); err == nil && parsed > 0 {
			maxBackoff = parsed
		}
	}

	currentBatchConfig = BatchConfig{
		MaxBatchSize:      maxBatchSize,
		MaxRetries:        maxRetries,
		InitialBackoff:    initialBackoff,
		MaxBackoff:        maxBackoff,
		BackoffMultiplier: 2.0,
	}
}

// LoadBatchConfig loads the batch configuration from environment variables or defaults.
func LoadBatchConfig() BatchConfig {
	batchConfigMu.Lock()
	defer batchConfigMu.Unlock()
	initBatchConfig()
	return currentBatchConfig
}

// GetMaxBatchSize returns the current active maximum batch size limit.
func GetMaxBatchSize() int {
	batchConfigOnce.Do(initBatchConfig)
	batchConfigMu.RLock()
	defer batchConfigMu.RUnlock()
	return currentBatchConfig.MaxBatchSize
}

// SetMaxBatchSize dynamically updates the maximum batch size limit for runtime configuration.
func SetMaxBatchSize(size int) {
	batchConfigOnce.Do(initBatchConfig)
	if size <= 0 {
		return
	}
	batchConfigMu.Lock()
	defer batchConfigMu.Unlock()
	currentBatchConfig.MaxBatchSize = size
}

// ResetBatchConfig resets batch configuration back to defaults (primarily for testing).
func ResetBatchConfig() {
	batchConfigMu.Lock()
	defer batchConfigMu.Unlock()
	initBatchConfig()
}
