package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"go-server/internal/config"
	"go-server/pkg/logger"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // CRITICAL: Register pgx driver for sql.DB
	migrate "github.com/rubenv/sql-migrate"
	"go.uber.org/zap"
)

var (
	ErrPoolNotInitialized = errors.New("database pool not initialized")
	ErrConnectionFailed   = errors.New("failed to connect to database")
)

// Database wraps the pgxpool connection pool
type Database struct {
	Pool *pgxpool.Pool
	cfg  *config.Config
}

// Config holds database-specific configuration
type Config struct {
	MaxRetries     int
	RetryDelay     time.Duration
	ConnectTimeout time.Duration
	EnableMetrics  bool
}

// DefaultConfig returns sensible production defaults
func DefaultConfig() *Config {
	return &Config{
		MaxRetries:     5,
		RetryDelay:     2 * time.Second,
		ConnectTimeout: 10 * time.Second,
		EnableMetrics:  true,
	}
}

// New creates a new Database instance with connection pooling
func New(ctx context.Context, cfg *config.Config, dbCfg *Config) (*Database, error) {
	if dbCfg == nil {
		dbCfg = DefaultConfig()
	}

	db := &Database{
		cfg: cfg,
	}

	// Attempt connection with retry logic
	var lastErr error
	for attempt := 1; attempt <= dbCfg.MaxRetries; attempt++ {
		pool, err := db.connect(ctx, dbCfg.ConnectTimeout)
		if err == nil {
			db.Pool = pool

			if dbCfg.EnableMetrics {
				db.logPoolStats()
			}

			logger.Info("✅ Connected to Database successfully",
				zap.String("host", cfg.DBHost),
				zap.String("database", cfg.DBName),
				zap.Int("max_conns", cfg.DBMaxOpenConns),
				zap.Int("min_conns", cfg.DBMaxIdleConns),
				zap.Int("attempt", attempt),
			)

			return db, nil
		}

		lastErr = err
		logger.Warn("Failed to connect to database, retrying...",
			zap.Int("attempt", attempt),
			zap.Int("max_retries", dbCfg.MaxRetries),
			zap.Error(err),
		)

		if attempt < dbCfg.MaxRetries {
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("context cancelled during retry: %w", ctx.Err())
			case <-time.After(dbCfg.RetryDelay * time.Duration(attempt)): // Exponential backoff
			}
		}
	}

	return nil, fmt.Errorf("%w after %d attempts: %v", ErrConnectionFailed, dbCfg.MaxRetries, lastErr)
}

// connect establishes a connection to the database
func (db *Database) connect(ctx context.Context, timeout time.Duration) (*pgxpool.Pool, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Build connection string without exposing password in logs
	dsn := fmt.Sprintf(
		"postgresql://%s:%s@%s:%d/%s?sslmode=%s",
		db.cfg.DBUser,
		db.cfg.DBPassword,
		db.cfg.DBHost,
		db.cfg.DBPort,
		db.cfg.DBName,
		db.cfg.DBSSLMode,
	)

	// Parse configuration
	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database config: %w", err)
	}

	// Apply connection pool settings
	poolCfg.MaxConns = int32(db.cfg.DBMaxOpenConns)
	poolCfg.MinConns = int32(db.cfg.DBMaxIdleConns)
	poolCfg.MaxConnLifetime = db.cfg.DBMaxLifetime
	poolCfg.MaxConnIdleTime = 5 * time.Minute
	poolCfg.HealthCheckPeriod = 1 * time.Minute

	// Create connection pool
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create pool: %w", err)
	}

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database ping failed: %w", err)
	}

	return pool, nil
}

// GetPool returns the connection pool or error if not initialized
func (db *Database) GetPool() (*pgxpool.Pool, error) {
	if db == nil || db.Pool == nil {
		return nil, ErrPoolNotInitialized
	}
	return db.Pool, nil
}

// Ping checks if the database is reachable
func (db *Database) Ping(ctx context.Context) error {
	if db.Pool == nil {
		return ErrPoolNotInitialized
	}
	return db.Pool.Ping(ctx)
}

// HealthCheck performs a comprehensive health check
func (db *Database) HealthCheck(ctx context.Context) error {
	if err := db.Ping(ctx); err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}

	stats := db.Pool.Stat()
	if stats.TotalConns() == 0 {
		return errors.New("no active connections in pool")
	}

	return nil
}

