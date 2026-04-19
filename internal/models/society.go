package models

import (
	"strings"
	"time"
)

// Society represents a residential society (tenant) stored in the database.
//
// DeletedAt is set on soft-delete. All queries MUST include a
// "WHERE deleted_at IS NULL" guard unless explicitly fetching deleted records.
// CreatorID is the user who registered the society and is assigned SuperAdmin role.
type Society struct {
	Id          int64      `json:"id"           db:"id"`
	Name        string     `json:"name"         db:"name"`
	Address     string     `json:"address"      db:"address"`
	City        string     `json:"city"         db:"city"`
	State       string     `json:"state"        db:"state"`
	PinCode     string     `json:"pin_code"     db:"pin_code"`
	SocietyCode string     `json:"society_code" db:"society_code"`
	CreatorID   int64      `json:"creator_id"   db:"creator_id"`
	IsActive    bool       `json:"is_active"    db:"is_active"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty" db:"deleted_at"` // nil = not deleted
	CreatedAt   time.Time  `json:"created_at"   db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"   db:"updated_at"`
}
func (s *Society) ToResponse() *SocietyResponse {
	return &SocietyResponse{
		Id:          s.Id,
		Name:        s.Name,
		Address:     s.Address,
		City:        s.City,
		State:       s.State,
		PinCode:     s.PinCode,
		SocietyCode: s.SocietyCode,
		CreatorID:   s.CreatorID,
		IsActive:    s.IsActive,
		CreatedAt:   s.CreatedAt,
		UpdatedAt:   s.UpdatedAt,
	}
}
type SocietyResponse struct {
	Id          int64     `json:"id"`
	Name        string    `json:"name"`
	Address     string    `json:"address"`
	City        string    `json:"city"`
	State       string    `json:"state"`
	PinCode     string    `json:"pin_code"`
	SocietyCode string    `json:"society_code"`
	CreatorID   int64     `json:"creator_id"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type SocietySummary struct {
	Id   int64  `json:"id"`
	Name string `json:"name"`
}

func (s *Society) ToSummary() *SocietySummary {
	return &SocietySummary{
		Id:   s.Id,
		Name: s.Name,
	}
}

// IsDeleted reports whether this society has been soft-deleted.
func (s *Society) IsDeleted() bool { return s.DeletedAt != nil }

// ==================== CREATE ====================

// CreateSocietyRequest holds the validated input for creating a new society.
// CreatorID is set from the authenticated user context in the handler — never
// from the request body — so it is tagged json:"-".
type CreateSocietyRequest struct {
	Name      string `json:"name"     validate:"required,min=2,max=100,alphanumeric_space"`
	Address   string `json:"address"  validate:"required,min=5,max=255"`
	City      string `json:"city"     validate:"required,min=2,max=100,alphanumeric_space"`
	State     string `json:"state"    validate:"required,min=2,max=100,alphanumeric_space"`
	PinCode   string `json:"pin_code" validate:"required,len=6,numeric"`
	CreatorID int64  `json:"-"` // injected from auth context, not the request body
}

// Sanitize trims whitespace from all string fields.
func (r *CreateSocietyRequest) Sanitize() {
	r.Name = strings.TrimSpace(r.Name)
	r.Address = strings.TrimSpace(r.Address)
	r.City = strings.TrimSpace(r.City)
	r.State = strings.TrimSpace(r.State)
	r.PinCode = strings.TrimSpace(r.PinCode)
}

// ==================== UPDATE ====================

// UpdateSocietyRequest holds the validated input for updating an existing society.
// All fields are optional pointers — only non-nil fields are applied.
//
// SocietyCode is excluded — it is derived data and must not change.
// IsActive is excluded — use Activate / Deactivate instead.
// CreatorID is excluded — immutable after creation.
type UpdateSocietyRequest struct {
	Name    *string `json:"name"     validate:"omitempty,min=2,max=100,alphanumeric_space"`
	Address *string `json:"address"  validate:"omitempty,min=5,max=255"`
	City    *string `json:"city"     validate:"omitempty,min=2,max=100,alphanumeric_space"`
	State   *string `json:"state"    validate:"omitempty,min=2,max=100,alphanumeric_space"`
	PinCode *string `json:"pin_code" validate:"omitempty,len=6,numeric"`
}

// Sanitize trims whitespace from all non-nil string pointer fields.
func (r *UpdateSocietyRequest) Sanitize() {
	trimPtr := func(p *string) *string {
		if p == nil {
			return nil
		}
		t := strings.TrimSpace(*p)
		return &t
	}
	r.Name = trimPtr(r.Name)
	r.Address = trimPtr(r.Address)
	r.City = trimPtr(r.City)
	r.State = trimPtr(r.State)
	r.PinCode = trimPtr(r.PinCode)
}

// IsEmpty reports whether the request carries no updates at all.
func (r *UpdateSocietyRequest) IsEmpty() bool {
	return r.Name == nil &&
		r.Address == nil &&
		r.City == nil &&
		r.State == nil &&
		r.PinCode == nil
}

// ==================== FILTER ====================

// SocietyFilter holds optional filtering criteria for list queries.
// All fields are optional — nil / zero-value means no filter on that field.
//
// City and State use case-insensitive partial matching (ILIKE).
// PinCode is matched exactly.
// ActiveOnly = true → only is_active = true rows.
// IncludeDeleted = true → include soft-deleted rows (default: false).
type SocietyFilter struct {
	City           *string `json:"city"            validate:"omitempty,min=2,max=100"`
	State          *string `json:"state"           validate:"omitempty,min=2,max=100"`
	PinCode        *string `json:"pin_code"        validate:"omitempty,len=6,numeric"`
	ActiveOnly     bool    `json:"active_only"`
	IncludeDeleted bool    `json:"include_deleted"`
}

// Sanitize trims and lowercases string filter fields for consistent ILIKE matching.
func (f *SocietyFilter) Sanitize() {
	trimLower := func(p *string) *string {
		if p == nil {
			return nil
		}
		t := strings.TrimSpace(strings.ToLower(*p))
		return &t
	}
	f.City = trimLower(f.City)
	f.State = trimLower(f.State)
	if f.PinCode != nil {
		t := strings.TrimSpace(*f.PinCode)
		f.PinCode = &t
	}
}