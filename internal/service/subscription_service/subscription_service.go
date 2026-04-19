package subsservice

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"go-server/internal/models"
	"go-server/internal/repository"
	"go-server/internal/service"
)

// trialPlanName is the canonical name of the trial plan seeded in migrations.
const trialPlanName = "Trial"

// ==================== INTERFACE ====================

type SubscriptionService interface {
	// Subscribe creates a new subscription for a society.
	// If a trial plan is selected, IsTrial is set automatically.
	// Returns ErrActiveSubscriptionExists if the society already has a live
	// subscription — callers must use ChangePlan to replace it.
	Subscribe(ctx context.Context, societyID int64, planID int64) (*models.Subscription, error)

	// GetActiveSubscription returns the currently live subscription.
	// Checks both status ∈ {active, cancel_pending} and end_date > now.
	GetActiveSubscription(ctx context.Context, societyID int64) (*models.Subscription, error)

	// ListSubscriptions returns the full subscription history for a society,
	// newest first. Used in billing dashboards, audit logs, and admin views.
	ListSubscriptions(ctx context.Context, societyID int64) ([]*models.Subscription, error)

	// ChangePlan upgrades or downgrades to a new plan.
	// Atomically closes the current subscription and opens a new one inside a
	// single DB transaction so the society is never left without a subscription.
	// The new billing period starts immediately from today.
	ChangePlan(ctx context.Context, societyID int64, newPlanID int64) (*models.Subscription, error)

	// Cancel cancels the active subscription.
	// cancelAtPeriodEnd = true  → status becomes cancel_pending (access until end_date).
	// cancelAtPeriodEnd = false → status becomes cancelled immediately.
	Cancel(ctx context.Context, societyID int64, cancelAtPeriodEnd bool) error

	// Renew extends the active (or expired) subscription by one billing period.
	// Used for manual payment, admin override, and offline billing flows.
	Renew(ctx context.Context, societyID int64) (*models.Subscription, error)

	// IsActive returns true if the society has a currently live subscription.
	// Designed for middleware and feature-gate checks — fast path.
	IsActive(ctx context.Context, societyID int64) (bool, error)

	// ValidateUsage checks whether currentUsage is within the plan limit for
	// the given feature. Returns a 429 AppError if the limit is exceeded.
	// Valid feature names: models.FeatureFlats, FeatureStaff, FeatureAdmins.
	ValidateUsage(ctx context.Context, societyID int64, feature string, currentUsage int) error
}


// ==================== IMPLEMENTATION ====================

type subscriptionService struct {
	subRepo  repository.SubscriptionRepository
	planRepo repository.PlanRepository
	txManager        repository.TransactionManager
}

// NewSubscriptionService constructs the service.
// db is injected solely to provide WithTransaction for multi-step operations.
// planRepo is injected to validate plans without coupling to the plan service.
func NewSubscriptionService(
	subRepo repository.SubscriptionRepository,
	planRepo repository.PlanRepository,
	txManager repository.TransactionManager,
) SubscriptionService {
	return &subscriptionService{
		subRepo:  subRepo,
		planRepo: planRepo,
		txManager:        txManager,
	}
}

// ==================== METHODS ====================

