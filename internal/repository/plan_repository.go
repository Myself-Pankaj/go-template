package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5/pgconn"

	"go-server/internal/models"
	"go-server/pkg/database"
)

// ==================== SENTINEL ERRORS ====================

var (
	ErrPlanNotFound = models.NewAppError(
		models.ErrCodeNotFound,
		"plan not found",
		http.StatusNotFound,
		nil,
	)

	ErrInvalidPlanID = models.NewAppError(
		models.ErrCodeBadRequest,
		"invalid plan ID",
		http.StatusBadRequest,
		nil,
	)

	ErrInvalidPlanName = models.NewAppError(
		models.ErrCodeBadRequest,
		"plan name must not be empty",
		http.StatusBadRequest,
		nil,
	)

	ErrPlanAlreadyExists = models.NewAppError(
		models.ErrCodeConflict,
		"a plan with this name already exists",
		http.StatusConflict,
		nil,
	)

	ErrInvalidBillingCycle = models.NewAppError(
		models.ErrCodeBadRequest,
		"billing cycle must be 'monthly' or 'yearly'",
		http.StatusBadRequest,
		nil,
	)
)

// ==================== INTERFACE ====================

type PlanRepository interface {
	CreatePlan(ctx context.Context, plan *models.Plan) (*models.Plan, error)
	GetPlanByID(ctx context.Context, id int64) (*models.Plan, error)
	GetPlanByName(ctx context.Context, name string) (*models.Plan, error)
	ListActivePlans(ctx context.Context) ([]*models.Plan, error)
	ListAllPlans(ctx context.Context) ([]*models.Plan, error)
	UpdatePlan(ctx context.Context, id int64, req *models.UpdatePlanRequest) (*models.Plan, error)
	SetPlanActiveStatus(ctx context.Context, id int64, active bool) (*models.Plan, error)
}

// ==================== IMPLEMENTATION ====================

type planRepository struct {
	db *database.Database
}

func NewPlanRepository(db *database.Database) PlanRepository {
	return &planRepository{db: db}
}

// ==================== QUERIES ====================

const (
	// planSelectColumns is the canonical column list shared by all SELECT / RETURNING clauses.
	// Column order must match scanPlan exactly.
	planSelectColumns = `id, name, price, billing_cycle, max_flats, max_staff, max_admins, is_active, created_at, updated_at`

	createPlanQuery = `
		INSERT INTO plans (name, price, billing_cycle, max_flats, max_staff, max_admins, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING ` + planSelectColumns

	getPlanByIDQuery = `
		SELECT ` + planSelectColumns + `
		FROM plans
		WHERE id = $1`

	getPlanByNameQuery = `
		SELECT ` + planSelectColumns + `
		FROM plans
		WHERE name = $1`

	// listActivePlansQuery returns only plans currently visible to users (pricing page, mobile app).
	// Ordered by price ascending so the cheapest plan appears first naturally.
	listActivePlansQuery = `
		SELECT ` + planSelectColumns + `
		FROM plans
		WHERE is_active = true
		ORDER BY price ASC`

	// listAllPlansQuery returns every plan regardless of status.
	// Used by the Super Admin dashboard to manage the full plan catalogue.
	listAllPlansQuery = `
		SELECT ` + planSelectColumns + `
		FROM plans
		ORDER BY price ASC`

	// setPlanActiveStatusQuery is used by both Activate and Deactivate.
	// $1 = desired is_active value, $2 = plan id.
	// The WHERE guard (is_active != $1) prevents a no-op from counting as a hit,
	// which allows us to detect "already in that state" via RowsAffected == 0.
	setPlanActiveStatusQuery = `
		UPDATE plans
		SET    is_active  = $1,
		       updated_at = NOW()
		WHERE  id         = $2
		AND    is_active  != $1
		RETURNING ` + planSelectColumns
)

// ==================== METHODS ====================

func (r *planRepository) CreatePlan(ctx context.Context, plan *models.Plan) (*models.Plan, error) {
	executor := GetExecutor(ctx, r.db)

	row := executor.QueryRow(ctx, createPlanQuery,
		plan.Name,
		plan.Price,
		plan.BillingCycle,
		plan.MaxFlats,  // *int — pgx sends NULL for nil automatically
		plan.MaxStaff,  // *int
		plan.MaxAdmins, // *int
		plan.IsActive,
	)

	created, err := scanPlan(row)
	if err != nil {
		return nil, mapPlanWriteError(err, "create")
	}
	return created, nil
}

