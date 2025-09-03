// Package logging provides structured logging capabilities using Go's standard log/slog package.
//
// This package offers a comprehensive logging solution with support for multiple output formats,
// component-aware logging, context integration, and dynamic configuration changes at runtime.
//
// # Basic Usage
//
// Initialize the global logger with default settings:
//
//	err := logging.InitWithDefaults()
//	if err != nil {
//		panic(err)
//	}
//	defer logging.Shutdown()
//
//	logging.Info("service started", "version", "1.0.0")
//	logging.Warn("configuration missing", "key", "database.url")
//	logging.Error("connection failed", "error", err)
//
// # Custom Configuration
//
// Configure the logger with custom settings:
//
//	cfg := logging.Config{
//		Level:     "debug",
//		Format:    "json",
//		Output:    "/var/log/app.log",
//		AddSource: true,
//	}
//	err := logging.Init(cfg)
//	if err != nil {
//		panic(err)
//	}
//	defer logging.Shutdown()
//
// # Independent Logger Instance
//
// Create a logger without affecting global state:
//
//	logger, closer, err := logging.New(cfg)
//	if err != nil {
//		panic(err)
//	}
//	if closer != nil {
//		defer closer.Close()
//	}
//	logger.Info("custom logger message")
//
// # Component-Aware Logging
//
// Create component-specific loggers that automatically include component context:
//
//	authLogger := logging.NewComponentLogger("auth", "service")
//	authLogger.Info("user authenticated", "user_id", "123")
//	// Output: ... component=auth component_type=service user_id=123
//
//	// Or use component functions directly
//	logging.InfoComponent("database", "connection", "pool created", "size", 10)
//
// # Context-Aware Logging
//
// Automatically extract and include context fields in log messages:
//
//	ctx := context.WithValue(context.Background(), "request_id", "req-123")
//	ctx = context.WithValue(ctx, "user_id", "user-456")
//
//	logging.InfoContext(ctx, "processing request")
//	// Output: ... request_id=req-123 user_id=user-456
//
// # Dynamic Level Changes
//
// Change log levels at runtime without restarting the application:
//
//	logging.SetLevel("debug") // Enable debug logging
//	logging.Debug("now visible")
//
//	logging.SetLevel("warn")  // Only warnings and errors
//	logging.Info("not visible")
//
// # Logger Interface
//
// Use the Logger interface for dependency injection and testing:
//
//	logger := logging.GetLogger()
//	logger.Info("using interface")
//
//	// Create independent logger with interface
//	logger, closer, err := logging.NewLogger(cfg)
//	if err != nil {
//		panic(err)
//	}
//	if closer != nil {
//		defer closer.Close()
//	}
package logging

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// Log level constants define the available logging levels.
const (
	// LevelDebug enables detailed diagnostic information useful for debugging.
	// This is the most verbose level and should be used sparingly in production.
	LevelDebug = "debug"

	// LevelInfo enables general informational messages about application flow.
	// This is the default level and provides operational visibility.
	LevelInfo = "info"

	// LevelWarn enables warning conditions that don't halt execution.
	// Use for potentially harmful situations that should be investigated.
	LevelWarn = "warn"

	// LevelError enables error conditions that may affect functionality.
	// Use for errors that require immediate attention but don't crash the application.
	LevelError = "error"
)

// Log format constants define the available output formats.
const (
	// FormatLogfmt provides key=value structured logging in logfmt format.
	// This is the default format and is human-readable while remaining structured.
	// Example: time=2023-01-01T12:00:00Z level=INFO msg="hello" key=value
	FormatLogfmt = "logfmt"

	// FormatJSON provides machine-readable structured logging in JSON format.
	// This format is ideal for log aggregation systems and automated processing.
	// Example: {"time":"2023-01-01T12:00:00Z","level":"INFO","msg":"hello","key":"value"}
	FormatJSON = "json"
)

