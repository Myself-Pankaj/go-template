// pkg/validator/validator.go
package validator

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"unicode"

	"github.com/go-playground/validator/v10"
)

var (
	validate *validator.Validate
	
	// Email regex pattern (RFC 5322 simplified)
	emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	
	// Phone regex - supports international format with + and digits
	phoneRegex = regexp.MustCompile(`^\+?[1-9]\d{1,14}$`)
)

func init() {
	validate = validator.New()

	// Register custom tag name func to use json tags for better error messages
	validate.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})

	// Register custom validators
	registerCustomValidators()
}

// registerCustomValidators registers all custom validation functions
func registerCustomValidators() {
	_ = validate.RegisterValidation("phone_intl", validatePhoneIntl)
	_ = validate.RegisterValidation("strong_password", validateStrongPassword)
	_ = validate.RegisterValidation("otp_code", validateOTPCode)
	_ = validate.RegisterValidation("user_role", validateUserRole)
	_ = validate.RegisterValidation("no_whitespace", validateNoWhitespace)
	_ = validate.RegisterValidation("alphanumeric_space", validateAlphanumericSpace)
}

// ==================== CUSTOM VALIDATORS ====================

// validatePhone validates phone numbers (E.164 format recommended)
// Allows: +1234567890, +919876543210, etc.
// Regex: +<countrycode 1-3 digits> followed by exactly 10 digits
var phoneIntlRegex = regexp.MustCompile(`^\+\d{1,3}[0-9]{10}$`)

// validatePhoneIntl validates phone numbers like +911234567890, +11234567890
func validatePhoneIntl(fl validator.FieldLevel) bool {
    phone := strings.TrimSpace(fl.Field().String())
    if phone == "" {
        return false // required
    }
    return phoneIntlRegex.MatchString(phone)
}

// validateStrongPassword ensures password meets security requirements
// Requirements: min 8 chars, 1 uppercase, 1 lowercase, 1 number, 1 special char
func validateStrongPassword(fl validator.FieldLevel) bool {
	password := fl.Field().String()

	if len(password) < 8 {
		return false
	}

	var (
		hasUpper   bool
		hasLower   bool
		hasNumber  bool
		hasSpecial bool
	)

	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsNumber(char):
			hasNumber = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}

	return hasUpper && hasLower && hasNumber && hasSpecial
}

// validateOTPCode validates OTP format (6 digits)
func validateOTPCode(fl validator.FieldLevel) bool {
	otp := fl.Field().String()
	if len(otp) != 6 {
		return false
	}

	// Must be exactly 6 digits
	for _, char := range otp {
		if !unicode.IsDigit(char) {
			return false
		}
	}

	return true
}

// validateUserRole validates user role against allowed values
func validateUserRole(fl validator.FieldLevel) bool {
	role := fl.Field().String()
	allowedRoles := []string{"user", "admin", "moderator"}

	for _, allowedRole := range allowedRoles {
		if role == allowedRole {
			return true
		}
	}

	return false
}

// validateNoWhitespace ensures field has no whitespace
func validateNoWhitespace(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	return !strings.Contains(value, " ")
}

// validateAlphanumericSpace allows only alphanumeric characters and spaces
func validateAlphanumericSpace(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	for _, char := range value {
		if !unicode.IsLetter(char) && !unicode.IsNumber(char) && !unicode.IsSpace(char) {
			return false
		}
	}
	return true
}

// ==================== VALIDATION TYPES ====================

// ValidationError represents a single validation error
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Tag     string `json:"tag,omitempty"`
	Value   string `json:"value,omitempty"`
}

// ValidationErrors represents multiple validation errors
type ValidationErrors []ValidationError

// ToMap converts ValidationErrors to map[string]interface{}
func (ve ValidationErrors) ToMap() map[string]interface{} {
	result := make(map[string]interface{}, len(ve))
	for _, err := range ve {
		result[err.Field] = err.Message
	}
	return result
}

// ToStringMap converts ValidationErrors to map[string]string
func (ve ValidationErrors) ToStringMap() map[string]string {
	result := make(map[string]string, len(ve))
	for _, err := range ve {
		result[err.Field] = err.Message
	}
	return result
}

// Error implements the error interface
func (ve ValidationErrors) Error() string {
	if len(ve) == 0 {
		return ""
	}
	
	var messages []string
	for _, err := range ve {
		messages = append(messages, fmt.Sprintf("%s: %s", err.Field, err.Message))
	}
	return strings.Join(messages, "; ")
}

// ==================== VALIDATION FUNCTIONS ====================

// ValidateStruct validates a struct and returns formatted errors
func ValidateStruct(s interface{}) ValidationErrors {
	if s == nil {
		return ValidationErrors{{
			Field:   "request",
			Message: "request body cannot be empty",
		}}
	}

	err := validate.Struct(s)
	if err == nil {
		return nil
	}

	// Handle non-validation errors
	if _, ok := err.(*validator.InvalidValidationError); ok {
		return ValidationErrors{{
			Field:   "validation",
			Message: "invalid validation error",
		}}
	}

	var validationErrors ValidationErrors
	for _, err := range err.(validator.ValidationErrors) {
		validationErrors = append(validationErrors, ValidationError{
			Field:   err.Field(),
			Message: formatErrorMessage(err),
			Tag:     err.Tag(),
		})
	}

	return validationErrors
}

// ValidateVar validates a single variable
func ValidateVar(field interface{}, tag string) error {
	return validate.Var(field, tag)
}

