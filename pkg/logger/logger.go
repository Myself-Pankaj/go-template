package logger

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	log  *zap.Logger
	mu   sync.RWMutex
	once sync.Once
)

// Config holds logger configuration
type Config struct {
	Env            string // "dev" or "prod"
	LogDir         string // directory for log files
	MaxSize        int    // max size in MB before rotation
	MaxBackups     int    // max number of old log files
	MaxAge         int    // max days to retain old log files
	Compress       bool   // compress rotated files
	EnableConsole  bool   // enable console output (auto-enabled in dev)
	EnableCaller   bool   // include caller information
	EnableStacktrc bool   // enable stack traces on errors
}

// DefaultConfig returns sensible defaults
func DefaultConfig() Config {
	return Config{
		Env:            "production",
		LogDir:         "logs",
		MaxSize:        100,
		MaxBackups:     5,
		MaxAge:         30,
		Compress:       true,
		EnableConsole:  false,
		EnableCaller:   true,
		EnableStacktrc: true,
	}
}

// InitLogger initializes the global logger with custom config
func InitLogger(cfg Config) error {
	var initErr error
	once.Do(func() {
		initErr = initLoggerInternal(cfg)
	})
	return initErr
}

// Init initializes the logger with defaults (convenience function)
func Init(env string) error {
	cfg := DefaultConfig()
	cfg.Env = env
	return InitLogger(cfg)
}

func initLoggerInternal(cfg Config) error {
	// Normalize environment
	cfg.Env = strings.ToLower(strings.TrimSpace(cfg.Env))
	if cfg.Env == "" {
		cfg.Env = "production"
	}
	isDev := cfg.Env == "development" || cfg.Env == "dev"
	
	// Create logs directory
	if err := os.MkdirAll(cfg.LogDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Load timezone with fallback
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load Asia/Kolkata timezone: %v, using UTC\n", err)
		loc = time.UTC
	}

	// Custom time encoder
	timeEncoder := func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(t.In(loc).Format("02 Jan 2006 15:04:05"))
	}
	//Encoder for console
	consoleEncCfg := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "message",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalColorLevelEncoder,
		EncodeTime:     timeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}
	// JSON encoder for files
	fileEncCfg := zapcore.EncoderConfig{
		TimeKey:        "@timestamp",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "message",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// Console encoder with colors
	
	consoleEncCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder

	// Create encoders
	jsonEncoder := zapcore.NewJSONEncoder(fileEncCfg)
	consoleEncoder := zapcore.NewConsoleEncoder(consoleEncCfg)


	// File paths
	infoFilePath := fmt.Sprintf("%s/info.%s.log", cfg.LogDir, cfg.Env)
	errorFilePath := fmt.Sprintf("%s/error.%s.log", cfg.LogDir, cfg.Env)

	// Setup log rotation with lumberjack
	infoWriter := &lumberjack.Logger{
		Filename:   infoFilePath,
		MaxSize:    cfg.MaxSize,
		MaxBackups: cfg.MaxBackups,
		MaxAge:     cfg.MaxAge,
		Compress:   cfg.Compress,
		LocalTime:  true,
	}

	errorWriter := &lumberjack.Logger{
		Filename:   errorFilePath,
		MaxSize:    cfg.MaxSize,
		MaxBackups: cfg.MaxBackups,
		MaxAge:     cfg.MaxAge,
		Compress:   cfg.Compress,
		LocalTime:  true,
	}

	// Create write syncers
	infoWS := zapcore.AddSync(infoWriter)
	errorWS := zapcore.AddSync(errorWriter)

	// Level enablers
	infoLevel := zap.LevelEnablerFunc(func(l zapcore.Level) bool {
		if isDev {
			return l >= zapcore.DebugLevel // Show debug in dev
		}
		return l >= zapcore.InfoLevel
	})
	errorLevel := zap.LevelEnablerFunc(func(l zapcore.Level) bool {
		return l >= zapcore.ErrorLevel
	})

	// Build cores
	cores := []zapcore.Core{
		zapcore.NewCore(jsonEncoder, infoWS, infoLevel),   // All logs >= info (or debug in dev)
		zapcore.NewCore(jsonEncoder, errorWS, errorLevel), // Only errors
	}

	// Add console output in dev or if explicitly enabled
	if isDev || cfg.EnableConsole {
		cores = append(cores, zapcore.NewCore(consoleEncoder, zapcore.AddSync(os.Stdout), infoLevel))
	}

	// Build logger options
	opts := []zap.Option{}
	if cfg.EnableCaller {
		opts = append(opts, zap.AddCaller())
	}
	if cfg.EnableStacktrc {
		opts = append(opts, zap.AddStacktrace(zap.ErrorLevel))
	}

	// Create logger
	mu.Lock()
	log = zap.New(zapcore.NewTee(cores...), opts...)
	mu.Unlock()

	// Log initialization
	log.Info("Logger initialized",
		zap.String("env", cfg.Env),
		zap.String("log_dir", cfg.LogDir),
		zap.Bool("console", isDev || cfg.EnableConsole),
	)

	return nil
}

// GetLogger returns the global logger instance (safe for concurrent use)
func GetLogger() *zap.Logger {
	mu.RLock()
	defer mu.RUnlock()
	if log == nil {
		// Return a no-op logger if not initialized
		return zap.NewNop()
	}
	return log
}

// Sync flushes any buffered log entries. Call this before program exit.
func Sync() error {
	mu.RLock()
	defer mu.RUnlock()
	if log == nil {
		return nil
	}
	return log.Sync()
}

// Convenience wrapper functions with proper caller information
func Debug(msg string, fields ...zap.Field) {
	GetLogger().WithOptions(zap.AddCallerSkip(1)).Debug(msg, fields...)
}

func Info(msg string, fields ...zap.Field) {
	GetLogger().WithOptions(zap.AddCallerSkip(1)).Info(msg, fields...)
}

func Warn(msg string, fields ...zap.Field) {
	GetLogger().WithOptions(zap.AddCallerSkip(1)).Warn(msg, fields...)
}

func Error(msg string, fields ...zap.Field) {
	GetLogger().WithOptions(zap.AddCallerSkip(1)).Error(msg, fields...)
}

func Fatal(msg string, fields ...zap.Field) {
	GetLogger().WithOptions(zap.AddCallerSkip(1)).Fatal(msg, fields...)
}

func Panic(msg string, fields ...zap.Field) {
	GetLogger().WithOptions(zap.AddCallerSkip(1)).Panic(msg, fields...)
}

// With creates a child logger with preset fields
func With(fields ...zap.Field) *zap.Logger {
	return GetLogger().With(fields...)
}

// Named creates a named logger (useful for subsystems)
func Named(name string) *zap.Logger {
	return GetLogger().Named(name)
}

// Sugar returns a sugared logger for printf-style logging
func Sugar() *zap.SugaredLogger {
	return GetLogger().Sugar()
}