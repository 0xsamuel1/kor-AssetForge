package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/yourusername/kor-assetforge/config"
	"github.com/yourusername/kor-assetforge/models"
	"github.com/yourusername/kor-assetforge/services/cache"
	"github.com/yourusername/kor-assetforge/utils"
	"gorm.io/gorm"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using system environment variables")
	}

	concurrencyFlag := flag.Int("concurrency", 4, "Number of concurrent warming workers")
	targetFlag := flag.String("target", "all", "Warming target group: all, assets, taxonomy, metrics")
	timeoutFlag := flag.Duration("timeout", 2*time.Minute, "Timeout for overall cache warming run")
	flag.Parse()

	log.Printf("Starting AssetForge Cache Warmer (target=%s, concurrency=%d)...", *targetFlag, *concurrencyFlag)

	// Initialize Database
	db, err := config.InitDB()
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// Initialize Redis
	redisURL := os.Getenv("REDIS_URL")
	redisClient, err := utils.InitRedis(redisURL)
	if err != nil {
		log.Printf("Warning: Failed to connect to Redis: %v. Running in degraded L1 mode.", err)
		redisClient = nil
	} else {
		defer redisClient.Close()
	}

	cacheConfig := config.LoadCacheConfig()
	twoTier := cache.NewTwoTierCache(
		cacheConfig.L1MaxEntries,
		cacheConfig.L1MaxSizeBytes,
		redisClient,
		cacheConfig.RedisCircuitBreakerThreshold,
		cacheConfig.RedisCircuitBreakerTimeout,
		cacheConfig.RedisFallbackEnabled,
	)

	warmer := cache.NewCacheWarmer(twoTier, *concurrencyFlag)
	targets := buildWarmingTargets(db, *targetFlag)

	ctx, cancel := context.WithTimeout(context.Background(), *timeoutFlag)
	defer cancel()

	stats := warmer.WarmSync(ctx, targets)

	fmt.Println("\n==========================================")
	fmt.Println("       CACHE WARMING EXECUTION REPORT     ")
	fmt.Println("==========================================")
	fmt.Printf("Total Targets:   %d\n", stats.TotalTargets)
	fmt.Printf("Keys Warmed:     %d\n", stats.WarmedKeys)
	fmt.Printf("Keys Skipped:    %d (already warm)\n", stats.SkippedKeys)
	fmt.Printf("Keys Failed:     %d\n", stats.FailedKeys)
	fmt.Printf("Duration:        %s\n", stats.Duration)
	fmt.Printf("Started At:      %s\n", stats.StartedAt.Format(time.RFC3339))
	fmt.Printf("Completed At:    %s\n", stats.CompletedAt.Format(time.RFC3339))
	fmt.Println("==========================================")

	if stats.FailedKeys > 0 {
		os.Exit(1)
	}
}

func buildWarmingTargets(db *gorm.DB, group string) []cache.WarmTarget {
	var targets []cache.WarmTarget

	if group == "all" || group == "assets" {
		// Target 1: Top 20 Recent Assets (page 1)
		targets = append(targets, cache.WarmTarget{
			Key:      "kor:asset:list:page1",
			TTL:      10 * time.Minute,
			Priority: 1,
			Loader: func(ctx context.Context) (interface{}, error) {
				var assets []models.Asset
				if err := db.WithContext(ctx).Order("created_at desc").Limit(20).Find(&assets).Error; err != nil {
					return nil, err
				}
				var total int64
				db.WithContext(ctx).Model(&models.Asset{}).Count(&total)
				return map[string]interface{}{
					"limit": 20,
					"page":  1,
					"total": total,
					"data":  assets,
				}, nil
			},
		})

		// Target 2: Top Active Listings
		targets = append(targets, cache.WarmTarget{
			Key:      "kor:listings:active:page1",
			TTL:      5 * time.Minute,
			Priority: 2,
			Loader: func(ctx context.Context) (interface{}, error) {
				var listings []models.Listing
				if err := db.WithContext(ctx).Where("status = ?", "active").Order("created_at desc").Limit(20).Find(&listings).Error; err != nil {
					return nil, err
				}
				return listings, nil
			},
		})
	}

	if group == "all" || group == "taxonomy" {
		// Target 3: Asset Categories
		targets = append(targets, cache.WarmTarget{
			Key:      "kor:categories:all",
			TTL:      30 * time.Minute,
			Priority: 2,
			Loader: func(ctx context.Context) (interface{}, error) {
				var categories []models.Category
				if err := db.WithContext(ctx).Find(&categories).Error; err != nil {
					return nil, err
				}
				return categories, nil
			},
		})

		// Target 4: Asset Tags
		targets = append(targets, cache.WarmTarget{
			Key:      "kor:tags:all",
			TTL:      30 * time.Minute,
			Priority: 3,
			Loader: func(ctx context.Context) (interface{}, error) {
				var tags []models.Tag
				if err := db.WithContext(ctx).Find(&tags).Error; err != nil {
					return nil, err
				}
				return tags, nil
			},
		})
	}

	if group == "all" || group == "metrics" {
		// Target 5: Active Fee Configuration
		targets = append(targets, cache.WarmTarget{
			Key:      "kor:fee:active",
			TTL:      1 * time.Hour,
			Priority: 1,
			Loader: func(ctx context.Context) (interface{}, error) {
				var fee models.FeeConfig
				if err := db.WithContext(ctx).Where("is_active = ?", true).First(&fee).Error; err != nil {
					if err == gorm.ErrRecordNotFound {
						return map[string]interface{}{"status": "none"}, nil
					}
					return nil, err
				}
				return fee, nil
			},
		})
	}

	return targets
}
