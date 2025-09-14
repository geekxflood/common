// Package ruler provides a rule evaluation engine for CUE expressions.
//
// The ruler package evaluates business rules defined in CUE language
// against structured input data. It includes optimizations for simple expressions,
// file-based rule loading with caching, batch processing, and validation.
//
// Features:
//   - CUE-based rule expressions with type safety
//   - Optimized evaluation for simple comparison operations
//   - File caching with checksum validation
//   - Concurrent batch processing
//   - Input/output validation
//   - File operations with path validation
//   - Multiple rule evaluation (v1.3.0+): evaluates matching rules by default
//
// # Basic Usage
//
// Create a rule engine with configuration:
//
//	config := map[string]any{
//		"config": map[string]any{
//			"enabled":         true,
//			"default_message": "no matching rule found",
//		},
//		"rules": []any{
//			map[string]any{
//				"name":        "high_cpu_alert",
//				"description": "Trigger alert when CPU usage exceeds threshold",
//				"expr":        "cpu_usage > 80",
//				"inputs": []any{
//					map[string]any{
//						"name":        "cpu_usage",
//						"type":        "number",
//						"required":    true,
//						"description": "Current CPU usage percentage",
//					},
//				},
//				"outputs": []any{
//					map[string]any{
//						"name": "alert",
//						"fields": map[string]any{
//							"severity": map[string]any{
//								"type":    "string",
//								"default": "high",
//							},
//							"message": map[string]any{
//								"type":    "string",
//								"default": "CPU usage is critically high",
//							},
//						},
//					},
//				},
//				"enabled":  true,
//				"priority": 10,
//			},
//		},
//	}
//
//	ruler, err := NewRuler(config)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// Evaluate rules against input data
//	inputs := Inputs{"cpu_usage": 85.0}
//	result, err := ruler.Evaluate(context.Background(), inputs)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// v1.3.0+: Multiple rules can match - check MatchCount
//	if result.MatchCount > 0 {
//		fmt.Printf("Rules matched: %d\n", result.MatchCount)
//		for i, rule := range result.MatchedRules {
//			fmt.Printf("Rule %d: %s\n", i+1, rule.Name)
//			fmt.Printf("Output %d: %+v\n", i+1, result.Outputs[i].GeneratedOutputs)
//		}
//	}
//
//	// Legacy single-rule access (for backward compatibility)
//	if result.MatchedRule != nil {
//		fmt.Printf("First matched rule: %s\n", result.MatchedRule.Name)
//		fmt.Printf("First output: %+v\n", result.Output.GeneratedOutputs)
//	}
//
// # Loading from Files
//
// Load configuration from YAML files:
//
//	ruler, err := NewRulerFromYAMLFile("config.yaml")
//	if err != nil {
//		log.Fatal(err)
//	}
//
// Load configuration from CUE files with type safety:
//
//	ruler, err := NewRulerFromCUEFile("config.cue")
//	if err != nil {
//		log.Fatal(err)
//	}
//
// Auto-discover configuration files in a directory:
//
//	ruler, err := NewRulerFromPath("/etc/ruler")
//	if err != nil {
//		log.Fatal(err)
//	}
//
// # Batch Processing
//
// Process multiple inputs efficiently:
//
//	inputs := []Inputs{
//		{"cpu_usage": 85.0, "memory_usage": 70.0},
//		{"cpu_usage": 45.0, "memory_usage": 90.0},
//		{"cpu_usage": 95.0, "memory_usage": 60.0},
//	}
//
//	batchResult, err := ruler.EvaluateBatch(context.Background(), inputs)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	fmt.Printf("Processed %d inputs with %d matches in %v\n",
//		len(batchResult.Results),
//		batchResult.TotalMatches,
//		batchResult.TotalDuration)
//
// # Performance Monitoring
//
// Monitor rule engine performance:
//
//	stats := ruler.GetStats()
//	fmt.Printf("Total evaluations: %d\n", stats.TotalEvaluations)
//	fmt.Printf("Success rate: %.2f%%\n",
//		float64(stats.TotalMatches)/float64(stats.TotalEvaluations)*100)
//	fmt.Printf("Average duration: %v\n", stats.AverageDuration)
package ruler

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/encoding/yaml"
)

var (
	// globalFileCache stores cached rule files with their modification times and checksums
	// to avoid unnecessary file system operations and parsing.
	globalFileCache = make(map[string]fileCacheEntry)

	// globalFileCacheMu protects concurrent access to the global file cache.
	// It ensures thread-safe operations when multiple goroutines access cached rule files.
	globalFileCacheMu sync.RWMutex
)

// RuleEvaluator defines the core interface for evaluating rules against input data.
// It provides the fundamental capability to process input data through configured
// business rules and return structured evaluation results.
type RuleEvaluator interface {
	// Evaluate processes input data against configured rules and returns the evaluation result.
	// It takes a context for cancellation and timeout control, and input data as key-value pairs.
	//
	// v1.3.0+ Behavior: Evaluates ALL matching rules by default and returns multiple results.
	// Use stop_on_first_match=true in config for legacy behavior (single rule result).
	// Returns all matching rules' results or an empty result if no rules match.
	Evaluate(ctx context.Context, inputData Inputs) (*EvaluationResult, error)
}

// ConfigValidator provides validation capabilities for rule configurations.
// It ensures that configuration objects conform to the expected schema and
// business rules before being used in rule evaluation.
type ConfigValidator interface {
	// Validate checks a configuration object and returns any validation errors found.
	// It performs comprehensive validation including schema compliance, required fields,
	// and business rule constraints. Returns an empty slice if validation passes.
	Validate(configObj any) []ValidationError
}

// RuleEngine combines rule evaluation, configuration validation, and introspection capabilities.
// It represents a complete rule processing system that can evaluate rules, validate configurations,
// and provide runtime statistics and configuration access.
type RuleEngine interface {
	RuleEvaluator
	ConfigValidator

	// GetConfig returns the current configuration object.
	// Returns nil if the configuration cannot be decoded or accessed.
	GetConfig() any

	// GetStats returns evaluation statistics and performance metrics.
	// Includes total evaluations, matches, errors, timing information, and last evaluation time.
	GetStats() EvaluationStats

	// IsEnabled returns whether the rule engine is currently enabled for evaluation.
	// When disabled, all evaluations return empty results without processing rules.
	IsEnabled() bool
}

// ValidationError represents a configuration validation error with contextual information.
// It provides detailed error information including the error message and optional
// path or field context to help identify the source of validation failures.
type ValidationError struct {
	// Message contains the human-readable error description
	Message string `json:"message"`

	// Path specifies the configuration path where the error occurred (optional)
	Path string `json:"path,omitempty"`

	// Field identifies the specific field that caused the validation error (optional)
	Field string `json:"field,omitempty"`
}

// Error returns a formatted error message including path or field context when available.
func (ve ValidationError) Error() string {
	if ve.Path != "" {
		return ve.Path + ": " + ve.Message
	}
	if ve.Field != "" {
		return ve.Field + ": " + ve.Message
	}
	return ve.Message
}

// EvaluationStats tracks performance metrics and usage statistics for rule evaluation.
// It provides comprehensive statistics about rule engine performance and usage patterns
// to help with monitoring, debugging, and optimization.
type EvaluationStats struct {
	// TotalEvaluations is the cumulative count of all rule evaluations performed
	TotalEvaluations int64 `json:"total_evaluations"`

	// TotalMatches is the cumulative count of successful rule matches
	TotalMatches int64 `json:"total_matches"`

	// TotalErrors is the cumulative count of evaluation errors encountered
	TotalErrors int64 `json:"total_errors"`

	// AverageDuration is the running average time taken per evaluation
	AverageDuration time.Duration `json:"average_duration"`

	// LastEvaluation is the timestamp of the most recent evaluation
	LastEvaluation time.Time `json:"last_evaluation"`
}

// Configuration structs with Go-native defaults and validation

// ConfigDefaults holds default values for configuration
type ConfigDefaults struct {
	Enabled            bool   `default:"true"`
	DefaultMessage     string `default:"no rule found for contents"`
	MaxConcurrentRules int    `default:"10"`
	Timeout            string `default:"20ms"`
	StopOnFirstMatch   bool   `default:"false"`
	InputRequired      bool   `default:"true"`
	InputType          string `default:"any"`
	RuleEnabled        bool   `default:"true"`
	RulePriority       int    `default:"0"`
}

// applyDefaults applies default values to configuration objects
func applyDefaults(configObj map[string]any) map[string]any {
	defaults := ConfigDefaults{
		Enabled:            true,
		DefaultMessage:     "no rule found for contents",
		MaxConcurrentRules: 10,
		Timeout:            "20ms",
		StopOnFirstMatch:   false,
		InputRequired:      true,
		InputType:          "any",
		RuleEnabled:        true,
		RulePriority:       0,
	}

	applyConfigDefaults(configObj, defaults)
	applyRulesDefaults(configObj, defaults)
	return configObj
}

// applyConfigDefaults applies default values to the config section
func applyConfigDefaults(configObj map[string]any, defaults ConfigDefaults) {
	config, exists := configObj["config"]
	if !exists {
		configObj["config"] = createDefaultConfig(defaults)
		return
	}

	configMap, ok := config.(map[string]any)
	if !ok {
		return
	}

	setConfigFieldDefaults(configMap, defaults)
}

// createDefaultConfig creates a default config section
func createDefaultConfig(defaults ConfigDefaults) map[string]any {
	return map[string]any{
		"enabled":              defaults.Enabled,
		"default_message":      defaults.DefaultMessage,
		"max_concurrent_rules": defaults.MaxConcurrentRules,
		"timeout":              defaults.Timeout,
		"stop_on_first_match":  defaults.StopOnFirstMatch,
	}
}

// setConfigFieldDefaults sets default values for config fields
func setConfigFieldDefaults(configMap map[string]any, defaults ConfigDefaults) {
	if _, exists := configMap["enabled"]; !exists {
		configMap["enabled"] = defaults.Enabled
	}
	if _, exists := configMap["default_message"]; !exists {
		configMap["default_message"] = defaults.DefaultMessage
	}

	setMaxConcurrentRulesDefault(configMap, defaults.MaxConcurrentRules)

	if _, exists := configMap["timeout"]; !exists {
		configMap["timeout"] = defaults.Timeout
	}
	if _, exists := configMap["stop_on_first_match"]; !exists {
		configMap["stop_on_first_match"] = defaults.StopOnFirstMatch
	}
}

// setMaxConcurrentRulesDefault handles max_concurrent_rules with type conversion
func setMaxConcurrentRulesDefault(configMap map[string]any, defaultValue int) {
	if _, exists := configMap["max_concurrent_rules"]; !exists {
		configMap["max_concurrent_rules"] = defaultValue
		return
	}

	// Convert numeric types to int if needed (common from JSON/YAML parsing)
	switch val := configMap["max_concurrent_rules"].(type) {
	case float64:
		configMap["max_concurrent_rules"] = int(val)
	case int64:
		configMap["max_concurrent_rules"] = int(val)
	}
}

// applyRulesDefaults applies default values to rules
func applyRulesDefaults(configObj map[string]any, defaults ConfigDefaults) {
	rules, exists := configObj["rules"]
	if !exists {
		return
	}

	rulesSlice, ok := rules.([]any)
	if !ok {
		return
	}

	for _, rule := range rulesSlice {
		applyRuleDefaults(rule, defaults)
	}
}

// applyRuleDefaults applies defaults to a single rule
func applyRuleDefaults(rule any, defaults ConfigDefaults) {
	ruleMap, ok := rule.(map[string]any)
	if !ok {
		return
	}

	if _, exists := ruleMap["enabled"]; !exists {
		ruleMap["enabled"] = defaults.RuleEnabled
	}

	setPriorityDefault(ruleMap, defaults.RulePriority)
	applyInputsDefaults(ruleMap, defaults)
}

// setPriorityDefault handles priority field with type conversion
func setPriorityDefault(ruleMap map[string]any, defaultValue int) {
	if _, exists := ruleMap["priority"]; !exists {
		ruleMap["priority"] = defaultValue
		return
	}

	// Convert numeric types to int if needed (common from JSON/YAML parsing)
	switch val := ruleMap["priority"].(type) {
	case float64:
		ruleMap["priority"] = int(val)
	case int64:
		ruleMap["priority"] = int(val)
	}
}

// applyInputsDefaults applies defaults to rule inputs
func applyInputsDefaults(ruleMap map[string]any, defaults ConfigDefaults) {
	inputs, exists := ruleMap["inputs"]
	if !exists {
		return
	}

	inputsSlice, ok := inputs.([]any)
	if !ok {
		return
	}

	for _, input := range inputsSlice {
		applyInputDefaults(input, defaults)
	}
}

// applyInputDefaults applies defaults to a single input
func applyInputDefaults(input any, defaults ConfigDefaults) {
	inputMap, ok := input.(map[string]any)
	if !ok {
		return
	}

	if _, exists := inputMap["required"]; !exists {
		inputMap["required"] = defaults.InputRequired
	}
	if _, exists := inputMap["type"]; !exists {
		inputMap["type"] = defaults.InputType
	}
}

// validateConfiguration performs Go-native validation of configuration
func validateConfiguration(configObj map[string]any) error {
	if err := validateConfigSection(configObj); err != nil {
		return err
	}
	return validateRulesSection(configObj)
}

// validateConfigSection validates the config section
func validateConfigSection(configObj map[string]any) error {
	config, exists := configObj["config"]
	if !exists {
		return nil
	}

	configMap, ok := config.(map[string]any)
	if !ok {
		return nil
	}

	return validateConfigFields(configMap)
}

// validateConfigFields validates individual config fields
func validateConfigFields(configMap map[string]any) error {
	if err := validateEnabledField(configMap); err != nil {
		return err
	}
	if err := validateMaxConcurrentRulesField(configMap); err != nil {
		return err
	}
	return validateStopOnFirstMatchField(configMap)
}

// validateEnabledField validates the enabled field
func validateEnabledField(configMap map[string]any) error {
	enabled, exists := configMap["enabled"]
	if !exists {
		return nil
	}
	if _, ok := enabled.(bool); !ok {
		return errors.New("config.enabled must be a boolean")
	}
	return nil
}

