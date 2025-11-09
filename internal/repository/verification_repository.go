package repository

import (
	"context"
	"errors"
	"go-server/internal/models"
	"go-server/pkg/database"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

// Repository-level sentinel errors
var (
	ErrNotFound = models.NewAppError(
		models.ErrCodeNotFound,
		"record not found",
		http.StatusNotFound,
		nil,
	)
	ErrOTPExpired = models.NewAppError(
		models.ErrCodeOTPExpired,
		"otp expired",
		http.StatusBadRequest,
		nil,
	)
)

type VerificationRepository interface {
	CreateVerification(ctx context.Context, verification *models.UserVerification) error
	GetActiveVerification(ctx context.Context, userID int64, otp string) (*models.UserVerification, error)
	MarkAsUsed(ctx context.Context, verificationID int64) error
	DeleteExpiredVerifications(ctx context.Context) error
	DeleteUserVerifications(ctx context.Context, userID int64) error
}

type verificationRepository struct {
	db *database.Database
}

func NewVerificationRepository(db *database.Database) VerificationRepository {
	return &verificationRepository{db: db}
}

// ✅ CreateVerification inserts a new verification record after clearing old ones
func (r *verificationRepository) CreateVerification(ctx context.Context, verification *models.UserVerification) error {
	executor := GetExecutor(ctx, r.db)

	// Delete any unused verifications for this user
	deleteQuery := `DELETE FROM user_verifications WHERE user_id = $1 AND is_used = FALSE`
	if _, err := executor.Exec(ctx, deleteQuery, verification.UserID); err != nil {
		return models.NewAppError(models.ErrCodeDatabaseError, "failed to delete old OTP records", http.StatusInternalServerError, err)
	}

	// Insert new verification record
	query := `
		INSERT INTO user_verifications (user_id, otp, expires_at)
		VALUES ($1, $2, $3)
		RETURNING id, created_at
	`
	err := executor.QueryRow(ctx, query, verification.UserID, verification.OTP, verification.ExpiresAt).
		Scan(&verification.ID, &verification.CreatedAt)
	if err != nil {
		return models.NewAppError(models.ErrCodeDatabaseError, "failed to create verification record", http.StatusInternalServerError, err)
	}

	return nil
}

// ✅ GetActiveVerification retrieves the latest active OTP for a user
func (r *verificationRepository) GetActiveVerification(ctx context.Context, userID int64, otp string) (*models.UserVerification, error) {
	query := `
		SELECT id, user_id, otp, is_used, expires_at, created_at
		FROM user_verifications
		WHERE user_id = $1 
		AND otp = $2
		AND is_used = FALSE
		AND expires_at > NOW()
		ORDER BY created_at DESC
		LIMIT 1
	`

	var verification models.UserVerification
	executor := GetExecutor(ctx, r.db)
	err := executor.QueryRow(ctx, query, userID, otp).Scan(
		&verification.ID,
		&verification.UserID,
		&verification.OTP,
		&verification.IsUsed,
		&verification.ExpiresAt,
		&verification.CreatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, models.NewAppError(models.ErrCodeNotFound, "no active verification found", http.StatusNotFound, err)
		}
		return nil, models.NewAppError(models.ErrCodeDatabaseError, "failed to query verification record", http.StatusInternalServerError, err)
	}

	// Double-check expiry (extra safety)
	if time.Now().After(verification.ExpiresAt) {
		return nil, models.NewAppError(models.ErrCodeOTPExpired, "OTP has expired", http.StatusBadRequest, ErrOTPExpired)
	}

	return &verification, nil
}

// ✅ MarkAsUsed marks a verification record as used
func (r *verificationRepository) MarkAsUsed(ctx context.Context, verificationID int64) error {
	query := `UPDATE user_verifications SET is_used = TRUE WHERE id = $1`
	executor := GetExecutor(ctx, r.db)
	cmdTag, err := executor.Exec(ctx, query, verificationID)
	if err != nil {
		return models.NewAppError(models.ErrCodeDatabaseError, "failed to mark verification as used", http.StatusInternalServerError, err)
	}

	if cmdTag.RowsAffected() == 0 {
		return models.NewAppError(models.ErrCodeNotFound, "verification not found to mark as used", http.StatusNotFound, ErrNotFound)
	}

	return nil
}

// ✅ DeleteExpiredVerifications cleans up old OTPs
func (r *verificationRepository) DeleteExpiredVerifications(ctx context.Context) error {
	query := `DELETE FROM user_verifications WHERE expires_at < NOW()`
	executor := GetExecutor(ctx, r.db)
	if _, err := executor.Exec(ctx, query); err != nil {
		return models.NewAppError(models.ErrCodeDatabaseError, "failed to delete expired verifications", http.StatusInternalServerError, err)
	}
	return nil
}

// ✅ DeleteUserVerifications removes all verifications for a given user
func (r *verificationRepository) DeleteUserVerifications(ctx context.Context, userID int64) error {
	query := `DELETE FROM user_verifications WHERE user_id = $1`
	executor := GetExecutor(ctx, r.db)
	if _, err := executor.Exec(ctx, query, userID); err != nil {
		return models.NewAppError(models.ErrCodeDatabaseError, "failed to delete user verifications", http.StatusInternalServerError, err)
	}
	return nil
}
