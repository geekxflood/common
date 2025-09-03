# Geekxflood Common

![Go Version](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go)[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

## Packages

- [**config**](#package-config) - CUE-powered configuration with hot reload and environment variables
- [**logging**](#package-logging) - Structured logging with context support
- [**ruler**](#package-ruler) - CUE expression evaluation with intelligent rule selection

## Installation

```bash
go get github.com/geekxflood/common/config
go get github.com/geekxflood/common/logging
go get github.com/geekxflood/common/ruler
```

## Package `config`

A package for comprehensive configuration management using CUE (cuelang.org) for schema definition and validation.

### Features

- ✅ **CUE Schema Validation** - Type-safe configuration with schema enforcement
- ✅ **Granular Hot Reload** - Watch schema and/or config files independently
- ✅ **Environment Variables** - `$VAR`, `${VAR}`, and `${VAR:-default}` patterns
- ✅ **Inline Schemas** - Embed CUE schemas directly in code
- ✅ **Multiple Formats** - YAML and JSON configuration files

### Quick Start

```go
package main

import (
    "log"
    "github.com/geekxflood/common/config"
)

func main() {
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

    // Access configuration
    host, _ := manager.GetString("server.host")
    port, _ := manager.GetInt("server.port")

    log.Printf("Server: %s:%d", host, port)
}
```

### Advanced Features

#### Inline Schema Content

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

#### Environment Variable Substitution

```yaml
# config.yaml
server:
  host: "${SERVER_HOST:-localhost}"
  port: $SERVER_PORT
database:
  url: "${DATABASE_URL:-sqlite://./app.db}"
```

#### Granular Hot Reload Control

```go
manager, err := config.NewManager(config.Options{
    SchemaPath:            "schema.cue",
    ConfigPath:            "config.yaml",
    EnableSchemaHotReload: true, // Watch schema file
    EnableConfigHotReload: false, // Don't watch config file
})
```

## Package `logging`

A package for structured logging using Go's standard log/slog package.

### Features

- ✅ **Structured Output** - JSON and logfmt formats
- ✅ **Component Isolation** - Separate loggers for different parts of your app
- ✅ **Context Integration** - Automatic extraction of trace IDs and user info
- ✅ **Multiple Outputs** - Console and file output with automatic directory creation
- ✅ **Dynamic Configuration** - Runtime log level changes

### Quick Start

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

## Package `ruler`

A package for evaluating CUE expressions against structured input data with intelligent rule selection and type-safe outputs.

### Features

- ✅ **Structured Rules** - InputSpec and OutputSpec for self-documenting, type-safe rules
- ✅ **Smart Rule Selection** - Rules pre-filtered based on available input data
- ✅ **Type Safety** - Input and output validation prevents runtime errors
- ✅ **Thread-Safe** - Concurrent evaluation support with optimized locking
- ✅ **CUE Integration** - Leverages CUE's powerful type system and validation
- ✅ **Fast-Path Optimization** - Common patterns bypass full CUE evaluation

### Quick Start

```go
package main

import (
    "context"
    "log"
    "github.com/geekxflood/common/ruler"
)

func main() {
    // Create configuration
    config := map[string]any{
        "config": map[string]any{
            "enabled":         true,
            "default_message": "no rule found",
            "timeout":         "20ms",
        },
        "rules": []any{
            map[string]any{
                "name": "high_cpu_alert",
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

    // Create ruler and evaluate
    ruler, err := ruler.NewRuler(config)
    if err != nil {
        log.Fatal(err)
    }

    inputs := ruler.Inputs{"cpu_usage": 85.5}
    result, err := ruler.Evaluate(context.Background(), inputs)
    if err != nil {
        log.Fatal(err)
    }

    if result.Output != nil {
        log.Printf("Rule matched: %s", result.MatchedRule.Name)
    }
}
```

## Development

### Testing

```bash
# Run all tests
go test ./...

# Run tests with race detection
go test -race ./...

# Run benchmarks
go test -bench=. ./...

# Test specific package
go test ./config -v
```

### Code Quality

```bash
# Lint code
golangci-lint run --config .golangci.yml

# Security scan
gosec -conf .gosec.json ./...

# Check for dead code
deadcode -test ./...
```

### Documentation

Each package includes comprehensive documentation:

- **Package Documentation** - See individual `*.md` files in each package directory
- **API Reference** - Available at [pkg.go.dev](https://pkg.go.dev/github.com/geekxflood/common)
- **Examples** - Included in package documentation and tests

## License

MIT License - see [LICENSE](LICENSE) file for details.
