# github.com/geekxflood/common

![Go Version](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go) [![Go Reference](https://pkg.go.dev/badge/github.com/geekxflood/common.svg)](https://pkg.go.dev/github.com/geekxflood/common) [![Go Report Card](https://goreportcard.com/badge/github.com/geekxflood/common)](https://goreportcard.com/report/github.com/geekxflood/common) [![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

A small collection of Go packages shared across GeekxFlood projects.

Current packages:

- logging: Structured logging built on the standard log/slog
- config: CUE-powered configuration loading, validation, and hot reload
- helpers: Utilities package (foundation for future helpers)

Install packages:

```bash
go get github.com/geekxflood/common/logging
go get github.com/geekxflood/common/config
```

Quality gates:

```bash
# Lint
golangci-lint run --config .golangci.yml

# Security scan
gosec -conf=.gosec.json -fmt=json ./...

# Tests
go test ./...
```

## Logging

A comprehensive structured logging package for Go applications built on the standard `log/slog` package. This library provides enterprise-grade logging capabilities with a focus on performance, flexibility, and ease of use.

## Features

- **🏗️ Structured Logging**: Built on Go's standard `log/slog` for consistent, structured output
- **📋 Multiple Formats**: Support for both human-readable logfmt and machine-readable JSON formats
- **🔧 Component-Aware**: Automatic component identification and context extraction for modular applications
- **⚡ Dynamic Configuration**: Runtime log level changes and configuration updates without restart
- **📁 Flexible Output**: Console (stdout/stderr) and file output with automatic directory creation
- **🔍 Context Integration**: Automatic extraction of request IDs, trace IDs, user IDs, and other context fields
- **🧪 Comprehensive Testing**: Extensive test coverage with 60%+ code coverage
- **🔌 Interface-Based**: Clean Logger interface for dependency injection and testing
- **🏭 Factory Pattern**: Component logger factory for consistent logger creation
- **📚 Full Documentation**: Complete godoc documentation following Go best practices

## Quick Start

### Basic Usage

```go
package main

import (
    "context"
    "github.com/geekxflood/common/logging"
)

func main() {
    // Initialize with defaults (info level, logfmt format, stdout)
    if err := logging.InitWithDefaults(); err != nil {
        panic(err)
    }
    defer logging.Shutdown() // Always cleanup resources

    // Basic structured logging
    logging.Info("application started", "version", "1.0.0", "environment", "production")
    logging.Warn("configuration missing", "key", "database.timeout", "using_default", true)
    logging.Error("connection failed", "error", "connection refused", "retry_count", 3)
}
```

### Custom Configuration

```go
package main

import "github.com/geekxflood/common/logging"

func main() {
    // Configure with custom settings
    cfg := logging.Config{
        Level:     "debug",           // Enable debug messages
        Format:    "json",            // Machine-readable format
        Output:    "/var/log/app.log", // File output
        AddSource: true,              // Include source file info
    }

    if err := logging.Init(cfg); err != nil {
        panic(err)
    }
    defer logging.Shutdown()

    logging.Debug("debug information", "module", "startup")
    logging.Info("service ready", "port", 8080)
}
```

### Component-Aware Logging

```go
package main

import "github.com/geekxflood/common/logging"

func main() {
    logging.InitWithDefaults()
    defer logging.Shutdown()

    // Create component-specific loggers
    authLogger := logging.NewComponentLogger("auth", "service")
    dbLogger := logging.NewComponentLogger("database", "connection")

    // Component context is automatically included
    authLogger.Info("user authenticated", "user_id", "123", "method", "oauth")
    // Output: ... component=auth component_type=service user_id=123 method=oauth

    dbLogger.Warn("connection pool low", "active", 8, "max", 10)
    // Output: ... component=database component_type=connection active=8 max=10

    // Or use component functions directly
    logging.InfoComponent("cache", "redis", "cache hit", "key", "user:123", "ttl", 3600)
}
```

### Context-Aware Logging

```go
package main

import (
    "context"
    "github.com/geekxflood/common/logging"
)

func processRequest(ctx context.Context) {
    // Context fields are automatically extracted and included
    logging.InfoContext(ctx, "processing request", "action", "get_user")
    // Output includes: request_id=req-123 user_id=user-456 trace_id=trace-abc

    // Component logging with context
    authLogger := logging.NewComponentLogger("auth", "middleware")
    authLogger.InfoContext(ctx, "token validated", "token_type", "bearer")
}

func main() {
    logging.InitWithDefaults()
    defer logging.Shutdown()

    // Create context with tracking fields
    ctx := context.Background()
    ctx = context.WithValue(ctx, "request_id", "req-123")
    ctx = context.WithValue(ctx, "user_id", "user-456")
    ctx = context.WithValue(ctx, "trace_id", "trace-abc")

    processRequest(ctx)
}
```

## Advanced Usage

### Dynamic Level Changes

```go
// Change log level at runtime
if err := logging.SetLevel("debug"); err != nil {
    logging.Error("failed to set log level", "error", err)
}

// Enable debug logging temporarily
logging.SetLevel("debug")
logging.Debug("detailed debugging info", "state", "processing")

// Reduce verbosity for production
logging.SetLevel("warn") // Only warnings and errors
```

### Logger Interface for Dependency Injection

```go
type UserService struct {
    logger logging.Logger
}

func NewUserService(logger logging.Logger) *UserService {
    return &UserService{
        logger: logger.With("service", "user"),
    }
}

func (s *UserService) CreateUser(ctx context.Context, user User) error {
    s.logger.InfoContext(ctx, "creating user", "username", user.Username)

    if err := s.validateUser(user); err != nil {
        s.logger.ErrorContext(ctx, "user validation failed", "error", err)
        return err
    }

    s.logger.InfoContext(ctx, "user created successfully", "user_id", user.ID)
    return nil
}

// Usage
func main() {
    logging.InitWithDefaults()
    defer logging.Shutdown()

    logger := logging.GetLogger()
    userService := NewUserService(logger)

    ctx := context.WithValue(context.Background(), "request_id", "req-789")
    userService.CreateUser(ctx, user)
}
```

### Independent Logger Instances

```go
// Create independent loggers for different purposes
auditLogger, auditCloser, err := logging.NewLogger(logging.Config{
    Level:  "info",
    Format: "json",
    Output: "/var/log/audit.log",
})
if err != nil {
    panic(err)
}
defer auditCloser.Close()

debugLogger, debugCloser, err := logging.NewLogger(logging.Config{
    Level:     "debug",
    Format:    "logfmt",
    Output:    "/tmp/debug.log",
    AddSource: true,
})
if err != nil {
    panic(err)
}
defer debugCloser.Close()

// Use different loggers for different purposes
auditLogger.Info("user action", "action", "login", "user_id", "123")
debugLogger.Debug("internal state", "cache_size", 1024, "memory_usage", "45MB")
```

## Configuration

The `Config` struct provides comprehensive configuration options:

| Field       | Type   | Default    | Description                                          |
| ----------- | ------ | ---------- | ---------------------------------------------------- |
| `Level`     | string | `"info"`   | Log level: `debug`, `info`, `warn`, `error`          |
| `Format`    | string | `"logfmt"` | Output format: `logfmt`, `json`                      |
| `Output`    | string | `"stdout"` | Output destination: `stdout`, `stderr`, or file path |
| `AddSource` | bool   | `false`    | Include source file and line information             |

### Configuration Examples

```go
// Development configuration
devConfig := logging.Config{
    Level:     "debug",
    Format:    "logfmt",
    Output:    "stdout",
    AddSource: true,
}

// Production configuration
prodConfig := logging.Config{
    Level:  "info",
    Format: "json",
    Output: "/var/log/app.log",
}

// High-performance configuration
perfConfig := logging.Config{
    Level:     "warn",
    Format:    "json",
    Output:    "stdout",
    AddSource: false,
}
```

## Context Fields

The logging package automatically extracts and includes the following context fields when using context-aware logging methods:

- `request_id` - HTTP request identifier
- `user_id` - Authenticated user identifier
- `trace_id` - Distributed tracing identifier
- `span_id` - Tracing span identifier
- `alert_id` - Alert or incident identifier
- `operation` - Current operation name

To use context fields, add them to your context:

```go
ctx := context.Background()
ctx = context.WithValue(ctx, "request_id", "req-12345")
ctx = context.WithValue(ctx, "user_id", "user-67890")

logging.InfoContext(ctx, "processing request")
// Output: ... request_id=req-12345 user_id=user-67890
```

## API Reference

### Package Functions

- `Init(config Config) error` - Initialize global logger
- `InitWithDefaults() error` - Initialize with default settings
- `Shutdown() error` - Cleanup resources
- `SetLevel(level string) error` - Change log level at runtime
- `Get() *slog.Logger` - Get underlying slog logger
- `GetLogger() Logger` - Get Logger interface

### Logging Functions

- `Debug/Info/Warn/Error(msg string, args ...any)` - Basic logging
- `DebugContext/InfoContext/WarnContext/ErrorContext(ctx context.Context, msg string, args ...any)` - Context-aware logging
- `DebugComponent/InfoComponent/WarnComponent/ErrorComponent(component, componentType, msg string, args ...any)` - Component logging
- `*ComponentContext(ctx context.Context, component, componentType, msg string, args ...any)` - Component + context logging

### Types

- `Config` - Logger configuration
- `Logger` - Logging interface
- `ComponentLogger` - Component-aware logger

## Testing

Run the comprehensive test suite:

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run tests with verbose output
go test -v ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Test Coverage

The package maintains high test coverage with comprehensive tests for:

- ✅ Configuration validation and parsing
- ✅ Multiple output formats (logfmt, JSON)
- ✅ File and console output
- ✅ Component-aware logging
- ✅ Context field extraction
- ✅ Dynamic level changes
- ✅ Logger interface implementation
- ✅ Error handling and edge cases

## Performance

The logging package is built on Go's standard `log/slog`, providing excellent performance characteristics:

- **Zero-allocation logging** for most common cases
- **Structured logging** without reflection overhead
- **Lazy evaluation** of expensive operations
- **Efficient JSON encoding** using standard library
- **Minimal memory footprint** with reusable buffers

## Best Practices

1. **Always call Shutdown()**: Use `defer logging.Shutdown()` after initialization
2. **Use structured logging**: Prefer key-value pairs over formatted strings
3. **Choose appropriate levels**: Debug for development, Info for operations, Warn for issues, Error for failures
4. **Include context**: Use context-aware methods for request tracing
5. **Component identification**: Use component loggers for modular applications
6. **Avoid source info in production**: Set `AddSource: false` for better performance
7. **Use JSON in production**: Machine-readable format for log aggregation
8. **File rotation**: Implement external log rotation for file outputs

## Config

A comprehensive configuration management package for Go applications built on CUE (cuelang.org) for schema definition and validation. This library provides enterprise-grade configuration capabilities with a focus on type safety, hot reload, and configuration-driven development.

## Features

- **🔧 CUE-Based Schema Definition**: Define configuration schemas using CUE language for powerful validation and type safety
- **📋 Multiple Format Support**: Load configuration from YAML and JSON files with automatic validation
- **🔄 Hot Reload**: Automatic configuration reloading with file watching and change notifications
- **✅ Schema Validation**: Comprehensive validation against CUE schemas with detailed error messages
- **🏗️ Configuration-Driven Development**: Support patterns where application components are dynamically configured
- **🔌 Interface-Based Design**: Clean interfaces for dependency injection and testing
- **📚 Rich Template Library**: Pre-built CUE templates for common configuration patterns (server, database, web app, microservice)
- **🧪 Comprehensive Testing**: Extensive test coverage with 60%+ code coverage and real-world scenarios
- **📖 Full Documentation**: Complete godoc documentation following Go best practices

## Quick Start

### 1. Define Schema

Create a CUE schema file (`schema.cue`):

```cue
package config

server: {
  host: string | *"localhost"
  port: int & >=1024 & <=65535 | *8080
  timeout: string | *"30s"
}

database: {
  type: "postgres" | "mysql" | "sqlite" | *"postgres"
  host: string | *"localhost"
  port: int & >0 | *5432
  name: string | *"myapp"
  username?: string
  password?: string
}

features: {
  auth: bool | *false
  metrics: bool | *true
  debug: bool | *false
}
```

### 2. Create Configuration

Create a YAML configuration file (`config.yaml`):

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  timeout: "60s"

database:
  type: "postgres"
  host: "db.example.com"
  port: 5432
  name: "production_db"
  username: "app_user"
  password: "secure_password"

features:
  auth: true
  metrics: true
  debug: false
```

### 3. Use in Application

```go
package main

import (
    "log"
    "github.com/geekxflood/common/config"
)

func main() {
    // Create configuration manager
    manager, err := config.NewManager(config.Options{
        SchemaPath: "schema.cue",
        ConfigPath: "config.yaml",
    })
    if err != nil {
        log.Fatal("Failed to create config manager:", err)
    }
    defer manager.Close()

    // Access configuration values
    host, _ := manager.GetString("server.host")
    port, _ := manager.GetInt("server.port")
    timeout, _ := manager.GetDuration("server.timeout")

    log.Printf("Server: %s:%d (timeout: %v)", host, port, timeout)

    // Check features
    if authEnabled, _ := manager.GetBool("features.auth"); authEnabled {
        log.Println("Authentication enabled")
    }

    // Get nested configuration
    dbConfig, _ := manager.GetMap("database")
    log.Printf("Database: %+v", dbConfig)
}
```

## Advanced Usage

### Hot Reload

Enable automatic configuration reloading:

```go
// Enable hot reload
ctx := context.Background()
if err := manager.StartHotReload(ctx); err != nil {
    log.Fatal("Failed to start hot reload:", err)
}

// Register change handler
manager.OnConfigChange(func(err error) {
    if err != nil {
        log.Printf("Config reload failed: %v", err)
    } else {
        log.Println("Configuration reloaded")
        // Reconfigure your application components here
    }
})
```

### Configuration-Driven Development

Use configuration to dynamically create application components:

```go
// Get component configurations
components, _ := manager.GetMap("components")

for name, config := range components {
    component := factory.Create(name, config)
    app.Register(component)
}
```

### Validation

Validate configuration programmatically:

```go
// Validate current configuration
if err := manager.Validate(); err != nil {
    log.Printf("Configuration validation failed: %v", err)
}

// Validate a configuration file without loading it
validator, _ := schemaLoader.GetValidator()
if err := validator.ValidateFile("config.yaml"); err != nil {
    log.Printf("File validation failed: %v", err)
}
```

## API Reference

### Core Types

#### Manager

Main interface for configuration management:

- `GetString(path string, defaultValue ...string) (string, error)`
- `GetInt(path string, defaultValue ...int) (int, error)`
- `GetBool(path string, defaultValue ...bool) (bool, error)`
- `GetDuration(path string, defaultValue ...time.Duration) (time.Duration, error)`
- `GetStringSlice(path string, defaultValue ...[]string) ([]string, error)`
- `GetMap(path string) (map[string]any, error)`
- `Exists(path string) bool`
- `Validate() error`
- `StartHotReload(ctx context.Context) error`
- `StopHotReload()`
- `OnConfigChange(callback func(error))`
- `Reload() error`
- `Close() error`

#### Provider

Interface for configuration value access (subset of Manager).

#### Validator

Interface for configuration validation:

- `ValidateConfig(config map[string]any) error`
- `ValidateValue(path string, value any) error`
- `ValidateFile(configPath string) error`

### Functions

#### NewManager(options Options) (Manager, error)

Creates a new configuration manager with the specified options.

#### NewSchemaLoader() SchemaLoader

Creates a new CUE schema loader.

## Testing

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run tests with verbose output
go test -v ./...
```

## Best Practices

1. **Use CUE Defaults**: Define sensible defaults in your CUE schemas
2. **Validate Early**: Validate configuration at application startup
3. **Handle Hot Reload**: Design your application to handle configuration changes gracefully
4. **Use Templates**: Leverage the provided templates for common patterns
5. **Environment-Specific Configs**: Use different configuration files for different environments
6. **Secure Secrets**: Don't store secrets in configuration files; use environment variables or secret management systems
