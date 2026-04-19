package handler

import (
	"context"
	"net/http"
	"time"

	"go-server/internal/middleware"
	"go-server/internal/models"
	authservice "go-server/internal/service/auth_service"
	inviteservice "go-server/internal/service/invite_service"
	onboardingservice "go-server/internal/service/onboarding_service"
	"go-server/pkg/utils"
	"go-server/pkg/validator"

	"github.com/gin-gonic/gin"
)

// OnboardingHandler handles flat-claim and invite-based resident onboarding.
//
// Public (no auth required):
//   POST /onboarding/register              → RegisterResident (no OTP)
//
// Resident (requires valid JWT, any role):
//   GET  /onboarding/society/:code/flats   → GetSocietyFlats
//   POST /onboarding/claims                → SubmitClaim
//   GET  /onboarding/claims/me             → GetMyClaimHistory
//   POST /onboarding/redeem                → RedeemInvite
//
// Admin (requires admin or super_admin role):
//   GET    /societies/:id/onboarding/claims                        → ListPendingClaims
//   GET    /societies/:id/onboarding/claims/all                    → ListAllClaims
//   POST   /societies/:id/onboarding/claims/:claimId/approve       → ApproveClaim
//   POST   /societies/:id/onboarding/claims/:claimId/reject        → RejectClaim
//   POST   /societies/:id/onboarding/invites                       → CreateInvite
//   GET    /societies/:id/onboarding/flats/:flatId/invites         → GetActiveInvites
//   DELETE /societies/:id/onboarding/invites/:inviteId             → RevokeInvite
type OnboardingHandler struct {
	onboardingSvc onboardingservice.OnboardingService
	authService   authservice.AuthService
	*TokenHandler
}

func NewOnboardingHandler(
	svc onboardingservice.OnboardingService,
	authService authservice.AuthService,
	jwtSecret string,
	jwtIssuer string,
	accessExpiry time.Duration,
	refreshExpiry time.Duration,
	isProduction bool,
) *OnboardingHandler {
	return &OnboardingHandler{
		onboardingSvc: svc,
		authService:   authService,
		TokenHandler:  NewTokenHandler(jwtSecret, jwtIssuer, accessExpiry, refreshExpiry, isProduction),
	}

}

// ==================== PATH HELPERS ====================

func claimIDFromPath(c *gin.Context) (int64, bool) {
	id, err := utils.GetIDParam(c, "claimId")
	if err != nil || id <= 0 {
		utils.BadRequestResponse(c, "Invalid claim ID")
		return 0, false
	}
	return id, true
}

func inviteIDFromPath(c *gin.Context) (int64, bool) {
	id, err := utils.GetIDParam(c, "inviteId")
	if err != nil || id <= 0 {
		utils.BadRequestResponse(c, "Invalid invite ID")
		return 0, false
	}
	return id, true
}

// ==================== PUBLIC HANDLERS ====================

// RegisterResident godoc
// POST /onboarding/register
//
// Body: { "name": "John", "email": "john@example.com", "phone_number": "+911234567890", "password": "..." }
//
// Creates a resident account without requiring OTP verification.
// The account is immediately active (is_verified = true). Trust is established
// by the subsequent flat claim approval (admin reviews) or invite token (pre-vetted).
// Note: role is always forced to "user" regardless of any role field in the body.
func (h *OnboardingHandler) RegisterResident(c *gin.Context) {
	var req models.RegisterRequest
	if !bindJSON(c, &req) {
		return
	}
	req.Sanitize()
	// Force role to "user" — residents cannot self-assign privileged roles.
	req.Role = "user"

	if validationErrs := validator.ValidateStruct(&req); validationErrs != nil {
		utils.ValidationErrorResponse(c, validationErrs.ToMap())
		return
	}

	user, err := h.onboardingSvc.RegisterResident(c.Request.Context(), &req)
	if handleServiceError(c, err) {
		return
	}

	// Update last login asynchronously — don't block the response
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = h.authService.UpdateLastLogin(ctx, user.ID)
	}()

	if err := h.issueBothTokens(c, user); err != nil {
		utils.InternalServerErrorResponse(c, err)
		return
	}

	utils.SuccessResponse(c, http.StatusCreated, "Account created. You can now submit a flat claim or redeem an invite.", gin.H{
		"user": user.ToResponse(),
	})
}

