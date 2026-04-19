package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
)

var (
	instance *Config
	once     sync.Once
	mu       sync.RWMutex
)

// Config holds all application configuration
type Config struct {
	// Application
	Environment string
	AppName     string
	Version     string
	Host        string
	Port        string

	// Logging
	LogLevel        string
	LogDir          string
	LogMaxSize      int // MB
	LogMaxBackups   int
	LogMaxAge       int // days
	LogCompress     bool
	EnableConsole   bool

	// Server timeouts
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration

	// CORS
	AllowOrigins     []string
	AllowMethods     []string
	AllowHeaders     []string
	AllowCredentials bool

	// Rate limiting
	RateLimitEnabled bool
	RateLimitRPS     int
	RateLimitBurst   int

	// Database
	DBHost         string
	DBPort         int
	DBUser         string
	DBPassword     string
	DBName         string
	DBSSLMode      string
	DBMaxOpenConns int
	DBMaxIdleConns int
	DBMaxLifetime  time.Duration
	AutoMigrate    bool

	// Redis (optional)
	RedisEnabled  bool
	RedisHost     string
	RedisPort     int
	RedisPassword string
	RedisDB       int

	// JWT
	JWTSecret  string
	JWTAccessTokenExpiry  time.Duration
	JWTRefreshTokenExpiry time.Duration
	JWTIssuer          string

	// Email
	SMTPHost     string
	SMTPPort     int
	SMTPUser     string
	SMTPPassword string
	SMTPFrom     string

	// Security
	EnableHTTPS      bool
	TLSCertFile      string
	TLSKeyFile       string
	TrustedProxies   []string
	EnableCSRF       bool
	CSRFSecret       string

	// Monitoring
	EnableMetrics    bool
	MetricsPort      string
	EnableHealthz    bool
	EnableProfiling  bool
}

// LoadConfig loads configuration from environment files and variables
func LoadConfig() (*Config, error) {
	var loadErr error
	once.Do(func() {
		instance, loadErr = loadConfigInternal()
	})
	return instance, loadErr
}

// GetConfig returns the singleton config instance (must call LoadConfig first)
func GetConfig() *Config {
	mu.RLock()
	defer mu.RUnlock()
	if instance == nil {
		panic("config not initialized, call LoadConfig() first")
	}
	return instance
}

// MustLoadConfig loads config or panics
func MustLoadConfig() *Config {
	cfg, err := LoadConfig()
	if err != nil {
		panic(fmt.Sprintf("Failed to load config: %v", err))
	}
	return cfg
}

