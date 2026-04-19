// internal/middleware/guards/guards.go
package guards

import (
	"go-server/internal/middleware"
	"go-server/internal/repository"

	"github.com/gin-gonic/gin"
)

// ==================== ACCESS LEVELS ====================
//
//  Level 1 — Public
//      Anyone can call the route.  Apply no middleware at all.
//
//  Level 2 — Authenticated
//      A valid access_token JWT cookie is required.
//      Guards.Authenticated()
//
//  Level 3 — Admin only
//      Valid JWT + role must be "admin".
//      Guards.AdminOnly()
//
//  Level 4a — Admin + active plan (no feature-limit check)
//      Valid JWT + "admin" role + society has a live subscription.
//      Guards.AdminWithPlan()
//
//  Level 4b — Admin + active plan + feature within limit
//      Valid JWT + "admin" role + live subscription + current usage < limit.
//      Guards.AdminWithFeature(models.FeatureFlats | FeatureStaff | FeatureAdmins)

// ==================== GUARDS STRUCT ====================

// Guards is the central access-control factory.
//
// Construct it once during application startup (see app/dependency.go) and
// inject it into every route-setup function.  Route files call helper methods
// to get the correct middleware chain for each endpoint.
//
// Example — route file usage:
//
//	func SetupFlatRoutes(rg *gin.RouterGroup, h *handler.FlatHandler, g *guards.Guards) {
//	    base := rg.Group("/societies/:id/flats")
//
//	    // GET  — any authenticated user
//	    read := base.Group("")
//	    read.Use(g.Authenticated()...)
//	    read.GET("", h.ListFlats)
//
//	    // POST — admin + plan + flat-count limit
//	    create := base.Group("")
//	    create.Use(g.AdminWithFeature(models.FeatureFlats)...)
//	    create.POST("", h.CreateFlat)
//
//	    // PATCH / DELETE — admin only
//	    manage := base.Group("/:flatId")
//	    manage.Use(g.AdminOnly()...)
//	    manage.PATCH("", h.UpdateFlat)
//	    manage.DELETE("", h.DeleteFlat)
//	}
type Guards struct {
	jwtSecret string
	jwtIssuer string
	plan      *PlanGuard
}

// New constructs a Guards instance.  Pass it to every route-setup function
// instead of raw jwtSecret/jwtIssuer strings.
func New(
	jwtSecret, jwtIssuer string,
	subsRepo repository.SubscriptionRepository,
	flatRepo repository.FlatRepository,
	userRepo repository.UserRepository,
) *Guards {
	return &Guards{
		jwtSecret: jwtSecret,
		jwtIssuer: jwtIssuer,
		plan:      NewPlanGuard(subsRepo, flatRepo, userRepo),
	}
}

// ==================== LEVEL 2: AUTHENTICATED ====================

// Authenticated returns the middleware chain for Level 2.
// Any logged-in user passes; role is irrelevant.
//
//	rg.GET("/profile", append(g.Authenticated(), h.GetProfile)...)
func (g *Guards) Authenticated() []gin.HandlerFunc {
	return []gin.HandlerFunc{
		middleware.AuthMiddleware(g.jwtSecret, g.jwtIssuer),
	}
}

// ==================== LEVEL 3: ADMIN ONLY ====================

// AdminOnly returns the middleware chain for Level 3.
// The caller must be authenticated AND carry the "admin" role.
//
//	rg.DELETE("/:id", append(g.AdminOnly(), h.Delete)...)
func (g *Guards) AdminOnly() []gin.HandlerFunc {
	return []gin.HandlerFunc{
		middleware.AuthMiddleware(g.jwtSecret, g.jwtIssuer),
		middleware.RequireRole(RoleAdmin),
	}
}

// ==================== LEVEL 4a: ADMIN + ACTIVE PLAN ====================

// AdminWithPlan returns the middleware chain for Level 4a.
// The caller must be an admin AND the society must have a live subscription.
// No per-feature usage limit is checked — use this for dashboards, reports,
// and other gate-by-plan-only routes.
//
//	rg.GET("/dashboard", append(g.AdminWithPlan(), h.Dashboard)...)
func (g *Guards) AdminWithPlan() []gin.HandlerFunc {
	return []gin.HandlerFunc{
		middleware.AuthMiddleware(g.jwtSecret, g.jwtIssuer),
		middleware.RequireRole(RoleAdmin),
		g.plan.RequireActivePlan(),
	}
}

// ==================== LEVEL 4b: ADMIN + PLAN + FEATURE LIMIT ====================

// AdminWithFeature returns the middleware chain for Level 4b.
// The caller must be an admin, the society must have a live subscription, AND
// current usage of `feature` must be below the plan's snapshot limit.
//
// `feature` must be one of the constants defined in models:
//
//	models.FeatureFlats   — enforces SnapshotMaxFlats
//	models.FeatureStaff   — enforces SnapshotMaxStaff
//	models.FeatureAdmins  — enforces SnapshotMaxAdmins
//
//	rg.POST("/flats",  append(g.AdminWithFeature(models.FeatureFlats),  h.CreateFlat)...)
//	rg.POST("/staff",  append(g.AdminWithFeature(models.FeatureStaff),  h.InviteStaff)...)
//	rg.POST("/admins", append(g.AdminWithFeature(models.FeatureAdmins), h.AddAdmin)...)
func (g *Guards) AdminWithFeature(feature string) []gin.HandlerFunc {
	return []gin.HandlerFunc{
		middleware.AuthMiddleware(g.jwtSecret, g.jwtIssuer),
		middleware.RequireRole(RoleAdmin),
		g.plan.RequireFeature(feature),
	}
}

// ==================== REFRESH (special-case) ====================

// Refresh returns the middleware chain for the token-refresh endpoint.
// It validates the refresh_token cookie instead of the access_token cookie.
// Should only be applied to POST /auth/refresh.
func (g *Guards) Refresh() []gin.HandlerFunc {
	return []gin.HandlerFunc{
		middleware.RefreshMiddleware(g.jwtSecret, g.jwtIssuer),
	}
}
