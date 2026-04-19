// internal/middleware/guards/plan_guard.go
package guards

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"go-server/internal/models"
	"go-server/internal/repository"
	"go-server/pkg/utils"

	"github.com/gin-gonic/gin"
)

// ==================== CONTEXT KEY ====================

// CtxKeySubscription is the gin context key under which the resolved
// *models.Subscription is stored after a successful plan guard check.
// Downstream handlers can read it without an additional DB round-trip.
const CtxKeySubscription = "active_subscription"

// ==================== PLAN GUARD ====================

// PlanGuard enforces subscription-based access control.
//
// It resolves the society ID from the URL parameter ":id", fetches the
// current active subscription, checks expiry, and — when a Feature is
// specified — verifies that the current usage is below the plan limit.
//
// PlanGuard must always be placed AFTER AuthMiddleware so that user_id /
// user_role are already populated in the gin context.
type PlanGuard struct {
	subsRepo repository.SubscriptionRepository
	flatRepo repository.FlatRepository
	userRepo repository.UserRepository
}

// NewPlanGuard constructs a PlanGuard.  All three repositories are required.
func NewPlanGuard(
	subsRepo repository.SubscriptionRepository,
	flatRepo repository.FlatRepository,
	userRepo repository.UserRepository,
) *PlanGuard {
	return &PlanGuard{
		subsRepo: subsRepo,
		flatRepo: flatRepo,
		userRepo: userRepo,
	}
}

// ==================== MIDDLEWARE FACTORIES ====================

// RequireActivePlan returns a gin.HandlerFunc that checks whether the society
// has a currently-active subscription.
//
// On success it attaches the *models.Subscription to the context under
// CtxKeySubscription so downstream handlers can read it for free.
//
// HTTP responses:
//   - 400 Bad Request      — missing or non-numeric ":id" URL param
//   - 402 Payment Required — no active subscription / subscription expired
func (g *PlanGuard) RequireActivePlan() gin.HandlerFunc {
	return func(c *gin.Context) {
		sub, ok := g.resolveActiveSub(c)
		if !ok {
			return // response already written & request aborted
		}
		c.Set(CtxKeySubscription, sub)
		c.Next()
	}
}

// RequireFeature returns a gin.HandlerFunc that checks both:
//  1. The society has a currently-active subscription.
//  2. The current usage of `feature` is strictly below the plan's snapshot
//     limit.  A nil snapshot limit means unlimited — always passes.
//
// `feature` must be one of the constants defined in models:
//
//	models.FeatureFlats   ("flats")
//	models.FeatureStaff   ("staff")
//	models.FeatureAdmins  ("admins")
//
// HTTP responses:
//   - 400 Bad Request      — missing or non-numeric ":id" URL param
//   - 402 Payment Required — no active / expired subscription
//   - 403 Forbidden        — plan limit already reached
func (g *PlanGuard) RequireFeature(feature string) gin.HandlerFunc {
	return func(c *gin.Context) {
		sub, ok := g.resolveActiveSub(c)
		if !ok {
			return
		}

		limit := planLimit(sub, feature)
		if limit != nil {
			// nil limit = unlimited — skip count check entirely
			ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
			defer cancel()

			current, err := g.countFeature(ctx, sub.SocietyID, feature)
			if err != nil {
				utils.ErrorResponse(
					c,
					http.StatusInternalServerError,
					models.ErrCodeInternalServer,
					"Failed to verify plan usage",
					err,
				)
				c.Abort()
				return
			}

			if current >= *limit {
				utils.ErrorResponse(
					c,
					http.StatusForbidden,
					models.ErrCodeLimitExceeded,
					fmt.Sprintf(
						"Plan limit reached: your current plan allows a maximum of %d %s. "+
							"Please upgrade your plan to add more.",
						*limit, feature,
					),
					nil,
				)
				c.Abort()
				return
			}
		}

		c.Set(CtxKeySubscription, sub)
		c.Next()
	}
}

// ==================== CONTEXT HELPER ====================

// GetSubscriptionFromContext retrieves the active *models.Subscription that
// was injected by RequireActivePlan or RequireFeature.
// Returns (nil, false) when the guard was not applied to the route.
func GetSubscriptionFromContext(c *gin.Context) (*models.Subscription, bool) {
	v, exists := c.Get(CtxKeySubscription)
	if !exists {
		return nil, false
	}
	sub, ok := v.(*models.Subscription)
	return sub, ok
}

// ==================== PRIVATE HELPERS ====================

// resolveActiveSub is the shared preamble used by both middleware factories.
// It parses the society ID, fetches the subscription, and validates liveness.
// Returns (nil, false) if it has already written an error response.
func (g *PlanGuard) resolveActiveSub(c *gin.Context) (*models.Subscription, bool) {
	societyID, err := parseSocietyID(c)
	if err != nil {
		utils.ErrorResponse(
			c,
			http.StatusBadRequest,
			models.ErrCodeBadRequest,
			"Invalid society ID in URL",
			err,
		)
		c.Abort()
		return nil, false
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	sub, err := g.subsRepo.GetActiveBySocietyID(ctx, societyID)
	if err != nil {
		utils.ErrorResponse(
			c,
			http.StatusPaymentRequired,
			models.ErrCodeSubscriptionRequired,
			"No active subscription found. Please subscribe to a plan to access this feature.",
			nil,
		)
		c.Abort()
		return nil, false
	}

	// Double-check in-process: DB query already filters by end_date > NOW(),
	// but this guard ensures correctness even under clock skew.
	if !sub.IsCurrentlyActive() {
		utils.ErrorResponse(
			c,
			http.StatusPaymentRequired,
			models.ErrCodeSubscriptionRequired,
			"Your subscription has expired. Please renew your plan to continue.",
			nil,
		)
		c.Abort()
		return nil, false
	}

	return sub, true
}

// parseSocietyID extracts and validates the ":id" URL parameter.
func parseSocietyID(c *gin.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("'%s' is not a valid society ID", c.Param("id"))
	}
	return id, nil
}

// planLimit returns the snapshot limit pointer for the given feature.
// Returns nil when the feature is unlimited.
func planLimit(sub *models.Subscription, feature string) *int {
	switch feature {
	case models.FeatureFlats:
		return sub.SnapshotMaxFlats
	case models.FeatureStaff:
		return sub.SnapshotMaxStaff
	case models.FeatureAdmins:
		return sub.SnapshotMaxAdmins
	default:
		return nil
	}
}

// countFeature returns the current in-use count for the given feature.
//
// NOTE: These methods perform a full SELECT to count rows.  For high-traffic
// societies consider adding COUNT-specific repository methods (e.g.
// CountFlatsBySocietyID, CountUsersBySocietyAndRole) to avoid over-fetching.
func (g *PlanGuard) countFeature(ctx context.Context, societyID int64, feature string) (int, error) {
	switch feature {

	case models.FeatureFlats:
		flats, err := g.flatRepo.GetFlatsBySocietyID(ctx, societyID)
		if err != nil {
			return 0, fmt.Errorf("plan guard: count flats: %w", err)
		}
		return len(flats), nil

	case models.FeatureStaff:
		users, err := g.userRepo.GetBySocietyAndRole(ctx, societyID, RoleStaff)
		if err != nil {
			return 0, fmt.Errorf("plan guard: count staff: %w", err)
		}
		return len(users), nil

	case models.FeatureAdmins:
		users, err := g.userRepo.GetBySocietyAndRole(ctx, societyID, RoleAdmin)
		if err != nil {
			return 0, fmt.Errorf("plan guard: count admins: %w", err)
		}
		return len(users), nil

	default:
		return 0, nil
	}
}
