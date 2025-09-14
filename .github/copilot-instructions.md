# GitHub Copilot Instructions for geekxflood/common

## Project Overview

This is a Go 1.25 enterprise-grade common library providing configuration management, structured logging, rule evaluation, and SNMP utilities. The project follows clean architecture principles with five main packages:

- **`config`**: CUE-powered configuration with hot reload, schema validation, and environment variables
- **`logging`**: Structured logging with component-aware logging, context extraction, and dynamic configuration
- **`ruler`**: CUE expression evaluation with intelligent rule selection and type-safe outputs
- **`snmptranslate`**: MIB-based OID translation with trie-based lookups and caching
- **`trapprocessor`**: SNMP trap processing with template engine and worker pools

## Architecture Patterns

### CUE-Based Configuration Schema

**Always define configuration schemas using CUE for type safety:**

```cue
package config

server: {
    host: string | *"localhost"
    port: int & >=1024 & <=65535 | *8080
    timeout: string | *"30s"
}
```

**Use inline schemas for embedded configurations:**

```go
manager, err := config.NewManager(config.Options{
    SchemaContent: `
server: {
    host: string | *"localhost"
    port: int & >=1024 & <=65535 | *8080
}
`,
    ConfigPath: "config.yaml",
})
```

**Leverage environment variable substitution:**

```yaml
server:
  host: "${SERVER_HOST:-localhost}"
  port: $SERVER_PORT
database:
  url: "${DATABASE_URL:-sqlite://./app.db}"
```

### Component-Aware Logging Architecture

**Create component-specific loggers for modular applications:**

```go
// Initialize global logger first
logging.InitWithDefaults()
defer logging.Shutdown()

// Create component loggers
authLogger := logging.NewComponentLogger("auth", "service")
dbLogger := logging.NewComponentLogger("database", "connection")

// Component context automatically included
authLogger.Info("user authenticated", "user_id", "123")
// Output: ... component=auth component_type=service user_id=123
```

**Use context-aware logging for request tracing:**

```go
ctx := context.WithValue(context.Background(), "request_id", "req-123")
logging.InfoContext(ctx, "processing request")
// Output includes: request_id=req-123
```

### Rule Evaluation with Structured Specifications

**Define rules with InputSpec and OutputSpec for type safety:**

```go
config := map[string]any{
    "rules": []any{
        map[string]any{
            "name": "high_cpu_alert",
            "inputs": []any{
                map[string]any{
                    "name": "cpu_usage",
                    "type": "number",
                    "required": true,
                },
            },
            "expr": `cpu_usage > 80`,
            "outputs": []any{
                map[string]any{
                    "name": "alert",
                    "fields": map[string]any{
                        "severity": map[string]any{"default": "high"},
                    },
                },
            },
        },
    },
}
```

### SNMP MIB Translation with Performance Optimization

**Initialize MIB translator with directory scanning:**

```go
translator := snmptranslate.New()
if err := translator.Init("/usr/share/snmp/mibs"); err != nil {
    log.Fatal(err)
}

// Fast OID-to-name translation
name, err := translator.Translate(".1.3.6.1.6.3.1.1.5.1")
// Returns: "coldStart"
```

**Batch translation for multiple OIDs:**

```go
oids := []string{".1.3.6.1.2.1.1.1.0", ".1.3.6.1.2.1.1.3.0"}
results := translator.TranslateBatch(oids)
```

### SNMP Trap Processing with Template Engine

**Configure trap processor with worker pools:**

```go
config := map[string]any{
    "snmp": map[string]any{
        "port":         1162,
        "bind_address": "0.0.0.0",
        "community":    "public",
        "version":      "2c",
    },
    "worker_pool": map[string]any{
        "enabled": true,
        "size":    10,
    },
    "templates": map[string]any{
        "path":    "/etc/templates",
        "default": "default",
    },
}

processor, err := trapprocessor.New(config)
```

## Developer Workflows

### Testing Strategy

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

**Run comprehensive test suite:**

```bash
# Full test coverage with race detection
go test -race -cover ./...

# Test specific package
go test ./config -v

# Run benchmarks
go test -bench=. ./...
```

### Code Quality & Security

**Enforce strict linting with golangci-lint:**

```bash
golangci-lint run --config .golangci.yml --path-mode=abs --fast-only
```

**Key linting rules:**

- `revive`: Go style enforcement (receiver naming, error handling)
- `gosec`: Security vulnerability detection
- `staticcheck`: Advanced static analysis
- `errcheck`: Error handling validation
- `sloglint`: Structured logging validation

**Security scanning:**

```bash
gosec -conf=.gosec.json -fmt=json ./...
```

## Code Conventions

### Go Formatting & Style

**Follow strict Go formatting rules:**

```go
// .editorconfig rules
[*.go]
indent_style = tab
indent_size = 4

[*.yaml]
indent_style = space
indent_size = 2
```

**Naming conventions:**

- Exported functions/types: `PascalCase`
- Unexported: `camelCase`
- Use descriptive names with context

### Error Handling

**Use standard library errors with proper wrapping:**

```go
// ✓ Good - standard library with wrapping
return fmt.Errorf("failed to connect: %w", err)

// ✗ Avoid - third-party error libraries
return errors.Wrap(err, "failed to connect")
```

### Interface Design

**Design clean interfaces for dependency injection:**

```go
type Provider interface {
    GetString(path string, defaultValue ...string) (string, error)
    GetInt(path string, defaultValue ...int) (int, error)
    Exists(path string) bool
}

type Manager interface {
    Provider
    StartHotReload(ctx context.Context) error
    OnConfigChange(callback func(error))
    Reload() error
}
```

