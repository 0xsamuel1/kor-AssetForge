package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"github.com/ulule/limiter/v3"
	"github.com/yourusername/kor-assetforge/config"
	"github.com/yourusername/kor-assetforge/constants"
	_ "github.com/yourusername/kor-assetforge/docs"
	"github.com/yourusername/kor-assetforge/handlers"
	"github.com/yourusername/kor-assetforge/handlers/auth"
	handlersv2 "github.com/yourusername/kor-assetforge/handlers/v2"
	"github.com/yourusername/kor-assetforge/handlers/webhooks"
	"github.com/yourusername/kor-assetforge/middleware"
	"github.com/yourusername/kor-assetforge/models"
	"github.com/yourusername/kor-assetforge/monitoring"
	"github.com/yourusername/kor-assetforge/services"
	"github.com/yourusername/kor-assetforge/utils"
	"github.com/yourusername/kor-assetforge/validator"
	"golang.org/x/time/rate"
)

// @title kor-AssetForge API
// @version 0.1.0
// @description Decentralized marketplace for tokenizing and trading real-world assets on Stellar.
// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.url http://www.swagger.io/support
// @contact.email support@swagger.io

// @license.name MIT
// @license.url https://opensource.org/licenses/MIT

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization

// @host localhost:8080
// @BasePath /api/v1
func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using system environment variables")
	}

	// Initialize database
	db, err := config.InitDB()
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	if poolMetrics, err := monitoring.NewDBPoolMetrics(db); err != nil {
		log.Printf("Warning: database pool metrics unavailable: %v", err)
	} else {
		poolMetrics.Start(context.Background())
	}
	auditService := middleware.NewAuditService(db)
	middleware.StartAuditMaintenance(context.Background(), auditService)

	// Initialize Stellar client
	stellarClient, err := config.InitStellarClient()
	if err != nil {
		log.Fatalf("Failed to initialize Stellar client: %v", err)
	}

	// Initialize Redis
	redisURL := os.Getenv("REDIS_URL")
	redisClient, err := utils.InitRedis(redisURL)
	if err != nil {
		log.Printf("Warning: Failed to initialize Redis, continuing without cache: %v", err)
		redisClient = nil
	} else {
		defer redisClient.Close()
	}

	// Initialize advanced cache manager (wraps Redis with L1 + metrics)
	cacheManager := utils.NewCacheManager(redisClient)

	// Warm common cache entries on startup
	go cacheManager.Warm(context.Background(), config.WarmCacheEntries(db))

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
	// The limiter is attached to routes that opt into Redis-backed throttling.
	if rateLimiterMiddleware != nil {
		log.Printf("Redis-backed rate limiter initialized")
	}

	// Setup authentication
	authConfig := &auth.AuthConfig{
 refactor/backend-structure-error-handling-291-294
		JWTSecret:          config.EnvOrDefault("JWT_SECRET", "your-super-secret-jwt-key-change-in-production"),
		JWTExpirationHours: config.EnvIntOrDefault("JWT_EXPIRATION_HOURS", 24),
		RefreshTokenHours:  config.EnvIntOrDefault("REFRESH_TOKEN_HOURS", 168),
		EmailTokenHours:    config.EnvIntOrDefault("EMAIL_TOKEN_HOURS", 24),
		PasswordResetMinutes: config.EnvIntOrDefault("PASSWORD_RESET_HOURS", 30),
		BcryptCost:         config.EnvIntOrDefault("BCRYPT_COST", 12),

		JWTSecret:            getEnvOrDefault("JWT_SECRET", "your-super-secret-jwt-key-change-in-production"),
		JWTExpirationHours:   getEnvIntOrDefault("JWT_EXPIRATION_HOURS", 24),
		RefreshTokenHours:    getEnvIntOrDefault("REFRESH_TOKEN_HOURS", 168),
		EmailTokenHours:      getEnvIntOrDefault("EMAIL_TOKEN_HOURS", 24),
		PasswordResetMinutes: getEnvIntOrDefault("PASSWORD_RESET_HOURS", 30),
		BcryptCost:           getEnvIntOrDefault("BCRYPT_COST", 12),
 main
	}
	emailService := services.NewEmailServiceFromEnv()
	reportScheduler := services.NewReportSchedulerService(db, emailService)
	if err := reportScheduler.Start(context.Background()); err != nil {
		log.Printf("Warning: report scheduler failed to start: %v", err)
	}
	tracerProvider, err := monitoring.InitializeTracing(context.Background(), "kor-assetforge-api", os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	if err != nil {
		log.Printf("Warning: tracing disabled: %v", err)
	} else {
		defer monitoring.ShutdownTracing(context.Background(), tracerProvider)
	}
	authHandler := auth.NewAuthHandler(db, authConfig, emailService)
	authMiddleware := auth.NewAuthMiddleware(authConfig.JWTSecret)
	authRateLimiter := auth.NewAuthRateLimiter(rate.Limit(5.0/60.0), 10)
	registrationConfig := config.LoadRegistrationRateLimitConfig()
	registrationRateLimiter := auth.NewRegistrationRateLimiter(registrationConfig.MaxPerWindow, registrationConfig.Window)

	// Setup router
	router := gin.New()

	if err := validator.Init(); err != nil {
		log.Fatalf("Failed to initialize validator: %v", err)
	}

	// Use custom enhanced middleware
	router.Use(
		middleware.VersionNegotiation(),
		middleware.TracingMiddleware("kor-assetforge-api"),
		handlers.RequestLogger(),
		middleware.DetectLanguage(), // resolve request language for i18n (#185)
		handlers.GlobalErrorHandler(),
		handlers.MetricsMiddleware(),
		handlers.BusinessMetricsMiddleware(nil),
 refactor/backend-structure-error-handling-291-294
		middleware.RequestSizeLimiter(constants.DefaultRequestSize),

		middleware.RequestSizeLimiter(2<<20),
		middleware.SecurityHeaders(config.LoadSecurityHeadersConfig()),
 main
		middleware.RequireJSON(),
		middleware.RateLimit(constants.GlobalRateLimit, constants.GlobalRateLimitWindowMinutes*time.Minute),
		middleware.CSRFProtection(os.Getenv("CSRF_SECRET")),
		middleware.VersionFromPath(), // attach api_version to every request context (#124)
		middleware.AuditMiddleware(auditService),
	)

	// Health check handlers
	healthHandler := handlers.NewHealthHandler(db, redisClient, stellarClient)
	router.GET("/health", healthHandler.LivenessCheck)
	router.GET("/health/ready", healthHandler.ReadinessCheck)
	router.GET("/health/live", healthHandler.LivenessCheck)

	// Metrics endpoint
	// @Summary Prometheus metrics
	// @Description Get service metrics in Prometheus format
	// @Tags monitoring
	// @Produce plain
	// @Success 200 {string} string "Prometheus metrics"
	// @Router /metrics [get]
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Swagger documentation
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	router.GET("/api/version/migration-guide", middleware.MigrationGuide)

	// Cache metrics
	router.GET("/metrics/cache", middleware.CacheMetricsHandler(cacheManager))

	// API v1 routes (deprecated — Deprecation + Sunset headers injected on all responses)
	v1 := router.Group("/api/v1")
	v1.Use(middleware.DeprecationWarning())
	{
		// Authentication routes (public)
		authGroup := v1.Group("/auth")
		authGroup.Use(authRateLimiter.GeneralAuthRateLimit())
		{
			authGroup.POST("/register", registrationRateLimiter.RegisterRateLimit(), middleware.CaptchaAfter(registrationConfig.CaptchaAfter, registrationConfig.Window), authHandler.Register)
			authGroup.POST("/login", authRateLimiter.LoginRateLimit(), authHandler.Login)
			authGroup.POST("/refresh", authHandler.RefreshToken)
			authGroup.POST("/verify-email", authRateLimiter.EmailVerificationRateLimit(), authHandler.VerifyEmail)
			authGroup.POST("/forgot-password", authRateLimiter.PasswordResetRateLimit(), authHandler.ForgotPassword)
			authGroup.POST("/reset-password", authHandler.ResetPassword)
		}

		// OAuth social login routes (#183)
		oauthService := services.NewOAuthService()
		oauthHandler := auth.NewOAuthHandler(db, authConfig, oauthService)
		oauthGroup := v1.Group("/auth/oauth")
		{
			oauthGroup.GET("/:provider/url", oauthHandler.GetAuthURL)
			oauthGroup.POST("/:provider/callback", oauthHandler.Callback)
		}

		// Protected user routes
		protected := v1.Group("")
		protected.Use(authMiddleware.JWTAuth())
		adminGroup := protected.Group("")
		adminGroup.Use(authMiddleware.RequireRole(models.RoleAdmin))
		{
			protected.GET("/profile", authHandler.GetProfile)
			protected.POST("/logout", authHandler.Logout)

			// 2FA routes
			protected.POST("/auth/2fa/setup", authHandler.Setup2FA)
			protected.POST("/auth/2fa/verify", authHandler.Verify2FA)
			protected.POST("/auth/2fa/disable", authHandler.Disable2FA)

			// 2FA recovery code routes (#182)
			protected.POST("/auth/2fa/recovery-codes", authHandler.GenerateRecoveryCodes)
			protected.GET("/auth/2fa/recovery-codes", authHandler.GetRecoveryCodesStatus)

			// OAuth account linking routes (#183)
			protected.GET("/auth/oauth/linked", oauthHandler.ListLinkedAccounts)
			protected.POST("/auth/oauth/:provider/link", oauthHandler.LinkAccount)
			protected.DELETE("/auth/oauth/:provider/unlink", oauthHandler.UnlinkAccount)

			// Admin-only routes
			{
				// Dispute admin endpoints — handlers declared below, referenced via closures
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

				// Fee distribution admin endpoints (#181)
				feeDistributionHandler := handlers.NewFeeDistributionAdminHandler(db)
				feeDistributionAdmin := adminGroup.Group("/fee-distribution")
				{
					feeDistributionAdmin.POST("/rules", feeDistributionHandler.CreateRule)
					feeDistributionAdmin.GET("/rules", feeDistributionHandler.ListRules)
					feeDistributionAdmin.POST("/rules/:id/activate", feeDistributionHandler.ActivateRule)
					feeDistributionAdmin.POST("/runs", feeDistributionHandler.RunDistribution)
					feeDistributionAdmin.GET("/runs", feeDistributionHandler.ListRuns)
					feeDistributionAdmin.GET("/runs/:id", feeDistributionHandler.GetRun)
				}

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
		v1.POST("/auth/2fa/recovery-codes/login", authHandler.LoginWithRecoveryCode)

		// Asset routes (with write-through cache invalidation)
		workflowService := services.NewWorkflowService(db, emailService)
		assetHandler := handlers.NewAssetHandler(db, stellarClient, redisClient, emailService, workflowService)
		v1.POST("/assets/tokenize",
			middleware.InvalidateOnWrite(cacheManager, "kor:asset:*"),
			middleware.InvalidateOnWriteTags(cacheManager, "asset"),
			assetHandler.TokenizeAsset)
		v1.POST("/assets",
			middleware.InvalidateOnWrite(cacheManager, "kor:asset:*"),
			middleware.InvalidateOnWriteTags(cacheManager, "asset"),
			assetHandler.TokenizeAsset)
		v1.GET("/assets",
			middleware.HTTPCache(cacheManager, constants.AssetCacheTTLMinutes*time.Minute, "kor:asset", nil, "asset"),
			assetHandler.ListAssets)
		v1.GET("/assets/:id",
			middleware.HTTPCache(cacheManager, 5*time.Minute, "kor:asset", nil, "asset"),
			assetHandler.GetAsset)

		// NFT Metadata routes
		v1.POST("/assets/metadata",
			middleware.InvalidateOnWrite(cacheManager, "kor:asset:*"),
			middleware.InvalidateOnWriteTags(cacheManager, "asset"),
			assetHandler.UpdateMetadata)
		v1.GET("/assets/:id/metadata",
			middleware.HTTPCache(cacheManager, 5*time.Minute, "kor:asset", nil, "asset"),
			assetHandler.GetMetadata)
		v1.POST("/assets/metadata/immutable",
			middleware.InvalidateOnWrite(cacheManager, "kor:asset:*"),
			middleware.InvalidateOnWriteTags(cacheManager, "asset"),
			assetHandler.MakeMetadataImmutable)

		// Oracle price feed routes (#104)
		v1.GET("/oracle/price",
			middleware.HTTPCache(cacheManager, constants.OracleCacheTTLMinutes*time.Minute, "kor:oracle", nil),
			assetHandler.GetOraclePrice)
		v1.GET("/assets/:id/oracle-price",
			middleware.HTTPCache(cacheManager, 1*time.Minute, "kor:oracle", nil),
			assetHandler.GetAssetOraclePrice)

		// Batch transaction routes (#106, #367, #370)
		batchHandler := handlers.NewBatchHandler(db, nil)
		v1.POST("/batch/execute",
			middleware.BatchValidator(),
			middleware.InvalidateOnWrite(cacheManager, "kor:asset:*"),
			middleware.InvalidateOnWriteTags(cacheManager, "asset"),
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
			middleware.InvalidateOnWriteTags(cacheManager, "asset"),
			assetHandler.ListAssetForSale)
		v1.POST("/marketplace/transfer",
			authMiddleware.JWTAuth(),
			middleware.InvalidateOnWrite(cacheManager, "kor:asset:*"),
			middleware.InvalidateOnWriteTags(cacheManager, "asset"),
			assetHandler.TransferAsset)
		v1.GET("/transactions", assetHandler.ListTransactions)

		// Search routes (#57)
		searchBackend, searchErr := services.NewESSearchBackend(os.Getenv("ELASTICSEARCH_URL"), db)
		if searchErr != nil {
			log.Printf("Warning: Elasticsearch disabled: %v", searchErr)
			searchBackend = services.NewDBSearchBackend(db)
		}
		searchHandler := handlers.NewSearchHandler(searchBackend)
		v1.GET("/search/assets", searchHandler.Search)
		v1.GET("/search/suggestions", searchHandler.Suggest)
		v1.GET("/search/autocomplete", searchHandler.Autocomplete)
		v1.GET("/search/facets", searchHandler.Facets)
		v1.GET("/search/analytics", searchHandler.SearchAnalytics)
		v1.POST("/search/analytics/click", searchHandler.RecordSearchClick)

		// Configurable transfer-approval workflow routes (#155).
		approvalHandler := handlers.NewApprovalHandler(workflowService)
		approvalRoutes := protected.Group("/approvals")
		{
			approvalRoutes.POST("/requests", approvalHandler.CreateRequest)
			approvalRoutes.GET("/requests", approvalHandler.ListRequests)
			approvalRoutes.GET("/requests/:id", approvalHandler.GetRequest)
			approvalRoutes.POST("/requests/:id/decisions", approvalHandler.Decide)
			approvalRoutes.POST("/requests/:id/delegate", approvalHandler.Delegate)
		}
		approvalAdmin := adminGroup.Group("/approvals")
		{
			approvalAdmin.POST("/workflows", approvalHandler.CreateWorkflow)
			approvalAdmin.GET("/workflows", approvalHandler.ListWorkflows)
			approvalAdmin.POST("/expire", approvalHandler.ExpireTimedOut)
		}

		// Compliance audit retrieval/export is restricted to administrators.
		auditHandler := handlers.NewAuditHandler(auditService)
		auditRoutes := adminGroup.Group("/audit/assets")
		{
			auditRoutes.GET("", auditHandler.List)
			auditRoutes.GET("/export", auditHandler.Export)
		}

		// KYC / AML routes (#55)
		kycHandler := handlers.NewKYCHandler(db, nil, emailService, nil) // nil, nil = mock KYC provider, AML provider from env
		v1.POST("/kyc/submit", kycHandler.SubmitKYC)
		v1.GET("/kyc/status", kycHandler.GetKYCStatus)
		v1.POST("/kyc/documents", kycHandler.UploadDocument)
		v1.POST("/kyc/aml/screen", kycHandler.ScreenAML)
		v1.POST("/kyc/aml/review/:id", kycHandler.ResolveAMLReview)
		v1.POST("/kyc/accredited", kycHandler.VerifyAccreditedInvestor)
		v1.GET("/kyc/audit", kycHandler.GetAuditLog)
		v1.GET("/compliance/report", kycHandler.ComplianceReport)

		// Beneficial ownership (UBO) routes (#379)
		complianceHandler := handlers.NewComplianceHandler(db)
		v1.POST("/compliance/ubo", complianceHandler.DeclareBeneficialOwner)
		v1.GET("/compliance/ubo", complianceHandler.ListBeneficialOwners)
		v1.POST("/compliance/ubo/:id/resolve", complianceHandler.ResolveUBOReview)
		v1.POST("/compliance/ubo/documents", complianceHandler.UploadCorporateStructureDocument)

		// Dispute resolution routes (#107)
		disputeHandler := handlers.NewDisputeHandler(db)
		v1.POST("/disputes", disputeHandler.FileDispute)
		v1.GET("/disputes", disputeHandler.ListDisputes)
		v1.GET("/disputes/history", disputeHandler.GetDisputeHistory)
		v1.GET("/disputes/:id", disputeHandler.GetDispute)

		// P2P secondary marketplace routes (#108)
		p2pHandler := handlers.NewP2PHandler(db)
		v1.POST("/p2p/orders", p2pHandler.CreateOrder)
		v1.GET("/p2p/orders", p2pHandler.ListOrders)
		v1.PUT("/p2p/orders/:id/cancel", p2pHandler.CancelOrder)
		v1.GET("/p2p/trades", p2pHandler.GetTradeHistory)
		v1.GET("/p2p/prices", p2pHandler.GetPriceChart)

		// Staking rewards routes (#109)
		stakingHandler := handlers.NewStakingHandler(db)
		v1.POST("/staking/stake", stakingHandler.Stake)
		v1.POST("/staking/unstake", stakingHandler.Unstake)
		v1.POST("/staking/claim", stakingHandler.ClaimRewards)
		v1.GET("/staking/dashboard", stakingHandler.GetStakingDashboard)
		v1.GET("/staking/rewards/history", stakingHandler.GetRewardHistory)

		// Liquidity pool routes (#110)
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

		// Incoming webhook routes (#372)
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

		// Outgoing webhook subscription routes (#126)
		outgoingWebhookHandler := handlers.NewOutgoingWebhookHandler(db)
		webhookSubs := protected.Group("/webhooks/subscriptions")
		{
			webhookSubs.POST("", outgoingWebhookHandler.CreateSubscription)
			webhookSubs.GET("", outgoingWebhookHandler.ListSubscriptions)
			webhookSubs.PUT("/:id", outgoingWebhookHandler.UpdateSubscription)
			webhookSubs.DELETE("/:id", outgoingWebhookHandler.DeleteSubscription)
			webhookSubs.GET("/:id/logs", outgoingWebhookHandler.GetDeliveryLogs)
		}

		// Webhook retry and DLQ routes (#186)
		webhookDeliveryGroup := protected.Group("/webhooks/delivery")
		{
			webhookDeliveryGroup.POST("/:id/retry", outgoingWebhookHandler.RetryDelivery)
			webhookDeliveryGroup.POST("/retry-all", outgoingWebhookHandler.RetryAllFailedDeliveries)
			webhookDeliveryGroup.POST("/replay-dlq", outgoingWebhookHandler.ReplayDLQ)
			webhookDeliveryGroup.GET("/dashboard", outgoingWebhookHandler.GetDeliveryDashboard)
			webhookDeliveryGroup.GET("/:id", outgoingWebhookHandler.GetDeliveryLog)
		}

		// Notification routes (#123)
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

		// Legal compliance routes (#120)
		legalHandler := handlers.NewLegalHandler(db)
		legalGroup := v1.Group("/legal")
		{
			legalGroup.GET("/:type", legalHandler.GetActiveDocument)
			legalGroup.GET("/:type/versions", legalHandler.ListDocumentVersions)
		}

		// GDPR data export routes (#178)
		gdprExportService := services.NewDataExportService(db, emailService)
		gdprHandler := handlers.NewGDPRHandler(db, gdprExportService)
		legalGroup.GET("/gdpr/export/download/:token", gdprHandler.DownloadExport)

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

		// Asset taxonomy routes (#159)
		taxonomyHandler := handlers.NewTaxonomyHandler(db)
		v1.GET("/taxonomy/categories", taxonomyHandler.ListCategories)
		v1.GET("/taxonomy/tags", taxonomyHandler.ListTags)
		v1.GET("/taxonomy/tags/autocomplete", taxonomyHandler.TagAutocomplete)
		v1.GET("/assets/:id/taxonomy", taxonomyHandler.GetAssetTaxonomy)
		taxonomyAdmin := adminGroup.Group("/taxonomy")
		{
			taxonomyAdmin.POST("/categories", taxonomyHandler.CreateCategory)
			taxonomyAdmin.PUT("/categories/:id", taxonomyHandler.UpdateCategory)
			taxonomyAdmin.DELETE("/categories/:id", taxonomyHandler.DeleteCategory)
			taxonomyAdmin.POST("/tags", taxonomyHandler.CreateTag)
			taxonomyAdmin.DELETE("/tags/:id", taxonomyHandler.DeleteTag)
		}
		protected.PUT("/assets/:id/categories", taxonomyHandler.SetAssetCategories)
		protected.PUT("/assets/:id/tags", taxonomyHandler.SetAssetTags)

		// Watchlist routes (#160)
		watchlistHandler := handlers.NewWatchlistHandler(db)
		v1.GET("/watchlists/public", watchlistHandler.ListPublicWatchlists)
		watchlistGroup := protected.Group("/watchlists")
		{
			watchlistGroup.GET("", watchlistHandler.ListWatchlists)
			watchlistGroup.POST("", watchlistHandler.CreateWatchlist)
			watchlistGroup.GET("/:id", watchlistHandler.GetWatchlist)
			watchlistGroup.PATCH("/:id", watchlistHandler.UpdateWatchlist)
			watchlistGroup.DELETE("/:id", watchlistHandler.DeleteWatchlist)
			watchlistGroup.POST("/:id/assets", watchlistHandler.AddAsset)
			watchlistGroup.DELETE("/:id/assets/:assetId", watchlistHandler.RemoveAsset)
		}

		// User activity dashboard routes (#161)
		analyticsService := services.NewAnalyticsService(db, redisClient)
		dashboardHandler := handlers.NewUserDashboardHandler(db, analyticsService)
		dashboardGroup := protected.Group("/dashboard")
		{
			dashboardGroup.GET("", dashboardHandler.GetDashboard)
			dashboardGroup.GET("/activity", dashboardHandler.GetActivityTimeline)
			dashboardGroup.GET("/export", dashboardHandler.ExportReport)
			dashboardGroup.POST("/activity", dashboardHandler.RecordActivity)
		}

		// Scheduled report generation and delivery routes (#173)
		reportHandler := handlers.NewReportHandler(reportScheduler)
		reportGroup := protected.Group("/reports")
		{
			reportGroup.GET("/schedules", reportHandler.ListSchedules)
			reportGroup.POST("/schedules", reportHandler.CreateSchedule)
			reportGroup.GET("/schedules/:id", reportHandler.GetSchedule)
			reportGroup.PUT("/schedules/:id", reportHandler.UpdateSchedule)
			reportGroup.DELETE("/schedules/:id", reportHandler.DeleteSchedule)
			reportGroup.POST("/schedules/:id/run", reportHandler.RunSchedule)
			reportGroup.GET("/history", reportHandler.ListHistory)
		}

		// Metadata version history routes (#195)
		v1.GET("/assets/:id/metadata/versions", assetHandler.ListMetadataVersions)
		v1.GET("/assets/:id/metadata/versions/:version", assetHandler.GetMetadataVersion)
		protected.POST("/assets/:id/metadata/versions/:version/revert", assetHandler.RevertMetadataVersion)

		// Fiat payment gateway routes (#196)
		paymentHandler := handlers.NewPaymentHandler(db)
		paymentGroup := protected.Group("/payments")
		{
			paymentGroup.POST("", paymentHandler.CreatePayment)
			paymentGroup.GET("", paymentHandler.ListPayments)
			paymentGroup.GET("/:id", paymentHandler.GetPayment)
		}
		router.POST("/payments/webhooks/:gateway", paymentHandler.HandleWebhook)
		adminGroup.POST("/payments/reconcile", paymentHandler.ReconcilePayments)

		// IP whitelist management routes (#194)
		adminSecurityHandler := handlers.NewAdminSecurityHandler(db)
		ipWhitelistGroup := adminGroup.Group("/security/ip-whitelist")
		{
			ipWhitelistGroup.POST("", adminSecurityHandler.AddIPWhitelistEntry)
			ipWhitelistGroup.GET("", adminSecurityHandler.ListIPWhitelistEntries)
			ipWhitelistGroup.DELETE("/:id", adminSecurityHandler.DeleteIPWhitelistEntry)
		}

		// Email template management routes (#163) — admin-only
		emailTemplateHandler := handlers.NewEmailTemplateHandler(db)
		emailTemplateAdmin := adminGroup.Group("/email-templates")
		{
			emailTemplateAdmin.GET("", emailTemplateHandler.ListTemplates)
			emailTemplateAdmin.POST("", emailTemplateHandler.CreateTemplate)
			emailTemplateAdmin.POST("/preview", emailTemplateHandler.Preview)
			emailTemplateAdmin.POST("/render", emailTemplateHandler.RenderTemplate)
			emailTemplateAdmin.GET("/:id", emailTemplateHandler.GetTemplate)
			emailTemplateAdmin.PUT("/:id", emailTemplateHandler.UpdateTemplate)
			emailTemplateAdmin.DELETE("/:id", emailTemplateHandler.DeleteTemplate)
			emailTemplateAdmin.POST("/:id/activate", emailTemplateHandler.ActivateTemplate)
			emailTemplateAdmin.GET("/:id/versions", emailTemplateHandler.ListVersions)
			emailTemplateAdmin.GET("/:id/variants", emailTemplateHandler.ListVariants)
			emailTemplateAdmin.POST("/:id/variants", emailTemplateHandler.CreateVariant)
		}

		// Asset comparison routes (#167)
		comparisonHandler := handlers.NewComparisonHandler(db)
		v1.POST("/comparisons", comparisonHandler.Compare)
		comparisonGroup := protected.Group("/comparisons")
		{
			comparisonGroup.GET("/history", comparisonHandler.ListHistory)
			comparisonGroup.GET("/history/:id", comparisonHandler.GetHistory)
		}

		// Asset performance metrics routes (#169)
		valuationTracker := services.NewValuationTrackerService(db, redisClient)
		analyticsHandler := handlers.NewAnalyticsHandler(db, valuationTracker)
		metricsCalculator := services.NewMetricsCalculatorService(db)
		metricsCalculator.Start(context.Background())
		v1.GET("/assets/:id/performance", analyticsHandler.GetPerformanceMetrics)
		v1.GET("/assets/:id/performance/history", analyticsHandler.GetPerformanceHistory)
		adminGroup.POST("/assets/:id/performance/recalculate", analyticsHandler.RecalculatePerformance)
		adminGroup.POST("/assets/:id/dividends", analyticsHandler.RecordDividend)

		// User referral program routes (#170)
		referralHandler := handlers.NewReferralHandler(db)
		referralGroup := protected.Group("/referrals")
		{
			referralGroup.GET("", referralHandler.ListReferrals)
			referralGroup.GET("/code", referralHandler.GetMyCode)
			referralGroup.GET("/stats", referralHandler.GetStats)
			referralGroup.POST("/apply", referralHandler.ApplyCode)
		}
		adminGroup.POST("/referrals/:refereeId/qualify", referralHandler.QualifyReferral)
	}

	// API v2 routes (#124)
	v2 := router.Group("/api/v2")
	{
		v2AssetsHandler := handlersv2.NewAssetsHandler(db)
		v2.GET("/assets", v2AssetsHandler.ListAssets)
		v2.GET("/assets/:id", v2AssetsHandler.GetAsset)
		v2.GET("/health", healthHandler.ReadinessCheck)

		v2Protected := v2.Group("")
		v2Protected.Use(authMiddleware.JWTAuth())
		{
			v2Protected.GET("/profile", authHandler.GetProfile)
			v2Protected.POST("/logout", authHandler.Logout)

			v2ReportHandler := handlers.NewReportHandler(reportScheduler)
			v2Reports := v2Protected.Group("/reports")
			{
				v2Reports.GET("/schedules", v2ReportHandler.ListSchedules)
				v2Reports.POST("/schedules", v2ReportHandler.CreateSchedule)
				v2Reports.GET("/schedules/:id", v2ReportHandler.GetSchedule)
				v2Reports.PUT("/schedules/:id", v2ReportHandler.UpdateSchedule)
				v2Reports.DELETE("/schedules/:id", v2ReportHandler.DeleteSchedule)
				v2Reports.POST("/schedules/:id/run", v2ReportHandler.RunSchedule)
				v2Reports.GET("/history", v2ReportHandler.ListHistory)
			}

			v2NotificationHandler := handlers.NewNotificationHandler(db)
			v2Notifications := v2Protected.Group("/notifications")
			{
				v2Notifications.GET("", v2NotificationHandler.ListNotifications)
				v2Notifications.GET("/unread-count", v2NotificationHandler.UnreadCount)
				v2Notifications.PUT("/read-all", v2NotificationHandler.MarkAllRead)
				v2Notifications.PUT("/:id/read", v2NotificationHandler.MarkRead)
				v2Notifications.GET("/preferences", v2NotificationHandler.GetPreferences)
				v2Notifications.PUT("/preferences", v2NotificationHandler.UpdatePreference)
			}
		}
	}

	// WebSocket routes (#54) — outside v1 group so the CSRF/JSON middleware
	// does not block the Upgrade handshake.
	wsHandler := handlers.NewWebSocketHandler(
		config.EnvOrDefault("JWT_SECRET", "your-super-secret-jwt-key-change-in-production"),
		config.LoadWSRateLimitConfig(),
	)
	router.GET("/ws", wsHandler.HandleWS)
	router.GET("/ws/stats", wsHandler.HandleWSStats)

	// Pre-launch the hub so it's ready before the first connection.
	_ = handlers.GetHub()

	// Initialize and start event indexer (#180)
	eventIndexer := services.NewEventIndexer(db)
	eventIndexer.Start()

	r := router

	// Start server
	port := os.Getenv("SERVER_PORT")
	if port == "" {
		port = constants.DefaultServerPort
	}

	log.Printf("Starting server on port %s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

