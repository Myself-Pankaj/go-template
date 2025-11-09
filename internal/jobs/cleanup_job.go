package jobs

import (
	"context"
	"go-server/internal/repository"
	"go-server/pkg/logger"
	"time"

	"go.uber.org/zap"
)

type CleanupJob struct {
	userRepo         repository.UserRepository
	verificationRepo repository.VerificationRepository
}

func NewCleanupJob(userRepo repository.UserRepository, verificationRepo repository.VerificationRepository) *CleanupJob {
	return &CleanupJob{
		userRepo:         userRepo,
		verificationRepo: verificationRepo,
	}
}

// Start runs the cleanup job periodically
func (j *CleanupJob) Start(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run immediately on start
	j.runCleanup(ctx)

	for {
		select {
		case <-ticker.C:
			j.runCleanup(ctx)
		case <-ctx.Done():
			logger.Info("Cleanup job stopped")
			return
		}
	}
}

func (j *CleanupJob) runCleanup(ctx context.Context) {
	logger.Info("Starting cleanup of expired unverified users and verifications")

	// Step 1: Delete expired unverified users (CASCADE will delete their verifications)
	deletedUsers, err := j.userRepo.DeleteExpiredUnverified(ctx)
	if err != nil {
		logger.Error("Failed to delete expired unverified users", zap.Error(err))
	} else {
		logger.Info("Deleted expired unverified users", zap.Int64("count", deletedUsers))
	}

	// Step 2: Delete orphaned expired verifications (for users that are still active)
	err = j.verificationRepo.DeleteExpiredVerifications(ctx)
	if err != nil {
		logger.Error("Failed to delete expired verifications", zap.Error(err))
	} else {
		logger.Info("Deleted expired verifications")
	}

	logger.Info("Cleanup completed successfully")
}

// StartWithGracefulShutdown runs the cleanup job with graceful shutdown support
func (j *CleanupJob) StartWithGracefulShutdown(interval time.Duration) context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())
	
	go j.Start(ctx, interval)
	
	return cancel
}
