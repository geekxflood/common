// Package config provides comprehensive configuration management for Go applications
package config

import (
	"os"
	"testing"
	"time"
)

// Test data and helper functions

// createTempFile creates a temporary file with the given content and returns its path.
func createTempFile(t *testing.T, content, suffix string) string {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "config_test_*"+suffix)
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}

	if err := tmpFile.Close(); err != nil {
		t.Fatalf("Failed to close temp file: %v", err)
	}

	// Clean up the file when the test completes
	t.Cleanup(func() {
		os.Remove(tmpFile.Name())
	})

	return tmpFile.Name()
}

// Test CUE schema embedded directly in the test
const testSchema = `
package config

// Application configuration
app: {
	name: string | *"test-app"
	version: string | *"1.0.0"
	environment: "development" | "staging" | "production" | *"development"
	debug: bool | *false
}

// Server configuration
server: {
	host: string | *"localhost"
	port: int & >=1024 & <=65535 | *8080
	timeout: string | *"30s"
	readTimeout: string | *"30s"
	writeTimeout: string | *"30s"
	maxConnections: int & >0 | *1000
}

// Database configuration
database: {
	type: "postgres" | "mysql" | "sqlite" | *"postgres"
	host: string | *"localhost"
	port: int & >0 | *5432
	name: string | *"testdb"
	username?: string
	password?: string
	pool: {
		maxOpen: int & >=0 | *25
		maxIdle: int & >=0 | *5
		maxLifetime: string | *"5m"
	}
	ssl: {
		mode: "disable" | "require" | "verify-ca" | "verify-full" | *"disable"
	}
}

// Logging configuration
logging: {
	level: "debug" | "info" | "warn" | "error" | *"info"
	format: "json" | "logfmt" | "text" | *"logfmt"
	output: string | *"stdout"
	addSource: bool | *false
}

// Metrics configuration
metrics: {
	enabled: bool | *true
	path: string | *"/metrics"
	port: int & >=1024 & <=65535 | *9090
	namespace: string | *"test"
}

// Feature flags
features: {
	auth: bool | *false
	metrics: bool | *true
	debug: bool | *false
	experimental: bool | *false
}
`

// Test configuration content embedded directly in the test
const testConfig = `
app:
  name: "test-application"
  version: "2.1.0"
  environment: "development"
  debug: true

server:
  host: "0.0.0.0"
  port: 9090
  timeout: "45s"
  readTimeout: "60s"
  writeTimeout: "60s"
  maxConnections: 500

database:
  type: "postgres"
  host: "db.example.com"
  port: 5432
  name: "testdb"
  username: "testuser"
  password: "testpass"
  pool:
    maxOpen: 50
    maxIdle: 10
    maxLifetime: "10m"
  ssl:
    mode: "require"

logging:
  level: "debug"
  format: "json"
  output: "stdout"
  addSource: true

metrics:
  enabled: true
  path: "/metrics"
  port: 9090
  namespace: "test_app"

features:
  auth: true
  metrics: true
  debug: true
  experimental: false
`

// Invalid configuration for validation testing
const invalidConfig = `
app:
  name: "test-app"
  version: "1.0.0"
  environment: "invalid_env"  # Invalid environment value
  debug: true

server:
  host: "0.0.0.0"
  port: 99999  # Invalid port (too high)
  timeout: "45s"

database:
  type: "invalid_db_type"  # Invalid database type
  host: "db.example.com"
  port: -1  # Invalid port (negative)
  name: "testdb"

logging:
  level: "invalid_level"  # Invalid log level
  format: "logfmt"
  output: "stdout"

metrics:
  enabled: true
  path: "/metrics"
  port: 70000  # Invalid port (too high)
  namespace: "test"
`

