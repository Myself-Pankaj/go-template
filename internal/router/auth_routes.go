package routes

import (
	"go-server/internal/handler"
	"go-server/internal/middleware/guards"

	"github.com/gin-gonic/gin"
)

// SetupAuthRoutes mounts authentication endpoints under /auth.
//
// Access levels:
//
//	Level 1 — Public        : register, login, OTP, password-reset
//	Level 1 — Refresh token : POST /refresh  (validates refresh_token cookie)
//	Level 2 — Authenticated : profile update, password change, logout
func SetupAuthRoutes(rg *gin.RouterGroup, h *handler.AuthHandler, g *guards.Guards) {
	auth := rg.Group("/auth")

	// ---- Level 1: Public ----
	auth.POST("/register", h.Register)
	auth.POST("/login", h.Login)
	auth.POST("/verify-otp", h.VerifyOTP)
	auth.POST("/resend-otp", h.ResendOTP)
	auth.POST("/forget-password", h.ForgetPassword)
	auth.POST("/reset-password", h.ResetPassword)

	// ---- Level 1 (special): Refresh token ----
	// Uses refresh_token cookie, NOT access_token.
	refresh := auth.Group("")
	refresh.Use(g.Refresh()...)
	{
		refresh.POST("/refresh", h.RefreshToken)
	}

	// ---- Level 2: Authenticated ----
	protected := auth.Group("")
	protected.Use(g.Authenticated()...)
	{
		protected.PUT("/user", h.UpdateUser)
		protected.POST("/change-password", h.ChangePassword)
		protected.POST("/logout", h.Logout)
	}
}