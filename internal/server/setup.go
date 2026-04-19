package server

import (
	"context"
	"go-server/internal/app"
	"go-server/internal/config"

	"go-server/internal/middleware"

	routes "go-server/internal/router"
	"go-server/pkg/database"
	"go-server/pkg/logger"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"go.uber.org/zap"
)

// SetupMiddleware initializes all middleware for the Gin engine
	func SetupMiddleware(r *gin.Engine, cfg *config.Config) {
		// Request ID (must be first for logging)
		r.Use(middleware.RequestID())

		// Logger middleware
		loggerCfg := middleware.DefaultLoggerConfig()
		loggerCfg.SkipHealthCheck = true
		loggerCfg.SkipPaths = []string{"/favicon.ico"}
		r.Use(middleware.LoggerWithConfig(loggerCfg))

		// Recovery middleware
		recoveryCfg := middleware.DefaultRecoveryConfig()
		recoveryCfg.EnableStackTrace = true
		recoveryCfg.EnableRequestDump = cfg.IsDevelopment()
		r.Use(middleware.RecoveryWithConfig(recoveryCfg))

		// CORS + Rate limiting
		r.Use(middleware.CORS(cfg))
		r.Use(middleware.RateLimit(cfg))

		// Trusted proxies
		if len(cfg.TrustedProxies) > 0 {
			r.SetTrustedProxies(cfg.TrustedProxies)
		}
	}


	// setupRoutes configures all application routes
	func SetupRoutes(r *gin.Engine, deps *app.Dependencies, cfg *config.Config) {
		logger.Info("🛣️  Setting up routes...")

		// API version 1
		api := r.Group("/api/v1")
		{
			// Auth routes
			routes.SetupAuthRoutes(api, deps.AuthHandler, deps.Guards)
			// Society routes
			routes.SetupSocietyRoutes(api, deps.SocietyHandler, deps.Guards)
			// Subscription routes
			routes.SetupSubscriptionRoutes(api, deps.SubsHandler, deps.Guards)
			// Flat routes
			routes.SetupFlatRoutes(api, deps.FlatHandler, deps.Guards)
			// Plan routes
			routes.SetupPlanRoutes(api, deps.PlanHandler, deps.Guards)
			// Onboarding routes (QR claims + invite redemption)
			routes.SetupOnboardingRoutes(api, deps.OnboardingHandler, deps.Guards)

			// Add more route groups here as your application grows
			// routes.SetupUserRoutes(api, deps.UserHandler, cfg.JWTSecret, cfg.JWTIssuer)
			// routes.SetupProductRoutes(api, deps.ProductHandler, cfg.JWTSecret, cfg.JWTIssuer)
		}

		logger.Info("✅ Routes configured successfully")
	}




	func SetupHealthCheck(r *gin.Engine, cfg *config.Config, db *database.Database) {
		r.GET("/health", func(c *gin.Context) {
			// Basic health check
			c.JSON(http.StatusOK, gin.H{
				"status":      "healthy",
				"app":         cfg.AppName,
				"version":     cfg.Version,
				"environment": cfg.Environment,
				"timestamp":   time.Now().UTC().Format(time.RFC3339),
			})
		})

		r.GET("/health/ready", func(c *gin.Context) {
			// Readiness check - includes database connectivity
			ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
			defer cancel()

			if err := db.Ping(ctx); err != nil {
				logger.Error("Readiness check failed: database ping error", zap.Error(err))
				c.JSON(http.StatusServiceUnavailable, gin.H{
					"status": "not_ready",
					"error":  "database connection failed",
				})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"status":    "ready",
				"database":  "connected",
				"timestamp": time.Now().UTC().Format(time.RFC3339),
			})
		})

		r.GET("/health/live", func(c *gin.Context) {
			// Liveness check - simple check that the server is running
			c.JSON(http.StatusOK, gin.H{
				"status":    "alive",
				"timestamp": time.Now().UTC().Format(time.RFC3339),
			})
		})

		logger.Info("✅ Health check endpoints configured")
	}