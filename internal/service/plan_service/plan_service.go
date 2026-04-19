package planservice

import (
	"context"
	"errors"
	"net/http"

	"go-server/internal/models"
	"go-server/internal/repository"
	"go-server/internal/service"
)

// ==================== INTERFACE ====================

type PlanService interface {
	// Create persists a new plan with the given pricing, billing cycle, and feature limits.
	Create(ctx context.Context, req *models.CreatePlanRequest) (*models.Plan, error)

	// GetByID returns a plan by its primary key.
	// Used by the subscription service when enrolling a society into a plan.
	GetByID(ctx context.Context, id int64) (*models.Plan, error)

	// GetByName returns a plan by its unique name.
	// Used to find the default Trial plan during onboarding and during migrations.
	GetByName(ctx context.Context, name string) (*models.Plan, error)

	// ListActive returns only active plans, ordered by price ascending.
	// Used by the public pricing page and mobile app plan selector.
	ListActive(ctx context.Context) ([]*models.Plan, error)

	// List returns every plan regardless of status, ordered by price ascending.
	// Used by the Super Admin dashboard to manage the full plan catalogue.
	List(ctx context.Context) ([]*models.Plan, error)

	// Update applies the non-nil fields in req to the plan identified by id.
	// Pricing and limit changes do NOT affect existing subscriptions directly —
	// that is the subscription service's concern.
	// is_active cannot be changed through Update; use Activate / Deactivate instead.
	Update(ctx context.Context, id int64, req *models.UpdatePlanRequest) (*models.Plan, error)

	// Activate makes a plan selectable in the UI by setting is_active = true.
	// Returns the updated plan. Safe to call on an already-active plan (no-op).
	Activate(ctx context.Context, id int64) (*models.Plan, error)

	// Deactivate prevents new subscriptions to this plan by setting is_active = false.
	// Existing subscriptions that reference this plan remain valid.
	// Returns the updated plan. Safe to call on an already-inactive plan (no-op).
	Deactivate(ctx context.Context, id int64) (*models.Plan, error)

	CanAddStaff(ctx context.Context, societyID int64) (bool, error)
}

// ==================== IMPLEMENTATION ====================

type planService struct {
	planRepo    repository.PlanRepository
	userRepo    repository.UserRepository
	subsRepo    repository.SubscriptionRepository
}

func NewPlanService(planRepo repository.PlanRepository, userRepo repository.UserRepository, subsRepo repository.SubscriptionRepository) PlanService {
	return &planService{planRepo: planRepo, userRepo: userRepo, subsRepo: subsRepo}
}

// ==================== METHODS ====================

// Create sanitizes, validates, and persists a new plan with is_active = true.
func (s *planService) Create(ctx context.Context, req *models.CreatePlanRequest) (*models.Plan, error) {
	req.Sanitize()

	if !req.BillingCycle.IsValid() {
		return nil, models.NewAppError(
			models.ErrCodeBadRequest,
			"billing cycle must be 'monthly' or 'yearly'",
			http.StatusBadRequest,
			nil,
		)
	}

	ctx, cancel := context.WithTimeout(ctx, service.DefaultTimeout)
	defer cancel()

	plan := &models.Plan{
		Name:         req.Name,
		Price:        req.Price,
		BillingCycle: req.BillingCycle,
		MaxFlats:     req.MaxFlats,
		MaxStaff:     req.MaxStaff,
		MaxAdmins:    req.MaxAdmins,
		IsActive:     true,
	}

	created, err := s.planRepo.CreatePlan(ctx, plan)
	if err != nil {
		return nil, mapPlanRepoError(err, "failed to create plan")
	}
	return created, nil
}

// GetByID returns the plan with the given ID.
func (s *planService) GetByID(ctx context.Context, id int64) (*models.Plan, error) {
	ctx, cancel := context.WithTimeout(ctx, service.DefaultTimeout)
	defer cancel()

	plan, err := s.planRepo.GetPlanByID(ctx, id)
	if err != nil {
		return nil, mapPlanRepoError(err, "failed to get plan")
	}
	return plan, nil
}

