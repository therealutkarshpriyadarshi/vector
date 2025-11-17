package observability

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestLogger_New(t *testing.T) {
	logger := NewLogger(INFO, nil)
	if logger == nil {
		t.Fatal("Expected logger to be created")
	}

	if logger.level != INFO {
		t.Errorf("Expected log level INFO, got %v", logger.level)
	}
}

func TestLogger_WithFields(t *testing.T) {
	logger := NewLogger(INFO, nil)
	fields := map[string]interface{}{
		"key1": "value1",
		"key2": 123,
	}

	newLogger := logger.WithFields(fields)

	if len(newLogger.fields) != 2 {
		t.Errorf("Expected 2 fields, got %d", len(newLogger.fields))
	}
}

func TestLogger_WithField(t *testing.T) {
	logger := NewLogger(INFO, nil)
	newLogger := logger.WithField("test", "value")

	if len(newLogger.fields) != 1 {
		t.Errorf("Expected 1 field, got %d", len(newLogger.fields))
	}

	if newLogger.fields["test"] != "value" {
		t.Errorf("Expected field 'test' to be 'value', got %v", newLogger.fields["test"])
	}
}

func TestLogger_Info(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(INFO, &buf)

	logger.Info("test message")

	output := buf.String()
	if !strings.Contains(output, "INFO") {
		t.Error("Expected log to contain 'INFO'")
	}
	if !strings.Contains(output, "test message") {
		t.Error("Expected log to contain 'test message'")
	}
}

func TestLogger_Debug(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(DEBUG, &buf)

	logger.Debug("debug message")

	output := buf.String()
	if !strings.Contains(output, "DEBUG") {
		t.Error("Expected log to contain 'DEBUG'")
	}
	if !strings.Contains(output, "debug message") {
		t.Error("Expected log to contain 'debug message'")
	}
}

func TestLogger_DebugFiltered(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(INFO, &buf) // INFO level should filter DEBUG

	logger.Debug("debug message")

	output := buf.String()
	if output != "" {
		t.Errorf("Expected no output for DEBUG when level is INFO, got: %s", output)
	}
}

func TestLogger_Warn(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(WARN, &buf)

	logger.Warn("warning message")

	output := buf.String()
	if !strings.Contains(output, "WARN") {
		t.Error("Expected log to contain 'WARN'")
	}
}

func TestLogger_Error(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(ERROR, &buf)

	logger.Error("error message")

	output := buf.String()
	if !strings.Contains(output, "ERROR") {
		t.Error("Expected log to contain 'ERROR'")
	}
}

func TestLogger_InfoWithFields(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(INFO, &buf)

	logger.Info("test", map[string]interface{}{
		"key1": "value1",
		"key2": 123,
	})

	output := buf.String()
	if !strings.Contains(output, "key1=value1") {
		t.Error("Expected log to contain 'key1=value1'")
	}
	if !strings.Contains(output, "key2=123") {
		t.Error("Expected log to contain 'key2=123'")
	}
}

func TestLogger_Infof(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(INFO, &buf)

	logger.Infof("formatted %s %d", "message", 123)

	output := buf.String()
	if !strings.Contains(output, "formatted message 123") {
		t.Error("Expected log to contain formatted message")
	}
}

func TestLogger_Debugf(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(DEBUG, &buf)

	logger.Debugf("debug %d", 42)

	output := buf.String()
	if !strings.Contains(output, "debug 42") {
		t.Error("Expected log to contain 'debug 42'")
	}
}

func TestLogger_LogOperation_Success(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(INFO, &buf)

	err := logger.LogOperation("test_operation", func() error {
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Starting operation: test_operation") {
		t.Error("Expected log to contain 'Starting operation'")
	}
	if !strings.Contains(output, "Operation completed: test_operation") {
		t.Error("Expected log to contain 'Operation completed'")
	}
}

func TestLogger_LogOperation_Failure(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(INFO, &buf)

	testErr := errors.New("test error")
	err := logger.LogOperation("test_operation", func() error {
		return testErr
	})

	if err != testErr {
		t.Errorf("Expected error to be returned, got %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Operation failed: test_operation") {
		t.Error("Expected log to contain 'Operation failed'")
	}
}

func TestLogger_SetLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(INFO, &buf)

	logger.SetLevel(WARN)

	logger.Info("should not appear")
	if buf.String() != "" {
		t.Error("Expected INFO message to be filtered")
	}

	logger.Warn("should appear")
	if !strings.Contains(buf.String(), "should appear") {
		t.Error("Expected WARN message to appear")
	}
}

func TestLogLevel_String(t *testing.T) {
	tests := []struct {
		level    LogLevel
		expected string
	}{
		{DEBUG, "DEBUG"},
		{INFO, "INFO"},
		{WARN, "WARN"},
		{ERROR, "ERROR"},
		{FATAL, "FATAL"},
	}

	for _, tt := range tests {
		if tt.level.String() != tt.expected {
			t.Errorf("Expected %s, got %s", tt.expected, tt.level.String())
		}
	}
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected LogLevel
	}{
		{"DEBUG", DEBUG},
		{"debug", DEBUG},
		{"INFO", INFO},
		{"info", INFO},
		{"WARN", WARN},
		{"warn", WARN},
		{"WARNING", WARN},
		{"ERROR", ERROR},
		{"error", ERROR},
		{"FATAL", FATAL},
		{"fatal", FATAL},
		{"unknown", INFO}, // Default
	}

	for _, tt := range tests {
		result := ParseLogLevel(tt.input)
		if result != tt.expected {
			t.Errorf("ParseLogLevel(%s): expected %v, got %v", tt.input, tt.expected, result)
		}
	}
}

func TestGlobalLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(INFO, &buf)
	SetGlobalLogger(logger)

	Info("global test")

	output := buf.String()
	if !strings.Contains(output, "global test") {
		t.Error("Expected global logger to log message")
	}
}

func TestAccessLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(INFO, &buf)
	accessLogger := NewAccessLogger(logger)

	accessLogger.LogAccess("GET", "/api/search", "200", 0, map[string]interface{}{
		"user": "test",
	})

	output := buf.String()
	if !strings.Contains(output, "Access") {
		t.Error("Expected log to contain 'Access'")
	}
	if !strings.Contains(output, "method=GET") {
		t.Error("Expected log to contain 'method=GET'")
	}
	if !strings.Contains(output, "user=test") {
		t.Error("Expected log to contain 'user=test'")
	}
}

func TestLogger_LogOperationWithFields(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(INFO, &buf)

	fields := map[string]interface{}{
		"request_id": "12345",
	}

	err := logger.LogOperationWithFields("test_op", fields, func() error {
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "request_id=12345") {
		t.Error("Expected log to contain request_id field")
	}
}
