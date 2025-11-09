package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"go-server/internal/models"
	"net/http"

	"go-server/pkg/database"
	"strings"
	"time"
)

// ==================== SENTINEL ERRORS ====================
var (
	// ErrUserNotFound indicates that a user was not found in the database
	ErrUserNotFound = models.NewAppError(
		models.ErrCodeNotFound,
		"User not found",
		http.StatusNotFound,
		nil,
	)
	// ErrInvalidUserID indicates that the provided user ID is invalid
	ErrInvalidUserID = models.NewAppError(
		models.ErrCodeBadRequest,
		"Invalid user ID",
		http.StatusBadRequest,
		nil,
	)
)

// ==================== INTERFACE ====================

type UserRepository interface {
	// Core CRUD operations
	Create(ctx context.Context, user *models.User) error
	GetByID(ctx context.Context, id int64) (*models.User, error)
	GetByEmail(ctx context.Context, email string) (*models.User, error)
	GetByPhoneNumber(ctx context.Context, phoneNumber string) (*models.User, error)
	Update(ctx context.Context, user *models.User) error
	Delete(ctx context.Context, id int64) error

	// Authentication & verification
	UpdateLastLogin(ctx context.Context, id int64) error
	UpdatePassword(ctx context.Context, id int64, passwordHash string) error
	UpdateIsVerified(ctx context.Context, userID int64, isVerified bool) error

	// Existence checks
	EmailExists(ctx context.Context, email string) (bool, error)
	PhoneExists(ctx context.Context, phoneNumber string) (bool, error)

	// Maintenance operations
	DeleteExpiredUnverified(ctx context.Context) (int64, error)
}

// ==================== IMPLEMENTATION ====================

type userRepository struct {
	db *database.Database
}

// ==================== CONSTRUCTOR ====================

func NewUserRepository(db *database.Database) UserRepository {
	return &userRepository{db: db}
}

// ==================== PRIVATE HELPERS ====================

// scanUser scans a database row into a User model
func scanUser(row interface{ Scan(dest ...any) error }) (*models.User, error) {
	var user models.User
	err := row.Scan(
		&user.ID,
		&user.Name,
		&user.Email,
		&user.PhoneNumber,
		&user.PasswordHash,
		&user.Role,
		&user.IsVerified,
		&user.LastLogin,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to scan user row: %w", err)
	}
	return &user, nil
}

// validateUserID validates that user ID is positive
func validateUserID(id int64) error {
	if id <= 0 {
		return ErrInvalidUserID
	}
	return nil
}

// ==================== CRUD OPERATIONS ====================

// Create inserts a new user into the database
// Uses transaction if available in context
func (r *userRepository) Create(ctx context.Context, user *models.User) error {
	query := `
		INSERT INTO users (
			name, email, phone_number, password_hash, role
		)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id,created_at, updated_at
	`

	executor := GetExecutor(ctx, r.db)
	err := executor.QueryRow(ctx, query,
		user.Name,
		user.Email,
		user.PhoneNumber,
		user.PasswordHash,
		user.Role,
	).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)

	if err != nil {
		return err
	}

	return nil
}

// GetByID retrieves a user by their ID
func (r *userRepository) GetByID(ctx context.Context, id int64) (*models.User, error) {
	if err := validateUserID(id); err != nil {
		return nil, err
	}

	query := `
		SELECT 
			id, name, email, phone_number, password_hash, role, 
			is_verified, last_login, created_at, updated_at
		FROM users
		WHERE id = $1
	`

	executor := GetExecutor(ctx, r.db)
	user, err := scanUser(executor.QueryRow(ctx, query, id))
	if err != nil {
		return nil, err
	}

	return user, nil
}

func (r *userRepository) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	query := `
		SELECT 
			id, name, email, phone_number, password_hash, role, 
			 is_verified,  last_login, created_at, updated_at
		FROM users
		WHERE LOWER(email) = $1
	`

	executor := GetExecutor(ctx, r.db)
	user, err := scanUser(executor.QueryRow(ctx, query, strings.ToLower(email)))
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (r *userRepository) GetByPhoneNumber(ctx context.Context, phoneNumber string) (*models.User, error) {
	query := `
		SELECT 
			id, name, email, phone_number, password_hash, role, 
			is_verified, 
			last_login, created_at, updated_at
		FROM users
		WHERE phone_number = $1
	`

	executor := GetExecutor(ctx, r.db)
	user, err := scanUser(executor.QueryRow(ctx, query, phoneNumber))
	if err != nil {

		return nil, err
	}
	return user, nil
}

