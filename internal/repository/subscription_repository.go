package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"go-server/internal/models"
	"go-server/pkg/database"
)

// ==================== SENTINEL ERRORS ====================

var (
	ErrSubscriptionNotFound = models.NewAppError(
		models.ErrCodeNotFound,
		"subscription not found",
		http.StatusNotFound,
		nil,
	)

	ErrInvalidSubscriptionID = models.NewAppError(
		models.ErrCodeBadRequest,
		"invalid subscription ID",
		http.StatusBadRequest,
		nil,
	)

	ErrInvalidSocietyIDSub = models.NewAppError(
		models.ErrCodeBadRequest,
		"invalid society ID",
		http.StatusBadRequest,
		nil,
	)

	// ErrActiveSubscriptionExists is returned when Subscribe is called but an
	// active/cancel_pending subscription already exists. The caller must use
	// ChangePlan to replace it.
	ErrActiveSubscriptionExists = models.NewAppError(
		models.ErrCodeConflict,
		"an active subscription already exists for this society",
		http.StatusConflict,
		nil,
	)

	// ErrNoActiveSubscription is returned when an operation requires a live
	// subscription but none is found (e.g. Renew, Cancel, ChangePlan).
	ErrNoActiveSubscription = models.NewAppError(
		models.ErrCodeNotFound,
		"no active subscription found for this society",
		http.StatusNotFound,
		nil,
	)

	// ErrSubscriptionAlreadyCancelled is returned when Cancel is called on a
	// subscription that is already in a terminal state.
	ErrSubscriptionAlreadyCancelled = models.NewAppError(
		models.ErrCodeConflict,
		"subscription is already cancelled",
		http.StatusConflict,
		nil,
	)
)

// ==================== INTERFACE ====================

// SubscriptionRepository defines pure single-SQL data operations.
//
// Atomicity rule: every method here executes exactly one SQL statement.
// Multi-step atomicity (e.g. close + create) is the service layer's
// responsibility — it starts a transaction, injects a txCtx, and calls
// these methods in sequence. GetExecutor picks up the transaction
// automatically when a txCtx is present.
type SubscriptionRepository interface {
	// CreateSubscription inserts a new subscription row and returns the full record.
	// When called with a transactional context it participates in that transaction.
	CreateSubscription(ctx context.Context, sub *models.Subscription) (*models.Subscription, error)

	// GetByID returns a single subscription by primary key.
	GetByID(ctx context.Context, id int64) (*models.Subscription, error)

	// GetActiveBySocietyID returns the single active or cancel_pending subscription
	// for a society. Returns ErrNoActiveSubscription if none exists.
	GetActiveBySocietyID(ctx context.Context, societyID int64) (*models.Subscription, error)

	// ListBySocietyID returns all subscriptions for a society, newest first.
	// Includes all statuses — used for billing history and audit logs.
	ListBySocietyID(ctx context.Context, societyID int64) ([]*models.Subscription, error)

	// UpdateStatus sets the status (and optionally cancelled_at) for a subscription.
	// Used by Cancel and the expiry background job.
	UpdateStatus(ctx context.Context, id int64, status models.SubscriptionStatus) (*models.Subscription, error)

	// CloseSubscription immediately cancels the subscription identified by id.
	// Only affects rows with status in {active, cancel_pending}.
	// When called with a transactional context it participates in that transaction.
	// Returns ErrNoActiveSubscription if no matching active row is found.
	CloseSubscription(ctx context.Context, id int64) error

	// RenewSubscription sets status = active and advances end_date to newEndDate.
	// The WHERE guard prevents renewing an already-cancelled subscription.
	RenewSubscription(ctx context.Context, id int64, newEndDate time.Time) (*models.Subscription, error)
}

// ==================== IMPLEMENTATION ====================

type subscriptionRepository struct {
	db *database.Database
}

func NewSubscriptionRepository(db *database.Database) SubscriptionRepository {
	return &subscriptionRepository{db: db}
}

// ==================== QUERIES ====================