// TestNewManager tests the creation of a new configuration manager.
func TestNewManager(t *testing.T) {
	tests := []struct {
		name        string
		schema      string
		config      string
		expectError bool
	}{
		{
			name:        "valid schema and config",
			schema:      testSchema,
			config:      testConfig,
			expectError: false,
		},
		{
			name:        "valid schema only (no config file)",
			schema:      testSchema,
			config:      "",
			expectError: false,
		},
		{
			name:        "invalid schema",
			schema:      "invalid cue syntax {{{",
			config:      testConfig,
			expectError: true,
		},
		{
			name:        "invalid config",
			schema:      testSchema,
			config:      invalidConfig,
			expectError: true,
		},
		{
			name:        "schema with no package declaration",
			schema:      "server: { host: string }", // Missing package declaration
			config:      testConfig,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary files for this test
			schemaPath := createTempFile(t, tt.schema, ".cue")

			var configPath string
			if tt.config != "" {
				configPath = createTempFile(t, tt.config, ".yaml")
			}

			options := Options{
				SchemaPath: schemaPath,
				ConfigPath: configPath,
			}

			manager, err := NewManager(options)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if manager == nil {
				t.Error("Expected manager but got nil")
				return
			}

			// Clean up
			if err := manager.Close(); err != nil {
				t.Errorf("Failed to close manager: %v", err)
			}
		})
	}
}

// TestManagerGetString tests string value retrieval.
func TestManagerGetString(t *testing.T) {
	// Create manager with embedded test content
	schemaPath := createTempFile(t, testSchema, ".cue")
	configPath := createTempFile(t, testConfig, ".yaml")

	manager, err := NewManager(Options{
		SchemaPath: schemaPath,
		ConfigPath: configPath,
	})
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Close()

	tests := []struct {
		name         string
		path         string
		defaultValue []string
		expected     string
		expectError  bool
	}{
		{
			name:     "app name",
			path:     "app.name",
			expected: "test-application",
		},
		{
			name:     "server host",
			path:     "server.host",
			expected: "0.0.0.0",
		},
		{
			name:     "server timeout",
			path:     "server.timeout",
			expected: "45s",
		},
		{
			name:     "database type",
			path:     "database.type",
			expected: "postgres",
		},
		{
			name:     "logging format",
			path:     "logging.format",
			expected: "json",
		},
		{
			name:         "non-existent path with default",
			path:         "nonexistent.path",
			defaultValue: []string{"default_value"},
			expected:     "default_value",
		},
		{
			name:        "non-existent path without default",
			path:        "nonexistent.path",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := manager.GetString(tt.path, tt.defaultValue...)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("Expected %q but got %q", tt.expected, result)
			}
		})
	}
}

// TestManagerGetInt tests integer value retrieval.
func TestManagerGetInt(t *testing.T) {
	// Create manager with embedded test content
	schemaPath := createTempFile(t, testSchema, ".cue")
	configPath := createTempFile(t, testConfig, ".yaml")

	manager, err := NewManager(Options{
		SchemaPath: schemaPath,
		ConfigPath: configPath,
	})
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Close()

	tests := []struct {
		name         string
		path         string
		defaultValue []int
		expected     int
		expectError  bool
	}{
		{
			name:     "server port",
			path:     "server.port",
			expected: 9090,
		},
		{
			name:     "database port",
			path:     "database.port",
			expected: 5432,
		},
		{
			name:     "server max connections",
			path:     "server.maxConnections",
			expected: 500,
		},
		{
			name:     "database pool max open",
			path:     "database.pool.maxOpen",
			expected: 50,
		},
		{
			name:     "metrics port",
			path:     "metrics.port",
			expected: 9090,
		},
		{
			name:         "non-existent path with default",
			path:         "nonexistent.path",
			defaultValue: []int{42},
			expected:     42,
		},
		{
			name:        "non-existent path without default",
			path:        "nonexistent.path",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := manager.GetInt(tt.path, tt.defaultValue...)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("Expected %d but got %d", tt.expected, result)
			}
		})
	}
}

