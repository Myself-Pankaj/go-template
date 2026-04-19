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
	ErrInviteNotFound      = errors.New("invite not found")
	ErrInviteAlreadyExists = errors.New("invite token already exists") // token collision (extremely rare)
	ErrInviteRevoked       = errors.New("invite has been revoked")
	ErrInviteExpired       = errors.New("invite has expired")
	ErrInviteExhausted     = errors.New("invite usage limit reached")
)

// ==================== INTERFACE ====================

type InviteRepository interface {
	// Write
	CreateInvite(ctx context.Context, invite *models.FlatInvite) (*models.FlatInvite, error)
	IncrementUsedCount(ctx context.Context, id int64) error
	RevokeInvite(ctx context.Context, id int64) error
	RevokeAllInvitesByFlat(ctx context.Context, flatID int64) error

	// Read
	GetInviteByID(ctx context.Context, id int64) (*models.FlatInvite, error)
	GetInviteByToken(ctx context.Context, token string) (*models.FlatInvite, error)
	GetActiveInvitesByFlat(ctx context.Context, flatID int64) ([]*models.FlatInvite, error)
	GetInvitesByCreator(ctx context.Context, createdBy int64) ([]*models.FlatInvite, error)
}

// ==================== IMPLEMENTATION ====================

type inviteRepository struct {
	db *database.Database
}

func NewInviteRepository(db *database.Database) InviteRepository {
	return &inviteRepository{db: db}
}

// ==================== QUERIES ====================

const (
	createInviteQuery = `
		INSERT INTO flat_invites (flat_id, token, created_by, max_uses, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, flat_id, token, created_by, max_uses, used_count,
		          expires_at, is_revoked, created_at, updated_at
	`

	// Atomic increment — prevents race conditions when multiple goroutines
	// try to redeem the same invite concurrently.
	incrementUsedCountQuery = `
		UPDATE flat_invites
		SET    used_count = used_count + 1
		WHERE  id = $1
		RETURNING id
	`

	revokeInviteQuery = `
		UPDATE flat_invites
		SET    is_revoked = TRUE
		WHERE  id = $1
		AND    is_revoked = FALSE
		RETURNING id
	`

	revokeAllByFlatQuery = `
		UPDATE flat_invites
		SET    is_revoked = TRUE
		WHERE  flat_id    = $1
		AND    is_revoked = FALSE
	`

	getInviteByIDQuery = `
		SELECT id, flat_id, token, created_by, max_uses, used_count,
		       expires_at, is_revoked, created_at, updated_at
		FROM   flat_invites
		WHERE  id = $1
	`

	// Hot path: called on every token redemption attempt.
	getInviteByTokenQuery = `
		SELECT id, flat_id, token, created_by, max_uses, used_count,
		       expires_at, is_revoked, created_at, updated_at
		FROM   flat_invites
		WHERE  token = $1
	`

	getActiveInvitesByFlatQuery = `
		SELECT id, flat_id, token, created_by, max_uses, used_count,
		       expires_at, is_revoked, created_at, updated_at
		FROM   flat_invites
		WHERE  flat_id    = $1
		AND    is_revoked = FALSE
		ORDER  BY created_at DESC
	`

	getInvitesByCreatorQuery = `
		SELECT id, flat_id, token, created_by, max_uses, used_count,
		       expires_at, is_revoked, created_at, updated_at
		FROM   flat_invites
		WHERE  created_by = $1
		ORDER  BY created_at DESC
	`
)

// ==================== METHODS ====================

func (r *inviteRepository) CreateInvite(ctx context.Context, invite *models.FlatInvite) (*models.FlatInvite, error) {
	executor := GetExecutor(ctx, r.db)
	row := executor.QueryRow(ctx, createInviteQuery,
		invite.FlatID,
		invite.Token,
		invite.CreatedBy,
		invite.MaxUses,
		invite.ExpiresAt,
	)
	created, err := scanInvite(row)
	if err != nil {
		return nil, mapInviteWriteError(err, "create")
	}
	return created, nil
}

// IncrementUsedCount atomically bumps the used_count by 1.
// Call this inside the same transaction as user creation on redemption
// so the count and the new user record are always in sync.
func (r *inviteRepository) IncrementUsedCount(ctx context.Context, id int64) error {
	executor := GetExecutor(ctx, r.db)
	row := executor.QueryRow(ctx, incrementUsedCountQuery, id)

	var returnedID int64
	if err := row.Scan(&returnedID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInviteNotFound
		}
		return fmt.Errorf("repository: failed to increment invite used_count: %w", err)
	}
	return nil
}

