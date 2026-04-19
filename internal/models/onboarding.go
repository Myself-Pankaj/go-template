package models

import (
	"strings"
	"time"
)

// ==================== ENUMS ====================

type ClaimStatus string

const (
	ClaimStatusPending  ClaimStatus = "pending"
	ClaimStatusApproved ClaimStatus = "approved"
	ClaimStatusRejected ClaimStatus = "rejected"
)

// ==================== ENTITY ====================

// FlatClaimRequest is created when a resident scans the General Society Join
// QR code and picks their flat from the dropdown. It always requires admin
// approval before the user is activated and the flat is marked occupied.
type FlatClaimRequest struct {
	ID              int64       `db:"id"               json:"id"`
	UserID          int64       `db:"user_id"          json:"user_id"`
	FlatID          int64       `db:"flat_id"          json:"flat_id"`
	SocietyID       int64       `db:"society_id"       json:"society_id"`
	Status          ClaimStatus `db:"status"           json:"status"`
	Note            *string     `db:"note"             json:"note,omitempty"`
	ReviewedBy      *int64      `db:"reviewed_by"      json:"reviewed_by,omitempty"`
	ReviewedAt      *time.Time  `db:"reviewed_at"      json:"reviewed_at,omitempty"`
	RejectionReason *string     `db:"rejection_reason" json:"rejection_reason,omitempty"`
	CreatedAt       time.Time   `db:"created_at"       json:"created_at"`
	UpdatedAt       time.Time   `db:"updated_at"       json:"updated_at"`
}

// IsPending returns true if this claim has not yet been reviewed.
func (c *FlatClaimRequest) IsPending() bool { return c.Status == ClaimStatusPending }

// IsApproved returns true if this claim has been approved.
func (c *FlatClaimRequest) IsApproved() bool { return c.Status == ClaimStatusApproved }

// IsRejected returns true if this claim has been rejected.
func (c *FlatClaimRequest) IsRejected() bool { return c.Status == ClaimStatusRejected }

// ==================== REQUEST / RESPONSE DTOs ====================

// SubmitClaimRequest is sent by a resident after picking a flat from the
// society's flat list during QR-based onboarding.
// SocietyID and FlatID come from the request body — the resident explicitly
// identifies the society (via the QR code) and the flat they are claiming.
type SubmitClaimRequest struct {
	// Injected from JWT context — never from the request body.
	UserID int64 `json:"-"`

	// Provided in the request body.
	SocietyID int64   `json:"society_id" validate:"required,min=1"`
	FlatID    int64   `json:"flat_id"    validate:"required,min=1"`
	Note      *string `json:"note"       validate:"omitempty,max=500"`
}

// Sanitize trims whitespace from the optional note.
func (r *SubmitClaimRequest) Sanitize() {
	if r.Note != nil {
		s := strings.TrimSpace(*r.Note)
		r.Note = &s
	}
}

// ReviewClaimRequest is sent by an admin when approving or rejecting a claim.
type ReviewClaimRequest struct {
	// Injected from JWT context.
	ReviewerID int64 `json:"-"`

	ClaimID         int64       `json:"-"` // from URL path
	Status          ClaimStatus `json:"status"           validate:"required,oneof=approved rejected"`
	RejectionReason *string     `json:"rejection_reason" validate:"omitempty,max=500"`
}

// Sanitize trims the rejection reason.
func (r *ReviewClaimRequest) Sanitize() {
	if r.RejectionReason != nil {
		s := strings.TrimSpace(*r.RejectionReason)
		r.RejectionReason = &s
	}
}

// RedeemInviteRequest is sent by any user tapping an invite deep-link.
// The token carries all context; the caller only needs to be authenticated.
type RedeemInviteRequest struct {
	// Injected from JWT context.
	UserID int64 `json:"-"`

	Token string `json:"token" validate:"required,min=64,max=128"`
}

// Sanitize strips whitespace from the token.
func (r *RedeemInviteRequest) Sanitize() {
	r.Token = strings.TrimSpace(r.Token)
}

// OnboardingResult is the unified response returned after any successful
// onboarding action (claim submission, invite redemption, claim approval).
type OnboardingResult struct {
	User    *UserResponse     `json:"user"`
	Flat    *FlatResponse     `json:"flat"`
	Claim   *FlatClaimRequest `json:"claim,omitempty"`  // nil for invite flow
	Message string            `json:"message"`
}

// PendingClaimResponse is the response structure for a pending claim.
type PendingClaimResponse struct {
	ID        int64            `json:"id"`
	Note      *string          `json:"note,omitempty"`
	Status    ClaimStatus      `json:"status"`
	CreatedAt time.Time        `json:"created_at"`
	User      *UserResponse    `json:"user"`
	Flat      *FlatResponse    `json:"flat"`
	Society   *SocietySummary `json:"society"`
}
