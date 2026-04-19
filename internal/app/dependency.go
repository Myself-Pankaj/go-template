package app

import (
	"context"
	"go-server/internal/config"
	"go-server/internal/handler"
	"go-server/internal/jobs"
	"go-server/internal/middleware/guards"
	"go-server/internal/repository"
	"go-server/internal/service"
	authservice "go-server/internal/service/auth_service"
	flatservice "go-server/internal/service/flat_service"
	inviteservice "go-server/internal/service/invite_service"
	onboardingservice "go-server/internal/service/onboarding_service"
	planservice "go-server/internal/service/plan_service"
	societyservice "go-server/internal/service/society_service"
	subsservice "go-server/internal/service/subscription_service"
	"go-server/pkg/database"
	"go-server/pkg/logger"
	"time"

	"go.uber.org/zap"
)

// Dependencies holds all application dependencies
type Dependencies struct {
	// Handlers
	AuthHandler       *handler.AuthHandler
	SocietyHandler    *handler.SocietyHandler
	SubsHandler       *handler.SubscriptionHandler
	FlatHandler       *handler.FlatHandler
	PlanHandler       *handler.PlanHandler
	OnboardingHandler *handler.OnboardingHandler

	// Guards — central access-control factory; passed to every route file.
	Guards *guards.Guards

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
	flatRepo := repository.NewFlatRepository(db)
	societyRepo := repository.NewSocietyRepository(db)
	planRepo := repository.NewPlanRepository(db)
	subsrepo := repository.NewSubscriptionRepository(db)
	inviteRepo := repository.NewInviteRepository(db)
	claimRepo := repository.NewClaimRepository(db)
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
	subsService := subsservice.NewSubscriptionService(subsrepo, planRepo, txManager)
	societyService := societyservice.NewSocietyService(societyRepo, userRepo, subsService, planRepo, txManager)
	flatService := flatservice.NewFlatService(flatRepo)
	plansService := planservice.NewPlanService(planRepo, userRepo, subsrepo)
	inviteService := inviteservice.NewInviteService(inviteRepo, flatRepo)
	onboardingSvc := onboardingservice.NewOnboardingService(
		userRepo, flatRepo, societyRepo, claimRepo, inviteRepo, inviteService, txManager, plansService,
	)

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
		cfg.JWTAccessTokenExpiry,
		cfg.JWTRefreshTokenExpiry,
		cfg.IsProduction(),
	)

	societyHandler := handler.NewSocietyHandler(societyService)
	subsHandler := handler.NewSubscriptionHandler(subsService)
	flatHandler := handler.NewFlatHandler(flatService)
	planHandler := handler.NewPlanHandler(plansService)
	onboardingHandler := handler.NewOnboardingHandler(
		onboardingSvc,
		authService,
		cfg.JWTSecret,
		cfg.JWTIssuer,
		cfg.JWTAccessTokenExpiry,
		cfg.JWTRefreshTokenExpiry,
		cfg.IsProduction(),
	)

	// ==================== GUARDS ====================
	appGuards := guards.New(cfg.JWTSecret, cfg.JWTIssuer, subsrepo, flatRepo, userRepo)

	logger.Info("✅ Dependencies initialized successfully")

	return &Dependencies{
		AuthHandler:       authHandler,
		SocietyHandler:    societyHandler,
		SubsHandler:       subsHandler,
		FlatHandler:       flatHandler,
		PlanHandler:       planHandler,
		OnboardingHandler: onboardingHandler,
		Guards:            appGuards,
		CleanupJob:        cleanupJob,
		cleanupCtx:        cleanupCtx,
		cleanupCancel:     cleanupCancel,
	}, nil
}