const (
	// subSelectColumns is the canonical column list for subscriptions.
	// Column order must match scanSubscription exactly.
	subSelectColumns = `
		id, society_id, plan_id, status, is_trial,
		start_date, end_date, cancelled_at,
		snapshot_price, snapshot_billing_cycle,
		snapshot_max_flats, snapshot_max_staff, snapshot_max_admins,
		created_at, updated_at`

	createSubscriptionQuery = `
		INSERT INTO subscriptions (
			society_id, plan_id, status, is_trial,
			start_date, end_date,
			snapshot_price, snapshot_billing_cycle,
			snapshot_max_flats, snapshot_max_staff, snapshot_max_admins
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING ` + subSelectColumns

	getSubByIDQuery = `
		SELECT ` + subSelectColumns + `
		FROM subscriptions
		WHERE id = $1`

	// getActiveBySOcietyIDQuery checks both status values that grant access.
	// A cancel_pending subscription is still live until end_date.
	getActiveBySOcietyIDQuery = `
		SELECT ` + subSelectColumns + `
		FROM subscriptions
		WHERE  society_id = $1
		AND    status     IN ('active', 'cancel_pending')
		AND    end_date   > NOW()
		LIMIT  1`

	listBySocietyIDQuery = `
		SELECT ` + subSelectColumns + `
		FROM subscriptions
		WHERE  society_id = $1
		ORDER  BY created_at DESC`

	// updateStatusQuery stamps cancelled_at automatically when transitioning to
	// a cancellation state, so callers do not need to pass the timestamp.
	updateStatusQuery = `
		UPDATE subscriptions
		SET    status       = $1,
		       cancelled_at = CASE
		                        WHEN $1 IN ('cancelled', 'cancel_pending') THEN NOW()
		                        ELSE cancelled_at
		                      END,
		       updated_at   = NOW()
		WHERE  id           = $2
		RETURNING ` + subSelectColumns

	// closeSubscriptionQuery terminates an active subscription.
	// RowsAffected == 0 when the row is already in a terminal state.
	closeSubscriptionQuery = `
		UPDATE subscriptions
		SET    status       = 'cancelled',
		       cancelled_at = NOW(),
		       updated_at   = NOW()
		WHERE  id           = $1
		AND    status       IN ('active', 'cancel_pending')`

	// renewSubscriptionQuery reactivates and advances end_date.
	// Cancelled subscriptions cannot be renewed.
	renewSubscriptionQuery = `
		UPDATE subscriptions
		SET    status     = 'active',
		       end_date   = $1,
		       updated_at = NOW()
		WHERE  id         = $2
		AND    status     NOT IN ('cancelled')
		RETURNING ` + subSelectColumns
)

// ==================== METHODS ====================

func (r *subscriptionRepository) CreateSubscription(ctx context.Context, sub *models.Subscription) (*models.Subscription, error) {
	executor := GetExecutor(ctx, r.db)

	row := executor.QueryRow(ctx, createSubscriptionQuery,
		sub.SocietyID,
		sub.PlanID,
		sub.Status,
		sub.IsTrial,
		sub.StartDate,
		sub.EndDate,
		sub.SnapshotPrice,
		sub.SnapshotBillingCycle,
		sub.SnapshotMaxFlats,
		sub.SnapshotMaxStaff,
		sub.SnapshotMaxAdmins,
	)

	created, err := scanSubscription(row)
	if err != nil {
		return nil, fmt.Errorf("repository: failed to create subscription: %w", err)
	}
	return created, nil
}

func (r *subscriptionRepository) GetByID(ctx context.Context, id int64) (*models.Subscription, error) {
	if err := validateSubscriptionID(id); err != nil {
		return nil, err
	}

	executor := GetExecutor(ctx, r.db)
	row := executor.QueryRow(ctx, getSubByIDQuery, id)

	return scanSubscription(row)
}

func (r *subscriptionRepository) GetActiveBySocietyID(ctx context.Context, societyID int64) (*models.Subscription, error) {
	if err := validateSubSocietyID(societyID); err != nil {
		return nil, err
	}

	executor := GetExecutor(ctx, r.db)
	row := executor.QueryRow(ctx, getActiveBySOcietyIDQuery, societyID)

	sub, err := scanSubscription(row)
	if err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			return nil, ErrNoActiveSubscription
		}
		return nil, err
	}
	return sub, nil
}

