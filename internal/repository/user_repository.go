package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go-server/internal/models"
	"go-server/pkg/database"
)

// ==================== SENTINEL ERRORS ====================

var (
	ErrUserNotFound = models.NewAppError(
		models.ErrCodeNotFound,
		"user not found",
		http.StatusNotFound,
		nil,
	)

	ErrInvalidUserID = models.NewAppError(
		models.ErrCodeBadRequest,
		"invalid user ID",
		http.StatusBadRequest,
		nil,
	)
)

// ==================== INTERFACE ====================

type UserRepository interface {
	// Core CRUD
	Create(ctx context.Context, user *models.User) error
	GetByID(ctx context.Context, id int64) (*models.User, error)
	GetByEmail(ctx context.Context, email string) (*models.User, error)
	GetByPhoneNumber(ctx context.Context, phoneNumber string) (*models.User, error)
	GetByIDs(ctx context.Context, ids []int64) ([]*models.User, error)
	Update(ctx context.Context, user *models.User) error
	Delete(ctx context.Context, id int64) error

	// Society assignment — called inside the society-creation transaction.
	// Sets society_id and role on the user row atomically with society insert.
	UpdateSocietyID(ctx context.Context, userID int64, societyID int64, role string) error

	// Flat assignment — called inside the onboarding approval transaction.
	// Sets flat_id and optionally role; used when approving a claim or redeeming an invite.
	UpdateFlatID(ctx context.Context, userID int64, flatID *int64) error
	UpdateFlatIDAndRole(ctx context.Context, userID int64, flatID *int64, role string) error

	// Onboarding helpers
	GetBySocietyAndRole(ctx context.Context, societyID int64, role string) ([]*models.User, error)
	CountBySocietyAndRole(ctx context.Context, societyID int64, role string) (int, error)
	GetByFlatID(ctx context.Context, flatID int64) ([]*models.User, error)
	CountByFlatAndRole(ctx context.Context, flatID int64, role string) (int, error)

	// Authentication & verification
	UpdateLastLogin(ctx context.Context, id int64) error
	UpdatePassword(ctx context.Context, id int64, passwordHash string) error
	UpdateIsVerified(ctx context.Context, userID int64, isVerified bool) error

	// Existence checks (used during registration)
	EmailExists(ctx context.Context, email string) (bool, error)
	PhoneExists(ctx context.Context, phoneNumber string) (bool, error)

	// Maintenance — run by a background job
	DeleteExpiredUnverified(ctx context.Context) (int64, error)
}

// ==================== IMPLEMENTATION ====================

type userRepository struct {
	db *database.Database
}

func NewUserRepository(db *database.Database) UserRepository {
	return &userRepository{db: db}
}

// ==================== QUERIES ====================

// userSelectColumns is the canonical SELECT list for all user queries.
// Column order must match scanUser exactly.
const userSelectColumns = `
	id, name, email, phone_number, password_hash, role,
	society_id, flat_id, is_verified, last_login, created_at, updated_at`

// ==================== HELPERS ====================

// scanUser scans one database row into a User.
// Column order must match userSelectColumns exactly.
func scanUser(row interface{ Scan(dest ...any) error }) (*models.User, error) {
	var u models.User
	err := row.Scan(
		&u.ID,
		&u.Name,
		&u.Email,
		&u.PhoneNumber,
		&u.PasswordHash,
		&u.Role,
		&u.SocietyID, // *int64 — nil when NULL
		&u.FlatID,   // *int64 — nil when NULL
		&u.IsVerified,
		&u.LastLogin, // *time.Time — nil when NULL
		&u.CreatedAt,
		&u.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("repository: failed to scan user: %w", err)
	}
	return &u, nil
}

// validateUserID returns an error if the ID is not positive.
func validateUserID(id int64) error {
	if id <= 0 {
		return ErrInvalidUserID
	}
	return nil
}

// ==================== METHODS ====================

// Create inserts a new user row and populates the generated id, created_at,
// and updated_at back onto the caller's struct.
// When called with a transactional context it participates in that transaction.
func (r *userRepository) Create(ctx context.Context, user *models.User) error {
	query := `
		INSERT INTO users (name, email, phone_number, password_hash, role, is_verified)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at`

	executor := GetExecutor(ctx, r.db)
	return executor.QueryRow(ctx, query,
		user.Name,
		user.Email,
		user.PhoneNumber,
		user.PasswordHash,
		user.Role,
		user.IsVerified,
	).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)
}

func (r *userRepository) GetByID(ctx context.Context, id int64) (*models.User, error) {
	if err := validateUserID(id); err != nil {
		return nil, err
	}

	query := `SELECT ` + userSelectColumns + ` FROM users WHERE id = $1`

	executor := GetExecutor(ctx, r.db)
	return scanUser(executor.QueryRow(ctx, query, id))
}