// Subscribe creates a new subscription for the given society and plan.
//
// Business rules:
//  1. societyID and planID must be positive.
//  2. The plan must exist and be active.
//  3. No active/cancel_pending subscription may already exist — use ChangePlan.
//  4. start_date = today UTC, end_date = one billing period from today.
//  5. IsTrial is set when the plan name is "Trial".
func (s *subscriptionService) Subscribe(ctx context.Context, societyID int64, planID int64) (*models.Subscription, error) {
	ctx, cancel := context.WithTimeout(ctx, service.DefaultTimeout)
	defer cancel()

	// 1. Validate plan exists and is active.
	plan, err := s.planRepo.GetPlanByID(ctx, planID)
	if err != nil {
		return nil, mapSubRepoError(err, "failed to fetch plan")
	}
	if !plan.IsActive {
		return nil, models.NewAppError(
			models.ErrCodeBadRequest,
			"selected plan is not active",
			http.StatusBadRequest,
			nil,
		)
	}

	// 2. Ensure no active subscription already exists.
	existing, err := s.subRepo.GetActiveBySocietyID(ctx, societyID)
	if err != nil && !errors.Is(err, repository.ErrNoActiveSubscription) {
		return nil, mapSubRepoError(err, "failed to check existing subscription")
	}
	if existing != nil {
		return nil, repository.ErrActiveSubscriptionExists
	}

	// 3. Build and persist.
	now := time.Now().UTC()
	sub := buildSubscription(societyID, plan, now, plan.Name == trialPlanName)

	created, err := s.subRepo.CreateSubscription(ctx, sub)
	if err != nil {
		return nil, mapSubRepoError(err, "failed to create subscription")
	}
	return created, nil
}

// GetActiveSubscription returns the currently live subscription for a society.
func (s *subscriptionService) GetActiveSubscription(ctx context.Context, societyID int64) (*models.Subscription, error) {
	ctx, cancel := context.WithTimeout(ctx, service.DefaultTimeout)
	defer cancel()

	sub, err := s.subRepo.GetActiveBySocietyID(ctx, societyID)
	if err != nil {
		return nil, mapSubRepoError(err, "failed to get active subscription")
	}
	return sub, nil
}

// ListSubscriptions returns the full subscription history for a society.
func (s *subscriptionService) ListSubscriptions(ctx context.Context, societyID int64) ([]*models.Subscription, error) {
	ctx, cancel := context.WithTimeout(ctx, service.DefaultTimeout)
	defer cancel()

	subs, err := s.subRepo.ListBySocietyID(ctx, societyID)
	if err != nil {
		return nil, mapSubRepoError(err, "failed to list subscriptions")
	}
	return subs, nil
}

// ChangePlan upgrades or downgrades a society to a new plan.
//
// Business rules:
//  1. A live subscription must exist.
//  2. The new plan must exist and be active.
//  3. Changing to the same plan is rejected as a no-op.
//  4. CloseSubscription + CreateSubscription run inside a single DB transaction
//     so the society is never left without a subscription on failure.
func (s *subscriptionService) ChangePlan(ctx context.Context, societyID int64, newPlanID int64) (*models.Subscription, error) {
	ctx, cancel := context.WithTimeout(ctx, service.DefaultTimeout)
	defer cancel()

	// 1. Get current active subscription (outside tx — read-only, no contention).
	current, err := s.subRepo.GetActiveBySocietyID(ctx, societyID)
	if err != nil {
		return nil, mapSubRepoError(err, "failed to get current subscription")
	}

	// 2. Validate new plan.
	plan, err := s.planRepo.GetPlanByID(ctx, newPlanID)
	if err != nil {
		return nil, mapSubRepoError(err, "failed to fetch new plan")
	}
	if !plan.IsActive {
		return nil, models.NewAppError(
			models.ErrCodeBadRequest,
			"selected plan is not active",
			http.StatusBadRequest,
			nil,
		)
	}

	// 3. Reject same-plan change.
	if current.PlanID == newPlanID {
		return nil, models.NewAppError(
			models.ErrCodeBadRequest,
			"society is already subscribed to this plan",
			http.StatusBadRequest,
			nil,
		)
	}

	// 4. Build the replacement subscription before entering the transaction
	//    so no allocation happens inside the critical section.
	now := time.Now().UTC()
	newSub := buildSubscription(societyID, plan, now, false /* plan changes are never trial */)

	// 5. Atomically close old + create new.
	//    The transaction context (txCtx) is passed directly to each repo method.
	//    GetExecutor detects the transaction in txCtx and uses it automatically,
	//    so neither CloseSubscription nor CreateSubscription needs to know about
	//    transactions — they remain pure single-SQL operations.
	var created *models.Subscription
	txErr := s.txManager.WithTransaction(ctx, func(txCtx context.Context) error {
		if err := s.subRepo.CloseSubscription(txCtx, current.Id); err != nil {
			return err
		}

		created, err = s.subRepo.CreateSubscription(txCtx, newSub)
		if err != nil {
			return err
		}
		return nil
	})
	if txErr != nil {
		return nil, mapSubRepoError(txErr, "failed to change plan")
	}
	return created, nil
}