func (r *subscriptionRepository) ListBySocietyID(ctx context.Context, societyID int64) ([]*models.Subscription, error) {
	if err := validateSubSocietyID(societyID); err != nil {
		return nil, err
	}

	executor := GetExecutor(ctx, r.db)
	rows, err := executor.Query(ctx, listBySocietyIDQuery, societyID)
	if err != nil {
		return nil, fmt.Errorf("repository: failed to list subscriptions: %w", err)
	}
	defer rows.Close()

	return scanSubscriptionRows(rows)
}

func (r *subscriptionRepository) UpdateStatus(ctx context.Context, id int64, status models.SubscriptionStatus) (*models.Subscription, error) {
	if err := validateSubscriptionID(id); err != nil {
		return nil, err
	}

	executor := GetExecutor(ctx, r.db)
	row := executor.QueryRow(ctx, updateStatusQuery, string(status), id)

	sub, err := scanSubscription(row)
	if err != nil {
		return nil, fmt.Errorf("repository: failed to update subscription status: %w", err)
	}
	return sub, nil
}

// CloseSubscription cancels the subscription identified by id.
// Only rows with status in {active, cancel_pending} are affected.
// When the caller holds a transaction in ctx, this participates automatically.
// Returns ErrNoActiveSubscription if no matching row was updated.
func (r *subscriptionRepository) CloseSubscription(ctx context.Context, id int64) error {
	if err := validateSubscriptionID(id); err != nil {
		return err
	}

	executor := GetExecutor(ctx, r.db)
	result, err := executor.Exec(ctx, closeSubscriptionQuery, id)
	if err != nil {
		return fmt.Errorf("repository: failed to close subscription %d: %w", id, err)
	}

	if result.RowsAffected() == 0 {
		return ErrNoActiveSubscription
	}
	return nil
}

// RenewSubscription sets status = active and advances end_date to newEndDate.
// Returns ErrSubscriptionNotFound when the id doesn't exist or is already cancelled.
func (r *subscriptionRepository) RenewSubscription(ctx context.Context, id int64, newEndDate time.Time) (*models.Subscription, error) {
	if err := validateSubscriptionID(id); err != nil {
		return nil, err
	}

	executor := GetExecutor(ctx, r.db)
	row := executor.QueryRow(ctx, renewSubscriptionQuery, newEndDate, id)

	sub, err := scanSubscription(row)
	if err != nil {
		return nil, fmt.Errorf("repository: failed to renew subscription: %w", err)
	}
	return sub, nil
}

// ==================== HELPERS ====================

// scanSubscription scans one row into a Subscription.
// Column order must match subSelectColumns exactly.
func scanSubscription(row interface{ Scan(dest ...any) error }) (*models.Subscription, error) {
	var s models.Subscription
	err := row.Scan(
		&s.Id,
		&s.SocietyID,
		&s.PlanID,
		&s.Status,
		&s.IsTrial,
		&s.StartDate,
		&s.EndDate,
		&s.CancelledAt,
		&s.SnapshotPrice,
		&s.SnapshotBillingCycle,
		&s.SnapshotMaxFlats,
		&s.SnapshotMaxStaff,
		&s.SnapshotMaxAdmins,
		&s.CreatedAt,
		&s.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrSubscriptionNotFound
		}
		return nil, fmt.Errorf("repository: failed to scan subscription: %w", err)
	}
	return &s, nil
}

// scanSubscriptionRows iterates a Rows result and scans each into a Subscription.
// Always returns an empty slice (never nil) when no rows exist.
func scanSubscriptionRows(rows interface {
	Next() bool
	Err() error
	Scan(dest ...any) error
}) ([]*models.Subscription, error) {
	var subs []*models.Subscription
	for rows.Next() {
		s, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		subs = append(subs, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repository: error iterating subscription rows: %w", err)
	}
	if subs == nil {
		subs = []*models.Subscription{}
	}
	return subs, nil
}

// validateSubscriptionID returns an error if the ID is not positive.
func validateSubscriptionID(id int64) error {
	if id <= 0 {
		return ErrInvalidSubscriptionID
	}
	return nil
}

// validateSubSocietyID returns an error if the society ID is not positive.
func validateSubSocietyID(id int64) error {
	if id <= 0 {
		return ErrInvalidSocietyIDSub
	}
	return nil
}