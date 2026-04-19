package handler

import (
	"go-server/internal/models"
	flatservice "go-server/internal/service/flat_service"
	"go-server/pkg/utils"
	"go-server/pkg/validator"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// FlatHandler handles all flat-related HTTP endpoints.
// Routes are nested under /societies/:id/flats so that every action is
// implicitly scoped to a specific society.
type FlatHandler struct {
	flatService flatservice.FlatService
}

func NewFlatHandler(flatService flatservice.FlatService) *FlatHandler {
	return &FlatHandler{flatService: flatService}
}

// ==================== PATH HELPERS ====================

// flatIDFromPath extracts and validates the :flatId URL parameter.
func flatIDFromPath(c *gin.Context) (int64, bool) {
	id, err := utils.GetIDParam(c, "flatId")
	if err != nil || id <= 0 {
		utils.BadRequestResponse(c, "Invalid flat ID")
		return 0, false
	}
	return id, true
}

// ==================== HANDLERS ====================

// CreateFlat godoc
// POST /societies/:id/flats
//
// Body: { "flat_number": "A-101", "floor": 1, "block": "A" }
// Creates a new flat in the society.
// Returns 409 if the flat_number already exists in that society.
func (h *FlatHandler) CreateFlat(c *gin.Context) {
	societyID, ok := societyIDFromPath(c)
	if !ok {
		return
	}

	var req models.CreateFlatRequest
	if !bindJSON(c, &req) {
		return
	}

	req.Sanitize()
	req.SocietyID = societyID

	if validationErrs := validator.ValidateStruct(&req); validationErrs != nil {
		utils.ValidationErrorResponse(c, validationErrs.ToMap())
		return
	}

	flat, err := h.flatService.CreateFlat(c.Request.Context(), &req)
	if handleServiceError(c, err) {
		return
	}

	utils.SuccessResponse(c, http.StatusCreated, "Flat created successfully", gin.H{
		"flat": flat.ToResponse(),
	})
}

// GetFlat godoc
// GET /societies/:id/flats/:flatId
//
// Fetches a single flat by ID.
// Returns 404 if the flat is not found.
func (h *FlatHandler) GetFlat(c *gin.Context) {
	flatID, ok := flatIDFromPath(c)
	if !ok {
		return
	}

	flat, err := h.flatService.GetByID(c.Request.Context(), flatID)
	if handleServiceError(c, err) {
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Flat fetched successfully", gin.H{
		"flat": flat.ToResponse(),
	})
}

// GetFlatByNumber godoc
// GET /societies/:id/flats/number/:flatNumber
//
// Looks up a flat within a society by its human-readable flat number (e.g. "A-101").
// Used during QR-based self-registration so residents can locate their flat.
func (h *FlatHandler) GetFlatByNumber(c *gin.Context) {
	societyID, ok := societyIDFromPath(c)
	if !ok {
		return
	}

	rawNumber := c.Param("flatNumber")
	flatNumber := strings.ToUpper(strings.TrimSpace(rawNumber))
	if flatNumber == "" {
		utils.BadRequestResponse(c, "flat_number is required")
		return
	}

	flat, err := h.flatService.GetByNumber(c.Request.Context(), societyID, flatNumber)
	if handleServiceError(c, err) {
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Flat fetched successfully", gin.H{
		"flat": flat.ToResponse(),
	})
}

// ListFlats godoc
// GET /societies/:id/flats
//
// Returns all flats belonging to a society, ordered by flat_number ascending.
func (h *FlatHandler) ListFlats(c *gin.Context) {
	societyID, ok := societyIDFromPath(c)
	if !ok {
		return
	}

	flats, err := h.flatService.ListBySociety(c.Request.Context(), societyID)
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

// UpdateFlat godoc
// PATCH /societies/:id/flats/:flatId
//
// Partially updates flat_number, floor, and/or block.
// At least one field must be provided.
// Returns 409 if the new flat_number conflicts within the same society.
func (h *FlatHandler) UpdateFlat(c *gin.Context) {
	flatID, ok := flatIDFromPath(c)
	if !ok {
		return
	}

	var req models.UpdateFlatRequest
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

	flat, err := h.flatService.UpdateFlat(c.Request.Context(), flatID, &req)
	if handleServiceError(c, err) {
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Flat updated successfully", gin.H{
		"flat": flat.ToResponse(),
	})
}

// ActivateFlat godoc
// PATCH /societies/:id/flats/:flatId/activate
//
// Marks a flat as active. Idempotent — activating an already-active flat
// succeeds without error.
func (h *FlatHandler) ActivateFlat(c *gin.Context) {
	flatID, ok := flatIDFromPath(c)
	if !ok {
		return
	}

	flat, err := h.flatService.ActivateFlat(c.Request.Context(), flatID)
	if handleServiceError(c, err) {
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Flat activated successfully", gin.H{
		"flat": flat.ToResponse(),
	})
}

// DeactivateFlat godoc
// PATCH /societies/:id/flats/:flatId/deactivate
//
// Marks a flat as inactive. Use when a resident vacates and the admin wants
// to lock the flat before a new claim is accepted.
func (h *FlatHandler) DeactivateFlat(c *gin.Context) {
	flatID, ok := flatIDFromPath(c)
	if !ok {
		return
	}

	flat, err := h.flatService.DeactivateFlat(c.Request.Context(), flatID)
	if handleServiceError(c, err) {
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Flat deactivated successfully", gin.H{
		"flat": flat.ToResponse(),
	})
}

// DeleteFlat godoc
// DELETE /societies/:id/flats/:flatId
//
// Permanently removes a flat. This will cascade to any users whose flat_id
// references this flat (ON DELETE SET NULL in the DB schema).
// Returns 404 if the flat does not exist.
func (h *FlatHandler) DeleteFlat(c *gin.Context) {
	flatID, ok := flatIDFromPath(c)
	if !ok {
		return
	}

	if handleServiceError(c, h.flatService.DeleteFlat(c.Request.Context(), flatID)) {
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Flat deleted successfully", nil)
}