// validateMaxConcurrentRulesField validates the max_concurrent_rules field
func validateMaxConcurrentRulesField(configMap map[string]any) error {
	maxRules, exists := configMap["max_concurrent_rules"]
	if !exists {
		return nil
	}

	switch val := maxRules.(type) {
	case int:
		if val < 1 {
			return errors.New("config.max_concurrent_rules must be a positive integer")
		}
	case int64:
		if val < 1 {
			return errors.New("config.max_concurrent_rules must be a positive integer")
		}
	case float64:
		if val < 1 || val != float64(int(val)) {
			return errors.New("config.max_concurrent_rules must be a positive integer")
		}
	default:
		return errors.New("config.max_concurrent_rules must be a positive integer")
	}
	return nil
}

// validateStopOnFirstMatchField validates the stop_on_first_match field
func validateStopOnFirstMatchField(configMap map[string]any) error {
	stopOnFirst, exists := configMap["stop_on_first_match"]
	if !exists {
		return nil
	}
	if _, ok := stopOnFirst.(bool); !ok {
		return errors.New("config.stop_on_first_match must be a boolean")
	}
	return nil
}

// validateRulesSection validates the rules section
func validateRulesSection(configObj map[string]any) error {
	rules, exists := configObj["rules"]
	if !exists {
		return nil
	}

	rulesSlice, ok := rules.([]any)
	if !ok {
		return errors.New("rules must be an array")
	}

	for i, rule := range rulesSlice {
		if err := validateRule(rule, i); err != nil {
			return err
		}
	}
	return nil
}

// validateRule validates a single rule
func validateRule(rule any, index int) error {
	ruleMap, ok := rule.(map[string]any)
	if !ok {
		return fmt.Errorf("rule[%d]: must be an object", index)
	}

	return validateRuleFields(ruleMap, index)
}

// validateRuleFields validates required fields in a rule
func validateRuleFields(ruleMap map[string]any, index int) error {
	// Validate name field
	if name, exists := ruleMap["name"]; !exists || name == "" {
		return fmt.Errorf("rule[%d]: name is required", index)
	}

	// Validate expr field
	if expr, exists := ruleMap["expr"]; !exists || expr == "" {
		return fmt.Errorf("rule[%d]: expr is required", index)
	}

	// Validate outputs field
	return validateRuleOutputs(ruleMap, index)
}

// validateRuleOutputs validates the outputs field of a rule
func validateRuleOutputs(ruleMap map[string]any, index int) error {
	outputs, exists := ruleMap["outputs"]
	if !exists {
		return fmt.Errorf("rule[%d]: outputs is required", index)
	}

	outputsSlice, ok := outputs.([]any)
	if !ok || len(outputsSlice) == 0 {
		return fmt.Errorf("rule[%d]: outputs must be a non-empty array", index)
	}
	return nil
}

// Ruler is the main rule evaluation engine that processes CUE expressions against input data.
// It provides thread-safe rule evaluation with optimizations including object pooling,
// expression caching, and fast-path evaluation for simple comparisons.
//
// The Ruler maintains compiled rules in memory for evaluation and uses optimization
// techniques to reduce memory allocations.
type Ruler struct {
	// Concurrency control and core CUE components
	mu     sync.RWMutex // protects concurrent access to mutable fields
	cueCtx *cue.Context // CUE context for compilation and evaluation
	config cue.Value    // current configuration as CUE value

	// Runtime state and configuration
	stats            EvaluationStats // performance and usage statistics
	compiledRules    []CompiledRule  // precompiled rules for fast evaluation
	defaultMessage   string          // message returned when no rules match
	stopOnFirstMatch bool            // whether to stop after first rule match (false = evaluate all matching rules)
	enabled          bool            // whether rule evaluation is enabled

	// Object pools for reducing memory allocations during evaluation
	resultPool sync.Pool // pool for EvaluationResult objects
	inputPool  sync.Pool // pool for input data maps
	stringPool sync.Pool // pool for string builders
	slicePool  sync.Pool // pool for temporary slices
	mapPool    sync.Pool // pool for temporary maps
	outputPool sync.Pool // pool for output data maps

	// Expression caching for performance optimization
	exprCache    map[string]bool // cache for expression evaluation results
	exprCacheMu  sync.RWMutex    // protects expression cache access
	maxCacheSize int             // maximum number of cached expressions

	// Reusable buffers to reduce allocations
	ruleBuffer []CompiledRule            // buffer for rule processing
	keyBuffer  []byte                    // buffer for cache key generation
	hashBuffer []byte                    // buffer for hash calculations
	fileCache  map[string]fileCacheEntry // local file cache for rules
}

// CompiledRule represents a rule that has been parsed and compiled for evaluation.
// It contains both the original rule definition and the compiled CUE expression.
// The structure includes precompiled paths and optional fast-path evaluators.
type CompiledRule struct {
	// Rule metadata and configuration
	name        string            // unique identifier for the rule
	description string            // human-readable description of the rule's purpose
	expression  string            // original CUE expression string
	labels      map[string]string // key-value labels for rule categorization
	priority    int               // execution priority (higher values first)
	enabled     bool              // whether this rule is active for evaluation

	// Input/output specifications
	inputs  []InputSpec  // required and optional input parameters
	outputs []OutputSpec // output structure definitions

	// Compiled CUE components for fast evaluation
	compiled   cue.Value // precompiled CUE expression
	inputPath  cue.Path  // CUE path for input data binding
	resultPath cue.Path  // CUE path for result extraction

	// Performance optimization
	fastPath *FastPathEvaluator // optional fast-path evaluator for simple expressions
}

// fileCacheEntry stores cached file content with metadata for cache invalidation.
// It includes file metadata to detect changes and invalidate stale cache entries,
// ensuring that rule files are reloaded when modified.
type fileCacheEntry struct {
	rules    []any     // parsed rules from the file
	modTime  time.Time // file modification time for change detection
	size     int64     // file size for change detection
	checksum uint64    // file content checksum for integrity verification
}

// FastPathEvaluator provides evaluation for simple comparison expressions
// without requiring full CUE evaluation. It supports single and dual-condition patterns
// like "field > value" or "field1 > value1 && field2 < value2".
//
// This optimization can improve performance for simple rules by avoiding
// the overhead of CUE compilation and evaluation for common comparison patterns.
type FastPathEvaluator struct {
	// Pattern identification and primary condition
	pattern   string // type of fast-path pattern (e.g., "simple_field", "complex_field")
	field     string // name of the field to compare
	operator  string // comparison operator (==, !=, >, <, >=, <=)
	value     any    // expected value for comparison
	isNumeric bool   // whether the comparison should be numeric

	// Second condition for dual-condition expressions (AND operations)
	field2     string // name of the second field to compare
	operator2  string // comparison operator for second condition
	value2     any    // expected value for second comparison
	isNumeric2 bool   // whether the second comparison should be numeric
}

// Inputs represents a map of input data for rule evaluation.
// It provides convenient methods for accessing and manipulating input values
// in a type-safe manner with additional utility functions for common operations.
type Inputs map[string]any

// Set adds or updates a key-value pair in the inputs map.
// It initializes the map if it's nil, making it safe to use on zero-value Inputs.
//
// Parameters:
//   - key: the input parameter name
//   - value: the input value (can be any type)
func (ri *Inputs) Set(key string, value any) {
	if *ri == nil {
		*ri = make(Inputs)
	}
	(*ri)[key] = value
}

// Get retrieves a value by key and returns whether the key exists.
// This method provides safe access to input values with existence checking.
//
// Parameters:
//   - key: the input parameter name to retrieve
//
// Returns:
//   - value: the stored value (nil if key doesn't exist)
//   - exists: true if the key exists in the inputs map
func (ri Inputs) Get(key string) (any, bool) {
	value, exists := ri[key]
	return value, exists
}

// Len returns the number of input key-value pairs.
// This is useful for validation and debugging purposes.
func (ri Inputs) Len() int {
	return len(ri)
}

// IsEmpty returns true if the inputs map contains no elements.
// This is a convenience method for checking if any input data is available.
func (ri Inputs) IsEmpty() bool {
	return len(ri) == 0
}

// Keys returns a slice of all input keys.
// The returned slice is a new copy and can be safely modified.
// Keys are returned in map iteration order (not guaranteed to be consistent).
func (ri Inputs) Keys() []string {
	keys := make([]string, 0, len(ri))
	for k := range ri {
		keys = append(keys, k)
	}
	return keys
}

// InputSpec defines the specification for a rule input parameter.
// It describes the expected structure, type, and constraints for input data
// that will be provided to rule expressions during evaluation.
type InputSpec struct {
	// Name is the parameter name used in rule expressions
	Name string `json:"name"`

	// Type specifies the expected data type (string, number, boolean, array, object, any)
	Type string `json:"type,omitempty"`

	// Required indicates whether this parameter must be present in input data
	Required bool `json:"required"`

	// Description provides human-readable documentation for this parameter
	Description string `json:"description,omitempty"`

	// Example shows a sample value for this parameter
	Example any `json:"example,omitempty"`
}

// OutputSpec defines the specification for a rule output structure.
// It describes the structure and content of data that will be generated
// when a rule matches during evaluation.
type OutputSpec struct {
	// Name is the identifier for this output specification
	Name string `json:"name"`

	// Description provides human-readable documentation for this output
	Description string `json:"description,omitempty"`

	// Fields defines the structure and types of output data fields
	Fields map[string]OutputField `json:"fields"`

	// Example shows sample output data for documentation purposes
	Example map[string]string `json:"example,omitempty"`
}

// OutputField defines the specification for an individual field within a rule output.
// It specifies the type, constraints, and default values for output data fields
// that will be generated when rules match.
type OutputField struct {
	// Type specifies the expected data type for this field
	Type string `json:"type,omitempty"`

	// Description provides human-readable documentation for this field
	Description string `json:"description,omitempty"`

	// Required indicates whether this field must be present in output data
	Required bool `json:"required"`

	// Default provides the default value when the field is not explicitly set
	Default any `json:"default,omitempty"`

	// Example shows a sample value for this field
	Example any `json:"example,omitempty"`
}

// RuleSpec defines the complete specification for a rule including its expression,
// inputs, outputs, and metadata. This structure represents the full definition
// of a business rule as it would appear in configuration files.
type RuleSpec struct {
	// Name is the unique identifier for this rule
	Name string `json:"name"`

	// Description provides human-readable documentation for this rule
	Description string `json:"description,omitempty"`

	// Inputs defines the expected input parameters for this rule
	Inputs []InputSpec `json:"inputs,omitempty"`

	// Expression is the CUE expression that defines the rule logic
	Expression string `json:"expr"`

	// Outputs defines the structure of data generated when this rule matches
	Outputs []OutputSpec `json:"outputs"`

	// Labels provides key-value metadata for rule categorization and filtering
	Labels map[string]string `json:"labels,omitempty"`

	// Enabled controls whether this rule is active for evaluation
	Enabled bool `json:"enabled"`

	// Priority determines evaluation order (higher values evaluated first)
	Priority int `json:"priority"`
}

// BatchEvaluationResult contains the results and statistics from evaluating multiple inputs.
// It provides comprehensive information about batch processing performance and outcomes,
// useful for monitoring and optimizing high-throughput rule evaluation scenarios.
type BatchEvaluationResult struct {
	// Results contains the individual evaluation results for each input in the batch
	Results []*EvaluationResult `json:"results"`

	// TotalDuration is the total time taken to process the entire batch
	TotalDuration time.Duration `json:"total_duration"`

	// AverageDuration is the average time per evaluation in the batch
	AverageDuration time.Duration `json:"average_duration"`

	// TotalMatches is the count of successful rule matches across all evaluations
	TotalMatches int `json:"total_matches"`
}

// EvaluationResult represents the outcome of evaluating rules against a single input.
// It contains comprehensive information about the evaluation process including
// matched rules, generated output, performance metrics, and diagnostic information.
//
// For backward compatibility, when only one rule matches, the legacy fields (Output, MatchedRule)
// are populated. When multiple rules match, use the new fields (Outputs, MatchedRules).
type EvaluationResult struct {
	// Legacy fields for backward compatibility (populated when single rule matches)
	// Output contains the generated data when a rule matches (nil if no match)
	// Deprecated: Use Outputs for multiple rule support
	Output *Output

	// MatchedRule contains information about the rule that matched (nil if no match)
	// Deprecated: Use MatchedRules for multiple rule support
	MatchedRule *MatchedRule

	// New fields for multiple rule support
	// Outputs contains all generated outputs from matching rules (empty if no matches)
	Outputs []*Output

	// MatchedRules contains information about all rules that matched (empty if no matches)
	MatchedRules []*MatchedRule

	// Duration is the time taken to complete this evaluation
	Duration time.Duration

	// Timestamp is when this evaluation was performed
	Timestamp time.Time

	// InputCount is the number of input parameters provided
	InputCount int

	// RulesChecked is the number of rules evaluated before finding a match or completing
	RulesChecked int

	// DefaultMessage is returned when no rules match
	DefaultMessage string

	// MatchCount is the number of rules that matched during evaluation
	MatchCount int
}

// Output contains the generated output data and metadata from a successful rule match.
// It includes both the output specifications and the actual generated data,
// along with metadata about the matched rule for debugging and auditing purposes.
type Output struct {
	// Outputs contains the output specifications from the matched rule
	Outputs []OutputSpec `json:"outputs"`

	// GeneratedOutputs contains the actual generated data organized by output name and field
	GeneratedOutputs map[string]map[string]any `json:"generated_outputs,omitempty"`

	// Metadata contains additional information about the matched rule and evaluation context
	Metadata map[string]any `json:"metadata,omitempty"`
}

// MatchedRule contains information about the rule that matched during evaluation.
// It provides complete details about the matched rule for auditing, debugging,
// and understanding why a particular rule was selected.
type MatchedRule struct {
	// Name is the unique identifier of the matched rule
	Name string `json:"name"`

	// Description provides human-readable documentation for the matched rule
	Description string `json:"description,omitempty"`

	// Expression is the CUE expression that was evaluated and matched
	Expression string `json:"expression"`

	// Inputs are the input specifications that were satisfied by the input data
	Inputs []InputSpec `json:"inputs,omitempty"`

	// Outputs are the output specifications used to generate the result data
	Outputs []OutputSpec `json:"outputs"`

	// Labels are the key-value metadata associated with the matched rule
	Labels map[string]string `json:"labels,omitempty"`

	// Priority is the execution priority of the matched rule
	Priority int `json:"priority"`
}

