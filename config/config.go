// Package config provides comprehensive configuration management
// using CUE (cuelang.org) for schema definition and validation.
//
// This package implements configuration-driven development principles where
// application behavior is primarily driven by configuration rather than
// hardcoded logic. It provides CUE-based schema validation, granular hot reload
// capabilities, environment variable support, and type-safe configuration access.
//
// # Key Features
//
//   - CUE-based schema definition and validation
//   - Support for YAML and JSON configuration files
//   - Granular hot reload (schema and/or config files independently)
//   - Environment variable substitution with default values ($VAR, ${VAR}, ${VAR:-default})
//   - Inline schema content support
//   - Schema-defined default values
//   - Thread-safe operations
//
// # Basic Usage
//
// Initialize with a CUE schema and configuration file:
//
//	manager, err := config.NewManager(config.Options{
//		SchemaPath:            "schema.cue",
//		ConfigPath:            "config.yaml",
//		EnableSchemaHotReload: true,
//		EnableConfigHotReload: true,
//	})
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer manager.Close()
//
//	// Access configuration values
//	host, _ := manager.GetString("server.host")
//	port, _ := manager.GetInt("server.port")
//
// # Inline Schema Content
//
// Use inline schema instead of loading from file:
//
//	manager, err := config.NewManager(config.Options{
//		SchemaContent: `
//			server: {
//				host: string | *"localhost"
//				port: int & >=1024 & <=65535 | *8080
//			}
//		`,
//		ConfigPath:            "config.yaml",
//		EnableConfigHotReload: true, // Only config file watched
//	})
//
// # Environment Variables
//
// Configuration files support environment variable substitution:
//
//	# config.yaml
//	server:
//	  host: "${SERVER_HOST:-localhost}"
//	  port: $SERVER_PORT
//	database:
//	  url: "${DATABASE_URL:-sqlite://./app.db}"
//
// # Granular Hot Reload
//
// Control which files are watched independently:
//
//	manager.OnConfigChange(func(err error) {
//		if err != nil {
//			log.Printf("Config reload failed: %v", err)
//		} else {
//			log.Println("Configuration reloaded")
//		}
//	})
package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"
	"cuelang.org/go/encoding/yaml"
	"github.com/fsnotify/fsnotify"
)

// Provider defines the interface for accessing configuration values.
//
// Provider provides type-safe access to configuration data with support for
// dot-notation paths and default values. All methods return typed values
// extracted from the merged configuration (schema defaults + user config).
//
// Example:
//
//	host, err := provider.GetString("server.host")
//	port, err := provider.GetInt("server.port", 8080)
//	enabled, err := provider.GetBool("features.auth")
//
// The defaultValue parameters are optional and are used when the path
// does not exist in the configuration.
type Provider interface {
	// GetString returns the string value at the specified path.
	// It uses dot notation (e.g., "server.host") for nested access.
	// If the path doesn't exist, it returns the first defaultValue if provided,
	// otherwise it returns an error.
	GetString(path string, defaultValue ...string) (string, error)

	// GetInt returns the integer value at the specified path.
	// It uses dot notation (e.g., "server.port") for nested access.
	// If the path doesn't exist, it returns the first defaultValue if provided,
	// otherwise it returns an error.
	GetInt(path string, defaultValue ...int) (int, error)

	// GetFloat returns the float64 value at the specified path.
	// It uses dot notation (e.g., "metrics.threshold") for nested access.
	// If the path doesn't exist, it returns the first defaultValue if provided,
	// otherwise it returns an error.
	GetFloat(path string, defaultValue ...float64) (float64, error)

	// GetBool returns the boolean value at the specified path.
	// It uses dot notation (e.g., "features.enabled") for nested access.
	// If the path doesn't exist, it returns the first defaultValue if provided,
	// otherwise it returns an error.
	GetBool(path string, defaultValue ...bool) (bool, error)

	// GetDuration returns the time.Duration value at the specified path.
	// The underlying value should be a string parseable by time.ParseDuration.
	// It uses dot notation for nested access.
	// If the path doesn't exist, it returns the first defaultValue if provided,
	// otherwise it returns an error.
	GetDuration(path string, defaultValue ...time.Duration) (time.Duration, error)

	// GetStringSlice returns the string slice value at the specified path.
	// It uses dot notation (e.g., "server.allowedHosts") for nested access.
	// If the path doesn't exist, it returns the first defaultValue if provided,
	// otherwise it returns an error.
	GetStringSlice(path string, defaultValue ...[]string) ([]string, error)

	// GetMap returns the map value at the specified path.
	// This is useful for accessing complex nested structures.
	// It uses dot notation for nested access.
	GetMap(path string) (map[string]any, error)

	// Exists reports whether the specified configuration path exists.
	// It uses dot notation for path specification.
	Exists(path string) bool

	// Validate validates the current configuration against the CUE schema.
	// It returns an error if validation fails.
	Validate() error
}

// Manager defines the interface for configuration lifecycle management.
//
// Manager extends Provider with hot reload capabilities, change notifications,
// and resource cleanup. It orchestrates the complete configuration lifecycle
// from loading to cleanup.
//
// Example:
//
//	manager, err := config.NewManager(options)
//	if err != nil {
//		return err
//	}
//	defer manager.Close()
//
//	// Enable hot reload
//	ctx := context.Background()
//	manager.StartHotReload(ctx)
//	manager.OnConfigChange(func(err error) {
//		if err != nil {
//			log.Printf("Reload failed: %v", err)
//		}
//	})
type Manager interface {
	Provider

	// StartHotReload enables automatic configuration reloading when files change.
	// The provided context controls the hot reload lifecycle and can be used to stop it.
	StartHotReload(ctx context.Context) error

	// StopHotReload disables automatic configuration reloading.
	StopHotReload()

	// OnConfigChange registers a callback function that is invoked
	// when configuration changes are detected. The callback receives
	// any error that occurred during reload.
	OnConfigChange(callback func(error))

	// Reload manually reloads the configuration from files.
	// It validates the new configuration before applying it.
	Reload() error

	// Close releases all resources and stops hot reload if active.
	Close() error
}