// TestManagerGetBool tests boolean value retrieval.
func TestManagerGetBool(t *testing.T) {
	// Create manager with embedded test content
	schemaPath := createTempFile(t, testSchema, ".cue")
	configPath := createTempFile(t, testConfig, ".yaml")

	manager, err := NewManager(Options{
		SchemaPath: schemaPath,
		ConfigPath: configPath,
	})
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Close()

	tests := []struct {
		name         string
		path         string
		defaultValue []bool
		expected     bool
		expectError  bool
	}{
		{
			name:     "app debug enabled",
			path:     "app.debug",
			expected: true,
		},
		{
			name:     "features auth enabled",
			path:     "features.auth",
			expected: true,
		},
		{
			name:     "features metrics enabled",
			path:     "features.metrics",
			expected: true,
		},
		{
			name:     "features debug enabled",
			path:     "features.debug",
			expected: true,
		},
		{
			name:     "features experimental disabled",
			path:     "features.experimental",
			expected: false,
		},
		{
			name:     "logging add source enabled",
			path:     "logging.addSource",
			expected: true,
		},
		{
			name:     "metrics enabled",
			path:     "metrics.enabled",
			expected: true,
		},
		{
			name:         "non-existent path with default",
			path:         "nonexistent.path",
			defaultValue: []bool{true},
			expected:     true,
		},
		{
			name:        "non-existent path without default",
			path:        "nonexistent.path",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := manager.GetBool(tt.path, tt.defaultValue...)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("Expected %t but got %t", tt.expected, result)
			}
		})
	}
}

// TestManagerGetDuration tests duration value retrieval.
func TestManagerGetDuration(t *testing.T) {
	// Create manager with embedded test content
	schemaPath := createTempFile(t, testSchema, ".cue")
	configPath := createTempFile(t, testConfig, ".yaml")

	manager, err := NewManager(Options{
		SchemaPath: schemaPath,
		ConfigPath: configPath,
	})
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Close()

	tests := []struct {
		name         string
		path         string
		defaultValue []time.Duration
		expected     time.Duration
		expectError  bool
	}{
		{
			name:     "server timeout",
			path:     "server.timeout",
			expected: 45 * time.Second,
		},
		{
			name:     "server read timeout",
			path:     "server.readTimeout",
			expected: 60 * time.Second,
		},
		{
			name:     "server write timeout",
			path:     "server.writeTimeout",
			expected: 60 * time.Second,
		},
		{
			name:     "database pool max lifetime",
			path:     "database.pool.maxLifetime",
			expected: 10 * time.Minute,
		},
		{
			name:         "non-existent path with default",
			path:         "nonexistent.path",
			defaultValue: []time.Duration{30 * time.Second},
			expected:     30 * time.Second,
		},
		{
			name:        "non-existent path without default",
			path:        "nonexistent.path",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := manager.GetDuration(tt.path, tt.defaultValue...)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("Expected %v but got %v", tt.expected, result)
			}
		})
	}
}

// TestManagerValidation tests configuration validation.
func TestManagerValidation(t *testing.T) {
	tests := []struct {
		name        string
		schema      string
		config      string
		expectError bool
	}{
		{
			name:        "valid configuration",
			schema:      testSchema,
			config:      testConfig,
			expectError: false,
		},
		{
			name:        "invalid configuration - bad environment",
			schema:      testSchema,
			config:      invalidConfig,
			expectError: true,
		},
		{
			name:   "invalid configuration - port out of range",
			schema: testSchema,
			config: `
app:
  name: "test-app"
  environment: "development"
server:
  port: 99999  # Invalid port
database:
  type: "postgres"
logging:
  level: "info"
  format: "logfmt"
metrics:
  enabled: true
features:
  auth: false
`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schemaPath := createTempFile(t, tt.schema, ".cue")
			configPath := createTempFile(t, tt.config, ".yaml")

			manager, err := NewManager(Options{
				SchemaPath: schemaPath,
				ConfigPath: configPath,
			})

			if tt.expectError {
				if err == nil {
					t.Error("Expected error during manager creation but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error during manager creation: %v", err)
				return
			}

			defer manager.Close()

			// Test explicit validation
			if err := manager.Validate(); err != nil {
				t.Errorf("Unexpected validation error: %v", err)
			}
		})
	}
}