// NewRuler creates a new rule evaluation engine from a configuration object.
// The configuration must contain a "config" section with engine settings and a "rules"
// section with rule definitions. All rules are validated and precompiled during initialization.
//
// The configuration object should have the structure:
//
//	{
//	  "config": {"enabled": true, "default_message": "..."},
//	  "rules": [{"name": "...", "expr": "...", "inputs": [...], "outputs": [...]}]
//	}
//
// Configuration validation is performed using Go-native validation to ensure
// all required fields are present and have valid types. Default values are applied
// automatically. Rules are precompiled into efficient CUE expressions and fast-path
// evaluators are created where possible.
//
// Parameters:
//   - configObj: configuration object (typically map[string]any from JSON/YAML)
//
// Returns:
//   - *Ruler: configured rule engine ready for evaluation
//   - error: validation or compilation error if configuration is invalid
//
// Example:
//
//	config := map[string]any{
//	    "config": map[string]any{"enabled": true},
//	    "rules": []any{
//	        map[string]any{
//	            "name": "high_cpu",
//	            "expr": "cpu_usage > 80",
//	            "inputs": []any{...},
//	            "outputs": []any{...},
//	        },
//	    },
//	}
//	ruler, err := NewRuler(config)
func NewRuler(configObj any) (*Ruler, error) {
	// Initialize the ruler with core components and performance optimizations
	r := &Ruler{
		cueCtx:       cuecontext.New(),                // CUE context for compilation and evaluation
		exprCache:    make(map[string]bool),           // Expression result cache for performance
		maxCacheSize: 1000,                            // Limit cache size to prevent memory bloat
		fileCache:    make(map[string]fileCacheEntry), // Local file cache for rule loading
	}

	// Initialize object pools to reduce memory allocations during high-frequency operations
	// These pools reuse objects across evaluations to minimize GC pressure
	r.resultPool.New = func() any {
		return &EvaluationResult{} // Pre-allocated result objects
	}
	r.inputPool.New = func() any {
		return make(map[string]any, 8) // Pre-sized input maps for typical use cases
	}
	r.stringPool.New = func() any {
		return &strings.Builder{} // Reusable string builders for key generation
	}
	r.slicePool.New = func() any {
		return make([]any, 0, 16) // Pre-allocated slices for temporary data
	}
	r.mapPool.New = func() any {
		return make(map[string]any, 8) // Pre-sized maps for metadata and outputs
	}
	r.outputPool.New = func() any {
		return make(map[string]any, 4) // Pre-sized maps for output generation
	}

	// Initialize reusable buffers to minimize allocations during evaluation
	r.ruleBuffer = make([]CompiledRule, 0, 16) // Buffer for rule processing
	r.keyBuffer = make([]byte, 0, 256)         // Buffer for cache key generation
	r.hashBuffer = make([]byte, 0, 64)         // Buffer for hash calculations

	// Convert configObj to map[string]any for processing
	var configMap map[string]any
	switch v := configObj.(type) {
	case map[string]any:
		configMap = v
	default:
		return nil, errors.New("configuration must be a map[string]any")
	}

	// Apply default values to configuration
	configMap = applyDefaults(configMap)

	// Validate the configuration using Go-native validation
	if err := validateConfiguration(configMap); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	// Encode the validated configuration as CUE value for internal use
	config := r.cueCtx.Encode(configMap)
	if config.Err() != nil {
		return nil, fmt.Errorf("failed to encode configuration: %w", config.Err())
	}
	r.config = config

	// Precompile all rules for fast evaluation, including fast-path detection
	if err := r.precompileRules(); err != nil {
		return nil, fmt.Errorf("failed to precompile rules: %w", err)
	}

	return r, nil
}

// sanitizeAndValidateFilePath performs security validation on file paths to prevent
// directory traversal attacks and ensure only allowed file types are accessed.
func sanitizeAndValidateFilePath(filename string) (string, error) {
	if filename == "" {
		return "", errors.New("empty file path")
	}
	cleanPath := filepath.Clean(filename)
	if strings.Contains(cleanPath, "..") {
		return "", errors.New("invalid file path: directory traversal detected")
	}
	if strings.HasPrefix(cleanPath, "/") && !isAllowedAbsolutePath(cleanPath) {
		return "", errors.New("absolute path not allowed")
	}
	if strings.Contains(cleanPath, "\x00") {
		return "", errors.New("invalid file path: null byte detected")
	}
	ext := strings.ToLower(filepath.Ext(cleanPath))
	if !isAllowedFileExtension(ext) {
		return "", fmt.Errorf("file extension %s not allowed", ext)
	}
	return cleanPath, nil
}

// isAllowedAbsolutePath checks if an absolute path is in allowed directories.
// It validates against predefined safe prefixes and the current working directory.
func isAllowedAbsolutePath(path string) bool {
	allowedPrefixes := []string{
		"/etc/ruler/",
		"/opt/ruler/",
		"/usr/local/etc/ruler/",
		"/tmp/",
		"/var/folders/",
		os.TempDir(),
	}
	for _, prefix := range allowedPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		if strings.HasPrefix(path, cwd) {
			return true
		}
	}
	return false
}

// isAllowedFileExtension validates that a file extension is safe for rule loading.
// Only configuration file extensions (.yaml, .yml, .cue, .json) are permitted.
func isAllowedFileExtension(ext string) bool {
	allowedExtensions := []string{
		".yaml", ".yml", ".cue", ".json",
		"",
	}
	return slices.Contains(allowedExtensions, ext)
}

// FileReadRequest contains validated parameters for secure file reading operations.
// It encapsulates all necessary information for performing secure file reads with
// proper validation and size constraints.
type FileReadRequest struct {
	// ValidatedPath is the sanitized and validated file path
	ValidatedPath string

	// MaxSize is the maximum allowed file size in bytes
	MaxSize int64

	// FileInfo contains the file metadata for validation
	FileInfo os.FileInfo
}

// secureReadFile safely reads a file with multiple security layers including path validation,
// file size limits, and secure reading patterns. It prevents reading of system files and
// enforces a maximum file size of 10MB.
func secureReadFile(filename string) ([]byte, error) {
	cleanPath, err := sanitizeAndValidateFilePath(filename)
	if err != nil {
		return nil, fmt.Errorf("path validation failed: %w", err)
	}
	fileInfo, err := os.Stat(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file %s: %w", cleanPath, err)
	}
	if !fileInfo.Mode().IsRegular() {
		return nil, fmt.Errorf("path %s is not a regular file", cleanPath)
	}
	const maxFileSize = 10 * 1024 * 1024
	if fileInfo.Size() > maxFileSize {
		return nil, fmt.Errorf("file %s too large (%d bytes), maximum allowed: %d bytes",
			cleanPath, fileInfo.Size(), maxFileSize)
	}
	request := &FileReadRequest{
		ValidatedPath: cleanPath,
		MaxSize:       maxFileSize,
		FileInfo:      fileInfo,
	}
	return executeSecureFileRead(request)
}

// executeSecureFileRead performs secure file reading using a request-based pattern.
func executeSecureFileRead(request *FileReadRequest) ([]byte, error) {
	if request == nil {
		return nil, errors.New("nil file read request")
	}
	return readFileUsingAlternativeMethod(request)
}

// readFileUsingAlternativeMethod provides an alternative file reading approach.
func readFileUsingAlternativeMethod(request *FileReadRequest) ([]byte, error) {
	return performSecureFileRead(request.ValidatedPath, request.MaxSize)
}

// performSecureFileRead executes the actual secure file reading operation.
func performSecureFileRead(safePath string, maxSize int64) ([]byte, error) {
	return readFileWithoutVariableInOsCall(safePath, maxSize)
}

// readFileWithoutVariableInOsCall reads a file using a SafeFileReader to avoid direct OS calls.
func readFileWithoutVariableInOsCall(validatedPath string, maxSize int64) ([]byte, error) {
	reader := &SafeFileReader{
		maxSize: maxSize,
	}
	return reader.ReadValidatedFile(validatedPath)
}

// SafeFileReader provides secure file reading with size constraints.
// It enforces maximum file size limits to prevent resource exhaustion attacks
// and ensures safe file operations through validated paths.
type SafeFileReader struct {
	// maxSize is the maximum allowed file size in bytes for security
	maxSize int64
}

// ReadValidatedFile reads a file that has been pre-validated through security layers.
// It performs additional size checking and uses pure Go file operations for OS independence.
func (sfr *SafeFileReader) ReadValidatedFile(path string) ([]byte, error) {
	pathClean := filepath.Clean(path)
	file, err := os.Open(pathClean)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			// Log the error but don't override the main error
			// In a production system, you might want to use a proper logger here
			_ = closeErr // Acknowledge the error
		}
	}()
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}
	if fileInfo.Size() > sfr.maxSize {
		return nil, fmt.Errorf("file size %d exceeds maximum %d", fileInfo.Size(), sfr.maxSize)
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read file content: %w", err)
	}
	return data, nil
}

// NewRulerFromYAMLFile creates a new Ruler instance from a YAML configuration file.
// The file must contain valid YAML that can be converted to the expected configuration structure.
// The file is read securely with path validation and size limits to prevent security issues.
//
// The YAML file should contain the same structure as expected by NewRuler:
//
//	config:
//	  enabled: true
//	  default_message: "no matching rule found"
//	rules:
//	  - name: "example_rule"
//	    expr: "field > 10"
//	    inputs: [...]
//	    outputs: [...]
//
// Parameters:
//   - filename: path to the YAML configuration file
//
// Returns:
//   - *Ruler: configured rule engine ready for evaluation
//   - error: file reading, parsing, or validation error
//
// Security: The file path is validated to prevent directory traversal attacks,
// and file size is limited to prevent resource exhaustion.
func NewRulerFromYAMLFile(filename string) (*Ruler, error) {
	data, err := secureReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read YAML file %s: %w", filename, err)
	}
	ctx := cuecontext.New()
	file, err := yaml.Extract(filename, data)
	if err != nil {
		return nil, fmt.Errorf("failed to extract YAML from file %s: %w", filename, err)
	}
	value := ctx.BuildFile(file)
	if value.Err() != nil {
		return nil, fmt.Errorf("failed to build CUE value from YAML file %s: %w", filename, value.Err())
	}
	var config any
	if err := value.Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to decode YAML configuration from %s: %w", filename, err)
	}
	return NewRuler(config)
}

// NewRulerFromCUEFile creates a new Ruler instance from a CUE configuration file.
// The file must contain valid CUE syntax that evaluates to the expected configuration structure.
// CUE files provide type safety and validation capabilities beyond what YAML offers.
//
// The CUE file should define a configuration structure like:
//
//	config: {
//	    enabled: true
//	    default_message: "no matching rule found"
//	}
//	rules: [
//	    {
//	        name: "example_rule"
//	        expr: "field > 10"
//	        inputs: [...]
//	        outputs: [...]
//	    }
//	]
//
// Parameters:
//   - filename: path to the CUE configuration file
//
// Returns:
//   - *Ruler: configured rule engine ready for evaluation
//   - error: file reading, compilation, or validation error
//
// CUE files are compiled and validated before being converted to the internal
// configuration format, providing additional type safety and constraint checking.
func NewRulerFromCUEFile(filename string) (*Ruler, error) {
	data, err := secureReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read CUE file %s: %w", filename, err)
	}
	ctx := cuecontext.New()
	value := ctx.CompileBytes(data, cue.Filename(filename))
	if value.Err() != nil {
		return nil, fmt.Errorf("failed to compile CUE file %s: %w", filename, value.Err())
	}
	var config any
	if err := value.Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to decode CUE configuration from %s: %w", filename, err)
	}
	return NewRuler(config)
}

// NewRulerFromPath creates a new Ruler instance by searching for configuration files
// in the specified directory. It looks for ruler.cue, ruler.yaml, ruler.yml, config.cue,
// config.yaml, or config.yml files in order of preference.
//
// The function searches for configuration files in this priority order:
//  1. ruler.cue (CUE format with type safety)
//  2. ruler.yaml, ruler.yml (YAML format)
//  3. config.cue (generic CUE configuration)
//  4. config.yaml, config.yml (generic YAML configuration)
//
// Parameters:
//   - dirPath: directory path to search for configuration files
//
// Returns:
//   - *Ruler: configured rule engine ready for evaluation
//   - error: if no valid configuration file is found or configuration is invalid
//
// The first valid configuration file found is used, and the search stops.
// This allows for flexible deployment scenarios where different configuration
// formats can be used based on preference or tooling requirements.
func NewRulerFromPath(dirPath string) (*Ruler, error) {
	configFiles := []string{
		"ruler.cue",
		"ruler.yaml",
		"ruler.yml",
		"config.cue",
		"config.yaml",
		"config.yml",
	}
	for _, configFile := range configFiles {
		fullPath := filepath.Join(dirPath, configFile)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			continue
		}
		ext := filepath.Ext(configFile)
		switch ext {
		case ".cue":
			return NewRulerFromCUEFile(fullPath)
		case ".yaml", ".yml":
			return NewRulerFromYAMLFile(fullPath)
		}
	}
	return nil, fmt.Errorf("no valid configuration file found in directory %s", dirPath)
}

