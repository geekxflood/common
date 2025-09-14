---
type: "always_apply"
---

# Development Standards

This document defines the development standards for the Geekxflood Common project, covering Go code standards, package design, and testing practices.

## Go Code Standards

### Package Structure and Principles

- **Single Responsibility**: Each package has one clear, well-defined purpose
- **Minimal Dependencies**: Prefer standard library over external dependencies
- **Interface Design**: Define interfaces in consuming packages, not providing packages
- **Error Handling**: Always handle errors explicitly, never ignore them
- **Thread Safety**: Design for concurrent use from the start

### Naming Conventions

- **Packages**: lowercase, single words (`config`, `logging`, `ruler`, `snmptranslate`)
- **Types**: PascalCase (`Manager`, `ComponentLogger`, `EvaluationResult`, `Translator`)
- **Functions**: PascalCase for exported, camelCase for unexported
- **Variables**: camelCase, descriptive names (`configManager`, `evaluationResult`)
- **Constants**: PascalCase or UPPER_CASE for package-level constants

### Code Organization Template

```go
// Package declaration and documentation
package config

// Imports (standard library first, then external, then internal)
import (
    "context"
    "fmt"

    "cuelang.org/go/cue"

    "github.com/geekxflood/common/internal/helpers"
)

// Constants
const (
    DefaultTimeout = 30 * time.Second
)

// Types (interfaces first, then structs)
type Manager interface {
    Get(key string) (any, error)
    Close() error
}

type manager struct {
    schema *cue.Value
    config map[string]any
}

// Constructor functions
func NewManager(opts Options) (Manager, error) {
    // Implementation
}

// Methods (receiver methods grouped by type)
func (m *manager) Get(key string) (any, error) {
    // Implementation
}
```

### Function Design Principles

- **Keep Functions Small**: Aim for 20-30 lines maximum
- **Single Purpose**: Each function should do one thing well
- **Clear Parameters**: Use descriptive parameter names
- **Return Early**: Use early returns to reduce nesting
- **Context First**: Context should be the first parameter when used

### Error Handling Standards

```go
// Good: Explicit error handling with context
result, err := someOperation()
if err != nil {
    return fmt.Errorf("operation failed: %w", err)
}

// Good: Package-specific error types
type ValidationError struct {
    Field   string
    Message string
    Cause   error
}

func (e ValidationError) Error() string {
    return fmt.Sprintf("validation failed for %s: %s", e.Field, e.Message)
}

func (e ValidationError) Unwrap() error {
    return e.Cause
}

// Bad: Ignoring errors
result, _ := someOperation() // Never do this
```

## Package Design Principles

### Interface Design

#### Interface Placement

- **Define interfaces in consuming packages**, not providing packages
- **Keep interfaces small** - prefer many small interfaces over large ones
- **Use composition** to build larger interfaces from smaller ones

```go
// Good: Small, focused interface
type ConfigReader interface {
    Get(key string) (any, error)
    GetString(key string) (string, error)
}

// Good: Composed interface
type ConfigManager interface {
    ConfigReader
    ConfigWriter
    io.Closer
}
```

#### Interface Naming

- Use `-er` suffix for interfaces that describe behavior (`Reader`, `Writer`, `Manager`)
- Use descriptive names that clearly indicate purpose
- Avoid generic names like `Handler` or `Service`

### Dependency Management

#### Dependency Direction

- **Depend on abstractions**, not concretions
- **Higher-level packages** should not depend on lower-level packages
- **Use dependency injection** for external dependencies

```go
// Good: Depend on interface
type Manager struct {
    logger Logger  // Interface
    store  Store   // Interface
}

// Bad: Depend on concrete types
type Manager struct {
    logger *slog.Logger    // Concrete type
    store  *FileStore     // Concrete type
}
```

#### External Dependencies

- **Minimize external dependencies** - prefer standard library
- **Wrap external dependencies** behind interfaces
- **Pin dependency versions** in go.mod
- **Regularly update dependencies** for security

### File Organization

