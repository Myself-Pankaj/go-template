package app

import (
	"context"
	"go-server/internal/config"
	"go-server/internal/handler"
	"go-server/internal/jobs"
	"go-server/internal/repository"
	"go-server/internal/service"
	authservice "go-server/internal/service/auth_service"
	"go-server/pkg/database"
	"go-server/pkg/logger"
	"time"

	"go.uber.org/zap"
)

// Dependencies holds all application dependencies
type Dependencies struct {
	// Handlers
	AuthHandler *handler.AuthHandler

	// Jobs (for graceful shutdown)
	CleanupJob *jobs.CleanupJob

	// Context for cleanup
	cleanupCtx    context.Context
	cleanupCancel context.CancelFunc
}

// Shutdown gracefully stops all background jobs
func (d *Dependencies) Shutdown() {
	logger.Info("🛑 Shutting down background jobs...")
	if d.cleanupCancel != nil {
		d.cleanupCancel()
	}
	logger.Info("✅ Background jobs stopped")
}

// InitializeDependencies sets up all application dependencies
func InitializeDependencies(db *database.Database, cfg *config.Config) (*Dependencies, error) {
	logger.Info("🔧 Initializing application dependencies...")

	// ==================== REPOSITORIES ====================
	logger.Debug("Initializing repositories...")
	userRepo := repository.NewUserRepository(db)
	verificationRepo := repository.NewVerificationRepository(db)

	// Transaction Manager
	txManager := repository.NewTransactionManager(db)

	// ==================== SERVICES ====================
	logger.Debug("Initializing services...")

	// Email Service
	emailSvc, err := service.NewEmailService(cfg)
	if err != nil {
		logger.Error("Failed to initialize email service", zap.Error(err))
		return nil, err
	}

	// Auth Services
	authService := authservice.NewAuthService(userRepo)
	regService := authservice.NewRegistrationService(userRepo, verificationRepo, emailSvc, txManager)
	verifService := authservice.NewVerificationService(userRepo, verificationRepo, emailSvc, txManager)

	// ==================== BACKGROUND JOBS ====================
	logger.Debug("Initializing background jobs...")

	// Create context for cleanup job
	cleanupCtx, cleanupCancel := context.WithCancel(context.Background())

	// Cleanup Job
	cleanupJob := jobs.NewCleanupJob(userRepo, verificationRepo)
	go func() {
		cleanupJob.Start(cleanupCtx, 1*time.Hour)
	}()
	logger.Info("✅ Cleanup job started (runs every hour)")

	// ==================== HANDLERS ====================
	logger.Debug("Initializing handlers...")

	authHandler := handler.NewAuthHandler(
		regService,
		verifService,
		authService,
		cfg.JWTSecret,
		cfg.JWTIssuer,
		cfg.JWTExpiry,
		cfg.IsProduction(),
	)

	logger.Info("✅ Dependencies initialized successfully")

	return &Dependencies{
		AuthHandler:   authHandler,
		CleanupJob:    cleanupJob,
		cleanupCtx:    cleanupCtx,
		cleanupCancel: cleanupCancel,
	}, nil
}