## Configuration Management

### Hot Reload Patterns

**Enable granular hot reload control:**

```go
manager, err := config.NewManager(config.Options{
    SchemaPath: "schema.cue",
    ConfigPath: "config.yaml",
    EnableSchemaHotReload: true,  // Watch schema changes
    EnableConfigHotReload: false, // Manual config reload
})

manager.OnConfigChange(func(err error) {
    if err != nil {
        logging.Error("config reload failed", "error", err)
    } else {
        logging.Info("configuration reloaded")
    }
})
```

### Schema Validation

**Validate configurations against CUE schemas:**

```go
validator, err := schemaLoader.GetValidator()
if err := validator.ValidateConfig(config); err != nil {
    return fmt.Errorf("config validation failed: %w", err)
}
```

## Logging Patterns

### Context Field Extraction

**Automatically extracted context fields:**

- `request_id`: HTTP request identifier
- `user_id`: Authenticated user identifier
- `trace_id`: Distributed tracing identifier
- `operation`: Current operation name

### Dynamic Configuration

**Change log levels at runtime:**

```go
// Enable debug logging
logging.SetLevel("debug")

// Switch to production mode
logging.SetLevel("warn")
```

## SNMP Integration Patterns

### MIB File Management

**Load MIB files from multiple directories:**

```go
translator := snmptranslate.New()
dirs := []string{
    "/usr/share/snmp/mibs",
    "/opt/mibs",
    "./custom-mibs",
}

for _, dir := range dirs {
    if err := translator.LoadMIBsFromDir(dir); err != nil {
        log.Printf("Failed to load MIBs from %s: %v", dir, err)
    }
}
```

### Trap Processing with Templates

**Define trap processing templates:**

```go
// Template file: /etc/templates/linkDown.json
{
  "name": "linkDown",
  "oid": ".1.3.6.1.6.3.1.1.5.3",
  "fields": [
    {"name": "ifIndex", "oid": ".1.3.6.1.2.1.2.2.1.1", "type": "integer"},
    {"name": "ifAdminStatus", "oid": ".1.3.6.1.2.1.2.2.1.7", "type": "integer"}
  ],
  "message_template": "Interface {{.ifIndex}} went down (admin status: {{.ifAdminStatus}})"
}
```

## Build & Deployment

### CI/CD Pipeline

**GitHub Actions workflows:**

- CodeQL security analysis on push
- Issue summarization with AI
- Automated dependency updates via Dependabot

### Dependencies

**Core dependencies:**

- `cuelang.org/go v0.14.1`: CUE language support
- `github.com/fsnotify/fsnotify v1.9.0`: File watching for hot reload
- `github.com/gosnmp/gosnmp v1.42.1`: SNMP protocol support
- `github.com/stretchr/testify v1.10.0`: Testing utilities

## Best Practices

1. **Always call cleanup functions:** `defer logging.Shutdown()`, `defer manager.Close()`
2. **Use structured logging:** Key-value pairs over formatted strings
3. **Validate early:** Schema validation at application startup
4. **Handle hot reload gracefully:** Design for configuration changes
5. **Use component loggers:** For modular applications
6. **JSON for production:** Machine-readable log format
7. **Test thoroughly:** Table-driven tests with high coverage
8. **Follow security practices:** gosec scanning, safe file operations
9. **Load MIBs efficiently:** Use lazy loading and caching for SNMP operations
10. **Process traps asynchronously:** Use worker pools for high-throughput scenarios

## Common Patterns

### Factory Pattern for Components

```go
// Component factory with configuration
factory := logging.NewComponentLoggerFactory()
logger := factory.CreateLogger("component", "type")
```

### Configuration-Driven Development

```go
// Create components from configuration
components, _ := manager.GetMap("components")
for name, config := range components {
    component := factory.Create(name, config)
    app.Register(component)
}
```

### Safe File Operations

```go
func safeReadFile(filePath string) ([]byte, error) {
    cleanPath := filepath.Clean(filePath)
    // Validate path security...
    return os.ReadFile(cleanPath)
}
```

### SNMP Error Handling

```go
type SNMPError struct {
    OID   string
    Op    string
    Cause error
}

func (e SNMPError) Error() string {
    return fmt.Sprintf("SNMP %s failed for OID %s: %v", e.Op, e.OID, e.Cause)
}

func (e SNMPError) Unwrap() error {
    return e.Cause
}
```

## Performance Optimizations

### Ruler Package Optimizations

**Leverage fast-path optimizations:**

- Smart rule filtering based on input requirements
- Pre-compiled CUE expressions
- Expression result caching
- Object pooling with sync.Pool
- Batch processing for multiple inputs

**Performance targets:**

- Single evaluation: ~8μs per rule set
- Batch evaluation: ~4μs per input
- 10.5x faster than baseline implementations

### SNMP Translation Optimizations

**Use trie-based lookups for fast OID translation:**

- O(log n) lookup performance
- LRU cache for frequently accessed translations
- Lazy loading of MIB files to reduce startup time
- Batch operations for processing multiple OIDs efficiently

### Trap Processing Optimizations

**Worker pool pattern for concurrent processing:**

- Configurable worker pool size
- Non-blocking trap reception
- Template-based message formatting
- Efficient memory usage with object pooling
  })
  host, _ := manager.GetString("server.host")
  port, _ := manager.GetInt("server.port")

````

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
````

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
