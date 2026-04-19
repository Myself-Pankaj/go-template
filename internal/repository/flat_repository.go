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
	ErrFlatNotFound      = errors.New("flat not found")
	ErrFlatAlreadyExists = errors.New("flat already exists in this society")
)

// ==================== INTERFACE ====================

type FlatRepository interface {
	CreateFlat(ctx context.Context, flat *models.Flat) (*models.Flat, error)
	GetFlatByID(ctx context.Context, id int64) (*models.Flat, error)
	GetFlatsBySocietyID(ctx context.Context, societyID int64) ([]*models.Flat, error)
	GetByIDs(ctx context.Context, ids []int64) ([]*models.Flat, error)
	GetFlatByNumber(ctx context.Context, societyID int64, flatNumber string) (*models.Flat, error)
	UpdateFlat(ctx context.Context, flat *models.Flat) (*models.Flat, error)
	UpdateFlatStatus(ctx context.Context, id int64, status models.FlatStatus) (*models.Flat, error)
	DeleteFlat(ctx context.Context, id int64) error
}

// ==================== IMPLEMENTATION ====================

type flatRepository struct {
	db *database.Database
}

func NewFlatRepository(db *database.Database) FlatRepository {
	return &flatRepository{db: db}
}

// ==================== QUERIES ====================

const (
	createFlatQuery = `
		INSERT INTO flats (society_id, flat_number, floor, block)
		VALUES ($1, $2, $3, $4)
		RETURNING id, society_id, flat_number, floor, block, status, created_at, updated_at
	`
	getFlatByIDQuery = `
		SELECT id, society_id, flat_number, floor, block, status, created_at, updated_at
		FROM   flats
		WHERE  id = $1
	`
	getFlatsBySocietyIDQuery = `
		SELECT id, society_id, flat_number, floor, block, status, created_at, updated_at
		FROM   flats
		WHERE  society_id = $1
		ORDER  BY flat_number ASC
	`
	updateFlatQuery = `
		UPDATE flats
		SET    flat_number = $1,
		       floor       = $2,
		       block       = $3
		WHERE  id = $4
		RETURNING id, society_id, flat_number, floor, block, status, created_at, updated_at
	`
	updateFlatStatusQuery = `
		UPDATE flats
		SET    status = $1
		WHERE  id = $2
		RETURNING id, society_id, flat_number, floor, block, status, created_at, updated_at
	`
	deleteFlatQuery = `
		DELETE FROM flats
		WHERE id = $1
	`
	getFlatByNumberQuery = `
        SELECT id, society_id, flat_number, floor, block, status, created_at, updated_at
        FROM   flats
        WHERE  society_id = $1
        AND    flat_number = $2
    `

)

// ==================== METHODS ====================

func (r *flatRepository) CreateFlat(ctx context.Context, flat *models.Flat) (*models.Flat, error) {
	executor := GetExecutor(ctx, r.db)
	row := executor.QueryRow(ctx, createFlatQuery,
		flat.SocietyID,
		flat.FlatNumber,
		flat.Floor,
		flat.Block,
	)
	created, err := scanFlat(row)
	if err != nil {
		return nil, mapFlatWriteError(err, "create")
	}
	return created, nil
}

func (r *flatRepository) GetFlatByID(ctx context.Context, id int64) (*models.Flat, error) {
	executor := GetExecutor(ctx, r.db)
	row := executor.QueryRow(ctx, getFlatByIDQuery, id)
	flat, err := scanFlat(row)
	if err != nil {
		return nil, fmt.Errorf("repository: failed to get flat by ID: %w", err)
	}
	return flat, nil
}