// TestManagerExists tests path existence checking.
func TestManagerExists(t *testing.T) {
	// Create manager with embedded test content
	schemaPath := createTempFile(t, testSchema, ".cue")
	configPath := createTempFile(t, testConfig, ".yaml")

	manager, err := NewManager(Options{
		SchemaPath: schemaPath,
		ConfigPath: configPath,
	})
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Close()

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "app name exists",
			path:     "app.name",
			expected: true,
		},
		{
			name:     "server host exists",
			path:     "server.host",
			expected: true,
		},
		{
			name:     "database pool settings exist",
			path:     "database.pool.maxOpen",
			expected: true,
		},
		{
			name:     "logging format exists",
			path:     "logging.format",
			expected: true,
		},
		{
			name:     "metrics namespace exists",
			path:     "metrics.namespace",
			expected: true,
		},
		{
			name:     "features auth exists",
			path:     "features.auth",
			expected: true,
		},
		{
			name:     "non-existent top level path",
			path:     "nonexistent",
			expected: false,
		},
		{
			name:     "non-existent nested path",
			path:     "server.nonexistent",
			expected: false,
		},
		{
			name:     "non-existent deep path",
			path:     "database.pool.nonexistent",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.Exists(tt.path)

			if result != tt.expected {
				t.Errorf("Expected %t but got %t for path %s", tt.expected, result, tt.path)
			}
		})
	}
}

// TestManagerReload tests manual configuration reload functionality.
func TestManagerReload(t *testing.T) {
	// Create manager with embedded test content
	schemaPath := createTempFile(t, testSchema, ".cue")
	configPath := createTempFile(t, testConfig, ".yaml")

	manager, err := NewManager(Options{
		SchemaPath: schemaPath,
		ConfigPath: configPath,
	})
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Close()

	// Test initial value
	initialHost, err := manager.GetString("server.host")
	if err != nil {
		t.Fatalf("Failed to get initial host: %v", err)
	}
	if initialHost != "0.0.0.0" {
		t.Errorf("Expected initial host to be '0.0.0.0', got %q", initialHost)
	}

	// Update the configuration file with new content
	updatedConfig := `
app:
  name: "updated-test-app"
  version: "2.1.0"
  environment: "development"
  debug: true

server:
  host: "127.0.0.1"  # Changed from 0.0.0.0
  port: 8080         # Changed from 9090
  timeout: "30s"     # Changed from 45s
  readTimeout: "45s"
  writeTimeout: "45s"
  maxConnections: 200

database:
  type: "postgres"
  host: "localhost"  # Changed from db.example.com
  port: 5432
  name: "testdb"
  username: "testuser"
  password: "testpass"
  pool:
    maxOpen: 25      # Changed from 50
    maxIdle: 5       # Changed from 10
    maxLifetime: "5m" # Changed from 10m
  ssl:
    mode: "disable"  # Changed from require

logging:
  level: "info"      # Changed from debug
  format: "logfmt"   # Changed from json
  output: "stdout"
  addSource: false   # Changed from true

metrics:
  enabled: false     # Changed from true
  path: "/metrics"
  port: 9090
  namespace: "test_app"

features:
  auth: false        # Changed from true
  metrics: false     # Changed from true
  debug: false       # Changed from true
  experimental: true # Changed from false
`

	// Write the updated configuration
	if err := os.WriteFile(configPath, []byte(updatedConfig), 0644); err != nil {
		t.Fatalf("Failed to update config file: %v", err)
	}

	// Manually reload the configuration
	if err := manager.Reload(); err != nil {
		t.Fatalf("Failed to reload configuration: %v", err)
	}

	// Verify the configuration was updated
	updatedHost, err := manager.GetString("server.host")
	if err != nil {
		t.Errorf("Failed to get updated host: %v", err)
		return
	}
	if updatedHost != "127.0.0.1" {
		t.Errorf("Expected updated host to be '127.0.0.1', got %q", updatedHost)
	}

	// Verify other updated values
	updatedPort, err := manager.GetInt("server.port")
	if err != nil {
		t.Errorf("Failed to get updated port: %v", err)
	} else if updatedPort != 8080 {
		t.Errorf("Expected updated port to be 8080, got %d", updatedPort)
	}

	updatedAuth, err := manager.GetBool("features.auth")
	if err != nil {
		t.Errorf("Failed to get updated auth feature: %v", err)
	} else if updatedAuth != false {
		t.Errorf("Expected updated auth feature to be false, got %t", updatedAuth)
	}

	updatedLevel, err := manager.GetString("logging.level")
	if err != nil {
		t.Errorf("Failed to get updated logging level: %v", err)
	} else if updatedLevel != "info" {
		t.Errorf("Expected updated logging level to be 'info', got %q", updatedLevel)
	}
}

