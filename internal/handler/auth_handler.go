package handler

import (
	"context"
	"go-server/internal/middleware"
	"go-server/internal/models"
	authservice "go-server/internal/service/auth_service"
	"go-server/pkg/utils"
	"go-server/pkg/validator"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// ==================== HANDLER STRUCT ====================

type AuthHandler struct {
	registrationService authservice.RegistrationService
	verificationService authservice.VerificationService
	authService         authservice.AuthService
	*TokenHandler
}

// ==================== CONSTRUCTOR ====================

func NewAuthHandler(
	registrationService authservice.RegistrationService,
	verificationService authservice.VerificationService,
	authService authservice.AuthService,
	jwtSecret string,
	jwtIssuer string,
	accessExpiry time.Duration,
	refreshExpiry time.Duration,
	isProduction bool,
) *AuthHandler {
	return &AuthHandler{
		registrationService: registrationService,
		verificationService: verificationService,
		authService:         authService,
		TokenHandler:        NewTokenHandler(jwtSecret, jwtIssuer, accessExpiry, refreshExpiry, isProduction),
	}
}

// ==================== COOKIE HELPERS ====================

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
	if !bindJSON(c, &req) {
		return
	}

	req.Sanitize()

	if validationErrs := validator.ValidateStruct(&req); validationErrs != nil {
		utils.ValidationErrorResponse(c, validationErrs.ToMap())
		return
	}

	user, err := h.registrationService.Register(c.Request.Context(), &req)
	if handleServiceError(c, err) {
		return
	}

	utils.SuccessResponse(c, http.StatusCreated, "Account created successfully", gin.H{
		"user":    user.ToResponse(),
		"message": "Please verify your email using the OTP sent to your email address",
	})
}

// VerifyOTP godoc
// @Summary Verify OTP
// @Description Verifies OTP and marks user as verified, returns access + refresh tokens
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

	if !bindJSON(c, &req) {
		return
	}

	req.Sanitize()

	if validationErrs := validator.ValidateStruct(&req); validationErrs != nil {
		utils.ValidationErrorResponse(c, validationErrs.ToMap())
		return
	}

	user, err := h.verificationService.VerifyOTP(c.Request.Context(), &req)
	if handleServiceError(c, err) {
		return
	}

	if err := h.issueBothTokens(c, user); err != nil {
		utils.InternalServerErrorResponse(c, err)
		return
	}

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

	if !bindJSON(c, &req) {
		return
	}

	req.Sanitize()

	if validationErrs := validator.ValidateStruct(&req); validationErrs != nil {
		utils.ValidationErrorResponse(c, validationErrs.ToMap())
		return
	}

	err := h.verificationService.ResendOTP(c.Request.Context(), &req)
	if handleServiceError(c, err) {
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "OTP has been resent to your email", nil)
}

// ==================== LOGIN / LOGOUT ====================

// Login godoc
// @Summary User login
// @Description Authenticates user and returns access + refresh tokens as cookies
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

	if !bindJSON(c, &req) {
		return
	}

	req.Sanitize()

	// Custom struct validation (at least one identifier must be provided)
	if err := req.Validate(); err != nil {
		utils.SingleFieldValidationError(c, "identifier", err.Error())
		return
	}

	if validationErrs := validator.ValidateStruct(&req); validationErrs != nil {
		utils.ValidationErrorResponse(c, validationErrs.ToMap())
		return
	}

	user, err := h.authService.Login(c.Request.Context(), &req)
	if handleServiceError(c, err) {
		return
	}

	// Update last login asynchronously — don't block the response
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = h.authService.UpdateLastLogin(ctx, user.ID)
	}()

	if err := h.issueBothTokens(c, user); err != nil {
		utils.InternalServerErrorResponse(c, err)
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Login successful", gin.H{
		"user": user.ToResponse(),
	})
}

// Logout godoc
// @Summary User logout
// @Description Clears both access and refresh token cookies
// @Tags auth
// @Produce json
// @Success 200 {object} utils.Response
// @Router /auth/logout [post]
func (h *AuthHandler) Logout(c *gin.Context) {
	h.clearAccessCookie(c)
	h.clearRefreshCookie(c)
	utils.SuccessResponse(c, http.StatusOK, "Logged out successfully", nil)
}

// ==================== TOKEN REFRESH ====================

