package authservice

import (
	"context"
	"go-server/internal/models"
	"go-server/internal/repository"
	"go-server/internal/service"
	"go-server/pkg/utils"
	"time"

	// "go-server/pkg/validator"
	"strings"
)

type RegistrationService interface {
	Register(ctx context.Context, req *models.RegisterRequest) (*models.User, error)
}

type registrationService struct {
	userRepo         repository.UserRepository
	verificationRepo repository.VerificationRepository
	emailService     EmailService
	txManager        repository.TransactionManager
}

func NewRegistrationService(
	userRepo repository.UserRepository,
	verificationRepo repository.VerificationRepository,
	emailService EmailService,
	txManager repository.TransactionManager,
) RegistrationService {
	return &registrationService{
		userRepo:         userRepo,
		verificationRepo: verificationRepo,
		emailService:     emailService,
		txManager:        txManager,
	}
}

func (s *registrationService) Register(ctx context.Context, req *models.RegisterRequest) (*models.User, error) {
	ctx, cancel := context.WithTimeout(ctx, service.DefaultTimeout)
	defer cancel()

	// --- Sanitize ---
	req.Name = strings.TrimSpace(req.Name)
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))

	emailExists, err := s.userRepo.EmailExists(ctx, req.Email)
	if err != nil {
		return nil, models.NewAppError(models.ErrCodeDatabaseError, "failed to check "+req.Email+" existence", 500, err)
	}
	if emailExists {
		return nil, models.NewAppError(models.ErrCodeConflict, req.Email+" already exists", 409, nil)
	}

	hashedPassword, err := utils.HashPassword(req.Password)
	if err != nil {
		return nil, models.NewAppError(models.ErrCodeInternalServer, "failed to hash password", 500, err)
	}

	otp, err := utils.GenerateOTP()
	if err != nil {
		return nil, models.NewAppError(models.ErrCodeInternalServer, "failed to generate OTP", 500, err)
	}

	var user *models.User
	err = s.txManager.WithTransaction(ctx, func(txCtx context.Context) error {
		user = &models.User{
			Name:         req.Name,
			Email:        req.Email,
			PhoneNumber:  req.PhoneNumber,
			PasswordHash: hashedPassword,
			Role:         "owner",
			IsVerified:   false,
		}

		if err := s.userRepo.Create(ctx, user); err != nil {
			return models.NewAppError(models.ErrCodeDatabaseError, "failed to create user", 500, err)
		}

		verification := &models.UserVerification{
			UserID:    user.ID,
			OTP:       otp,
			ExpiresAt: time.Now().UTC().Add(service.OTPExpiryDuration),
			IsUsed:    false,
		}

		return s.verificationRepo.CreateVerification(txCtx, verification)
	})

	if err != nil {
		return nil, models.NewAppError(models.ErrCodeDatabaseError, "Transaction Failed", 500, err)
	}

	emailCtx, emailCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer emailCancel()
	if err := s.emailService.SendOTP(emailCtx, req.Email, otp, req.Name); err != nil {
		return user, models.NewAppError(models.ErrCodeEmailSendFailed, "User Created but unable to send an email", 400, err)
	}

	return user, nil
}