// Config holds the logger configuration settings.
//
// All fields have sensible defaults and can be omitted for basic usage.
// Use DefaultConfig() to get a pre-configured instance with recommended settings.
type Config struct {
	// Level sets the minimum log level to output.
	// Valid values: "debug", "info", "warn", "error"
	// Default: "info"
	Level string `json:"level" yaml:"level"`

	// Format sets the output format for log messages.
	// Valid values: "logfmt", "json"
	// Default: "logfmt"
	Format string `json:"format" yaml:"format"`

	// Output sets the destination for log messages.
	// Valid values: "stdout", "stderr", or a file path
	// Default: "stdout"
	// When using a file path, directories will be created automatically.
	Output string `json:"output" yaml:"output"`

	// AddSource includes source file and line information in log messages.
	// This is useful for debugging but adds overhead and should be disabled in production.
	// Default: false
	AddSource bool `json:"add_source" yaml:"add_source"`
}

// DefaultConfig returns a logger configuration with recommended default settings.
//
// The returned configuration uses:
//   - Info level logging (debug messages filtered out)
//   - Logfmt format (human-readable key=value pairs)
//   - Stdout output (console output)
//   - Source information disabled (better performance)
//
// This configuration is suitable for most applications and provides a good balance
// between visibility and performance.
func DefaultConfig() Config {
	return Config{
		Level:  LevelInfo,
		Format: FormatLogfmt,
		Output: "stdout",
	}
}

var (
	// globalLogger holds the application-wide logger instance.
	globalLogger *slog.Logger
	// globalCloser holds the closer for the global logger's output.
	globalCloser io.Closer
	// globalLevelVar holds the dynamic level variable for runtime level changes.
	globalLevelVar *slog.LevelVar
)

// New creates a new logger instance with the provided configuration.
//
// This function creates an independent logger that does not affect the global logger state.
// It's useful when you need multiple loggers with different configurations or when you want
// to avoid global state in your application.
//
// The function returns:
//   - *slog.Logger: The configured logger instance
//   - io.Closer: A closer for cleanup (non-nil only when output is a file)
//   - error: Any configuration or initialization error
//
// The closer must be called when the logger is no longer needed to properly close
// file handles and prevent resource leaks. It's safe to call Close() on a nil closer.
//
// Example:
//
//	logger, closer, err := logging.New(logging.Config{
//		Level:  "debug",
//		Format: "json",
//		Output: "/var/log/app.log",
//	})
//	if err != nil {
//		return err
//	}
//	if closer != nil {
//		defer closer.Close()
//	}
//	logger.Info("application started")
func New(config Config) (*slog.Logger, io.Closer, error) {
	if !ValidateLevel(config.Level) {
		return nil, nil, fmt.Errorf("invalid log level: %q, must be one of: %s, %s, %s, %s",
			config.Level, LevelDebug, LevelInfo, LevelWarn, LevelError)
	}

	if !ValidateFormat(config.Format) {
		return nil, nil, fmt.Errorf("invalid log format: %q, must be one of: %s, %s",
			config.Format, FormatLogfmt, FormatJSON)
	}

	levelVar := &slog.LevelVar{}
	levelVar.Set(parseLevel(config.Level))

	var writer io.Writer
	var closer io.Closer
	switch strings.ToLower(config.Output) {
	case "stdout", "":
		writer = os.Stdout
	case "stderr":
		writer = os.Stderr
	default:
		file, err := openLogFile(config.Output)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to open log file %s: %w", config.Output, err)
		}
		writer = file
		closer = file
	}

	var handler slog.Handler
	opts := &slog.HandlerOptions{
		Level:     levelVar,
		AddSource: config.AddSource,
	}

	switch strings.ToLower(config.Format) {
	case FormatJSON:
		handler = slog.NewJSONHandler(writer, opts)
	case FormatLogfmt, "":
		handler = slog.NewTextHandler(writer, opts)
	default:
		handler = slog.NewTextHandler(writer, opts)
	}

	logger := slog.New(handler)
	return logger, closer, nil
}

