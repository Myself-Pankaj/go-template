package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"go-server/internal/models"
	"go-server/pkg/database"
)

// ==================== SENTINEL ERRORS ====================

var (
	ErrSocietyNotFound = models.NewAppError(
		models.ErrCodeNotFound,
		"society not found",
		http.StatusNotFound,
		nil,
	)

	ErrInvalidSocietyID = models.NewAppError(
		models.ErrCodeBadRequest,
		"invalid society ID",
		http.StatusBadRequest,
		nil,
	)

	ErrInvalidSocietyCode = models.NewAppError(
		models.ErrCodeBadRequest,
		"society code must not be empty",
		http.StatusBadRequest,
		nil,
	)

	ErrSocietyCodeConflict = models.NewAppError(
		models.ErrCodeConflict,
		"society code already exists, please retry",
		http.StatusConflict,
		nil,
	)

	ErrSocietyAlreadyExists = models.NewAppError(
		models.ErrCodeConflict,
		"a society with this name already exists in this city",
		http.StatusConflict,
		nil,
	)

	ErrSocietyAlreadyDeleted = models.NewAppError(
		models.ErrCodeConflict,
		"society is already deleted",
		http.StatusConflict,
		nil,
	)
)

// pgUniqueViolation is the Postgres error code for unique constraint violations.
const pgUniqueViolation = "23505"

// ==================== INTERFACE ====================

type SocietyRepository interface {
	// CreateSociety inserts a new society row and returns the full record.
	// When called with a transactional context it participates in that transaction.
	CreateSociety(ctx context.Context, society *models.Society) (*models.Society, error)

	// GetSocietyByID returns a society by primary key.
	// Returns ErrSocietyNotFound for soft-deleted rows.
	GetSocietyByID(ctx context.Context, id int64) (*models.Society, error)

	// GetSocietyByCode returns a society by its unique public code.
	// Returns ErrSocietyNotFound for soft-deleted rows.
	GetSocietyByCode(ctx context.Context, code string) (*models.Society, error)

	// ListSocieties returns societies matching the given filter, ordered by name.
	// Soft-deleted rows are excluded unless filter.IncludeDeleted is true.
	ListSocieties(ctx context.Context, filter *models.SocietyFilter) ([]*models.Society, error)

	// UpdateSociety applies only the non-nil fields from req.
	// is_active, society_code, creator_id, and deleted_at are not touched here.
	UpdateSociety(ctx context.Context, id int64, req *models.UpdateSocietyRequest) (*models.Society, error)

	// SetSocietyActiveStatus sets is_active. Returns the updated record.
	// Idempotent: if already in the requested state, returns current record unchanged.
	SetSocietyActiveStatus(ctx context.Context, id int64, active bool) (*models.Society, error)

	// DeleteSociety soft-deletes by setting deleted_at = NOW().
	// Returns ErrSocietyNotFound if the society doesn't exist.
	// Returns ErrSocietyAlreadyDeleted if already soft-deleted.
	DeleteSociety(ctx context.Context, id int64) error
}

// ==================== IMPLEMENTATION ====================

type societyRepository struct {
	db *database.Database
}

func NewSocietyRepository(db *database.Database) SocietyRepository {
	return &societyRepository{db: db}
}

// ==================== QUERIES ====================

const (
	// societySelectColumns is the canonical column list for all SELECT / RETURNING.
	// Column order must match scanSociety exactly.
	societySelectColumns = `
		id, name, address, city, state, pin_code,
		society_code, creator_id, is_active, deleted_at,
		created_at, updated_at`

	createSocietyQuery = `
		INSERT INTO societies (name, address, city, state, pin_code, society_code, creator_id, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING ` + societySelectColumns

	// getSocietyByIDQuery excludes soft-deleted rows.
	getSocietyByIDQuery = `
		SELECT ` + societySelectColumns + `
		FROM   societies
		WHERE  id         = $1
		AND    deleted_at IS NULL`

	// getSocietyByCodeQuery excludes soft-deleted rows.
	getSocietyByCodeQuery = `
		SELECT ` + societySelectColumns + `
		FROM   societies
		WHERE  society_code = $1
		AND    deleted_at   IS NULL`

	// setSocietyActiveStatusQuery is used by both Activate and Deactivate.
	// $1 = desired is_active, $2 = society id.
	// WHERE is_active != $1 means RowsAffected == 0 on idempotent calls.
	setSocietyActiveStatusQuery = `
		UPDATE societies
		SET    is_active  = $1,
		       updated_at = NOW()
		WHERE  id         = $2
		AND    is_active  != $1
		AND    deleted_at IS NULL
		RETURNING ` + societySelectColumns

	// deleteSocietyQuery soft-deletes by stamping deleted_at.
	// WHERE deleted_at IS NULL prevents double-deleting.
	deleteSocietyQuery = `
		UPDATE societies
		SET    deleted_at = NOW(),
		       is_active  = FALSE,
		       updated_at = NOW()
		WHERE  id         = $1
		AND    deleted_at IS NULL`
)

