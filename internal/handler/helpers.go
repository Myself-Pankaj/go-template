package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"go-server/internal/models"
	"go-server/pkg/utils"

	"github.com/gin-gonic/gin"
)

// bindJSON deserializes the request body into dst.
// On failure it writes the correct 400 response and returns false so the
// caller can do a bare `return`.
//
// Handles two common cases:
//   - json.UnmarshalTypeError  → specific field + type message
//   - everything else          → generic "Invalid request payload"
func bindJSON(c *gin.Context, dst interface{}) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		var unmarshalErr *json.UnmarshalTypeError
		if errors.As(err, &unmarshalErr) {
			utils.BadRequestResponse(c,
				fmt.Sprintf("%s must be of type %s",
					unmarshalErr.Field,
					unmarshalErr.Type.String(),
				),
			)
		} else {
			utils.BadRequestResponse(c, "Invalid request payload")
		}
		return false
	}
	return true
}

// handleServiceError writes the appropriate HTTP error response for a
// service-layer error and returns true so the caller can do a bare `return`.
// Returns false when err is nil (no error to handle).
//
// Error mapping:
//   - *models.AppError  → uses its own StatusCode / Code / Message
//   - anything else     → 500 Internal Server Error (avoids leaking internals)
func handleServiceError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	var appErr *models.AppError
	if errors.As(err, &appErr) {
		utils.ErrorResponse(c, appErr.StatusCode, appErr.Code, appErr.Message, appErr.Internal)
	} else {
		utils.InternalServerErrorResponse(c, err)
	}
	return true
}
