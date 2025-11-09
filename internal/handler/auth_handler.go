package handler

import (
	"context"
	"errors"
	"go-server/internal/middleware"
	"go-server/internal/models"
	authservice "go-server/internal/service/auth_service"

	"go-server/pkg/utils"
	"go-server/pkg/validator"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ==================== HANDLER STRUCT ====================

type AuthHandler struct {
	registrationService authservice.RegistrationService
	verificationService authservice.VerificationService
	authService         authservice.AuthService
	jwtSecret           string
	jwtIssuer           string
	jwtExpiry           time.Duration
	isProduction        bool
}

// ==================== CONSTRUCTOR ====================

func NewAuthHandler(
	registrationService authservice.RegistrationService,
	verificationService authservice.VerificationService,
	authService authservice.AuthService,
	jwtSecret string,
	jwtIssuer string,
	jwtExpiry time.Duration,
	isProduction bool,
) *AuthHandler {
	return &AuthHandler{
		registrationService: registrationService,
		verificationService: verificationService,
		authService:         authService,
		jwtSecret:           jwtSecret,
		jwtIssuer:           jwtIssuer,
		jwtExpiry:           jwtExpiry,
		isProduction:        isProduction,
	}
}

// ==================== COOKIE HELPERS ====================

// setAuthCookie sets the JWT token as an HTTP-only cookie with secure defaults
func (h *AuthHandler) setAuthCookie(c *gin.Context, token string) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(
		"auth_token",
		token,
		int(h.jwtExpiry.Seconds()),
		"/",
		"", // Let browser determine domain
		h.isProduction,
		true, // httpOnly - prevents XSS attacks
	)
}

// clearAuthCookie removes the authentication cookie securely
func (h *AuthHandler) clearAuthCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(
		"auth_token",
		"",
		-1, // Immediate expiry
		"/",
		"",
		h.isProduction,
		true,
	)
}

// ==================== REGISTRATION FLOW ====================

// Register godoc
// @Summary Register a new user
// @Description Creates a new user account and sends OTP verification email
// @Tags auth
// @Accept json
// @Produce json
// @Param request body models.RegisterRequest true "Registration details"
// @Success 201 {object} utils.Response
// @Failure 400 {object} utils.Response
// @Failure 409 {object} utils.Response
// @Failure 500 {object} utils.Response
// @Router /auth/register [post]
func (h *AuthHandler) Register(c *gin.Context) {
	var req models.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {

		utils.BadRequestResponse(c, "Invalid request format. Please check your input")
		return
	}
	//validation
	if validationErrs := validator.ValidateStruct(req); validationErrs != nil {
		utils.ValidationErrorResponse(c, validationErrs.ToMap())
		return
	}

	// Register user
	user, err := h.registrationService.Register(c.Request.Context(), &req)
	if err != nil {

		var appErr *models.AppError
		if errors.As(err, &appErr) {
			utils.ErrorResponse(c, appErr.StatusCode, appErr.Code, appErr.Message, appErr)
			return
		}
	}

	utils.SuccessResponse(c, http.StatusCreated, "Account created successfully", gin.H{
		"user":    user.ToResponse(),
		"message": "Please verify your email using the OTP sent to your email address",
	})
}

// VerifyOTP godoc
// @Summary Verify OTP
// @Description Verifies OTP and marks user as verified, returns auth token
// @Tags auth
// @Accept json
// @Produce json
// @Param request body models.VerifyOTPRequest true "OTP verification details"
// @Success 200 {object} utils.Response
// @Failure 400 {object} utils.Response
// @Failure 404 {object} utils.Response
// @Failure 500 {object} utils.Response
// @Router /auth/verify-otp [post]
func (h *AuthHandler) VerifyOTP(c *gin.Context) {
	var req models.VerifyOTPRequest

	// --- Parse JSON ---
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequestResponse(c, "Invalid request format. Please check your input")
		return
	}

	// --- Validate Input ---
	if validationErrs := validator.ValidateStruct(&req); validationErrs != nil {
		utils.ValidationErrorResponse(c, validationErrs.ToMap())
		return
	}

	// --- Sanitize ---
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.OTP = strings.TrimSpace(req.OTP)

	// --- Verify OTP ---
	user, err := h.verificationService.VerifyOTP(c.Request.Context(), &req)
	if err != nil {
		var appErr *models.AppError
		if errors.As(err, &appErr) {
			utils.ErrorResponse(c, appErr.StatusCode, appErr.Code, appErr.Message, appErr)
			return
		}

		// Unexpected error (non-AppError)
		utils.ErrorResponse(c, http.StatusInternalServerError, "AuthService", "unexpected_error", err)
		return
	}

	// --- Generate JWT token ---
	token, err := h.generateToken(user)
	if err != nil {
		utils.InternalServerErrorResponse(c, err)
		return
	}

	// --- Set Auth Cookie ---
	h.setAuthCookie(c, token)

	utils.SuccessResponse(c, http.StatusOK, "Email verified successfully", gin.H{
		"user": user.ToResponse(),
	})
}

