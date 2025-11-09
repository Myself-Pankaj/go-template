package utils

import (
	"go-server/internal/models"
	"go-server/pkg/logger"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ResponseConfig holds configuration for response utilities
type ResponseConfig struct {
	// EnableDetailedErrors includes detailed error info in dev mode
	EnableDetailedErrors bool
	// EnableErrorLogging logs all errors
	EnableErrorLogging bool
	// IncludeRequestID includes request ID in response
	IncludeRequestID bool
}

var config = ResponseConfig{
	EnableDetailedErrors: false,
	EnableErrorLogging:   true,
	IncludeRequestID:     true,
}

// SetResponseConfig updates the global response configuration
func SetResponseConfig(cfg ResponseConfig) {
	config = cfg
}

// SuccessResponse sends a standardized success response
func SuccessResponse(c *gin.Context, statusCode int, message string, data interface{}) {
	response := models.APIResponse{
		Success: true,
		Message: message,
		Data:    data,
	}

	// Add request ID if enabled
	if config.IncludeRequestID {
		if requestID := c.GetString("request_id"); requestID != "" {
			c.Header("X-Request-ID", requestID)
		}
	}

	c.JSON(statusCode, response)
}

// ErrorResponse sends a standardized error response
func ErrorResponse(c *gin.Context, statusCode int, code, message string, err error) {
	// Build log fields
	fields := []zap.Field{
		zap.Int("status_code", statusCode),
		zap.String("error_code", code),
		zap.String("message", message),
		zap.String("method", c.Request.Method),
		zap.String("path", c.Request.URL.Path),
		zap.String("ip", c.ClientIP()),
	}

	// Add request ID
	if requestID := c.GetString("request_id"); requestID != "" {
		fields = append(fields, zap.String("request_id", requestID))
		if config.IncludeRequestID {
			c.Header("X-Request-ID", requestID)
		}
	}

	// Add user ID if available
	if userID := c.GetString("user_id"); userID != "" {
		fields = append(fields, zap.String("user_id", userID))
	}

	// Add error details if provided
	if err != nil {
		fields = append(fields, zap.Error(err))
	}

	// Log based on status code severity
	if config.EnableErrorLogging {
		switch {
		case statusCode >= 500:
			logger.GetLogger().WithOptions(zap.AddCallerSkip(1)).Error("Server error", fields...)
		case statusCode >= 400:
			logger.GetLogger().WithOptions(zap.AddCallerSkip(1)).Warn("Client error", fields...)
		default:
			logger.GetLogger().WithOptions(zap.AddCallerSkip(1)).Info("Error response", fields...)
		}
	}

	// Build error response
	errorData := &models.ErrorData{
		Code:    code,
		Message: message,
	}

	// Include detailed error info in development
	if config.EnableDetailedErrors && err != nil {
		errorData.Details = map[string]interface{}{
			"error": err.Error(),
		}
	}

	response := models.APIResponse{
		Success: false,
		Error:   errorData,
	}

	c.JSON(statusCode, response)
}

// PaginatedResponse sends a paginated success response
func PaginatedResponse(c *gin.Context, statusCode int, message string, pagination *models.PaginationResponse) {
	// Validate pagination data
	if pagination == nil {
		ErrorResponse(c, http.StatusInternalServerError, models.ErrCodeInternalServer, "Invalid pagination data", nil)
		return
	}

	// Add request ID
	if config.IncludeRequestID {
		if requestID := c.GetString("request_id"); requestID != "" {
			c.Header("X-Request-ID", requestID)
		}
	}

	response := models.APIResponse{
		Success: true,
		Message: message,
		Data:    pagination,
	}

	c.JSON(statusCode, response)
}

// AppErrorResponse handles AppError type specifically
func AppErrorResponse(c *gin.Context, appErr *models.AppError) {
	if appErr == nil {
		ErrorResponse(c, http.StatusInternalServerError, models.ErrCodeInternalServer, "Unknown error occurred", nil)
		return
	}

	ErrorResponse(c, appErr.StatusCode, appErr.Code, appErr.Message, appErr.Internal)
}

// NoContentResponse sends 204 No Content
func NoContentResponse(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

// CreatedResponse sends 201 Created with location header
func CreatedResponse(c *gin.Context, message string, data interface{}, location string) {
	if location != "" {
		c.Header("Location", location)
	}
	SuccessResponse(c, http.StatusCreated, message, data)
}

// AcceptedResponse sends 202 Accepted for async operations
func AcceptedResponse(c *gin.Context, message string, data interface{}) {
	SuccessResponse(c, http.StatusAccepted, message, data)
}

// UnauthorizedResponse sends 401 Unauthorized
func UnauthorizedResponse(c *gin.Context, message string) {
	if message == "" {
		message = "Authentication required"
	}
	ErrorResponse(c, http.StatusUnauthorized, models.ErrCodeUnauthorized, message, nil)
}

// ForbiddenResponse sends 403 Forbidden
func ForbiddenResponse(c *gin.Context, message string) {
	if message == "" {
		message = "Access forbidden"
	}
	ErrorResponse(c, http.StatusForbidden, models.ErrCodeForbidden, message, nil)
}

// NotFoundResponse sends 404 Not Found
func NotFoundResponse(c *gin.Context, resource string) {
	message := "Resource not found"
	if resource != "" {
		message = resource + " not found"
	}
	ErrorResponse(c, http.StatusNotFound, models.ErrCodeNotFound, message, nil)
}

// ConflictResponse sends 409 Conflict
func ConflictResponse(c *gin.Context, message string) {
	if message == "" {
		message = "Resource conflict"
	}
	ErrorResponse(c, http.StatusConflict, models.ErrCodeConflict, message, nil)
}

// BadRequestResponse sends 400 Bad Request
func BadRequestResponse(c *gin.Context, message string) {
	if message == "" {
		message = "Invalid request"
	}
	ErrorResponse(c, http.StatusBadRequest, models.ErrCodeBadRequest, message, nil)
}

// InternalServerErrorResponse sends 500 Internal Server Error
func InternalServerErrorResponse(c *gin.Context, err error) {
	ErrorResponse(
		c,
		http.StatusInternalServerError,
		models.ErrCodeInternalServer,
		"An internal error occurred",
		err,
	)
}

// ServiceUnavailableResponse sends 503 Service Unavailable
func ServiceUnavailableResponse(c *gin.Context, message string) {
	if message == "" {
		message = "Service temporarily unavailable"
	}
	ErrorResponse(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", message, nil)
}

// TooManyRequestsResponse sends 429 Too Many Requests
func TooManyRequestsResponse(c *gin.Context, retryAfter int) {
	if retryAfter > 0 {
		c.Header("Retry-After", string(rune(retryAfter)))
	}
	ErrorResponse(c, http.StatusTooManyRequests, "RATE_LIMIT_EXCEEDED", "Too many requests", nil)
}
func ValidationErrorResponse(c *gin.Context, validationErrors map[string]interface{}) {
	// Ensure validationErrors is never nil
	if validationErrors == nil {
		validationErrors = make(map[string]interface{})
	}

	ErrorResponseWithDetails(
		c,
		http.StatusBadRequest,
		models.ErrCodeValidation,
		"Validation failed. Please check the following fields",
		validationErrors,
	)
}

// SingleFieldValidationError sends validation error for a single field
func SingleFieldValidationError(c *gin.Context, field, message string) {
	ValidationErrorResponse(c, map[string]interface{}{
		field: message,
	})
}

// MultipleFieldValidationErrors sends validation errors for multiple fields
func MultipleFieldValidationErrors(c *gin.Context, errors map[string]string) {
	validationErrors := make(map[string]interface{}, len(errors))
	for field, msg := range errors {
		validationErrors[field] = msg
	}
	ValidationErrorResponse(c, validationErrors)
}

// Example of enhanced ErrorResponseWithDetails with nil checks
func ErrorResponseWithDetails(c *gin.Context, statusCode int, code, message string, details map[string]interface{}) {
	fields := []zap.Field{
		zap.Int("status_code", statusCode),
		zap.String("error_code", code),
		zap.String("message", message),
		zap.String("path", c.Request.URL.Path),
	}

	// Only add details to log if not empty
	if len(details) > 0 {
		fields = append(fields, zap.Any("details", details))
	}

	if requestID := c.GetString("request_id"); requestID != "" {
		fields = append(fields, zap.String("request_id", requestID))
		if config.IncludeRequestID {
			c.Header("X-Request-ID", requestID)
		}
	}

	if config.EnableErrorLogging {
		logger.Warn("Error with details", fields...)
	}

	errorData := &models.ErrorData{
		Code:    code,
		Message: message,
	}

	// Only include details if not empty
	if len(details) > 0 {
		errorData.Details = details
	}

	response := models.APIResponse{
		Success: false,
		Error:   errorData,
	}

	c.JSON(statusCode, response)
}