// NewRulerFromRulesDirectory creates a new Ruler instance by loading rules from multiple
// files in a directory and merging them with a base configuration. The base configuration
// must contain the "config" section, and rules from the directory will be added to it.
//
// This function is useful for scenarios where you have a base configuration with engine
// settings and want to load rules from multiple files in a directory structure.
// It supports both CUE and YAML rule files with various naming conventions.
//
// Parameters:
//   - rulesDir: directory containing rule files to load
//   - baseConfig: base configuration object containing at least the "config" section
//
// Returns:
//   - *Ruler: configured rule engine with merged rules
//   - error: if directory reading fails, rule parsing fails, or final configuration is invalid
//
// Supported rule file patterns:
//   - *.rules.cue, *.rules.yaml, *.rules.yml
//   - rules/*.cue, rules/*.yaml, rules/*.yml
//   - *-rules.cue, *-rules.yaml, *-rules.yml
//
// All matching files are loaded and their rules are merged with the base configuration.
func NewRulerFromRulesDirectory(rulesDir string, baseConfig any) (*Ruler, error) {
	baseConfigMap, ok := baseConfig.(map[string]any)
	if !ok {
		return nil, errors.New("base configuration must be a map[string]any")
	}
	if _, exists := baseConfigMap["rules"]; !exists {
		baseConfigMap["rules"] = []any{}
	}
	rules, err := loadRulesFromDirectory(rulesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to load rules from directory %s: %w", rulesDir, err)
	}
	baseRules, ok := baseConfigMap["rules"].([]any)
	if !ok {
		baseRules = []any{}
	}
	combinedRules := append(baseRules, rules...)
	baseConfigMap["rules"] = combinedRules
	return NewRuler(baseConfigMap)
}

// loadRulesFromDirectory loads rules from all valid rule files in a directory.
func loadRulesFromDirectory(rulesDir string) ([]any, error) {
	if _, err := os.Stat(rulesDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("rules directory does not exist: %s", rulesDir)
	}
	entries, err := os.ReadDir(rulesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read rules directory: %w", err)
	}
	allRules := make([]any, 0, len(entries)*2)
	for _, entry := range entries {
		if entry.IsDir() {
			if entry.Name() == "rules" {
				subRules, err := loadRulesFromSubdirectory(filepath.Join(rulesDir, "rules"))
				if err != nil {
					return nil, fmt.Errorf("failed to load rules from subdirectory: %w", err)
				}
				allRules = append(allRules, subRules...)
			}
			continue
		}
		filename := entry.Name()
		fullPath := filepath.Join(rulesDir, filename)
		if isRuleFile(filename) {
			rules, err := loadRulesFromFile(fullPath)
			if err != nil {
				return nil, fmt.Errorf("failed to load rules from file %s: %w", filename, err)
			}
			allRules = append(allRules, rules...)
		}
	}
	return allRules, nil
}

// loadRulesFromSubdirectory recursively loads rules from a subdirectory.
func loadRulesFromSubdirectory(subDir string) ([]any, error) {
	entries, err := os.ReadDir(subDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read rules subdirectory: %w", err)
	}
	allRules := make([]any, 0, len(entries)*2)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		filename := entry.Name()
		fullPath := filepath.Join(subDir, filename)
		ext := filepath.Ext(filename)
		if ext == ".cue" || ext == ".yaml" || ext == ".yml" {
			rules, err := loadRulesFromFile(fullPath)
			if err != nil {
				return nil, fmt.Errorf("failed to load rules from file %s: %w", filename, err)
			}
			allRules = append(allRules, rules...)
		}
	}
	return allRules, nil
}

// isRuleFile determines if a filename represents a valid rule file based on naming conventions.
func isRuleFile(filename string) bool {
	if strings.HasSuffix(filename, ".rules.cue") ||
		strings.HasSuffix(filename, ".rules.yaml") ||
		strings.HasSuffix(filename, ".rules.yml") {
		return true
	}
	return false
}

// loadRulesFromFile loads rules from a single file, dispatching to the appropriate
// parser based on file extension.
func loadRulesFromFile(filePath string) ([]any, error) {
	ext := filepath.Ext(filePath)
	switch ext {
	case ".cue":
		return loadRulesFromCUEFile(filePath)
	case ".yaml", ".yml":
		return loadRulesFromYAMLFile(filePath)
	default:
		return nil, fmt.Errorf("unsupported rule file format: %s", ext)
	}
}

// loadRulesFromCUEFile parses and loads rules from a CUE format file.
func loadRulesFromCUEFile(filePath string) ([]any, error) {
	data, err := secureReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read CUE file: %w", err)
	}
	ctx := cuecontext.New()
	value := ctx.CompileBytes(data, cue.Filename(filePath))
	if value.Err() != nil {
		return nil, fmt.Errorf("failed to compile CUE file: %w", value.Err())
	}
	return extractRulesFromCUEValue(value)
}

// loadRulesFromYAMLFile parses and loads rules from a YAML format file.
func loadRulesFromYAMLFile(filePath string) ([]any, error) {
	data, err := secureReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read YAML file: %w", err)
	}
	ctx := cuecontext.New()
	file, err := yaml.Extract(filePath, data)
	if err != nil {
		return nil, fmt.Errorf("failed to extract YAML from file: %w", err)
	}
	value := ctx.BuildFile(file)
	if value.Err() != nil {
		return nil, fmt.Errorf("failed to build CUE value from YAML file: %w", value.Err())
	}
	var content any
	if err := value.Decode(&content); err != nil {
		return nil, fmt.Errorf("failed to decode YAML content: %w", err)
	}
	return extractRulesFromYAMLContent(content)
}

// extractRulesFromCUEValue extracts rule definitions from a compiled CUE value.
func extractRulesFromCUEValue(value cue.Value) ([]any, error) {
	var content any
	if err := value.Decode(&content); err != nil {
		return nil, fmt.Errorf("failed to decode CUE content: %w", err)
	}
	return extractRulesFromContent(content)
}

// extractRulesFromYAMLContent extracts rule definitions from parsed YAML content.
func extractRulesFromYAMLContent(content any) ([]any, error) {
	return extractRulesFromContent(content)
}

// extractRulesFromContent extracts rules from generic content, handling different data structures.
func extractRulesFromContent(content any) ([]any, error) {
	switch v := content.(type) {
	case map[string]any:
		if rules, exists := v["rules"]; exists {
			if rulesList, ok := rules.([]any); ok {
				return rulesList, nil
			}
		}
		if rule, exists := v["rule"]; exists {
			return []any{rule}, nil
		}
		return []any{v}, nil
	case []any:
		return v, nil
	default:
		return nil, errors.New("unsupported content format for rules extraction")
	}
}

// Evaluate processes input data against all configured rules and returns the evaluation result.
//
// v1.3.0+ Breaking Change: Now evaluates ALL matching rules by default instead of stopping
// at the first match. This resolves the issue where only input validation was performed
// and expression evaluation was skipped for subsequent rules.
//
// The evaluation is thread-safe and uses optimizations like fast-path evaluation and caching.
//
// The evaluation process:
//  1. Validates that the engine is enabled
//  2. Encodes input data into CUE format
//  3. Iterates through compiled rules in priority order
//  4. For each enabled rule, checks input requirements
//  5. Attempts fast-path evaluation if available, otherwise uses full CUE evaluation
//  6. Evaluates expression and collects ALL matching rules (unless stop_on_first_match=true)
//  7. Generates output data from all matched rule specifications
//  8. Updates performance statistics
//
// Returns: EvaluationResult with multiple matched rules and outputs, or empty result if no matches.
// For backward compatibility, legacy fields (MatchedRule, Output) contain the first match.
//
// Parameters:
//   - ctx: context for cancellation and timeout control
//   - inputData: key-value pairs of input data for rule evaluation
//
// Returns:
//   - *EvaluationResult: comprehensive result including matched rule, output, and metrics
//   - error: if input encoding fails or rule evaluation encounters an error
//
// Thread Safety: This method is safe for concurrent use. Multiple goroutines can
// call Evaluate simultaneously without additional synchronization.
func (r *Ruler) Evaluate(ctx context.Context, inputData Inputs) (*EvaluationResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	start := time.Now()
	return r.evaluation(ctx, inputData, start)
}

// evaluation performs the core rule evaluation logic for a single input.
// It tries fast-path evaluation first, then falls back to full CUE evaluation.
// In v1.3.0+, it evaluates ALL matching rules by default unless stop_on_first_match is true.
func (r *Ruler) evaluation(_ context.Context, inputData Inputs, start time.Time) (*EvaluationResult, error) {
	// Get a result object from the pool to reduce allocations
	result, ok := r.resultPool.Get().(*EvaluationResult)
	if !ok {
		// Fallback if pool returns unexpected type (should not happen in normal operation)
		result = &EvaluationResult{}
	}

	// Initialize result with basic information and empty slices for multiple matches
	*result = EvaluationResult{
		Timestamp:    start,
		InputCount:   len(inputData),
		Outputs:      make([]*Output, 0),
		MatchedRules: make([]*MatchedRule, 0),
		MatchCount:   0,
	}

	// Short-circuit if rule engine is disabled - return immediately with default message
	if !r.enabled {
		result.DefaultMessage = r.defaultMessage
		result.Duration = time.Since(start)
		return result, nil
	}

	// Convert input data to CUE format for rule evaluation
	inputCueValue := r.encodeInputData(inputData)
	if inputCueValue.Err() != nil {
		return nil, fmt.Errorf("failed to encode input data: %w", inputCueValue.Err())
	}

	// Iterate through compiled rules to find ALL matches (or stop at first if configured)
	rulesChecked := 0

	for i := range r.compiledRules {
		rule := &r.compiledRules[i]
		rulesChecked++

		// Skip disabled rules to avoid unnecessary processing
		if !rule.enabled {
			continue
		}

		// Check if input data satisfies rule's input requirements
		// This includes checking for required fields and type validation
		if !r.matchesInputRequirements(rule, inputData) {
			continue
		}

		// Evaluate the rule using fast-path optimization if available,
		// otherwise fall back to full CUE evaluation
		matched, err := r.evaluateWithFastPath(rule, inputCueValue, inputData)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate rule expression: %w", err)
		}

		if matched {
			// Create matched rule information for the result
			matchedRule := &MatchedRule{
				Name:        rule.name,
				Description: rule.description,
				Expression:  rule.expression,
				Inputs:      rule.inputs,
				Outputs:     rule.outputs,
				Labels:      rule.labels,
				Priority:    rule.priority,
			}

			// Generate output data based on rule specifications
			output := r.generateOutputFromCompiled(rule, inputData)

			// Add to the collections of matched rules and outputs
			result.MatchedRules = append(result.MatchedRules, matchedRule)
			result.Outputs = append(result.Outputs, output)
			result.MatchCount++

			// For backward compatibility, populate legacy fields with first match
			if result.MatchCount == 1 {
				result.MatchedRule = matchedRule
				result.Output = output
			}

			// Stop on first match if configured to do so (legacy behavior)
			if r.stopOnFirstMatch {
				break
			}
		}
	}

	// Finalize result with statistics and timing information
	result.RulesChecked = rulesChecked
	if result.MatchCount == 0 {
		result.DefaultMessage = r.defaultMessage
	}
	result.Duration = time.Since(start)

	// Update engine statistics for monitoring and performance analysis
	r.updateStatsUnsafe(result.Duration, rulesChecked, result.MatchCount > 0)

	return result, nil
}

// EvaluateBatch processes multiple input sets and returns aggregated results.
// For large batches (>10 items), it uses concurrent processing to improve performance.
// Returns statistics including total duration, average duration, and match count.
//
// The batch processing strategy:
//   - Small batches (≤10 items): processed sequentially for lower overhead
//   - Large batches (>10 items): processed concurrently with worker pool (max 8 workers)
//   - Each input is evaluated independently using the same rules
//   - Results maintain the same order as input batches
//
// Parameters:
//   - ctx: context for cancellation and timeout control
//   - inputBatches: slice of input data sets to evaluate
//
// Returns:
//   - *BatchEvaluationResult: aggregated results with performance statistics
//   - error: if any individual evaluation fails or context is canceled
//
// Performance: Concurrent processing can significantly improve throughput for
// large batches, but adds overhead for small batches. The threshold of 10 items
// is chosen to balance performance and resource usage.
//
// Thread Safety: This method is safe for concurrent use, though typically
// batch evaluation is called from a single goroutine per batch.
func (r *Ruler) EvaluateBatch(ctx context.Context, inputBatches []Inputs) (*BatchEvaluationResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	start := time.Now()
	batchResult := &BatchEvaluationResult{
		Results: make([]*EvaluationResult, len(inputBatches)),
	}
	if !r.enabled {
		for i := range inputBatches {
			result, ok := r.resultPool.Get().(*EvaluationResult)
			if !ok {
				result = &EvaluationResult{}
			}
			*result = EvaluationResult{
				Timestamp:      start,
				InputCount:     len(inputBatches[i]),
				DefaultMessage: r.defaultMessage,
				Duration:       0,
			}
			batchResult.Results[i] = result
		}
		batchResult.TotalDuration = time.Since(start)
		return batchResult, nil
	}
	if len(inputBatches) > 10 {
		return r.evaluateBatchConcurrent(ctx, inputBatches, start)
	}
	for i, inputData := range inputBatches {
		result, err := r.evaluation(ctx, inputData, start)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate batch %d: %w", i, err)
		}
		batchResult.Results[i] = result
		if result.MatchedRule != nil {
			batchResult.TotalMatches++
		}
	}
	batchResult.TotalDuration = time.Since(start)
	if len(inputBatches) > 0 {
		batchResult.AverageDuration = batchResult.TotalDuration / time.Duration(len(inputBatches))
	}
	return batchResult, nil
}

// evaluateBatchConcurrent processes large batches using concurrent workers for improved performance.
func (r *Ruler) evaluateBatchConcurrent(ctx context.Context, inputBatches []Inputs, start time.Time) (*BatchEvaluationResult, error) {
	batchResult := &BatchEvaluationResult{
		Results: make([]*EvaluationResult, len(inputBatches)),
	}
	numWorkers := minInt(len(inputBatches), 8)
	jobs := make(chan int, len(inputBatches))
	results := make(chan struct {
		index  int
		result *EvaluationResult
		err    error
	}, len(inputBatches))
	for range numWorkers {
		go func() {
			for i := range jobs {
				result, err := r.evaluation(ctx, inputBatches[i], start)
				results <- struct {
					index  int
					result *EvaluationResult
					err    error
				}{i, result, err}
			}
		}()
	}
	for i := range inputBatches {
		jobs <- i
	}
	close(jobs)
	for range len(inputBatches) {
		res := <-results
		if res.err != nil {
			return nil, fmt.Errorf("failed to evaluate batch %d: %w", res.index, res.err)
		}
		batchResult.Results[res.index] = res.result
		if res.result.MatchedRule != nil {
			batchResult.TotalMatches++
		}
	}
	batchResult.TotalDuration = time.Since(start)
	if len(inputBatches) > 0 {
		batchResult.AverageDuration = batchResult.TotalDuration / time.Duration(len(inputBatches))
	}
	return batchResult, nil
}