func (r *planRepository) GetPlanByID(ctx context.Context, id int64) (*models.Plan, error) {
	
	executor := GetExecutor(ctx, r.db)
	row := executor.QueryRow(ctx, getPlanByIDQuery, id)

	return scanPlan(row)
}

func (r *planRepository) GetPlanByName(ctx context.Context, name string) (*models.Plan, error) {
	if name == "" {
		return nil, ErrInvalidPlanName
	}

	executor := GetExecutor(ctx, r.db)
	row := executor.QueryRow(ctx, getPlanByNameQuery, name)

	return scanPlan(row)
}

// ListActivePlans returns all plans with is_active = true, ordered by price ascending.
// This is the query behind the public pricing page and mobile app plan selector.
func (r *planRepository) ListActivePlans(ctx context.Context) ([]*models.Plan, error) {
	executor := GetExecutor(ctx, r.db)

	rows, err := executor.Query(ctx, listActivePlansQuery)
	if err != nil {
		return nil, fmt.Errorf("repository: failed to list active plans: %w", err)
	}
	defer rows.Close()

	return scanPlanRows(rows)
}

// ListAllPlans returns every plan regardless of active status, ordered by price ascending.
// Intended for the Super Admin dashboard only.
func (r *planRepository) ListAllPlans(ctx context.Context) ([]*models.Plan, error) {
	executor := GetExecutor(ctx, r.db)

	rows, err := executor.Query(ctx, listAllPlansQuery)
	if err != nil {
		return nil, fmt.Errorf("repository: failed to list all plans: %w", err)
	}
	defer rows.Close()

	return scanPlanRows(rows)
}

// UpdatePlan applies only the non-nil (or explicitly cleared) fields from req
// to the plan identified by id. Returns the full updated record on success.
// is_active is not touched here — use SetPlanActiveStatus for that.
func (r *planRepository) UpdatePlan(ctx context.Context, id int64, req *models.UpdatePlanRequest) (*models.Plan, error) {
	if err := validatePlanID(id); err != nil {
		return nil, err
	}

	setClauses, args := buildPlanUpdateArgs(req)
	if len(setClauses) == 0 {
		// No fields to update — return the current record unchanged.
		return r.GetPlanByID(ctx, id)
	}

	args = append(args, id)
	idParam := fmt.Sprintf("$%d", len(args))

	query := fmt.Sprintf(`
		UPDATE plans
		SET    %s,
		       updated_at = NOW()
		WHERE  id         = %s
		RETURNING %s`,
		joinPlanClauses(setClauses), idParam, planSelectColumns,
	)

	executor := GetExecutor(ctx, r.db)
	row := executor.QueryRow(ctx, query, args...)

	updated, err := scanPlan(row)
	if err != nil {
		return nil, mapPlanWriteError(err, "update")
	}
	return updated, nil
}

// SetPlanActiveStatus sets is_active to the given value for the plan with the given ID.
// Returns ErrPlanNotFound if the plan does not exist.
// Returns ErrPlanAlreadyInState if the plan is already in the requested state,
// so callers can distinguish a true no-op from a real state change.
func (r *planRepository) SetPlanActiveStatus(ctx context.Context, id int64, active bool) (*models.Plan, error) {
	if err := validatePlanID(id); err != nil {
		return nil, err
	}

	executor := GetExecutor(ctx, r.db)
	row := executor.QueryRow(ctx, setPlanActiveStatusQuery, active, id)

	plan, err := scanPlan(row)
	if err != nil {
		// scanPlan returns ErrPlanNotFound on sql.ErrNoRows.
		// But here ErrNoRows has two meanings: plan doesn't exist, OR it was
		// already in the requested state (WHERE is_active != $1 filtered it out).
		// We distinguish them with a second lookup.
		if errors.Is(err, ErrPlanNotFound) {
			return r.checkPlanExists(ctx, id, active)
		}
		return nil, err
	}
	return plan, nil
}

// checkPlanExists is called after a SetPlanActiveStatus no-op to distinguish
// "plan not found" from "plan already in the requested state".
func (r *planRepository) checkPlanExists(ctx context.Context, id int64, requestedActive bool) (*models.Plan, error) {
	existing, err := r.GetPlanByID(ctx, id)
	if err != nil {
		// Plan truly does not exist.
		return nil, err
	}
	// Plan exists but is already in the requested state — return it as-is.
	if existing.IsActive == requestedActive {
		return existing, nil
	}
	// Shouldn't be reachable, but surface the original not-found to be safe.
	return nil, ErrPlanNotFound
}

