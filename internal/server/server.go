package server

import (
	"context"
	"fmt"
	"go-server/internal/config"
	"go-server/pkg/database"
	"go-server/pkg/logger"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"go.uber.org/zap"
)
const (
	shutdownTimeout = 30 * time.Second
)
// StartServerWithGracefulShutdown starts the server and handles graceful shutdown
func StartServerWithGracefulShutdown(srv *http.Server, cfg *config.Config, db *database.Database) error {
	// Channel to listen for interrupt signals
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Channel to listen for server errors
	serverErrors := make(chan error, 1)

	// Start server in a goroutine
	go func() {
		logger.Info("🌐 Starting HTTP server",
			zap.String("address", srv.Addr),
			zap.String("environment", cfg.Environment),
		)
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("✨ Server is ready to handle requests")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrors <- fmt.Errorf("server failed to start: %w", err)
		}
	}()

	// Wait for interrupt signal or server error
	select {
	case err := <-serverErrors:
		return err
	case sig := <-quit:
		logger.Info("🛑 Shutdown signal received",
			zap.String("signal", sig.String()),
		)
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("🔄 Initiating graceful shutdown...")

		// Create shutdown context with timeout
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer shutdownCancel()

		// Shutdown steps
		logger.Info("⏳ Shutting down HTTP server...")
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("❌ HTTP server shutdown error", zap.Error(err))
			return fmt.Errorf("server forced to shutdown: %w", err)
		}
		logger.Info("✅ HTTP server stopped gracefully")

		// Close database connections
		logger.Info("⏳ Closing database connections...")
		db.Close()
		logger.Info("✅ Database connections closed")

		// Log final runtime statistics
		logShutdownStats()

		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("👋 Server shutdown completed successfully")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		return nil
	}
}

// logShutdownStats logs runtime statistics during shutdown
func logShutdownStats() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	logger.Info("📊 Runtime Statistics",
		zap.Int("goroutines", runtime.NumGoroutine()),
		zap.Uint64("alloc_mb", m.Alloc/1024/1024),
		zap.Uint64("total_alloc_mb", m.TotalAlloc/1024/1024),
		zap.Uint64("sys_mb", m.Sys/1024/1024),
		zap.Uint32("num_gc", m.NumGC),
	)
}

// InitializeLogger sets up the application logger
func InitializeLogger(cfg *config.Config) error {
	logCfg := logger.DefaultConfig()
	logCfg.Env = cfg.Environment
	logCfg.MaxSize = cfg.LogMaxSize
	logCfg.MaxBackups = cfg.LogMaxBackups
	logCfg.MaxAge = cfg.LogMaxAge
	logCfg.Compress = cfg.LogCompress
	logCfg.EnableConsole = cfg.EnableConsole

	return logger.InitLogger(logCfg)
}

func LogStartupInfo(cfg *config.Config) {
	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	logger.Info("🚀 Starting Application",
		zap.String("app_name", cfg.AppName),
		zap.String("version", cfg.Version),
		zap.String("environment", cfg.Environment),
		zap.String("go_version", runtime.Version()),
		zap.Int("cpus", runtime.NumCPU()),
		zap.Int("goroutines", runtime.NumGoroutine()),
	)
	logger.Info("📊 Configuration",
		zap.String("server_addr", cfg.GetServerAddr()),
		zap.Duration("read_timeout", cfg.ReadTimeout),
		zap.Duration("write_timeout", cfg.WriteTimeout),
		zap.Duration("idle_timeout", cfg.IdleTimeout),
		zap.Duration("jwt_expiry", cfg.JWTExpiry),
		zap.String("jwt_issuer", cfg.JWTIssuer),
	)
	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}

func InitializeDatabase(ctx context.Context, cfg *config.Config) (*database.Database, error) {
	db, err := database.New(ctx, cfg, database.DefaultConfig())
	if err != nil {
		return nil, fmt.Errorf("database connection failed: %w", err)
	}
	return db, nil
}

// RunMigrations executes database migrations
func RunMigrations(ctx context.Context, db *database.Database) error {
	logger.Info("🔄 Running database migrations...")
	
	if err := db.RunMigrations(ctx); err != nil {
		return fmt.Errorf("migration execution failed: %w", err)
	}

	logger.Info("✅ Database migrations completed successfully")
	return nil
}