func loadConfigInternal() (*Config, error) {
	// Determine environment
	env := getEnv("GO_ENV", "development")
	env = strings.ToLower(env)

	// Load appropriate .env file
	if err := loadEnvFile(env); err != nil {
		// Non-fatal, continue with system env vars
		fmt.Printf("Warning: %v\n", err)
	}

	// Build config
	config := &Config{
		// Application
		Environment: env,
		AppName:     getEnv("APP_NAME", "go-server"),
		Version:     getEnv("APP_VERSION", "1.0.0"),
		Host:        getEnv("HOST", "127.0.0.1"),
		Port:        getEnv("PORT", "8080"),

		// Logging
		LogLevel:      getEnv("LOG_LEVEL", getDefaultLogLevel(env)),
		LogDir:        getEnv("LOG_DIR", "logs"),
		LogMaxSize:    getEnvAsInt("LOG_MAX_SIZE", 100),
		LogMaxBackups: getEnvAsInt("LOG_MAX_BACKUPS", 5),
		LogMaxAge:     getEnvAsInt("LOG_MAX_AGE", 30),
		LogCompress:   getEnvAsBool("LOG_COMPRESS", true),
		EnableConsole: getEnvAsBool("LOG_CONSOLE", env != "production"),

		// Server timeouts (convert seconds to duration)
		ReadTimeout:  time.Duration(getEnvAsInt("READ_TIMEOUT", 10)) * time.Second,
		WriteTimeout: time.Duration(getEnvAsInt("WRITE_TIMEOUT", 10)) * time.Second,
		IdleTimeout:  time.Duration(getEnvAsInt("IDLE_TIMEOUT", 120)) * time.Second,

		// CORS
		AllowOrigins:     getEnvAsSlice("ALLOW_ORIGINS", []string{"*"}),
		AllowMethods:     getEnvAsSlice("ALLOW_METHODS", []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}),
		AllowHeaders:     getEnvAsSlice("ALLOW_HEADERS", []string{"Origin", "Content-Type", "Authorization"}),
		AllowCredentials: getEnvAsBool("ALLOW_CREDENTIALS", true),

		// Rate limiting
		RateLimitEnabled: getEnvAsBool("RATE_LIMIT_ENABLED", env == "production"),
		RateLimitRPS:     getEnvAsInt("RATE_LIMIT_RPS", 100),
		RateLimitBurst:   getEnvAsInt("RATE_LIMIT_BURST", 200),

		// Database
		DBHost:         getEnv("DB_HOST", "localhost"),
		DBPort:         getEnvAsInt("DB_PORT", 5432),
		DBUser:         getEnv("DB_USER", "postgres"),
		DBPassword:     getEnv("DB_PASSWORD", ""),
		DBName:         getEnv("DB_NAME", "app_db"),
		DBSSLMode:      getEnv("DB_SSL_MODE", getDefaultSSLMode(env)),
		DBMaxOpenConns: getEnvAsInt("DB_MAX_OPEN_CONNS", 25),
		DBMaxIdleConns: getEnvAsInt("DB_MAX_IDLE_CONNS", 5),
		DBMaxLifetime:  time.Duration(getEnvAsInt("DB_MAX_LIFETIME", 300)) * time.Second,
		AutoMigrate:    getEnvAsBool("AUTO_MIGRATE", env != "production"),

		// Redis
		// RedisEnabled:  getEnvAsBool("REDIS_ENABLED", false),
		// RedisHost:     getEnv("REDIS_HOST", "localhost"),
		// RedisPort:     getEnvAsInt("REDIS_PORT", 6379),
		// RedisPassword: getEnv("REDIS_PASSWORD", ""),
		// RedisDB:       getEnvAsInt("REDIS_DB", 0),

		// JWT
		JWTSecret:        getEnv("JWT_SECRET", ""),
		// JWTSecret:        getEnv("JWT_SECRET", ""),
		// JWTExpiry:        time.Duration(getEnvAsInt("JWT_EXPIRY", 3600)) * time.Hour,
		// JWTRefreshExpiry: time.Duration(getEnvAsInt("JWT_REFRESH_EXPIRY", 86400)) * time.Hour,
		JWTAccessTokenExpiry:  time.Duration(getEnvAsInt("JWT_ACCESS_TOKEN_EXPIRY", 15*60)) * time.Second,//Expire in seconds
		JWTRefreshTokenExpiry: time.Duration(getEnvAsInt("JWT_REFRESH_TOKEN_EXPIRY", 24*60*60*7)) * time.Second,//Expire in seconds
		JWTIssuer:        getEnv("JWT_ISSUER", "go-server"),

		//Email
		SMTPHost:     getEnv("SMTP_HOST", "smtp.example.com"),
		SMTPPort:     getEnvAsInt("SMTP_PORT", 587),
		SMTPUser:     getEnv("SMTP_USERNAME", "user@example.com"),
		SMTPPassword: getEnv("SMTP_PASSWORD", "password"),
		SMTPFrom:     getEnv("SMTP_FROM_EMAIL", "noreply@example.com"),	

		// Security
		// EnableHTTPS:    getEnvAsBool("ENABLE_HTTPS", env == "production"),
		// TLSCertFile:    getEnv("TLS_CERT_FILE", ""),
		// TLSKeyFile:     getEnv("TLS_KEY_FILE", ""),
		// TrustedProxies: getEnvAsSlice("TRUSTED_PROXIES", []string{}),
		// EnableCSRF:     getEnvAsBool("ENABLE_CSRF", env == "production"),
		// CSRFSecret:     getEnv("CSRF_SECRET", ""),

		// Monitoring
		EnableMetrics:   getEnvAsBool("ENABLE_METRICS", true),
		MetricsPort:     getEnv("METRICS_PORT", "9090"),
		EnableHealthz:   getEnvAsBool("ENABLE_HEALTHZ", true),
		EnableProfiling: getEnvAsBool("ENABLE_PROFILING", env != "production"),
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	fmt.Printf("✓ Configuration loaded successfully [env=%s]\n", config.Environment)

	return config, nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	// Production-specific validations
	if c.Environment == "production" {
		if c.DBPassword == "" {
			return fmt.Errorf("DB_PASSWORD is required in production")
		}
		if c.JWTSecret == ""{
			return fmt.Errorf("JWT_SECRET is required in production")
		}
		if c.EnableHTTPS && (c.TLSCertFile == "" || c.TLSKeyFile == "") {
			return fmt.Errorf("TLS_CERT_FILE and TLS_KEY_FILE required when HTTPS is enabled")
		}
		if c.AllowOrigins[0] == "*" {
			return fmt.Errorf("wildcard CORS origins not allowed in production")
		}
	}

	// General validations
	if c.Port == "" {
		return fmt.Errorf("PORT is required")
	}
	if c.DBName == "" {
		return fmt.Errorf("DB_NAME is required")
	}
	if c.ReadTimeout < 0 || c.WriteTimeout < 0 || c.IdleTimeout < 0 {
		return fmt.Errorf("timeouts cannot be negative")
	}

	return nil
}