// RevokeInvite soft-deletes a single invite. Idempotent — no error if already revoked.
func (r *inviteRepository) RevokeInvite(ctx context.Context, id int64) error {
	executor := GetExecutor(ctx, r.db)
	row := executor.QueryRow(ctx, revokeInviteQuery, id)

	var returnedID int64
	if err := row.Scan(&returnedID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Either not found or already revoked — both are acceptable outcomes.
			return ErrInviteNotFound
		}
		return fmt.Errorf("repository: failed to revoke invite: %w", err)
	}
	return nil
}

// RevokeAllInvitesByFlat bulk-revokes every active invite for a flat.
// Call this when a flat claim is approved so dangling tokens cannot be redeemed.
func (r *inviteRepository) RevokeAllInvitesByFlat(ctx context.Context, flatID int64) error {
	executor := GetExecutor(ctx, r.db)
	_, err := executor.Exec(ctx, revokeAllByFlatQuery, flatID)
	if err != nil {
		return fmt.Errorf("repository: failed to revoke invites by flat: %w", err)
	}
	return nil
}

func (r *inviteRepository) GetInviteByID(ctx context.Context, id int64) (*models.FlatInvite, error) {
	executor := GetExecutor(ctx, r.db)
	row := executor.QueryRow(ctx, getInviteByIDQuery, id)
	invite, err := scanInvite(row)
	if err != nil {
		return nil, fmt.Errorf("repository: failed to get invite by ID: %w", err)
	}
	return invite, nil
}

func (r *inviteRepository) GetInviteByToken(ctx context.Context, token string) (*models.FlatInvite, error) {
	executor := GetExecutor(ctx, r.db)
	row := executor.QueryRow(ctx, getInviteByTokenQuery, token)
	invite, err := scanInvite(row)
	if err != nil {
		return nil, fmt.Errorf("repository: failed to get invite by token: %w", err)
	}
	return invite, nil
}

func (r *inviteRepository) GetActiveInvitesByFlat(ctx context.Context, flatID int64) ([]*models.FlatInvite, error) {
	executor := GetExecutor(ctx, r.db)
	rows, err := executor.Query(ctx, getActiveInvitesByFlatQuery, flatID)
	if err != nil {
		return nil, fmt.Errorf("repository: failed to get active invites by flat: %w", err)
	}
	defer rows.Close()
	return scanInviteRows(rows)
}

func (r *inviteRepository) GetInvitesByCreator(ctx context.Context, createdBy int64) ([]*models.FlatInvite, error) {
	executor := GetExecutor(ctx, r.db)
	rows, err := executor.Query(ctx, getInvitesByCreatorQuery, createdBy)
	if err != nil {
		return nil, fmt.Errorf("repository: failed to get invites by creator: %w", err)
	}
	defer rows.Close()
	return scanInviteRows(rows)
}

// ==================== HELPERS ====================

func scanInvite(row interface{ Scan(dest ...any) error }) (*models.FlatInvite, error) {
	var i models.FlatInvite
	err := row.Scan(
		&i.ID,
		&i.FlatID,
		&i.Token,
		&i.CreatedBy,
		&i.MaxUses,
		&i.UsedCount,
		&i.ExpiresAt,
		&i.IsRevoked,
		&i.CreatedAt,
		&i.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInviteNotFound
		}
		return nil, fmt.Errorf("repository: failed to scan invite: %w", err)
	}
	return &i, nil
}

func scanInviteRows(rows interface {
	Next() bool
	Err() error
	Scan(dest ...any) error
}) ([]*models.FlatInvite, error) {
	var invites []*models.FlatInvite
	for rows.Next() {
		invite, err := scanInvite(rows)
		if err != nil {
			return nil, fmt.Errorf("repository: failed to scan invite row: %w", err)
		}
		invites = append(invites, invite)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repository: error iterating invite rows: %w", err)
	}
	return invites, nil
}

func mapInviteWriteError(err error, op string) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation
			if pgErr.ConstraintName == "uq_flat_invites_token" {
				return ErrInviteAlreadyExists
			}
		case "23503": // foreign_key_violation
			return fmt.Errorf("repository: referenced flat or user does not exist: %w", err)
		}
	}
	return fmt.Errorf("repository: failed to %s invite: %w", op, err)
}