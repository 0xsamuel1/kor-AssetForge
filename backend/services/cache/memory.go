package cache

import (
	"sync/atomic"
)

// MemoryStats tracks in-memory L1 cache memory consumption and metrics.
type MemoryStats struct {
	TotalEntries   atomic.Int64
	UsedSizeBytes  atomic.Int64
	MaxSizeBytes   int64
	MaxEntries     int
	TotalEvictions atomic.Int64
	TotalHits      atomic.Int64
	TotalMisses    atomic.Int64
}

// Snapshot returns a point-in-time snapshot of the cache memory metrics.
func (m *MemoryStats) Snapshot() map[string]interface{} {
	usedBytes := m.UsedSizeBytes.Load()
	entries := m.TotalEntries.Load()
	hits := m.TotalHits.Load()
	misses := m.TotalMisses.Load()
	total := hits + misses

	var hitRate float64
	if total > 0 {
		hitRate = float64(hits) / float64(total)
	}

	var usagePct float64
	if m.MaxSizeBytes > 0 {
		usagePct = (float64(usedBytes) / float64(m.MaxSizeBytes)) * 100.0
	}

	return map[string]interface{}{
		"total_entries":    entries,
		"max_entries":      m.MaxEntries,
		"used_size_bytes":  usedBytes,
		"max_size_bytes":   m.MaxSizeBytes,
		"usage_pct":        usagePct,
		"total_evictions":  m.TotalEvictions.Load(),
		"total_hits":       hits,
		"total_misses":     misses,
		"hit_rate":         hitRate,
	}
}

// EstimateEntrySize calculates the approximate memory size in bytes of a key and value.
func EstimateEntrySize(key string, val []byte) int64 {
	// Base pointer overhead + string header (16) + slice header (24) + payload bytes
	return int64(len(key) + len(val) + 64)
}