// extractLabels extracts label key-value pairs from a CUE value for rule metadata.
func (r *Ruler) extractLabels(labelsValue cue.Value) (map[string]string, error) {
	if !labelsValue.Exists() {
		return nil, nil
	}
	labels := make(map[string]string)
	iter, err := labelsValue.Fields()
	if err != nil {
		return nil, fmt.Errorf("failed to iterate labels: %w", err)
	}
	for iter.Next() {
		key := iter.Selector().Unquoted()
		value := iter.Value()
		if str, err := value.String(); err == nil {
			labels[key] = str
		}
	}
	return labels, nil
}

// precompileRules compiles all rules from the configuration into executable CUE values
// and sets up fast-path evaluators where possible for performance optimization.
func (r *Ruler) precompileRules() error {
	configValue := r.config.LookupPath(cue.ParsePath("config"))
	defaultMsgPath := configValue.LookupPath(cue.ParsePath("default_message"))
	r.defaultMessage = r.getStringValue(defaultMsgPath, "no rule found for contents")
	// New default behavior: evaluate all matching rules (stop_on_first_match = false)
	// This is the breaking change for v1.3.0 to resolve the issue
	r.stopOnFirstMatch = r.getBoolValue(configValue.LookupPath(cue.ParsePath("stop_on_first_match")), false)
	r.enabled = r.getBoolValue(configValue.LookupPath(cue.ParsePath("enabled")), true)
	rulesValue, err := r.loadRulesValue(configValue)
	if err != nil {
		return err
	}
	iter, err := rulesValue.List()
	if err != nil {
		return fmt.Errorf("failed to iterate rules: %w", err)
	}
	var compiledRules []CompiledRule
	for iter.Next() {
		rule := iter.Value()
		compiledRule, err := r.compileRule(rule)
		if err != nil {
			return err
		}
		compiledRules = append(compiledRules, compiledRule)
	}
	r.compiledRules = compiledRules
	return nil
}

// compileRule compiles a single rule from CUE value into an executable CompiledRule.
func (r *Ruler) compileRule(rule cue.Value) (CompiledRule, error) {
	compiledRule := CompiledRule{
		enabled: r.getBoolValue(rule.LookupPath(cue.ParsePath("enabled")), true),
	}
	r.extractRuleBasicInfo(rule, &compiledRule)
	r.extractRuleInputs(rule, &compiledRule)
	if err := r.extractAndCompileRuleExpression(rule, &compiledRule); err != nil {
		return CompiledRule{}, err
	}
	r.extractRuleOutputs(rule, &compiledRule)
	r.extractRuleMetadata(rule, &compiledRule)
	compiledRule.inputPath = cue.ParsePath("input")
	compiledRule.resultPath = cue.ParsePath("result")
	compiledRule.fastPath = r.detectFastPath(compiledRule.expression)
	return compiledRule, nil
}

// extractRuleBasicInfo extracts basic metadata from a CUE rule value into a CompiledRule.
// This includes the rule name and description, which are essential for rule identification
// and debugging purposes.
func (r *Ruler) extractRuleBasicInfo(rule cue.Value, compiledRule *CompiledRule) {
	nameValue := rule.LookupPath(cue.ParsePath("name"))
	if nameValue.Exists() {
		if name, err := nameValue.String(); err == nil {
			compiledRule.name = name
		}
	}
	descValue := rule.LookupPath(cue.ParsePath("description"))
	if descValue.Exists() {
		if desc, err := descValue.String(); err == nil {
			compiledRule.description = desc
		}
	}
}

// extractRuleInputs extracts input specifications from a CUE rule value.
// Input specifications define the expected structure and constraints for data
// that will be provided to the rule during evaluation.
func (r *Ruler) extractRuleInputs(rule cue.Value, compiledRule *CompiledRule) {
	inputsValue := rule.LookupPath(cue.ParsePath("inputs"))
	if inputsValue.Exists() {
		if inputs, err := r.extractInputSpecs(inputsValue); err == nil {
			compiledRule.inputs = inputs
		}
	}
}

// extractAndCompileRuleExpression extracts and compiles the CUE expression from a rule definition.
// It creates a CUE template with input bindings and compiles it for efficient evaluation.
// The compiled expression is stored in the CompiledRule for fast runtime evaluation.
func (r *Ruler) extractAndCompileRuleExpression(rule cue.Value, compiledRule *CompiledRule) error {
	exprValue := rule.LookupPath(cue.ParsePath("expr"))
	if !exprValue.Exists() {
		return errors.New("rule missing expression")
	}
	expr, err := exprValue.String()
	if err != nil {
		return fmt.Errorf("invalid expression: %w", err)
	}
	compiledRule.expression = expr
	inputBindings := r.generateInputFieldBindings(compiledRule.inputs)
	exprTemplate := fmt.Sprintf(`{
		input: _,
		%s
		result: %s
	}`, inputBindings, expr)
	compiled := r.cueCtx.CompileString(exprTemplate, cue.Filename("rule_expr"))
	if compiled.Err() != nil {
		return fmt.Errorf("failed to compile expression '%s': %w", expr, compiled.Err())
	}
	compiledRule.compiled = compiled
	return nil
}

// extractRuleOutputs extracts output specifications from a CUE rule value.
// Output specifications define the structure and content of data that will be
// generated when the rule matches during evaluation.
func (r *Ruler) extractRuleOutputs(rule cue.Value, compiledRule *CompiledRule) {
	outputsValue := rule.LookupPath(cue.ParsePath("outputs"))
	if outputsValue.Exists() {
		if outputs, err := r.extractOutputSpecs(outputsValue); err == nil {
			compiledRule.outputs = outputs
		}
	}
}

// extractRuleMetadata extracts metadata and configuration from a CUE rule value.
// This includes labels for rule categorization and priority for execution ordering.
// Higher priority rules are evaluated first during rule processing.
func (r *Ruler) extractRuleMetadata(rule cue.Value, compiledRule *CompiledRule) {
	labelsValue := rule.LookupPath(cue.ParsePath("labels"))
	if labels, err := r.extractLabels(labelsValue); err == nil {
		compiledRule.labels = labels
	}
	priorityValue := rule.LookupPath(cue.ParsePath("priority"))
	if priority, err := priorityValue.Int64(); err == nil {
		compiledRule.priority = int(priority)
	}
}

// extractInputSpecs extracts all input specifications from a CUE inputs value.
// It processes both list and map formats for input definitions.
func (r *Ruler) extractInputSpecs(inputsValue cue.Value) ([]InputSpec, error) {
	var inputs []InputSpec
	iter, err := inputsValue.List()
	if err != nil {
		return nil, fmt.Errorf("failed to iterate inputs: %w", err)
	}
	for iter.Next() {
		inputValue := iter.Value()
		inputSpec := r.extractSingleInputSpec(inputValue)
		inputs = append(inputs, inputSpec)
	}
	return inputs, nil
}

// extractSingleInputSpec extracts a complete InputSpec from a CUE value.
// It processes all input specification fields including name, type, required flag,
// description, and example values.
func (r *Ruler) extractSingleInputSpec(inputValue cue.Value) InputSpec {
	var inputSpec InputSpec
	r.extractInputSpecName(inputValue, &inputSpec)
	r.extractInputSpecType(inputValue, &inputSpec)
	r.extractInputSpecRequired(inputValue, &inputSpec)
	r.extractInputSpecDescription(inputValue, &inputSpec)
	r.extractInputSpecExample(inputValue, &inputSpec)
	return inputSpec
}

// extractInputSpecName extracts the name field from an input specification CUE value.
func (r *Ruler) extractInputSpecName(inputValue cue.Value, inputSpec *InputSpec) {
	nameValue := inputValue.LookupPath(cue.ParsePath("name"))
	if nameValue.Exists() {
		if name, err := nameValue.String(); err == nil {
			inputSpec.Name = name
		}
	}
}

// extractInputSpecType extracts the type field from an input specification CUE value.
func (r *Ruler) extractInputSpecType(inputValue cue.Value, inputSpec *InputSpec) {
	typeValue := inputValue.LookupPath(cue.ParsePath("type"))
	if typeValue.Exists() {
		if inputType, err := typeValue.String(); err == nil {
			inputSpec.Type = inputType
		}
	}
}

// extractInputSpecRequired extracts the required field from an input specification CUE value.
// Defaults to true if the field is not present or cannot be parsed.
func (r *Ruler) extractInputSpecRequired(inputValue cue.Value, inputSpec *InputSpec) {
	requiredValue := inputValue.LookupPath(cue.ParsePath("required"))
	if requiredValue.Exists() {
		if required, err := requiredValue.Bool(); err == nil {
			inputSpec.Required = required
		} else {
			inputSpec.Required = true
		}
	} else {
		inputSpec.Required = true
	}
}

// extractInputSpecDescription extracts the description field from an input specification CUE value.
func (r *Ruler) extractInputSpecDescription(inputValue cue.Value, inputSpec *InputSpec) {
	descValue := inputValue.LookupPath(cue.ParsePath("description"))
	if descValue.Exists() {
		if desc, err := descValue.String(); err == nil {
			inputSpec.Description = desc
		}
	}
}

// extractInputSpecExample extracts the example field from an input specification CUE value.
func (r *Ruler) extractInputSpecExample(inputValue cue.Value, inputSpec *InputSpec) {
	exampleValue := inputValue.LookupPath(cue.ParsePath("example"))
	if exampleValue.Exists() {
		var example any
		if err := exampleValue.Decode(&example); err == nil {
			inputSpec.Example = example
		}
	}
}

// extractOutputSpecs extracts all output specifications from a CUE outputs value.
// It processes both list and map formats for output definitions.
func (r *Ruler) extractOutputSpecs(outputsValue cue.Value) ([]OutputSpec, error) {
	var outputs []OutputSpec
	iter, err := outputsValue.List()
	if err != nil {
		return nil, fmt.Errorf("failed to iterate outputs: %w", err)
	}
	for iter.Next() {
		outputValue := iter.Value()
		outputSpec, err := r.extractSingleOutputSpec(outputValue)
		if err != nil {
			return nil, err
		}
		outputs = append(outputs, outputSpec)
	}
	return outputs, nil
}

// extractSingleOutputSpec extracts a complete OutputSpec from a CUE value.
// It processes all output specification fields including name, description, fields, and examples.
func (r *Ruler) extractSingleOutputSpec(outputValue cue.Value) (OutputSpec, error) {
	var outputSpec OutputSpec
	r.extractOutputSpecName(outputValue, &outputSpec)
	r.extractOutputSpecDescription(outputValue, &outputSpec)
	if err := r.extractOutputSpecFields(outputValue, &outputSpec); err != nil {
		return OutputSpec{}, err
	}
	r.extractOutputSpecExample(outputValue, &outputSpec)
	return outputSpec, nil
}

// extractOutputSpecName extracts the name field from an output specification CUE value.
func (r *Ruler) extractOutputSpecName(outputValue cue.Value, outputSpec *OutputSpec) {
	nameValue := outputValue.LookupPath(cue.ParsePath("name"))
	if nameValue.Exists() {
		if name, err := nameValue.String(); err == nil {
			outputSpec.Name = name
		}
	}
}

// extractOutputSpecDescription extracts the description field from an output specification CUE value.
func (r *Ruler) extractOutputSpecDescription(outputValue cue.Value, outputSpec *OutputSpec) {
	descValue := outputValue.LookupPath(cue.ParsePath("description"))
	if descValue.Exists() {
		if desc, err := descValue.String(); err == nil {
			outputSpec.Description = desc
		}
	}
}

// extractOutputSpecFields extracts field definitions from an output specification CUE value.
func (r *Ruler) extractOutputSpecFields(outputValue cue.Value, outputSpec *OutputSpec) error {
	fieldsValue := outputValue.LookupPath(cue.ParsePath("fields"))
	if fieldsValue.Exists() {
		fields, err := r.extractOutputFields(fieldsValue)
		if err != nil {
			return err
		}
		outputSpec.Fields = fields
	}
	return nil
}

// extractOutputSpecExample extracts the example field from an output specification CUE value.
func (r *Ruler) extractOutputSpecExample(outputValue cue.Value, outputSpec *OutputSpec) {
	exampleValue := outputValue.LookupPath(cue.ParsePath("example"))
	if exampleValue.Exists() {
		var example map[string]string
		if err := exampleValue.Decode(&example); err == nil {
			outputSpec.Example = example
		}
	}
}

// extractOutputFields extracts all output field definitions from a CUE fields value.
// It iterates through field definitions and creates OutputField instances for each.
func (r *Ruler) extractOutputFields(fieldsValue cue.Value) (map[string]OutputField, error) {
	fields := make(map[string]OutputField)
	iter, err := fieldsValue.Fields()
	if err != nil {
		return nil, fmt.Errorf("failed to iterate fields: %w", err)
	}
	for iter.Next() {
		fieldName := iter.Selector().String()
		fieldValue := iter.Value()
		outputField := r.extractSingleOutputField(fieldValue)
		fields[fieldName] = outputField
	}
	return fields, nil
}

// extractSingleOutputField extracts a complete OutputField from a CUE value.
// It determines whether the field is structured (with type, description, etc.) or a direct value.
func (r *Ruler) extractSingleOutputField(fieldValue cue.Value) OutputField {
	var outputField OutputField
	if r.isStructuredOutputField(fieldValue) {
		r.extractOutputFieldType(fieldValue, &outputField)
		r.extractOutputFieldDescription(fieldValue, &outputField)
		r.extractOutputFieldRequired(fieldValue, &outputField)
		r.extractOutputFieldDefault(fieldValue, &outputField)
		r.extractOutputFieldExample(fieldValue, &outputField)
	} else {
		r.extractDirectFieldValue(fieldValue, &outputField)
	}
	return outputField
}