// RegisterStaff godoc
// POST /societies/:id/onboarding/register/staff
//
// Body: { "name": "Alice", "email": "alice@example.com", "phone_number": "+911234567890", "password": "..." }
//
// Creates a staff account that is immediately active (is_verified = true). Only admins can call this.
// Note: role is always forced to "staff" regardless of any role field in the body. Staff users have a separate flow and permissions from residents.
func (h *OnboardingHandler) RegisterStaff(c *gin.Context) {
	societyID, ok := societyIDFromPath(c)
	if !ok {
		return
	}

	if !requireAdminRole(c) {
		return
	}

	var req models.RegisterRequest
	if !bindJSON(c, &req) {
		return
	}
	req.Sanitize()
	// Force role to "staff" — staff users have a separate flow and permissions from residents.
	req.Role = "staff"

	if validationErrs := validator.ValidateStruct(&req); validationErrs != nil {
		utils.ValidationErrorResponse(c, validationErrs.ToMap())
		return
	}

	user, err := h.onboardingSvc.RegisterStaff(c.Request.Context(), &req, societyID)
	if handleServiceError(c, err) {
		return
	}

	// Update last login asynchronously — don't block the response
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = h.authService.UpdateLastLogin(ctx, user.ID)
	}()

	if err := h.issueBothTokens(c, user); err != nil {
		utils.InternalServerErrorResponse(c, err)
		return
	}

	utils.SuccessResponse(c, http.StatusCreated, "Staff account created successfully", gin.H{
		"user": user.ToResponse(),
	})
}

// ==================== RESIDENT HANDLERS ====================

// GetSocietyFlats godoc
// GET /onboarding/society/:code/flats
//
// Returns the flat list for a society by its QR-encoded society code.
// Called during self-service onboarding so the resident can pick their flat.
// Requires a valid JWT but no society membership.
func (h *OnboardingHandler) GetSocietyFlats(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		utils.BadRequestResponse(c, "Society code is required")
		return
	}

	flats, err := h.onboardingSvc.GetSocietyFlatsByCode(c.Request.Context(), code)
	if handleServiceError(c, err) {
		return
	}

	responses := make([]*models.FlatResponse, 0, len(flats))
	for _, f := range flats {
		responses = append(responses, f.ToResponse())
	}

	utils.SuccessResponse(c, http.StatusOK, "Flats fetched successfully", gin.H{
		"flats": responses,
		"count": len(responses),
	})
}

// SubmitClaim godoc
// POST /onboarding/claims
//
// Body: { "society_id": 1, "flat_id": 42, "note": "Owner of flat A-101" }
//
// Creates a pending FlatClaimRequest. Admin must approve before the user is
// assigned to the flat and their account is activated.
func (h *OnboardingHandler) SubmitClaim(c *gin.Context) {
	userID, exists := middleware.GetUserIDFromContext(c)
	if !exists {
		utils.UnauthorizedResponse(c, "Authentication required")
		return
	}

	var body struct {
		SocietyID int64   `json:"society_id"`
		FlatID    int64   `json:"flat_id"`
		Note      *string `json:"note"`
	}
	if !bindJSON(c, &body) {
		return
	}
	if body.SocietyID <= 0 || body.FlatID <= 0 {
		utils.BadRequestResponse(c, "society_id and flat_id must be positive integers")
		return
	}

	req := &models.SubmitClaimRequest{
		UserID:    userID,
		SocietyID: body.SocietyID,
		FlatID:    body.FlatID,
		Note:      body.Note,
	}
	req.Sanitize()

	if validationErrs := validator.ValidateStruct(req); validationErrs != nil {
		utils.ValidationErrorResponse(c, validationErrs.ToMap())
		return
	}
	claim, err := h.onboardingSvc.SubmitClaim(c.Request.Context(), req)
	if handleServiceError(c, err) {
		return
	}

	utils.SuccessResponse(c, http.StatusCreated, "Claim submitted. Awaiting admin approval.", gin.H{
		"claim": claim,
	})
}

// GetMyClaimHistory godoc
// GET /onboarding/claims/me
//
// Returns all claim requests submitted by the authenticated user.
func (h *OnboardingHandler) GetMyClaimHistory(c *gin.Context) {
	userID, exists := middleware.GetUserIDFromContext(c)
	if !exists {
		utils.UnauthorizedResponse(c, "Authentication required")
		return
	}

	claims, err := h.onboardingSvc.GetMyClaimHistory(c.Request.Context(), userID)
	if handleServiceError(c, err) {
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Claim history fetched successfully", gin.H{
		"claims": claims,
		"count":  len(claims),
	})
}

