package observability

import (
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"time"
)

// LogLevel represents the severity of a log message
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
	FATAL
)

// String returns the string representation of a log level
func (l LogLevel) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	case FATAL:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// Logger provides structured logging capabilities
type Logger struct {
	level      LogLevel
	output     io.Writer
	fields     map[string]interface{}
	timeFormat string
}

// NewLogger creates a new logger
func NewLogger(level LogLevel, output io.Writer) *Logger {
	if output == nil {
		output = os.Stdout
	}

	return &Logger{
		level:      level,
		output:     output,
		fields:     make(map[string]interface{}),
		timeFormat: time.RFC3339,
	}
}

// NewDefaultLogger creates a logger with default settings
func NewDefaultLogger() *Logger {
	return NewLogger(INFO, os.Stdout)
}

// WithFields returns a new logger with additional fields
func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	newFields := make(map[string]interface{})
	for k, v := range l.fields {
		newFields[k] = v
	}
	for k, v := range fields {
		newFields[k] = v
	}

	return &Logger{
		level:      l.level,
		output:     l.output,
		fields:     newFields,
		timeFormat: l.timeFormat,
	}
}

// WithField returns a new logger with an additional field
func (l *Logger) WithField(key string, value interface{}) *Logger {
	return l.WithFields(map[string]interface{}{key: value})
}

// SetLevel sets the minimum log level
func (l *Logger) SetLevel(level LogLevel) {
	l.level = level
}

// Debug logs a debug message
func (l *Logger) Debug(msg string, fields ...map[string]interface{}) {
	l.log(DEBUG, msg, fields...)
}

// Info logs an info message
func (l *Logger) Info(msg string, fields ...map[string]interface{}) {
	l.log(INFO, msg, fields...)
}

// Warn logs a warning message
func (l *Logger) Warn(msg string, fields ...map[string]interface{}) {
	l.log(WARN, msg, fields...)
}

// Error logs an error message
func (l *Logger) Error(msg string, fields ...map[string]interface{}) {
	l.log(ERROR, msg, fields...)
}

// Fatal logs a fatal message and exits
func (l *Logger) Fatal(msg string, fields ...map[string]interface{}) {
	l.log(FATAL, msg, fields...)
	os.Exit(1)
}

// log writes a log entry
func (l *Logger) log(level LogLevel, msg string, extraFields ...map[string]interface{}) {
	if level < l.level {
		return
	}

	// Build field map
	allFields := make(map[string]interface{})
	for k, v := range l.fields {
		allFields[k] = v
	}
	for _, fields := range extraFields {
		for k, v := range fields {
			allFields[k] = v
		}
	}

	// Get caller information
	_, file, line, ok := runtime.Caller(2)
	if ok {
		allFields["file"] = fmt.Sprintf("%s:%d", file, line)
	}

	// Format log entry
	timestamp := time.Now().Format(l.timeFormat)
	entry := fmt.Sprintf("[%s] %s: %s", timestamp, level.String(), msg)

	// Add fields
	if len(allFields) > 0 {
		entry += " |"
		for k, v := range allFields {
			entry += fmt.Sprintf(" %s=%v", k, v)
		}
	}

	entry += "\n"

	// Write to output
	l.output.Write([]byte(entry))
}

// Debugf logs a formatted debug message
func (l *Logger) Debugf(format string, args ...interface{}) {
	l.Debug(fmt.Sprintf(format, args...))
}

// Infof logs a formatted info message
func (l *Logger) Infof(format string, args ...interface{}) {
	l.Info(fmt.Sprintf(format, args...))
}

// Warnf logs a formatted warning message
func (l *Logger) Warnf(format string, args ...interface{}) {
	l.Warn(fmt.Sprintf(format, args...))
}

// Errorf logs a formatted error message
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.Error(fmt.Sprintf(format, args...))
}

