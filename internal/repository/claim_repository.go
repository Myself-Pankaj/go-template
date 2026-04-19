package repository

import (
	"context"
	"errors"
	"fmt"

	"go-server/internal/models"
	"go-server/pkg/database"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ==================== SENTINEL ERRORS ====================

var (
	ErrClaimNotFound      = errors.New("claim request not found")
	ErrClaimAlreadyExists = errors.New("a pending claim already exists for this user and flat")
	ErrClaimNotPending    = errors.New("claim is not in pending status")
)

// ==================== INTERFACE ====================

type ClaimRepository interface {
	// Write
	CreateClaim(ctx context.Context, claim *models.FlatClaimRequest) (*models.FlatClaimRequest, error)
	ApproveClaim(ctx context.Context, claimID int64, reviewerID int64) (*models.FlatClaimRequest, error)
	RejectClaim(ctx context.Context, claimID int64, reviewerID int64, reason *string) (*models.FlatClaimRequest, error)

	// Read
	GetClaimByID(ctx context.Context, id int64) (*models.FlatClaimRequest, error)
	GetPendingClaimsByFlat(ctx context.Context, flatID int64) ([]*models.FlatClaimRequest, error)
	GetClaimsBySociety(ctx context.Context, societyID int64, status *models.ClaimStatus) ([]*models.FlatClaimRequest, error)
	GetClaimsByUser(ctx context.Context, userID int64) ([]*models.FlatClaimRequest, error)
	GetPendingClaimByUserAndFlat(ctx context.Context, userID int64, flatID int64) (*models.FlatClaimRequest, error)
}

// ==================== IMPLEMENTATION ====================

type claimRepository struct {
	db *database.Database
}

func NewClaimRepository(db *database.Database) ClaimRepository {
	return &claimRepository{db: db}
}

// ==================== QUERIES ====================

const claimSelectColumns = `
	id, user_id, flat_id, society_id, status, note,
	reviewed_by, reviewed_at, rejection_reason, created_at, updated_at`

const (
	createClaimQuery = `
		INSERT INTO flat_claim_requests (user_id, flat_id, society_id, note)
		VALUES ($1, $2, $3, $4)
		RETURNING ` + claimSelectColumns

	approveClaimQuery = `
		UPDATE flat_claim_requests
		SET    status      = 'approved',
		       reviewed_by = $1,
		       reviewed_at = NOW()
		WHERE  id          = $2
		  AND  status      = 'pending'
		RETURNING ` + claimSelectColumns

	rejectClaimQuery = `
		UPDATE flat_claim_requests
		SET    status           = 'rejected',
		       reviewed_by      = $1,
		       reviewed_at      = NOW(),
		       rejection_reason = $2
		WHERE  id               = $3
		  AND  status           = 'pending'
		RETURNING ` + claimSelectColumns

	getClaimByIDQuery = `
		SELECT ` + claimSelectColumns + `
		FROM   flat_claim_requests
		WHERE  id = $1`

	getPendingClaimsByFlatQuery = `
		SELECT ` + claimSelectColumns + `
		FROM   flat_claim_requests
		WHERE  flat_id = $1
		  AND  status  = 'pending'
		ORDER  BY created_at ASC`

	getClaimsBySocietyQuery = `
		SELECT ` + claimSelectColumns + `
		FROM   flat_claim_requests
		WHERE  society_id = $1
		ORDER  BY created_at DESC`

	getClaimsBySocietyAndStatusQuery = `
		SELECT ` + claimSelectColumns + `
		FROM   flat_claim_requests
		WHERE  society_id = $1
		  AND  status     = $2
		ORDER  BY created_at DESC`

	getClaimsByUserQuery = `
		SELECT ` + claimSelectColumns + `
		FROM   flat_claim_requests
		WHERE  user_id = $1
		ORDER  BY created_at DESC`

	getPendingClaimByUserAndFlatQuery = `
		SELECT ` + claimSelectColumns + `
		FROM   flat_claim_requests
		WHERE  user_id = $1
		  AND  flat_id = $2
		  AND  status  = 'pending'`
)

// ==================== METHODS ====================

func (r *claimRepository) CreateClaim(ctx context.Context, claim *models.FlatClaimRequest) (*models.FlatClaimRequest, error) {
	executor := GetExecutor(ctx, r.db)
	row := executor.QueryRow(ctx, createClaimQuery,
		claim.UserID,
		claim.FlatID,
		claim.SocietyID,
		claim.Note,
	)
	created, err := scanClaim(row)
	if err != nil {
		return nil, mapClaimWriteError(err, "create")
	}
	return created, nil
}

func (r *claimRepository) ApproveClaim(ctx context.Context, claimID int64, reviewerID int64) (*models.FlatClaimRequest, error) {
	executor := GetExecutor(ctx, r.db)
	row := executor.QueryRow(ctx, approveClaimQuery, reviewerID, claimID)
	updated, err := scanClaim(row)
	if err != nil {
		if errors.Is(err, ErrClaimNotFound) {
			// Row existed but status != 'pending', OR row doesn't exist.
			// Distinguish by doing a quick existence check isn't worth it;
			// surface ErrClaimNotPending as the more likely cause.
			return nil, ErrClaimNotPending
		}
		return nil, fmt.Errorf("repository: failed to approve claim %d: %w", claimID, err)
	}
	return updated, nil
}

func (r *claimRepository) RejectClaim(ctx context.Context, claimID int64, reviewerID int64, reason *string) (*models.FlatClaimRequest, error) {
	executor := GetExecutor(ctx, r.db)
	row := executor.QueryRow(ctx, rejectClaimQuery, reviewerID, reason, claimID)
	updated, err := scanClaim(row)
	if err != nil {
		if errors.Is(err, ErrClaimNotFound) {
			return nil, ErrClaimNotPending
		}
		return nil, fmt.Errorf("repository: failed to reject claim %d: %w", claimID, err)
	}
	return updated, nil
}

func (r *claimRepository) GetClaimByID(ctx context.Context, id int64) (*models.FlatClaimRequest, error) {
	executor := GetExecutor(ctx, r.db)
	row := executor.QueryRow(ctx, getClaimByIDQuery, id)
	claim, err := scanClaim(row)
	if err != nil {
		return nil, fmt.Errorf("repository: failed to get claim by ID: %w", err)
	}
	return claim, nil
}

func (r *claimRepository) GetPendingClaimsByFlat(ctx context.Context, flatID int64) ([]*models.FlatClaimRequest, error) {
	executor := GetExecutor(ctx, r.db)
	rows, err := executor.Query(ctx, getPendingClaimsByFlatQuery, flatID)
	if err != nil {
		return nil, fmt.Errorf("repository: failed to get pending claims by flat: %w", err)
	}
	defer rows.Close()
	return scanClaimRows(rows)
}

func (r *claimRepository) GetClaimsBySociety(ctx context.Context, societyID int64, status *models.ClaimStatus) ([]*models.FlatClaimRequest, error) {
	executor := GetExecutor(ctx, r.db)

	var rows interface {
		Next() bool
		Err() error
		Scan(dest ...any) error
		Close()
	}

	if status == nil {
		r2, err := executor.Query(ctx, getClaimsBySocietyQuery, societyID)
		if err != nil {
			return nil, fmt.Errorf("repository: failed to get claims by society: %w", err)
		}
		rows = r2
	} else {
		r2, err := executor.Query(ctx, getClaimsBySocietyAndStatusQuery, societyID, string(*status))
		if err != nil {
			return nil, fmt.Errorf("repository: failed to get claims by society and status: %w", err)
		}
		rows = r2
	}

	defer rows.Close()
	return scanClaimRows(rows)
}

func (r *claimRepository) GetClaimsByUser(ctx context.Context, userID int64) ([]*models.FlatClaimRequest, error) {
	executor := GetExecutor(ctx, r.db)
	rows, err := executor.Query(ctx, getClaimsByUserQuery, userID)
	if err != nil {
		return nil, fmt.Errorf("repository: failed to get claims by user: %w", err)
	}
	defer rows.Close()
	return scanClaimRows(rows)
}

func (r *claimRepository) GetPendingClaimByUserAndFlat(ctx context.Context, userID int64, flatID int64) (*models.FlatClaimRequest, error) {
	executor := GetExecutor(ctx, r.db)
	row := executor.QueryRow(ctx, getPendingClaimByUserAndFlatQuery, userID, flatID)
	claim, err := scanClaim(row)
	if err != nil {
		return nil, fmt.Errorf("repository: failed to get pending claim by user and flat: %w", err)
	}
	return claim, nil
}

// ==================== HELPERS ====================

func scanClaim(row interface{ Scan(dest ...any) error }) (*models.FlatClaimRequest, error) {
	var c models.FlatClaimRequest
	err := row.Scan(
		&c.ID,
		&c.UserID,
		&c.FlatID,
		&c.SocietyID,
		&c.Status,
		&c.Note,
		&c.ReviewedBy,
		&c.ReviewedAt,
		&c.RejectionReason,
		&c.CreatedAt,
		&c.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrClaimNotFound
		}
		return nil, fmt.Errorf("repository: failed to scan claim: %w", err)
	}
	return &c, nil
}

func scanClaimRows(rows interface {
	Next() bool
	Err() error
	Scan(dest ...any) error
}) ([]*models.FlatClaimRequest, error) {
	var claims []*models.FlatClaimRequest
	for rows.Next() {
		claim, err := scanClaim(rows)
		if err != nil {
			return nil, err
		}
		claims = append(claims, claim)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repository: error iterating claims: %w", err)
	}
	return claims, nil
}

func mapClaimWriteError(err error, op string) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation → pending claim already exists
			return ErrClaimAlreadyExists
		}
	}
	return fmt.Errorf("repository: failed to %s claim: %w", op, err)
}