```text
package/
├── package.go              # Main types and interfaces
├── options.go              # Configuration options
├── errors.go               # Package-specific errors
├── internal/               # Internal implementation
│   ├── parser.go
│   └── validator.go
├── package_test.go         # Unit tests
├── integration_test.go     # Integration tests
├── benchmark_test.go       # Benchmarks
├── example_test.go         # Examples
├── testdata/              # Test data
│   ├── config.yaml
│   └── schema.cue
├── package.md             # Package documentation
└── README.md              # Quick reference (optional)
```

### API Design

#### Constructor Functions

- Use `New` prefix for constructors that return concrete types
- Use `NewXxx` for constructors that return specific types
- Return interfaces when possible to allow for different implementations

```go
// Good: Returns interface
func NewManager(opts Options) (Manager, error) {
    return &manager{...}, nil
}

// Good: Specific constructor
func NewComponentLogger(component, componentType string) *ComponentLogger {
    return &ComponentLogger{...}
}
```

#### Options Pattern

Use the options pattern for complex configuration:

```go
// Options struct for configuration
type Options struct {
    SchemaPath            string
    ConfigPath            string
    EnableSchemaHotReload bool
    EnableConfigHotReload bool
}

// Constructor with options
func NewManager(opts Options) (Manager, error) {
    // Validate and use options
}
```

## Testing Standards

### Testing Philosophy

#### Core Principles

- **Test-Driven Development**: Write tests before or alongside code
- **Comprehensive Coverage**: Aim for 80%+ test coverage
- **Fast Feedback**: Tests should run quickly and provide clear feedback
- **Reliable**: Tests should be deterministic and not flaky
- **Maintainable**: Tests should be easy to understand and modify

#### Testing Pyramid

1. **Unit Tests** (70%): Test individual functions and methods
2. **Integration Tests** (20%): Test component interactions
3. **End-to-End Tests** (10%): Test complete workflows

### Test Organization

#### File Structure

```text
package/
├── package.go
├── package_test.go          # Unit tests
├── integration_test.go      # Integration tests (if needed)
├── benchmark_test.go        # Benchmarks (if needed)
└── testdata/               # Test data files
    ├── config.yaml
    └── schema.cue
```

#### Test Naming

- **Test Files**: `*_test.go`
- **Test Functions**: `TestFunctionName`
- **Benchmark Functions**: `BenchmarkFunctionName`
- **Example Functions**: `ExampleFunctionName`

### Unit Testing Standards

#### Test Function Structure

```go
func TestManagerGetString(t *testing.T) {
    // Arrange
    manager := setupTestManager(t)
    
    // Act
    result, err := manager.GetString("test.key")
    
    // Assert
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if result != "expected" {
        t.Errorf("got %q, want %q", result, "expected")
    }
}
```

#### Table-Driven Tests

Use table-driven tests for multiple test cases:

```go
func TestValidateLevel(t *testing.T) {
    tests := []struct {
        name     string
        level    string
        expected bool
    }{
        {
            name:     "valid debug level",
            level:    "debug",
            expected: true,
        },
        {
            name:     "invalid level",
            level:    "invalid",
            expected: false,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := ValidateLevel(tt.level)
            if result != tt.expected {
                t.Errorf("ValidateLevel(%q) = %v, want %v", 
                    tt.level, result, tt.expected)
            }
        })
    }
}
```

#### Test Helpers

Create helper functions for common test setup:

```go
func setupTestManager(t *testing.T) *Manager {
    t.Helper()
    
    manager, err := NewManager(Options{
        SchemaContent: testSchema,
        ConfigPath:    "testdata/config.yaml",
    })
    if err != nil {
        t.Fatalf("failed to create test manager: %v", err)
    }
    
    t.Cleanup(func() {
        manager.Close()
    })
    
    return manager
}
```

### Integration Testing

#### Database Testing

```go
func TestDatabaseIntegration(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test in short mode")
    }
    
    db := setupTestDatabase(t)
    defer cleanupTestDatabase(t, db)
    
    // Test database operations
}
```

#### File System Testing

```go
func TestFileOperations(t *testing.T) {
    tempDir := t.TempDir() // Automatically cleaned up
    
    configPath := filepath.Join(tempDir, "config.yaml")
    err := writeTestConfig(configPath)
    if err != nil {
        t.Fatalf("failed to write test config: %v", err)
    }
    
    // Test file operations
}
```

