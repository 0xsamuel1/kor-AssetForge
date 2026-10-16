package config

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/yourusername/kor-assetforge/migrations"
	"github.com/yourusername/kor-assetforge/models"
	"github.com/yourusername/kor-assetforge/utils"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// DBPoolConfig keeps production pool sizing explicit and environment-driven.
// Defaults fit a small API instance and should be benchmarked per deployment.
type DBPoolConfig struct {
	MaxOpen      int
	MaxIdle      int
	MaxLifetime  time.Duration
	MaxIdleTime  time.Duration
	ConnectRetry int
	ConnectWait  time.Duration
}

func LoadDBPoolConfig() DBPoolConfig {
	return DBPoolConfig{
		MaxOpen: envInt("DB_MAX_OPEN_CONNS", 25), MaxIdle: envInt("DB_MAX_IDLE_CONNS", 10),
		MaxLifetime: envDuration("DB_CONN_MAX_LIFETIME", 30*time.Minute), MaxIdleTime: envDuration("DB_CONN_MAX_IDLE_TIME", 5*time.Minute),
		ConnectRetry: envInt("DB_CONNECT_RETRIES", 3), ConnectWait: envDuration("DB_CONNECT_RETRY_WAIT", time.Second),
	}
}

func (c DBPoolConfig) apply(db *sql.DB) {
	db.SetMaxOpenConns(c.MaxOpen)
	db.SetMaxIdleConns(c.MaxIdle)
	db.SetConnMaxLifetime(c.MaxLifetime)
	db.SetConnMaxIdleTime(c.MaxIdleTime)
}

func envInt(key string, fallback int) int {
	if value, err := strconv.Atoi(os.Getenv(key)); err == nil && value > 0 {
		return value
	}
	return fallback
}
func envDuration(key string, fallback time.Duration) time.Duration {
	if value, err := time.ParseDuration(os.Getenv(key)); err == nil && value > 0 {
		return value
	}
	return fallback
}

// InitDB initializes the database connection, runs SQL migrations, then GORM AutoMigrate.
func InitDB() (*gorm.DB, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "host=localhost user=postgres password=password dbname=assetforge port=5432 sslmode=disable"
	}

	// Run versioned SQL migrations first
	rawDB, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	poolConfig := LoadDBPoolConfig()
	poolConfig.apply(rawDB)
	if err := pingWithRetry(rawDB, poolConfig); err != nil {
		rawDB.Close()
		return nil, err
	}
	defer rawDB.Close()

	if err := migrations.New(rawDB).Up(); err != nil {
		return nil, fmt.Errorf("failed to apply migrations: %w", err)
	}

	// Open GORM connection for the rest of the application
	var db *gorm.DB
	for attempt := 0; attempt < poolConfig.ConnectRetry; attempt++ {
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err == nil {
			var sqlDB *sql.DB
			sqlDB, err = db.DB()
			if err == nil {
				poolConfig.apply(sqlDB)
				err = pingWithRetry(sqlDB, DBPoolConfig{ConnectRetry: 1, ConnectWait: poolConfig.ConnectWait})
			}
		}
		if err == nil {
			break
		}
		if attempt+1 < poolConfig.ConnectRetry {
			time.Sleep(poolConfig.ConnectWait * time.Duration(attempt+1))
		}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database after retries: %w", err)
	}

	// GORM AutoMigrate handles columns added after the initial SQL migration
	if err := db.AutoMigrate(
		&models.Asset{},
		&models.Listing{},
		&models.Transaction{},
		&models.User{},
		&models.UserBalance{},
		&models.UserSession{},
		// KYC / AML models (#55)
		&models.KYCRecord{},
		&models.KYCDocument{},
		&models.AMLScreening{},
		&models.ComplianceAuditLog{},
		// Batch transaction model (#106)
		&models.BatchTransaction{},
		// Event indexer models (#180)
		&models.IndexedEvent{},
		&models.EventCheckpoint{},
		// GDPR export models (#178)
		&models.ExportJob{},
		// Dynamic fee config models (#177)
		&models.FeeConfig{},
		&models.FeeAuditLog{},
		// Approval workflow models
		&models.ApprovalWorkflow{},
		&models.ApprovalStep{},
		&models.ApprovalRequest{},
		&models.ApprovalAction{},
		&models.AuditLog{},
		// Taxonomy models (#159)
		&models.Category{},
		&models.AssetCategory{},
		&models.Tag{},
		&models.AssetTag{},
		// Watchlist models (#160)
		&models.Watchlist{},
		&models.WatchlistItem{},
		// User activity model (#161)
		&models.UserActivity{},
		// Recommendation history powers personalized search ranking.
		&models.UserInteraction{},
		&models.UserPreference{},
		&models.AssetRecommendation{},
		// Email template models (#163)
		&models.EmailTemplate{},
		&models.EmailTemplateVersion{},
		&models.EmailTemplateVariant{},
		// Asset comparison history (#167)
		&models.ComparisonHistory{},
		// Performance metrics models (#169)
		&models.AssetDividend{},
		&models.PerformanceMetric{},
		// Referral program models (#170)
		&models.ReferralCode{},
		&models.Referral{},
		&models.ReferralReward{},
	); err != nil {
		return nil, fmt.Errorf("failed to auto-migrate models: %w", err)
	}

	return db, nil
}

func pingWithRetry(db *sql.DB, config DBPoolConfig) error {
	attempts := config.ConnectRetry
	if attempts < 1 {
		attempts = 1
	}
	for attempt := 0; attempt < attempts; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := db.PingContext(ctx)
		cancel()
		if err == nil {
			return nil
		}
		if attempt+1 == attempts {
			return fmt.Errorf("database health check failed: %w", err)
		}
		time.Sleep(config.ConnectWait * time.Duration(attempt+1))
	}
	return nil
}

// InitStellarClient initializes the Stellar client.
func InitStellarClient() (*utils.StellarClient, error) {
	horizonURL := os.Getenv("STELLAR_HORIZON_URL")
	networkType := os.Getenv("STELLAR_NETWORK")
	if networkType == "" {
		networkType = "testnet"
	}
	return utils.NewStellarClient(horizonURL, networkType)
}

// WarmCacheEntries returns the list of keys to pre-populate on startup (#56).
// Loaders hit the database and the results are stored in the cache manager.
func WarmCacheEntries(db *gorm.DB) []utils.WarmEntry {
	return []utils.WarmEntry{
		{
			Key: "kor:asset:list:page1",
			TTL: 5 * time.Minute,
			Loader: func() (interface{}, error) {
				var assets []models.Asset
				if err := db.Order("created_at desc").Limit(10).Find(&assets).Error; err != nil {
					return nil, err
				}
				var total int64
				db.Model(&models.Asset{}).Count(&total)
				return map[string]interface{}{
					"limit": 10,
					"page":  1,
					"total": total,
					"data":  assets,
				}, nil
			},
		},
	}
}