// Cancel cancels the active subscription.
//
// cancelAtPeriodEnd = true  → status → cancel_pending (access until end_date)
// cancelAtPeriodEnd = false → status → cancelled immediately
func (s *subscriptionService) Cancel(ctx context.Context, societyID int64, cancelAtPeriodEnd bool) error {
	if err := validateSubServiceSocietyID(societyID); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, service.DefaultTimeout)
	defer cancel()

	sub, err := s.subRepo.GetActiveBySocietyID(ctx, societyID)
	if err != nil {
		return mapSubRepoError(err, "failed to get active subscription")
	}

	if sub.Status == models.SubscriptionStatusCancelled {
		return repository.ErrSubscriptionAlreadyCancelled
	}

	targetStatus := models.SubscriptionStatusCancelled
	if cancelAtPeriodEnd {
		targetStatus = models.SubscriptionStatusCancelPending
	}

	if _, err = s.subRepo.UpdateStatus(ctx, sub.Id, targetStatus); err != nil {
		return mapSubRepoError(err, "failed to cancel subscription")
	}
	return nil
}

// Renew extends the most recent subscription by one billing period.
//
// Renewal base date: max(now, current end_date).
// This means renewing before expiry preserves the remaining days — the new
// end_date extends from the natural expiry, not from today.
func (s *subscriptionService) Renew(ctx context.Context, societyID int64) (*models.Subscription, error) {
	if err := validateSubServiceSocietyID(societyID); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, service.DefaultTimeout)
	defer cancel()

	// Fetch history so we can renew expired subscriptions too.
	subs, err := s.subRepo.ListBySocietyID(ctx, societyID)
	if err != nil {
		return nil, mapSubRepoError(err, "failed to fetch subscriptions for renewal")
	}
	if len(subs) == 0 {
		return nil, repository.ErrNoActiveSubscription
	}

	// ListBySocietyID is ordered newest-first.
	latest := subs[0]

	if latest.Status == models.SubscriptionStatusCancelled {
		return nil, models.NewAppError(
			models.ErrCodeBadRequest,
			"cannot renew a cancelled subscription — create a new subscription instead",
			http.StatusBadRequest,
			nil,
		)
	}

	base := latest.EndDate
	if time.Now().UTC().After(base) {
		base = time.Now().UTC()
	}
	newEndDate := calcEndDate(base, latest.SnapshotBillingCycle)

	renewed, err := s.subRepo.RenewSubscription(ctx, latest.Id, newEndDate)
	if err != nil {
		return nil, mapSubRepoError(err, "failed to renew subscription")
	}
	return renewed, nil
}

// IsActive returns true when the society has a currently live subscription.
func (s *subscriptionService) IsActive(ctx context.Context, societyID int64) (bool, error) {
	if err := validateSubServiceSocietyID(societyID); err != nil {
		return false, err
	}

	ctx, cancel := context.WithTimeout(ctx, service.DefaultTimeout)
	defer cancel()

	sub, err := s.subRepo.GetActiveBySocietyID(ctx, societyID)
	if err != nil {
		if errors.Is(err, repository.ErrNoActiveSubscription) {
			return false, nil
		}
		return false, mapSubRepoError(err, "failed to check subscription status")
	}

	return sub.IsCurrentlyActive(), nil
}

