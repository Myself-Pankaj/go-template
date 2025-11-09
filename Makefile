# Detect OS for minimal cross-platform support
ifeq ($(OS),Windows_NT)
    DETECTED_OS := Windows
    AIR_CONFIG := .air.windows.toml
else
    DETECTED_OS := $(shell uname -s)
    AIR_CONFIG := .air.linux.toml
endif

# Binary output path
BIN_DIR := bin
SERVER_BIN := $(BIN_DIR)/server

.PHONY: dev prod build build-prod install test test-coverage clean watch fmt lint help run

# Help target
help:
	@echo "Available targets:"
	@echo "  dev              - Start development server"
	@echo "  prod             - Start production server"
	@echo "  build            - Build application"
	@echo "  build-prod       - Build optimized production binary"
	@echo "  install          - Install dependencies"
	@echo "  test             - Run tests"
	@echo "  test-coverage    - Run tests with coverage"
	@echo "  clean            - Clean build artifacts"
	@echo "  watch            - Run with live reload (requires air)"
	@echo "  fmt              - Format code"
	@echo "  lint             - Lint code (requires golangci-lint)"
	@echo "  run              - Alias for dev"
	@echo "  migrate-up       - Run migrations up"
	@echo "  migrate-down     - Rollback last migration"
	@echo "  migrate-create   - Create new migration file"
	@echo "  migrate-status   - Show migration status"
# Development
dev:
	@echo "Starting development server on $(DETECTED_OS)..."
	GO_ENV=development SERVER_HOST=127.0.0.1 go run cmd/server/main.go


# Production
prod:
	@echo "Starting production server on $(DETECTED_OS)..."
	GO_ENV=production SERVER_HOST=127.0.0.1 go run cmd/server/main.go


# Build
build:
	@echo "Building application for $(DETECTED_OS)..."
	@mkdir -p $(BIN_DIR) 2>/dev/null || true
	go build -o $(SERVER_BIN) cmd/server/main.go
	@echo "Binary created at: $(SERVER_BIN)"

# Build for production (optimized)
build-prod:
	@echo "Building production binary for $(DETECTED_OS)..."
	@mkdir -p $(BIN_DIR) 2>/dev/null || true
	CGO_ENABLED=0 go build -a -ldflags="-w -s" -o $(SERVER_BIN) cmd/server/main.go
	@echo "Production binary created at: $(SERVER_BIN)"

# Install dependencies
install:
	@echo "Installing dependencies..."
	go mod download
	go mod tidy

# Run tests
test:
	@echo "Running tests..."
	go test -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts on $(DETECTED_OS)..."
ifeq ($(OS),Windows_NT)
	@if exist $(BIN_DIR) rmdir /S /Q $(BIN_DIR) 2>nul || echo.
	@if exist coverage.out del /Q coverage.out 2>nul || echo.
	@if exist coverage.html del /Q coverage.html 2>nul || echo.
	@if exist tmp rmdir /S /Q tmp 2>nul || echo.
else
	@rm -rf $(BIN_DIR)
	@rm -f coverage.out coverage.html
	@rm -rf tmp
endif
	go clean
	@echo "Clean complete"

# Run with live reload (requires air)
watch:
	@echo "Starting with live reload on $(DETECTED_OS)..."
	@echo "Make sure 'air' is installed: go install github.com/cosmtrek/air@latest"

	@GO_ENV=development SERVER_HOST=127.0.0.1 air -c $(AIR_CONFIG)


# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...

# Lint code (requires golangci-lint)
lint:
	@echo "Linting code..."
	@echo "Make sure 'golangci-lint' is installed"
	golangci-lint run

# Run application (shorthand for dev)
run: dev

migrate-up:
	@echo "Running migrations..."
	sql-migrate up -env="$(GO_ENV)"

migrate-down:
	@echo "Rolling back last migration..."
	sql-migrate down -env="$(GO_ENV)" -limit=1
migrate-create:
	@read -p "Enter migration name: " name; \
	timestamp=$$(date +%Y%m%d%H%M%S); \
	filename="./migrations/$${timestamp}_$${name}.sql"; \
	echo "-- +migrate Up" > $$filename; \
	echo "" >> $$filename; \
	echo "-- +migrate Down" >> $$filename; \
	echo "Created migration: $$filename"

# Check migration status
migrate-status:
	@echo "Checking migration status..."
	sql-migrate status -env="$(GO_ENV)"