// RefreshToken godoc
// @Summary Refresh access token
// @Description Uses the long-lived refresh_token cookie to issue a new access_token
// @Tags auth
// @Produce json
// @Success 200 {object} utils.Response
// @Failure 401 {object} utils.Response
// @Failure 500 {object} utils.Response
// @Router /auth/refresh [post]
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	// User info is set by RefreshMiddleware (reads refresh_token cookie)
	userID, exists := middleware.GetUserIDFromContext(c)
	if !exists {
		utils.UnauthorizedResponse(c, "Authentication required")
		return
	}

	email, _ := middleware.GetUserEmailFromContext(c)
	role, _ := middleware.GetUserRoleFromContext(c)

	// Issue a new access token only
	// Refresh token stays alive — no rotation unless you want it
	accessToken, err := middleware.GenerateToken(
		userID,
		email,
		role,
		middleware.TokenTypeAccess,
		h.jwtSecret,
		h.jwtIssuer,
		h.accessExpiry,
	)
	if err != nil {
		utils.InternalServerErrorResponse(c, err)
		return
	}

	h.setAccessCookie(c, accessToken)
	utils.SuccessResponse(c, http.StatusOK, "Token refreshed successfully", nil)
}

// ==================== USER MANAGEMENT ====================

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
	if !bindJSON(c, &req) {
		return
	}

	req.Sanitize()

	// Custom validation: ensure at least one field is provided
	if err := req.Validate(); err != nil {
		utils.BadRequestResponse(c, err.Error())
		return
	}

	if validationErrs := validator.ValidateStruct(&req); validationErrs != nil {
		utils.ValidationErrorResponse(c, validationErrs.ToMap())
		return
	}

	userID, exists := middleware.GetUserIDFromContext(c)
	if !exists {
		utils.UnauthorizedResponse(c, "Authentication required")
		return
	}
	req.ID = userID

	user, err := h.authService.Update(c.Request.Context(), &req)
	if handleServiceError(c, err) {
		return
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
	if !bindJSON(c, &req) {
		return
	}

	// struct tags already enforce: strong_password, nefield=CurrentPassword, eqfield=NewPassword
	if validationErrs := validator.ValidateStruct(&req); validationErrs != nil {
		utils.ValidationErrorResponse(c, validationErrs.ToMap())
		return
	}

	userID, exists := middleware.GetUserIDFromContext(c)
	if !exists {
		utils.UnauthorizedResponse(c, "Authentication required")
		return
	}

	err := h.authService.ChangePassword(c.Request.Context(), userID, &req)
	if handleServiceError(c, err) {
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Password changed successfully", nil)
}

// ==================== PASSWORD RESET ====================

// ForgetPassword godoc
// @Summary Initiate password reset
// @Description Sends a password reset OTP to the user's email
// @Tags auth
// @Accept json
// @Produce json
// @Param request body models.ForgotPasswordRequest true "Password reset details"
// @Success 200 {object} utils.Response
// @Failure 400 {object} utils.Response
// @Failure 404 {object} utils.Response
// @Failure 500 {object} utils.Response
// @Router /auth/forget-password [post]
func (h *AuthHandler) ForgetPassword(c *gin.Context) {
	var req models.ForgotPasswordRequest
	if !bindJSON(c, &req) {
		return
	}

	req.Sanitize()

	if validationErrs := validator.ValidateStruct(&req); validationErrs != nil {
		utils.ValidationErrorResponse(c, validationErrs.ToMap())
		return
	}

	err := h.registrationService.ForgetPassword(c.Request.Context(), &req)
	if handleServiceError(c, err) {
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Password reset initiated successfully", nil)
}

// ResetPassword godoc
// @Summary Reset user password
// @Description Resets password using OTP token
// @Tags auth
// @Accept json
// @Produce json
// @Param request body models.ResetPasswordRequest true "Reset password details"
// @Success 200 {object} utils.Response
// @Failure 400 {object} utils.Response
// @Failure 500 {object} utils.Response
// @Router /auth/reset-password [post]
func (h *AuthHandler) ResetPassword(c *gin.Context) {
	var req models.ResetPasswordRequest
	if !bindJSON(c, &req) {
		return
	}

	req.Sanitize()

	// struct tags already enforce: strong_password on new_password, eqfield=NewPassword on confirm_password
	if validationErrs := validator.ValidateStruct(&req); validationErrs != nil {
		utils.ValidationErrorResponse(c, validationErrs.ToMap())
		return
	}

	err := h.verificationService.ResetPassword(c.Request.Context(), &req)
	if handleServiceError(c, err) {
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Password reset successfully", nil)
}