// ==================== HELPERS ====================

// scanPlan scans a single database row into a Plan model.
// Column order must match planSelectColumns exactly.
// MaxFlats, MaxStaff, MaxAdmins are scanned into *int — NULL becomes nil.
func scanPlan(row interface{ Scan(dest ...any) error }) (*models.Plan, error) {
	var p models.Plan
	err := row.Scan(
		&p.Id,
		&p.Name,
		&p.Price,
		&p.BillingCycle,
		&p.MaxFlats,  // *int — nil when NULL
		&p.MaxStaff,  // *int
		&p.MaxAdmins, // *int
		&p.IsActive,
		&p.CreatedAt,
		&p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrPlanNotFound
		}
		return nil, fmt.Errorf("repository: failed to scan plan: %w", err)
	}
	return &p, nil
}

// scanPlanRows iterates a Rows result set and scans each row into a Plan.
// The caller is responsible for calling rows.Close() before this is called,
// or — as is done in the list methods — deferring it immediately after Query.
func scanPlanRows(rows interface {
	Next() bool
	Err() error
	Scan(dest ...any) error
}) ([]*models.Plan, error) {
	var plans []*models.Plan
	for rows.Next() {
		p, err := scanPlan(rows)
		if err != nil {
			return nil, err
		}
		plans = append(plans, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repository: error iterating plan rows: %w", err)
	}
	// Return an empty slice rather than nil so JSON serialises to [] not null.
	if plans == nil {
		plans = []*models.Plan{}
	}
	return plans, nil
}

// buildPlanUpdateArgs converts an UpdatePlanRequest into parallel slices of
// SET clause fragments ("name = $1") and bind values, numbered from $1.
//
// is_active is intentionally excluded — use SetPlanActiveStatus for that.
//
// ClearMax* fields emit "column = NULL" directly in SQL (no bind parameter needed)
// because NULL is not a value that can be passed as a typed parameter in all drivers.
// The corresponding value field is ignored when its Clear flag is set.
func buildPlanUpdateArgs(req *models.UpdatePlanRequest) ([]string, []any) {
	var clauses []string
	var args []any

	// addVal appends a parameterised assignment: column = $N
	addVal := func(col string, val any) {
		args = append(args, val)
		clauses = append(clauses, fmt.Sprintf("%s = $%d", col, len(args)))
	}

	// addNull appends a literal NULL assignment: column = NULL (no bind param)
	addNull := func(col string) {
		clauses = append(clauses, fmt.Sprintf("%s = NULL", col))
	}

	if req.Name != nil {
		addVal("name", *req.Name)
	}
	if req.Price != nil {
		addVal("price", *req.Price)
	}
	if req.BillingCycle != nil {
		addVal("billing_cycle", string(*req.BillingCycle))
	}

	// Limit fields: Clear flag takes precedence over value.
	switch {
	case req.ClearMaxFlats:
		addNull("max_flats")
	case req.MaxFlats != nil:
		addVal("max_flats", *req.MaxFlats)
	}

	switch {
	case req.ClearMaxStaff:
		addNull("max_staff")
	case req.MaxStaff != nil:
		addVal("max_staff", *req.MaxStaff)
	}

	switch {
	case req.ClearMaxAdmins:
		addNull("max_admins")
	case req.MaxAdmins != nil:
		addVal("max_admins", *req.MaxAdmins)
	}

	return clauses, args
}

// joinPlanClauses joins SET clause fragments with a comma and consistent indentation.
func joinPlanClauses(clauses []string) string {
	result := ""
	for i, c := range clauses {
		if i > 0 {
			result += ",\n		       "
		}
		result += c
	}
	return result
}

// mapPlanWriteError translates Postgres constraint violations into domain errors.
// op should be "create" or "update" for clear wrapped error messages.
func mapPlanWriteError(err error, op string) error {
	// Pass through our own AppErrors (e.g. ErrPlanNotFound from scanPlan on UPDATE).
	var appErr *models.AppError
	if errors.As(err, &appErr) {
		return err
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
		if pgErr.ConstraintName == "plans_name_key" {
			return ErrPlanAlreadyExists
		}
	}

	return fmt.Errorf("repository: failed to %s plan: %w", op, err)
}

// validatePlanID returns an error if the given ID is not positive.
func validatePlanID(id int64) error {
	if id <= 0 {
		return ErrInvalidPlanID
	}
	return nil
}