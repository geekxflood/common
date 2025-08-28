# GitHub Copilot Instructions for geekxflood/common

## Project Overview

This is a Go 1.25 enterprise-grade common library providing structured logging and configuration management capabilities. The project follows clean architecture principles with two main packages:

- **`logging`**: Comprehensive logging built on Go's standard `log/slog` with component-aware logging, context extraction, and dynamic configuration
- **`config`**: CUE-based configuration management with schema validation, hot reload, and type-safe access

## Architecture Patterns

### Structured Logging with Component Context

**Always use component-aware logging for modular applications:**

```go
// Create component-specific loggers
authLogger := logging.NewComponentLogger("auth", "service")
dbLogger := logging.NewComponentLogger("database", "connection")

// Component context is automatically included
authLogger.Info("user authenticated", "user_id", "123")
// Output: ... component=auth component_type=service user_id=123
```

**Use context-aware logging for request tracing:**

```go
ctx := context.WithValue(context.Background(), "request_id", "req-123")
logging.InfoContext(ctx, "processing request")
// Output includes: request_id=req-123
```

### CUE-Based Configuration Schema

**Define configuration schemas using CUE:**

```cue
package config

server: {
  host: string | *"localhost"
  port: int & >=1024 & <=65535 | *8080
  timeout: string | *"30s"
}
```

**Access configuration with type safety:**

```go
manager, err := config.NewManager(config.Options{
    SchemaPath: "schema.cue",
    ConfigPath: "config.yaml",
})
host, _ := manager.GetString("server.host")
port, _ := manager.GetInt("server.port")
```

## Developer Workflows

### Testing

**Use table-driven tests for comprehensive coverage:**

```go
func TestValidateLevel(t *testing.T) {
    tests := []struct {
        level string
        valid bool
    }{
        {"debug", true},
        {"info", true},
        {"invalid", false},
    }

    for _, tt := range tests {
        t.Run(tt.level, func(t *testing.T) {
            if got := ValidateLevel(tt.level); got != tt.valid {
                t.Errorf("ValidateLevel(%q) = %v, want %v", tt.level, got, tt.valid)
            }
        })
    }
}
```

**Run the full test suite:**

```bash
go test ./... -cover
```

### Code Quality

**Linting is enforced with golangci-lint:**

```bash
golangci-lint run --path-mode=abs --config .golangci.yml --fast-only
```

**Key linting rules:**

- Use `revive` for Go style enforcement
- `errcheck` for error handling
- `gosec` for security issues
- `staticcheck` for static analysis

### Security

**Security scanning with gosec:**

```bash
gosec -conf=.gosec.json -fmt=json ./...
```

**Safe file operations:**

- Always use `filepath.Clean()` for path sanitization
- Validate file paths before reading
- Check file size limits
- Use absolute paths with directory traversal protection

## Code Conventions

### Formatting and Style

**Go files use tabs, others use spaces:**

```go
// .editorconfig rules
[*.go]
indent_style = tab
indent_size = 4

[*.yaml]
indent_style = space
indent_size = 2
```

**Follow Go naming conventions:**

- Exported functions/types: `PascalCase`
- Unexported: `camelCase`
- Use descriptive names with context

### Error Handling

**Use standard library errors:**

```go
// ✓ Good
return fmt.Errorf("failed to connect: %w", err)

// ✗ Avoid
return errors.Wrap(err, "failed to connect")
```

### Interface Design

**Use interfaces for dependency injection:**

```go
type Logger interface {
    Info(msg string, args ...any)
    Error(msg string, args ...any)
    // ...
}
```

## Configuration Management

### Hot Reload

**Enable automatic configuration reloading:**

```go
manager.StartHotReload(ctx)
manager.OnConfigChange(func(err error) {
    if err != nil {
        log.Printf("Config reload failed: %v", err)
    }
})
```

### Schema Validation

**Validate configurations against CUE schemas:**

```go
validator, _ := schemaLoader.GetValidator()
err := validator.ValidateConfig(config)
```

## Logging Patterns

### Context Fields

**Automatically extracted context fields:**

- `request_id` - HTTP request identifier
- `user_id` - Authenticated user identifier
- `trace_id` - Distributed tracing identifier
- `operation` - Current operation name

### Log Levels

**Use appropriate levels:**

- `DEBUG`: Detailed diagnostic information
- `INFO`: General operational information
- `WARN`: Potentially harmful situations
- `ERROR`: Failure conditions requiring attention

## Build and Deployment

### CI/CD Pipeline

**GitLab CI runs on tagged releases:**

- Security scanning with gosec
- Code validation (fmt, vet)
- Dependency management

### Dependencies

**Core dependencies:**

- `cuelang.org/go` - CUE language support
- `github.com/fsnotify/fsnotify` - File watching for hot reload

## Best Practices

1. **Always call cleanup functions:** `defer logging.Shutdown()`, `defer manager.Close()`
2. **Use structured logging:** Prefer key-value pairs over formatted strings
3. **Validate early:** Validate configuration at application startup
4. **Handle hot reload gracefully:** Design for configuration changes
5. **Use component loggers:** For modular applications
6. **JSON for production:** Machine-readable format for log aggregation
7. **Test thoroughly:** Maintain high test coverage (60%+)
8. **Follow security practices:** Use gosec, safe file operations

## Common Patterns

### Factory Creation

```go
// Component logger factory
factory := logging.NewComponentLoggerFactory()
logger := factory.CreateLogger("component", "type")
```

### Configuration-Driven Development

```go
// Use configuration to create components
components, _ := manager.GetMap("components")
for name, config := range components {
    component := factory.Create(name, config)
    app.Register(component)
}
```

### Safe File Operations

````go
func safeReadFile(filePath string) ([]byte, error) {
    cleanPath := filepath.Clean(filePath)
    // Validate path security...
    return os.ReadFile(cleanPath)
}
```</content>
<parameter name="filePath">/home/cri/dev/geekxflood/common/.github/copilot-instructions.md
````