// Stats returns current pool statistics
func (db *Database) Stats() map[string]interface{} {
	if db.Pool == nil {
		return map[string]interface{}{"error": "pool not initialized"}
	}

	stats := db.Pool.Stat()
	return map[string]interface{}{
		"total_conns":                    stats.TotalConns(),
		"idle_conns":                     stats.IdleConns(),
		"acquired_conns":                 stats.AcquiredConns(),
		"max_conns":                      stats.MaxConns(),
		"acquired_duration_ms":           stats.AcquireDuration().Milliseconds(),
		"canceled_acquire_count":         stats.CanceledAcquireCount(),
		"constructing_conns":             stats.ConstructingConns(),
		"empty_acquire_count":            stats.EmptyAcquireCount(),
		"max_idle_destroy_count":         stats.MaxIdleDestroyCount(),
		"max_lifetime_destroy_count":     stats.MaxLifetimeDestroyCount(),
	}
}

// logPoolStats logs current pool statistics
func (db *Database) logPoolStats() {
	if db.Pool == nil {
		return
	}

	stats := db.Pool.Stat()
	logger.Info("🔧 Database pool statistics",
		zap.Int32("total_conns", stats.TotalConns()),
		zap.Int32("idle_conns", stats.IdleConns()),
		zap.Int32("acquired_conns", stats.AcquiredConns()),
		zap.Int32("max_conns", stats.MaxConns()),
	)
}

// Close gracefully closes the database connection pool
func (db *Database) Close() {
	if db.Pool != nil {
		logger.Info("Closing PostgreSQL connection pool...")
		db.logPoolStats()
		db.Pool.Close()
		db.Pool = nil
	}
}

// WithTimeout returns a context with the configured timeout
func (db *Database) WithTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, timeout)
}

// Transaction executes a function within a database transaction
func (db *Database) Transaction(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
	if db.Pool == nil {
		return ErrPoolNotInitialized
	}

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(ctx); rbErr != nil {
				logger.Error("Failed to rollback transaction",
					zap.Error(rbErr),
					zap.Error(err),
				)
			}
		}
	}()

	if err = fn(ctx, tx); err != nil {
		return err
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// RunMigrations executes all pending database migrations
func (db *Database) RunMigrations(ctx context.Context) error {
	if db.Pool == nil {
		return ErrPoolNotInitialized
	}

	// Build DSN in key=value format for pgx stdlib driver
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		db.cfg.DBHost,
		db.cfg.DBPort,
		db.cfg.DBUser,
		db.cfg.DBPassword,
		db.cfg.DBName,
		db.cfg.DBSSLMode,
	)

	// Open a standard sql.DB connection using pgx driver
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("failed to open database for migrations: %w", err)
	}
	defer sqlDB.Close()

	// Verify connection
	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to ping database for migrations: %w", err)
	}

	migrations := &migrate.FileMigrationSource{
		Dir: "./migrations",
	}

	// Run migrations up - use "postgres" dialect (compatible with pgx)
	n, err := migrate.Exec(sqlDB, "postgres", migrations, migrate.Up)
	if err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}
	

	logger.Info("Applied " + fmt.Sprint(n) + " migrations successfully")

	return nil
}

// RollbackMigrations rolls back the last migration (useful for testing)
func (db *Database) RollbackMigrations(ctx context.Context, steps int) error {
	if db.Pool == nil {
		return ErrPoolNotInitialized
	}

	// Build DSN in key=value format for pgx stdlib driver
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		db.cfg.DBHost,
		db.cfg.DBPort,
		db.cfg.DBUser,
		db.cfg.DBPassword,
		db.cfg.DBName,
		db.cfg.DBSSLMode,
	)

	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("failed to open database for rollback: %w", err)
	}
	defer sqlDB.Close()

	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to ping database for rollback: %w", err)
	}

	migrations := &migrate.FileMigrationSource{
		Dir: "./migrations",
	}

	n, err := migrate.ExecMax(sqlDB, "postgres", migrations, migrate.Down, steps)
	if err != nil {
		return fmt.Errorf("failed to rollback migrations: %w", err)
	}

	logger.Info("Rolled back migrations successfully", zap.Int("count", n))
	return nil
}