// Validator defines the interface for configuration validation.
//
// Validator provides methods for validating configuration data against
// CUE schemas, supporting both complete configurations and individual values.
type Validator interface {
	// ValidateConfig validates a complete configuration against the schema.
	// It returns an error if the configuration does not conform to the schema.
	ValidateConfig(config map[string]any) error

	// ValidateValue validates a specific value at a path against the schema.
	// It returns an error if the value does not conform to the schema at that path.
	ValidateValue(path string, value any) error

	// ValidateFile validates a configuration file without loading it.
	// It returns an error if the file does not conform to the schema.
	ValidateFile(configPath string) error
}

// SchemaLoader defines the interface for loading and managing CUE schemas.
//
// SchemaLoader handles loading CUE schema files from the filesystem or
// from inline content, validating them, and providing schema information
// for validation and default value extraction.
//
// Example:
//
//	loader := NewSchemaLoader()
//	err := loader.LoadSchema("./schema/config.cue")
//	if err != nil {
//		log.Fatal(err)
//	}
//	defaults, err := loader.GetDefaults()
//	validator, err := loader.GetValidator()
//
// Or with inline content:
//
//	loader := NewSchemaLoader()
//	err := loader.LoadSchemaContent("server: { host: string | *\"localhost\" }")
//	if err != nil {
//		log.Fatal(err)
//	}
//	defaults, err := loader.GetDefaults()
type SchemaLoader interface {
	// LoadSchema loads a CUE schema from the specified file or directory path.
	// It validates the schema and prepares it for use in validation and
	// default value extraction.
	LoadSchema(schemaPath string) error

	// LoadSchemaContent loads a CUE schema from the provided content string.
	// It validates the schema and prepares it for use in validation and
	// default value extraction.
	LoadSchemaContent(schemaContent string) error

	// GetDefaults extracts default values from the loaded schema.
	// It returns a map containing all default values defined in the schema.
	GetDefaults() (map[string]any, error)

	// GetValidator returns a validator for the loaded schema.
	// The validator can be used to validate configurations against the schema.
	GetValidator() (Validator, error)
}

// ChangeNotifier defines the interface for configuration change notifications.
//
// ChangeNotifier provides a way to register callbacks that are invoked
// when configuration changes are detected, typically during hot reload.
type ChangeNotifier interface {
	// OnChange registers a callback that is invoked when configuration changes.
	// The callback receives any error that occurred during the change detection.
	OnChange(callback func(error))

	// NotifyChange triggers all registered change callbacks with the given error.
	// If err is nil, it indicates a successful configuration change.
	NotifyChange(err error)
}

// Options defines configuration options for creating a Manager.
//
// Options contains all settings needed to initialize a configuration manager
// with schema and configuration file paths, hot reload behavior, and other
// operational parameters.
//
// Example:
//
//	options := config.Options{
//		SchemaPath:            "./schema/config.cue",
//		ConfigPath:            "./config.yaml",
//		EnableSchemaHotReload: true,
//		EnableConfigHotReload: true,
//	}
//	manager, err := config.NewManager(options)
//
// Or with inline schema content:
//
//	options := config.Options{
//		SchemaContent:         "server: { host: string | *\"localhost\", port: int | *8080 }",
//		ConfigPath:            "./config.yaml",
//		EnableConfigHotReload: true, // Only config file will be watched
//	}
//	manager, err := config.NewManager(options)
type Options struct {
	// SchemaPath specifies the path to the CUE schema file or directory.
	// Either SchemaPath or SchemaContent must be provided, but not both.
	SchemaPath string

	// SchemaContent specifies the CUE schema content directly as a string.
	// Either SchemaPath or SchemaContent must be provided, but not both.
	// When using SchemaContent, hot reload will only watch the config file,
	// not the schema (since it's provided inline).
	SchemaContent string

	// ConfigPath specifies the path to the configuration file (YAML or JSON).
	// If empty, only schema defaults will be used and no user configuration
	// will be loaded.
	ConfigPath string

	// EnableSchemaHotReload determines whether to watch the schema file for changes.
	// Only works when SchemaPath is provided (ignored when using SchemaContent).
	// When true, schema file changes will trigger a full reload.
	EnableSchemaHotReload bool

	// EnableConfigHotReload determines whether to watch the config file for changes.
	// Only works when ConfigPath is provided.
	// When true, config file changes will trigger a reload with the current schema.
	EnableConfigHotReload bool

	// HotReloadContext provides the context for hot reload operations.
	// If nil, a background context will be used when any hot reload is enabled.
	// The context can be used to cancel hot reload operations.
	HotReloadContext context.Context
}

// cueSchemaLoader implements the SchemaLoader interface using CUE.
// It provides thread-safe loading and management of CUE schemas.
type cueSchemaLoader struct {
	ctx           *cue.Context
	schemaValue   cue.Value
	schemaPath    string
	schemaContent string
	isFromContent bool
	mu            sync.RWMutex
}

// NewSchemaLoader creates a new CUE-based schema loader.
//
// The returned loader can load CUE schemas from files or directories,
// validate them, and extract default values for configuration.
//
// Example:
//
//	loader := config.NewSchemaLoader()
//	err := loader.LoadSchema("./schema/config.cue")
//	if err != nil {
//		log.Fatal(err)
//	}
//	defaults, err := loader.GetDefaults()
func NewSchemaLoader() SchemaLoader {
	return &cueSchemaLoader{
		ctx: cuecontext.New(),
	}
}

