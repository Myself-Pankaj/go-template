package models

import "time"

// ==================== REQUEST MODELS ====================

// SubscribeRequest is the input for creating a brand-new subscription.
// SocietyID is injected from the URL parameter — never from the body.
type SubscribeRequest struct {
	PlanID int64 `json:"plan_id" validate:"required,min=1"`
}

// ChangePlanRequest is the input for upgrading or downgrading to a new plan.
type ChangePlanRequest struct {
	NewPlanID int64 `json:"new_plan_id" validate:"required,min=1"`
}

// CancelSubscriptionRequest controls whether the subscription stops immediately
// or at the end of the current billing period.
type CancelSubscriptionRequest struct {
	// CancelAtPeriodEnd = true  → status becomes cancel_pending, access continues until end_date.
	// CancelAtPeriodEnd = false → status becomes cancelled immediately.
	CancelAtPeriodEnd bool `json:"cancel_at_period_end"`
}

// ==================== RESPONSE MODEL ====================

// SubscriptionResponse is the safe outbound DTO exposed to API consumers.
// It carries a computed IsCurrentlyActive field so clients never need to
// re-implement the (status ∈ {active, cancel_pending} && end_date > now) logic.
type SubscriptionResponse struct {
	ID                   int64              `json:"id"`
	SocietyID            int64              `json:"society_id"`
	PlanID               int64              `json:"plan_id"`
	Status               SubscriptionStatus `json:"status"`
	IsTrial              bool               `json:"is_trial"`
	StartDate            time.Time          `json:"start_date"`
	EndDate              time.Time          `json:"end_date"`
	CancelledAt          *time.Time         `json:"cancelled_at,omitempty"`
	SnapshotPrice        float64            `json:"snapshot_price"`
	SnapshotBillingCycle BillingCycle       `json:"snapshot_billing_cycle"`
	SnapshotMaxFlats     *int               `json:"snapshot_max_flats"`
	SnapshotMaxStaff     *int               `json:"snapshot_max_staff"`
	SnapshotMaxAdmins    *int               `json:"snapshot_max_admins"`
	IsCurrentlyActive    bool               `json:"is_currently_active"`
	CreatedAt            time.Time          `json:"created_at"`
	UpdatedAt            time.Time          `json:"updated_at"`
}

// ToResponse converts a Subscription to its safe outbound DTO.
func (s *Subscription) ToResponse() *SubscriptionResponse {
	return &SubscriptionResponse{
		ID:                   s.Id,
		SocietyID:            s.SocietyID,
		PlanID:               s.PlanID,
		Status:               s.Status,
		IsTrial:              s.IsTrial,
		StartDate:            s.StartDate,
		EndDate:              s.EndDate,
		CancelledAt:          s.CancelledAt,
		SnapshotPrice:        s.SnapshotPrice,
		SnapshotBillingCycle: s.SnapshotBillingCycle,
		SnapshotMaxFlats:     s.SnapshotMaxFlats,
		SnapshotMaxStaff:     s.SnapshotMaxStaff,
		SnapshotMaxAdmins:    s.SnapshotMaxAdmins,
		IsCurrentlyActive:    s.IsCurrentlyActive(),
		CreatedAt:            s.CreatedAt,
		UpdatedAt:            s.UpdatedAt,
	}
}

// ==================== ENUMS ====================

// SubscriptionStatus represents the lifecycle state of a subscription.
// The valid transitions are:
//
//	active → cancelled         (immediate cancel)
//	active → cancel_pending    (cancel at period end)
//	cancel_pending → cancelled  (period end reached)
//	active → expired           (end_date passed, not renewed)
//	expired → active           (manual renewal)
type SubscriptionStatus string