// Fatalf logs a formatted fatal message and exits
func (l *Logger) Fatalf(format string, args ...interface{}) {
	l.Fatal(fmt.Sprintf(format, args...))
}

// LogOperation logs the start and end of an operation
func (l *Logger) LogOperation(operation string, fn func() error) error {
	start := time.Now()
	l.Info(fmt.Sprintf("Starting operation: %s", operation))

	err := fn()

	duration := time.Since(start)
	if err != nil {
		l.Error(fmt.Sprintf("Operation failed: %s", operation), map[string]interface{}{
			"duration": duration,
			"error":    err.Error(),
		})
	} else {
		l.Info(fmt.Sprintf("Operation completed: %s", operation), map[string]interface{}{
			"duration": duration,
		})
	}

	return err
}

// LogOperationWithFields logs an operation with additional fields
func (l *Logger) LogOperationWithFields(operation string, fields map[string]interface{}, fn func() error) error {
	logger := l.WithFields(fields)
	return logger.LogOperation(operation, fn)
}

// Global logger instance
var globalLogger = NewDefaultLogger()

// SetGlobalLogger sets the global logger
func SetGlobalLogger(logger *Logger) {
	globalLogger = logger
}

// GetGlobalLogger returns the global logger
func GetGlobalLogger() *Logger {
	return globalLogger
}

// Global convenience functions

// Debug logs a debug message using the global logger
func Debug(msg string, fields ...map[string]interface{}) {
	globalLogger.Debug(msg, fields...)
}

// Info logs an info message using the global logger
func Info(msg string, fields ...map[string]interface{}) {
	globalLogger.Info(msg, fields...)
}

// Warn logs a warning message using the global logger
func Warn(msg string, fields ...map[string]interface{}) {
	globalLogger.Warn(msg, fields...)
}

// Error logs an error message using the global logger
func Error(msg string, fields ...map[string]interface{}) {
	globalLogger.Error(msg, fields...)
}

// Fatal logs a fatal message using the global logger and exits
func Fatal(msg string, fields ...map[string]interface{}) {
	globalLogger.Fatal(msg, fields...)
}

// Debugf logs a formatted debug message using the global logger
func Debugf(format string, args ...interface{}) {
	globalLogger.Debugf(format, args...)
}

// Infof logs a formatted info message using the global logger
func Infof(format string, args ...interface{}) {
	globalLogger.Infof(format, args...)
}

// Warnf logs a formatted warning message using the global logger
func Warnf(format string, args ...interface{}) {
	globalLogger.Warnf(format, args...)
}

// Errorf logs a formatted error message using the global logger
func Errorf(format string, args ...interface{}) {
	globalLogger.Errorf(format, args...)
}

// Fatalf logs a formatted fatal message using the global logger and exits
func Fatalf(format string, args ...interface{}) {
	globalLogger.Fatalf(format, args...)
}

// ParseLogLevel parses a log level string
func ParseLogLevel(level string) LogLevel {
	switch level {
	case "DEBUG", "debug":
		return DEBUG
	case "INFO", "info":
		return INFO
	case "WARN", "warn", "WARNING", "warning":
		return WARN
	case "ERROR", "error":
		return ERROR
	case "FATAL", "fatal":
		return FATAL
	default:
		log.Printf("Unknown log level '%s', defaulting to INFO", level)
		return INFO
	}
}

// AccessLogger logs HTTP/gRPC access logs
type AccessLogger struct {
	logger *Logger
}

// NewAccessLogger creates a new access logger
func NewAccessLogger(logger *Logger) *AccessLogger {
	return &AccessLogger{
		logger: logger,
	}
}

// LogAccess logs an access entry
func (al *AccessLogger) LogAccess(method, path, status string, duration time.Duration, fields map[string]interface{}) {
	allFields := map[string]interface{}{
		"method":   method,
		"path":     path,
		"status":   status,
		"duration": duration,
	}

	for k, v := range fields {
		allFields[k] = v
	}

	al.logger.Info("Access", allFields)
}