func (r *flatRepository) GetFlatsBySocietyID(ctx context.Context, societyID int64) ([]*models.Flat, error) {
	executor := GetExecutor(ctx, r.db)
	rows, err := executor.Query(ctx, getFlatsBySocietyIDQuery, societyID)
	if err != nil {
		return nil, fmt.Errorf("repository: failed to get flats by society ID: %w", err)
	}
	defer rows.Close()

	flats, err := scanFlatRows(rows)
	if err != nil {
		return nil, fmt.Errorf("repository: failed to scan flats by society ID: %w", err)
	}
	return flats, nil
}
func (r *flatRepository) GetByIDs(ctx context.Context, ids []int64) ([]*models.Flat, error) {
	executor := GetExecutor(ctx, r.db)
	rows, err := executor.Query(ctx, `SELECT id, society_id, flat_number, floor, block, status, created_at, updated_at FROM flats WHERE id = ANY($1)`,
		ids,
	)
	if err != nil {
		return nil, fmt.Errorf("repository: failed to get flats by IDs: %w", err)
	}
	defer rows.Close()

	flats, err := scanFlatRows(rows)
	if err != nil {
		return nil, fmt.Errorf("repository: failed to scan flats by IDs: %w", err)
	}
	return flats, nil
}
func (r *flatRepository) GetFlatByNumber(ctx context.Context, societyID int64, flatNumber string) (*models.Flat, error) {
    executor := GetExecutor(ctx, r.db)
    row := executor.QueryRow(ctx, getFlatByNumberQuery, societyID, flatNumber)
    flat, err := scanFlat(row)
    if err != nil {
        return nil, fmt.Errorf("repository: failed to get flat by number: %w", err)
    }
    return flat, nil
}
func (r *flatRepository) UpdateFlat(ctx context.Context, flat *models.Flat) (*models.Flat, error) {
	executor := GetExecutor(ctx, r.db)
	row := executor.QueryRow(ctx, updateFlatQuery,
		flat.FlatNumber,
		flat.Floor,
		flat.Block,
		flat.ID,
	)
	updated, err := scanFlat(row)
	if err != nil {
		return nil, mapFlatWriteError(err, "update")
	}
	return updated, nil
}

func (r *flatRepository) UpdateFlatStatus(ctx context.Context, id int64, status models.FlatStatus) (*models.Flat, error) {
	executor := GetExecutor(ctx, r.db)
	row := executor.QueryRow(ctx, updateFlatStatusQuery, status, id)
	updated, err := scanFlat(row)
	if err != nil {
		return nil, mapFlatWriteError(err, "update status")
	}
	return updated, nil
}

func (r *flatRepository) DeleteFlat(ctx context.Context, id int64) error {
	executor := GetExecutor(ctx, r.db)
	result, err := executor.Exec(ctx, deleteFlatQuery, id)
	if err != nil {
		return fmt.Errorf("repository: failed to delete flat: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrFlatNotFound
	}
	return nil
}

// ==================== HELPERS ====================

func scanFlat(row interface{ Scan(dest ...any) error }) (*models.Flat, error) {
	var f models.Flat
	err := row.Scan(
		&f.ID,
		&f.SocietyID,
		&f.FlatNumber,
		&f.Floor,
		&f.Block,
		&f.Status,
		&f.CreatedAt,
		&f.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrFlatNotFound
		}
		return nil, fmt.Errorf("repository: failed to scan flat: %w", err)
	}
	return &f, nil
}

func scanFlatRows(rows interface {
	Next() bool
	Err() error
	Scan(dest ...any) error
}) ([]*models.Flat, error) {
	var flats []*models.Flat
	for rows.Next() {
		flat, err := scanFlat(rows)
		if err != nil {
			return nil, fmt.Errorf("repository: failed to scan flat row: %w", err)
		}
		flats = append(flats, flat)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repository: error iterating flat rows: %w", err)
	}
	return flats, nil
}

func mapFlatWriteError(err error, op string) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		if pgErr.ConstraintName == "uq_flats_society_flat" {
			return ErrFlatAlreadyExists
		}
	}
	return fmt.Errorf("repository: failed to %s flat: %w", op, err)
}