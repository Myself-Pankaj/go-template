package authservice

import (
	"context"

	"go-server/internal/models"
	"go-server/internal/repository"
	"go-server/internal/service"

	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// ==================== AUTH SERVICE ====================

type AuthService interface {
	Login(ctx context.Context, req *models.LoginRequest) (*models.User, error)
	Update(ctx context.Context, req *models.UpdateUserRequest) (*models.User, error)
	UpdateLastLogin(ctx context.Context, userID int64) error
	ChangePassword(ctx context.Context, userID int64, req *models.ChangePasswordRequest) error
}

type authService struct {
	userRepo repository.UserRepository
}

func NewAuthService(userRepo repository.UserRepository) AuthService {
	return &authService{userRepo: userRepo}
}

// Login authenticates a user with email/phone and password
func (s *authService) Login(ctx context.Context, req *models.LoginRequest) (*models.User, error) {
	ctx, cancel := context.WithTimeout(ctx, service.DefaultTimeout)
	defer cancel()

	// Fetch user by email or phone
	user, err := s.getUserByIdentifier(ctx, req)
	if err != nil {
		// Return generic error to prevent user enumeration
		return nil, models.NewAppError(models.ErrCodeNotFound, "Unable to found user", http.StatusNotFound, err)
	}

	// Check if user is verified
	if !user.IsVerified {
		return nil, models.NewAppError(models.ErrCodeUserNotVerified, "User is not verified", http.StatusUnauthorized, nil)
	}

	// Verify password
	if err := s.verifyPassword(user.PasswordHash, req.Password); err != nil {
		return nil, models.NewAppError(models.ErrCodeInvalidCredentials, "Invalid credentials", http.StatusUnauthorized, err)
	}

	return user, nil
}

// Update modifies user profile information
func (s *authService) Update(ctx context.Context, req *models.UpdateUserRequest) (*models.User, error) {
	ctx, cancel := context.WithTimeout(ctx, service.DefaultTimeout)
	defer cancel()

	// Validate user ID
	if req.ID <= 0 {
		return nil, models.NewAppError(models.ErrCodeBadRequest, "Invalid User Id", http.StatusBadRequest, nil)
	}

	// Fetch existing user
	user, err := s.userRepo.GetByID(ctx, req.ID)
	if err != nil {
		return nil, models.NewAppError(models.ErrCodeDatabaseError, "Failed to fetch user", http.StatusInternalServerError, err)
	}

	// Apply changes if provided
	if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
		user.Name = strings.TrimSpace(*req.Name)
	}

	// Update timestamps & persist
	user.UpdatedAt = time.Now().UTC()
	if err := s.userRepo.Update(ctx, user); err != nil {
		return nil, models.NewAppError(models.ErrCodeDatabaseError, "Failed to update user", http.StatusInternalServerError, err)
	}

	return user, nil
}

// UpdateLastLogin updates the user's last login timestamp
func (s *authService) UpdateLastLogin(ctx context.Context, userID int64) error {
	ctx, cancel := context.WithTimeout(ctx, service.DefaultTimeout)
	defer cancel()

	// Validate user ID
	if userID <= 0 {
		return models.NewAppError(models.ErrCodeBadRequest, "Kindly enter valid user id", http.StatusBadRequest, nil)
	}

	if err := s.userRepo.UpdateLastLogin(ctx, userID); err != nil {
		return models.NewAppError(models.ErrCodeDatabaseError, "Failed to update last login", http.StatusInternalServerError, err)
	}
	return nil
}

// ChangePassword allows users to change their password
func (s *authService) ChangePassword(ctx context.Context, userID int64, req *models.ChangePasswordRequest) error {
	ctx, cancel := context.WithTimeout(ctx, service.DefaultTimeout)
	defer cancel()

	// Validate user ID
	if userID <= 0 {
		return models.NewAppError(models.ErrCodeBadRequest, "Invalid User Id", http.StatusBadRequest, nil)
	}

	// Fetch user
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return models.NewAppError(models.ErrCodeNotFound, "Unable to find user", http.StatusNotFound, err)
	}

	// Verify current password
	if err := s.verifyPassword(user.PasswordHash, req.CurrentPassword); err != nil {
		return models.NewAppError(models.ErrCodeInvalidCredentials, "Invalid credentials", http.StatusUnauthorized, err)
	}

	// Hash new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return models.NewAppError(models.ErrCodeInternalServer, "Failed to hash password", http.StatusInternalServerError, err)
	}

	// Update password
	if err := s.userRepo.UpdatePassword(ctx, userID, string(hashedPassword)); err != nil {
		return models.NewAppError(models.ErrCodeDatabaseError, "Failed to update password", http.StatusInternalServerError, err)
	}

	return nil
}

// ==================== HELPER METHODS ====================

// getUserByIdentifier fetches user by email or phone number
func (s *authService) getUserByIdentifier(ctx context.Context, req *models.LoginRequest) (*models.User, error) {
	var user *models.User
	var err error

	// Prioritize email if both are provided
	if req.Email != nil && *req.Email != "" {
		user, err = s.userRepo.GetByEmail(ctx, *req.Email)
	} else if req.PhoneNumber != nil && *req.PhoneNumber != "" {
		user, err = s.userRepo.GetByPhoneNumber(ctx, *req.PhoneNumber)
	}

	if err != nil {

		return nil, models.NewAppError(models.ErrCodeNotFound, "User Not found", http.StatusNotFound, err)
	}

	if user == nil {
		return nil, models.NewAppError(models.ErrCodeInvalidCredentials, "Kindly Provide the valid credential", http.StatusUnauthorized, err)
	}

	return user, nil
}

// verifyPassword compares hashed password with plaintext password
func (s *authService) verifyPassword(hashedPassword, plainPassword string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(plainPassword)); err != nil {

		return models.NewAppError(models.ErrCodeInternalServer, "Unable to verify password", http.StatusInternalServerError, err)
	}
	return nil
}
