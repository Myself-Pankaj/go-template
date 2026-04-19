package routes

import (
	"go-server/internal/handler"
	"go-server/internal/middleware/guards"

	"github.com/gin-gonic/gin"
)

// SetupSocietyRoutes mounts society endpoints under /societies.
//
// Access levels:
//
//	Level 2 — Authenticated : create society, read society details
//	Level 3 — Admin only    : update, activate, deactivate, delete
func SetupSocietyRoutes(rg *gin.RouterGroup, h *handler.SocietyHandler, g *guards.Guards) {
	societies := rg.Group("/societies")

	// ---- Level 2: Authenticated ----
	read := societies.Group("")
	read.Use(g.Authenticated()...)
	{
		read.POST("", h.RegisterSociety)
		read.GET("", h.ListSocieties) // intended for super-admins; add RoleGuard when ready
		read.GET("/:id", h.GetSocietyDetails)
		read.GET("/code/:code", h.GetSocietyByCode)
	}

	// ---- Level 3: Admin only ----
	admin := societies.Group("")
	admin.Use(g.AdminOnly()...)
	{
		admin.PATCH("/:id", h.UpdateSociety)
		admin.PATCH("/:id/activate", h.ActivateSociety)
		admin.PATCH("/:id/deactivate", h.DeactivateSociety)
		admin.DELETE("/:id", h.DeleteSociety)
	}
}