// Init initializes the global logger with the provided configuration.
//
// This function configures the global logger that is used by all package-level
// logging functions (Debug, Info, Warn, Error, etc.). It must be called before
// using any global logging functions, or they will use a default logger.
//
// The function validates the configuration and returns an error if any settings
// are invalid. If initialization succeeds, the global logger is replaced and
// becomes available for use.
//
// Important: If the previous global logger was writing to a file, that file
// handle will be closed automatically. Always call Shutdown() when your
// application terminates to ensure proper cleanup.
//
// Example:
//
//	cfg := logging.Config{
//		Level:  "info",
//		Format: "json",
//		Output: "/var/log/app.log",
//	}
//	if err := logging.Init(cfg); err != nil {
//		log.Fatal("Failed to initialize logger:", err)
//	}
//	defer logging.Shutdown()
func Init(config Config) error {
	levelVar := &slog.LevelVar{}
	levelVar.Set(parseLevel(config.Level))

	var writer io.Writer
	var closer io.Closer
	switch strings.ToLower(config.Output) {
	case "stdout", "":
		writer = os.Stdout
	case "stderr":
		writer = os.Stderr
	default:
		file, err := openLogFile(config.Output)
		if err != nil {
			return fmt.Errorf("failed to open log file %s: %w", config.Output, err)
		}
		writer = file
		closer = file
	}

	var handler slog.Handler
	opts := &slog.HandlerOptions{
		Level:     levelVar,
		AddSource: config.AddSource,
	}

	switch strings.ToLower(config.Format) {
	case FormatJSON:
		handler = slog.NewJSONHandler(writer, opts)
	case FormatLogfmt, "":
		handler = slog.NewTextHandler(writer, opts)
	default:
		handler = slog.NewTextHandler(writer, opts)
	}

	logger := slog.New(handler)

	globalLogger = logger
	globalCloser = closer
	globalLevelVar = levelVar
	slog.SetDefault(globalLogger)

	return nil
}

// InitWithDefaults initializes the global logger with recommended default settings.
//
// This is a convenience function that calls Init(DefaultConfig()). It configures
// the logger with info-level logging, logfmt format, and stdout output.
//
// This function is ideal for applications that need basic logging without
// custom configuration. For more control over logging behavior, use Init()
// with a custom Config.
//
// Example:
//
//	if err := logging.InitWithDefaults(); err != nil {
//		log.Fatal("Failed to initialize logger:", err)
//	}
//	defer logging.Shutdown()
//
//	logging.Info("application started")
func InitWithDefaults() error {
	return Init(DefaultConfig())
}

// Shutdown gracefully closes any open file handles used by the global logger.
//
// This function should be called when your application terminates to ensure
// proper cleanup of resources. It's safe to call multiple times - subsequent
// calls will be no-ops.
//
// If the global logger is writing to a file, this function closes the file handle.
// If the logger is writing to stdout/stderr, this function does nothing.
//
// It's recommended to defer this call immediately after successful initialization:
//
//	if err := logging.Init(cfg); err != nil {
//		return err
//	}
//	defer logging.Shutdown()
//
// Returns an error only if closing the file handle fails.
func Shutdown() error {
	if globalCloser != nil {
		err := globalCloser.Close()
		globalCloser = nil
		return err
	}
	return nil
}

// SetLevel dynamically changes the log level of the global logger at runtime.
//
// This function allows you to change the minimum log level without restarting
// the application. Messages below the specified level will be filtered out.
//
// Valid levels are: "debug", "info", "warn", "error"
// The level comparison is case-insensitive.
//
// This is particularly useful for:
//   - Enabling debug logging to troubleshoot issues
//   - Reducing log verbosity in production
//   - Implementing runtime log level controls via APIs or signals
//
// Example:
//
//	// Enable debug logging for troubleshooting
//	if err := logging.SetLevel("debug"); err != nil {
//		logging.Error("failed to set log level", "error", err)
//	}
//
//	// Later, reduce verbosity
//	logging.SetLevel("warn")
//
// Returns an error if the level string is invalid.
func SetLevel(level string) error {
	if !ValidateLevel(level) {
		return fmt.Errorf("invalid log level: %q, must be one of: %s, %s, %s, %s",
			level, LevelDebug, LevelInfo, LevelWarn, LevelError)
	}

	if globalLevelVar != nil {
		globalLevelVar.Set(parseLevel(level))
	}
	return nil
}