// TestSchemaContent tests the SchemaContent functionality
func TestSchemaContent(t *testing.T) {
	// Test with inline schema content
	schemaContent := `
package config

server: {
	host: string | *"localhost"
	port: int & >=1024 & <=65535 | *8080
}

app: {
	name: string | *"test-app"
	debug: bool | *false
}
`

	configContent := `
server:
  host: "example.com"
  port: 9000
app:
  name: "my-app"
  debug: true
`

	configPath := createTempFile(t, configContent, ".yaml")

	// Create manager with schema content
	manager, err := NewManager(Options{
		SchemaContent: schemaContent,
		ConfigPath:    configPath,
	})
	if err != nil {
		t.Fatalf("Failed to create manager with schema content: %v", err)
	}
	defer manager.Close()

	// Test configuration values
	host, err := manager.GetString("server.host")
	if err != nil {
		t.Errorf("Failed to get server host: %v", err)
	} else if host != "example.com" {
		t.Errorf("Expected host to be 'example.com', got %q", host)
	}

	port, err := manager.GetInt("server.port")
	if err != nil {
		t.Errorf("Failed to get server port: %v", err)
	} else if port != 9000 {
		t.Errorf("Expected port to be 9000, got %d", port)
	}

	appName, err := manager.GetString("app.name")
	if err != nil {
		t.Errorf("Failed to get app name: %v", err)
	} else if appName != "my-app" {
		t.Errorf("Expected app name to be 'my-app', got %q", appName)
	}

	debug, err := manager.GetBool("app.debug")
	if err != nil {
		t.Errorf("Failed to get debug flag: %v", err)
	} else if !debug {
		t.Errorf("Expected debug to be true, got %v", debug)
	}
}

// TestSchemaContentValidation tests validation of SchemaContent options
func TestSchemaContentValidation(t *testing.T) {
	tests := []struct {
		name          string
		setupOptions  func(t *testing.T) Options
		expectError   bool
		errorContains string
	}{
		{
			name: "both schema path and content provided",
			setupOptions: func(t *testing.T) Options {
				return Options{
					SchemaPath:    "test.cue",
					SchemaContent: "server: { host: string }",
				}
			},
			expectError:   true,
			errorContains: "cannot specify both schema path and schema content",
		},
		{
			name: "neither schema path nor content provided",
			setupOptions: func(t *testing.T) Options {
				return Options{}
			},
			expectError:   true,
			errorContains: "either schema path or schema content is required",
		},
		{
			name: "config hot reload with schema content should work",
			setupOptions: func(t *testing.T) Options {
				return Options{
					SchemaContent:         "server: { host: string | *\"localhost\" }",
					ConfigPath:            createTempFile(t, "server:\n  host: \"test\"", ".yaml"),
					EnableConfigHotReload: true,
				}
			},
			expectError:   false,
			errorContains: "",
		},
		{
			name: "schema hot reload with schema content should fail",
			setupOptions: func(t *testing.T) Options {
				return Options{
					SchemaContent:         "server: { host: string | *\"localhost\" }",
					EnableSchemaHotReload: true,
				}
			},
			expectError:   true,
			errorContains: "schema hot reload is not supported when using schema content",
		},
		{
			name: "invalid schema content",
			setupOptions: func(t *testing.T) Options {
				return Options{
					SchemaContent: "invalid cue syntax {{{",
				}
			},
			expectError:   true,
			errorContains: "failed to load schema content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := tt.setupOptions(t)
			_, err := NewManager(options)
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tt.errorContains != "" && !containsString(err.Error(), tt.errorContains) {
					t.Errorf("Expected error to contain %q, got %q", tt.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			}
		})
	}
}

