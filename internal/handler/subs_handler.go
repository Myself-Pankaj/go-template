package handler

import (
	"go-server/internal/models"
	subsservice "go-server/internal/service/subscription_service"
	"go-server/pkg/utils"
	"go-server/pkg/validator"
	"net/http"

	"github.com/gin-gonic/gin"
)

type SubscriptionHandler struct {
	subsService subsservice.SubscriptionService
}

func NewSubscriptionHandler(subsService subsservice.SubscriptionService) *SubscriptionHandler {
	return &SubscriptionHandler{subsService: subsService}
}

// ==================== HELPERS ====================

// societyIDFromPath extracts and validates the :id URL path parameter.
// Returns false and writes a 400 response when the param is missing or non-positive.
func societyIDFromPath(c *gin.Context) (int64, bool) {
	id, err := utils.GetIDParam(c, "id")
	if err != nil || id <= 0 {
		utils.BadRequestResponse(c, "Invalid society ID")
		return 0, false
	}
	return id, true
}

// ==================== HANDLERS ====================

// Subscribe godoc
// POST /societies/:societyId/subscriptions
//
// Creates a new subscription for the given society.
// Fails if the society already has an active or cancel_pending subscription —
// use PATCH /change-plan to switch plans.
func (h *SubscriptionHandler) Subscribe(c *gin.Context) {
	societyID, ok := societyIDFromPath(c)
	if !ok {
		return
	}

	var req models.SubscribeRequest
	if !bindJSON(c, &req) {
		return
	}

	if validationErrs := validator.ValidateStruct(&req); validationErrs != nil {
		utils.ValidationErrorResponse(c, validationErrs.ToMap())
		return
	}

	sub, err := h.subsService.Subscribe(c.Request.Context(), societyID, req.PlanID)
	if handleServiceError(c, err) {
		return
	}

	utils.SuccessResponse(c, http.StatusCreated, "Subscription created successfully", gin.H{
		"subscription": sub.ToResponse(),
	})
}

// GetActiveSubscription godoc
// GET /societies/:societyId/subscriptions/active
//
// Returns the single live subscription (status ∈ {active, cancel_pending} and
// end_date > now). Returns 404 when no active subscription exists.
func (h *SubscriptionHandler) GetActiveSubscription(c *gin.Context) {
	societyID, ok := societyIDFromPath(c)
	if !ok {
		return
	}

	sub, err := h.subsService.GetActiveSubscription(c.Request.Context(), societyID)
	if handleServiceError(c, err) {
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Active subscription fetched successfully", gin.H{
		"subscription": sub.ToResponse(),
	})
}

// ListSubscriptions godoc
// GET /societies/:societyId/subscriptions
//
// Returns the full subscription history for the society, newest first.
// Includes all statuses (active, cancel_pending, cancelled, expired).
func (h *SubscriptionHandler) ListSubscriptions(c *gin.Context) {
	societyID, ok := societyIDFromPath(c)
	if !ok {
		return
	}

	subs, err := h.subsService.ListSubscriptions(c.Request.Context(), societyID)
	if handleServiceError(c, err) {
		return
	}

	responses := make([]*models.SubscriptionResponse, 0, len(subs))
	for _, s := range subs {
		responses = append(responses, s.ToResponse())
	}

	utils.SuccessResponse(c, http.StatusOK, "Subscription history fetched successfully", gin.H{
		"subscriptions": responses,
		"count":         len(responses),
	})
}

// ChangePlan godoc
// PATCH /societies/:societyId/subscriptions/change-plan
//
// Atomically closes the current subscription and opens a new one on the
// requested plan.  The society is never left without a subscription on failure
// because both writes are wrapped in a single DB transaction inside the service.
func (h *SubscriptionHandler) ChangePlan(c *gin.Context) {
	societyID, ok := societyIDFromPath(c)
	if !ok {
		return
	}

	var req models.ChangePlanRequest
	if !bindJSON(c, &req) {
		return
	}

	if validationErrs := validator.ValidateStruct(&req); validationErrs != nil {
		utils.ValidationErrorResponse(c, validationErrs.ToMap())
		return
	}

	sub, err := h.subsService.ChangePlan(c.Request.Context(), societyID, req.NewPlanID)
	if handleServiceError(c, err) {
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Plan changed successfully", gin.H{
		"subscription": sub.ToResponse(),
	})
}

// Cancel godoc
// PATCH /societies/:societyId/subscriptions/cancel
//
// Cancels the active subscription.
//   - cancel_at_period_end: true  → status → cancel_pending (access until end_date)
//   - cancel_at_period_end: false → status → cancelled immediately
func (h *SubscriptionHandler) Cancel(c *gin.Context) {
	societyID, ok := societyIDFromPath(c)
	if !ok {
		return
	}

	var req models.CancelSubscriptionRequest
	if !bindJSON(c, &req) {
		return
	}

	// CancelSubscriptionRequest has no string fields requiring sanitization.
	// No extra validation needed beyond JSON parsing since the only field is a bool.

	if err := h.subsService.Cancel(c.Request.Context(), societyID, req.CancelAtPeriodEnd); handleServiceError(c, err) {
		return
	}

	msg := "Subscription cancelled immediately"
	if req.CancelAtPeriodEnd {
		msg = "Subscription will be cancelled at the end of the billing period"
	}
	utils.SuccessResponse(c, http.StatusOK, msg, nil)
}

// Renew godoc
// PATCH /societies/:societyId/subscriptions/renew
//
// Extends the latest subscription by one billing period.
// If renewing before expiry the new end_date extends from the natural expiry
// (preserving remaining days). If renewing after expiry it starts from today.
func (h *SubscriptionHandler) Renew(c *gin.Context) {
	societyID, ok := societyIDFromPath(c)
	if !ok {
		return
	}

	sub, err := h.subsService.Renew(c.Request.Context(), societyID)
	if handleServiceError(c, err) {
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Subscription renewed successfully", gin.H{
		"subscription": sub.ToResponse(),
	})
}

// IsActive godoc
// GET /societies/:societyId/subscriptions/is-active
//
// Fast-path check for middleware and feature gates.
// Returns {"active": true/false} without exposing subscription details.
func (h *SubscriptionHandler) IsActive(c *gin.Context) {
	societyID, ok := societyIDFromPath(c)
	if !ok {
		return
	}

	active, err := h.subsService.IsActive(c.Request.Context(), societyID)
	if handleServiceError(c, err) {
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Subscription status fetched successfully", gin.H{
		"active": active,
	})
}
