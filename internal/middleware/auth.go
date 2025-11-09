// internal/middleware/auth.go
package middleware

import (
	"errors"
	"go-server/internal/models"
	"go-server/pkg/utils"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidToken   = models.NewAppError(
		models.ErrCodeInvalidToken,
		"invalid token",
		http.StatusUnauthorized,
		nil,
	)
	ErrExpiredToken   = models.NewAppError(
		models.ErrCodeTokenExpired,
		"token has expired",
		http.StatusUnauthorized,
		nil,
	)
	ErrMissingToken   = models.NewAppError(
		models.ErrCodeValidation,
		"missing authorization token",
		http.StatusUnauthorized,
		nil,
	)
	ErrInvalidIssuer  = models.NewAppError(
		models.ErrCodeInvalidToken,
		"invalid token issuer",
		http.StatusUnauthorized,
		nil,
	)
)

type Claims struct {
	UserID int64  `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

// GenerateToken generates a new JWT token
func GenerateToken(userID int64, email, role, secret, issuer string, expiry time.Duration) (string, error) {
	claims := &Claims{
		UserID: userID,
		Email:  email,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    issuer,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	return token.SignedString([]byte(secret))
}

// ValidateToken validates and parses JWT token
func ValidateToken(tokenString, secret, expectedIssuer string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return []byte(secret), nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	// Validate issuer
	if claims.Issuer != expectedIssuer {
		return nil, ErrInvalidIssuer
	}

	return claims, nil
}

// AuthMiddleware validates JWT token from cookie
func AuthMiddleware(jwtSecret, jwtIssuer string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get token from cookie
		token, err := c.Cookie("auth_token")
		if err != nil {
			utils.ErrorResponse(
				c,
				http.StatusUnauthorized,
				models.ErrCodeUnauthorized,
				"Authentication required. Please login",
				ErrMissingToken,
			)
			c.Abort()
			return
		}

		if token == "" {
			utils.ErrorResponse(
				c,
				http.StatusUnauthorized,
				models.ErrCodeUnauthorized,
				"Token cannot be empty",
				ErrInvalidToken,
			)
			c.Abort()
			return
		}

		claims, err := ValidateToken(token, jwtSecret, jwtIssuer)
		if err != nil {
			switch {
			case errors.Is(err, ErrExpiredToken):
				utils.ErrorResponse(
					c,
					http.StatusUnauthorized,
					models.ErrCodeTokenExpired,
					"Your session has expired. Please login again",
					err,
				)
			case errors.Is(err, ErrInvalidIssuer):
				utils.ErrorResponse(
					c,
					http.StatusUnauthorized,
					models.ErrCodeInvalidToken,
					"Token issuer validation failed",
					err,
				)
			case errors.Is(err, ErrInvalidToken):
				utils.ErrorResponse(
					c,
					http.StatusUnauthorized,
					models.ErrCodeInvalidToken,
					"Invalid authentication token",
					err,
				)
			default:
				utils.ErrorResponse(
					c,
					http.StatusUnauthorized,
					models.ErrCodeUnauthorized,
					"Authentication failed",
					err,
				)
			}
			c.Abort()
			return
		}

		// Set user info in context
		c.Set("user_id", claims.UserID)
		c.Set("user_email", claims.Email)
		c.Set("user_role", claims.Role)

		c.Next()
	}
}

// GetUserIDFromContext retrieves user ID from gin context
func GetUserIDFromContext(c *gin.Context) (int64, bool) {
	userID, exists := c.Get("user_id")
	if !exists {
		return 0, false
	}
	id, ok := userID.(int64)
	return id, ok
}

// GetUserEmailFromContext retrieves user email from gin context
func GetUserEmailFromContext(c *gin.Context) (string, bool) {
	email, exists := c.Get("user_email")
	if !exists {
		return "", false
	}
	e, ok := email.(string)
	return e, ok
}

// GetUserRoleFromContext retrieves user role from gin context
func GetUserRoleFromContext(c *gin.Context) (string, bool) {
	role, exists := c.Get("user_role")
	if !exists {
		return "", false
	}
	r, ok := role.(string)
	return r, ok
}

// RequireRole middleware checks if user has required role
func RequireRole(allowedRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := GetUserRoleFromContext(c)
		if !exists {
			utils.ErrorResponse(
				c,
				http.StatusUnauthorized,
				models.ErrCodeUnauthorized,
				"User role information not found",
				nil,
			)
			c.Abort()
			return
		}

		// Check if role is allowed
		allowed := false
		for _, r := range allowedRoles {
			if role == r {
				allowed = true
				break
			}
		}

		if !allowed {
			utils.ErrorResponse(
				c,
				http.StatusForbidden,
				models.ErrCodeForbidden,
				"You do not have permission to access this resource",
				nil,
			)
			c.Abort()
			return
		}

		c.Next()
	}
}

// OptionalAuth middleware that doesn't abort if auth fails
// Useful for endpoints that work with or without authentication
func OptionalAuth(jwtSecret, jwtIssuer string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Try to get token from cookie
		token, err := c.Cookie("auth_token")
		if err != nil || token == "" {
			c.Next()
			return
		}

		claims, err := ValidateToken(token, jwtSecret, jwtIssuer)
		if err == nil {
			// Set user info in context if valid
			c.Set("user_id", claims.UserID)
			c.Set("user_email", claims.Email)
			c.Set("user_role", claims.Role)
		}

		c.Next()
	}
}