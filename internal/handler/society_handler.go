package handler

import (
	"go-server/internal/middleware"
	"go-server/internal/models"
	societyservice "go-server/internal/service/society_service"
	"go-server/pkg/utils"
	"go-server/pkg/validator"
	"net/http"

	"github.com/gin-gonic/gin"
)

type SocietyHandler struct {
	societyService societyservice.SocietyService
}

func NewSocietyHandler(
	societyService societyservice.SocietyService,
) *SocietyHandler {
	return &SocietyHandler{
		societyService: societyService,
	}
}

// RegisterSociety godoc
// POST /societies
func (h *SocietyHandler) RegisterSociety(c *gin.Context) {
	var req models.CreateSocietyRequest

	if !bindJSON(c, &req) {
		return
	}

	req.Sanitize()

	if validationErr := validator.ValidateStruct(&req); validationErr != nil {
		utils.ValidationErrorResponse(c, validationErr.ToMap())
		return
	}

	creatorID, exists := middleware.GetUserIDFromContext(c)
	if !exists {
		utils.UnauthorizedResponse(c, "Authentication required")
		return
	}
	req.CreatorID = creatorID

	society, err := h.societyService.Create(c.Request.Context(), &req)
	if handleServiceError(c, err) {
		return
	}

	utils.SuccessResponse(c, http.StatusCreated, "Society Registered Successfully", gin.H{
		"society": society.ToResponse(),
		"message": "Society created successfully and trial plan is assigned to you. Thanks",
	})
}

// GetSocietyDetails godoc
// GET /societies/:id
func (h *SocietyHandler) GetSocietyDetails(c *gin.Context) {
	societyID, err := utils.GetIDParam(c, "id")
	if err != nil {
		utils.BadRequestResponse(c, "Invalid society ID")
		return
	}

	society, err := h.societyService.GetByID(c.Request.Context(), societyID)
	if handleServiceError(c, err) {
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Society details fetched successfully", gin.H{
		"society": society.ToResponse(),
	})
}

// GetSocietyByCode godoc
// GET /societies/code/:code
func (h *SocietyHandler) GetSocietyByCode(c *gin.Context) {
	code := c.Param("code")

	if code == "" {
		utils.BadRequestResponse(c, "Invalid society code")
		return
	}

	society, err := h.societyService.GetByCode(c.Request.Context(), code)
	if handleServiceError(c, err) {
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Society fetched successfully", gin.H{
		"society": society.ToResponse(),
	})
}

// ListSocieties godoc
// GET /societies
func (h *SocietyHandler) ListSocieties(c *gin.Context) {
	var filter models.SocietyFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		utils.BadRequestResponse(c, "Invalid query parameters")
		return
	}

	filter.Sanitize()

	societies, err := h.societyService.List(c.Request.Context(), filter)
	if handleServiceError(c, err) {
		return
	}

	responses := make([]interface{}, 0, len(societies))
	for _, s := range societies {
		responses = append(responses, s.ToResponse())
	}

	utils.SuccessResponse(c, http.StatusOK, "Societies fetched successfully", gin.H{
		"societies": responses,
		"count":     len(responses),
	})
}

// UpdateSociety godoc
// PATCH /societies/:id
func (h *SocietyHandler) UpdateSociety(c *gin.Context) {
	societyID, err := utils.GetIDParam(c, "id")
	if err != nil {
		utils.BadRequestResponse(c, "Invalid society ID")
		return
	}

	var req models.UpdateSocietyRequest
	if !bindJSON(c, &req) {
		return
	}

	req.Sanitize()

	if req.IsEmpty() {
		utils.BadRequestResponse(c, "At least one field must be provided for update")
		return
	}

	if validationErr := validator.ValidateStruct(&req); validationErr != nil {
		utils.ValidationErrorResponse(c, validationErr.ToMap())
		return
	}

	society, err := h.societyService.Update(c.Request.Context(), societyID, &req)
	if handleServiceError(c, err) {
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Society updated successfully", gin.H{
		"society": society.ToResponse(),
	})
}

// ActivateSociety godoc
// PATCH /societies/:id/activate
func (h *SocietyHandler) ActivateSociety(c *gin.Context) {
	societyID, err := utils.GetIDParam(c, "id")
	if err != nil {
		utils.BadRequestResponse(c, "Invalid society ID")
		return
	}

	if handleServiceError(c, h.societyService.Activate(c.Request.Context(), societyID)) {
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Society activated successfully", nil)
}

// DeactivateSociety godoc
// PATCH /societies/:id/deactivate
func (h *SocietyHandler) DeactivateSociety(c *gin.Context) {
	societyID, err := utils.GetIDParam(c, "id")
	if err != nil {
		utils.BadRequestResponse(c, "Invalid society ID")
		return
	}

	if handleServiceError(c, h.societyService.Deactivate(c.Request.Context(), societyID)) {
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Society deactivated successfully", nil)
}

// DeleteSociety godoc
// DELETE /societies/:id
func (h *SocietyHandler) DeleteSociety(c *gin.Context) {
	societyID, err := utils.GetIDParam(c, "id")
	if err != nil {
		utils.BadRequestResponse(c, "Invalid society ID")
		return
	}

	if handleServiceError(c, h.societyService.Delete(c.Request.Context(), societyID)) {
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Society deleted successfully", nil)
}