package authservice

import (
	"context"
	"errors"
	"go-server/internal/models"
	"go-server/internal/repository"
	"go-server/internal/service"
	"go-server/pkg/utils"
	"net/http"
	"time"
)

type VerificationService interface {
	VerifyOTP(ctx context.Context, req *models.VerifyOTPRequest) (*models.User, error)
	ResendOTP(ctx context.Context, req *models.ResendOTPRequest) error
}

type EmailService interface {
	SendOTP(ctx context.Context, to, otp, name string) error
}

type verificationService struct {
	userRepo         repository.UserRepository
	verificationRepo repository.VerificationRepository
	emailService     EmailService
	txManager        repository.TransactionManager
}

func NewVerificationService(
	userRepo repository.UserRepository,
	verificationRepo repository.VerificationRepository,
	emailService EmailService,
	txManager repository.TransactionManager,
) VerificationService {
	return &verificationService{
		userRepo:         userRepo,
		verificationRepo: verificationRepo,
		emailService:     emailService,
		txManager:        txManager,
	}
}

func (s *verificationService) VerifyOTP(ctx context.Context, req *models.VerifyOTPRequest) (*models.User, error) {
	ctx, cancel := context.WithTimeout(ctx, service.DefaultTimeout)
	defer cancel()

	user, err := s.userRepo.GetByEmail(ctx, req.Email)
	if err != nil {
		return nil, models.NewAppError(models.ErrCodeNotFound, "user not found", http.StatusNotFound, err)
	}

	if user.IsVerified {
		return user, models.NewAppError(models.ErrCodeUnauthorized, "user is already verified", http.StatusUnauthorized, nil)
	}

	verification, err := s.verificationRepo.GetActiveVerification(ctx, user.ID, req.OTP)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, models.NewAppError(models.ErrCodeNotFound, "no active verification found for this OTP", http.StatusNotFound, err)
		}
		return nil, models.NewAppError(models.ErrCodeInvalidOTP, "invalid or incorrect OTP provided", http.StatusBadRequest, err)
	}

	if time.Now().UTC().After(verification.ExpiresAt) {
		return nil, models.NewAppError(models.ErrCodeOTPExpired, "the OTP has expired, please request a new one", http.StatusBadRequest, nil)
	}

	err = s.txManager.WithTransaction(ctx, func(txCtx context.Context) error {
		if err := s.verificationRepo.MarkAsUsed(txCtx, verification.ID); err != nil {
			return models.NewAppError(models.ErrCodeDatabaseError, "failed to mark OTP as used", http.StatusInternalServerError, err)
		}
		if err := s.userRepo.UpdateIsVerified(txCtx, user.ID, true); err != nil {
			return models.NewAppError(models.ErrCodeDatabaseError, "failed to update user verification status", http.StatusInternalServerError, err)
		}
		return nil
	})

	if err != nil {

		return nil, models.NewAppError(models.ErrCodeTransactionFailed, "failed to complete verification transaction", http.StatusInternalServerError, err)
	}

	user.IsVerified = true
	return user, nil
}

func (s *verificationService) ResendOTP(ctx context.Context, req *models.ResendOTPRequest) error {
	ctx, cancel := context.WithTimeout(ctx, service.DefaultTimeout)
	defer cancel()

	user, err := s.userRepo.GetByEmail(ctx, req.Email)
	if err != nil {
		return models.NewAppError(models.ErrCodeNotFound, req.Email+" not found in DB", http.StatusNotFound, err)
	}

	if user.IsVerified {
		return models.NewAppError(models.ErrCodeUnauthorized, "user is already verified", http.StatusUnauthorized, nil)
	}

	otp, err := utils.GenerateOTP()
	if err != nil {
		return models.NewAppError(models.ErrCodeInternalServer, "failed to generate OTP", http.StatusInternalServerError, err)
	}

	verification := &models.UserVerification{
		UserID:    user.ID,
		OTP:       otp,
		ExpiresAt: time.Now().UTC().Add(service.OTPExpiryDuration),
		IsUsed:    false,
	}

	if err := s.verificationRepo.CreateVerification(ctx, verification); err != nil {
		return models.NewAppError(models.ErrCodeDatabaseError, "failed to create new OTP record", http.StatusInternalServerError, err)
	}

	emailCtx, emailCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer emailCancel()

	if err := s.emailService.SendOTP(emailCtx, user.Email, otp, user.Name); err != nil {
		return models.NewAppError(models.ErrCodeEmailSendFailed, "failed to send OTP email", http.StatusInternalServerError, err)
	}

	return nil
}