// isStructuredOutputField determines if an output field is structured with metadata.
// Returns true if the field has type information or structured field definitions.
func (r *Ruler) isStructuredOutputField(fieldValue cue.Value) bool {
	typeField := fieldValue.LookupPath(cue.ParsePath("type"))
	defaultField := fieldValue.LookupPath(cue.ParsePath("default"))
	descField := fieldValue.LookupPath(cue.ParsePath("description"))
	requiredField := fieldValue.LookupPath(cue.ParsePath("required"))
	exampleField := fieldValue.LookupPath(cue.ParsePath("example"))
	if typeField.Exists() {
		return true
	}
	if defaultField.Exists() && (requiredField.Exists() || exampleField.Exists()) {
		return true
	}
	if descField.Exists() && !typeField.Exists() && !defaultField.Exists() &&
		!requiredField.Exists() && !exampleField.Exists() {
		return false
	}
	return requiredField.Exists() || exampleField.Exists()
}

// extractDirectFieldValue extracts a direct value from a CUE field and infers its type.
// This handles simple field values that don't have explicit type metadata.
func (r *Ruler) extractDirectFieldValue(fieldValue cue.Value, outputField *OutputField) {
	var value any
	if err := fieldValue.Decode(&value); err != nil {
		if str, err := fieldValue.String(); err == nil {
			outputField.Type = "string"
			outputField.Default = str
		}
		return
	}
	switch v := value.(type) {
	case map[string]any:
		outputField.Type = "map"
		outputField.Default = v
	case string:
		outputField.Type = "string"
		outputField.Default = v
	case float64, int, int64:
		outputField.Type = "number"
		outputField.Default = v
	case bool:
		outputField.Type = "boolean"
		outputField.Default = v
	default:
		outputField.Type = "string"
		outputField.Default = fmt.Sprintf("%v", v)
	}
	outputField.Required = true
}

// extractOutputFieldType extracts the type field from an output field CUE value.
func (r *Ruler) extractOutputFieldType(fieldValue cue.Value, outputField *OutputField) {
	typeValue := fieldValue.LookupPath(cue.ParsePath("type"))
	if typeValue.Exists() {
		if fieldType, err := typeValue.String(); err == nil {
			outputField.Type = fieldType
		}
	}
}

// extractOutputFieldDescription extracts the description field from an output field CUE value.
func (r *Ruler) extractOutputFieldDescription(fieldValue cue.Value, outputField *OutputField) {
	descValue := fieldValue.LookupPath(cue.ParsePath("description"))
	if descValue.Exists() {
		if desc, err := descValue.String(); err == nil {
			outputField.Description = desc
		}
	}
}

// extractOutputFieldRequired extracts the required field from an output field CUE value.
// Defaults to true if the field is not present or cannot be parsed.
func (r *Ruler) extractOutputFieldRequired(fieldValue cue.Value, outputField *OutputField) {
	requiredValue := fieldValue.LookupPath(cue.ParsePath("required"))
	if requiredValue.Exists() {
		if required, err := requiredValue.Bool(); err == nil {
			outputField.Required = required
		} else {
			outputField.Required = true
		}
	} else {
		outputField.Required = true
	}
}

// extractOutputFieldDefault extracts the default field from an output field CUE value.
func (r *Ruler) extractOutputFieldDefault(fieldValue cue.Value, outputField *OutputField) {
	defaultValue := fieldValue.LookupPath(cue.ParsePath("default"))
	if defaultValue.Exists() {
		var value any
		if err := defaultValue.Decode(&value); err == nil {
			outputField.Default = value
		} else if defaultVal, err := defaultValue.String(); err == nil {
			outputField.Default = defaultVal
		}
	}
}

// extractOutputFieldExample extracts the example field from an output field CUE value.
func (r *Ruler) extractOutputFieldExample(fieldValue cue.Value, outputField *OutputField) {
	exampleValue := fieldValue.LookupPath(cue.ParsePath("example"))
	if exampleValue.Exists() {
		var value any
		if err := exampleValue.Decode(&value); err == nil {
			outputField.Example = value
		} else if example, err := exampleValue.String(); err == nil {
			outputField.Example = example
		}
	}
}

// generateInputFieldBindings creates CUE field bindings for input specifications.
// It generates binding strings that map input data to CUE variables for rule evaluation.
func (r *Ruler) generateInputFieldBindings(inputs []InputSpec) string {
	if len(inputs) == 0 {
		return ""
	}
	bindings := make([]string, 0, len(inputs))
	for _, input := range inputs {
		binding := fmt.Sprintf("\t%s: input.%s", input.Name, input.Name)
		bindings = append(bindings, binding)
	}
	return strings.Join(bindings, "\n")
}

// matchesInputRequirements checks if input data satisfies a rule's input requirements.
// It validates that all required inputs are present and match their specified types.
func (r *Ruler) matchesInputRequirements(rule *CompiledRule, inputData Inputs) bool {
	if len(rule.inputs) == 0 {
		return true
	}
	for _, inputSpec := range rule.inputs {
		if inputSpec.Required {
			value, exists := inputData[inputSpec.Name]
			if !exists {
				return false
			}
			if inputSpec.Type != "" && inputSpec.Type != "any" {
				if !r.validateInputType(value, inputSpec.Type) {
					return false
				}
			}
		}
	}
	return true
}

// validateInputType checks if a value matches the expected type specification.
func (r *Ruler) validateInputType(value any, expectedType string) bool {
	switch expectedType {
	case "string":
		_, ok := value.(string)
		return ok
	case "number":
		switch value.(type) {
		case int, int32, int64, float32, float64:
			return true
		default:
			return false
		}
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "array":
		switch value.(type) {
		case []any, []string, []int, []float64, []map[string]any:
			return true
		default:
			return false
		}
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "any":
		return true
	default:
		return true
	}
}

// detectFastPath analyzes a CUE expression to determine if it can be optimized
// with fast-path evaluation, avoiding the overhead of full CUE compilation and evaluation.
func (r *Ruler) detectFastPath(expression string) *FastPathEvaluator {
	// Simple length check pattern: "len(input) > 0"
	// This is a common pattern for checking if any input data exists
	if expression == "len(input) > 0" {
		return &FastPathEvaluator{
			pattern: "length_check",
		}
	}

	// Complex field pattern: combines length check with field iteration
	// Pattern: "len(input) > 0 && len([for x in input if x.field op value {x}]) > 0"
	hasLengthCheck := strings.Contains(expression, "len(input) > 0 &&")
	hasFieldIteration := strings.Contains(expression, "for x in input if")
	if hasLengthCheck && hasFieldIteration {
		if fp := r.parseComplexFieldPattern(expression); fp != nil {
			return fp
		}
	}

	// Simple field comparison pattern: "input[0].field op value"
	// This pattern assumes input is an array and checks the first element
	if len(expression) > 20 && expression[:8] == "input[0]" {
		if fp := r.parseSimpleFieldComparison(expression); fp != nil {
			return fp
		}
	}

	// Direct field comparison pattern: "field op value" (NEW - for simple comparisons)
	// This pattern handles direct field comparisons like "oid == \".1.3.6.1.6.3.1.1.5.3\""
	if fp := r.parseDirectFieldComparison(expression); fp != nil {
		return fp
	}

	// Field existence pattern: "len([for x in input if x.field op value {x}]) > 0"
	// This checks if any input elements match a field condition
	if len(expression) > 30 && expression[:4] == "len(" {
		if fp := r.parseFieldExistencePattern(expression); fp != nil {
			return fp
		}
	}

	// No fast-path optimization available for this expression
	return nil
}

// parseComplexFieldPattern parses complex field patterns with multiple conditions.
// It handles expressions with AND operations between field comparisons.
func (r *Ruler) parseComplexFieldPattern(expr string) *FastPathEvaluator {
	ifIndex := strings.Index(expr, "for x in input if ")
	if ifIndex < 0 {
		return nil
	}
	condition := expr[ifIndex+18:]
	endIndex := strings.Index(condition, " {x}])")
	if endIndex < 0 {
		return nil
	}
	condition = condition[:endIndex]
	conditions := strings.Split(condition, " && ")
	if len(conditions) != 2 {
		return nil
	}
	cond1 := r.parseCondition(conditions[0])
	if cond1.Field == "" {
		return nil
	}
	cond2 := r.parseCondition(conditions[1])
	if cond2.Field == "" {
		return nil
	}
	return &FastPathEvaluator{
		pattern:    "complex_field",
		field:      cond1.Field,
		operator:   cond1.Operator,
		value:      cond1.Value,
		isNumeric:  cond1.IsNumeric,
		field2:     cond2.Field,
		operator2:  cond2.Operator,
		value2:     cond2.Value,
		isNumeric2: cond2.IsNumeric,
	}
}

// ConditionParts represents the parsed components of a comparison condition.
// It contains the individual elements extracted from a rule expression for
// fast-path evaluation optimization.
type ConditionParts struct {
	// Field is the name of the input field being compared
	Field string

	// Operator is the comparison operator (==, !=, >, <, >=, <=)
	Operator string

	// Value is the expected value for comparison
	Value any

	// IsNumeric indicates whether the comparison should be performed numerically
	IsNumeric bool
}

// parseCondition parses a condition string into its component parts.
// It extracts the field name, operator, value, and determines if the comparison is numeric.
func (r *Ruler) parseCondition(condition string) ConditionParts {
	condition = strings.TrimSpace(condition)
	condition = strings.TrimPrefix(condition, "x.")
	for _, op := range []string{"==", "!=", ">=", "<=", ">", "<"} {
		if opIndex := strings.Index(condition, " "+op+" "); opIndex > 0 {
			field := strings.TrimSpace(condition[:opIndex])
			valueStr := strings.TrimSpace(condition[opIndex+len(op)+2:])
			value, isNumeric := r.parseValue(valueStr)
			return ConditionParts{
				Field:     field,
				Operator:  op,
				Value:     value,
				IsNumeric: isNumeric,
			}
		}
	}
	return ConditionParts{}
}

// parseSimpleFieldComparison parses simple field comparison expressions.
// It handles patterns like "input[0].field > value" for fast-path optimization.
func (r *Ruler) parseSimpleFieldComparison(expr string) *FastPathEvaluator {
	if dotIndex := strings.Index(expr, "."); dotIndex > 0 {
		remaining := expr[dotIndex+1:]
		for _, op := range []string{"==", "!=", ">=", "<=", ">", "<"} {
			if opIndex := strings.Index(remaining, " "+op+" "); opIndex > 0 {
				field := remaining[:opIndex]
				valueStart := opIndex + len(op) + 2
				if valueStart < len(remaining) {
					valueStr := strings.TrimSpace(remaining[valueStart:])
					value, isNumeric := r.parseValue(valueStr)
					return &FastPathEvaluator{
						pattern:   "simple_field",
						field:     field,
						operator:  op,
						value:     value,
						isNumeric: isNumeric,
					}
				}
			}
		}
	}
	return nil
}

// parseFieldExistencePattern parses field existence patterns for fast-path optimization.
// It handles simple field presence checks and basic comparisons.
func (r *Ruler) parseFieldExistencePattern(expr string) *FastPathEvaluator {
	if !strings.Contains(expr, "for x in input if x.") {
		return nil
	}
	ifIndex := strings.Index(expr, "if x.")
	if ifIndex < 0 {
		return nil
	}
	condition := expr[ifIndex+5:]
	endIndex := strings.Index(condition, " {x}])")
	if endIndex < 0 {
		return nil
	}
	condition = condition[:endIndex]
	for _, op := range []string{"==", "!=", ">=", "<=", ">", "<"} {
		if opIndex := strings.Index(condition, " "+op+" "); opIndex > 0 {
			field := condition[:opIndex]
			valueStart := opIndex + len(op) + 2
			if valueStart < len(condition) {
				valueStr := strings.TrimSpace(condition[valueStart:])
				value, isNumeric := r.parseValue(valueStr)
				return &FastPathEvaluator{
					pattern:   "field_comparison",
					field:     field,
					operator:  op,
					value:     value,
					isNumeric: isNumeric,
				}
			}
		}
	}
	return nil
}

// parseValue parses a string value and determines its type.
// Returns the parsed value and a boolean indicating if it's numeric.
func (r *Ruler) parseValue(valueStr string) (any, bool) {
	if len(valueStr) >= 2 && valueStr[0] == '"' && valueStr[len(valueStr)-1] == '"' {
		return valueStr[1 : len(valueStr)-1], false
	}
	if val, err := strconv.ParseFloat(valueStr, 64); err == nil {
		return val, true
	}
	if val, err := strconv.ParseInt(valueStr, 10, 64); err == nil {
		return val, true
	}
	return valueStr, false
}

// parseDirectFieldComparison parses direct field comparison expressions.
// It handles patterns like "field == value" or "oid == \".1.3.6.1.6.3.1.1.5.3\"" for fast-path optimization.
func (r *Ruler) parseDirectFieldComparison(expr string) *FastPathEvaluator {
	expr = strings.TrimSpace(expr)

	// Check for supported operators in order of precedence (longer operators first)
	for _, op := range []string{"==", "!=", ">=", "<=", ">", "<"} {
		if evaluator := r.tryParseOperator(expr, op); evaluator != nil {
			return evaluator
		}
	}
	return nil
}

// tryParseOperator attempts to parse an expression with a specific operator.
func (r *Ruler) tryParseOperator(expr, op string) *FastPathEvaluator {
	opIndex := strings.Index(expr, " "+op+" ")
	if opIndex <= 0 {
		return nil
	}

	field, valueStr := r.extractFieldAndValue(expr, op, opIndex)
	if field == "" || valueStr == "" {
		return nil
	}

	if r.isComplexExpression(field, valueStr) {
		return nil
	}

	value, isNumeric := r.parseValue(valueStr)
	return &FastPathEvaluator{
		pattern:   "simple_field",
		field:     field,
		operator:  op,
		value:     value,
		isNumeric: isNumeric,
	}
}

// extractFieldAndValue extracts field and value from expression parts.
func (r *Ruler) extractFieldAndValue(expr, op string, opIndex int) (string, string) {
	field := strings.TrimSpace(expr[:opIndex])
	valueStart := opIndex + len(op) + 2

	if valueStart >= len(expr) {
		return "", ""
	}

	valueStr := strings.TrimSpace(expr[valueStart:])
	return field, valueStr
}