// RedeemInvite godoc
// POST /onboarding/redeem
//
// Body: { "token": "<64-char hex string>" }
//
// Validates the invite token and immediately assigns the user to the flat.
// No admin approval required — trust is implied by the invite creator.
func (h *OnboardingHandler) RedeemInvite(c *gin.Context) {
	userID, exists := middleware.GetUserIDFromContext(c)
	if !exists {
		utils.UnauthorizedResponse(c, "Authentication required")
		return
	}

	var req models.RedeemInviteRequest
	if !bindJSON(c, &req) {
		return
	}
	req.Sanitize()
	req.UserID = userID

	if validationErrs := validator.ValidateStruct(&req); validationErrs != nil {
		utils.ValidationErrorResponse(c, validationErrs.ToMap())
		return
	}

	result, err := h.onboardingSvc.RedeemInvite(c.Request.Context(), &req)
	if handleServiceError(c, err) {
		return
	}

	utils.SuccessResponse(c, http.StatusOK, result.Message, gin.H{
		"user": result.User,
		"flat": result.Flat,
	})
}

// ==================== ADMIN HANDLERS ====================

// ListPendingClaims godoc
// GET /societies/:id/onboarding/claims
//
// Returns all pending claim requests in a society. Admin / super_admin only.
func (h *OnboardingHandler) ListPendingClaims(c *gin.Context) {
	societyID, ok := societyIDFromPath(c)
	if !ok {
		return
	}
	if !requireAdminRole(c) {
		return
	}

	claims, err := h.onboardingSvc.ListPendingClaims(c.Request.Context(), societyID)
	if handleServiceError(c, err) {
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Pending claims fetched successfully", gin.H{
		"claims": claims,
		"count":  len(claims),
	})
}

// ListAllClaims godoc
// GET /societies/:id/onboarding/claims/all?status=pending|approved|rejected
//
// Returns all claim requests with optional status filter. Admin only.
func (h *OnboardingHandler) ListAllClaims(c *gin.Context) {
	societyID, ok := societyIDFromPath(c)
	if !ok {
		return
	}
	if !requireAdminRole(c) {
		return
	}

	var statusFilter *models.ClaimStatus
	if raw := c.Query("status"); raw != "" {
		s := models.ClaimStatus(raw)
		if s != models.ClaimStatusPending && s != models.ClaimStatusApproved && s != models.ClaimStatusRejected {
			utils.BadRequestResponse(c, "status must be one of: pending, approved, rejected")
			return
		}
		statusFilter = &s
	}

	items, err := h.onboardingSvc.ListAllClaims(c.Request.Context(), societyID, statusFilter)
	if handleServiceError(c, err) {
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Claims fetched successfully", gin.H{
		"claims": items,
		"count":  len(items),
	})
}

// ApproveClaim godoc
// POST /societies/:id/onboarding/claims/:claimId/approve
//
// Atomically approves a pending claim:
//   - sets user.flat_id, society_id, role = primary resident
//   - activates the user account (is_verified = true)
//   - revokes all pending invites for the same flat
func (h *OnboardingHandler) ApproveClaim(c *gin.Context) {
	_, ok := societyIDFromPath(c)
	if !ok {
		return
	}
	claimID, ok := claimIDFromPath(c)
	if !ok {
		return
	}
	if !requireAdminRole(c) {
		return
	}
	reviewerID, exi := middleware.GetUserIDFromContext(c)
	if !exi {
		utils.UnauthorizedResponse(c, "Authentication required")
		return
	}

	req := models.ReviewClaimRequest{
		ReviewerID: reviewerID,
		ClaimID:    claimID,
		Status:     models.ClaimStatusApproved,
	}

	result, err := h.onboardingSvc.ApproveClaim(c.Request.Context(), &req)
	if handleServiceError(c, err) {
		return
	}

	utils.SuccessResponse(c, http.StatusOK, result.Message, gin.H{
		"user":  result.User,
		"flat":  result.Flat,
		"claim": result.Claim,
	})
}

