package routes

import (
	"go-server/internal/handler"
	"go-server/internal/middleware/guards"
	"go-server/internal/models"

	"github.com/gin-gonic/gin"
)

// SetupOnboardingRoutes mounts all onboarding endpoints.
//
// Access levels:
//
//	Level 1 — Public        : resident self-registration
//	Level 2 — Authenticated : resident claim / invite / flat-browse flows
//	Level 3 — Admin only    : claim approval, invite management (scoped to society)
//
// Public:
//
//	POST /onboarding/register                              → RegisterResident      [1]
//
// Resident (authenticated):
//
//	GET  /onboarding/society/:code/flats                   → GetSocietyFlats       [2]
//	POST /onboarding/claims                                → SubmitClaim           [2]
//	GET  /onboarding/claims/me                             → GetMyClaimHistory     [2]
//	POST /onboarding/redeem                                → RedeemInvite          [2]
//
// Admin (scoped under /societies/:id):
//
//	GET    /societies/:id/onboarding/claims                → ListPendingClaims     [3]
//	GET    /societies/:id/onboarding/claims/all            → ListAllClaims         [3]
//	POST   /societies/:id/onboarding/claims/:claimId/approve → ApproveClaim        [3]
//	POST   /societies/:id/onboarding/claims/:claimId/reject  → RejectClaim         [3]
//	POST   /societies/:id/onboarding/invites               → CreateInvite          [3]
//	GET    /societies/:id/onboarding/flats/:flatId/invites → GetActiveInvites      [3]
//	DELETE /societies/:id/onboarding/invites/:inviteId     → RevokeInvite          [3]

// Admin + plan staff-feature limit (scoped under /societies/:id):
//
//	POST /societies/:id/onboarding/register/staff          → RegisterStaff         [4b]
func SetupOnboardingRoutes(
	rg *gin.RouterGroup,
	h *handler.OnboardingHandler,
	g *guards.Guards,
) {
	// ---- Level 1: Public ----
	rg.POST("/onboarding/register", h.RegisterResident)

	// ---- Level 2: Authenticated — resident flows ----
	resident := rg.Group("/onboarding")
	resident.Use(g.Authenticated()...)
	{
		resident.GET("/society/:code/flats", h.GetSocietyFlats)
		resident.POST("/claims", h.SubmitClaim)
		resident.GET("/claims/me", h.GetMyClaimHistory)
		resident.POST("/redeem", h.RedeemInvite)
	}

	// ---- Level 3: Admin only — society-scoped management ----
	admin := rg.Group("/societies/:id/onboarding")
	admin.Use(g.AdminOnly()...)
	{
		// Claim management
		admin.GET("/claims", h.ListPendingClaims)
		admin.GET("/claims/all", h.ListAllClaims)
		admin.POST("/claims/:claimId/approve", h.ApproveClaim)
		admin.POST("/claims/:claimId/reject", h.RejectClaim)

		// Invite management
		admin.POST("/invites", h.CreateInvite)
		admin.GET("/flats/:flatId/invites", h.GetActiveInvites)
		admin.DELETE("/invites/:inviteId", h.RevokeInvite)
		
	}
	// ---- Level 4b: Admin + plan staff-feature limit — register staff ----
    registerStaff := rg.Group("/societies/:id/onboarding")
    registerStaff.Use(g.AdminWithFeature(models.FeatureStaff)...)
    {
        registerStaff.POST("/register/staff", h.RegisterStaff)
    }
	
}
