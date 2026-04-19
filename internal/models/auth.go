// internal/models/auth_models.go
package models

import (
	"errors"
	"strings"
)

// ==================== REGISTRATION ====================

// RegisterRequest represents user registration input
type RegisterRequest struct {
	Name        string `json:"name" validate:"required,min=2,max=100,alphanumeric_space"`
	Email       string `json:"email" validate:"required,email,max=255"`
	PhoneNumber string `json:"phone_number" validate:"required,min=10,phone_intl"`
	Password    string `json:"password" validate:"required,strong_password,min=8,max=128"`
	Role        string `json:"role,omitempty" validate:"omitempty,user_role"`
}

// Sanitize cleans and normalizes registration data
func (r *RegisterRequest) Sanitize() {
	r.Name = strings.TrimSpace(r.Name)
	r.Email = strings.TrimSpace(strings.ToLower(r.Email))
	r.PhoneNumber = strings.TrimSpace(r.PhoneNumber)
	r.Role = strings.TrimSpace(strings.ToLower(r.Role))
}



// ==================== LOGIN ====================

// LoginRequest represents user login input
type LoginRequest struct {
	Email       *string `json:"email,omitempty" validate:"omitempty,email,max=255"`
	PhoneNumber *string `json:"phone_number,omitempty" validate:"omitempty,phone_intl"`
	Password    string  `json:"password" validate:"required,min=8,max=128"`
}

// Validate performs custom validation for LoginRequest
func (r *LoginRequest) Validate() error {
	// At least one identifier must be provided
	if (r.Email == nil || *r.Email == "") && (r.PhoneNumber == nil || *r.PhoneNumber == "") {
		return errors.New("either email or phone_number must be provided")
	}
	return nil
}

// Sanitize cleans and normalizes login data
func (r *LoginRequest) Sanitize() {
	if r.Email != nil {
		email := strings.TrimSpace(strings.ToLower(*r.Email))
		r.Email = &email
	}
	
	if r.PhoneNumber != nil {
		phone := strings.TrimSpace(*r.PhoneNumber)
		r.PhoneNumber = &phone
	}
}

// ==================== OTP VERIFICATION ====================

// VerifyOTPRequest represents OTP verification input
type VerifyOTPRequest struct {
	Email string `json:"email" validate:"required,email,max=255"`
	OTP   string `json:"otp" validate:"required,otp_code"`
}

// Sanitize cleans and normalizes OTP verification data
func (r *VerifyOTPRequest) Sanitize() {
	r.Email = strings.TrimSpace(strings.ToLower(r.Email))
	r.OTP = strings.TrimSpace(r.OTP)
}

// ResendOTPRequest represents resend OTP input
type ResendOTPRequest struct {
	Email string `json:"email" validate:"required,email,max=255"`
}

// Sanitize cleans and normalizes resend OTP data
func (r *ResendOTPRequest) Sanitize() {
	r.Email = strings.TrimSpace(strings.ToLower(r.Email))
}

// ==================== USER UPDATE ====================

// UpdateUserRequest represents user profile update input
type UpdateUserRequest struct {
	ID          int64   `json:"-"` // Set from context, not from request body
	Name        *string `json:"name,omitempty" validate:"omitempty,min=2,max=100,alphanumeric_space"`
	PhoneNumber *string `json:"phone_number,omitempty" validate:"omitempty,phone_intl"`
}

// Sanitize cleans and normalizes update data
func (r *UpdateUserRequest) Sanitize() {
	if r.Name != nil {
		name := strings.TrimSpace(*r.Name)
		r.Name = &name
	}
	
	if r.PhoneNumber != nil {
		phone := strings.TrimSpace(*r.PhoneNumber)
		r.PhoneNumber = &phone
	}
}

// Validate performs custom validation for UpdateUserRequest
func (r *UpdateUserRequest) Validate() error {
	// At least one field must be provided for update
	if r.Name == nil && r.PhoneNumber == nil {
		return errors.New("at least one field (name or phone_number) must be provided for update")
	}
	
	// If name is provided, it should not be empty
	if r.Name != nil && strings.TrimSpace(*r.Name) == "" {
		return errors.New("name cannot be empty")
	}
	
	return nil
}

// ==================== PASSWORD MANAGEMENT ====================

// ChangePasswordRequest represents password change input
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" validate:"required,min=8,max=128"`
	NewPassword     string `json:"new_password" validate:"required,strong_password,min=8,max=128,nefield=CurrentPassword"`
	ConfirmPassword string `json:"confirm_password" validate:"required,eqfield=NewPassword"`
}

// Sanitize cleans password change data
func (r *ChangePasswordRequest) Sanitize() {
	// Passwords should not be trimmed as spaces might be intentional
}

// ForgotPasswordRequest represents forgot password input
type ForgotPasswordRequest struct {
	Email string `json:"email" validate:"required,email,max=255"`
}

// Sanitize cleans forgot password data
func (r *ForgotPasswordRequest) Sanitize() {
	r.Email = strings.TrimSpace(strings.ToLower(r.Email))
}

// ResetPasswordRequest represents password reset input
type ResetPasswordRequest struct {
	Email           string `json:"email" validate:"required,email,max=255"`
	ResetToken      string `json:"reset_token" validate:"required,min=6,max=10"`
	NewPassword     string `json:"new_password" validate:"required,strong_password,min=8,max=128"`
	ConfirmPassword string `json:"confirm_password" validate:"required,eqfield=NewPassword"`
}

// Sanitize cleans password reset data
func (r *ResetPasswordRequest) Sanitize() {
	r.Email = strings.TrimSpace(strings.ToLower(r.Email))
	r.ResetToken = strings.TrimSpace(r.ResetToken)
}

// ==================== EMAIL VERIFICATION ====================

// VerifyEmailRequest represents email verification with token
type VerifyEmailRequest struct {
	Email string `json:"email" validate:"required,email,max=255"`
	Token string `json:"token" validate:"required,min=32,max=64"`
}

// Sanitize cleans email verification data
func (r *VerifyEmailRequest) Sanitize() {
	r.Email = strings.TrimSpace(strings.ToLower(r.Email))
	r.Token = strings.TrimSpace(r.Token)
}

// ==================== HELPER FUNCTIONS ====================

// ValidateAndSanitize validates and sanitizes any request that implements both interfaces
func ValidateAndSanitize(req interface{}) error {
	// Sanitize first
	if sanitizer, ok := req.(interface{ Sanitize() }); ok {
		sanitizer.Sanitize()
	}
	
	// Then validate
	if validator, ok := req.(interface{ Validate() error }); ok {
		if err := validator.Validate(); err != nil {
			return err
		}
	}
	
	return nil
}