# Package `ruler`

A package for evaluating CUE expressions against structured input data with intelligent rule selection and type-safe outputs.

## Key Features

✅ **Structured Rules** - InputSpec and OutputSpec for self-documenting, type-safe rules
✅ **Smart Rule Selection** - Rules pre-filtered based on available input data
✅ **Type Safety** - Input and output validation prevents runtime errors
✅ **Thread-Safe** - Concurrent evaluation support with optimized locking
✅ **CUE Integration** - Leverages CUE's powerful type system and validation
✅ **Fast-Path Optimization** - Common patterns bypass full CUE evaluation
✅ **Batch Processing** - Efficient evaluation of multiple input sets

## Installation

```bash
go get github.com/geekxflood/common/ruler
```

## Architecture

The package consists of four main components:

1. **Configuration Validation** - Uses embedded CUE schemas with InputSpec and OutputSpec support
2. **Smart Rule Selection** - Filters rules based on input requirements before evaluation
3. **Expression Evaluation** - Leverages CUE's expression language with input field bindings
4. **Structured Output Generation** - Produces type-safe outputs with field specifications

## Package Structure

```text
ruler/
├── ruler.go          # Main implementation with types and methods
├── ruler_test.go     # Comprehensive unit tests and benchmarks
├── ruler.md          # This documentation file
└── schema/
    └── ruler.cue     # CUE schema for configuration validation
```

## Quick Start

### Basic Usage

The ruler accepts structured configuration objects with InputSpec and OutputSpec definitions:

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
            "default_message": "no rule found for contents",
            "timeout":         "20ms",
        },
        "rules": []any{
            map[string]any{
                "name": "high_cpu_alert",
                "description": "Triggers when CPU usage exceeds threshold",
                "inputs": []any{
                    map[string]any{
                        "name": "cpu_usage",
                        "type": "number",
                        "required": true,
                        "description": "CPU usage percentage",
                    },
                },
                "expr": `cpu_usage > 80`,
                "outputs": []any{
                    map[string]any{
                        "name": "alert",
                        "description": "High CPU usage alert",
                        "fields": map[string]any{
                            "severity": map[string]any{
                                "type": "string",
                                "default": "high",
                            },
                        },
                    },
                },
            },
        },
    }

    // Create ruler instance
    ruler, err := ruler.NewRuler(config)
    if err != nil {
        log.Fatal("Failed to create ruler:", err)
    }

    // Prepare input data
    inputs := ruler.Inputs{
        "cpu_usage": 85.5,
        "hostname": "server1",
    }

    // Evaluate rules
    ctx := context.Background()
    result, err := ruler.Evaluate(ctx, inputs)
    if err != nil {
        log.Fatal("Evaluation failed:", err)
    }

    // Handle results
    if result.Output != nil {
        fmt.Printf("Rule matched: %s\n", result.MatchedRule.Name)
        fmt.Printf("Duration: %v\n", result.Duration)
    } else {
        fmt.Printf("No rule matched: %s\n", result.DefaultMessage)
    }
}
```

## Configuration Reference

### Config Structure

The configuration consists of two main sections:

```go
config := map[string]any{
    "config": map[string]any{
        "enabled":         true,
        "default_message": "no rule found for contents",
        "timeout":         "20ms",
    },
    "rules": []any{
        // Rule definitions...
    },
}
```

### Rule Definition

Each rule contains the following fields:

```go
map[string]any{
    "name":        "rule_name",        // Required: Unique identifier
    "description": "Rule description", // Optional: Human-readable description
    "expr":        "cpu_usage > 80",   // Required: CUE expression
    "inputs": []any{
        map[string]any{
            "name":        "cpu_usage",
            "type":        "number",
            "required":    true,
            "description": "CPU usage percentage",
        },
    },
    "outputs": []any{
        map[string]any{
            "name":        "alert",
            "description": "Alert information",
            "fields": map[string]any{
                "severity": map[string]any{
                    "type":    "string",
                    "default": "high",
                },
            },
        },
    },
    "enabled":  true, // Optional: Enable/disable rule
    "priority": 10,   // Optional: Execution priority
}
```

## Expression Language

Rules use CUE expressions with direct access to input fields as variables:

```cue
// Simple comparison
cpu_usage > 80