func (r *userRepository) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	query := `SELECT ` + userSelectColumns + ` FROM users WHERE LOWER(email) = $1`

	executor := GetExecutor(ctx, r.db)
	return scanUser(executor.QueryRow(ctx, query, strings.ToLower(email)))
}

func (r *userRepository) GetByPhoneNumber(ctx context.Context, phoneNumber string) (*models.User, error) {
	query := `SELECT ` + userSelectColumns + ` FROM users WHERE phone_number = $1`

	executor := GetExecutor(ctx, r.db)
	return scanUser(executor.QueryRow(ctx, query, phoneNumber))
}

func (r *userRepository) GetByIDs(ctx context.Context, ids []int64) ([]*models.User, error) {
	query := `SELECT ` + userSelectColumns + `
		FROM users
		WHERE id = ANY($1)`

	executor := GetExecutor(ctx, r.db)
	rows, err := executor.Query(ctx, query, ids)
	if err != nil {
		return nil, fmt.Errorf("repository: failed to get users by IDs: %w", err)
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repository: error iterating users: %w", err)
	}
	return users, nil
}

// Update persists changes to name and phone_number.
// When called with a transactional context it participates in that transaction.
func (r *userRepository) Update(ctx context.Context, user *models.User) error {
	query := `
		UPDATE users
		SET    name         = $1,
		       phone_number = $2,
		       updated_at   = $3
		WHERE  id           = $4
		RETURNING updated_at`

	executor := GetExecutor(ctx, r.db)
	return executor.QueryRow(ctx, query,
		user.Name,
		user.PhoneNumber,
		user.UpdatedAt,
		user.ID,
	).Scan(&user.UpdatedAt)
}

// Delete hard-deletes the user. For most cases prefer deactivation at the
// society level; this is used only for GDPR erasure and test teardown.
func (r *userRepository) Delete(ctx context.Context, id int64) error {
	if err := validateUserID(id); err != nil {
		return err
	}

	executor := GetExecutor(ctx, r.db)
	result, err := executor.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("repository: failed to delete user %d: %w", id, err)
	}
	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