// LoadSchema loads a CUE schema from the specified file or directory path.
// It validates the schema syntax and prepares it for use.
// Returns an error if the path is invalid, the file doesn't exist,
// or the schema contains syntax errors.
func (l *cueSchemaLoader) LoadSchema(schemaPath string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Clean and validate the schema path
	cleanPath := filepath.Clean(schemaPath)
	if !filepath.IsAbs(cleanPath) {
		absPath, err := filepath.Abs(cleanPath)
		if err != nil {
			return fmt.Errorf("failed to resolve schema path %s: %w", schemaPath, err)
		}
		cleanPath = absPath
	}

	// Check if path exists
	if _, err := os.Stat(cleanPath); err != nil {
		return fmt.Errorf("schema path %s does not exist: %w", cleanPath, err)
	}

	l.schemaPath = cleanPath

	// Load the CUE schema
	var schemaValue cue.Value
	var err error

	// Check if it's a directory or file
	info, err := os.Stat(cleanPath)
	if err != nil {
		return fmt.Errorf("failed to stat schema path %s: %w", cleanPath, err)
	}

	if info.IsDir() {
		// Load from directory
		schemaValue, err = l.loadFromDirectory(cleanPath)
	} else {
		// Load from single file
		schemaValue, err = l.loadFromFile(cleanPath)
	}

	if err != nil {
		return fmt.Errorf("failed to load CUE schema from %s: %w", cleanPath, err)
	}

	// Validate the loaded schema
	if err := schemaValue.Err(); err != nil {
		return fmt.Errorf("invalid CUE schema: %w", err)
	}

	l.schemaValue = schemaValue
	return nil
}

// LoadSchemaContent loads a CUE schema from the provided content string.
// It validates the schema syntax and prepares it for use.
// Returns an error if the content is empty or contains syntax errors.
func (l *cueSchemaLoader) LoadSchemaContent(schemaContent string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if schemaContent == "" {
		return errors.New("schema content cannot be empty")
	}

	// Compile the CUE content directly
	schemaValue := l.ctx.CompileString(schemaContent, cue.Filename("inline-schema"))
	if err := schemaValue.Err(); err != nil {
		return fmt.Errorf("failed to compile CUE schema content: %w", err)
	}

	// Validate the loaded schema
	if err := schemaValue.Err(); err != nil {
		return fmt.Errorf("invalid CUE schema: %w", err)
	}

	l.schemaContent = schemaContent
	l.schemaPath = ""
	l.isFromContent = true
	l.schemaValue = schemaValue
	return nil
}

// loadFromFile loads a CUE schema from a single file.
func (l *cueSchemaLoader) loadFromFile(filePath string) (cue.Value, error) {
	// Securely read the file content with validation
	content, err := safeReadFile(filePath)
	if err != nil {
		return cue.Value{}, fmt.Errorf("failed to read schema file: %w", err)
	}

	// Compile the CUE content
	schemaValue := l.ctx.CompileBytes(content, cue.Filename(filePath))
	if err := schemaValue.Err(); err != nil {
		return cue.Value{}, fmt.Errorf("failed to compile CUE schema: %w", err)
	}

	return schemaValue, nil
}

// loadFromDirectory loads CUE schemas from a directory.
func (l *cueSchemaLoader) loadFromDirectory(dirPath string) (cue.Value, error) {
	// Use CUE's load package to load from directory
	buildInstances := load.Instances([]string{dirPath}, &load.Config{
		Dir: dirPath,
	})

	if len(buildInstances) == 0 {
		return cue.Value{}, fmt.Errorf("no CUE files found in directory %s", dirPath)
	}

	// Check for errors in build instances
	for _, bi := range buildInstances {
		if bi.Err != nil {
			return cue.Value{}, fmt.Errorf("failed to load CUE files: %w", bi.Err)
		}
	}

	// Build the first instance (assuming single package)
	schemaValue := l.ctx.BuildInstance(buildInstances[0])
	if err := schemaValue.Err(); err != nil {
		return cue.Value{}, fmt.Errorf("failed to build CUE schema: %w", err)
	}

	return schemaValue, nil
}

// GetDefaults extracts default values from the loaded schema.
// It returns a map containing all default values defined in the CUE schema.
// Returns an error if no schema is loaded or if defaults cannot be extracted.
func (l *cueSchemaLoader) GetDefaults() (map[string]any, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if !l.schemaValue.Exists() {
		return nil, errors.New("no schema loaded")
	}

	// Create an empty configuration and unify with schema to get defaults
	emptyConfig := l.ctx.Encode(map[string]any{})
	unified := l.schemaValue.Unify(emptyConfig)
	if err := unified.Err(); err != nil {
		return nil, fmt.Errorf("failed to unify schema with empty config: %w", err)
	}

	// Decode the unified value to extract defaults
	var defaults map[string]any
	if err := unified.Decode(&defaults); err != nil {
		return nil, fmt.Errorf("failed to decode defaults: %w", err)
	}

	return defaults, nil
}

// GetValidator returns a validator for the loaded schema.
// The validator can be used to validate configuration data against the schema.
// Returns an error if no schema is loaded.
func (l *cueSchemaLoader) GetValidator() (Validator, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if !l.schemaValue.Exists() {
		return nil, errors.New("no schema loaded")
	}

	return &cueValidator{
		ctx:         l.ctx,
		schemaValue: l.schemaValue,
	}, nil
}

// cueValidator implements the Validator interface using CUE.
// It provides validation of configuration data against CUE schemas.
type cueValidator struct {
	ctx         *cue.Context
	schemaValue cue.Value
}

// ValidateConfig validates a complete configuration against the schema.
func (v *cueValidator) ValidateConfig(config map[string]any) error {
	// Convert the config to a CUE value
	configValue := v.ctx.Encode(config)
	if err := configValue.Err(); err != nil {
		return fmt.Errorf("failed to encode configuration: %w", err)
	}

	// Unify with schema to validate
	unified := v.schemaValue.Unify(configValue)
	if err := unified.Err(); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	// Validate the unified result
	if err := unified.Validate(); err != nil {
		return v.formatValidationError(err)
	}

	return nil
}

// ValidateValue validates a specific value at a path against the schema.
func (v *cueValidator) ValidateValue(path string, value any) error {
	// Get the schema value at the specified path
	schemaAtPath, err := v.getValueAtPath(v.schemaValue, path)
	if err != nil {
		return fmt.Errorf("path %s not found in schema: %w", path, err)
	}

	// Convert the value to CUE
	valueAsCue := v.ctx.Encode(value)
	if err := valueAsCue.Err(); err != nil {
		return fmt.Errorf("failed to encode value: %w", err)
	}

	// Unify with schema at path
	unified := schemaAtPath.Unify(valueAsCue)
	if err := unified.Err(); err != nil {
		return fmt.Errorf("value validation failed at path %s: %w", path, err)
	}

	// Validate the unified result
	if err := unified.Validate(); err != nil {
		return v.formatValidationError(err)
	}

	return nil
}

