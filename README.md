# Geekxflood Common

![Go Version](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go)[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

## Packages

- [**config**](#package-config) - CUE-powered configuration with hot reload and environment variables
- [**logging**](#package-logging) - Structured logging with context support
- [**ruler**](#package-ruler) - CUE expression evaluation with intelligent rule selection
- [**snmptranslate**](#package-snmptranslate) - MIB-based OID translation for SNMP operations
- [**trapprocessor**](#package-trapprocessor) - SNMP trap processing with template engine and worker pools

## Installation

```bash
go get github.com/geekxflood/common/config
go get github.com/geekxflood/common/logging
go get github.com/geekxflood/common/ruler
go get github.com/geekxflood/common/snmptranslate
go get github.com/geekxflood/common/trapprocessor
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

## Package `snmptranslate`

A package for MIB-based OID translation functionality that provides efficient, scalable SNMP OID-to-name translation.

### Features

✅ **MIB File Processing** - Parse and load MIB files from directories to build OID-to-name mappings
✅ **Efficient Data Structures** - Trie-based OID storage for O(log n) lookup performance
✅ **Lazy Loading** - Load MIB files on-demand to reduce startup time and memory usage
✅ **LRU Caching** - Intelligent caching of frequently accessed translations
✅ **Batch Operations** - Process multiple OIDs efficiently in single operations
✅ **Thread-Safe** - Concurrent access support with optimized locking

### Quick Start

```go
package main

import (
    "log"
    "github.com/geekxflood/common/snmptranslate"
)

func main() {
    // Create translator with default configuration
    translator := snmptranslate.New()

    // Initialize with MIB directory
    if err := translator.Init("/usr/share/snmp/mibs"); err != nil {
        log.Fatal(err)
    }
    defer translator.Close()

    // Translate single OID
    name, err := translator.Translate(".1.3.6.1.6.3.1.1.5.1")
    if err != nil {
        log.Printf("Translation failed: %v", err)
    } else {
        log.Printf("OID translates to: %s", name) // "coldStart"
    }

    // Batch translation
    oids := []string{
        ".1.3.6.1.6.3.1.1.5.1",
        ".1.3.6.1.6.3.1.1.5.2",
        ".1.3.6.1.6.3.1.1.5.3",
    }

    results, err := translator.TranslateBatch(oids)
    if err != nil {
        log.Printf("Batch translation errors: %v", err)
    }

    for oid, name := range results {
        log.Printf("%s -> %s", oid, name)
    }
}
```

## Package `trapprocessor`

A package for consolidated SNMP trap processing with template engine and core utilities.

### Features

✅ **SNMP Trap Processing** - Complete SNMP trap listening, parsing, and processing
✅ **Template Engine** - Built-in template processing for trap formatting
✅ **Worker Pool** - Configurable worker pool for concurrent trap processing
✅ **Multiple SNMP Versions** - Support for SNMPv1, SNMPv2c, and SNMPv3
✅ **Flexible Configuration** - Map-based configuration with sensible defaults
✅ **Production Ready** - Thread-safe operations with comprehensive error handling

### Quick Start

```go
package main

import (
    "context"
    "log"
    "github.com/geekxflood/common/trapprocessor"
)

func main() {
    // Create configuration
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
    }

    // Create and start trap processor
    processor, err := trapprocessor.New(config)
    if err != nil {
        log.Fatal(err)
    }

    ctx := context.Background()
    if err := processor.Start(ctx); err != nil {
        log.Fatal(err)
    }
    defer processor.Stop()

    log.Println("SNMP trap processor started")
    // Process traps...
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