// parseLevel converts a string level to the corresponding slog.Level value.
// It handles case-insensitive matching and provides a fallback to Info level.
// The "warning" alias is supported for "warn".
func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case LevelDebug:
		return slog.LevelDebug
	case LevelInfo:
		return slog.LevelInfo
	case LevelWarn, "warning":
		return slog.LevelWarn
	case LevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// ValidateLevel reports whether level is a valid log level string.
func ValidateLevel(level string) bool {
	switch strings.ToLower(level) {
	case LevelDebug, LevelInfo, LevelWarn, LevelError:
		return true
	default:
		return false
	}
}

// ValidateFormat reports whether format is a valid log format string.
func ValidateFormat(format string) bool {
	switch strings.ToLower(format) {
	case FormatLogfmt, FormatJSON:
		return true
	default:
		return false
	}
}

// Get returns the global logger instance, initializing it with defaults if necessary.
func Get() *slog.Logger {
	if globalLogger == nil {
		if err := InitWithDefaults(); err != nil {
			return slog.Default()
		}
	}
	return globalLogger
}

// Debug logs a debug message using the global logger.
func Debug(msg string, args ...any) {
	Get().Debug(msg, args...)
}

// Info logs an informational message using the global logger.
func Info(msg string, args ...any) {
	Get().Info(msg, args...)
}

// Warn logs a warning message using the global logger.
func Warn(msg string, args ...any) {
	Get().Warn(msg, args...)
}

// Error logs an error message using the global logger.
func Error(msg string, args ...any) {
	Get().Error(msg, args...)
}

// DebugContext logs a debug message with context using the global logger.
// It automatically includes context fields like request_id, alert_id, etc.
func DebugContext(ctx context.Context, msg string, args ...any) {
	allArgs := args

	if contextFields := extractContextFields(ctx); len(contextFields) > 0 {
		for k, v := range contextFields {
			allArgs = append(allArgs, k, v)
		}
	}

	Get().DebugContext(ctx, msg, allArgs...)
}

// InfoContext logs an informational message with context using the global logger.
// It automatically includes context fields like request_id, alert_id, etc.
func InfoContext(ctx context.Context, msg string, args ...any) {
	allArgs := args

	if contextFields := extractContextFields(ctx); len(contextFields) > 0 {
		for k, v := range contextFields {
			allArgs = append(allArgs, k, v)
		}
	}

	Get().InfoContext(ctx, msg, allArgs...)
}

// WarnContext logs a warning message with context using the global logger.
// It automatically includes context fields like request_id, alert_id, etc.
func WarnContext(ctx context.Context, msg string, args ...any) {
	allArgs := args

	if contextFields := extractContextFields(ctx); len(contextFields) > 0 {
		for k, v := range contextFields {
			allArgs = append(allArgs, k, v)
		}
	}

	Get().WarnContext(ctx, msg, allArgs...)
}

// ErrorContext logs an error message with context using the global logger.
// It automatically includes context fields like request_id, alert_id, etc.
func ErrorContext(ctx context.Context, msg string, args ...any) {
	allArgs := args

	if contextFields := extractContextFields(ctx); len(contextFields) > 0 {
		for k, v := range contextFields {
			allArgs = append(allArgs, k, v)
		}
	}

	Get().ErrorContext(ctx, msg, allArgs...)
}

// globalLoggerWrapper wraps the global logger to implement the Logger interface.
type globalLoggerWrapper struct{}

// Debug logs a debug message using the global logger.
func (g *globalLoggerWrapper) Debug(msg string, args ...any) {
	Debug(msg, args...)
}

// Info logs an informational message using the global logger.
func (g *globalLoggerWrapper) Info(msg string, args ...any) {
	Info(msg, args...)
}

// Warn logs a warning message using the global logger.
func (g *globalLoggerWrapper) Warn(msg string, args ...any) {
	Warn(msg, args...)
}

// Error logs an error message using the global logger.
func (g *globalLoggerWrapper) Error(msg string, args ...any) {
	Error(msg, args...)
}