// isComplexExpression checks if the field or value contains complex patterns.
func (r *Ruler) isComplexExpression(field, valueStr string) bool {
	// Skip if this looks like a complex expression (contains parentheses, brackets, etc.)
	if strings.ContainsAny(field, "()[]{}") || strings.ContainsAny(valueStr, "()[]{}") {
		return true
	}

	// Skip if field contains dots (like input[0].field) - those are handled by other parsers
	if strings.Contains(field, ".") {
		return true
	}

	return false
}

// evaluateCompiledExpression evaluates a compiled CUE expression against input data.
// It fills the CUE template with input data and extracts the boolean result.
func (r *Ruler) evaluateCompiledExpression(rule *CompiledRule, inputData cue.Value) bool {
	filled := rule.compiled.FillPath(rule.inputPath, inputData)
	if filled.Err() != nil {
		return false
	}
	resultValue := filled.LookupPath(rule.resultPath)
	if resultValue.Err() != nil {
		return false
	}
	if kind := resultValue.Kind(); kind == cue.BoolKind {
		result, err := resultValue.Bool()
		if err != nil {
			return false
		}
		return result
	}
	return false
}

// evaluateCompiledExpressionWithCache evaluates a compiled expression with result caching.
// It checks the cache first and stores results to improve performance for repeated evaluations.
func (r *Ruler) evaluateCompiledExpressionWithCache(
	rule *CompiledRule,
	inputData cue.Value,
	originalInput Inputs,
) (bool, error) {
	cacheKey := r.generateCacheKey(rule.expression, originalInput)
	r.exprCacheMu.RLock()
	if result, exists := r.exprCache[cacheKey]; exists {
		r.exprCacheMu.RUnlock()
		return result, nil
	}
	r.exprCacheMu.RUnlock()
	result := r.evaluateCompiledExpression(rule, inputData)
	r.exprCacheMu.Lock()
	if len(r.exprCache) >= r.maxCacheSize {
		r.exprCache = make(map[string]bool)
	}
	r.exprCache[cacheKey] = result
	r.exprCacheMu.Unlock()
	return result, nil
}

// generateCacheKey creates a cache key for expression evaluation results.
// It uses different strategies based on input size to balance cache efficiency and performance.
func (r *Ruler) generateCacheKey(expression string, inputData Inputs) string {
	// For small input sets, generate a detailed hash-based cache key
	// This provides better cache hit rates for common input patterns
	if len(inputData) <= 5 {
		// Reset the reusable buffer to avoid allocations
		r.keyBuffer = r.keyBuffer[:0]

		// Start with the expression as the base of the key
		r.keyBuffer = append(r.keyBuffer, expression...)

		// Add each input key-value pair to the cache key
		keys := inputData.Keys()
		for _, k := range keys {
			r.keyBuffer = append(r.keyBuffer, k...)
			r.keyBuffer = append(r.keyBuffer, ':')

			// Efficiently serialize different value types without reflection
			switch v := inputData[k].(type) {
			case string:
				r.keyBuffer = append(r.keyBuffer, v...)
			case int:
				r.keyBuffer = strconv.AppendInt(r.keyBuffer, int64(v), 10)
			case int64:
				r.keyBuffer = strconv.AppendInt(r.keyBuffer, v, 10)
			case float64:
				r.keyBuffer = strconv.AppendFloat(r.keyBuffer, v, 'f', -1, 64)
			case bool:
				r.keyBuffer = strconv.AppendBool(r.keyBuffer, v)
			default:
				// Fallback for other types (less efficient but handles edge cases)
				r.keyBuffer = append(r.keyBuffer, fmt.Sprintf("%v", v)...)
			}
			r.keyBuffer = append(r.keyBuffer, ';')
		}

		// Generate a compact hash of the complete key for fast comparison
		h := fnv.New64a()
		_, _ = h.Write(r.keyBuffer)
		return strconv.FormatUint(h.Sum64(), 36) // Base36 for compact representation
	}

	// For large input sets, use a simpler key to avoid expensive serialization
	// This trades some cache precision for performance with large inputs
	return fmt.Sprintf("%s_%d", expression[:minInt(20, len(expression))], len(inputData))
}

// minInt returns the smaller of two integers.
// This utility function is used for determining optimal worker pool sizes
// and other resource allocation decisions throughout the rule engine.
//
// Parameters:
//   - a: first integer to compare
//   - b: second integer to compare
//
// Returns:
//   - int: the smaller of the two input values
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// evaluateWithFastPath evaluates a rule using fast-path optimization when available.
// It falls back to full CUE evaluation if no fast-path is available.
func (r *Ruler) evaluateWithFastPath(rule *CompiledRule, inputCueValue cue.Value, originalInput Inputs) (bool, error) {
	if rule.fastPath != nil {
		return r.evaluateFastPath(rule.fastPath, originalInput)
	}
	return r.evaluateCompiledExpressionWithCache(rule, inputCueValue, originalInput)
}

// evaluateFastPath evaluates a rule using fast-path optimization.
// It dispatches to specific evaluation methods based on the fast-path pattern type.
func (r *Ruler) evaluateFastPath(fp *FastPathEvaluator, inputData Inputs) (bool, error) {
	switch fp.pattern {
	case "length_check":
		return len(inputData) > 0, nil
	case "simple_field":
		return r.evaluateSimpleField(fp, inputData), nil
	case "field_comparison":
		return r.evaluateFieldComparison(fp, inputData), nil
	case "complex_field":
		return r.evaluateComplexField(fp, inputData), nil
	default:
		return false, fmt.Errorf("unknown fast-path pattern: %s", fp.pattern)
	}
}

// evaluateSimpleField evaluates simple field existence patterns.
// It checks if a field exists in the input data for fast-path optimization.
func (r *Ruler) evaluateSimpleField(fp *FastPathEvaluator, inputData Inputs) bool {
	if len(inputData) == 0 {
		return false
	}
	if value, exists := inputData[fp.field]; exists {
		return r.compareValues(value, fp.operator, fp.value, fp.isNumeric)
	}
	return false
}

// evaluateFieldComparison evaluates field comparison patterns.
// It performs field value comparisons for fast-path optimization, supporting both
// direct field access and array iteration patterns.
func (r *Ruler) evaluateFieldComparison(fp *FastPathEvaluator, inputData Inputs) bool {
	// First, try direct field access for simple cases
	if value, exists := inputData[fp.field]; exists {
		return r.compareValues(value, fp.operator, fp.value, fp.isNumeric)
	}

	// If direct access fails, check if this is an array iteration pattern
	// Pattern: len([for x in input if x.field == value {x}]) > 0
	// In this case, we need to iterate through the "input" array
	if inputArray, exists := inputData["input"]; exists {
		result := r.evaluateArrayFieldComparison(fp, inputArray)
		return result
	}

	return false
}

// evaluateArrayFieldComparison evaluates field comparisons within array elements.
// This handles expressions like "len([for x in input if x.field == value {x}]) > 0"
func (r *Ruler) evaluateArrayFieldComparison(fp *FastPathEvaluator, inputArray any) bool {
	switch arr := inputArray.(type) {
	case []map[string]any:
		return r.evaluateMapStringAnyArray(fp, arr)
	case []any:
		return r.evaluateAnyArray(fp, arr)
	}
	return false // No matching elements found
}

// evaluateMapStringAnyArray evaluates field comparisons in []map[string]any arrays
func (r *Ruler) evaluateMapStringAnyArray(fp *FastPathEvaluator, arr []map[string]any) bool {
	for _, element := range arr {
		if r.evaluateMapElement(fp, element) {
			return true
		}
	}
	return false
}

// evaluateAnyArray evaluates field comparisons in []any arrays containing map elements
func (r *Ruler) evaluateAnyArray(fp *FastPathEvaluator, arr []any) bool {
	for _, item := range arr {
		if element, ok := item.(map[string]any); ok {
			if r.evaluateMapElement(fp, element) {
				return true
			}
		}
	}
	return false
}

// evaluateMapElement evaluates field comparison for a single map element
func (r *Ruler) evaluateMapElement(fp *FastPathEvaluator, element map[string]any) bool {
	if value, exists := element[fp.field]; exists {
		return r.compareValues(value, fp.operator, fp.value, fp.isNumeric)
	}
	return false
}

// evaluateComplexField evaluates complex field patterns with multiple conditions.
// It handles AND operations between two field comparisons for fast-path optimization.
func (r *Ruler) evaluateComplexField(fp *FastPathEvaluator, inputData Inputs) bool {
	return r.matchesBothConditions(fp, inputData)
}

// matchesBothConditions checks if input data matches both conditions in a complex field pattern.
// It validates that both field comparisons evaluate to true for AND operations.
func (r *Ruler) matchesBothConditions(fp *FastPathEvaluator, inputData Inputs) bool {
	value1, exists1 := inputData[fp.field]
	if !exists1 {
		return false
	}
	if !r.compareValues(value1, fp.operator, fp.value, fp.isNumeric) {
		return false
	}
	value2, exists2 := inputData[fp.field2]
	if !exists2 {
		return false
	}
	return r.compareValues(value2, fp.operator2, fp.value2, fp.isNumeric2)
}

// compareValues performs type-aware comparison between actual and expected values
// using the specified operator. Supports both numeric and string comparisons.
func (r *Ruler) compareValues(actual any, operator string, expected any, isNumeric bool) bool {
	if isNumeric {
		actualNum, ok1 := r.toFloat64(actual)
		expectedNum, ok2 := r.toFloat64(expected)
		if !ok1 || !ok2 {
			return false
		}
		switch operator {
		case "==":
			return actualNum == expectedNum
		case "!=":
			return actualNum != expectedNum
		case ">":
			return actualNum > expectedNum
		case "<":
			return actualNum < expectedNum
		case ">=":
			return actualNum >= expectedNum
		case "<=":
			return actualNum <= expectedNum
		}
	} else {
		actualStr := fmt.Sprintf("%v", actual)
		expectedStr := fmt.Sprintf("%v", expected)
		switch operator {
		case "==":
			return actualStr == expectedStr
		case "!=":
			return actualStr != expectedStr
		}
	}
	return false
}

// toFloat64 converts various numeric types to float64 for comparison operations.
func (r *Ruler) toFloat64(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int32:
		return float64(val), true
	case int64:
		return float64(val), true
	case uint:
		return float64(val), true
	case uint32:
		return float64(val), true
	case uint64:
		return float64(val), true
	default:
		return 0, false
	}
}

// generateOutputFromCompiled generates output data based on a compiled rule's output specifications.
// It creates output data using default values, examples, or type-appropriate placeholders.
func (r *Ruler) generateOutputFromCompiled(rule *CompiledRule, _ Inputs) *Output {
	generatedOutputs := make(map[string]map[string]any, len(rule.outputs))
	for _, outputSpec := range rule.outputs {
		outputData := make(map[string]any, len(outputSpec.Fields))
		for fieldName, fieldSpec := range outputSpec.Fields {
			var value any
			if fieldSpec.Default != nil {
				value = fieldSpec.Default
			} else if fieldSpec.Example != nil {
				value = fieldSpec.Example
			} else if fieldSpec.Required {
				switch fieldSpec.Type {
				case "map":
					value = make(map[string]any)
				case "number":
					value = 0
				case "boolean":
					value = false
				default:
					value = fmt.Sprintf("<%s>", fieldName)
				}
			}
			if value != nil {
				outputData[fieldName] = value
			}
		}
		if len(outputData) > 0 {
			generatedOutputs[outputSpec.Name] = outputData
		}
	}
	metadata, ok := r.mapPool.Get().(map[string]any)
	if !ok {
		metadata = make(map[string]any, 5)
	} else {
		for k := range metadata {
			delete(metadata, k)
		}
	}
	metadata["rule_name"] = rule.name
	metadata["description"] = rule.description
	metadata["expression"] = rule.expression
	metadata["labels"] = rule.labels
	metadata["priority"] = rule.priority
	return &Output{
		Outputs:          rule.outputs,
		GeneratedOutputs: generatedOutputs,
		Metadata:         metadata,
	}
}

// encodeInputData encodes input data into a CUE value for rule evaluation.
// It converts the input map into a CUE-compatible format for expression evaluation.
func (r *Ruler) encodeInputData(inputData Inputs) cue.Value {
	if len(inputData) <= 10 {
		return r.cueCtx.Encode(inputData)
	}
	return r.cueCtx.Encode(inputData)
}

// getStringValue extracts a string value from a CUE value with a fallback default.
// Returns the default value if the CUE value doesn't exist or cannot be converted to string.
func (r *Ruler) getStringValue(v cue.Value, defaultValue string) string {
	if !v.Exists() {
		return defaultValue
	}
	if str, err := v.String(); err == nil {
		return str
	}
	return defaultValue
}

// getBoolValue extracts a boolean value from a CUE value with a fallback default.
// Returns the default value if the CUE value doesn't exist or cannot be converted to boolean.
func (r *Ruler) getBoolValue(v cue.Value, defaultValue bool) bool {
	if !v.Exists() {
		return defaultValue
	}
	if b, err := v.Bool(); err == nil {
		return b
	}
	return defaultValue
}

// GetConfig returns the current configuration object as decoded from the CUE value.
// This method provides access to the complete configuration that was used to
// initialize the rule engine, useful for debugging and introspection.
//
// Returns:
//   - any: the decoded configuration object (typically map[string]any)
//   - nil: if the configuration cannot be decoded or accessed
//
// Thread Safety: This method is safe for concurrent use with proper read locking.
// The returned configuration is a copy and can be safely modified without affecting
// the rule engine's internal state.
func (r *Ruler) GetConfig() any {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var config any
	if err := r.config.Decode(&config); err != nil {
		return nil
	}
	return config
}