// Validate is a convenience function that returns true if validation passes
func Validate(s interface{}) bool {
	return validate.Struct(s) == nil
}

// GetValidator returns the validator instance for custom registrations
func GetValidator() *validator.Validate {
	return validate
}

// ==================== ERROR MESSAGE FORMATTING ====================

// formatErrorMessage formats validation error messages with user-friendly text
func formatErrorMessage(err validator.FieldError) string {
	field := err.Field()
	param := err.Param()

	switch err.Tag() {
	case "required":
		return fmt.Sprintf("%s is required", field)
	
	case "required_without":
		return fmt.Sprintf("%s is required when %s is not provided", field, param)
	
	case "required_with":
		return fmt.Sprintf("%s is required when %s is provided", field, param)

	case "email":
		return fmt.Sprintf("%s must be a valid email address", field)

	case "min":
		if err.Type().Kind() == reflect.String {
			return fmt.Sprintf("%s must be at least %s characters long", field, param)
		}
		return fmt.Sprintf("%s must be at least %s", field, param)

	case "max":
		if err.Type().Kind() == reflect.String {
			return fmt.Sprintf("%s must not exceed %s characters", field, param)
		}
		return fmt.Sprintf("%s must not exceed %s", field, param)

	case "len":
		if err.Type().Kind() == reflect.String {
			return fmt.Sprintf("%s must be exactly %s characters long", field, param)
		}
		return fmt.Sprintf("%s must be exactly %s", field, param)

	case "phone_intl":
		return fmt.Sprintf("%s must be a valid phone number (e.g., +1234567890)", field)

	case "strong_password":
		return fmt.Sprintf("%s must contain at least 8 characters with uppercase, lowercase, number, and special character", field)

	case "otp_code":
		return fmt.Sprintf("%s must be a valid 6-digit OTP code", field)

	case "user_role":
		return fmt.Sprintf("%s must be one of: user, admin, moderator", field)

	case "oneof":
		return fmt.Sprintf("%s must be one of: %s", field, strings.ReplaceAll(param, " ", ", "))

	case "gte":
		return fmt.Sprintf("%s must be greater than or equal to %s", field, param)

	case "lte":
		return fmt.Sprintf("%s must be less than or equal to %s", field, param)

	case "gt":
		return fmt.Sprintf("%s must be greater than %s", field, param)

	case "lt":
		return fmt.Sprintf("%s must be less than %s", field, param)

	case "eqfield":
		return fmt.Sprintf("%s must be equal to %s", field, param)

	case "nefield":
		return fmt.Sprintf("%s must not be equal to %s", field, param)

	case "alpha":
		return fmt.Sprintf("%s must contain only alphabetic characters", field)

	case "alphanum":
		return fmt.Sprintf("%s must contain only alphanumeric characters", field)

	case "alphanumeric_space":
		return fmt.Sprintf("%s must contain only letters, numbers, and spaces", field)

	case "numeric":
		return fmt.Sprintf("%s must contain only numbers", field)

	case "url":
		return fmt.Sprintf("%s must be a valid URL", field)

	case "uri":
		return fmt.Sprintf("%s must be a valid URI", field)

	case "no_whitespace":
		return fmt.Sprintf("%s must not contain whitespace", field)

	case "lowercase":
		return fmt.Sprintf("%s must be in lowercase", field)

	case "uppercase":
		return fmt.Sprintf("%s must be in uppercase", field)

	case "contains":
		return fmt.Sprintf("%s must contain '%s'", field, param)

	case "containsany":
		return fmt.Sprintf("%s must contain at least one of: %s", field, param)

	case "excludes":
		return fmt.Sprintf("%s must not contain '%s'", field, param)

	case "startswith":
		return fmt.Sprintf("%s must start with '%s'", field, param)

	case "endswith":
		return fmt.Sprintf("%s must end with '%s'", field, param)

	default:
		return fmt.Sprintf("%s is invalid", field)
	}
}

// ==================== HELPER FUNCTIONS ====================

// IsValidEmail checks if a string is a valid email
func IsValidEmail(email string) bool {
	email = strings.TrimSpace(strings.ToLower(email))
	return emailRegex.MatchString(email)
}

// IsValidPhone checks if a string is a valid phone number
func IsValidPhone(phone string) bool {
	phone = strings.TrimSpace(phone)
	return phoneRegex.MatchString(phone)
}

// SanitizeEmail normalizes email address
func SanitizeEmail(email string) string {
	return strings.TrimSpace(strings.ToLower(email))
}

// SanitizePhone removes common phone formatting characters
func SanitizePhone(phone string) string {
	phone = strings.TrimSpace(phone)
	phone = strings.ReplaceAll(phone, " ", "")
	phone = strings.ReplaceAll(phone, "-", "")
	phone = strings.ReplaceAll(phone, "(", "")
	phone = strings.ReplaceAll(phone, ")", "")
	return phone
}

// ValidatePassword checks password strength manually
func ValidatePassword(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters long")
	}

	var (
		hasUpper   bool
		hasLower   bool
		hasNumber  bool
		hasSpecial bool
	)

	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsNumber(char):
			hasNumber = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}

	if !hasUpper {
		return fmt.Errorf("password must contain at least one uppercase letter")
	}
	if !hasLower {
		return fmt.Errorf("password must contain at least one lowercase letter")
	}
	if !hasNumber {
		return fmt.Errorf("password must contain at least one number")
	}
	if !hasSpecial {
		return fmt.Errorf("password must contain at least one special character")
	}

	return nil
}