// ValidateFile validates a configuration file without loading it.
func (v *cueValidator) ValidateFile(configPath string) error {
	// Securely read the configuration file with validation
	content, err := safeReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file %s: %w", configPath, err)
	}

	// Parse based on file extension
	var config map[string]any
	ext := strings.ToLower(filepath.Ext(configPath))

	switch ext {
	case ".yaml", ".yml":
		// Use CUE's YAML support via Extract and compile
		astFile, err := yaml.Extract(configPath, content)
		if err != nil {
			return fmt.Errorf("failed to extract YAML config: %w", err)
		}

		ctx := cuecontext.New()
		value := ctx.BuildFile(astFile)
		if value.Err() != nil {
			return fmt.Errorf("failed to build YAML config: %w", value.Err())
		}

		// Convert CUE value to Go map
		if err := value.Decode(&config); err != nil {
			return fmt.Errorf("failed to decode YAML config: %w", err)
		}
	case ".json":
		// Use CUE's JSON support
		ctx := cuecontext.New()
		value := ctx.CompileBytes(content, cue.Filename(configPath))
		if value.Err() != nil {
			return fmt.Errorf("failed to parse JSON config: %w", value.Err())
		}

		// Convert CUE value to Go map
		if err := value.Decode(&config); err != nil {
			return fmt.Errorf("failed to decode JSON config: %w", err)
		}
	default:
		return fmt.Errorf("unsupported config file format: %s", ext)
	}

	// Validate the parsed configuration
	return v.ValidateConfig(config)
}

// getValueAtPath navigates to a value at the specified dot-notation path.
func (v *cueValidator) getValueAtPath(value cue.Value, path string) (cue.Value, error) {
	if path == "" {
		return value, nil
	}

	parts := strings.Split(path, ".")
	current := value

	for _, part := range parts {
		current = current.LookupPath(cue.ParsePath(part))
		if err := current.Err(); err != nil {
			return cue.Value{}, fmt.Errorf("path component %s not found: %w", part, err)
		}
	}

	return current, nil
}

// formatValidationError formats CUE validation errors with helpful context.
func (v *cueValidator) formatValidationError(err error) error {
	errStr := err.Error()

	// Try to extract path information from the error
	if strings.Contains(errStr, ":") {
		parts := strings.SplitN(errStr, ":", 2)
		if len(parts) == 2 {
			path := strings.TrimSpace(parts[0])
			message := strings.TrimSpace(parts[1])
			return fmt.Errorf("validation error at '%s': %s", path, message)
		}
	}

	// Return formatted error
	return fmt.Errorf("configuration validation failed: %w", err)
}

// configLoader handles loading and parsing configuration files.
// It merges user configuration with schema defaults and provides
// thread-safe access to configuration values.
type configLoader struct {
	schemaLoader SchemaLoader
	validator    Validator
	configPath   string
	configData   map[string]any
	mergedData   map[string]any
	mu           sync.RWMutex
}

// newConfigLoader creates a new configuration loader.
func newConfigLoader(schemaLoader SchemaLoader) (*configLoader, error) {
	validator, err := schemaLoader.GetValidator()
	if err != nil {
		return nil, fmt.Errorf("failed to get validator: %w", err)
	}

	return &configLoader{
		schemaLoader: schemaLoader,
		validator:    validator,
	}, nil
}

// LoadConfig loads configuration from a file and merges it with schema defaults.
func (l *configLoader) LoadConfig(configPath string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.configPath = configPath

	// Get schema defaults
	defaults, err := l.schemaLoader.GetDefaults()
	if err != nil {
		return fmt.Errorf("failed to get schema defaults: %w", err)
	}

	var configData map[string]any

	if configPath != "" {
		// Load user configuration
		configData, err = l.loadConfigFile(configPath)
		if err != nil {
			return fmt.Errorf("failed to load config file: %w", err)
		}

		// Validate the configuration
		if err := l.validator.ValidateConfig(configData); err != nil {
			return fmt.Errorf("configuration validation failed: %w", err)
		}
	} else {
		// Use empty config if no file specified
		configData = make(map[string]any)
	}

	l.configData = configData

	// Merge defaults with user configuration
	l.mergedData = l.mergeConfigs(defaults, configData)

	return nil
}

// expandEnvironmentVariables performs environment variable substitution on config content.
// It replaces patterns like $ENV_VARIABLE, ${ENV_VARIABLE}, and ${ENV_VARIABLE:-default} with their values.
// If an environment variable is not set and no default is provided, it leaves the pattern unchanged.
func expandEnvironmentVariables(content []byte) []byte {
	contentStr := string(content)

	// First handle ${VAR:-default} patterns
	contentStr = expandWithDefaults(contentStr)

	// Then use os.ExpandEnv for standard $VAR and ${VAR} patterns
	expanded := os.ExpandEnv(contentStr)

	return []byte(expanded)
}

// expandWithDefaults handles ${VAR:-default} syntax for environment variables
func expandWithDefaults(content string) string {
	result := content

	for {
		start, end, varExpr := findNextVariableExpression(result)
		if start == -1 {
			break
		}

		// Process the variable expression
		if processed, shouldReplace := processVariableExpression(varExpr); shouldReplace {
			result = result[:start] + processed + result[end+1:]
		} else {
			// Skip this occurrence to avoid infinite loop
			if !skipToNextOccurrence(result, start) {
				break
			}
		}
	}

	return result
}

// findNextVariableExpression finds the next ${...} pattern in the string
func findNextVariableExpression(content string) (start, end int, varExpr string) {
	start = strings.Index(content, "${")
	if start == -1 {
		return -1, -1, ""
	}

	endOffset := strings.Index(content[start:], "}")
	if endOffset == -1 {
		return -1, -1, ""
	}
	end = start + endOffset

	varExpr = content[start+2 : end]
	return start, end, varExpr
}

