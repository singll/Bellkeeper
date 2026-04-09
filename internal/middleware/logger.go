package middleware

import (
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	defaultLogger *zap.Logger
	loggerMu      sync.RWMutex
	currentLevel  zapcore.Level
)

// InitLogger initializes the global logger. Call this during application startup.
func InitLogger(level string) error {
	lvl := parseLevel(level)
	currentLevel = lvl

	var cfg zap.Config
	if level == "debug" {
		cfg = zap.NewDevelopmentConfig()
	} else {
		cfg = zap.NewProductionConfig()
	}

	logger, err := cfg.Build()
	if err != nil {
		return err
	}

	loggerMu.Lock()
	defaultLogger = logger
	currentLevel = lvl
	loggerMu.Unlock()

	return nil
}

// parseLevel converts a string level to zapcore.Level
func parseLevel(level string) zapcore.Level {
	switch level {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}

// GetLogger returns the global logger instance.
func GetLogger() *zap.Logger {
	loggerMu.RLock()
	defer loggerMu.RUnlock()
	if defaultLogger == nil {
		// Fallback to production logger if not initialized
		defaultLogger, _ = zap.NewProduction()
	}
	return defaultLogger
}

// GetLogLevel returns the current log level
func GetLogLevel() string {
	loggerMu.RLock()
	defer loggerMu.RUnlock()
	switch currentLevel {
	case zapcore.DebugLevel:
		return "debug"
	case zapcore.InfoLevel:
		return "info"
	case zapcore.WarnLevel:
		return "warn"
	case zapcore.ErrorLevel:
		return "error"
	default:
		return "info"
	}
}

// SetLogLevel dynamically changes the log level at runtime
func SetLogLevel(level string) error {
	newLevel := parseLevel(level)

	loggerMu.Lock()
	defer loggerMu.Unlock()

	// Create new logger with new level
	var cfg zap.Config
	if level == "debug" {
		cfg = zap.NewDevelopmentConfig()
		cfg.Level = zap.NewAtomicLevelAt(newLevel)
	} else {
		cfg = zap.NewProductionConfig()
		cfg.Level = zap.NewAtomicLevelAt(newLevel)
	}

	newLogger, err := cfg.Build()
	if err != nil {
		return err
	}

	defaultLogger = newLogger
	currentLevel = newLevel
	return nil
}

// Logger middleware logs HTTP requests
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		if query != "" {
			path = path + "?" + query
		}

		logger := GetLogger()
		logger.Info("request",
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.Int("status", status),
			zap.Duration("latency", latency),
			zap.String("ip", c.ClientIP()),
			zap.String("user-agent", c.Request.UserAgent()),
		)
	}
}