// GetByName returns the plan with the given unique name.
func (s *planService) GetByName(ctx context.Context, name string) (*models.Plan, error) {
	ctx, cancel := context.WithTimeout(ctx, service.DefaultTimeout)
	defer cancel()

	plan, err := s.planRepo.GetPlanByName(ctx, name)
	if err != nil {
		return nil, mapPlanRepoError(err, "failed to get plan")
	}
	return plan, nil
}

// ListActive returns all active plans ordered by price ascending.
func (s *planService) ListActive(ctx context.Context) ([]*models.Plan, error) {
	ctx, cancel := context.WithTimeout(ctx, service.DefaultTimeout)
	defer cancel()

	plans, err := s.planRepo.ListActivePlans(ctx)
	if err != nil {
		return nil, mapPlanRepoError(err, "failed to list active plans")
	}
	return plans, nil
}

// List returns all plans regardless of status, ordered by price ascending.
func (s *planService) List(ctx context.Context) ([]*models.Plan, error) {
	ctx, cancel := context.WithTimeout(ctx, service.DefaultTimeout)
	defer cancel()

	plans, err := s.planRepo.ListAllPlans(ctx)
	if err != nil {
		return nil, mapPlanRepoError(err, "failed to list plans")
	}
	return plans, nil
}

// Update applies the non-nil fields in req to the plan identified by id.
// An empty request returns the current record without making any DB write.
// is_active cannot be changed through this method.
func (s *planService) Update(ctx context.Context, id int64, req *models.UpdatePlanRequest) (*models.Plan, error) {
	ctx, cancel := context.WithTimeout(ctx, service.DefaultTimeout)
	defer cancel()

	updated, err := s.planRepo.UpdatePlan(ctx, id, req)
	if err != nil {
		return nil, mapPlanRepoError(err, "failed to update plan")
	}
	return updated, nil
}

// Activate sets is_active = true for the given plan.
// If the plan is already active, the current record is returned unchanged (no-op).
func (s *planService) Activate(ctx context.Context, id int64) (*models.Plan, error) {
	ctx, cancel := context.WithTimeout(ctx, service.DefaultTimeout)
	defer cancel()

	plan, err := s.planRepo.SetPlanActiveStatus(ctx, id, true)
	if err != nil {
		return nil, mapPlanRepoError(err, "failed to activate plan")
	}
	return plan, nil
}

// Deactivate sets is_active = false for the given plan, preventing new subscriptions.
// Existing subscriptions referencing this plan are unaffected.
// If the plan is already inactive, the current record is returned unchanged (no-op).
func (s *planService) Deactivate(ctx context.Context, id int64) (*models.Plan, error) {
	ctx, cancel := context.WithTimeout(ctx, service.DefaultTimeout)
	defer cancel()

	plan, err := s.planRepo.SetPlanActiveStatus(ctx, id, false)
	if err != nil {
		return nil, mapPlanRepoError(err, "failed to deactivate plan")
	}
	return plan, nil
}

// CanAddStaff checks if a society can add more staff members based on their current plan's limits.
// plan_service.go

func (s *planService) CanAddStaff(ctx context.Context, societyID int64) (bool, error) {
    // No new timeout here — caller (RegisterStaff) already set one.

    // 1. Get active subscription.
    sub, err := s.subsRepo.GetActiveBySocietyID(ctx, societyID)
    if err != nil {
        if errors.Is(err, repository.ErrSubscriptionNotFound) {
            return false, nil
        }
        return false, models.NewAppError(
            models.ErrCodeDatabaseError,
            "failed to get active subscription",
            http.StatusInternalServerError,
            err,
        )
    }

    // 2. Use the snapshot limit (consistent with PlanGuard), not live plan.
    //    nil snapshot means unlimited.
    if sub.SnapshotMaxStaff == nil {
        return true, nil
    }

    // 3. Count current staff.
    currentCount, err := s.userRepo.CountBySocietyAndRole(ctx, societyID, "staff")
    if err != nil {
        return false, models.NewAppError(
            models.ErrCodeDatabaseError,
            "failed to count staff",
            http.StatusInternalServerError,
            err,
        )
    }

    return currentCount < *sub.SnapshotMaxStaff, nil
}

// ==================== HELPERS ====================


// mapPlanRepoError passes well-formed AppErrors from the repository through unchanged
// and wraps anything else as a generic 500 with the provided fallback message.
func mapPlanRepoError(err error, fallbackMsg string) error {
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