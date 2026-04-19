package societyservice

import (
	"context"
	"errors"
	"net/http"

	"fmt"
	"go-server/internal/models"
	"go-server/internal/repository"
	"go-server/internal/service"
	"go-server/pkg/utils"
)

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
const (
	// maxSocietyCodeRetries is the number of times Create will regenerate a
	// society code and retry on a unique-constraint collision before giving up.
	maxSocietyCodeRetries = 5

	// superAdminRole is the role assigned to the society creator.
	superAdminRole = "admin"

	// trialPlanName is the name of the trial plan used for new societies.
	trialPlanName = "Trial"
)

// ==================== INTERFACE ====================

type SocietyService interface {
	// Create a new society (tenant).
	//
	// Steps (all inside one DB transaction):
	//  1. Generate a unique society code (retry on collision).
	//  2. Insert the society record.
	//  3. Assign the creator as SuperAdmin (update user.society_id + role).
	//  4. Trigger trial subscription via SubscriptionService.Subscribe.
	Create(ctx context.Context, req *models.CreateSocietyRequest) (*models.Society, error)

	// GetByID fetches a society by primary key.
	// Used internally by other services and admin panels.
	GetByID(ctx context.Context, id int64) (*models.Society, error)

	// GetByCode fetches a society by its unique public code.
	// For future use: QR entry, appointment booking, public onboarding.
	GetByCode(ctx context.Context, code string) (*models.Society, error)

	// List returns societies matching the given filter (system Super Admin only).
	// Supports filtering by active/inactive, city, state, pin_code, and deleted.
	List(ctx context.Context, filter models.SocietyFilter) ([]*models.Society, error)

	// Update applies profile-related changes only.
	// Cannot modify: subscription data, plan data, society code, or creator.
	Update(ctx context.Context, id int64, req *models.UpdateSocietyRequest) (*models.Society, error)

	// Activate allows login + feature access.
	// Called after payment success or admin approval.
	Activate(ctx context.Context, id int64) error

	// Deactivate blocks all login and API access.
	// Used when subscription expired, payment failed, or policy violation.
	Deactivate(ctx context.Context, id int64) error

	// Delete soft-deletes the society — sets deleted_at and is_active = false.
	// Data is retained for audit, billing history, and legal compliance.
	Delete(ctx context.Context, id int64) error
}

// ==================== IMPLEMENTATION ====================

type societyService struct {
	societyRepo repository.SocietyRepository
	userRepo    repository.UserRepository
	subSvc      SubscriptionService
	planRepo    repository.PlanRepository
	txManager        repository.TransactionManager
}

// NewSocietyService constructs the service with all required dependencies.
//
// db is injected for transaction management — Create runs society insert +
// user update atomically.
// subSvc is injected to trigger the trial subscription after the transaction
// commits (subscriptions have their own transaction in ChangePlan; here we
// call Subscribe which is a simple single-insert, no nested tx needed).
// planRepo is injected to look up the trial plan ID without depending on PlanService.
func NewSocietyService(
	societyRepo repository.SocietyRepository,
	userRepo repository.UserRepository,
	subSvc SubscriptionService,
	planRepo repository.PlanRepository,
	txManager  repository.TransactionManager,
) SocietyService {
	return &societyService{
		societyRepo: societyRepo,
		userRepo:    userRepo,
		subSvc:      subSvc,
		planRepo:    planRepo,
		txManager:        txManager,
	}
}

// ==================== METHODS ====================

// Create registers a new society and wires up the creator and trial subscription.
//
// Transaction covers steps 1–3 (society insert + user assignment).
// Step 4 (Subscribe) runs after the transaction commits — Subscribe's own
// single INSERT is safe outside the outer tx because the society row is
// already visible and the subscription has no shared invariant with the user
// assignment that requires joint atomicity.
//
// If the trial plan does not exist, the society is still created but no
// subscription is started. The caller receives the society back with no
// error; the subscription can be created manually by an admin.
func (s *societyService) Create(ctx context.Context, req *models.CreateSocietyRequest) (*models.Society, error) {
	ctx, cancel := context.WithTimeout(ctx, service.DefaultTimeout)
	defer cancel()

	// Build the society struct before the retry loop so only SocietyCode changes
	// between attempts — all other fields are stable.
	society := &models.Society{
		Name:      req.Name,
		Address:   req.Address,
		City:      req.City,
		State:     req.State,
		PinCode:   req.PinCode,
		CreatorID: req.CreatorID,
		IsActive:  true,
	}

	var created *models.Society

	// Retry loop for society code collisions.
	var lastCodeErr error
	for range maxSocietyCodeRetries {
		society.SocietyCode = utils.GenerateSocietyCode(req.Name, req.City, req.State, req.PinCode)

		// ── Transaction: insert society + assign creator ──────────────────────
		txErr := s.txManager.WithTransaction(ctx, func(txCtx context.Context) error {
			var err error

			// Step 1: Insert society.
			created, err = s.societyRepo.CreateSociety(txCtx, society)
			if err != nil {
				return err
			}

			// Step 2: Assign the creator as SuperAdmin of this society.
			// Sets society_id and role on the user row inside the same tx.
			if err = s.userRepo.UpdateSocietyID(txCtx, req.CreatorID, created.Id, superAdminRole); err != nil {
				return fmt.Errorf("failed to assign creator as super_admin: %w", err)
			}

			return nil
		})
		// ─────────────────────────────────────────────────────────────────────

		if txErr == nil {
			break // transaction succeeded
		}

		if errors.Is(txErr, repository.ErrSocietyCodeConflict) {
			lastCodeErr = txErr
			created = nil
			continue // retry with a new code
		}

		// Any other error (name+city conflict, DB error, user not found) is fatal.
		return nil, mapRepoError(txErr, "failed to create society")
	}

	if created == nil {
		// All retries exhausted — unique code could not be generated.
		return nil, models.NewAppError(
			models.ErrCodeDatabaseError,
			"failed to generate a unique society code, please try again",
			http.StatusInternalServerError,
			lastCodeErr,
		)
	}

	// Step 3 (outside tx): Trigger trial subscription.
	// Failure here is non-fatal — the society is created and the admin can
	// subscribe manually. We do not roll back the society for a missing trial plan.
	s.startTrialSubscription(ctx, created.Id)

	return created, nil
}