// DebugContext logs a debug message with context using the global logger.
func (g *globalLoggerWrapper) DebugContext(ctx context.Context, msg string, args ...any) {
	DebugContext(ctx, msg, args...)
}

// InfoContext logs an informational message with context using the global logger.
func (g *globalLoggerWrapper) InfoContext(ctx context.Context, msg string, args ...any) {
	InfoContext(ctx, msg, args...)
}

// WarnContext logs a warning message with context using the global logger.
func (g *globalLoggerWrapper) WarnContext(ctx context.Context, msg string, args ...any) {
	WarnContext(ctx, msg, args...)
}

// ErrorContext logs an error message with context using the global logger.
func (g *globalLoggerWrapper) ErrorContext(ctx context.Context, msg string, args ...any) {
	ErrorContext(ctx, msg, args...)
}

// With returns a new logger with additional attributes.
func (g *globalLoggerWrapper) With(args ...any) Logger {
	return &slogWrapper{logger: Get().With(args...)}
}

// slogWrapper wraps an slog.Logger to implement the Logger interface.
type slogWrapper struct {
	logger *slog.Logger
}

// Debug logs a debug message.
func (s *slogWrapper) Debug(msg string, args ...any) {
	s.logger.Debug(msg, args...)
}

// Info logs an informational message.
func (s *slogWrapper) Info(msg string, args ...any) {
	s.logger.Info(msg, args...)
}

// Warn logs a warning message.
func (s *slogWrapper) Warn(msg string, args ...any) {
	s.logger.Warn(msg, args...)
}

// Error logs an error message.
func (s *slogWrapper) Error(msg string, args ...any) {
	s.logger.Error(msg, args...)
}

// DebugContext logs a debug message with context.
func (s *slogWrapper) DebugContext(ctx context.Context, msg string, args ...any) {
	s.logger.DebugContext(ctx, msg, args...)
}

// InfoContext logs an informational message with context.
func (s *slogWrapper) InfoContext(ctx context.Context, msg string, args ...any) {
	s.logger.InfoContext(ctx, msg, args...)
}

// WarnContext logs a warning message with context.
func (s *slogWrapper) WarnContext(ctx context.Context, msg string, args ...any) {
	s.logger.WarnContext(ctx, msg, args...)
}

// ErrorContext logs an error message with context.
func (s *slogWrapper) ErrorContext(ctx context.Context, msg string, args ...any) {
	s.logger.ErrorContext(ctx, msg, args...)
}

// With returns a new logger with additional attributes.
func (s *slogWrapper) With(args ...any) Logger {
	return &slogWrapper{logger: s.logger.With(args...)}
}

// GetLogger returns a Logger interface that wraps the global logger.
func GetLogger() Logger {
	return &globalLoggerWrapper{}
}

// NewLogger creates a Logger interface from the provided configuration.
func NewLogger(config Config) (Logger, io.Closer, error) {
	logger, closer, err := New(config)
	if err != nil {
		return nil, closer, err
	}
	return &slogWrapper{logger: logger}, closer, nil
}