// processVariableExpression processes a variable expression and returns the result and whether to replace
func processVariableExpression(varExpr string) (result string, shouldReplace bool) {
	colonDashIndex := strings.Index(varExpr, ":-")
	if colonDashIndex == -1 {
		return "", false // Let os.ExpandEnv handle it
	}

	varName := varExpr[:colonDashIndex]
	defaultValue := varExpr[colonDashIndex+2:]

	envValue := os.Getenv(varName)
	if envValue == "" {
		envValue = defaultValue
	}

	return envValue, true
}

// skipToNextOccurrence attempts to skip to the next occurrence to avoid infinite loops
func skipToNextOccurrence(content string, currentStart int) bool {
	if currentStart+3 >= len(content) {
		return false
	}

	nextResult := content[currentStart+3:]
	nextStart := strings.Index(nextResult, "${")
	return nextStart != -1
}

// loadConfigFile loads and parses a configuration file.
func (l *configLoader) loadConfigFile(configPath string) (map[string]any, error) {
	// Clean and validate the config path
	cleanPath := filepath.Clean(configPath)
	if !filepath.IsAbs(cleanPath) {
		absPath, err := filepath.Abs(cleanPath)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve config path %s: %w", configPath, err)
		}
		cleanPath = absPath
	}

	// Read the configuration file
	content, err := os.ReadFile(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", cleanPath, err)
	}

	// Perform environment variable substitution
	content = expandEnvironmentVariables(content)

	// Check for empty or comments-only files
	if err := l.validateFileContent(content, cleanPath); err != nil {
		return nil, err
	}

	// Parse based on file extension
	var config map[string]any
	ext := strings.ToLower(filepath.Ext(cleanPath))

	switch ext {
	case ".yaml", ".yml":
		// Use CUE's YAML support via Extract and compile
		astFile, err := yaml.Extract(configPath, content)
		if err != nil {
			return nil, fmt.Errorf("failed to extract YAML config: %w", err)
		}

		ctx := cuecontext.New()
		value := ctx.BuildFile(astFile)
		if value.Err() != nil {
			return nil, fmt.Errorf("failed to build YAML config: %w", value.Err())
		}

		// Convert CUE value to Go map
		if err := value.Decode(&config); err != nil {
			return nil, fmt.Errorf("failed to decode YAML config: %w", err)
		}
	case ".json":
		// Use CUE's JSON support (CUE can handle JSON natively)
		ctx := cuecontext.New()
		value := ctx.CompileBytes(content, cue.Filename(configPath))
		if value.Err() != nil {
			return nil, fmt.Errorf("failed to parse JSON config: %w", value.Err())
		}

		// Convert CUE value to Go map
		if err := value.Decode(&config); err != nil {
			return nil, fmt.Errorf("failed to decode JSON config: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported config file format: %s (supported: .yaml, .yml, .json)", ext)
	}

	return config, nil
}

// validateFileContent validates that the file contains meaningful content.
func (l *configLoader) validateFileContent(content []byte, filePath string) error {
	trimmed := strings.TrimSpace(string(content))
	if trimmed == "" {
		return fmt.Errorf("configuration file %s is empty", filePath)
	}

	// Check if file contains only comments
	meaningful := false
	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		meaningful = true
		break
	}

	if !meaningful {
		return fmt.Errorf("configuration file %s contains only comments", filePath)
	}

	return nil
}

// mergeConfigs merges default configuration with user configuration.
// User configuration takes precedence over defaults.
func (l *configLoader) mergeConfigs(defaults, userConfig map[string]any) map[string]any {
	result := make(map[string]any)

	// Start with defaults
	for key, value := range defaults {
		result[key] = value
	}

	// Override with user configuration
	for key, value := range userConfig {
		if existingValue, exists := result[key]; exists {
			// If both are maps, merge recursively
			if existingMap, ok := existingValue.(map[string]any); ok {
				if userMap, ok := value.(map[string]any); ok {
					result[key] = l.mergeConfigs(existingMap, userMap)
					continue
				}
			}
		}
		// Otherwise, user value takes precedence
		result[key] = value
	}

	return result
}

// GetMergedConfig returns the merged configuration data.
func (l *configLoader) GetMergedConfig() map[string]any {
	l.mu.RLock()
	defer l.mu.RUnlock()

	// Return a copy to prevent external modifications
	return l.copyConfig(l.mergedData)
}

// GetValue returns a configuration value at the specified path.
func (l *configLoader) GetValue(path string) (any, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.getValueAtPath(l.mergedData, path)
}

// getValueAtPath navigates to a value using dot notation.
func (l *configLoader) getValueAtPath(data map[string]any, path string) (any, error) {
	if path == "" {
		return data, nil
	}

	parts := strings.Split(path, ".")
	current := data

	for i, part := range parts {
		value, exists := current[part]
		if !exists {
			return nil, fmt.Errorf("path %s not found", path)
		}

		if i == len(parts)-1 {
			// Last part, return the value
			return value, nil
		}

		// Navigate deeper
		if nextMap, ok := value.(map[string]any); ok {
			current = nextMap
		} else {
			return nil, fmt.Errorf("path %s: cannot navigate through non-map value", path)
		}
	}

	return nil, fmt.Errorf("path %s not found", path)
}

// copyConfig creates a deep copy of a configuration map.
func (l *configLoader) copyConfig(config map[string]any) map[string]any {
	result := make(map[string]any)
	for key, value := range config {
		if mapValue, ok := value.(map[string]any); ok {
			result[key] = l.copyConfig(mapValue)
		} else {
			result[key] = value
		}
	}
	return result
}

// changeNotifier implements the ChangeNotifier interface.
// It manages a list of callbacks that are invoked when configuration changes occur.
type changeNotifier struct {
	callbacks []func(error)
	mu        sync.RWMutex
}

// newChangeNotifier creates a new change notifier.
func newChangeNotifier() *changeNotifier {
	return &changeNotifier{
		callbacks: make([]func(error), 0),
	}
}