// TestEnvironmentVariableSubstitution tests environment variable expansion in config files
func TestEnvironmentVariableSubstitution(t *testing.T) {
	// Set test environment variables
	os.Setenv("TEST_HOST", "env-host.com")
	os.Setenv("TEST_PORT", "3000")
	os.Setenv("TEST_DEBUG", "true")
	defer func() {
		os.Unsetenv("TEST_HOST")
		os.Unsetenv("TEST_PORT")
		os.Unsetenv("TEST_DEBUG")
	}()

	schemaPath := createTempFile(t, testSchema, ".cue")

	// Config with environment variable patterns
	configContent := `
server:
  host: "$TEST_HOST"
  port: $TEST_PORT
app:
  name: "${TEST_APP_NAME:-default-app}"
  debug: $TEST_DEBUG
logging:
  level: "info"
`

	configPath := createTempFile(t, configContent, ".yaml")

	manager, err := NewManager(Options{
		SchemaPath: schemaPath,
		ConfigPath: configPath,
	})
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Close()

	// Test that environment variables were substituted
	host, err := manager.GetString("server.host")
	if err != nil {
		t.Errorf("Failed to get server host: %v", err)
	} else if host != "env-host.com" {
		t.Errorf("Expected host to be 'env-host.com', got %q", host)
	}

	port, err := manager.GetInt("server.port")
	if err != nil {
		t.Errorf("Failed to get server port: %v", err)
	} else if port != 3000 {
		t.Errorf("Expected port to be 3000, got %d", port)
	}

	// Test default value when env var is not set
	appName, err := manager.GetString("app.name")
	if err != nil {
		t.Errorf("Failed to get app name: %v", err)
	} else if appName != "default-app" {
		t.Errorf("Expected app name to be 'default-app', got %q", appName)
	}

	debug, err := manager.GetBool("app.debug")
	if err != nil {
		t.Errorf("Failed to get debug flag: %v", err)
	} else if !debug {
		t.Errorf("Expected debug to be true, got %v", debug)
	}
}

// TestGranularHotReload tests the new granular hot reload options
func TestGranularHotReload(t *testing.T) {
	schemaPath := createTempFile(t, testSchema, ".cue")
	configContent := `
server:
  host: "localhost"
  port: 8080
app:
  name: "test-app"
  debug: false
logging:
  level: "info"
`
	configPath := createTempFile(t, configContent, ".yaml")

	tests := []struct {
		name                  string
		enableSchemaHotReload bool
		enableConfigHotReload bool
		expectError           bool
		errorContains         string
	}{
		{
			name:                  "both hot reload options enabled",
			enableSchemaHotReload: true,
			enableConfigHotReload: true,
			expectError:           false,
		},
		{
			name:                  "only schema hot reload enabled",
			enableSchemaHotReload: true,
			enableConfigHotReload: false,
			expectError:           false,
		},
		{
			name:                  "only config hot reload enabled",
			enableSchemaHotReload: false,
			enableConfigHotReload: true,
			expectError:           false,
		},
		{
			name:                  "no hot reload enabled",
			enableSchemaHotReload: false,
			enableConfigHotReload: false,
			expectError:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewManager(Options{
				SchemaPath:            schemaPath,
				ConfigPath:            configPath,
				EnableSchemaHotReload: tt.enableSchemaHotReload,
				EnableConfigHotReload: tt.enableConfigHotReload,
			})

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tt.errorContains != "" && !containsString(err.Error(), tt.errorContains) {
					t.Errorf("Expected error to contain %q, got %q", tt.errorContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("Failed to create manager: %v", err)
			}
			defer manager.Close()

			// Test that configuration values are accessible
			host, err := manager.GetString("server.host")
			if err != nil {
				t.Errorf("Failed to get server host: %v", err)
			} else if host != "localhost" {
				t.Errorf("Expected host to be 'localhost', got %q", host)
			}
		})
	}
}

// Helper function to check if a string contains a substring
func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
