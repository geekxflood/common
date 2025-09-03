# Package `logging`

A package for structured logging using Go's standard log/slog package.

## Key Features

✅ **Structured Output** - JSON and logfmt formats with key-value pairs
✅ **Component Isolation** - Separate loggers for different parts of your app
✅ **Context Integration** - Automatic extraction of trace IDs and user info
✅ **Multiple Outputs** - Console and file output with automatic directory creation
✅ **Dynamic Configuration** - Runtime log level changes without restart
✅ **Performance Optimized** - Minimal overhead with efficient structured logging
✅ **Standard Library Based** - Built on Go's standard log/slog package

## Installation

```bash
go get github.com/geekxflood/common/logging
```

## Architecture

The package consists of three main components:

1. **Global Logger Management** - Centralized logging configuration and access
2. **Component-Aware Logging** - Automatic component detection and scoped logging
3. **Configuration Management** - Dynamic configuration with hot reload support

## Package Structure

```text
logging/
├── logging.go          # Main implementation with Logger and interfaces
├── logging_test.go     # Comprehensive unit tests
└── logging.md          # This documentation file
```

## Quick Start

### Basic Usage

```go
package main

import "github.com/geekxflood/common/logging"

func main() {
    // Initialize with defaults
    logging.InitWithDefaults()
    defer logging.Shutdown()

    // Structured logging
    logging.Info("application started", "version", "1.0.0")
    logging.Error("connection failed", "error", "timeout", "retry", 3)

    // Component loggers
    dbLogger := logging.NewComponentLogger("database", "service")
    dbLogger.Debug("connecting", "host", "localhost")
}
```

### Custom Configuration

```go
// Configure with custom settings
cfg := logging.Config{
    Level:     "debug",
    Format:    "json",
    Output:    "/var/log/app.log",
    AddSource: true,
    Component: "user-service",
}

err := logging.Init(cfg)
if err != nil {
    panic(err)
}
defer logging.Shutdown()

// Structured logging with context
logging.Debug("processing request",
    "user_id", 12345,
    "action", "login",
    "ip", "192.168.1.100")
```

### Context Integration

```go
import (
    "context"
    "github.com/geekxflood/common/logging"
)

func handleRequest(ctx context.Context, userID string) {
    // Context-aware logging
    logging.InfoContext(ctx, "handling request", "user_id", userID)

    if err := processUser(ctx, userID); err != nil {
        logging.ErrorContext(ctx, "failed to process user", "error", err)
        return
    }

    logging.InfoContext(ctx, "request completed successfully")
}
```

## Configuration Reference

### Config Structure

The `Config` struct defines all logging configuration options:

```go
type Config struct {
    // Level sets the minimum log level (debug, info, warn, error)
    Level string `json:"level" yaml:"level"`

    // Format specifies output format (logfmt, json)
    Format string `json:"format" yaml:"format"`

    // Output specifies where to write logs (stdout, stderr, file path)
    Output string `json:"output" yaml:"output"`

    // AddSource includes source file and line number in logs
    AddSource bool `json:"add_source" yaml:"add_source"`
}
```

### Default Configuration

```go
defaultConfig := logging.Config{
    Level:     "info",
    Format:    "logfmt",
    Output:    "stdout",
    AddSource: false,
}
```

## API Reference

### Types

#### Config

```go
type Config struct {
    Level     string
    Format    string
    Output    string
    AddSource bool
}
```

Configuration structure for logger initialization.

#### Logger

```go
type Logger interface {
    Debug(msg string, args ...any)
    Info(msg string, args ...any)
    Warn(msg string, args ...any)
    Error(msg string, args ...any)

    DebugContext(ctx context.Context, msg string, args ...any)
    InfoContext(ctx context.Context, msg string, args ...any)
    WarnContext(ctx context.Context, msg string, args ...any)
    ErrorContext(ctx context.Context, msg string, args ...any)

    With(args ...any) Logger
}
```

Logger interface for structured logging operations.

#### ComponentLogger

```go
type ComponentLogger struct {
    // Component-aware logger with automatic context
}
```

Component-specific logger that automatically includes component information.

### Functions

#### Initialization

```go
func Init(cfg Config) error
func InitWithDefaults() error
func Shutdown() error
```

#### Component Loggers

```go
func NewComponentLogger(component, componentType string) *ComponentLogger
```

#### Global Logging Functions

```go
func Debug(msg string, args ...any)
func Info(msg string, args ...any)
func Warn(msg string, args ...any)
func Error(msg string, args ...any)

func DebugContext(ctx context.Context, msg string, args ...any)
func InfoContext(ctx context.Context, msg string, args ...any)
func WarnContext(ctx context.Context, msg string, args ...any)
func ErrorContext(ctx context.Context, msg string, args ...any)
```

## Use Cases

### Web Service Logging

```go
package main

import (
    "net/http"
    "github.com/geekxflood/common/logging"
)

func main() {
    // Initialize logging for web service
    cfg := logging.Config{
        Level:     "info",
        Format:    "json",
        Output:    "/var/log/webservice.log",
        AddSource: true,
    }

    logging.Init(cfg)
    defer logging.Shutdown()

    http.HandleFunc("/api/users", handleUsers)

    logging.Info("web service starting", "port", 8080)
    http.ListenAndServe(":8080", nil)
}

func handleUsers(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    logging.InfoContext(ctx, "handling request",
        "method", r.Method,
        "path", r.URL.Path,
        "remote_addr", r.RemoteAddr)

    // Process request...

    logging.InfoContext(ctx, "request completed", "status", 200)
}
```

### Component-Based Logging

```go
func processOrder(ctx context.Context, orderID string) error {
    // Create component logger
    orderLogger := logging.NewComponentLogger("order-processor", "service")

    orderLogger.InfoContext(ctx, "starting order processing", "order_id", orderID)

    // Validate order
    if err := validateOrder(ctx, orderID); err != nil {
        orderLogger.ErrorContext(ctx, "order validation failed",
            "order_id", orderID, "error", err)
        return err
    }

    // Process payment
    if err := processPayment(ctx, orderID); err != nil {
        orderLogger.ErrorContext(ctx, "payment processing failed",
            "order_id", orderID, "error", err)
        return err
    }

    orderLogger.InfoContext(ctx, "order processing completed successfully",
        "order_id", orderID)
    return nil
}
```

## Testing

The package includes comprehensive tests covering:

- Logger initialization and configuration
- All log levels and output formats
- Context integration and field propagation
- Component logger functionality
- Error handling and edge cases
- Performance benchmarks

Run tests:

```bash
cd logging/
go test -v
go test -race
go test -bench=.
```

## Design Principles

1. **Structured by Default** - All logging uses structured key-value pairs
2. **Context-Aware** - Seamless integration with Go's context package
3. **Component Isolation** - Separate loggers for different application parts
4. **Performance First** - Minimal overhead for production use
5. **Configuration Driven** - Runtime configuration changes without restart
6. **Standard Library** - Built on Go's standard log/slog package

## Dependencies

The package has minimal external dependencies:

- Go standard library - `log/slog`, `context`, `sync`, `time`, `fmt`
- No external logging frameworks or dependencies

The logging package is designed to be lightweight and self-contained while providing enterprise-grade logging capabilities.