// OnChange registers a callback for configuration changes.
func (n *changeNotifier) OnChange(callback func(error)) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.callbacks = append(n.callbacks, callback)
}

// NotifyChange triggers all registered change callbacks.
func (n *changeNotifier) NotifyChange(err error) {
	n.mu.RLock()
	callbacks := make([]func(error), len(n.callbacks))
	copy(callbacks, n.callbacks)
	n.mu.RUnlock()

	// Call all callbacks
	for _, callback := range callbacks {
		if callback != nil {
			callback(err)
		}
	}
}

// hotReloader handles file watching and hot reload functionality.
// It monitors configuration and schema files for changes and triggers
// reloads when modifications are detected.
type hotReloader struct {
	watcher        *fsnotify.Watcher
	ctx            context.Context
	cancel         context.CancelFunc
	configPath     string
	schemaPath     string
	reloadCallback func() error
	notifier       *changeNotifier
	mu             sync.RWMutex
}

// newHotReloader creates a new hot reloader.
func newHotReloader(configPath, schemaPath string, reloadCallback func() error) (*hotReloader, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	return &hotReloader{
		watcher:        watcher,
		configPath:     configPath,
		schemaPath:     schemaPath,
		reloadCallback: reloadCallback,
		notifier:       newChangeNotifier(),
	}, nil
}

// Start starts the hot reload functionality.
func (h *hotReloader) Start(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.ctx != nil {
		return errors.New("hot reload already started")
	}

	h.ctx, h.cancel = context.WithCancel(ctx)

	// Add files to watch
	if h.configPath != "" {
		if err := h.watcher.Add(h.configPath); err != nil {
			h.cancel()
			return fmt.Errorf("failed to watch config file %s: %w", h.configPath, err)
		}
	}

	if h.schemaPath != "" {
		if err := h.addSchemaWatch(); err != nil {
			h.cancel()
			return err
		}
	}

	// Start watching in a goroutine
	go h.watchFiles()

	return nil
}

// addSchemaWatch adds the schema path to the file watcher.
// This method reduces complexity in the Start method.
func (h *hotReloader) addSchemaWatch() error {
	// For schema paths, we might need to watch a directory
	info, err := os.Stat(h.schemaPath)
	if err != nil {
		// If we can't stat the schema path, we'll skip watching it
		return nil
	}

	if info.IsDir() {
		if err := h.watcher.Add(h.schemaPath); err != nil {
			return fmt.Errorf("failed to watch schema directory %s: %w", h.schemaPath, err)
		}
	} else {
		if err := h.watcher.Add(h.schemaPath); err != nil {
			return fmt.Errorf("failed to watch schema file %s: %w", h.schemaPath, err)
		}
	}

	return nil
}

// Stop stops the hot reload functionality.
func (h *hotReloader) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.cancel != nil {
		h.cancel()
		h.cancel = nil
		h.ctx = nil
	}

	if h.watcher != nil {
		h.closeWatcher()
	}
}

// closeWatcher safely closes the file watcher with proper error handling.
func (h *hotReloader) closeWatcher() {
	if err := h.watcher.Close(); err != nil {
		// Log the error but don't fail the cleanup process
		// In a real application, you might want to use a proper logger here
		// For now, we'll just continue with cleanup
		return
	}
}

// OnChange registers a callback for configuration changes.
func (h *hotReloader) OnChange(callback func(error)) {
	h.notifier.OnChange(callback)
}

// watchFiles watches for file system events and triggers reloads.
func (h *hotReloader) watchFiles() {
	// Get the context safely
	h.mu.RLock()
	ctx := h.ctx
	h.mu.RUnlock()

	if ctx == nil {
		return // Context not set, exit early
	}

	for {
		select {
		case event, ok := <-h.watcher.Events:
			if !ok {
				return
			}

			// Only react to write events
			if event.Op&fsnotify.Write == fsnotify.Write {
				h.handleFileChange(event.Name)
			}

		case err, ok := <-h.watcher.Errors:
			if !ok {
				return
			}
			h.notifier.NotifyChange(fmt.Errorf("file watcher error: %w", err))

		case <-ctx.Done():
			return
		}
	}
}

// handleFileChange handles a file change event.
func (h *hotReloader) handleFileChange(_ string) {
	// Small delay to ensure file write is complete
	time.Sleep(100 * time.Millisecond)

	// Trigger reload
	if h.reloadCallback != nil {
		if err := h.reloadCallback(); err != nil {
			h.notifier.NotifyChange(fmt.Errorf("configuration reload failed: %w", err))
		} else {
			h.notifier.NotifyChange(nil) // Success
		}
	}
}

// configManager implements the Manager interface.
// It coordinates all aspects of configuration management including
// loading, validation, hot reload, and change notifications.
type configManager struct {
	schemaLoader SchemaLoader
	configLoader *configLoader
	hotReloader  *hotReloader
	options      Options
	mu           sync.RWMutex
}

// validateOptions validates the provided options for creating a Manager.
func validateOptions(options Options) error {
	// Validate required options - either SchemaPath or SchemaContent must be provided
	if options.SchemaPath == "" && options.SchemaContent == "" {
		return errors.New("either schema path or schema content is required")
	}
	if options.SchemaPath != "" && options.SchemaContent != "" {
		return errors.New("cannot specify both schema path and schema content")
	}

	// Validate hot reload options
	if options.EnableSchemaHotReload && options.SchemaContent != "" {
		return errors.New("schema hot reload is not supported when using schema content (use EnableConfigHotReload for config file watching)")
	}

	return nil
}

// createAndLoadSchema creates a schema loader and loads the schema from options.
func createAndLoadSchema(options Options) (SchemaLoader, error) {
	schemaLoader := NewSchemaLoader()

	if options.SchemaContent != "" {
		if err := schemaLoader.LoadSchemaContent(options.SchemaContent); err != nil {
			return nil, fmt.Errorf("failed to load schema content: %w", err)
		}
	} else {
		if err := schemaLoader.LoadSchema(options.SchemaPath); err != nil {
			return nil, fmt.Errorf("failed to load schema: %w", err)
		}
	}

	return schemaLoader, nil
}

