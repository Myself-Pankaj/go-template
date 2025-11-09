package router

import (
	"go-server/internal/handler"
	"go-server/internal/middleware"

	"github.com/gin-gonic/gin"
)

// SetupAuthRoutes configures all authentication-related routes
func SetupAuthRoutes(rg *gin.RouterGroup, authHandler *handler.AuthHandler, jwtSecret, jwtIssuer string) {
	auth := rg.Group("/auth")
	{
		// ==================== PUBLIC ROUTES ====================
		// No authentication required
		auth.POST("/register", authHandler.Register)
		auth.POST("/login", authHandler.Login)
		auth.POST("/verify-otp", authHandler.VerifyOTP)
		auth.POST("/resend-otp", authHandler.ResendOTP)

		// ==================== PROTECTED ROUTES ====================
		// Authentication required
		protected := auth.Group("")
		protected.Use(middleware.AuthMiddleware(jwtSecret, jwtIssuer))
		{
			// User profile management
			protected.PUT("/user", authHandler.UpdateUser)
			
			// Token management
			protected.POST("/refresh", authHandler.RefreshToken)
			protected.POST("/logout", authHandler.Logout)
		}
	}
}