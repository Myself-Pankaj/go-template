package models

import (
	"strings"
	"time"
)

type FlatStatus string

const (
	FlatStatusActive   FlatStatus = "active"
	FlatStatusInactive FlatStatus = "inactive"
)

// ==================== ENTITY ====================

type Flat struct {
	ID         int64      `db:"id"          json:"id"`
	SocietyID  int64      `db:"society_id"  json:"society_id"`
	FlatNumber string     `db:"flat_number" json:"flat_number"`
	Floor      *int       `db:"floor"       json:"floor,omitempty"`
	Block      *string    `db:"block"       json:"block,omitempty"`
	Status     FlatStatus `db:"status"      json:"status"`
	CreatedAt  time.Time  `db:"created_at"  json:"created_at"`
	UpdatedAt  time.Time  `db:"updated_at"  json:"updated_at"`
}

// ToResponse converts the entity into a client-safe JSON shape.
func (f *Flat) ToResponse() *FlatResponse {
	return &FlatResponse{
		ID:         f.ID,
		SocietyID:  f.SocietyID,
		FlatNumber: f.FlatNumber,
		Floor:      f.Floor,
		Block:      f.Block,
		Status:     string(f.Status),
		CreatedAt:  f.CreatedAt,
		UpdatedAt:  f.UpdatedAt,
	}
}

// ==================== RESPONSE ====================

// FlatResponse is the JSON payload returned to API consumers.
// It deliberately uses string for Status so the client never needs to
// know the Go type alias.
type FlatResponse struct {
	ID         int64     `json:"id"`
	SocietyID  int64     `json:"society_id"`
	FlatNumber string    `json:"flat_number"`
	Floor      *int      `json:"floor,omitempty"`
	Block      *string   `json:"block,omitempty"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// ==================== CREATE ====================

// CreateFlatRequest is the validated binding for POST /societies/:id/flats.
// SocietyID is injected from the URL path in the handler — not the request body.
type CreateFlatRequest struct {
	// flat_number: required, 1-20 chars, e.g. "A-101" or "B202"
	FlatNumber string  `json:"flat_number" validate:"required,min=1,max=20"`
	Floor      *int    `json:"floor"       validate:"omitempty,min=0,max=200"`
	Block      *string `json:"block"       validate:"omitempty,min=1,max=10"`

	// Injected from URL path — never from request body.
	SocietyID int64 `json:"-"`
}

// Sanitize normalises string fields before validation.
// flat_number is upper-cased so "a-101" and "A-101" are treated as the same flat.
func (r *CreateFlatRequest) Sanitize() {
	r.FlatNumber = strings.ToUpper(strings.TrimSpace(r.FlatNumber))
	if r.Block != nil {
		b := strings.ToUpper(strings.TrimSpace(*r.Block))
		r.Block = &b
	}
}

// ==================== UPDATE ====================

// UpdateFlatRequest is the validated binding for PATCH /societies/:id/flats/:flatId.
// All fields are optional pointers — only non-nil fields are applied.
// Status changes (activate/deactivate) use dedicated endpoints.
type UpdateFlatRequest struct {
	FlatNumber *string `json:"flat_number" validate:"omitempty,min=1,max=20"`
	Floor      *int    `json:"floor"       validate:"omitempty,min=0,max=200"`
	Block      *string `json:"block"       validate:"omitempty,min=1,max=10"`
}

// Sanitize trims and upper-cases non-nil string pointer fields.
func (r *UpdateFlatRequest) Sanitize() {
	if r.FlatNumber != nil {
		v := strings.ToUpper(strings.TrimSpace(*r.FlatNumber))
		r.FlatNumber = &v
	}
	if r.Block != nil {
		v := strings.ToUpper(strings.TrimSpace(*r.Block))
		r.Block = &v
	}
}

// IsEmpty reports whether the request carries no changes at all.
func (r *UpdateFlatRequest) IsEmpty() bool {
	return r.FlatNumber == nil && r.Floor == nil && r.Block == nil
}

// ==================== INTERNAL PARAMS (service → repository) ====================

// CreateFlatParams carries validated input into the repository.
// Normalization (trim, uppercase) is done in FlatService before this is built.
type CreateFlatParams struct {
	SocietyID  int64
	FlatNumber string
	Floor      *int
	Block      *string
}