// createAndLoadConfig creates a config loader and loads the configuration.
func createAndLoadConfig(schemaLoader SchemaLoader, configPath string) (*configLoader, error) {
	configLoader, err := newConfigLoader(schemaLoader)
	if err != nil {
		return nil, fmt.Errorf("failed to create config loader: %w", err)
	}

	if err := configLoader.LoadConfig(configPath); err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	return configLoader, nil
}

// setupHotReload sets up hot reload functionality if enabled.
func setupHotReload(manager *configManager, options Options) error {
	if !options.EnableSchemaHotReload && !options.EnableConfigHotReload {
		return nil
	}

	ctx := options.HotReloadContext
	if ctx == nil {
		ctx = context.Background()
	}

	if err := manager.StartHotReload(ctx); err != nil {
		return fmt.Errorf("failed to start hot reload: %w", err)
	}

	return nil
}

// NewManager creates a new configuration manager with the specified options.
//
// NewManager is the main entry point for creating a configuration manager.
// It loads the CUE schema, parses the configuration file, performs validation,
// and optionally enables hot reload functionality.
//
// The manager must be closed when no longer needed to release resources:
//
//	manager, err := config.NewManager(config.Options{
//		SchemaPath: "./schema/config.cue",
//		ConfigPath: "./config.yaml",
//	})
//	if err != nil {
//		return err
//	}
//	defer manager.Close()
//
//	host, _ := manager.GetString("server.host")
//	port, _ := manager.GetInt("server.port")
//
// Returns an error if the schema cannot be loaded, configuration cannot be parsed,
// or hot reload cannot be initialized.
func NewManager(options Options) (Manager, error) {
	// Validate options
	if err := validateOptions(options); err != nil {
		return nil, err
	}

	// Create and load schema
	schemaLoader, err := createAndLoadSchema(options)
	if err != nil {
		return nil, err
	}

	// Create and load config
	configLoader, err := createAndLoadConfig(schemaLoader, options.ConfigPath)
	if err != nil {
		return nil, err
	}

	// Create manager
	manager := &configManager{
		schemaLoader: schemaLoader,
		configLoader: configLoader,
		options:      options,
	}

	// Set up hot reload if needed
	if err := setupHotReload(manager, options); err != nil {
		return nil, err
	}

	return manager, nil
}

// GetString returns the string value at the specified path.
// It uses dot notation for nested access (e.g., "server.host").
// If the path doesn't exist, it returns the first defaultValue if provided,
// otherwise it returns an error.
func (m *configManager) GetString(path string, defaultValue ...string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	value, err := m.configLoader.GetValue(path)
	if err != nil {
		if len(defaultValue) > 0 {
			return defaultValue[0], nil
		}
		return "", err
	}

	if strValue, ok := value.(string); ok {
		return strValue, nil
	}

	return "", fmt.Errorf("value at path %s is not a string: %T", path, value)
}

// GetInt returns an integer configuration value at the given path.
func (m *configManager) GetInt(path string, defaultValue ...int) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	value, err := m.configLoader.GetValue(path)
	if err != nil {
		if len(defaultValue) > 0 {
			return defaultValue[0], nil
		}
		return 0, err
	}

	switch v := value.(type) {
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case float64:
		return int(v), nil
	default:
		return 0, fmt.Errorf("value at path %s is not an integer: %T", path, value)
	}
}

// GetFloat returns a float64 configuration value at the given path.
func (m *configManager) GetFloat(path string, defaultValue ...float64) (float64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	value, err := m.configLoader.GetValue(path)
	if err != nil {
		if len(defaultValue) > 0 {
			return defaultValue[0], nil
		}
		return 0, err
	}

	switch v := value.(type) {
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	default:
		return 0, fmt.Errorf("value at path %s is not a float: %T", path, value)
	}
}

// GetBool returns a boolean configuration value at the given path.
func (m *configManager) GetBool(path string, defaultValue ...bool) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	value, err := m.configLoader.GetValue(path)
	if err != nil {
		if len(defaultValue) > 0 {
			return defaultValue[0], nil
		}
		return false, err
	}

	if boolValue, ok := value.(bool); ok {
		return boolValue, nil
	}

	return false, fmt.Errorf("value at path %s is not a boolean: %T", path, value)
}

// GetDuration returns a time.Duration configuration value at the given path.
func (m *configManager) GetDuration(path string, defaultValue ...time.Duration) (time.Duration, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	value, err := m.configLoader.GetValue(path)
	if err != nil {
		if len(defaultValue) > 0 {
			return defaultValue[0], nil
		}
		return 0, err
	}

	if strValue, ok := value.(string); ok {
		duration, err := time.ParseDuration(strValue)
		if err != nil {
			return 0, fmt.Errorf("failed to parse duration at path %s: %w", path, err)
		}
		return duration, nil
	}

	return 0, fmt.Errorf("value at path %s is not a duration string: %T", path, value)
}

// GetStringSlice returns a string slice configuration value at the given path.
func (m *configManager) GetStringSlice(path string, defaultValue ...[]string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	value, err := m.configLoader.GetValue(path)
	if err != nil {
		if len(defaultValue) > 0 {
			return defaultValue[0], nil
		}
		return nil, err
	}

	switch v := value.(type) {
	case []string:
		return v, nil
	case []any:
		// Convert []any to []string
		result := make([]string, len(v))
		for i, item := range v {
			if strItem, ok := item.(string); ok {
				result[i] = strItem
			} else {
				return nil, fmt.Errorf("item at index %d in path %s is not a string: %T", i, path, item)
			}
		}
		return result, nil
	default:
		return nil, fmt.Errorf("value at path %s is not a string slice: %T", path, value)
	}
}

// GetMap returns a map configuration value at the given path.
func (m *configManager) GetMap(path string) (map[string]any, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	value, err := m.configLoader.GetValue(path)
	if err != nil {
		return nil, err
	}

	if mapValue, ok := value.(map[string]any); ok {
		// Return a copy to prevent external modifications
		return m.copyMap(mapValue), nil
	}

	return nil, fmt.Errorf("value at path %s is not a map: %T", path, value)
}

