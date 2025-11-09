package middleware

import (
	"go-server/pkg/logger"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// LoggerConfig defines configuration for the logger middleware
type LoggerConfig struct {
	// SkipPaths is a list of paths to skip logging (e.g., health checks)
	SkipPaths []string
	// SkipHealthCheck skips /health and /healthz endpoints
	SkipHealthCheck bool
	// EnableBody logs request/response bodies (use cautiously in prod)
	EnableBody bool
}

// DefaultLoggerConfig returns sensible defaults
func DefaultLoggerConfig() LoggerConfig {
	return LoggerConfig{
		SkipPaths:       []string{},
		SkipHealthCheck: true,
		EnableBody:      false,
	}
}

// Logger returns a gin.HandlerFunc middleware for request logging
func Logger() gin.HandlerFunc {
	return LoggerWithConfig(DefaultLoggerConfig())
}

// LoggerWithConfig returns a gin.HandlerFunc with custom config
func LoggerWithConfig(cfg LoggerConfig) gin.HandlerFunc {
	// Build skip paths map for O(1) lookup
	skipPaths := make(map[string]bool)
	for _, path := range cfg.SkipPaths {
		skipPaths[path] = true
	}
	if cfg.SkipHealthCheck {
		skipPaths["/health"] = true
		skipPaths["/healthz"] = true
		skipPaths["/ping"] = true
	}

	return func(c *gin.Context) {
		// Start timer
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		// Skip logging for configured paths
		if skipPaths[path] {
			c.Next()
			return
		}

		// Process request
		c.Next()

		// Calculate metrics
		latency := time.Since(start)
		statusCode := c.Writer.Status()
		clientIP := c.ClientIP()
		method := c.Request.Method
		
		// Build full path with query params
		fullPath := path
		if raw != "" {
			fullPath = path + "?" + raw
		}

		// Prepare log fields
		fields := []zap.Field{
			zap.String("method", method),
			zap.String("path", fullPath),
			zap.Int("status", statusCode),
			zap.Duration("latency", latency),
			zap.String("ip", clientIP),
			zap.String("user_agent", c.Request.UserAgent()),
		}

		// Add request ID if exists (common pattern)
		if requestID := c.GetString("request_id"); requestID != "" {
			fields = append(fields, zap.String("request_id", requestID))
		}

		// Add user ID if exists (for authenticated requests)
		if userID := c.GetString("user_id"); userID != "" {
			fields = append(fields, zap.String("user_id", userID))
		}

		// Add content length
		if c.Request.ContentLength > 0 {
			fields = append(fields, zap.Int64("request_size", c.Request.ContentLength))
		}
		fields = append(fields, zap.Int("response_size", c.Writer.Size()))

		// Log any errors that occurred during request processing
		if len(c.Errors) > 0 {
			// Log each error
			errFields := make([]zap.Field, 0, len(fields)+1)
			errFields = append(errFields, fields...)
			
			for _, ginErr := range c.Errors {
				errFields = append(errFields[:len(fields)], zap.Error(ginErr.Err))
				logger.Error("Request error occurred", errFields...)
			}
		}

		// Determine log level and message based on status code
		msg := "Request completed"
		switch {
		case statusCode >= 500:
			logger.Error(msg, fields...)
		case statusCode >= 400:
			logger.Warn(msg, fields...)
		case statusCode >= 300:
			logger.Info(msg, fields...)
		default:
			logger.Debug(msg, fields...)
		}
	}
}



// RequestID adds a unique request ID to each request
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if request ID exists in header
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			// Generate a simple request ID (use UUID in production)
			requestID = generateRequestID()
		}
		
		// Set request ID in context and response header
		c.Set("request_id", requestID)
		c.Header("X-Request-ID", requestID)
		
		c.Next()
	}
}

// Simple request ID generator (replace with UUID library in production)
func generateRequestID() string {
	return time.Now().Format("20060102150405") + "-" + randomString(8)
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[time.Now().UnixNano()%int64(len(letters))]
	}
	return string(b)
}