// ==================== METHODS ====================

func (r *societyRepository) CreateSociety(ctx context.Context, society *models.Society) (*models.Society, error) {
	executor := GetExecutor(ctx, r.db)

	row := executor.QueryRow(ctx, createSocietyQuery,
		society.Name,
		society.Address,
		society.City,
		society.State,
		society.PinCode,
		society.SocietyCode,
		society.CreatorID,
		society.IsActive,
	)

	created, err := scanSociety(row)
	if err != nil {
		return nil, mapWriteError(err, "create")
	}
	return created, nil
}

func (r *societyRepository) GetSocietyByID(ctx context.Context, id int64) (*models.Society, error) {
	if err := validateSocietyID(id); err != nil {
		return nil, err
	}

	executor := GetExecutor(ctx, r.db)
	return scanSociety(executor.QueryRow(ctx, getSocietyByIDQuery, id))
}

func (r *societyRepository) GetSocietyByCode(ctx context.Context, code string) (*models.Society, error) {
	if strings.TrimSpace(code) == "" {
		return nil, ErrInvalidSocietyCode
	}

	executor := GetExecutor(ctx, r.db)
	return scanSociety(executor.QueryRow(ctx, getSocietyByCodeQuery, code))
}

// ListSocieties returns societies matching filter, ordered by name ascending.
// Soft-deleted rows are excluded by default; set filter.IncludeDeleted to include them.
func (r *societyRepository) ListSocieties(ctx context.Context, filter *models.SocietyFilter) ([]*models.Society, error) {
	query, args := buildListSocietiesQuery(filter)

	executor := GetExecutor(ctx, r.db)
	rows, err := executor.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("repository: failed to list societies: %w", err)
	}
	defer rows.Close()

	return scanSocietyRows(rows)
}

// UpdateSociety applies only the non-nil fields from req.
// is_active, society_code, creator_id, and deleted_at are intentionally excluded.
func (r *societyRepository) UpdateSociety(ctx context.Context, id int64, req *models.UpdateSocietyRequest) (*models.Society, error) {
	if err := validateSocietyID(id); err != nil {
		return nil, err
	}

	setClauses, args := buildSocietyUpdateArgs(req)
	if len(setClauses) == 0 {
		return r.GetSocietyByID(ctx, id)
	}

	args = append(args, id)
	idParam := fmt.Sprintf("$%d", len(args))

	query := fmt.Sprintf(`
		UPDATE societies
		SET    %s,
		       updated_at = NOW()
		WHERE  id         = %s
		AND    deleted_at IS NULL
		RETURNING %s`,
		joinClauses(setClauses), idParam, societySelectColumns,
	)

	executor := GetExecutor(ctx, r.db)
	row := executor.QueryRow(ctx, query, args...)

	updated, err := scanSociety(row)
	if err != nil {
		return nil, mapWriteError(err, "update")
	}
	return updated, nil
}

// SetSocietyActiveStatus sets is_active and returns the updated record.
// If the society is already in the requested state, the current record is returned unchanged.
func (r *societyRepository) SetSocietyActiveStatus(ctx context.Context, id int64, active bool) (*models.Society, error) {
	if err := validateSocietyID(id); err != nil {
		return nil, err
	}

	executor := GetExecutor(ctx, r.db)
	row := executor.QueryRow(ctx, setSocietyActiveStatusQuery, active, id)

	society, err := scanSociety(row)
	if err != nil {
		if errors.Is(err, ErrSocietyNotFound) {
			// ErrNoRows has two meanings here: truly not found, or already in requested state.
			return r.checkSocietyExists(ctx, id, active)
		}
		return nil, err
	}
	return society, nil
}

// DeleteSociety soft-deletes a society by setting deleted_at = NOW().
// Also sets is_active = false so access is immediately blocked.
// Returns ErrSocietyAlreadyDeleted if already soft-deleted.
func (r *societyRepository) DeleteSociety(ctx context.Context, id int64) error {
	if err := validateSocietyID(id); err != nil {
		return err
	}

	executor := GetExecutor(ctx, r.db)
	result, err := executor.Exec(ctx, deleteSocietyQuery, id)
	if err != nil {
		return fmt.Errorf("repository: failed to delete society %d: %w", id, err)
	}

	if result.RowsAffected() == 0 {
		// Either doesn't exist or already deleted — distinguish with a lookup.
		existing, lookupErr := r.GetSocietyByID(ctx, id)
		if lookupErr != nil {
			return ErrSocietyNotFound
		}
		if existing.IsDeleted() {
			return ErrSocietyAlreadyDeleted
		}
		return ErrSocietyNotFound
	}
	return nil
}

