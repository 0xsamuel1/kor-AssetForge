package constants

// Global operational constants for kor-AssetForge

const (
	// MAX_BATCH_SIZE is the global maximum number of operations allowed in a single batch transaction.
	MaxBatchSize = 50
	MAX_BATCH_SIZE = MaxBatchSize

	// DefaultMaxBatchSize provides a default fallback for batch size limits.
	DefaultMaxBatchSize = 50
	DefaultServerPort = "8080"
	DefaultRequestSize = 2 << 20
	GlobalRateLimit = 20
	GlobalRateLimitWindowMinutes = 1
	AssetCacheTTLMinutes = 5
	OracleCacheTTLMinutes = 1
)

