package handler

import (
	"go-server/internal/models"
	planservice "go-server/internal/service/plan_service"
	"go-server/pkg/utils"
	"go-server/pkg/validator"
	"net/http"

	"github.com/gin-gonic/gin"
)

// PlanHandler handles all plan-related HTTP endpoints.
// Plans are global resources — not nested under societies — so routes live at /plans.
type PlanHandler struct {
	planService planservice.PlanService
}

func NewPlanHandler(planService planservice.PlanService) *PlanHandler {
	return &PlanHandler{planService: planService}
}

// ==================== HANDLERS ====================

// CreatePlan godoc
// POST /plans
//
// Creates a new subscription plan.
// billing_cycle must be "monthly" or "yearly".
// max_flats, max_staff, max_admins are optional; omit for unlimited.
// Returns 409 if a plan with the same name already exists.
func (h *PlanHandler) CreatePlan(c *gin.Context) {
	var req models.CreatePlanRequest
	if !bindJSON(c, &req) {
		return
	}

	req.Sanitize()

	if validationErrs := validator.ValidateStruct(&req); validationErrs != nil {
		utils.ValidationErrorResponse(c, validationErrs.ToMap())
		return
	}

	// Extra semantic check: billing_cycle value must be one of the accepted enum values.
	// ValidateStruct only checks the "required" tag; IsValid() checks the domain constraint.
	if !req.BillingCycle.IsValid() {
		utils.ValidationErrorResponse(c, map[string]interface{}{
			"billing_cycle": "must be 'monthly' or 'yearly'",
		})
		return
	}

	plan, err := h.planService.Create(c.Request.Context(), &req)
	if handleServiceError(c, err) {
		return
	}

	utils.SuccessResponse(c, http.StatusCreated, "Plan created successfully", gin.H{
		"plan": plan.ToResponse(),
	})
}

// GetPlan godoc
// GET /plans/:id
//
// Returns a single plan by its primary key.
// Returns 404 if the plan does not exist.
func (h *PlanHandler) GetPlan(c *gin.Context) {
	planID, err := utils.GetIDParam(c, "id")
	if err != nil || planID <= 0 {
		utils.BadRequestResponse(c, "Invalid plan ID")
		return
	}

	plan, svcErr := h.planService.GetByID(c.Request.Context(), planID)
	if handleServiceError(c, svcErr) {
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Plan fetched successfully", gin.H{
		"plan": plan.ToResponse(),
	})
}

// ListActivePlans godoc
// GET /plans
//
// Returns all plans with is_active = true, ordered by price ascending.
// This is the public-facing endpoint used by the pricing page and the mobile app
// plan selector. No authentication required.
func (h *PlanHandler) ListActivePlans(c *gin.Context) {
	plans, err := h.planService.ListActive(c.Request.Context())
	if handleServiceError(c, err) {
		return
	}

	responses := make([]*models.PlanResponse, 0, len(plans))
	for _, p := range plans {
		responses = append(responses, p.ToResponse())
	}

	utils.SuccessResponse(c, http.StatusOK, "Plans fetched successfully", gin.H{
		"plans": responses,
		"count": len(responses),
	})
}

// ListAllPlans godoc
// GET /plans/all
//
// Returns every plan regardless of active status, ordered by price ascending.
// Intended for the Super Admin dashboard to manage the full plan catalogue.
func (h *PlanHandler) ListAllPlans(c *gin.Context) {
	plans, err := h.planService.List(c.Request.Context())
	if handleServiceError(c, err) {
		return
	}

	responses := make([]*models.PlanResponse, 0, len(plans))
	for _, p := range plans {
		responses = append(responses, p.ToResponse())
	}

	utils.SuccessResponse(c, http.StatusOK, "Plans fetched successfully", gin.H{
		"plans":    responses,
		"count":    len(responses),
	})
}

// UpdatePlan godoc
// PATCH /plans/:id
//
// Partially updates plan fields: name, price, billing_cycle, and/or resource limits.
// At least one field (or Clear flag) must be provided.
// is_active cannot be changed here — use /activate or /deactivate instead.
// Returns 409 if the new name conflicts with an existing plan.
func (h *PlanHandler) UpdatePlan(c *gin.Context) {
	planID, err := utils.GetIDParam(c, "id")
	if err != nil || planID <= 0 {
		utils.BadRequestResponse(c, "Invalid plan ID")
		return
	}

	var req models.UpdatePlanRequest
	if !bindJSON(c, &req) {
		return
	}

	req.Sanitize()

	if req.IsEmpty() {
		utils.BadRequestResponse(c, "At least one field must be provided for update")
		return
	}

	if validationErrs := validator.ValidateStruct(&req); validationErrs != nil {
		utils.ValidationErrorResponse(c, validationErrs.ToMap())
		return
	}

	// billing_cycle pointer: validate enum value only when the caller is changing it.
	if req.BillingCycle != nil && !req.BillingCycle.IsValid() {
		utils.ValidationErrorResponse(c, map[string]interface{}{
			"billing_cycle": "must be 'monthly' or 'yearly'",
		})
		return
	}

	plan, svcErr := h.planService.Update(c.Request.Context(), planID, &req)
	if handleServiceError(c, svcErr) {
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Plan updated successfully", gin.H{
		"plan": plan.ToResponse(),
	})
}

// ActivatePlan godoc
// PATCH /plans/:id/activate
//
// Sets is_active = true, making the plan visible on the pricing page and selectable
// in the subscription flow. Safe to call on an already-active plan (no-op).
func (h *PlanHandler) ActivatePlan(c *gin.Context) {
	planID, err := utils.GetIDParam(c, "id")
	if err != nil || planID <= 0 {
		utils.BadRequestResponse(c, "Invalid plan ID")
		return
	}

	plan, svcErr := h.planService.Activate(c.Request.Context(), planID)
	if handleServiceError(c, svcErr) {
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Plan activated successfully", gin.H{
		"plan": plan.ToResponse(),
	})
}

// DeactivatePlan godoc
// PATCH /plans/:id/deactivate
//
// Sets is_active = false, preventing new subscriptions to this plan.
// Existing active subscriptions that reference this plan are NOT affected.
// Safe to call on an already-inactive plan (no-op).
func (h *PlanHandler) DeactivatePlan(c *gin.Context) {
	planID, err := utils.GetIDParam(c, "id")
	if err != nil || planID <= 0 {
		utils.BadRequestResponse(c, "Invalid plan ID")
		return
	}

	plan, svcErr := h.planService.Deactivate(c.Request.Context(), planID)
	if handleServiceError(c, svcErr) {
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Plan deactivated successfully", gin.H{
		"plan": plan.ToResponse(),
	})
}