// Multiple conditions
cpu_usage > 80 && hostname == "server1"

// Array operations
len(error_codes) > 2

// String pattern matching
hostname =~ "^server[0-9]+$"

// Complex logic
(cpu_usage > 80 || memory_usage > 90) && environment == "production"
```

## API Reference

### Types

#### Inputs

```go
type Inputs map[string]any
```

Represents input data for rule evaluation as a key-value map.

#### EvaluationResult

```go
type EvaluationResult struct {
    Output         *RulerOutput  // Structured output (nil if no match)
    MatchedRule    *MatchedRule  // Matched rule metadata (nil if no match)
    Duration       time.Duration // Evaluation duration
    Timestamp      time.Time     // Evaluation start time
    InputCount     int           // Number of input entries processed
    RulesChecked   int           // Number of rules evaluated
    DefaultMessage string        // Default message (when no match)
}
```

#### Ruler

```go
type Ruler struct {
    // Rule evaluation engine
}
```

Main ruler instance for evaluating rules against input data.

### Functions

#### NewRuler

```go
func NewRuler(configObj any) (*Ruler, error)
```

Creates a new Ruler instance from a configuration object.

#### Evaluate

```go
func (r *Ruler) Evaluate(ctx context.Context, inputData Inputs) (*EvaluationResult, error)
```

Evaluates input data against configured rules. Thread-safe for concurrent use.

## Performance

The ruler is optimized for ultra-high-performance scenarios:

- **Concurrent Evaluation** - Thread-safe for multiple goroutines
- **Pre-compiled Rules** - CUE expressions are compiled once during initialization
- **Smart Rule Selection** - Rules pre-filtered based on available input data
- **Fast-Path Optimization** - Common expression patterns bypass CUE evaluation
- **Object Pooling** - Result objects are pooled to reduce allocations
- **Ultra-Fast Execution** - ~8μs per evaluation with smart filtering

## Use Cases

### Alert Processing

```go
// Alert data
inputs := ruler.Inputs{
    "cpu_usage": 95.5,
    "hostname":  "web01",
    "service":   "api",
}

// Rule configuration
config := map[string]any{
    "rules": []any{
        map[string]any{
            "name": "high_cpu_alert",
            "expr": `cpu_usage > 90`,
            "outputs": []any{
                map[string]any{
                    "name": "alert",
                    "fields": map[string]any{
                        "severity": map[string]any{"default": "critical"},
                        "message":  map[string]any{"default": "High CPU usage detected"},
                    },
                },
            },
        },
    },
}
```

### Log Analysis

```go
// Log entry data
inputs := ruler.Inputs{
    "level":   "ERROR",
    "service": "user-api",
    "count":   5,
}

// Rule to detect critical errors
rule := map[string]any{
    "expr": `level == "ERROR" && count > 3`,
    "outputs": []any{
        map[string]any{
            "name": "incident",
            "fields": map[string]any{
                "severity": map[string]any{"default": "high"},
                "runbook":  map[string]any{"default": "error-handling-guide"},
            },
        },
    },
}
```

## Testing

The package includes comprehensive tests covering:

- Configuration validation
- Expression evaluation
- Output generation
- Smart rule selection
- Error handling and edge cases
- Performance benchmarks

Run tests:

```bash
cd ruler/
go test -v
go test -bench=.
go test -race
```

## Design Principles

1. **Separation of Concerns** - Configuration parsing handled by consuming applications
2. **Type Safety** - Strong typing with CUE schema validation
3. **Performance First** - Optimized for high-throughput scenarios
4. **Thread Safety** - Safe for concurrent use without external synchronization
5. **Structured Results** - Clean, predictable output formats
6. **No Side Effects** - Pure evaluation without logging or I/O operations

## Dependencies

The package has minimal external dependencies:

- `cuelang.org/go/cue` - CUE language support for expressions and validation
- Go standard library - `context`, `sync`, `time`, `fmt`

No external YAML/JSON parsing libraries are used - configuration parsing and output marshalling are the responsibilities of the consuming application.
