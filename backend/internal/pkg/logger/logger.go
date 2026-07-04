// Package logger wraps zap for structured logging. This is the Dexiask
// (open-source) build: it is a plain zap wrapper with no OpenTelemetry bridge.
package logger

import (
	"context"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger wraps zap.Logger for structured logging.
type Logger struct {
	*zap.Logger
}

// Config holds logger configuration.
type Config struct {
	Level string
	Env   string
}

// New creates a new logger instance.
func New(cfg Config) (*Logger, error) {
	var zapCfg zap.Config
	if cfg.Env == "production" {
		zapCfg = zap.NewProductionConfig()
	} else {
		zapCfg = zap.NewDevelopmentConfig()
		zapCfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	level, err := zapcore.ParseLevel(cfg.Level)
	if err != nil {
		level = zapcore.InfoLevel
	}
	zapCfg.Level = zap.NewAtomicLevelAt(level)
	zapCfg.OutputPaths = []string{"stdout"}

	zapLogger, err := zapCfg.Build(
		zap.AddCallerSkip(1),
		zap.AddStacktrace(zapcore.ErrorLevel),
	)
	if err != nil {
		return nil, err
	}
	return &Logger{zapLogger}, nil
}

// NewDefault creates a default development logger.
func NewDefault() *Logger {
	l, _ := New(Config{Level: "info", Env: "development"})
	return l
}

// NewNop creates a no-op logger for testing.
func NewNop() *Logger { return &Logger{zap.NewNop()} }

// Ctx returns this logger. The context hook is preserved so call sites can use
// `logger.Ctx(ctx)` symmetrically; in the
// open-source build there is no trace enrichment.
func (l *Logger) Ctx(_ context.Context) *Logger { return l }

// WithFields returns a logger with additional fields.
func (l *Logger) WithFields(fields ...zap.Field) *Logger {
	return &Logger{l.Logger.With(fields...)}
}

func (l *Logger) Debug(msg string, fields ...zap.Field) { l.Logger.Debug(msg, fields...) }
func (l *Logger) Info(msg string, fields ...zap.Field)  { l.Logger.Info(msg, fields...) }
func (l *Logger) Warn(msg string, fields ...zap.Field)  { l.Logger.Warn(msg, fields...) }
func (l *Logger) Error(msg string, fields ...zap.Field) { l.Logger.Error(msg, fields...) }
func (l *Logger) Fatal(msg string, fields ...zap.Field) { l.Logger.Fatal(msg, fields...) }

// Sync flushes buffered log entries.
func (l *Logger) Sync() error { return l.Logger.Sync() }

// GetEnv returns the environment variable value or a default.
func GetEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
