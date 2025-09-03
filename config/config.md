# Package `config`

A package for comprehensive configuration management using CUE (cuelang.org) for schema definition and validation.

## Key Features

✅ **CUE Schema Validation** - Type-safe configuration with schema enforcement
✅ **Granular Hot Reload** - Watch schema and/or config files independently
✅ **Environment Variables** - `$VAR`, `${VAR}`, and `${VAR:-default}` patterns
✅ **Inline Schemas** - Embed CUE schemas directly in code
✅ **Multiple Formats** - YAML and JSON configuration files
✅ **Default Values** - Schema-defined defaults with override capability
✅ **Thread-Safe Operations** - Safe for concurrent use
✅ **Configuration-Driven Development** - Dynamic component creation based on configuration

## Installation

```bash
go get github.com/geekxflood/common/config
```

## Architecture

The package consists of three main components:

1. **Configuration Manager** - Central configuration management with validation and hot reload
2. **CUE Schema Integration** - Embedded schema validation and type checking
3. **File System Monitoring** - Automatic detection of configuration file changes

## Package Structure

```text
config/
├── config.go          # Main implementation with Manager and interfaces
├── config_test.go     # Comprehensive unit tests
└── config.md          # This documentation file
```

## Quick Start

### Basic Usage

```go
package main

import (
    "log"
    "github.com/geekxflood/common/config"
)

func main() {
    // Create configuration manager
    manager, err := config.NewManager(config.Options{
        SchemaPath:            "schema.cue",
        ConfigPath:            "config.yaml",
        EnableSchemaHotReload: true,
        EnableConfigHotReload: true,
    })
    if err != nil {
        log.Fatal(err)
    }
    defer manager.Close()

    // Access configuration values
    host, _ := manager.GetString("server.host")
    port, _ := manager.GetInt("server.port")

    log.Printf("Server starting on %s:%d", host, port)
}
```

### Inline Schema Content

```go
manager, err := config.NewManager(config.Options{
    SchemaContent: `
server: {
    host: string | *"localhost"
    port: int & >=1024 & <=65535 | *8080
}
`,
    ConfigPath:            "config.yaml",
    EnableConfigHotReload: true, // Only config file watched
})
```

### Environment Variable Substitution

```yaml
# config.yaml
server:
  host: "${SERVER_HOST:-localhost}"
  port: $SERVER_PORT
database:
  url: "${DATABASE_URL:-sqlite://./app.db}"
```

### Hot Reload

```go
// Register change callback
manager.OnConfigChange(func(err error) {
    if err != nil {
        log.Printf("Config reload failed: %v", err)
    } else {
        log.Println("Configuration reloaded successfully")
        // Reconfigure application components
        reconfigureComponents()
    }
})
```

## Configuration Reference

### Options Structure

The `Options` struct configures the configuration manager:

```go
type Options struct {
    // SchemaPath specifies the path to the CUE schema file or directory
    SchemaPath string

    // SchemaContent specifies the CUE schema content directly as a string
    SchemaContent string

    // ConfigPath specifies the path to the configuration file (YAML or JSON)
    ConfigPath string

    // EnableSchemaHotReload determines whether to watch the schema file for changes
    EnableSchemaHotReload bool

    // EnableConfigHotReload determines whether to watch the config file for changes
    EnableConfigHotReload bool

    // HotReloadContext provides the context for hot reload operations
    HotReloadContext context.Context
}
```

### Value Access Methods

The Manager provides type-safe methods for accessing configuration values:

```go
// String values
host, err := manager.GetString("server.host")
timeout, err := manager.GetString("server.timeout")

// Numeric values
port, err := manager.GetInt("server.port")
maxConn, err := manager.GetInt("database.max_connections")

// Boolean values
enabled, err := manager.GetBool("features.feature_a")

// Complex values
serverConfig, err := manager.GetMap("server")
features, err := manager.GetMap("features")

// Raw values
rawValue, err := manager.Get("database")
```

## API Reference

### Types

#### Manager

```go
type Manager interface {
    Get(key string) (any, error)
    GetString(key string) (string, error)
    GetInt(key string) (int, error)
    GetBool(key string) (bool, error)
    GetMap(key string) (map[string]any, error)
    GetDuration(key string) (time.Duration, error)

    StartHotReload(ctx context.Context) error
    StopHotReload() error
    OnConfigChange(callback func(error))

    Reload() error
    Validate() error
    Close() error
}
```

The main configuration manager interface that handles loading, validation, and access.

### Functions

#### NewManager

```go
func NewManager(options Options) (Manager, error)
```

Creates a new configuration manager with the specified options. Validates the configuration against the schema during creation.

## Use Cases

### Web Service Configuration

```go
package main

import (
    "net/http"
    "github.com/geekxflood/common/config"
)

func main() {
    manager, err := config.NewManager(config.Options{
        SchemaPath:            "service-schema.cue",
        ConfigPath:            "service-config.yaml",
        EnableSchemaHotReload: true,
        EnableConfigHotReload: true,
    })
    if err != nil {
        log.Fatal(err)
    }
    defer manager.Close()

    // Configure HTTP server
    host, _ := manager.GetString("server.host")
    port, _ := manager.GetInt("server.port")
    timeout, _ := manager.GetDuration("server.timeout")

    server := &http.Server{
        Addr:         fmt.Sprintf("%s:%d", host, port),
        ReadTimeout:  timeout,
        WriteTimeout: timeout,
    }

    log.Printf("Starting server on %s:%d", host, port)
    server.ListenAndServe()
}
```

### Feature Flag Management

```go
// Dynamic feature flag evaluation
type FeatureManager struct {
    config config.Manager
}

func (fm *FeatureManager) IsEnabled(feature string) bool {
    enabled, err := fm.config.GetBool(fmt.Sprintf("features.%s", feature))
    return err == nil && enabled
}

// Usage in application
if featureManager.IsEnabled("new_payment_flow") {
    return handleNewPaymentFlow(request)
}
return handleLegacyPaymentFlow(request)
```

## Testing

The package includes comprehensive tests covering:

- Configuration loading and validation
- Schema validation with various constraint types
- Granular hot reload functionality
- Environment variable substitution
- Inline schema content support
- Error handling and edge cases

Run tests:

```bash
cd config/
go test -v
go test -race
```

## Design Principles

1. **Configuration-Driven Development** - Application behavior driven by configuration, not code
2. **Schema-First Approach** - CUE schemas define structure and constraints
3. **Type Safety** - Strong typing prevents configuration errors
4. **Granular Hot Reload** - Independent control over schema and config file watching
5. **Environment Integration** - Seamless environment variable support with defaults
6. **Validation First** - Invalid configurations are rejected early

## Dependencies

The package has minimal external dependencies:

- `cuelang.org/go/cue` - CUE language support for schema validation
- `github.com/fsnotify/fsnotify` - File system monitoring for hot reload
- Go standard library - `context`, `sync`, `time`, `fmt`

## Performance

The config package is optimized for production use:

- **Lazy Loading** - Configuration values are parsed on demand
- **Caching** - Parsed values are cached for subsequent access
- **Efficient Watching** - File system monitoring with minimal overhead
- **Atomic Updates** - Configuration changes are applied atomically

Configuration access is typically sub-microsecond after initial parsing and caching.
