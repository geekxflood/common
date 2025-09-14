// Package logging provides structured logging capabilities using Go's standard log/slog package.
package logging

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestLogging(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Logging Suite")
}

var _ = Describe("ValidateLevel", func() {
	DescribeTable("validating log levels",
		func(level string, valid bool) {
			result := ValidateLevel(level)
			Expect(result).To(Equal(valid), "ValidateLevel(%q) = %v, want %v", level, result, valid)
		},
		Entry("debug level", "debug", true),
		Entry("info level", "info", true),
		Entry("warn level", "warn", true),
		Entry("error level", "error", true),
		Entry("DEBUG level (case insensitive)", "DEBUG", true),
		Entry("INFO level (case insensitive)", "INFO", true),
		Entry("invalid level", "invalid", false),
		Entry("empty level", "", false),
	)
})

var _ = Describe("ValidateFormat", func() {
	DescribeTable("validating log formats",
		func(format string, valid bool) {
			result := ValidateFormat(format)
			Expect(result).To(Equal(valid), "ValidateFormat(%q) = %v, want %v", format, result, valid)
		},
		Entry("logfmt format", "logfmt", true),
		Entry("json format", "json", true),
		Entry("LOGFMT format (case insensitive)", "LOGFMT", true),
		Entry("JSON format (case insensitive)", "JSON", true),
		Entry("invalid format", "invalid", false),
		Entry("empty format", "", false),
	)
})

var _ = Describe("parseLevel", func() {
	DescribeTable("parsing log levels",
		func(input string, expected slog.Level) {
			result := parseLevel(input)
			Expect(result).To(Equal(expected), "parseLevel(%q) = %v, want %v", input, result, expected)
		},
		Entry("debug level", "debug", slog.LevelDebug),
		Entry("info level", "info", slog.LevelInfo),
		Entry("warn level", "warn", slog.LevelWarn),
		Entry("warning alias", "warning", slog.LevelWarn),
		Entry("error level", "error", slog.LevelError),
		Entry("DEBUG level (case insensitive)", "DEBUG", slog.LevelDebug),
		Entry("invalid level (fallback)", "invalid", slog.LevelInfo),
		Entry("empty level (fallback)", "", slog.LevelInfo),
	)
})

func TestComponentLoggerWithNilLogger(t *testing.T) {
	// Create a ComponentLogger with nil internal logger
	cl := &ComponentLogger{
		logger:        nil,
		component:     "test-component",
		componentType: "test-type",
	}

	// Test With method - should not panic and should create a working logger
	newCl := cl.With("key", "value")
	if newCl == nil {
		t.Fatal("With() returned nil")
	}

	// Since With() now returns Logger interface, we need to cast back to test internal fields
	if concreteLogger, ok := newCl.(*ComponentLogger); ok {
		if concreteLogger.component != "test-component" {
			t.Errorf("With() component = %q, want %q", concreteLogger.component, "test-component")
		}
		if concreteLogger.componentType != "test-type" {
			t.Errorf("With() componentType = %q, want %q", concreteLogger.componentType, "test-type")
		}
	} else {
		t.Error("With() should return *ComponentLogger")
	}

	// Test that we can call methods without panic
	newCl.Info("test message")
	newCl.Debug("debug message")
	newCl.Warn("warn message")
	newCl.Error("error message")
}

func TestComponentLoggerContextMethodsWithNilLogger(t *testing.T) {
	// Create a ComponentLogger with nil internal logger
	cl := &ComponentLogger{
		logger:        nil,
		component:     "test-component",
		componentType: "test-type",
	}

	ctx := context.Background()

	// Test all context methods - should not panic and should fall back to global logger
	cl.DebugContext(ctx, "debug message")
	cl.InfoContext(ctx, "info message")
	cl.WarnContext(ctx, "warn message")
	cl.ErrorContext(ctx, "error message")

	// If we get here without panic, the test passes
}

func TestComponentLoggerRegularMethodsWithNilLogger(t *testing.T) {
	// Create a ComponentLogger with nil internal logger
	cl := &ComponentLogger{
		logger:        nil,
		component:     "test-component",
		componentType: "test-type",
	}

	// These methods should handle nil logger gracefully (they already have nil checks)
	// Just verify they don't panic
	cl.Debug("debug message")
	cl.Info("info message")
	cl.Warn("warn message")
	cl.Error("error message")

	// No assertion needed - if we get here without panic, the test passes
}

func TestNewComponentLogger(t *testing.T) {
	// Ensure we have a global logger
	_ = InitWithDefaults()

	cl := NewComponentLogger("test-comp", "test-type")
	if cl == nil {
		t.Fatal("NewComponentLogger returned nil")
	}
	if cl.logger == nil {
		t.Fatal("NewComponentLogger created ComponentLogger with nil logger")
	}
	if cl.component != "test-comp" {
		t.Errorf("component = %q, want %q", cl.component, "test-comp")
	}
	if cl.componentType != "test-type" {
		t.Errorf("componentType = %q, want %q", cl.componentType, "test-type")
	}
}