### Benchmark Testing

#### Performance Benchmarks

```go
func BenchmarkManagerGetString(b *testing.B) {
    manager := setupBenchmarkManager(b)
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, err := manager.GetString("test.key")
        if err != nil {
            b.Fatal(err)
        }
    }
}
```

#### Memory Benchmarks

```go
func BenchmarkManagerGetString_Memory(b *testing.B) {
    manager := setupBenchmarkManager(b)
    
    b.ReportAllocs()
    b.ResetTimer()
    
    for i := 0; i < b.N; i++ {
        _, err := manager.GetString("test.key")
        if err != nil {
            b.Fatal(err)
        }
    }
}
```

## Performance Considerations

### Memory Management

- **Minimize allocations** in hot paths
- **Reuse objects** when possible (object pooling)
- **Use appropriate data structures** for the use case
- **Profile memory usage** regularly

```go
// Good: Reuse buffers
var bufferPool = sync.Pool{
    New: func() interface{} {
        return make([]byte, 0, 1024)
    },
}

func processData(data []byte) []byte {
    buf := bufferPool.Get().([]byte)
    defer bufferPool.Put(buf[:0])
    
    // Use buffer for processing
    return result
}
```

### Concurrency

- **Design for concurrency** from the start
- **Use channels** for communication between goroutines
- **Use mutexes** for protecting shared state
- **Avoid goroutine leaks** by ensuring proper cleanup

```go
// Good: Thread-safe design
type Manager struct {
    mu     sync.RWMutex
    config map[string]any
}

func (m *Manager) Get(key string) (any, error) {
    m.mu.RLock()
    defer m.mu.RUnlock()
    
    value, exists := m.config[key]
    if !exists {
        return nil, ErrKeyNotFound
    }
    return value, nil
}
```

## Test Execution

### Running Tests

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run tests with race detection
go test -race ./...

# Run benchmarks
go test -bench=. ./...

# Run specific test
go test -run TestManagerGetString ./config

# Run tests in short mode (skip slow tests)
go test -short ./...
```

### Test Coverage

```bash
# Generate coverage report
go test -coverprofile=coverage.out ./...

# View coverage in browser
go tool cover -html=coverage.out

# Check coverage percentage
go tool cover -func=coverage.out
```

## Quality Guidelines

### What Makes Good Code

- **Correctness**: Code does what it's supposed to do
- **Clarity**: Code is easy to understand and maintain
- **Consistency**: Code follows project conventions
- **Performance**: No obvious performance issues
- **Security**: No security vulnerabilities
- **Testability**: Code is easy to test

### What Makes Good Tests

- **Fast**: Tests should run quickly
- **Independent**: Tests should not depend on each other
- **Repeatable**: Tests should produce the same result every time
- **Self-Validating**: Tests should have clear pass/fail criteria
- **Timely**: Tests should be written close to when code is written

### Common Anti-Patterns to Avoid

- **Testing Implementation Details**: Test behavior, not implementation
- **Overly Complex Tests**: Keep tests simple and focused
- **Shared State**: Avoid global state that affects multiple tests
- **Flaky Tests**: Tests that sometimes pass and sometimes fail
- **Slow Tests**: Tests that take too long to run

## Documentation Requirements

- **Package Documentation**: Every package needs comprehensive docs
- **Function Documentation**: All exported functions must be documented
- **Example Code**: Include runnable examples
- **API Stability**: Document breaking changes clearly
- **Godoc Standards**: Follow Go documentation conventions

## Backwards Compatibility

### API Stability

- **Avoid breaking changes** in minor and patch releases
- **Use semantic versioning** to communicate compatibility
- **Deprecate before removing** - provide migration path
- **Document breaking changes** clearly in release notes

### Deprecation Process

1. **Mark as deprecated** with clear comments
2. **Provide alternative** - show what to use instead
3. **Grace period** - allow time for migration (minimum 2 releases)
4. **Remove** - clean removal with clear release notes

```go
// Deprecated: Use NewManager instead.
// This function will be removed in v2.0.0.
func CreateManager(opts Options) (*Manager, error) {
    return NewManager(opts)
}
```
