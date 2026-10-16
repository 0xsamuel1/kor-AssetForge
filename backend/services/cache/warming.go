package cache

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"
)

// WarmTarget defines a specific cache target to pre-populate.
type WarmTarget struct {
	Key      string
	TTL      time.Duration
	Priority int // 1 = highest, 5 = lowest
	Loader   func(ctx context.Context) (interface{}, error)
}

// WarmingStats records metrics on warming executions.
type WarmingStats struct {
	TotalTargets int           `json:"total_targets"`
	WarmedKeys   int           `json:"warmed_keys"`
	SkippedKeys  int           `json:"skipped_keys"`
	FailedKeys   int           `json:"failed_keys"`
	Duration     time.Duration `json:"duration"`
	StartedAt    time.Time     `json:"started_at"`
	CompletedAt  time.Time     `json:"completed_at"`
}

// CacheWarmer coordinates asynchronous or standalone cache pre-population.
type CacheWarmer struct {
	cache       CacheService
	concurrency int
}

// NewCacheWarmer creates a new warmer with worker pool concurrency limit.
func NewCacheWarmer(cache CacheService, concurrency int) *CacheWarmer {
	if concurrency <= 0 {
		concurrency = 4
	}
	return &CacheWarmer{
		cache:       cache,
		concurrency: concurrency,
	}
}

// WarmAsync runs the warming plan in a background goroutine so server startup is non-blocking.
func (cw *CacheWarmer) WarmAsync(ctx context.Context, targets []WarmTarget, onComplete func(stats WarmingStats)) {
	go func() {
		stats := cw.WarmSync(ctx, targets)
		if onComplete != nil {
			onComplete(stats)
		}
	}()
}

// WarmSync executes cache warming synchronously, respecting priority and worker concurrency.
func (cw *CacheWarmer) WarmSync(ctx context.Context, targets []WarmTarget) WarmingStats {
	start := time.Now()
	stats := WarmingStats{
		TotalTargets: len(targets),
		StartedAt:    start,
	}

	if len(targets) == 0 {
		stats.CompletedAt = time.Now()
		stats.Duration = stats.CompletedAt.Sub(start)
		return stats
	}

	targetChan := make(chan WarmTarget, len(targets))
	for _, t := range targets {
		targetChan <- t
	}
	close(targetChan)

	var mu sync.Mutex
	var wg sync.WaitGroup

	workers := cw.concurrency
	if workers > len(targets) {
		workers = len(targets)
	}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for target := range targetChan {
				select {
				case <-ctx.Done():
					mu.Lock()
					stats.FailedKeys++
					mu.Unlock()
					return
				default:
				}

				// Check if already warm in cache
				if _, found, _ := cw.cache.Get(ctx, target.Key); found {
					mu.Lock()
					stats.SkippedKeys++
					mu.Unlock()
					continue
				}

				data, err := target.Loader(ctx)
				if err != nil {
					log.Printf("CacheWarmer: Loader failed for key %s: %v", target.Key, err)
					mu.Lock()
					stats.FailedKeys++
					mu.Unlock()
					continue
				}

				rawBytes, err := json.Marshal(data)
				if err != nil {
					log.Printf("CacheWarmer: Marshal failed for key %s: %v", target.Key, err)
					mu.Lock()
					stats.FailedKeys++
					mu.Unlock()
					continue
				}

				if err := cw.cache.Set(ctx, target.Key, rawBytes, target.TTL); err != nil {
					log.Printf("CacheWarmer: Set failed for key %s: %v", target.Key, err)
					mu.Lock()
					stats.FailedKeys++
					mu.Unlock()
					continue
				}

				mu.Lock()
				stats.WarmedKeys++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	stats.CompletedAt = time.Now()
	stats.Duration = stats.CompletedAt.Sub(start)
	log.Printf("CacheWarmer completed: %d warmed, %d skipped, %d failed in %s",
		stats.WarmedKeys, stats.SkippedKeys, stats.FailedKeys, stats.Duration)

	return stats
}