const (
	// SubscriptionStatusActive means the subscription is live and grants access.
	SubscriptionStatusActive SubscriptionStatus = "active"

	// SubscriptionStatusCancelPending means the subscription will cancel at the
	// end of the current billing period. Access remains until end_date.
	SubscriptionStatusCancelPending SubscriptionStatus = "cancel_pending"

	// SubscriptionStatusCancelled means the subscription was explicitly cancelled.
	// No access is granted.
	SubscriptionStatusCancelled SubscriptionStatus = "cancelled"

	// SubscriptionStatusExpired means the billing period ended without renewal.
	// No access is granted.
	SubscriptionStatusExpired SubscriptionStatus = "expired"
)

// IsValid reports whether the status is one of the accepted values.
func (s SubscriptionStatus) IsValid() bool {
	switch s {
	case SubscriptionStatusActive,
		SubscriptionStatusCancelPending,
		SubscriptionStatusCancelled,
		SubscriptionStatusExpired:
		return true
	}
	return false
}

// IsAccessGranting reports whether this status grants feature access.
// Both active and cancel_pending subscriptions allow access until end_date.
func (s SubscriptionStatus) IsAccessGranting() bool {
	return s == SubscriptionStatusActive || s == SubscriptionStatusCancelPending
}

// ==================== FEATURE NAMES ====================

// Feature constants are the canonical keys used in ValidateUsage.
// They map directly to limit fields on the Plan struct.
const (
	FeatureFlats  = "flats"
	FeatureStaff  = "staff"
	FeatureAdmins = "admins"
)

// ==================== MODEL ====================

// Subscription represents a society's subscription to a plan.
//
// Design notes:
//   - A society can have at most one active/cancel_pending subscription at a time.
//     Historical (cancelled/expired) subscriptions are retained for audit.
//   - IsTrial is set at creation time and never changes.
//   - PlanSnapshot* fields capture plan pricing at subscription time so billing
//     history is accurate even if the plan is later repriced or deleted.
type Subscription struct {
	Id         int64              `json:"id"          db:"id"`
	SocietyID  int64              `json:"society_id"  db:"society_id"`
	PlanID     int64              `json:"plan_id"     db:"plan_id"`
	Status     SubscriptionStatus `json:"status"      db:"status"`
	IsTrial    bool               `json:"is_trial"    db:"is_trial"`
	StartDate  time.Time          `json:"start_date"  db:"start_date"`
	EndDate    time.Time          `json:"end_date"    db:"end_date"`
	CancelledAt *time.Time        `json:"cancelled_at" db:"cancelled_at"` // nil until cancelled

	// Plan snapshot — copied from the plan at subscription time.
	// Ensures billing history is stable even if the plan is later repriced.
	SnapshotPrice        float64      `json:"snapshot_price"         db:"snapshot_price"`
	SnapshotBillingCycle BillingCycle `json:"snapshot_billing_cycle" db:"snapshot_billing_cycle"`
	SnapshotMaxFlats     *int         `json:"snapshot_max_flats"     db:"snapshot_max_flats"`
	SnapshotMaxStaff     *int         `json:"snapshot_max_staff"     db:"snapshot_max_staff"`
	SnapshotMaxAdmins    *int         `json:"snapshot_max_admins"    db:"snapshot_max_admins"`

	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// IsCurrentlyActive reports whether this subscription grants access right now.
// It checks both status and that end_date has not yet passed.
func (s *Subscription) IsCurrentlyActive() bool {
	return s.Status.IsAccessGranting() && time.Now().Before(s.EndDate)
}

// EffectiveMaxFlats returns the flat limit from the snapshot, or nil if unlimited.
func (s *Subscription) EffectiveMaxFlats() *int { return s.SnapshotMaxFlats }

// EffectiveMaxStaff returns the staff limit from the snapshot, or nil if unlimited.
func (s *Subscription) EffectiveMaxStaff() *int { return s.SnapshotMaxStaff }

// EffectiveMaxAdmins returns the admin limit from the snapshot, or nil if unlimited.
func (s *Subscription) EffectiveMaxAdmins() *int { return s.SnapshotMaxAdmins }