// Exists checks if a configuration path exists.
func (m *configManager) Exists(path string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, err := m.configLoader.GetValue(path)
	return err == nil
}

// Validate validates the current configuration against the CUE schema.
// It checks that all required fields are present and that values conform
// to the schema constraints. Returns an error if validation fails.
func (m *configManager) Validate() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	validator, err := m.schemaLoader.GetValidator()
	if err != nil {
		return fmt.Errorf("failed to get validator: %w", err)
	}

	config := m.configLoader.GetMergedConfig()
	return validator.ValidateConfig(config)
}

// StartHotReload enables automatic configuration reloading when files change.
// It starts watching the configuration and schema files for modifications.
// The provided context controls the hot reload lifecycle.
// Returns an error if the file watcher cannot be started.
func (m *configManager) StartHotReload(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.hotReloader != nil {
		return errors.New("hot reload already started")
	}

	// Create hot reloader with reload callback
	reloadCallback := func() error {
		return m.reloadInternal()
	}

	// Determine which files to watch based on options
	var configPath, schemaPath string
	if m.options.EnableConfigHotReload {
		configPath = m.options.ConfigPath
	}
	if m.options.EnableSchemaHotReload {
		schemaPath = m.options.SchemaPath
	}

	// Don't create hot reloader if there are no files to watch
	if configPath == "" && schemaPath == "" {
		return errors.New("no files to watch: at least one of EnableConfigHotReload or EnableSchemaHotReload must be true with valid file paths")
	}

	hotReloader, err := newHotReloader(configPath, schemaPath, reloadCallback)
	if err != nil {
		return fmt.Errorf("failed to create hot reloader: %w", err)
	}

	if err := hotReloader.Start(ctx); err != nil {
		return fmt.Errorf("failed to start hot reloader: %w", err)
	}

	m.hotReloader = hotReloader
	return nil
}

// StopHotReload disables automatic configuration reloading.
func (m *configManager) StopHotReload() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.hotReloader != nil {
		m.hotReloader.Stop()
		m.hotReloader = nil
	}
}

// OnConfigChange registers a callback that is invoked when configuration changes.
// The callback receives any error that occurred during reload.
// If err is nil, the reload was successful.
func (m *configManager) OnConfigChange(callback func(error)) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.hotReloader != nil {
		m.hotReloader.OnChange(callback)
	}
}

// Reload manually reloads the configuration from files.
// It re-reads the schema and configuration files, validates the new configuration,
// and applies the changes if validation succeeds.
// Returns an error if reloading or validation fails.
func (m *configManager) Reload() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.reloadInternal()
}

// reloadInternal performs the actual reload operation (internal, assumes lock is held).
func (m *configManager) reloadInternal() error {
	// Reload schema - use SchemaContent if available, otherwise SchemaPath
	if m.options.SchemaContent != "" {
		if err := m.schemaLoader.LoadSchemaContent(m.options.SchemaContent); err != nil {
			return fmt.Errorf("failed to reload schema content: %w", err)
		}
	} else {
		if err := m.schemaLoader.LoadSchema(m.options.SchemaPath); err != nil {
			return fmt.Errorf("failed to reload schema: %w", err)
		}
	}

	// Create new config loader with updated schema
	newConfigLoader, err := newConfigLoader(m.schemaLoader)
	if err != nil {
		return fmt.Errorf("failed to create new config loader: %w", err)
	}

	// Load configuration
	if err := newConfigLoader.LoadConfig(m.options.ConfigPath); err != nil {
		return fmt.Errorf("failed to reload configuration: %w", err)
	}

	// Replace the config loader
	m.configLoader = newConfigLoader

	return nil
}

// Close releases all resources and stops hot reload if active.
// It should be called when the manager is no longer needed to prevent
// resource leaks. After calling Close, the manager should not be used.
func (m *configManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.hotReloader != nil {
		m.hotReloader.Stop()
		m.hotReloader = nil
	}

	return nil
}

// copyMap creates a deep copy of a map.
func (m *configManager) copyMap(original map[string]any) map[string]any {
	result := make(map[string]any)
	for key, value := range original {
		if mapValue, ok := value.(map[string]any); ok {
			result[key] = m.copyMap(mapValue)
		} else {
			result[key] = value
		}
	}
	return result
}

// safeReadFile securely reads a file after comprehensive validation.
// It performs security checks to prevent directory traversal attacks
// and other file system vulnerabilities. This function addresses
// gosec G304 by implementing proper path validation and access controls.
// Returns an error if the file path is invalid or the file cannot be read safely.
func safeReadFile(filePath string) ([]byte, error) {
	if filePath == "" {
		return nil, errors.New("file path cannot be empty")
	}

	// Clean the path to resolve any .. or . elements
	cleanPath := filepath.Clean(filePath)

	// Check for directory traversal patterns
	if strings.Contains(cleanPath, "..") {
		return nil, errors.New("invalid file path: contains directory traversal")
	}

	// Check for absolute paths that might be suspicious
	if filepath.IsAbs(cleanPath) {
		// Allow absolute paths but validate they don't access system directories
		systemDirs := []string{"/etc/passwd", "/etc/shadow", "/proc/", "/sys/"}
		for _, sysDir := range systemDirs {
			if strings.HasPrefix(cleanPath, sysDir) {
				return nil, fmt.Errorf("access to system directory not allowed: %s", sysDir)
			}
		}
	}

	// Ensure the file exists and is a regular file
	info, err := os.Stat(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("file validation failed: %w", err)
	}

	if !info.Mode().IsRegular() {
		return nil, errors.New("path must be a regular file")
	}

	// Check file size to prevent reading extremely large files
	const maxFileSize = 10 * 1024 * 1024 // 10MB limit
	if info.Size() > maxFileSize {
		return nil, fmt.Errorf("file too large: %d bytes (max %d)", info.Size(), maxFileSize)
	}

	// Now safely read the file using the cleaned path
	content, err := os.ReadFile(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return content, nil
}
