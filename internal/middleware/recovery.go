package middleware

import (
	"fmt"
	"go-server/internal/models"
	"go-server/pkg/logger"
	"go-server/pkg/utils"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// RecoveryConfig defines configuration for recovery middleware
type RecoveryConfig struct {
	// EnableStackTrace includes full stack trace in logs
	EnableStackTrace bool
	// EnableRequestDump includes full request dump in logs
	EnableRequestDump bool
	// StackTraceSize limits stack trace size (0 = unlimited)
	StackTraceSize int
	// CustomRecoveryHandler allows custom panic handling
	CustomRecoveryHandler func(*gin.Context, interface{})
}

// DefaultRecoveryConfig returns production-safe defaults
func DefaultRecoveryConfig() RecoveryConfig {
	return RecoveryConfig{
		EnableStackTrace:      true,
		EnableRequestDump:     false, // Privacy concern in prod
		StackTraceSize:        4096,  // 4KB limit
		CustomRecoveryHandler: nil,
	}
}

// Recovery returns a middleware that recovers from panics
func Recovery() gin.HandlerFunc {
	return RecoveryWithConfig(DefaultRecoveryConfig())
}

// RecoveryWithConfig returns a recovery middleware with custom config
func RecoveryWithConfig(cfg RecoveryConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// Check for broken connection
				var brokenPipe bool
				if ne, ok := err.(*net.OpError); ok {
					if se, ok := ne.Err.(*os.SyscallError); ok {
						errStr := strings.ToLower(se.Error())
						if strings.Contains(errStr, "broken pipe") ||
							strings.Contains(errStr, "connection reset by peer") {
							brokenPipe = true
						}
					}
				}

				// Build log fields
				fields := []zap.Field{
					zap.String("method", c.Request.Method),
					zap.String("path", c.Request.URL.Path),
					zap.String("ip", c.ClientIP()),
					zap.String("user_agent", c.Request.UserAgent()),
					zap.Time("timestamp", time.Now()),
				}

				// Add request ID if available
				if requestID := c.GetString("request_id"); requestID != "" {
					fields = append(fields, zap.String("request_id", requestID))
				}

				// Add user ID if available (authenticated requests)
				if userID := c.GetString("user_id"); userID != "" {
					fields = append(fields, zap.String("user_id", userID))
				}

				// Add panic details
				fields = append(fields, zap.Any("panic", err))

				// Add stack trace
				if cfg.EnableStackTrace {
					stack := debug.Stack()
					if cfg.StackTraceSize > 0 && len(stack) > cfg.StackTraceSize {
						stack = stack[:cfg.StackTraceSize]
					}
					fields = append(fields, zap.ByteString("stack", stack))
				}

				// Add request dump if enabled (careful in prod - may contain sensitive data)
				if cfg.EnableRequestDump && !brokenPipe {
					if dump, err := httputil.DumpRequest(c.Request, false); err == nil {
						fields = append(fields, zap.ByteString("request_dump", dump))
					}
				}

				// Log the panic
				if brokenPipe {
					logger.Error("Client connection broken (broken pipe)", fields...)
				} else {
					logger.Error("Panic recovered", fields...)
				}

				// Call custom handler if provided
				if cfg.CustomRecoveryHandler != nil {
					cfg.CustomRecoveryHandler(c, err)
					return
				}

				// Handle broken pipe - don't send response
				if brokenPipe {
					c.Abort()
					return
				}

				// Send error response
				utils.ErrorResponse(
					c,
					http.StatusInternalServerError,
					models.ErrCodeInternalServer,
					"An unexpected error occurred. Please try again later.",
					nil,
				)
			}
		}()

		c.Next()
	}
}

// RecoveryWithWriter returns a recovery middleware that also writes to a custom writer
func RecoveryWithWriter(cfg RecoveryConfig, alertFunc func(c *gin.Context, err interface{})) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// Call alert function for monitoring/alerting systems
				if alertFunc != nil {
					go alertFunc(c, err) // Non-blocking
				}

				// Standard recovery handling
				RecoveryWithConfig(cfg)(c)
			}
		}()

		c.Next()
	}
}

// TimeoutRecovery handles context deadline exceeded errors gracefully
func TimeoutRecovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// Check if it's a timeout error
				errStr := fmt.Sprintf("%v", err)
				if strings.Contains(errStr, "context deadline exceeded") ||
					strings.Contains(errStr, "timeout") {
					
					logger.Warn("Request timeout",
						zap.String("path", c.Request.URL.Path),
						zap.String("method", c.Request.Method),
						zap.Any("error", err),
					)

					utils.ErrorResponse(
						c,
						http.StatusGatewayTimeout,
						"REQUEST_TIMEOUT",
						"Request took too long to process",
						nil,
					)
					return
				}

				// Re-panic for normal recovery middleware to handle
				panic(err)
			}
		}()

		c.Next()
	}
}

// CircuitBreakerRecovery can be used with circuit breaker pattern
func CircuitBreakerRecovery(onFailure func()) gin.HandlerFunc {
	failureCount := 0
	const threshold = 5

	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				failureCount++
				
				if failureCount >= threshold && onFailure != nil {
					go onFailure() // Trigger circuit breaker
				}

				// Re-panic for normal recovery
				panic(err)
			} else {
				// Reset on success
				if failureCount > 0 {
					failureCount--
				}
			}
		}()

		c.Next()
	}
}