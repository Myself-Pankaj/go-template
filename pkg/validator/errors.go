package validator

import "go-server/internal/models"

var (
	ErrInvalidEmailFormat = models.NewAppError(
		models.ErrCodeBadRequest,
		"invalid email format",
		400,
		nil,
	)
	ErrInvalidPhoneNumberFormat = models.NewAppError(
		models.ErrCodeBadRequest,
		"invalid phone number format",
		400,
		nil,
	)
)