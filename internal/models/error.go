package models

import "fmt"

// AppError represents a custom application error
type AppError struct {
	Code       string
	Message    string
	StatusCode int
	Internal   error
}

func (e *AppError) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Internal)
	}
	return e.Message
}

// NewAppError creates a new AppError
func NewAppError(code, message string, statusCode int, err error) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		StatusCode: statusCode,
		Internal:   err,
	}
}

// Common error codes
const (
	// Client errors (4xx)
	ErrCodeBadRequest      = "BAD_REQUEST"
	ErrCodeUnauthorized    = "UNAUTHORIZED"
	ErrCodeForbidden       = "FORBIDDEN"
	ErrCodeNotFound        = "NOT_FOUND"
	ErrCodeConflict        = "CONFLICT"
	ErrCodeValidation      = "VALIDATION_ERROR"
	ErrCodeInvalidToken    = "INVALID_TOKEN"
	ErrCodeTokenExpired    = "TOKEN_EXPIRED"
	ErrCodeOTPExpired      = "OTP_EXPIRED"
	ErrCodeInvalidOTP      = "INVALID_OTP"
	ErrCodeUserNotVerified = "USER_NOT_VERIFIED"
	// Email errors
	ErrCodeEmailSendFailed = "EMAIL_SEND_FAILED"
	// Server errors (5xx)
	ErrCodeInternalServer     = "INTERNAL_SERVER_ERROR"
	ErrCodeServiceUnavailable = "SERVICE_UNAVAILABLE"
	ErrCodeDatabaseError      = "DATABASE_ERROR"
	ErrCodeTransactionFailed  = "TRANSACTION_FAILED"
	// Business logic errors for auth module
	ErrCodeUserExists         = "USER_ALREADY_EXISTS"
	ErrCodeInvalidCredentials = "INVALID_CREDENTIALS"
	ErrCodePhoneExists        = "PHONE_ALREADY_EXISTS"
	ErrCodeEmailExists        = "EMAIL_ALREADY_EXISTS"
	ErrUserNotVerified        = "USER_NOT_VERIFIED"
	ErrCodeEmailNotVerified   = "EMAIL_NOT_VERIFIED"
	ErrCodeTooManyRequests    = "TOO_MANY_REQUESTS"

	ErrCodeRateLimited   = "RATE_LIMITED"
	ErrMissingIdentifier = "MISSING_IDENTIFIER"

	ErrCodeLimitExceeded = "SUBSCRIBE_TO_PREMIUM"

	// Subscription / plan errors
	ErrCodeSubscriptionRequired = "SUBSCRIPTION_REQUIRED"
)
