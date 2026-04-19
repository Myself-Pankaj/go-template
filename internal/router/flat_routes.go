package routes

import (
	"go-server/internal/handler"
	"go-server/internal/middleware/guards"
	"go-server/internal/models"

	"github.com/gin-gonic/gin"
)

// SetupFlatRoutes mounts flat endpoints under /societies/:id/flats.
//
// Access levels:
//
//	Level 2 — Authenticated          : read flats (list, by-number, single)
//	Level 3 — Admin only             : update status, delete
//	Level 4b — Admin + plan feature  : create flat (enforces SnapshotMaxFlats)
//
// Route summary:
//
//	POST   /societies/:id/flats                        → CreateFlat    [4b: flats]
//	GET    /societies/:id/flats                        → ListFlats     [2]
//	GET    /societies/:id/flats/number/:flatNumber     → GetFlatByNumber [2]
//	GET    /societies/:id/flats/:flatId                → GetFlat       [2]
//	PATCH  /societies/:id/flats/:flatId                → UpdateFlat    [3]
//	PATCH  /societies/:id/flats/:flatId/activate       → ActivateFlat  [3]
//	PATCH  /societies/:id/flats/:flatId/deactivate     → DeactivateFlat [3]
//	DELETE /societies/:id/flats/:flatId                → DeleteFlat    [3]
func SetupFlatRoutes(rg *gin.RouterGroup, h *handler.FlatHandler, g *guards.Guards) {
	base := rg.Group("/societies/:id/flats")

	// ---- Level 2: Authenticated — read-only ----
	read := base.Group("")
	read.Use(g.Authenticated()...)
	{
		// /number/:flatNumber must be declared before /:flatId so Gin does not
		// capture the literal string "number" as a flatId value.
		read.GET("", h.ListFlats)
		read.GET("/number/:flatNumber", h.GetFlatByNumber)
		read.GET("/:flatId", h.GetFlat)
	}

	// ---- Level 4b: Admin + plan flats-feature limit — create ----
	create := base.Group("")
	create.Use(g.AdminWithFeature(models.FeatureFlats)...)
	{
		create.POST("", h.CreateFlat)
	}

	// ---- Level 3: Admin only — manage ----
	manage := base.Group("/:flatId")
	manage.Use(g.AdminOnly()...)
	{
		manage.PATCH("", h.UpdateFlat)
		manage.PATCH("/activate", h.ActivateFlat)
		manage.PATCH("/deactivate", h.DeactivateFlat)
		manage.DELETE("", h.DeleteFlat)
	}
}