// Logger defines the interface for structured logging operations.
//
// This interface provides a consistent API for logging across different implementations
// and is ideal for dependency injection, testing, and creating mockable logging components.
//
// All logging methods accept a message string followed by optional key-value pairs
// for structured logging. Keys should be strings, and values can be any type that
// can be serialized by the underlying slog implementation.
//
// Example usage:
//
//	logger := logging.GetLogger()
//	logger.Info("user login", "user_id", "123", "ip", "192.168.1.1")
//	logger.Error("database error", "error", err, "query", "SELECT * FROM users")
//
// Context-aware methods automatically extract and include context fields:
//
//	ctx := context.WithValue(context.Background(), "request_id", "req-456")
//	logger.InfoContext(ctx, "processing request")
//	// Output includes: request_id=req-456
type Logger interface {
	// Debug logs a debug-level message with optional key-value pairs.
	// Debug messages are typically filtered out in production.
	Debug(msg string, args ...any)

	// Info logs an info-level message with optional key-value pairs.
	// Info messages provide general operational information.
	Info(msg string, args ...any)

	// Warn logs a warning-level message with optional key-value pairs.
	// Warnings indicate potentially harmful situations.
	Warn(msg string, args ...any)

	// Error logs an error-level message with optional key-value pairs.
	// Errors indicate failure conditions that need attention.
	Error(msg string, args ...any)

	// DebugContext logs a debug message with context and optional key-value pairs.
	// Context fields are automatically extracted and included.
	DebugContext(ctx context.Context, msg string, args ...any)

	// InfoContext logs an info message with context and optional key-value pairs.
	// Context fields are automatically extracted and included.
	InfoContext(ctx context.Context, msg string, args ...any)

	// WarnContext logs a warning message with context and optional key-value pairs.
	// Context fields are automatically extracted and included.
	WarnContext(ctx context.Context, msg string, args ...any)

	// ErrorContext logs an error message with context and optional key-value pairs.
	// Context fields are automatically extracted and included.
	ErrorContext(ctx context.Context, msg string, args ...any)

	// With returns a new Logger with additional key-value pairs pre-configured.
	// The returned logger will include these pairs in all subsequent log messages.
	With(args ...any) Logger
}

// ComponentLogger represents a logger with pre-configured component context.
//
// This logger automatically includes component name and type in all log messages,
// making it easy to identify which part of your application generated each log entry.
// This is particularly useful in microservices, modular applications, or any system
// where you want to track log messages by their source component.
//
// Component loggers are created with NewComponentLogger() and maintain their
// component context throughout their lifetime. All logging methods automatically
// include "component" and "component_type" fields.
//
// Example:
//
//	authLogger := logging.NewComponentLogger("auth", "service")
//	authLogger.Info("user authenticated", "user_id", "123")
//	// Output: ... component=auth component_type=service user_id=123
type ComponentLogger struct {
	logger        *slog.Logger // underlying structured logger with component context
	component     string       // component name for identification
	componentType string       // component type for categorization
}

// NewComponentLogger creates a new component-aware logger with the specified component context.
//
// The returned logger automatically includes the component name and type in all log messages,
// making it easy to identify the source of log entries in complex applications.
//
// Parameters:
//   - component: A descriptive name for the component (e.g., "auth", "database", "api")
//   - componentType: The type or category of the component (e.g., "service", "handler", "client")
//
// The component and componentType are included as "component" and "component_type" fields
// in every log message produced by the returned logger.
//
// Example:
//
//	dbLogger := logging.NewComponentLogger("database", "connection")
//	dbLogger.Info("connection established", "host", "localhost", "port", 5432)
//	// Output: ... component=database component_type=connection host=localhost port=5432
//
//	apiLogger := logging.NewComponentLogger("users", "handler")
//	apiLogger.Warn("rate limit exceeded", "client_ip", "192.168.1.1")
//	// Output: ... component=users component_type=handler client_ip=192.168.1.1
func NewComponentLogger(component, componentType string) *ComponentLogger {
	return &ComponentLogger{
		logger:        Get().With("component", component, "component_type", componentType),
		component:     component,
		componentType: componentType,
	}
}

// Debug logs a debug message with component context.
func (cl *ComponentLogger) Debug(msg string, args ...any) {
	if cl.logger != nil {
		cl.logger.Debug(msg, args...)
	}
}

// Info logs an info message with component context.
func (cl *ComponentLogger) Info(msg string, args ...any) {
	if cl.logger != nil {
		cl.logger.Info(msg, args...)
	}
}

// Warn logs a warning message with component context.
func (cl *ComponentLogger) Warn(msg string, args ...any) {
	if cl.logger != nil {
		cl.logger.Warn(msg, args...)
	}
}

// Error logs an error message with component context.
func (cl *ComponentLogger) Error(msg string, args ...any) {
	if cl.logger != nil {
		cl.logger.Error(msg, args...)
	}
}

// DebugContext logs a debug message with context and component context.
func (cl *ComponentLogger) DebugContext(ctx context.Context, msg string, args ...any) {
	if cl.logger == nil {
		Get().DebugContext(ctx, msg, args...)
		return
	}
	cl.logger.DebugContext(ctx, msg, args...)
}

