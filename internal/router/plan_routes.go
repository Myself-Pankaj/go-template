package routes

import (
	"go-server/internal/handler"
	"go-server/internal/middleware/guards"

	"github.com/gin-gonic/gin"
)

// SetupPlanRoutes mounts plan management endpoints under /plans.
//
// Access levels:
//
//	Level 1 — Public        : list active plans (pricing page / app plan selector)
//	Level 2 — Authenticated : read all plans, get single plan
//	Level 3 — Admin only    : create / update / activate / deactivate
//
// Route summary:
//
//	GET    /plans                → ListActivePlans  [1]
//	GET    /plans/all            → ListAllPlans     [2]
//	GET    /plans/:id            → GetPlan          [2]
//	POST   /plans                → CreatePlan       [3]
//	PATCH  /plans/:id            → UpdatePlan       [3]
//	PATCH  /plans/:id/activate   → ActivatePlan     [3]
//	PATCH  /plans/:id/deactivate → DeactivatePlan   [3]
func SetupPlanRoutes(rg *gin.RouterGroup, h *handler.PlanHandler, g *guards.Guards) {
	plans := rg.Group("/plans")

	// ---- Level 1: Public ----
	plans.GET("", h.ListActivePlans)

	// ---- Level 2: Authenticated ----
	// /plans/all must be declared before /:id so Gin does not capture the
	// literal string "all" as a plan ID.
	read := plans.Group("")
	read.Use(g.Authenticated()...)
	{
		read.GET("/all", h.ListAllPlans)
		read.GET("/:id", h.GetPlan)
	}

	// ---- Level 3: Admin only ----
	admin := plans.Group("")
	admin.Use(g.AdminOnly()...)
	{
		admin.POST("", h.CreatePlan)
		admin.PATCH("/:id", h.UpdatePlan)
		admin.PATCH("/:id/activate", h.ActivatePlan)
		admin.PATCH("/:id/deactivate", h.DeactivatePlan)
	}
}