// IsDevelopment returns true if running in development mode
func (c *Config) IsDevelopment() bool {
	return c.Environment == "development" || c.Environment == "dev"
}

// IsProduction returns true if running in production mode
func (c *Config) IsProduction() bool {
	return c.Environment == "production" || c.Environment == "prod"
}

// IsTesting returns true if running in test mode
func (c *Config) IsTesting() bool {
	return c.Environment == "test" || c.Environment == "testing"
}

// GetDSN returns the database connection string
func (c *Config) GetDSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.DBHost, c.DBPort, c.DBUser, c.DBPassword, c.DBName, c.DBSSLMode,
	)
}

// GetRedisAddr returns Redis address
func (c *Config) GetRedisAddr() string {
	return fmt.Sprintf("%s:%d", c.RedisHost, c.RedisPort)
}

// GetServerAddr returns the server address
func (c *Config) GetServerAddr() string {
	return fmt.Sprintf("%s:%s", c.Host, c.Port)
}

// ==================== Helper Functions ====================

func loadEnvFile(env string) error {
	// Try environment-specific file first
	envFile := fmt.Sprintf(".env.%s", env)
	if err := godotenv.Load(envFile); err == nil {
		fmt.Printf("✓ Loaded %s\n", envFile)
		return nil
	}

	// Try default .env file
	if err := godotenv.Load(); err == nil {
		fmt.Printf("✓ Loaded .env\n")
		return nil
	}

	return fmt.Errorf("no .env file found, using system environment variables")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	if value, err := strconv.Atoi(valueStr); err == nil {
		return value
	}
	fmt.Printf("Warning: Invalid integer value for %s, using default %d\n", key, defaultValue)
	return defaultValue
}

func getEnvAsBool(key string, defaultValue bool) bool {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	value, err := strconv.ParseBool(valueStr)
	if err != nil {
		fmt.Printf("Warning: Invalid boolean value for %s, using default %v\n", key, defaultValue)
		return defaultValue
	}
	return value
}

func getEnvAsSlice(key string, defaultValue []string) []string {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	// Split by comma and trim spaces
	values := strings.Split(valueStr, ",")
	for i, v := range values {
		values[i] = strings.TrimSpace(v)
	}
	return values
}

func getDefaultLogLevel(env string) string {
	if env == "production" {
		return "info"
	}
	return "debug"
}

func getDefaultSSLMode(env string) string {
	if env == "production" {
		return "require"
	}
	return "disable"
}