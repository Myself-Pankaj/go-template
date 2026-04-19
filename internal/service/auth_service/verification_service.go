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

	"golang.org/x/crypto/bcrypt"
)
var (
	ErrNotFound = models.NewAppError(
		models.ErrCodeNotFound,
		"record not found",
		http.StatusNotFound,
		nil,
	)
	ErrOTPExpired = models.NewAppError(
		models.ErrCodeOTPExpired,
		"otp expired",
		http.StatusBadRequest,
		nil,
	)
)

type VerificationService interface {
	VerifyOTP(ctx context.Context, req *models.VerifyOTPRequest) (*models.User, error)
	ResendOTP(ctx context.Context, req *models.ResendOTPRequest) error
	ResetPassword(ctx context.Context, req *models.ResetPasswordRequest) error
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

    err = s.txManager.WithTransaction(ctx, func(txCtx context.Context) error {
        if err := s.validateAndConsumeOTP(txCtx, user.ID, req.OTP); err != nil {
            return err
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

func (s *verificationService) ResetPassword(ctx context.Context, req *models.ResetPasswordRequest) error {
    ctx, cancel := context.WithTimeout(ctx, service.DefaultTimeout)
    defer cancel()

    user, err := s.userRepo.GetByEmail(ctx, req.Email)
    if err != nil || user == nil {
        return models.NewAppError(models.ErrCodeNotFound, "Unable to find user with provided email", http.StatusNotFound, err)
    }

    if req.NewPassword != req.ConfirmPassword {
        return models.NewAppError(models.ErrCodeBadRequest, "new password and confirm password do not match", http.StatusBadRequest, nil)
    }

    err = s.txManager.WithTransaction(ctx, func(txCtx context.Context) error {
        if err := s.validateAndConsumeOTP(txCtx, user.ID, req.ResetToken); err != nil {
            return err
        }

        hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
        if err != nil {
            return models.NewAppError(models.ErrCodeInternalServer, "Failed to hash password", http.StatusInternalServerError, err)
        }

        if err := s.userRepo.UpdatePassword(txCtx, user.ID, string(hashedPassword)); err != nil {
            return models.NewAppError(models.ErrCodeDatabaseError, "Failed to update password", http.StatusInternalServerError, err)
        }
        return nil
    })

    return err
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
// private helper — just validates and marks OTP as used
func (s *verificationService) validateAndConsumeOTP(ctx context.Context, userID int64, otp string) error {
    verification, err := s.verificationRepo.GetActiveVerification(ctx, userID, otp)
    if err != nil {
        if errors.Is(err, ErrNotFound) {
            return models.NewAppError(models.ErrCodeNotFound, "no active verification found for this OTP", http.StatusNotFound, err)
        }
        return models.NewAppError(models.ErrCodeInvalidOTP, "invalid or incorrect OTP provided", http.StatusBadRequest, err)
    }

    if time.Now().UTC().After(verification.ExpiresAt) {
        return models.NewAppError(models.ErrCodeOTPExpired, "the OTP has expired, please request a new one", http.StatusBadRequest, nil)
    }

    if err := s.verificationRepo.MarkAsUsed(ctx, verification.ID); err != nil {
        return models.NewAppError(models.ErrCodeDatabaseError, "failed to mark OTP as used", http.StatusInternalServerError, err)
    }

    return nil
}