// ResendOTP godoc
// @Summary Resend OTP
// @Description Resends OTP verification code to user's email
// @Tags auth
// @Accept json
// @Produce json
// @Param request body models.ResendOTPRequest true "Email to resend OTP"
// @Success 200 {object} utils.Response
// @Failure 400 {object} utils.Response
// @Failure 404 {object} utils.Response
// @Failure 429 {object} utils.Response
// @Failure 500 {object} utils.Response
// @Router /auth/resend-otp [post]
func (h *AuthHandler) ResendOTP(c *gin.Context) {
	var req models.ResendOTPRequest

	// --- Parse JSON ---
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequestResponse(c, "Invalid request format. Please check your input")
		return
	}

	// --- Sanitize & Validate ---
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if validationErrs := validator.ValidateStruct(&req); validationErrs != nil {
		utils.ValidationErrorResponse(c, validationErrs.ToMap())
		return
	}

	if !validator.IsValidEmail(req.Email) {
		utils.BadRequestResponse(c, "Invalid email format")
		return
	}

	// --- Resend OTP ---
	err := h.verificationService.ResendOTP(c.Request.Context(), &req)
	if err != nil {
		var appErr *models.AppError
		if errors.As(err, &appErr) {
			utils.ErrorResponse(c, appErr.StatusCode, appErr.Code, appErr.Message, appErr)
			return
		}

		utils.ErrorResponse(c, http.StatusInternalServerError, "AuthService", "unexpected_error", err)
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "OTP has been resent to your email", nil)
}

// ==================== LOGIN/LOGOUT ====================

// Login godoc
// @Summary User login
// @Description Authenticates user with email/phone and password
// @Tags auth
// @Accept json
// @Produce json
// @Param request body models.LoginRequest true "Login credentials"
// @Success 200 {object} utils.Response
// @Failure 400 {object} utils.Response
// @Failure 401 {object} utils.Response
// @Failure 403 {object} utils.Response
// @Failure 500 {object} utils.Response
// @Router /auth/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var req models.LoginRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequestResponse(c, "Invalid request format")
		return
	}

	if validationErrs := validator.ValidateStruct(&req); validationErrs != nil {
		utils.ValidationErrorResponse(c, validationErrs.ToMap())
		return
	}

	// Authenticate user
	user, err := h.authService.Login(c.Request.Context(), &req)
	if err != nil {
		var appErr *models.AppError
		if errors.As(err, &appErr) {
			utils.ErrorResponse(c, appErr.StatusCode, appErr.Code, appErr.Message, appErr)
			return
		}
	}

	// Update last login time asynchronously (don't block response)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := h.authService.UpdateLastLogin(ctx, user.ID); err != nil {
			var appErr *models.AppError
			if errors.As(err, &appErr) {
				utils.ErrorResponse(c, appErr.StatusCode, appErr.Code, appErr.Message, appErr)
				return
			}
		}
	}()

	// Generate token
	token, err := h.generateToken(user)
	if err != nil {
		utils.InternalServerErrorResponse(c, err)
		return
	}

	// Set auth cookie
	h.setAuthCookie(c, token)

	utils.SuccessResponse(c, http.StatusOK, "Login successful", gin.H{
		"user": user.ToResponse(),
	})
}

// Logout godoc
// @Summary User logout
// @Description Clears authentication cookie
// @Tags auth
// @Produce json
// @Success 200 {object} utils.Response
// @Router /auth/logout [post]
func (h *AuthHandler) Logout(c *gin.Context) {
	// Get user info if available (optional logging)
	if _, exists := middleware.GetUserIDFromContext(c); exists {
		utils.CreatedResponse(c, "Logged out", nil, "Token removed successfully")
	}

	h.clearAuthCookie(c)
	utils.SuccessResponse(c, http.StatusOK, "Logged out successfully", nil)
}

// RefreshToken godoc
// @Summary Refresh authentication token
// @Description Generates a new JWT token and updates the cookie
// @Tags auth
// @Produce json
// @Security BearerAuth
// @Success 200 {object} utils.Response
// @Failure 401 {object} utils.Response
// @Failure 500 {object} utils.Response
// @Router /auth/refresh [post]
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	// Get user info from context (set by auth middleware)
	userID, exists := middleware.GetUserIDFromContext(c)
	if !exists {
		utils.UnauthorizedResponse(c, "Authentication required")
		return
	}

	email, _ := middleware.GetUserEmailFromContext(c)
	role, _ := middleware.GetUserRoleFromContext(c)

	// Generate new token
	token, err := middleware.GenerateToken(userID, email, role, h.jwtSecret, h.jwtIssuer, h.jwtExpiry)
	if err != nil {
		utils.InternalServerErrorResponse(c, err)
		return
	}

	// Update cookie with new token
	h.setAuthCookie(c, token)

	utils.SuccessResponse(c, http.StatusOK, "Token refreshed successfully", nil)
}