// GetByID returns the society with the given ID.
func (s *societyService) GetByID(ctx context.Context, id int64) (*models.Society, error) {
	ctx, cancel := context.WithTimeout(ctx, service.DefaultTimeout)
	defer cancel()

	society, err := s.societyRepo.GetSocietyByID(ctx, id)
	if err != nil {
		return nil, mapRepoError(err, "failed to get society")
	}
	return society, nil
}

// GetByCode returns the society with the given public code.
func (s *societyService) GetByCode(ctx context.Context, code string) (*models.Society, error) {
	ctx, cancel := context.WithTimeout(ctx, service.DefaultTimeout)
	defer cancel()

	society, err := s.societyRepo.GetSocietyByCode(ctx, code)
	if err != nil {
		return nil, mapRepoError(err, "failed to get society")
	}
	return society, nil
}

// List returns societies matching the given filter.
func (s *societyService) List(ctx context.Context, filter models.SocietyFilter) ([]*models.Society, error) {
	ctx, cancel := context.WithTimeout(ctx, service.DefaultTimeout)
	defer cancel()

	societies, err := s.societyRepo.ListSocieties(ctx, &filter)
	if err != nil {
		return nil, mapRepoError(err, "failed to list societies")
	}
	return societies, nil
}

// Update applies profile-related changes only.
// subscription data, plan data, society_code, creator_id, and is_active are
// all immutable through this method.
func (s *societyService) Update(ctx context.Context, id int64, req *models.UpdateSocietyRequest) (*models.Society, error) {
	ctx, cancel := context.WithTimeout(ctx, service.DefaultTimeout)
	defer cancel()

	updated, err := s.societyRepo.UpdateSociety(ctx, id, req)
	if err != nil {
		return nil, mapRepoError(err, "failed to update society")
	}
	return updated, nil
}

// Activate sets is_active = true, allowing login and feature access.
func (s *societyService) Activate(ctx context.Context, id int64) error {
	ctx, cancel := context.WithTimeout(ctx, service.DefaultTimeout)
	defer cancel()

	if _, err := s.societyRepo.SetSocietyActiveStatus(ctx, id, true); err != nil {
		return mapRepoError(err, "failed to activate society")
	}
	return nil
}

// Deactivate sets is_active = false, blocking all login and API access.
func (s *societyService) Deactivate(ctx context.Context, id int64) error {
	ctx, cancel := context.WithTimeout(ctx, service.DefaultTimeout)
	defer cancel()

	if _, err := s.societyRepo.SetSocietyActiveStatus(ctx, id, false); err != nil {
		return mapRepoError(err, "failed to deactivate society")
	}
	return nil
}

// Delete soft-deletes the society and immediately sets is_active = false.
// Data is retained for audit, billing history, and legal compliance.
func (s *societyService) Delete(ctx context.Context, id int64) error {
	if err := validateSocietyServiceID(id); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, service.DefaultTimeout)
	defer cancel()

	if err := s.societyRepo.DeleteSociety(ctx, id); err != nil {
		return mapRepoError(err, "failed to delete society")
	}
	return nil
}

// ==================== HELPERS ====================

// startTrialSubscription looks up the Trial plan and subscribes the new society.
// It is intentionally non-fatal — if the trial plan doesn't exist or Subscribe
// fails, the error is swallowed and the society creation still succeeds.
// Admins can assign a subscription manually via the dashboard.
func (s *societyService) startTrialSubscription(ctx context.Context, societyID int64) {
	plan, err := s.planRepo.GetPlanByName(ctx, trialPlanName)
	if err != nil || !plan.IsActive {
		// Trial plan not seeded yet or deactivated — skip silently.
		return
	}

	// Subscribe returns ErrActiveSubscriptionExists on duplicate — safe to ignore here
	// since the society was just created and cannot already have a subscription.
	_, _ = s.subSvc.Subscribe(ctx, societyID, plan.Id)
}

// validateSocietyServiceID fast-fails at the service boundary before any I/O.
func validateSocietyServiceID(id int64) error {
	if id <= 0 {
		return models.NewAppError(
			models.ErrCodeBadRequest,
			"invalid society ID",
			http.StatusBadRequest,
			nil,
		)
	}
	return nil
}

// mapRepoError passes well-formed AppErrors through unchanged and wraps
// anything else as a generic 500.
func mapRepoError(err error, fallbackMsg string) error {
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