// ValidateUsage checks whether currentUsage is within the plan limit for feature.
//
// Limits are read from the subscription snapshot, not the live plan, so a
// society's entitlement is stable for the duration of the billing period.
//
// Returns nil when the feature is unlimited (nil limit pointer) or within bounds.
// Returns a 429 AppError when the limit is reached or exceeded.
func (s *subscriptionService) ValidateUsage(ctx context.Context, societyID int64, feature string, currentUsage int) error {
	if err := validateSubServiceSocietyID(societyID); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, service.DefaultTimeout)
	defer cancel()

	sub, err := s.subRepo.GetActiveBySocietyID(ctx, societyID)
	if err != nil {
		return mapSubRepoError(err, "failed to fetch subscription for usage check")
	}

	if !sub.IsCurrentlyActive() {
		return models.NewAppError(
			models.ErrCodeForbidden,
			"subscription is not active",
			http.StatusForbidden,
			nil,
		)
	}

	limit, ok := featureLimit(sub, feature)
	if !ok {
		return models.NewAppError(
			models.ErrCodeBadRequest,
			fmt.Sprintf("unknown feature: %q", feature),
			http.StatusBadRequest,
			nil,
		)
	}

	// nil limit = unlimited, always passes.
	if limit == nil {
		return nil
	}

	if currentUsage >= *limit {
		return models.NewAppError(
			models.ErrCodeLimitExceeded,
			fmt.Sprintf("%s limit of %d reached (current: %d)", feature, *limit, currentUsage),
			http.StatusTooManyRequests,
			nil,
		)
	}
	return nil
}

// ==================== HELPERS ====================

// buildSubscription constructs a Subscription from a Plan, snapshotting all
// limit fields at creation time so billing history is stable against plan edits.
func buildSubscription(societyID int64, plan *models.Plan, now time.Time, isTrial bool) *models.Subscription {
	return &models.Subscription{
		SocietyID:            societyID,
		PlanID:               plan.Id,
		Status:               models.SubscriptionStatusActive,
		IsTrial:              isTrial,
		StartDate:            now,
		EndDate:              calcEndDate(now, plan.BillingCycle),
		SnapshotPrice:        plan.Price,
		SnapshotBillingCycle: plan.BillingCycle,
		SnapshotMaxFlats:     plan.MaxFlats,
		SnapshotMaxStaff:     plan.MaxStaff,
		SnapshotMaxAdmins:    plan.MaxAdmins,
	}
}

// calcEndDate advances start by one billing period.
func calcEndDate(start time.Time, cycle models.BillingCycle) time.Time {
	switch cycle {
	case models.BillingCycleYearly:
		return start.AddDate(1, 0, 0)
	default:
		return start.AddDate(0, 1, 0)
	}
}

// featureLimit resolves a feature name to the corresponding snapshot limit.
// Returns (nil, true) for unlimited features, (nil, false) for unknown names.
func featureLimit(sub *models.Subscription, feature string) (*int, bool) {
	switch feature {
	case models.FeatureFlats:
		return sub.EffectiveMaxFlats(), true
	case models.FeatureStaff:
		return sub.EffectiveMaxStaff(), true
	case models.FeatureAdmins:
		return sub.EffectiveMaxAdmins(), true
	default:
		return nil, false
	}
}

// validateSubServiceIDs fast-fails both a society ID and a plan ID.
func validateSubServiceIDs(societyID, planID int64) error {
	if societyID <= 0 {
		return models.NewAppError(models.ErrCodeBadRequest, "invalid society ID", http.StatusBadRequest, nil)
	}
	if planID <= 0 {
		return models.NewAppError(models.ErrCodeBadRequest, "invalid plan ID", http.StatusBadRequest, nil)
	}
	return nil
}

// validateSubServiceSocietyID fast-fails a society ID.
func validateSubServiceSocietyID(societyID int64) error {
	if societyID <= 0 {
		return models.NewAppError(models.ErrCodeBadRequest, "invalid society ID", http.StatusBadRequest, nil)
	}
	return nil
}

// mapSubRepoError passes well-formed AppErrors through unchanged and wraps
// anything else as a generic 500.
func mapSubRepoError(err error, fallbackMsg string) error {
	var appErr *models.AppError
	if errors.As(err, &appErr) {
		return err
	}
	return models.NewAppError(
		models.ErrCodeDatabaseError,
		fallbackMsg,
		http.StatusInternalServerError,
		err,
	)
}