// Update modifies an existing user's information
// Only updates name and phone_number fields
func (r *userRepository) Update(ctx context.Context, user *models.User) error {

	query := `
		UPDATE users
		SET 
			name = $1, 
			phone_number = $2, 
			updated_at = $3
		WHERE id = $4
		RETURNING updated_at
	`

	executor := GetExecutor(ctx, r.db)
	err := executor.QueryRow(ctx, query,
		user.Name,
		user.PhoneNumber,
		user.UpdatedAt,
		user.ID,
	).Scan(&user.UpdatedAt)

	if err != nil {

		return err
	}

	return nil
}

// Delete removes a user from the database
func (r *userRepository) Delete(ctx context.Context, id int64) error {
	if err := validateUserID(id); err != nil {
		return err
	}

	query := `DELETE FROM users WHERE id = $1`

	executor := GetExecutor(ctx, r.db)
	result, err := executor.Exec(ctx, query, id)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	return nil
}

// ==================== AUTHENTICATION & VERIFICATION ====================

// UpdateLastLogin updates the user's last login timestamp
func (r *userRepository) UpdateLastLogin(ctx context.Context, id int64) error {
	if err := validateUserID(id); err != nil {
		return err
	}

	query := `
		UPDATE users 
		SET last_login = $1 
		WHERE id = $2
	`

	executor := GetExecutor(ctx, r.db)
	result, err := executor.Exec(ctx, query, time.Now(), id)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	return nil
}

// UpdatePassword updates a user's password hash
func (r *userRepository) UpdatePassword(ctx context.Context, id int64, passwordHash string) error {
	if err := validateUserID(id); err != nil {
		return err
	}

	if passwordHash == "" {
		return fmt.Errorf("password hash cannot be empty")
	}

	query := `
		UPDATE users 
		SET 
			password_hash = $1, 
			updated_at = $2 
		WHERE id = $3
	`

	executor := GetExecutor(ctx, r.db)
	result, err := executor.Exec(ctx, query, passwordHash, time.Now(), id)
	if err != nil {
		return fmt.Errorf("update password for user %d: %w", id, err)
	}

	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	return nil
}

// UpdateIsVerified updates the user's verification status
func (r *userRepository) UpdateIsVerified(ctx context.Context, userID int64, isVerified bool) error {
	if err := validateUserID(userID); err != nil {
		return err
	}

	query := `
		UPDATE users 
		SET 
			is_verified = $1, 
			updated_at = CURRENT_TIMESTAMP 
		WHERE id = $2
	`

	executor := GetExecutor(ctx, r.db)
	result, err := executor.Exec(ctx, query, isVerified, userID)
	if err != nil {
		return fmt.Errorf("update verification status for user %d: %w", userID, err)
	}

	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	return nil
}

// ==================== EXISTENCE CHECKS ====================

// EmailExists checks if an email is already registered
// Returns true if email exists, false otherwise
func (r *userRepository) EmailExists(ctx context.Context, email string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM users WHERE LOWER(email) = LOWER($1))`
	executor := GetExecutor(ctx, r.db)
	var exists bool
	if err := executor.QueryRow(ctx, query, email).Scan(&exists); err != nil {
		return false, fmt.Errorf("check email exists: %w", err)
	}
	return exists, nil
}

func (r *userRepository) PhoneExists(ctx context.Context, phoneNumber string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM users WHERE phone_number = $1)`
	executor := GetExecutor(ctx, r.db)
	var exists bool
	if err := executor.QueryRow(ctx, query, phoneNumber).Scan(&exists); err != nil {
		return false, fmt.Errorf("check phone exists: %w", err)
	}
	return exists, nil
}

// ==================== MAINTENANCE OPERATIONS ====================

// DeleteExpiredUnverified removes unverified users that have expired
// Returns the number of users deleted
// A user is considered expired if:
// - They are not verified
// - Created more than 24 hours ago
// - Either have no verification record or all verification records are expired
func (r *userRepository) DeleteExpiredUnverified(ctx context.Context) (int64, error) {
	query := `
		DELETE FROM users 
		WHERE 
			is_verified = FALSE 
			AND created_at < NOW() - INTERVAL '24 hours'
			AND (
				NOT EXISTS (
					SELECT 1 
					FROM user_verifications 
					WHERE user_verifications.user_id = users.id
				)
				OR NOT EXISTS (
					SELECT 1 
					FROM user_verifications 
					WHERE 
						user_verifications.user_id = users.id 
						AND user_verifications.expires_at > NOW()
				)
			)
	`

	executor := GetExecutor(ctx, r.db)
	commandTag, err := executor.Exec(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("delete expired unverified users: %w", err)
	}

	rowsAffected := commandTag.RowsAffected()
	return rowsAffected, nil
}