func TestContextExtraction(t *testing.T) {
	tests := []struct {
		name     string
		setupCtx func() context.Context
		expected map[string]any
	}{
		{
			name: "empty context",
			setupCtx: func() context.Context {
				return context.Background()
			},
			expected: map[string]any{},
		},
		{
			name: "context with request_id",
			setupCtx: func() context.Context {
				return context.WithValue(context.Background(), "request_id", "req-123")
			},
			expected: map[string]any{"request_id": "req-123"},
		},
		{
			name: "context with multiple fields",
			setupCtx: func() context.Context {
				ctx := context.Background()
				ctx = context.WithValue(ctx, "request_id", "req-123")
				ctx = context.WithValue(ctx, "user_id", "user-456")
				return ctx
			},
			expected: map[string]any{"request_id": "req-123", "user_id": "user-456"},
		},
		{
			name: "context with non-string value",
			setupCtx: func() context.Context {
				return context.WithValue(context.Background(), "request_id", 123) // int, not string
			},
			expected: map[string]any{}, // should be ignored
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.setupCtx()
			fields := extractContextFields(ctx)

			if len(fields) != len(tt.expected) {
				t.Errorf("extractContextFields() returned %d fields, want %d", len(fields), len(tt.expected))
			}

			for k, v := range tt.expected {
				if got, ok := fields[k]; !ok || got != v {
					t.Errorf("extractContextFields()[%q] = %v, want %v", k, got, v)
				}
			}
		})
	}
}

func TestNew(t *testing.T) {
	tests := []struct {
		name   string
		config Config
		hasErr bool
	}{
		{
			name: "valid config stdout",
			config: Config{
				Level:  "info",
				Format: "logfmt",
				Output: "stdout",
			},
			hasErr: false,
		},
		{
			name: "valid config stderr",
			config: Config{
				Level:  "debug",
				Format: "json",
				Output: "stderr",
			},
			hasErr: false,
		},
		{
			name: "invalid level",
			config: Config{
				Level:  "invalid",
				Format: "logfmt",
				Output: "stdout",
			},
			hasErr: true,
		},
		{
			name: "invalid format",
			config: Config{
				Level:  "info",
				Format: "invalid",
				Output: "stdout",
			},
			hasErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, closer, err := New(tt.config)

			if tt.hasErr {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if logger == nil {
				t.Error("Expected logger but got nil")
			}

			// closer can be nil for stdout/stderr
			if tt.config.Output != "stdout" && tt.config.Output != "stderr" && closer == nil {
				t.Error("Expected closer for file output but got nil")
			}

			if closer != nil {
				closer.Close() // cleanup
			}
		})
	}
}

func TestInitAndShutdown(t *testing.T) {
	// Save original state
	origLogger := globalLogger
	origCloser := globalCloser
	origLevelVar := globalLevelVar
	defer func() {
		globalLogger = origLogger
		globalCloser = origCloser
		globalLevelVar = origLevelVar
	}()

	config := Config{
		Level:  "debug",
		Format: "json",
		Output: "stdout",
	}

	err := Init(config)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	if globalLogger == nil {
		t.Error("globalLogger should not be nil after Init")
	}
	if globalLevelVar == nil {
		t.Error("globalLevelVar should not be nil after Init")
	}

	// Test Shutdown
	err = Shutdown()
	if err != nil {
		t.Errorf("Shutdown failed: %v", err)
	}
}

func TestSetLevel(t *testing.T) {
	// Save original state
	origLevelVar := globalLevelVar
	defer func() {
		globalLevelVar = origLevelVar
	}()

	// Initialize with a level var
	globalLevelVar = &slog.LevelVar{}
	globalLevelVar.Set(slog.LevelInfo)

	tests := []struct {
		level   string
		wantErr bool
	}{
		{"debug", false},
		{"info", false},
		{"warn", false},
		{"error", false},
		{"invalid", true},
		{"", true},
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			err := SetLevel(tt.level)
			if (err != nil) != tt.wantErr {
				t.Errorf("SetLevel(%q) error = %v, wantErr %v", tt.level, err, tt.wantErr)
			}
		})
	}
}

// Example tests demonstrating usage patterns

// Interface compliance tests
func TestLoggerInterface(t *testing.T) {
	// Test that ComponentLogger implements Logger interface
	var _ Logger = (*ComponentLogger)(nil)

	// Test that globalLoggerWrapper implements Logger interface
	var _ Logger = (*globalLoggerWrapper)(nil)

	// Test that slogWrapper implements Logger interface
	var _ Logger = (*slogWrapper)(nil)
}

func TestGetLogger(t *testing.T) {
	_ = InitWithDefaults()
	defer Shutdown()

	logger := GetLogger()
	if logger == nil {
		t.Fatal("GetLogger() returned nil")
	}

	// Test that it implements the Logger interface
	var _ Logger = logger

	// Test basic functionality (should not panic)
	logger.Info("test message")
	logger.Debug("debug message")
	logger.Warn("warn message")
	logger.Error("error message")

	// Test context methods
	ctx := context.Background()
	logger.InfoContext(ctx, "context message")
	logger.DebugContext(ctx, "debug context message")
	logger.WarnContext(ctx, "warn context message")
	logger.ErrorContext(ctx, "error context message")

	// Test With method
	childLogger := logger.With("key", "value")
	if childLogger == nil {
		t.Fatal("With() returned nil")
	}
	childLogger.Info("child logger message")
}