// UpdateUser godoc
// @Summary Update user profile
// @Description Updates user information (name, phone number)
// @Tags auth
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body models.UpdateUserRequest true "User update details"
// @Success 200 {object} utils.Response
// @Failure 400 {object} utils.Response
// @Failure 404 {object} utils.Response
// @Failure 409 {object} utils.Response
// @Failure 500 {object} utils.Response
// @Router /auth/user [put]
func (h *AuthHandler) UpdateUser(c *gin.Context) {
	var req models.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequestResponse(c, "Invalid request format")
		return
	}

	if validationErrs := validator.ValidateStruct(&req); validationErrs != nil {
		utils.ValidationErrorResponse(c, validationErrs.ToMap())
		return
	}

	// Get user ID from auth middleware
	userID, exists := middleware.GetUserIDFromContext(c)
	if !exists {
		utils.UnauthorizedResponse(c, "Authentication required")
		return
	}
	req.ID = userID

	// Ensure at least one field provided
	if req.Name == nil && req.PhoneNumber == nil {
		utils.BadRequestResponse(c, "At least one field must be provided for update")
		return
	}

	// Pass directly to service (validation handled there)
	user, err := h.authService.Update(c.Request.Context(), &req)
	if err != nil {
		var appErr *models.AppError
		if errors.As(err, &appErr) {
			utils.ErrorResponse(c, appErr.StatusCode, appErr.Code, appErr.Message, appErr)
			return
		}
	}

	utils.SuccessResponse(c, http.StatusOK, "User updated successfully", gin.H{
		"user": user.ToResponse(),
	})
}

// ChangePassword godoc
// @Summary Change user password
// @Description Allows authenticated users to change their password
// @Tags auth
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body models.ChangePasswordRequest true "Password change details"
// @Success 200 {object} utils.Response
// @Failure 400 {object} utils.Response
// @Failure 401 {object} utils.Response
// @Failure 500 {object} utils.Response
// @Router /auth/change-password [post]
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	var req models.ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequestResponse(c, "Invalid request format")
		return
	}
	// Validate request structure
	if validationErrs := validator.ValidateStruct(req); validationErrs != nil {
		utils.ValidationErrorResponse(c, validationErrs.ToMap())
		return
	}

	// Get user ID from context
	userID, exists := middleware.GetUserIDFromContext(c)
	if !exists {
		utils.UnauthorizedResponse(c, "Authentication required")
		return
	}

	// Additional password validation
	if err := validator.ValidatePassword(req.NewPassword); err != nil {
		utils.BadRequestResponse(c, err.Error())
		return
	}

	// Ensure new password is different from current
	if req.CurrentPassword == req.NewPassword {
		utils.BadRequestResponse(c, "New password must be different from current password")
		return
	}

	// Change password
	err := h.authService.ChangePassword(c.Request.Context(), userID, &req)
	if err != nil {
		var appErr *models.AppError
		if errors.As(err, &appErr) {
			utils.ErrorResponse(c, appErr.StatusCode, appErr.Code, appErr.Message, appErr)
			return
		}
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Password changed successfully", nil)
}

// ForgetPassword godoc
// @Summary Initiate password reset
// @Description Allows users to initiate a password reset process
// @Tags auth
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body models.ForgotPasswordRequest true "Password reset details"
// @Success 200 {object} utils.Response
// @Failure 400 {object} utils.Response
// @Failure 401 {object} utils.Response
// @Failure 500 {object} utils.Response
// @Router /auth/forgot-password [post]
// func (h *AuthHandler) ForgetPassword(c *gin.Context) {
// 	var req models.ForgotPasswordRequest
// 	if err := c.ShouldBindJSON(&req); err != nil {
// 		utils.BadRequestResponse(c, "Invalid request format")
// 		return
// 	}
// 	// Validate request structure
// 	if validationErrs := validator.ValidateStruct(req); validationErrs != nil {
// 		utils.ValidationErrorResponse(c, validationErrs.ToMap())
// 		return
// 	}
// 	// Initiate password reset
// 	err := h.authService.ForgotPassword(c.Request.Context(), &req)
// 	if err != nil {
// 		var appErr *models.AppError
// 		if errors.As(err, &appErr) {
// 			utils.ErrorResponse(c, appErr.StatusCode, appErr.Code, appErr.Message, appErr)
// 			return
// 		}
// 		return
// 	}

// 	utils.SuccessResponse(c, http.StatusOK, "Password reset initiated successfully", nil)
// }

// ==================== HELPER METHODS ====================

// generateToken creates a JWT token for the user
func (h *AuthHandler) generateToken(user *models.User) (string, error) {
	return middleware.GenerateToken(
		user.ID,
		user.Email,
		user.Role,
		h.jwtSecret,
		h.jwtIssuer,
		h.jwtExpiry,
	)
}
