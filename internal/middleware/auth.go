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

// ==================== TOKEN TYPES ====================

const (
	TokenTypeAccess  = "access"
	TokenTypeRefresh = "refresh"
)

// ==================== ERRORS ====================

var (
	ErrInvalidToken = models.NewAppError(
		models.ErrCodeInvalidToken,
		"invalid token",
		http.StatusUnauthorized,
		nil,
	)
	ErrExpiredToken = models.NewAppError(
		models.ErrCodeTokenExpired,
		"token has expired",
		http.StatusUnauthorized,
		nil,
	)
	ErrMissingToken = models.NewAppError(
		models.ErrCodeValidation,
		"missing authorization token",
		http.StatusUnauthorized,
		nil,
	)
	ErrInvalidIssuer = models.NewAppError(
		models.ErrCodeInvalidToken,
		"invalid token issuer",
		http.StatusUnauthorized,
		nil,
	)
	ErrWrongTokenType = models.NewAppError(
		models.ErrCodeInvalidToken,
		"invalid token type for this operation",
		http.StatusUnauthorized,
		nil,
	)
)

// ==================== CLAIMS ====================

type Claims struct {
	UserID    int64  `json:"user_id"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	TokenType string `json:"token_type"` // "access" or "refresh"
	jwt.RegisteredClaims
}

// ==================== TOKEN GENERATION ====================

// GenerateToken generates a JWT token of specified type (access or refresh)
func GenerateToken(userID int64, email, role, tokenType, secret, issuer string, expiry time.Duration) (string, error) {
	claims := &Claims{
		UserID:    userID,
		Email:     email,
		Role:      role,
		TokenType: tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    issuer,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ==================== TOKEN VALIDATION ====================

// ValidateToken validates and parses a JWT token, enforcing expected token type
func ValidateToken(tokenString, secret, expectedIssuer, expectedTokenType string) (*Claims, error) {
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

	// Validate token type — prevents using refresh token on protected routes and vice versa
	if claims.TokenType != expectedTokenType {
		return nil, ErrWrongTokenType
	}

	return claims, nil
}

// ==================== AUTH MIDDLEWARE (access token) ====================

// AuthMiddleware validates the short-lived access_token cookie
func AuthMiddleware(jwtSecret, jwtIssuer string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie("access_token")
		
		if err != nil || token == "" {
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

		claims, err := ValidateToken(token, jwtSecret, jwtIssuer, TokenTypeAccess)
		if err != nil {
			handleTokenError(c, err)
			return
		}

		setUserContext(c, claims)
		c.Next()
	}
}

// ==================== REFRESH MIDDLEWARE (refresh token) ====================

// RefreshMiddleware validates the long-lived refresh_token cookie
// Used exclusively on the /auth/refresh endpoint
func RefreshMiddleware(jwtSecret, jwtIssuer string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie("refresh_token")
		if err != nil || token == "" {
			utils.ErrorResponse(
				c,
				http.StatusUnauthorized,
				models.ErrCodeUnauthorized,
				"Refresh token missing. Please login again",
				ErrMissingToken,
			)
			c.Abort()
			return
		}

		claims, err := ValidateToken(token, jwtSecret, jwtIssuer, TokenTypeRefresh)
		if err != nil {
			handleTokenError(c, err)
			return
		}

		setUserContext(c, claims)
		c.Next()
	}
}

// ==================== OPTIONAL AUTH ====================

// OptionalAuth middleware that doesn't abort if auth fails
// Useful for endpoints that work with or without authentication
func OptionalAuth(jwtSecret, jwtIssuer string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie("access_token")
		if err != nil || token == "" {
			c.Next()
			return
		}

		claims, err := ValidateToken(token, jwtSecret, jwtIssuer, TokenTypeAccess)
		if err == nil {
			setUserContext(c, claims)
		}

		c.Next()
	}
}

// ==================== ROLE MIDDLEWARE ====================

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

		for _, r := range allowedRoles {
			if role == r {
				c.Next()
				return
			}
		}

		utils.ErrorResponse(
			c,
			http.StatusForbidden,
			models.ErrCodeForbidden,
			"You do not have permission to access this resource",
			nil,
		)
		c.Abort()
	}
}

// ==================== CONTEXT HELPERS ====================

func setUserContext(c *gin.Context, claims *Claims) {
	c.Set("user_id", claims.UserID)
	c.Set("user_email", claims.Email)
	c.Set("user_role", claims.Role)
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

// ==================== PRIVATE HELPERS ====================

// handleTokenError centralizes token error responses and aborts the request
func handleTokenError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrExpiredToken):
		utils.ErrorResponse(c, http.StatusUnauthorized, models.ErrCodeTokenExpired, "Your session has expired. Please login again", err)
	case errors.Is(err, ErrInvalidIssuer):
		utils.ErrorResponse(c, http.StatusUnauthorized, models.ErrCodeInvalidToken, "Token issuer validation failed", err)
	case errors.Is(err, ErrWrongTokenType):
		utils.ErrorResponse(c, http.StatusUnauthorized, models.ErrCodeInvalidToken, "Invalid token type for this operation", err)
	case errors.Is(err, ErrInvalidToken):
		utils.ErrorResponse(c, http.StatusUnauthorized, models.ErrCodeInvalidToken, "Invalid authentication token", err)
	default:
		utils.ErrorResponse(c, http.StatusUnauthorized, models.ErrCodeUnauthorized, "Authentication failed", err)
	}
	c.Abort()
}