package routes

import (
	"go-server/internal/handler"
	"go-server/internal/middleware/guards"

	"github.com/gin-gonic/gin"
)

// SetupSubscriptionRoutes mounts subscription endpoints under
// /societies/:id/subscriptions.
//
// Access levels:
//
//	Level 2 — Authenticated : read subscription state (list, active, is-active)
//	Level 3 — Admin only    : mutate subscription (subscribe, change-plan, cancel, renew)
//
// NOTE: Subscribe/ChangePlan/Cancel/Renew are admin-only but do NOT require
// RequireActivePlan because these endpoints ARE the subscription management
// actions themselves (bootstrap + lifecycle).
func SetupSubscriptionRoutes(rg *gin.RouterGroup, h *handler.SubscriptionHandler, g *guards.Guards) {
	subs := rg.Group("/societies/:id/subscriptions")

	// ---- Level 2: Authenticated ----
	read := subs.Group("")
	read.Use(g.Authenticated()...)
	{
		read.GET("", h.ListSubscriptions)
		read.GET("/active", h.GetActiveSubscription)
		read.GET("/is-active", h.IsActive)
	}

	// ---- Level 3: Admin only ----
	admin := subs.Group("")
	admin.Use(g.AdminOnly()...)
	{
		admin.POST("", h.Subscribe)
		admin.PATCH("/change-plan", h.ChangePlan)
		admin.PATCH("/cancel", h.Cancel)
		admin.PATCH("/renew", h.Renew)
	}
}