func TestNewLogger(t *testing.T) {
	config := Config{
		Level:  "info",
		Format: "json",
		Output: "stdout",
	}

	logger, closer, err := NewLogger(config)
	if err != nil {
		t.Fatalf("NewLogger failed: %v", err)
	}
	if logger == nil {
		t.Fatal("NewLogger returned nil logger")
	}
	if closer != nil {
		defer closer.Close()
	}

	// Test that it implements the Logger interface
	var _ Logger = logger

	// Test basic functionality
	logger.Info("test message from NewLogger")

	// Test With method
	childLogger := logger.With("service", "test")
	childLogger.Info("child message")
}

func TestComponentLoggerInterface(t *testing.T) {
	_ = InitWithDefaults()
	defer Shutdown()

	cl := NewComponentLogger("test-component", "service")

	// Test that ComponentLogger implements Logger interface
	var logger Logger = cl

	// Test interface methods
	logger.Info("interface test message")
	logger.Debug("interface debug message")
	logger.Warn("interface warn message")
	logger.Error("interface error message")

	ctx := context.Background()
	logger.InfoContext(ctx, "interface context message")

	// Test With method returns Logger interface
	childLogger := logger.With("request_id", "123")
	if childLogger == nil {
		t.Fatal("With() returned nil")
	}
	childLogger.Info("child interface message")
}

// Additional tests for uncovered functions to reach 90% coverage
var _ = Describe("Coverage Improvements", func() {
	var tempDir string

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "logging_coverage_test")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if tempDir != "" {
			os.RemoveAll(tempDir)
		}
	})

	Describe("ComponentLogger Additional Methods", func() {
		var cl *ComponentLogger

		BeforeEach(func() {
			// Initialize global logger first
			_ = InitWithDefaults()
			cl = NewComponentLogger("test-component", "test-type")
		})

		AfterEach(func() {
			Shutdown()
		})

		It("should return component name", func() {
			component := cl.GetComponent()
			Expect(component).To(Equal("test-component"))
		})

		It("should return component type", func() {
			componentType := cl.GetComponentType()
			Expect(componentType).To(Equal("test-type"))
		})
	})

	Describe("File Logging", func() {
		It("should handle file output configuration", func() {
			logFile := filepath.Join(tempDir, "test.log")

			config := Config{
				Level:  "info",
				Format: "json",
				Output: logFile,
			}

			logger, closer, err := New(config)
			Expect(err).NotTo(HaveOccurred())
			Expect(logger).NotTo(BeNil())
			Expect(closer).NotTo(BeNil())

			// Test logging to file
			logger.Info("test message to file")

			// Clean up
			closer.Close()

			// Verify file was created
			_, err = os.Stat(logFile)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle invalid file path", func() {
			invalidPath := "/invalid/path/that/does/not/exist/test.log"

			config := Config{
				Level:  "info",
				Format: "json",
				Output: invalidPath,
			}

			_, _, err := New(config)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Global Logger Wrapper Methods", func() {
		BeforeEach(func() {
			_ = InitWithDefaults()
		})

		AfterEach(func() {
			Shutdown()
		})

		It("should handle Debug method", func() {
			logger := GetLogger()

			// This should call the Debug method on globalLoggerWrapper
			logger.Debug("debug message")

			// No assertion needed - if we get here without panic, the test passes
		})

		It("should handle Warn method", func() {
			logger := GetLogger()

			// This should call the Warn method on globalLoggerWrapper
			logger.Warn("warn message")

			// No assertion needed - if we get here without panic, the test passes
		})

		It("should handle Error method", func() {
			logger := GetLogger()

			// This should call the Error method on globalLoggerWrapper
			logger.Error("error message")

			// No assertion needed - if we get here without panic, the test passes
		})

		It("should handle DebugContext method", func() {
			logger := GetLogger()
			ctx := context.Background()

			// This should call the DebugContext method on globalLoggerWrapper
			logger.DebugContext(ctx, "debug context message")

			// No assertion needed - if we get here without panic, the test passes
		})

		It("should handle InfoContext method", func() {
			logger := GetLogger()
			ctx := context.Background()

			// This should call the InfoContext method on globalLoggerWrapper
			logger.InfoContext(ctx, "info context message")

			// No assertion needed - if we get here without panic, the test passes
		})

		It("should handle WarnContext method", func() {
			logger := GetLogger()
			ctx := context.Background()

			// This should call the WarnContext method on globalLoggerWrapper
			logger.WarnContext(ctx, "warn context message")

			// No assertion needed - if we get here without panic, the test passes
		})

		It("should handle ErrorContext method", func() {
			logger := GetLogger()
			ctx := context.Background()

			// This should call the ErrorContext method on globalLoggerWrapper
			logger.ErrorContext(ctx, "error context message")

			// No assertion needed - if we get here without panic, the test passes
		})
	})
})
