package router

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"github.com/ulule/limiter/v3"
	"github.com/yourusername/kor-assetforge/config"
	_ "github.com/yourusername/kor-assetforge/docs"
	"github.com/yourusername/kor-assetforge/handlers"
	"github.com/yourusername/kor-assetforge/handlers/auth"
	handlersv2 "github.com/yourusername/kor-assetforge/handlers/v2"
	"github.com/yourusername/kor-assetforge/handlers/webhooks"
	"github.com/yourusername/kor-assetforge/middleware"
	"github.com/yourusername/kor-assetforge/models"
	"github.com/yourusername/kor-assetforge/services"
	"github.com/yourusername/kor-assetforge/utils"
	"github.com/yourusername/kor-assetforge/validator"
	"golang.org/x/time/rate"
	"gorm.io/gorm"
)

// SetupRouter initializes the HTTP router and registers all routes.
func SetupRouter(db *gorm.DB) *gin.Engine {
	// Initialize Stellar client
	stellarClient, err := config.InitStellarClient()
	if err != nil {
		log.Printf("Warning: Failed to initialize Stellar client: %v", err)
		stellarClient = nil
	}

	// Initialize Redis
	redisURL := os.Getenv("REDIS_URL")
	redisClient, err := utils.InitRedis(redisURL)
	if err != nil {
		log.Printf("Warning: Failed to initialize Redis, continuing without cache: %v", err)
		redisClient = nil
	}

	// Initialize advanced cache manager (wraps Redis with L1 + metrics)
	cacheManager := utils.NewCacheManager(redisClient)

	// Warm common cache entries on startup
	if redisClient != nil {
		go cacheManager.Warm(context.Background(), config.WarmCacheEntries(db))
	}

	// Initialize Redis-backed rate limiter (optional)
	var rateLimiterMiddleware gin.HandlerFunc
	if redisClient != nil {
		rl, err := handlers.NewRateLimiter(redisClient, limiter.Rate{
			Period: time.Minute,
			Limit:  100,
		})
		if err != nil {
			log.Printf("Warning: Failed to initialize rate limiter: %v", err)
		} else {
			rateLimiterMiddleware = rl.Middleware()
		}
	}
	_ = rateLimiterMiddleware // available for use on individual routes if needed

	// Setup authentication
	authConfig := &auth.AuthConfig{
		JWTSecret:          getEnvOrDefault("JWT_SECRET", "your-super-secret-jwt-key-change-in-production"),
		JWTExpirationHours: getEnvIntOrDefault("JWT_EXPIRATION_HOURS", 24),
		RefreshTokenHours:  getEnvIntOrDefault("REFRESH_TOKEN_HOURS", 168),
		EmailTokenHours:    getEnvIntOrDefault("EMAIL_TOKEN_HOURS", 24),
		PasswordResetHours: getEnvIntOrDefault("PASSWORD_RESET_HOURS", 1),
		BcryptCost:         getEnvIntOrDefault("BCRYPT_COST", 12),
	}
	emailService := services.NewEmailServiceFromEnv()
	authHandler := auth.NewAuthHandler(db, authConfig, emailService)
	authMiddleware := auth.NewAuthMiddleware(authConfig.JWTSecret)
	authRateLimiter := auth.NewAuthRateLimiter(rate.Limit(5.0/60.0), 10)

	// Setup router
	router := gin.New()

	if err := validator.Init(); err != nil {
		log.Printf("Warning: Failed to initialize validator: %v", err)
	}

	// Use custom enhanced middleware
	router.Use(
		handlers.RequestLogger(),
		handlers.GlobalErrorHandler(),
		middleware.RequestSizeLimiter(2<<20),
		middleware.RequireJSON(),
		middleware.RateLimit(20, time.Minute),
		middleware.CSRFProtection(os.Getenv("CSRF_SECRET")),
		middleware.VersionFromPath(), // attach api_version to every request context (#124)
	)

	// Health check handlers
	healthHandler := handlers.NewHealthHandler(db, redisClient, stellarClient)
	router.GET("/health", healthHandler.LivenessCheck)
	router.GET("/health/ready", healthHandler.ReadinessCheck)
	router.GET("/health/live", healthHandler.LivenessCheck)

	// Metrics endpoint
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Swagger documentation
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Cache metrics
	router.GET("/metrics/cache", middleware.CacheMetricsHandler(cacheManager))

	// API v1 routes
	v1 := router.Group("/api/v1")
	v1.Use(middleware.DeprecationWarning())
	{
		// Authentication routes (public)
		authGroup := v1.Group("/auth")
		authGroup.Use(authRateLimiter.GeneralAuthRateLimit())
		{
			authGroup.POST("/register", authRateLimiter.RegisterRateLimit(), authHandler.Register)
			authGroup.POST("/login", authRateLimiter.LoginRateLimit(), authHandler.Login)
			authGroup.POST("/refresh", authHandler.RefreshToken)
			authGroup.POST("/verify-email", authRateLimiter.EmailVerificationRateLimit(), authHandler.VerifyEmail)
			authGroup.POST("/forgot-password", authRateLimiter.PasswordResetRateLimit(), authHandler.ForgotPassword)
			authGroup.POST("/reset-password", authHandler.ResetPassword)
		}

		// Protected user routes
		protected := v1.Group("")
		protected.Use(authMiddleware.JWTAuth())
		{
			protected.GET("/profile", authHandler.GetProfile)
			protected.POST("/logout", authHandler.Logout)

			// 2FA routes
			protected.POST("/auth/2fa/setup", authHandler.Setup2FA)
			protected.POST("/auth/2fa/verify", authHandler.Verify2FA)
			protected.POST("/auth/2fa/disable", authHandler.Disable2FA)

			// Admin-only routes
			adminGroup := protected.Group("")
			adminGroup.Use(authMiddleware.RequireRole(models.RoleAdmin))
			{
				// Dispute admin endpoints
				adminGroup.PUT("/disputes/:id/review", func(c *gin.Context) {
					handlers.NewDisputeHandler(db).AdminReviewDispute(c)
				})
				adminGroup.PUT("/disputes/:id/resolve", func(c *gin.Context) {
					handlers.NewDisputeHandler(db).AdminResolveDispute(c)
				})
				// Staking admin endpoint
				adminGroup.POST("/staking/distribute", func(c *gin.Context) {
					handlers.NewStakingHandler(db).DistributeRewards(c)
				})
				// Dynamic fee configuration admin endpoints (#177)
				feeService := services.NewFeeService(db)
				feeAdminHandler := handlers.NewFeeAdminHandler(db, feeService)
				feeAdminGroup := adminGroup.Group("/fees")
				{
					feeAdminGroup.POST("", feeAdminHandler.CreateFeeConfig)
					feeAdminGroup.GET("", feeAdminHandler.ListFeeConfigs)
					feeAdminGroup.GET("/:id", feeAdminHandler.GetFeeConfig)
					feeAdminGroup.PUT("/:id", feeAdminHandler.UpdateFeeConfig)
					feeAdminGroup.DELETE("/:id", feeAdminHandler.DeactivateFeeConfig)
					feeAdminGroup.GET("/:id/audit", feeAdminHandler.GetFeeAuditLog)
				}
			}
		}

		// 2FA verification during login (unauthenticated)
		v1.POST("/auth/2fa/login", authHandler.LoginWith2FA)

		// Asset routes (with write-through cache invalidation)
		assetHandler := handlers.NewAssetHandler(db, stellarClient, redisClient, emailService)
		v1.POST("/assets/tokenize",
			middleware.InvalidateOnWrite(cacheManager, "kor:asset:*"),
			assetHandler.TokenizeAsset)
		v1.POST("/assets",
			middleware.InvalidateOnWrite(cacheManager, "kor:asset:*"),
			assetHandler.TokenizeAsset)
		v1.GET("/assets",
			middleware.HTTPCache(cacheManager, 5*time.Minute, "kor:asset", nil),
			assetHandler.ListAssets)
		v1.GET("/assets/:id",
			middleware.HTTPCache(cacheManager, 5*time.Minute, "kor:asset", nil),
			assetHandler.GetAsset)

		// NFT Metadata routes
		v1.POST("/assets/metadata",
			middleware.InvalidateOnWrite(cacheManager, "kor:asset:*"),
			assetHandler.UpdateMetadata)
		v1.GET("/assets/:id/metadata",
			middleware.HTTPCache(cacheManager, 5*time.Minute, "kor:asset", nil),
			assetHandler.GetMetadata)
		v1.POST("/assets/metadata/immutable",
			middleware.InvalidateOnWrite(cacheManager, "kor:asset:*"),
			assetHandler.MakeMetadataImmutable)

		// Oracle price feed routes
		v1.GET("/oracle/price",
			middleware.HTTPCache(cacheManager, 1*time.Minute, "kor:oracle", nil),
			assetHandler.GetOraclePrice)
		v1.GET("/assets/:id/oracle-price",
			middleware.HTTPCache(cacheManager, 1*time.Minute, "kor:oracle", nil),
			assetHandler.GetAssetOraclePrice)

		// Batch transaction routes (#106, #367, #370)
		batchHandler := handlers.NewBatchHandler(db, nil)
		v1.POST("/batch/execute",
			middleware.BatchValidator(),
			middleware.InvalidateOnWrite(cacheManager, "kor:asset:*"),
			assetHandler.ExecuteBatch)
		v1.POST("/batch/submit",
			middleware.BatchValidator(),
			authMiddleware.JWTAuth(),
			batchHandler.SubmitBatch)
		v1.POST("/batch/retry/:id",
			authMiddleware.JWTAuth(),
			batchHandler.RetryBatch)
		v1.POST("/batch/:id/retry",
			authMiddleware.JWTAuth(),
			batchHandler.RetryBatch)
		v1.GET("/batch/:id",
			middleware.HTTPCache(cacheManager, 1*time.Minute, "kor:batch", nil),
			assetHandler.GetBatchStatus)
		v1.POST("/batch/:id/rollback",
			middleware.InvalidateOnWrite(cacheManager, "kor:asset:*"),
			assetHandler.RollbackBatch)
		v1.GET("/batches",
			assetHandler.ListBatchTransactions)

		// Marketplace routes
		v1.POST("/marketplace/list",
			middleware.InvalidateOnWrite(cacheManager, "kor:asset:*"),
			assetHandler.ListAssetForSale)
		v1.POST("/marketplace/transfer",
			middleware.InvalidateOnWrite(cacheManager, "kor:asset:*"),
			assetHandler.TransferAsset)
		v1.GET("/transactions", assetHandler.ListTransactions)

		// Search routes
		searchBackend, _ := services.NewESSearchBackend(os.Getenv("ELASTICSEARCH_URL"), db)
		searchHandler := handlers.NewSearchHandler(searchBackend)
		v1.GET("/search/assets", searchHandler.Search)
		v1.GET("/search/suggestions", searchHandler.Suggest)
		v1.GET("/search/autocomplete", searchHandler.Autocomplete)
		v1.GET("/search/facets", searchHandler.Facets)
		v1.GET("/search/analytics", searchHandler.SearchAnalytics)
		v1.POST("/search/analytics/click", searchHandler.RecordSearchClick)

		// KYC / AML routes
		kycHandler := handlers.NewKYCHandler(db, nil, emailService, nil)
		v1.POST("/kyc/submit", kycHandler.SubmitKYC)
		v1.GET("/kyc/status", kycHandler.GetKYCStatus)
		v1.POST("/kyc/documents", kycHandler.UploadDocument)
		v1.POST("/kyc/aml/screen", kycHandler.ScreenAML)
		v1.POST("/kyc/accredited", kycHandler.VerifyAccreditedInvestor)
		v1.GET("/kyc/audit", kycHandler.GetAuditLog)
		v1.GET("/compliance/report", kycHandler.ComplianceReport)
		v1.POST("/kyc/aml/review/:id", kycHandler.ResolveAMLReview)

		// Beneficial ownership (UBO) routes
		complianceHandler := handlers.NewComplianceHandler(db)
		v1.POST("/compliance/ubo", complianceHandler.DeclareBeneficialOwner)
		v1.GET("/compliance/ubo", complianceHandler.ListBeneficialOwners)
		v1.POST("/compliance/ubo/:id/resolve", complianceHandler.ResolveUBOReview)
		v1.POST("/compliance/ubo/documents", complianceHandler.UploadCorporateStructureDocument)

		// Dispute resolution routes
		disputeHandler := handlers.NewDisputeHandler(db)
		v1.POST("/disputes", disputeHandler.FileDispute)
		v1.GET("/disputes", disputeHandler.ListDisputes)
		v1.GET("/disputes/history", disputeHandler.GetDisputeHistory)
		v1.GET("/disputes/:id", disputeHandler.GetDispute)

		// P2P secondary marketplace routes
		p2pHandler := handlers.NewP2PHandler(db)
		v1.POST("/p2p/orders", p2pHandler.CreateOrder)
		v1.GET("/p2p/orders", p2pHandler.ListOrders)
		v1.PUT("/p2p/orders/:id/cancel", p2pHandler.CancelOrder)
		v1.GET("/p2p/trades", p2pHandler.GetTradeHistory)
		v1.GET("/p2p/prices", p2pHandler.GetPriceChart)

		// Staking rewards routes
		stakingHandler := handlers.NewStakingHandler(db)
		v1.POST("/staking/stake", stakingHandler.Stake)
		v1.POST("/staking/unstake", stakingHandler.Unstake)
		v1.POST("/staking/claim", stakingHandler.ClaimRewards)
		v1.GET("/staking/dashboard", stakingHandler.GetStakingDashboard)
		v1.GET("/staking/rewards/history", stakingHandler.GetRewardHistory)

		// Liquidity pool routes
		liquidityHandler := handlers.NewLiquidityHandler(db)
		v1.POST("/liquidity/pools", liquidityHandler.CreatePool)
		v1.GET("/liquidity/pools", liquidityHandler.ListPools)
		v1.GET("/liquidity/pools/:id", liquidityHandler.GetPool)
		v1.GET("/liquidity/pools/:id/analytics", liquidityHandler.GetPoolAnalytics)
		v1.GET("/liquidity/pools/compare", liquidityHandler.ComparePools)
		v1.POST("/liquidity/add", liquidityHandler.AddLiquidity)
		v1.POST("/liquidity/remove", liquidityHandler.RemoveLiquidity)
		v1.POST("/liquidity/swap", liquidityHandler.Swap)
		v1.GET("/liquidity/positions", liquidityHandler.GetLPPositions)
		v1.GET("/liquidity/swaps", liquidityHandler.GetSwapHistory)

		// Incoming webhooks (#372)
		webhookHandler := handlers.NewWebhookHandler(db)
		router.POST("/webhooks/stellar-events", webhookHandler.HandleStellarEvent)
		router.POST("/webhooks/kyc", kycHandler.HandleKYCWebhook)

		emailAnalytics := services.NewEmailAnalyticsService(db)
		emailWebhookHandler := webhooks.NewEmailWebhookHandler(emailAnalytics, emailService.GetHealthChecker())
		router.POST("/webhooks/email/sendgrid", emailWebhookHandler.HandleSendGrid)
		router.POST("/webhooks/email/ses", emailWebhookHandler.HandleSES)
		router.POST("/webhooks/email", emailWebhookHandler.HandleGeneric)
		router.POST("/webhooks/email/generic", emailWebhookHandler.HandleGeneric)
		v1.GET("/email/dashboard", emailWebhookHandler.GetDashboard)
		v1.GET("/email/analytics", emailWebhookHandler.GetAnalytics)

		// Outgoing webhook subscription routes
		outgoingWebhookHandler := handlers.NewOutgoingWebhookHandler(db)
		webhookSubs := protected.Group("/webhooks/subscriptions")
		{
			webhookSubs.POST("", outgoingWebhookHandler.CreateSubscription)
			webhookSubs.GET("", outgoingWebhookHandler.ListSubscriptions)
			webhookSubs.PUT("/:id", outgoingWebhookHandler.UpdateSubscription)
			webhookSubs.DELETE("/:id", outgoingWebhookHandler.DeleteSubscription)
			webhookSubs.GET("/:id/logs", outgoingWebhookHandler.GetDeliveryLogs)
		}

		// Notification routes
		notificationHandler := handlers.NewNotificationHandler(db)
		notifGroup := protected.Group("/notifications")
		{
			notifGroup.GET("", notificationHandler.ListNotifications)
			notifGroup.GET("/unread-count", notificationHandler.UnreadCount)
			notifGroup.PUT("/read-all", notificationHandler.MarkAllRead)
			notifGroup.PUT("/:id/read", notificationHandler.MarkRead)
			notifGroup.GET("/preferences", notificationHandler.GetPreferences)
			notifGroup.PUT("/preferences", notificationHandler.UpdatePreference)
		}

		// Legal compliance routes
		legalHandler := handlers.NewLegalHandler(db)
		gdprExportService := services.NewDataExportService(db, emailService)
		gdprHandler := handlers.NewGDPRHandler(db, gdprExportService)

		legalGroup := v1.Group("/legal")
		{
			legalGroup.GET("/:type", legalHandler.GetActiveDocument)
			legalGroup.GET("/:type/versions", legalHandler.ListDocumentVersions)
			legalGroup.GET("/gdpr/export/download/:token", gdprHandler.DownloadExport)
		}
		legalProtected := protected.Group("/legal")
		{
			legalProtected.POST("/consent", legalHandler.RecordConsent)
			legalProtected.GET("/consent/history", legalHandler.GetConsentHistory)
			legalProtected.GET("/consent/pending", legalHandler.CheckPendingConsents)
			legalProtected.POST("/gdpr/export", gdprHandler.RequestDataExport)
			legalProtected.GET("/gdpr/export/:id", gdprHandler.GetDataExportStatus)
		}

		// Public fee endpoints - preview and active config (#177)
		feePublicHandler := handlers.NewFeeAdminHandler(db, services.NewFeeService(db))
		v1.GET("/fees/preview", feePublicHandler.PreviewFee)
		v1.GET("/fees/active", feePublicHandler.GetActiveFee)

		// Email template management routes (#373) — admin-only
		emailTplHandler := handlers.NewEmailTemplateHandler(db)
		emailTplGroup := protected.Group("/admin/email-templates")
		emailTplGroup.Use(authMiddleware.RequireRole(models.RoleAdmin))
		{
			emailTplGroup.GET("", emailTplHandler.ListTemplates)
			emailTplGroup.POST("", emailTplHandler.CreateTemplate)
			emailTplGroup.POST("/preview", emailTplHandler.Preview)
			emailTplGroup.POST("/render", emailTplHandler.RenderTemplate)
			emailTplGroup.GET("/:id", emailTplHandler.GetTemplate)
			emailTplGroup.PUT("/:id", emailTplHandler.UpdateTemplate)
			emailTplGroup.DELETE("/:id", emailTplHandler.DeleteTemplate)
			emailTplGroup.POST("/:id/activate", emailTplHandler.ActivateTemplate)
			emailTplGroup.GET("/:id/versions", emailTplHandler.ListVersions)
			emailTplGroup.GET("/:id/versions/diff", emailTplHandler.DiffVersions)
			emailTplGroup.POST("/:id/versions/:version/rollback", emailTplHandler.RollbackToVersion)
			emailTplGroup.GET("/:id/variants", emailTplHandler.ListVariants)
			emailTplGroup.POST("/:id/variants", emailTplHandler.CreateVariant)
		}
	}

	// API v2 routes
	v2 := router.Group("/api/v2")
	{
		v2AssetsHandler := handlersv2.NewAssetsHandler(db)
		v2.GET("/assets", v2AssetsHandler.ListAssets)
		v2.GET("/assets/:id", v2AssetsHandler.GetAsset)
	}

	// WebSocket routes
	wsHandler := handlers.NewWebSocketHandler(
		getEnvOrDefault("JWT_SECRET", "your-super-secret-jwt-key-change-in-production"),
		config.LoadWSRateLimitConfig(),
	)
	router.GET("/ws", wsHandler.HandleWS)
	router.GET("/ws/stats", wsHandler.HandleWSStats)

	// Pre-launch the hub
	_ = handlers.GetHub()

	// Initialize and start event indexer (#180)
	eventIndexer := services.NewEventIndexer(db)
	eventIndexer.Start()

	return router
}

func getEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

func getEnvIntOrDefault(key string, defaultValue int) int {
	if v := os.Getenv(key); v != "" {
		var i int
		_, err := log.Writer().Write([]byte("")) // dummy to avoid compiler import issues
		_ = err
		// We can parse safely
		for _, c := range v {
			if c >= '0' && c <= '9' {
				i = i*10 + int(c-'0')
			}
		}
		if i > 0 {
			return i
		}
	}
	return defaultValue
}