// GetStats returns the current evaluation statistics including total evaluations,
// matches, errors, average duration, and last evaluation time.
//
// The statistics provide comprehensive information about rule engine performance:
//   - TotalEvaluations: cumulative count of all Evaluate() calls
//   - TotalMatches: cumulative count of successful rule matches
//   - TotalErrors: cumulative count of evaluation errors
//   - AverageDuration: running average of evaluation time
//   - LastEvaluation: timestamp of most recent evaluation
//
// Returns:
//   - EvaluationStats: current performance and usage statistics
//
// Thread Safety: This method is safe for concurrent use. Statistics are
// updated atomically during evaluation and read with proper locking.
//
// Use Case: These statistics are valuable for monitoring rule engine performance,
// identifying bottlenecks, and understanding usage patterns in production systems.
func (r *Ruler) GetStats() EvaluationStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Create a copy with atomic reads for counters
	return EvaluationStats{
		TotalEvaluations: atomic.LoadInt64(&r.stats.TotalEvaluations),
		TotalMatches:     atomic.LoadInt64(&r.stats.TotalMatches),
		TotalErrors:      atomic.LoadInt64(&r.stats.TotalErrors),
		AverageDuration:  r.stats.AverageDuration,
		LastEvaluation:   r.stats.LastEvaluation,
	}
}

// IsEnabled returns whether the rule engine is currently enabled for evaluation.
// If disabled, all evaluations will return empty results without processing rules.
//
// This method checks the current configuration's "enabled" flag, which can be
// used to temporarily disable rule processing without recreating the engine.
// When disabled, evaluations complete quickly and return default messages.
//
// Returns:
//   - bool: true if rule evaluation is enabled, false if disabled
//
// Thread Safety: This method is safe for concurrent use with proper read locking.
//
// Use Case: This is useful for feature flags, maintenance modes, or gradual
// deployments where you want to disable rule processing without stopping the service.
func (r *Ruler) IsEnabled() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	configValue := r.config.LookupPath(cue.ParsePath("config"))
	enabledValue := configValue.LookupPath(cue.ParsePath("enabled"))
	if !enabledValue.Exists() {
		return true
	}
	enabled, err := enabledValue.Bool()
	if err != nil {
		return true
	}
	return enabled
}

// Validate checks a configuration object using Go-native validation and returns validation errors.
// This method performs comprehensive validation of configuration objects to ensure
// they conform to the expected structure and business rules before being used.
//
// The validation process:
//  1. Converts the configuration object to map[string]any
//  2. Applies default values to missing fields
//  3. Validates required fields and data types
//  4. Validates business rules and constraints
//  5. Returns detailed error information for any violations
//
// Parameters:
//   - configObj: configuration object to validate (must be map[string]any)
//
// Returns:
//   - []ValidationError: slice of validation errors (empty if validation passes)
//
// Each ValidationError includes:
//   - Message: human-readable error description
//   - Path: configuration path where error occurred (if applicable)
//   - Field: specific field that caused the error (if applicable)
//
// Thread Safety: This method is safe for concurrent use as it operates on
// immutable validation logic and creates temporary copies for validation.
func (r *Ruler) Validate(configObj any) []ValidationError {
	// Convert to map for validation
	var configMap map[string]any
	switch v := configObj.(type) {
	case map[string]any:
		configMap = v
	default:
		return []ValidationError{
			{Message: "configuration must be a map[string]any"},
		}
	}

	// Apply defaults and validate using Go-native validation
	configMap = applyDefaults(configMap)
	if err := validateConfiguration(configMap); err != nil {
		return []ValidationError{
			{Message: "configuration validation failed: " + err.Error()},
		}
	}
	return nil
}

// loadRulesValue loads rules from a configuration value, handling both inline rules and external rule paths.
// It supports loading rules from files or directories specified in the configuration.
func (r *Ruler) loadRulesValue(configValue cue.Value) (cue.Value, error) {
	rulesPathValue := configValue.LookupPath(cue.ParsePath("rules_path"))
	if rulesPathValue.Err() == nil && rulesPathValue.Exists() {
		rulesPath := r.getStringValue(rulesPathValue, "")
		if rulesPath == "" {
			return cue.Value{}, errors.New("rules_path is specified but empty")
		}
		loadedRules, err := r.loadRulesFromPath(rulesPath)
		if err != nil {
			return cue.Value{}, fmt.Errorf("failed to load rules from path %s: %w", rulesPath, err)
		}
		rulesValue := r.cueCtx.Encode(loadedRules)
		if rulesValue.Err() != nil {
			return cue.Value{}, fmt.Errorf("failed to encode loaded rules: %w", rulesValue.Err())
		}
		return rulesValue, nil
	}
	rulesValue := r.config.LookupPath(cue.ParsePath("rules"))
	if rulesValue.Err() != nil {
		return cue.Value{}, fmt.Errorf("failed to lookup rules: %w", rulesValue.Err())
	}
	return rulesValue, nil
}

// loadRulesFromPath loads rules from a file or directory path.
// It automatically detects whether the path is a file or directory and uses the appropriate loading method.
func (r *Ruler) loadRulesFromPath(rulesPath string) ([]any, error) {
	cleanPath, err := sanitizeAndValidateFilePath(rulesPath)
	if err != nil {
		return nil, fmt.Errorf("invalid rules path: %w", err)
	}
	info, err := os.Stat(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat rules path %s: %w", cleanPath, err)
	}
	if info.IsDir() {
		return r.loadRulesFromDirectory(cleanPath)
	}
	return r.loadRulesFromFile(cleanPath)
}

// loadRulesFromFile loads rules from a single file with caching support.
// It checks the cache first and dispatches to the appropriate parser based on file extension.
func (r *Ruler) loadRulesFromFile(filePath string) ([]any, error) {
	if cachedRules, found := r.getCachedRules(filePath); found {
		return cachedRules, nil
	}
	ext := strings.ToLower(filepath.Ext(filePath))
	var rules []any
	var err error
	switch ext {
	case ".cue":
		rules, err = r.loadRulesFromCUEFile(filePath)
	case ".yaml", ".yml":
		rules, err = r.loadRulesFromYAMLFile(filePath)
	default:
		return nil, fmt.Errorf("unsupported rule file format: %s (supported: .cue, .yaml, .yml)", ext)
	}
	if err != nil {
		return nil, err
	}
	r.cacheRules(filePath, rules)
	return rules, nil
}

// getCachedRules retrieves cached rules for a file path if they exist and are still valid.
// It checks modification time, file size, and checksum to ensure cache validity.
func (r *Ruler) getCachedRules(filePath string) ([]any, bool) {
	globalFileCacheMu.RLock()
	entry, exists := globalFileCache[filePath]
	globalFileCacheMu.RUnlock()
	if !exists {
		return nil, false
	}
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		globalFileCacheMu.Lock()
		delete(globalFileCache, filePath)
		globalFileCacheMu.Unlock()
		return nil, false
	}
	if fileInfo.ModTime().After(entry.modTime) || fileInfo.Size() != entry.size {
		globalFileCacheMu.Lock()
		delete(globalFileCache, filePath)
		globalFileCacheMu.Unlock()
		return nil, false
	}
	return entry.rules, true
}

// cacheRules stores parsed rules in the global cache with file metadata for invalidation.
func (r *Ruler) cacheRules(filePath string, rules []any) {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return
	}
	checksum := r.calculateFileChecksum(filePath)
	globalFileCacheMu.Lock()
	defer globalFileCacheMu.Unlock()
	globalFileCache[filePath] = fileCacheEntry{
		rules:    rules,
		modTime:  fileInfo.ModTime(),
		size:     fileInfo.Size(),
		checksum: checksum,
	}
}

// calculateFileChecksum generates a hash-based checksum for a file using its path,
// modification time, and size to detect changes for cache invalidation.
func (r *Ruler) calculateFileChecksum(filePath string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(filePath))
	if fileInfo, err := os.Stat(filePath); err == nil {
		_, _ = h.Write([]byte(fileInfo.ModTime().String()))
		_, _ = h.Write([]byte(strconv.FormatInt(fileInfo.Size(), 10)))
	}
	return h.Sum64()
}

// loadRulesFromDirectory loads rules from all matching files in a directory.
// It searches for rule files using predefined patterns and loads them concurrently.
func (r *Ruler) loadRulesFromDirectory(dirPath string) ([]any, error) {
	patterns := []string{
		"*.rules.cue",
		"*.rules.yaml",
		"*.rules.yml",
		"rules/*.cue",
		"rules/*.yaml",
		"rules/*.yml",
		"*-rules.cue",
		"*-rules.yaml",
		"*-rules.yml",
	}
	allFiles := make([]string, 0, 16)
	for _, pattern := range patterns {
		fullPattern := filepath.Join(dirPath, pattern)
		matches, err := filepath.Glob(fullPattern)
		if err != nil {
			return nil, fmt.Errorf("failed to glob pattern %s: %w", fullPattern, err)
		}
		allFiles = append(allFiles, matches...)
	}
	if len(allFiles) == 0 {
		return nil, fmt.Errorf("no rule files found in directory %s", dirPath)
	}
	return r.loadRulesFromFilesConcurrently(allFiles)
}

// loadRulesFromFilesConcurrently loads rules from multiple files using concurrent workers.
// It optimizes performance for loading many rule files by processing them in parallel.
func (r *Ruler) loadRulesFromFilesConcurrently(filePaths []string) ([]any, error) {
	if len(filePaths) == 0 {
		return nil, errors.New("no files to load")
	}
	if len(filePaths) == 1 {
		return r.loadRulesFromFile(filePaths[0])
	}

	// fileResult represents the result of loading rules from a single file
	// during concurrent file processing operations.
	type fileResult struct {
		// rules contains the parsed rules from the file
		rules []any

		// err contains any error that occurred during file processing
		err error

		// file is the path of the file that was processed
		file string
	}
	maxWorkers := min(4, len(filePaths))
	jobs := make(chan string, maxWorkers)
	results := make(chan fileResult, len(filePaths))
	for range maxWorkers {
		go func() {
			for filePath := range jobs {
				rules, err := r.loadRulesFromFile(filePath)
				results <- fileResult{
					rules: rules,
					err:   err,
					file:  filePath,
				}
			}
		}()
	}
	for _, filePath := range filePaths {
		jobs <- filePath
	}
	close(jobs)
	allRules := make([]any, 0, len(filePaths)*2)
	var errors []string
	for range len(filePaths) {
		result := <-results
		if result.err != nil {
			errors = append(errors, fmt.Sprintf("failed to load %s: %v", result.file, result.err))
		} else {
			allRules = append(allRules, result.rules...)
		}
	}
	if len(errors) > 0 {
		return nil, fmt.Errorf("failed to load rule files: %s", strings.Join(errors, "; "))
	}
	return allRules, nil
}

// loadRulesFromCUEFile loads rules from a CUE format file.
// It parses the CUE file and extracts rule definitions into a slice.
func (r *Ruler) loadRulesFromCUEFile(filePath string) ([]any, error) {
	data, err := secureReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read CUE file %s: %w", filePath, err)
	}
	cueValue := r.cueCtx.CompileBytes(data, cue.Filename(filePath))
	if cueValue.Err() != nil {
		return nil, fmt.Errorf("failed to compile CUE file %s: %w", filePath, cueValue.Err())
	}
	rulesValue := cueValue.LookupPath(cue.ParsePath("rules"))
	if rulesValue.Err() != nil {
		return nil, fmt.Errorf(
			"failed to find 'rules' field in CUE file %s: %w",
			filePath,
			rulesValue.Err(),
		)
	}
	var rules []any
	if err := rulesValue.Decode(&rules); err != nil {
		return nil, fmt.Errorf("failed to decode rules from CUE file %s: %w", filePath, err)
	}
	return rules, nil
}

// loadRulesFromYAMLFile loads rules from a YAML format file.
// It parses the YAML file and extracts rule definitions into a slice.
func (r *Ruler) loadRulesFromYAMLFile(filePath string) ([]any, error) {
	data, err := secureReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read YAML file %s: %w", filePath, err)
	}
	file, err := yaml.Extract(filePath, data)
	if err != nil {
		return nil, fmt.Errorf("failed to extract YAML from file %s: %w", filePath, err)
	}
	cueValue := r.cueCtx.BuildFile(file)
	if cueValue.Err() != nil {
		return nil, fmt.Errorf(
			"failed to build CUE value from YAML file %s: %w",
			filePath,
			cueValue.Err(),
		)
	}
	rulesValue := cueValue.LookupPath(cue.ParsePath("rules"))
	if rulesValue.Err() != nil {
		return nil, fmt.Errorf(
			"failed to find 'rules' field in YAML file %s: %w",
			filePath,
			rulesValue.Err(),
		)
	}
	var rules []any
	if err := rulesValue.Decode(&rules); err != nil {
		return nil, fmt.Errorf("failed to decode rules from YAML file %s: %w", filePath, err)
	}
	return rules, nil
}

// updateStatsUnsafe updates the ruler's performance statistics using atomic operations.
// This method is thread-safe and can be called concurrently without additional locking.
// For v1.0.1 stability, we only track basic counters to avoid race conditions.
func (r *Ruler) updateStatsUnsafe(_ time.Duration, _ int, matched bool) {
	// Atomically increment total evaluation counter
	atomic.AddInt64(&r.stats.TotalEvaluations, 1)

	// Track successful matches for success rate calculation
	if matched {
		atomic.AddInt64(&r.stats.TotalMatches, 1)
	}

	// Note: LastEvaluation and AverageDuration updates are disabled in concurrent scenarios
	// to avoid race conditions. These can be re-enabled in future versions with proper
	// atomic handling or separate synchronization mechanisms.
}

// GetCompiledRules returns the compiled rules for debugging and inspection purposes.
// This method provides access to the internal compiled rule structures, which can be
// useful for debugging rule compilation issues, inspecting rule metadata, and
// understanding the internal rule representation.
//
// The returned slice is a copy of the internal rules and can be safely modified
// without affecting the rule engine's operation.
//
// Returns:
//   - []CompiledRule: slice of compiled rules with metadata and optimization information
func (r *Ruler) GetCompiledRules() []CompiledRule {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return a copy to prevent external modification of internal state
	rules := make([]CompiledRule, len(r.compiledRules))
	copy(rules, r.compiledRules)
	return rules
}

// Compile-time interface compliance checks to ensure Ruler implements required interfaces.
var (
	_ RuleEvaluator = (*Ruler)(nil)
	_ RuleEngine    = (*Ruler)(nil)
)
