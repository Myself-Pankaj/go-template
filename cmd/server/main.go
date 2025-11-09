package main

import (
	"context"
	"fmt"
	"go-server/internal/app"
	"go-server/internal/config"
	"net/http"

	"go-server/internal/server"

	"time"

	"go-server/pkg/logger"
	"go-server/pkg/utils"
	"os"

	"github.com/gin-gonic/gin"
)


func main() {
	
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Application error: %v\n", err)
		os.Exit(1)
	}

}



func run() error {
	// ========================================
	// 1. Configuration Loading
	// ========================================
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}
	// ========================================
	// 2. Logger Initialization
	// ========================================
	if err := server.InitializeLogger(cfg); err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	defer logger.Sync()

	// Log application startup information
	server.LogStartupInfo(cfg)

	// ========================================
	// 3. Configure Response Utilities
	// ========================================
	utils.SetResponseConfig(utils.ResponseConfig{
		EnableDetailedErrors: cfg.IsDevelopment(),
		EnableErrorLogging:   true,
		IncludeRequestID:     true,
	})

	// ========================================
	// 4. Gin Mode Configuration
	// ========================================
	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
		
	} else {
		gin.SetMode(gin.DebugMode)
	}
	// ========================================
	// 5. Database Connection & Migration
	// ========================================
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := server.InitializeDatabase(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	defer func() {
		db.Close()
	}()
	// Run migrations
	if err := server.RunMigrations(ctx, db); err != nil {
		return fmt.Errorf("database migration failed: %w", err)
	}

	// ========================================
	// 6. Dependency Injection & Initialization
	// ========================================
	deps, err := app.InitializeDependencies(db, cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize dependencies: %w", err)
	}
	defer deps.Shutdown()
	// ========================================
	// 7. HTTP Router Setup
	// ========================================
	router := gin.New()

	// Setup middleware
	server.SetupMiddleware(router, cfg)

	// Setup routes	
	server.SetupRoutes(router, deps, cfg)
	
	if cfg.EnableHealthz {
    	server.SetupHealthCheck(router, cfg, db)
	}

	// ========================================
	// 8. HTTP Server Configuration
	// ========================================
	srv := &http.Server{
		Addr:           cfg.GetServerAddr(),
		Handler:        router,
		ReadTimeout:    cfg.ReadTimeout,
		WriteTimeout:   cfg.WriteTimeout,
		IdleTimeout:    cfg.IdleTimeout,
		MaxHeaderBytes: 1 << 20, // 1 MB
	}
	// ========================================
	// 9. Graceful Shutdown Setup
	// ========================================
	return server.StartServerWithGracefulShutdown(srv, cfg, db)
}









