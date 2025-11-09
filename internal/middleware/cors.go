package middleware

import (
	"fmt"
	"go-server/internal/config"
	"net/http"

	"github.com/gin-gonic/gin"
)

// CORS middleware handles Cross-Origin Resource Sharing
func CORS(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		
		// Check if origin is allowed
		allowed := false
		for _, allowedOrigin := range cfg.AllowOrigins {
			if allowedOrigin == "*" || allowedOrigin == origin {
				allowed = true
				break
			}
		}

		if allowed {
			if origin != "" {
				c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			} else if len(cfg.AllowOrigins) == 1 {
				c.Writer.Header().Set("Access-Control-Allow-Origin", cfg.AllowOrigins[0])
			}
			
			c.Writer.Header().Set("Access-Control-Allow-Credentials", fmt.Sprintf("%v", cfg.AllowCredentials))
			c.Writer.Header().Set("Access-Control-Allow-Headers", joinStrings(cfg.AllowHeaders))
			c.Writer.Header().Set("Access-Control-Allow-Methods", joinStrings(cfg.AllowMethods))
			c.Writer.Header().Set("Access-Control-Max-Age", "86400")
		}

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
func joinStrings(strs []string) string {
	result := ""
	for i, s := range strs {
		if i > 0 {
			result += ", "
		}
		result += s
	}
	return result
}