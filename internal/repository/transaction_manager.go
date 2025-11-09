package repository

import (
	"context"
	"fmt"
	"go-server/pkg/database"
	"go-server/pkg/logger"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.uber.org/zap"
)

// TransactionManager handles database transactions
type TransactionManager interface {
	// WithTransaction executes the given function within a transaction
	// If the function returns an error, the transaction is rolled back
	// Otherwise, it is committed
	WithTransaction(ctx context.Context, fn func(txCtx context.Context) error) error
}

type transactionManager struct {
	db *database.Database
}

// NewTransactionManager creates a new transaction manager
func NewTransactionManager(db *database.Database) TransactionManager {
	return &transactionManager{db: db}
}

// txKey is the key for storing transaction in context
type txKey struct{}

// WithTransaction executes the given function within a database transaction
func (tm *transactionManager) WithTransaction(ctx context.Context, fn func(txCtx context.Context) error) error {
	// Check if we're already in a transaction
	if tx := getTxFromContext(ctx); tx != nil {
		// Nested transaction - just execute the function
		logger.Debug("Nested transaction detected, using existing transaction")
		return fn(ctx)
	}

	// Begin new transaction
	tx, err := tm.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	// Create new context with transaction
	txCtx := context.WithValue(ctx, txKey{}, tx)

	// Defer rollback in case of panic or error
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			logger.Error("Transaction rolled back due to panic", zap.Any("panic", p))
			panic(p) // Re-throw panic after rollback
		}
	}()

	// Execute function
	if err := fn(txCtx); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			logger.Error("Failed to rollback transaction", zap.Error(rbErr))
			return fmt.Errorf("transaction error: %w, rollback error: %v", err, rbErr)
		}
		logger.Debug("Transaction rolled back", zap.Error(err))
		return err
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		logger.Error("Failed to commit transaction", zap.Error(err))
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// getTxFromContext extracts transaction from context
func getTxFromContext(ctx context.Context) pgx.Tx {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}
	return nil
}
// GetExecutor returns either the transaction or the pool based on context
// This helper can be used by repositories to get the correct executor
func GetExecutor(ctx context.Context, pool *database.Database) interface {
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
	Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error)
} {
	if tx := getTxFromContext(ctx); tx != nil {
		return tx
	}
	return pool.Pool
}