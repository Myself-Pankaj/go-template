package handler

import (
	"net/http"
	"time"

	"go-server/internal/middleware"
	"go-server/internal/models"

	"github.com/gin-gonic/gin"
)

type TokenHandler struct {
	jwtSecret     string
	jwtIssuer     string
	accessExpiry  time.Duration
	refreshExpiry time.Duration
	isProduction  bool
}

func NewTokenHandler(
	jwtSecret string,
	jwtIssuer string,
	accessExpiry time.Duration,
	refreshExpiry time.Duration,
	isProduction bool,
) *TokenHandler {
	return &TokenHandler{
		jwtSecret:     jwtSecret,
		jwtIssuer:     jwtIssuer,
		accessExpiry:  accessExpiry,
		refreshExpiry: refreshExpiry,
		isProduction:  isProduction,
	}
}

// ==================== TOKEN HELPERS ====================

// generateAccessToken creates a short-lived access token
func (h *TokenHandler) generateAccessToken(user *models.User) (string, error) {
	return middleware.GenerateToken(
		user.ID,
		user.Email,
		user.Role,
		middleware.TokenTypeAccess,
		h.jwtSecret,
		h.jwtIssuer,
		h.accessExpiry,
	)
}

// generateRefreshToken creates a long-lived refresh token
func (h *TokenHandler) generateRefreshToken(user *models.User) (string, error) {
	return middleware.GenerateToken(
		user.ID,
		user.Email,
		user.Role,
		middleware.TokenTypeRefresh,
		h.jwtSecret,
		h.jwtIssuer,
		h.refreshExpiry,
	)
}

func (h *TokenHandler) setAccessCookie(c *gin.Context, token string) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(
		"access_token",
		token,
		int(h.accessExpiry.Seconds()),
		"/",
		"",
		h.isProduction,
		true,
	)
}

func (h *TokenHandler) setRefreshCookie(c *gin.Context, token string) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(
		"refresh_token",
		token,
		int(h.refreshExpiry.Seconds()),
		"/api/v1/auth/refresh",
		"",
		h.isProduction,
		true,
	)
}

func (h *TokenHandler) clearAccessCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("access_token", "", -1, "/", "", h.isProduction, true)
}

func (h *TokenHandler) clearRefreshCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("refresh_token", "", -1, "/api/v1/auth/refresh", "", h.isProduction, true)
}

// issueBothTokens generates and sets both access and refresh cookies
// Used after login and OTP verification
func (h *TokenHandler) issueBothTokens(c *gin.Context, user *models.User) error {
	accessToken, err := h.generateAccessToken(user)
	if err != nil {
		return err
	}

	refreshToken, err := h.generateRefreshToken(user)
	if err != nil {
		return err
	}

	h.setAccessCookie(c, accessToken)
	h.setRefreshCookie(c, refreshToken)
	return nil
}