// RejectClaim godoc
// POST /societies/:id/onboarding/claims/:claimId/reject
//
// Body: { "rejection_reason": "Unable to verify ownership" }  (optional)
//
// Rejects a pending claim and records an optional reason.
func (h *OnboardingHandler) RejectClaim(c *gin.Context) {
	_, ok := societyIDFromPath(c)
	if !ok {
		return
	}
	claimID, ok := claimIDFromPath(c)
	if !ok {
		return
	}
	if !requireAdminRole(c) {
		return
	}
	reviewerID, exi := middleware.GetUserIDFromContext(c)
	if !exi {
		utils.UnauthorizedResponse(c, "Authentication required")
		return
	}

	var body struct {
		RejectionReason *string `json:"rejection_reason"`
	}
	if !bindJSON(c, &body) {
		return
	}

	req := models.ReviewClaimRequest{
		ReviewerID:      reviewerID,
		ClaimID:         claimID,
		Status:          models.ClaimStatusRejected,
		RejectionReason: body.RejectionReason,
	}
	req.Sanitize()

	claim, err := h.onboardingSvc.RejectClaim(c.Request.Context(), &req)
	if handleServiceError(c, err) {
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Claim rejected", gin.H{
		"claim": claim,
	})
}

// CreateInvite godoc
// POST /societies/:id/onboarding/invites
//
// Body: { "flat_id": 42, "max_uses": 1, "expires_in_hours": 72 }
//
// Mints a time-bound invite token. Admin, super_admin, or the flat's primary
// resident (role=user) may call this.
func (h *OnboardingHandler) CreateInvite(c *gin.Context) {
	societyID, ok := societyIDFromPath(c)
	if !ok {
		return
	}
	actorID, exi := middleware.GetUserIDFromContext(c)
	if !exi {
		utils.UnauthorizedResponse(c, "Authentication required")
		return
	}

	role, _ := middleware.GetUserRoleFromContext(c)
	if role != "admin" && role != "super_admin" && role != "user" && role != "developer" {
		utils.ForbiddenResponse(c, "Only admins or residents can create invites")
		return
	}

	var body struct {
		FlatID         int64 `json:"flat_id"`
		MaxUses        *int  `json:"max_uses"`
		ExpiresInHours *int  `json:"expires_in_hours"`
	}
	if !bindJSON(c, &body) {
		return
	}
	if body.FlatID <= 0 {
		utils.BadRequestResponse(c, "flat_id must be a positive integer")
		return
	}

	req := inviteservice.CreateInviteRequest{
		FlatID:    body.FlatID,
		SocietyID: societyID,
		CreatedBy: actorID,
		MaxUses:   body.MaxUses,
	}
	if body.ExpiresInHours != nil {
		ttl := time.Duration(*body.ExpiresInHours) * time.Hour
		req.ExpiresIn = &ttl
	}

	resp, err := h.onboardingSvc.CreateInvite(c.Request.Context(), req)
	if handleServiceError(c, err) {
		return
	}

	utils.SuccessResponse(c, http.StatusCreated, "Invite created successfully", gin.H{
		"invite":    resp.Invite,
		"raw_token": resp.RawToken,
		"deep_link": resp.DeepLink,
	})
}

// GetActiveInvites godoc
// GET /societies/:id/onboarding/flats/:flatId/invites
//
// Lists all non-revoked invites for a flat (tokens scrubbed). Admin only.
func (h *OnboardingHandler) GetActiveInvites(c *gin.Context) {
	societyID, ok := societyIDFromPath(c)
	if !ok {
		return
	}
	flatID, ok := flatIDFromPath(c)
	if !ok {
		return
	}
	if !requireAdminRole(c) {
		return
	}

	invites, err := h.onboardingSvc.GetActiveInvitesForFlat(c.Request.Context(), flatID, societyID)
	if handleServiceError(c, err) {
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Active invites fetched successfully", gin.H{
		"invites": invites,
		"count":   len(invites),
	})
}

// RevokeInvite godoc
// DELETE /societies/:id/onboarding/invites/:inviteId
//
// Soft-deletes an invite. Admin or the original creator only.
func (h *OnboardingHandler) RevokeInvite(c *gin.Context) {
	_, ok := societyIDFromPath(c)
	if !ok {
		return
	}
	inviteID, ok := inviteIDFromPath(c)
	if !ok {
		return
	}
	actorID, exi := middleware.GetUserIDFromContext(c)
	if !exi {
		utils.UnauthorizedResponse(c, "Authentication required")
		return
	}

	if err := h.onboardingSvc.RevokeInvite(c.Request.Context(), inviteID, actorID); handleServiceError(c, err) {
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Invite revoked successfully", nil)
}

// ==================== PRIVATE HELPERS ====================

// requireAdminRole returns false and writes a 403 if the caller is not
// admin, super_admin, or developer.
func requireAdminRole(c *gin.Context) bool {
	role, _ := middleware.GetUserRoleFromContext(c)
	if role == "admin" || role == "super_admin" || role == "developer" {
		return true
	}
	utils.ForbiddenResponse(c, "Admin access required")
	return false
}