// checkSocietyExists is called after a SetSocietyActiveStatus no-op to distinguish
// "society not found" from "already in the requested state".
func (r *societyRepository) checkSocietyExists(ctx context.Context, id int64, requestedActive bool) (*models.Society, error) {
	existing, err := r.GetSocietyByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing.IsActive == requestedActive {
		return existing, nil
	}
	return nil, ErrSocietyNotFound
}

// ==================== HELPERS ====================

// scanSociety scans one row into a Society.
// Column order must match societySelectColumns exactly.
func scanSociety(row interface{ Scan(dest ...any) error }) (*models.Society, error) {
	var s models.Society
	err := row.Scan(
		&s.Id,
		&s.Name,
		&s.Address,
		&s.City,
		&s.State,
		&s.PinCode,
		&s.SocietyCode,
		&s.CreatorID,
		&s.IsActive,
		&s.DeletedAt,  // *time.Time — nil when not deleted
		&s.CreatedAt,
		&s.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrSocietyNotFound
		}
		return nil, fmt.Errorf("repository: failed to scan society: %w", err)
	}
	return &s, nil
}

// scanSocietyRows iterates a Rows result and scans each into a Society.
// Always returns an empty slice (never nil) when no rows exist.
func scanSocietyRows(rows interface {
	Next() bool
	Err() error
	Scan(dest ...any) error
}) ([]*models.Society, error) {
	var societies []*models.Society
	for rows.Next() {
		s, err := scanSociety(rows)
		if err != nil {
			return nil, err
		}
		societies = append(societies, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repository: error iterating society rows: %w", err)
	}
	if societies == nil {
		societies = []*models.Society{}
	}
	return societies, nil
}

// buildSocietyUpdateArgs builds SET clause fragments for UpdateSociety.
// is_active, society_code, creator_id, and deleted_at are intentionally excluded.
func buildSocietyUpdateArgs(req *models.UpdateSocietyRequest) ([]string, []any) {
	var clauses []string
	var args []any

	add := func(col string, val any) {
		args = append(args, val)
		clauses = append(clauses, fmt.Sprintf("%s = $%d", col, len(args)))
	}

	if req.Name != nil {
		add("name", *req.Name)
	}
	if req.Address != nil {
		add("address", *req.Address)
	}
	if req.City != nil {
		add("city", *req.City)
	}
	if req.State != nil {
		add("state", *req.State)
	}
	if req.PinCode != nil {
		add("pin_code", *req.PinCode)
	}

	return clauses, args
}

// buildListSocietiesQuery constructs the dynamic SELECT for ListSocieties.
// Soft-deleted rows are excluded by default via the deleted_at IS NULL guard.
func buildListSocietiesQuery(filter *models.SocietyFilter) (string, []any) {
	var conditions []string
	var args []any

	nextParam := func(val any) string {
		args = append(args, val)
		return fmt.Sprintf("$%d", len(args))
	}

	// Soft-delete guard — only bypassed when admin explicitly requests deleted records.
	if filter == nil || !filter.IncludeDeleted {
		conditions = append(conditions, "deleted_at IS NULL")
	}

	if filter != nil {
		if filter.ActiveOnly {
			conditions = append(conditions, "is_active = true")
		}
		if filter.City != nil {
			conditions = append(conditions, fmt.Sprintf("LOWER(city) ILIKE %s", nextParam("%"+*filter.City+"%")))
		}
		if filter.State != nil {
			conditions = append(conditions, fmt.Sprintf("LOWER(state) ILIKE %s", nextParam("%"+*filter.State+"%")))
		}
		if filter.PinCode != nil {
			conditions = append(conditions, fmt.Sprintf("pin_code = %s", nextParam(*filter.PinCode)))
		}
	}

	query := `SELECT ` + societySelectColumns + ` FROM societies`
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY name ASC"

	return query, args
}

// joinClauses joins SET clause fragments with consistent indentation.
func joinClauses(clauses []string) string {
	var sb strings.Builder
	for i, c := range clauses {
		if i > 0 {
			sb.WriteString(",\n		       ")
		}
		sb.WriteString(c)
	}
	return sb.String()
}

// mapWriteError translates Postgres constraint violations into domain errors.
func mapWriteError(err error, op string) error {
	var appErr *models.AppError
	if errors.As(err, &appErr) {
		return err
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
		switch pgErr.ConstraintName {
		case "societies_society_code_key":
			return ErrSocietyCodeConflict
		case "societies_name_city_key":
			return ErrSocietyAlreadyExists
		}
	}

	return fmt.Errorf("repository: failed to %s society: %w", op, err)
}

// validateSocietyID returns an error if the given ID is not positive.
func validateSocietyID(id int64) error {
	if id <= 0 {
		return ErrInvalidSocietyID
	}
	return nil
}