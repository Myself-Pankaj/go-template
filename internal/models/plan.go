package models

import (
	"strings"
	"time"
)

// BillingCycle is the set of valid billing cycles enforced by the DB CHECK constraint.
type BillingCycle string

const (
	BillingCycleMonthly BillingCycle = "monthly"
	BillingCycleYearly  BillingCycle = "yearly"
)

// IsValid reports whether the billing cycle is one of the accepted values.
func (b BillingCycle) IsValid() bool {
	return b == BillingCycleMonthly || b == BillingCycleYearly
}

// Plan represents a subscription plan stored in the database.
// MaxFlats, MaxStaff, and MaxAdmins are nullable — nil means unlimited.
type Plan struct {
	Id           int64        `json:"id"            db:"id"`
	Name         string       `json:"name"          db:"name"`
	Price        float64      `json:"price"         db:"price"`
	BillingCycle BillingCycle `json:"billing_cycle" db:"billing_cycle"`
	MaxFlats     *int         `json:"max_flats"     db:"max_flats"`  // nil = unlimited
	MaxStaff     *int         `json:"max_staff"     db:"max_staff"`  // nil = unlimited
	MaxAdmins    *int         `json:"max_admins"    db:"max_admins"` // nil = unlimited
	IsActive     bool         `json:"is_active"     db:"is_active"`
	CreatedAt    time.Time    `json:"created_at"    db:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"    db:"updated_at"`
}

// ToResponse returns a client-safe JSON representation of the plan.
func (p *Plan) ToResponse() *PlanResponse {
	return &PlanResponse{
		ID:           p.Id,
		Name:         p.Name,
		Price:        p.Price,
		BillingCycle: string(p.BillingCycle),
		MaxFlats:     p.MaxFlats,
		MaxStaff:     p.MaxStaff,
		MaxAdmins:    p.MaxAdmins,
		IsActive:     p.IsActive,
		CreatedAt:    p.CreatedAt,
		UpdatedAt:    p.UpdatedAt,
	}
}

// ==================== RESPONSE ====================

// PlanResponse is the JSON payload returned to API consumers.
// Limit fields are omitted when nil (unlimited) so the client can display
// "Unlimited" without needing to handle a null value explicitly.
type PlanResponse struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	Price        float64   `json:"price"`
	BillingCycle string    `json:"billing_cycle"`
	MaxFlats     *int      `json:"max_flats,omitempty"`
	MaxStaff     *int      `json:"max_staff,omitempty"`
	MaxAdmins    *int      `json:"max_admins,omitempty"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ==================== CREATE ====================

// CreatePlanRequest holds the validated input for creating a new plan.
// Limit fields are optional pointers — nil means the plan has no cap on that resource.
type CreatePlanRequest struct {
	Name         string       `json:"name"          validate:"required,min=2,max=50"`
	Price        float64      `json:"price"         validate:"required,min=0"`
	BillingCycle BillingCycle `json:"billing_cycle" validate:"required"`
	MaxFlats     *int         `json:"max_flats"     validate:"omitempty,min=1"`
	MaxStaff     *int         `json:"max_staff"     validate:"omitempty,min=1"`
	MaxAdmins    *int         `json:"max_admins"    validate:"omitempty,min=1"`
}

// Sanitize trims whitespace from string fields and normalises BillingCycle to lowercase.
func (r *CreatePlanRequest) Sanitize() {
	r.Name = strings.TrimSpace(r.Name)
	r.BillingCycle = BillingCycle(strings.ToLower(strings.TrimSpace(string(r.BillingCycle))))
}

// ==================== UPDATE ====================

// UpdatePlanRequest holds the validated input for updating an existing plan.
// All fields are optional pointers — only non-nil fields are applied to the record.
//
// IsActive is intentionally excluded. Plan activation state is managed exclusively
// through the dedicated Activate / Deactivate service methods to prevent accidental
// status changes via the general update endpoint.
//
// To explicitly remove a limit (set it to unlimited), use the dedicated
// ClearMaxFlats / ClearMaxStaff / ClearMaxAdmins booleans instead of passing nil,
// because nil here means "don't touch this field".
type UpdatePlanRequest struct {
	Name         *string       `json:"name"          validate:"omitempty,min=2,max=50"`
	Price        *float64      `json:"price"         validate:"omitempty,min=0"`
	BillingCycle *BillingCycle `json:"billing_cycle" validate:"omitempty"`
	MaxFlats     *int          `json:"max_flats"     validate:"omitempty,min=1"`
	MaxStaff     *int          `json:"max_staff"     validate:"omitempty,min=1"`
	MaxAdmins    *int          `json:"max_admins"    validate:"omitempty,min=1"`

	// Clear flags set the corresponding limit to NULL (unlimited).
	// These take precedence over the corresponding value fields above.
	ClearMaxFlats  bool `json:"clear_max_flats"`
	ClearMaxStaff  bool `json:"clear_max_staff"`
	ClearMaxAdmins bool `json:"clear_max_admins"`
}

// Sanitize trims whitespace and normalises BillingCycle on non-nil pointer fields.
func (r *UpdatePlanRequest) Sanitize() {
	trimPtr := func(p *string) *string {
		if p == nil {
			return nil
		}
		t := strings.TrimSpace(*p)
		return &t
	}
	r.Name = trimPtr(r.Name)

	if r.BillingCycle != nil {
		normalised := BillingCycle(strings.ToLower(strings.TrimSpace(string(*r.BillingCycle))))
		r.BillingCycle = &normalised
	}
}

// IsEmpty reports whether the request carries no updates at all.
func (r *UpdatePlanRequest) IsEmpty() bool {
	return r.Name == nil &&
		r.Price == nil &&
		r.BillingCycle == nil &&
		r.MaxFlats == nil &&
		r.MaxStaff == nil &&
		r.MaxAdmins == nil &&
		!r.ClearMaxFlats &&
		!r.ClearMaxStaff &&
		!r.ClearMaxAdmins
}