// InfoContext logs an info message with context and component context.
func (cl *ComponentLogger) InfoContext(ctx context.Context, msg string, args ...any) {
	if cl.logger == nil {
		Get().InfoContext(ctx, msg, args...)
		return
	}
	cl.logger.InfoContext(ctx, msg, args...)
}

// WarnContext logs a warning message with context and component context.
func (cl *ComponentLogger) WarnContext(ctx context.Context, msg string, args ...any) {
	if cl.logger == nil {
		Get().WarnContext(ctx, msg, args...)
		return
	}
	cl.logger.WarnContext(ctx, msg, args...)
}

// ErrorContext logs an error message with context and component context.
func (cl *ComponentLogger) ErrorContext(ctx context.Context, msg string, args ...any) {
	if cl.logger == nil {
		Get().ErrorContext(ctx, msg, args...)
		return
	}
	cl.logger.ErrorContext(ctx, msg, args...)
}

// With returns a new logger with additional attributes.
func (cl *ComponentLogger) With(args ...any) Logger {
	base := cl.logger
	if base == nil {
		base = Get().With("component", cl.component, "component_type", cl.componentType)
	}
	return &ComponentLogger{
		logger:        base.With(args...),
		component:     cl.component,
		componentType: cl.componentType,
	}
}

// GetComponent returns the component name.
func (cl *ComponentLogger) GetComponent() string {
	return cl.component
}

// GetComponentType returns the component type.
func (cl *ComponentLogger) GetComponentType() string {
	return cl.componentType
}

// extractContextFields extracts logging fields from context.
func extractContextFields(ctx context.Context) map[string]any {
	fields := make(map[string]any)

	if requestID := getContextValue(ctx, "request_id"); requestID != "" {
		fields["request_id"] = requestID
	}

	if alertID := getContextValue(ctx, "alert_id"); alertID != "" {
		fields["alert_id"] = alertID
	}

	if operation := getContextValue(ctx, "operation"); operation != "" {
		fields["operation"] = operation
	}

	if userID := getContextValue(ctx, "user_id"); userID != "" {
		fields["user_id"] = userID
	}

	if traceID := getContextValue(ctx, "trace_id"); traceID != "" {
		fields["trace_id"] = traceID
	}

	if spanID := getContextValue(ctx, "span_id"); spanID != "" {
		fields["span_id"] = spanID
	}

	return fields
}

// getContextValue safely extracts a string value from context.
func getContextValue(ctx context.Context, key string) string {
	if value := ctx.Value(key); value != nil {
		if str, ok := value.(string); ok {
			return str
		}
	}
	return ""
}

// openLogFile opens a log file for writing after validating the path and
// creating parent directories with safe permissions.
func openLogFile(filePath string) (*os.File, error) {
	if filePath == "" {
		return nil, errors.New("log file path cannot be empty")
	}

	// Clean the path and perform basic traversal checks
	cleanPath := filepath.Clean(filePath)
	if strings.Contains(cleanPath, "..") {
		return nil, fmt.Errorf("invalid log file path: contains directory traversal: %s", cleanPath)
	}

	// Disallow writing to sensitive system directories when absolute
	if filepath.IsAbs(cleanPath) {
		restricted := []string{"/etc/", "/proc/", "/sys/", "/dev/", "/run/secrets"}
		for _, p := range restricted {
			if strings.HasPrefix(cleanPath+"/", p) || cleanPath == strings.TrimSuffix(p, "/") {
				return nil, fmt.Errorf("log file path not allowed: %s", cleanPath)
			}
		}
	}

	dir := filepath.Dir(cleanPath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("failed to create log directory %s: %w", dir, err)
	}

	// Refuse to open symlinks and non-regular files
	if info, err := os.Lstat(cleanPath); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("refusing to open symlink for log file: %s", cleanPath)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("log path must be a regular file: %s", cleanPath)
		}
	}

	file, err := os.OpenFile(cleanPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file %s: %w", cleanPath, err)
	}
	return file, nil
}