// UpdateSocietyID sets society_id and role for a user.
// Called inside the society-creation transaction so the assignment is atomic
// with the society INSERT. When txCtx is passed, this participates in that tx.
func (r *userRepository) UpdateSocietyID(ctx context.Context, userID int64, societyID int64, role string) error {
	if err := validateUserID(userID); err != nil {
		return err
	}

	query := `
		UPDATE users
		SET    society_id = $1,
		       role       = $2,
		       updated_at = NOW()
		WHERE  id         = $3`

	executor := GetExecutor(ctx, r.db)
	result, err := executor.Exec(ctx, query, societyID, role, userID)
	if err != nil {
		return fmt.Errorf("repository: failed to assign society to user %d: %w", userID, err)
	}
	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

func (r *userRepository) UpdateLastLogin(ctx context.Context, id int64) error {
	if err := validateUserID(id); err != nil {
		return err
	}

	query := `UPDATE users SET last_login = $1 WHERE id = $2`

	executor := GetExecutor(ctx, r.db)
	result, err := executor.Exec(ctx, query, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("repository: failed to update last_login for user %d: %w", id, err)
	}
	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

func (r *userRepository) UpdatePassword(ctx context.Context, id int64, passwordHash string) error {
	if err := validateUserID(id); err != nil {
		return err
	}
	if passwordHash == "" {
		return fmt.Errorf("repository: password hash must not be empty")
	}

	query := `
		UPDATE users
		SET    password_hash = $1,
		       updated_at    = NOW()
		WHERE  id            = $2`

	executor := GetExecutor(ctx, r.db)
	result, err := executor.Exec(ctx, query, passwordHash, id)
	if err != nil {
		return fmt.Errorf("repository: failed to update password for user %d: %w", id, err)
	}
	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

func (r *userRepository) UpdateIsVerified(ctx context.Context, userID int64, isVerified bool) error {
	if err := validateUserID(userID); err != nil {
		return err
	}

	query := `
		UPDATE users
		SET    is_verified = $1,
		       updated_at  = NOW()
		WHERE  id          = $2`

	executor := GetExecutor(ctx, r.db)
	result, err := executor.Exec(ctx, query, isVerified, userID)
	if err != nil {
		return fmt.Errorf("repository: failed to update is_verified for user %d: %w", userID, err)
	}
	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

func (r *userRepository) EmailExists(ctx context.Context, email string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM users WHERE LOWER(email) = LOWER($1))`
	executor := GetExecutor(ctx, r.db)
	var exists bool
	if err := executor.QueryRow(ctx, query, email).Scan(&exists); err != nil {
		return false, fmt.Errorf("repository: failed to check email existence: %w", err)
	}
	return exists, nil
}

func (r *userRepository) PhoneExists(ctx context.Context, phoneNumber string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM users WHERE phone_number = $1)`
	executor := GetExecutor(ctx, r.db)
	var exists bool
	if err := executor.QueryRow(ctx, query, phoneNumber).Scan(&exists); err != nil {
		return false, fmt.Errorf("repository: failed to check phone existence: %w", err)
	}
	return exists, nil
}

// DeleteExpiredUnverified removes unverified users created more than 24 hours ago
// whose verification tokens have all expired. Run by a background job.
// Returns the number of rows deleted.
func (r *userRepository) DeleteExpiredUnverified(ctx context.Context) (int64, error) {
	query := `
		DELETE FROM users
		WHERE is_verified = FALSE
		  AND created_at  < NOW() - INTERVAL '24 hours'
		  AND NOT EXISTS (
		        SELECT 1
		        FROM   user_verifications
		        WHERE  user_verifications.user_id   = users.id
		          AND  user_verifications.expires_at > NOW()
		      )`

	executor := GetExecutor(ctx, r.db)
	result, err := executor.Exec(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("repository: failed to delete expired unverified users: %w", err)
	}
	return result.RowsAffected(), nil
}

// UpdateFlatID sets (or clears) flat_id for a user.
// Pass nil flatID to remove the flat assignment.
func (r *userRepository) UpdateFlatID(ctx context.Context, userID int64, flatID *int64) error {
	if err := validateUserID(userID); err != nil {
		return err
	}
	query := `
		UPDATE users
		SET    flat_id    = $1,
		       updated_at = NOW()
		WHERE  id         = $2`
	executor := GetExecutor(ctx, r.db)
	result, err := executor.Exec(ctx, query, flatID, userID)
	if err != nil {
		return fmt.Errorf("repository: failed to update flat_id for user %d: %w", userID, err)
	}
	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

// UpdateFlatIDAndRole sets flat_id and role atomically.
// Used during invite redemption where the role is determined by the invite.
func (r *userRepository) UpdateFlatIDAndRole(ctx context.Context, userID int64, flatID *int64, role string) error {
	if err := validateUserID(userID); err != nil {
		return err
	}
	query := `
		UPDATE users
		SET    flat_id    = $1,
		       role       = $2,
		       updated_at = NOW()
		WHERE  id         = $3`
	executor := GetExecutor(ctx, r.db)
	result, err := executor.Exec(ctx, query, flatID, role, userID)
	if err != nil {
		return fmt.Errorf("repository: failed to update flat_id+role for user %d: %w", userID, err)
	}
	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

// GetBySocietyAndRole returns all users in a society with the specified role.
// Useful for finding admins or listing all residents in a society.
func (r *userRepository) GetBySocietyAndRole(ctx context.Context, societyID int64, role string) ([]*models.User, error) {
	query := `SELECT ` + userSelectColumns + `
		FROM users
		WHERE society_id = $1
		  AND role       = $2
		ORDER BY name ASC`
	executor := GetExecutor(ctx, r.db)
	rows, err := executor.Query(ctx, query, societyID, role)
	if err != nil {
		return nil, fmt.Errorf("repository: failed to get users by society and role: %w", err)
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repository: error iterating users: %w", err)
	}
	return users, nil
}

// CountBySocietyAndRole counts the number of users in a society with a specific role.
// Used to enforce plan limits on the number of staff members.
func (r *userRepository) CountBySocietyAndRole(ctx context.Context, societyID int64, role string) (int, error) {
	query := `SELECT COUNT(*) FROM users WHERE society_id = $1 AND role = $2`
	executor := GetExecutor(ctx, r.db)
	var count int
	if err := executor.QueryRow(ctx, query, societyID, role).Scan(&count); err != nil {
		return 0, fmt.Errorf("repository: failed to count users by society and role: %w", err)
	}
	return count, nil
}

// GetByFlatID returns all users currently assigned to a flat.
func (r *userRepository) GetByFlatID(ctx context.Context, flatID int64) ([]*models.User, error) {
	query := `SELECT ` + userSelectColumns + `
		FROM users
		WHERE flat_id = $1
		ORDER BY name ASC`
	executor := GetExecutor(ctx, r.db)
	rows, err := executor.Query(ctx, query, flatID)
	if err != nil {
		return nil, fmt.Errorf("repository: failed to get users by flat_id: %w", err)
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repository: error iterating flat users: %w", err)
	}
	return users, nil
}

// CountByFlatAndRole counts how many users in a flat have a specific role.
// Used to enforce the one-primary-resident-per-flat rule.
func (r *userRepository) CountByFlatAndRole(ctx context.Context, flatID int64, role string) (int, error) {
	query := `SELECT COUNT(*) FROM users WHERE flat_id = $1 AND role = $2`
	executor := GetExecutor(ctx, r.db)
	var count int
	if err := executor.QueryRow(ctx, query, flatID, role).Scan(&count); err != nil {
		return 0, fmt.Errorf("repository: failed to count users by flat and role: %w", err)
	}
	return count, nil
}