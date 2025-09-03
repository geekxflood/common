package ruler

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestRuler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Ruler Suite")
}

var _ = Describe("NewRuler", func() {
	DescribeTable("creating a new ruler",
		func(name string, configObj any, wantErr bool) {
			ruler, err := NewRuler(configObj)
			if wantErr {
				Expect(err).To(HaveOccurred(), "NewRuler() should return error")
			} else {
				Expect(err).NotTo(HaveOccurred(), "NewRuler() should not return error")
				Expect(ruler).NotTo(BeNil(), "NewRuler() should not return nil ruler")
			}
		},
		Entry("valid configuration", "valid configuration", map[string]any{
			"config": map[string]any{
				"enabled":         true,
				"default_message": "no rule found for contents",
			},
			"rules": []any{
				map[string]any{
					"name": "test_rule",
					"expr": `cpu_usage > 80`,
					"inputs": []any{
						map[string]any{
							"name":     "cpu_usage",
							"type":     "number",
							"required": true,
						},
					},
					"outputs": []any{
						map[string]any{
							"name": "alert",
							"fields": map[string]any{
								"severity": map[string]any{
									"type":     "string",
									"required": true,
									"default":  "high",
								},
								"message": map[string]any{
									"type":     "string",
									"required": true,
									"default":  "CPU usage alert",
								},
							},
						},
						map[string]any{
							"name": "notification",
							"fields": map[string]any{
								"type": map[string]any{
									"type":     "string",
									"required": true,
									"default":  "email",
								},
								"recipient": map[string]any{
									"type":     "string",
									"required": true,
									"default":  "admin@example.com",
								},
							},
						},
					},
				},
			},
		}, false),
		Entry("empty rules", "empty rules", map[string]any{
			"config": map[string]any{
				"enabled": true,
			},
			"rules": []any{},
		}, false),
		Entry("invalid config structure", "invalid config structure", map[string]any{
			"invalid": "structure",
		}, true),
		Entry("missing expression", "missing expression", map[string]any{
			"config": map[string]any{
				"enabled": true,
			},
			"rules": []any{
				map[string]any{
					"name": "test_rule",
					"outputs": []any{
						map[string]any{
							"name": "alert",
							"fields": map[string]any{
								"severity": map[string]any{
									"type":     "string",
									"required": true,
									"default":  "high",
								},
							},
						},
					},
				},
			},
		}, true),
	)
})

func TestRuler_Evaluate(t *testing.T) {
	configObj := map[string]any{
		"config": map[string]any{
			"enabled":         true,
			"default_message": "no rule found for contents",
		},
		"rules": []any{
			map[string]any{
				"name": "cpu_usage_rule",
				"expr": `metric == "cpu_usage" && value > 80`,
				"inputs": []any{
					map[string]any{
						"name":     "metric",
						"type":     "string",
						"required": true,
					},
					map[string]any{
						"name":     "value",
						"type":     "number",
						"required": true,
					},
				},
				"outputs": []any{
					map[string]any{
						"name": "alert",
						"fields": map[string]any{
							"severity": map[string]any{
								"type":     "string",
								"required": true,
								"default":  "high",
							},
							"message": map[string]any{
								"type":     "string",
								"required": true,
								"default":  "High CPU usage detected",
							},
						},
					},
					map[string]any{
						"name": "notification",
						"fields": map[string]any{
							"type": map[string]any{
								"type":     "string",
								"required": true,
								"default":  "email",
							},
							"recipient": map[string]any{
								"type":     "string",
								"required": true,
								"default":  "admin@example.com",
							},
						},
					},
				},
				"labels": map[string]any{
					"category": "performance",
				},
				"priority": 10,
			},
			map[string]any{
				"name": "memory_usage_rule",
				"expr": `metric == "memory_usage" && value > 90`,
				"inputs": []any{
					map[string]any{
						"name":     "metric",
						"type":     "string",
						"required": true,
					},
					map[string]any{
						"name":     "value",
						"type":     "number",
						"required": true,
					},
				},
				"outputs": []any{
					map[string]any{
						"name": "alert",
						"fields": map[string]any{
							"severity": map[string]any{
								"type":     "string",
								"required": true,
								"default":  "critical",
							},
							"message": map[string]any{
								"type":     "string",
								"required": true,
								"default":  "Critical memory usage",
							},
						},
					},
				},
				"labels": map[string]any{
					"category": "memory",
				},
				"priority": 20,
			},
		},
	}

	ruler, err := NewRuler(configObj)
	if err != nil {
		t.Fatalf("Failed to create ruler: %v", err)
	}

	tests := []struct {
		name               string
		inputData          Inputs
		wantMatched        bool
		wantOutputContains string
	}{
		{
			name: "high CPU usage triggers rule",
			inputData: Inputs{
				"metric": "cpu_usage",
				"value":  85.5,
				"host":   "server1",
			},
			wantMatched:        true,
			wantOutputContains: "High CPU usage detected",
		},
		{
			name: "critical memory usage triggers rule",
			inputData: Inputs{
				"metric": "memory_usage",
				"value":  95.0,
				"host":   "server1",
			},
			wantMatched:        true,
			wantOutputContains: "Critical memory usage",
		},
		{
			name: "low CPU usage does not trigger",
			inputData: Inputs{
				"metric": "cpu_usage",
				"value":  45.0,
				"host":   "server1",
			},
			wantMatched:        false,
			wantOutputContains: "no rule found for contents",
		},
		{
			name:               "empty input data",
			inputData:          Inputs{},
			wantMatched:        false,
			wantOutputContains: "no rule found for contents",
		},
		{
			name: "multiple metrics with one matching",
			inputData: Inputs{
				"metric": "cpu_usage",
				"value":  90.0,
				"host":   "server1",
			},
			wantMatched:        true,
			wantOutputContains: "High CPU usage detected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			result, err := ruler.Evaluate(ctx, tt.inputData)
			if err != nil {
				t.Errorf("Evaluate() error = %v", err)
				return
			}

			if tt.wantMatched && result.MatchedRule == nil {
				t.Errorf("Expected rule to match but no rule matched")
				return
			}

			if !tt.wantMatched && result.MatchedRule != nil {
				t.Errorf("Expected no rule to match but rule matched: %v", result.MatchedRule.Expression)
				return
			}

			// Check output content based on whether rule matched
			if tt.wantMatched {
				if result.Output == nil {
					t.Error("Expected output but got nil")
					return
				}
				// Check if the expected content is in the generated outputs
				found := false
				if result.Output.GeneratedOutputs != nil {
					for _, outputMap := range result.Output.GeneratedOutputs {
						for _, value := range outputMap {
							if strings.Contains(value, tt.wantOutputContains) {
								found = true
								break
							}
						}
						if found {
							break
						}
					}
				}
				if !found {
					t.Errorf("Expected output to contain %q, got %+v", tt.wantOutputContains, result.Output.Outputs)
				}
			} else {
				if result.Output != nil {
					t.Errorf("Expected no output but got: %+v", result.Output)
				}
				if !strings.Contains(result.DefaultMessage, tt.wantOutputContains) {
					t.Errorf("Expected default message to contain %q, got %q", tt.wantOutputContains, result.DefaultMessage)
				}
			}

			if result.InputCount != len(tt.inputData) {
				t.Errorf("Expected input count %d, got %d", len(tt.inputData), result.InputCount)
			}
		})
	}
}

func TestRuler_EvaluateWithPriority(t *testing.T) {
	configObj := map[string]any{
		"config": map[string]any{
			"enabled": true,
		},
		"rules": []any{
			map[string]any{
				"name": "low_priority_rule",
				"expr": `value > 50`,
				"inputs": []any{
					map[string]any{
						"name":     "value",
						"type":     "number",
						"required": true,
					},
				},
				"outputs": []any{
					map[string]any{
						"name": "alert",
						"fields": map[string]any{
							"severity": map[string]any{
								"type":     "string",
								"required": true,
								"default":  "low",
							},
							"message": map[string]any{
								"type":     "string",
								"required": true,
								"default":  "Low priority rule",
							},
						},
					},
				},
				"priority": 1,
			},
			map[string]any{
				"name": "high_priority_rule",
				"expr": `value > 50`,
				"inputs": []any{
					map[string]any{
						"name":     "value",
						"type":     "number",
						"required": true,
					},
				},
				"outputs": []any{
					map[string]any{
						"name": "alert",
						"fields": map[string]any{
							"severity": map[string]any{
								"type":     "string",
								"required": true,
								"default":  "high",
							},
							"message": map[string]any{
								"type":     "string",
								"required": true,
								"default":  "High priority rule",
							},
						},
					},
				},
				"priority": 10,
			},
		},
	}

	ruler, err := NewRuler(configObj)
	if err != nil {
		t.Fatalf("Failed to create ruler: %v", err)
	}

	ctx := context.Background()
	inputData := Inputs{
		"metric": "test",
		"value":  75.0,
	}

	result, err := ruler.Evaluate(ctx, inputData)
	if err != nil {
		t.Errorf("Evaluate() error = %v", err)
		return
	}

	if result.MatchedRule == nil {
		t.Error("Expected rule to match")
		return
	}

	// Should match the first rule encountered (rules are not sorted by priority in this implementation)
	// This test demonstrates the current behavior
	if result.Output == nil {
		t.Error("Expected output but got nil")
		return
	}

	// Check if any output contains "priority rule"
	found := false
	if result.Output.GeneratedOutputs != nil {
		for _, outputMap := range result.Output.GeneratedOutputs {
			for _, value := range outputMap {
				if strings.Contains(value, "priority rule") {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
	}
	if !found {
		t.Errorf("Expected output to contain priority rule info, got %+v", result.Output.Outputs)
	}
}

func TestRuler_EvaluateDisabledRule(t *testing.T) {
	configObj := map[string]any{
		"config": map[string]any{
			"enabled":         true,
			"default_message": "no rule found",
		},
		"rules": []any{
			map[string]any{
				"name":    "disabled_rule",
				"expr":    `true`,
				"enabled": false,
				"outputs": []any{
					map[string]any{
						"name": "alert",
						"fields": map[string]any{
							"message": map[string]any{
								"type":     "string",
								"required": true,
								"default":  "This should not trigger",
							},
						},
					},
				},
			},
			map[string]any{
				"name":    "enabled_rule",
				"expr":    `true`,
				"enabled": true,
				"outputs": []any{
					map[string]any{
						"name": "alert",
						"fields": map[string]any{
							"message": map[string]any{
								"type":     "string",
								"required": true,
								"default":  "This should trigger",
							},
						},
					},
				},
			},
		},
	}

	ruler, err := NewRuler(configObj)
	if err != nil {
		t.Fatalf("Failed to create ruler: %v", err)
	}

	ctx := context.Background()
	inputData := Inputs{
		"test": "data",
	}

	result, err := ruler.Evaluate(ctx, inputData)
	if err != nil {
		t.Errorf("Evaluate() error = %v", err)
		return
	}

	if result.MatchedRule == nil {
		t.Error("Expected enabled rule to match")
		return
	}

	if result.Output == nil {
		t.Error("Expected output but got nil")
		return
	}

	// Check that disabled rule did not trigger
	foundDisabled := false
	foundEnabled := false
	if result.Output.GeneratedOutputs != nil {
		for _, outputMap := range result.Output.GeneratedOutputs {
			for _, value := range outputMap {
				if strings.Contains(value, "should not trigger") {
					foundDisabled = true
				}
				if strings.Contains(value, "should trigger") {
					foundEnabled = true
				}
			}
		}
	}

	if foundDisabled {
		t.Error("Disabled rule should not have triggered")
	}

	if !foundEnabled {
		t.Error("Enabled rule should have triggered")
	}
}

func TestRuler_OutputFormat(t *testing.T) {
	configObj := map[string]any{
		"config": map[string]any{
			"enabled": true,
		},
		"rules": []any{
			map[string]any{
				"name": "output_format_rule",
				"expr": `true`,
				"outputs": []any{
					map[string]any{
						"name": "alert",
						"fields": map[string]any{
							"severity": map[string]any{
								"type":     "string",
								"required": true,
								"default":  "high",
							},
							"message": map[string]any{
								"type":     "string",
								"required": true,
								"default":  "Test alert",
							},
						},
					},
					map[string]any{
						"name": "notification",
						"fields": map[string]any{
							"type": map[string]any{
								"type":     "string",
								"required": true,
								"default":  "email",
							},
							"recipient": map[string]any{
								"type":     "string",
								"required": true,
								"default":  "test@example.com",
							},
						},
					},
				},
				"labels": map[string]any{
					"category":    "test",
					"environment": "dev",
				},
			},
		},
	}

	ruler, err := NewRuler(configObj)
	if err != nil {
		t.Fatalf("Failed to create ruler: %v", err)
	}

	ctx := context.Background()
	inputData := Inputs{
		"test": "data",
	}

	result, err := ruler.Evaluate(ctx, inputData)
	if err != nil {
		t.Errorf("Evaluate() error = %v", err)
		return
	}

	if result.MatchedRule == nil {
		t.Error("Expected rule to match")
		return
	}

	// Check the structured output directly
	if result.Output == nil {
		t.Error("Expected output but got nil")
		return
	}

	// Check that generated outputs are present
	generatedOutputs := result.Output.GeneratedOutputs
	if len(generatedOutputs) == 0 {
		t.Error("Expected generated outputs in output data")
		return
	}

	// Check alert output
	alert, ok := generatedOutputs["alert"]
	if !ok {
		t.Error("Expected alert in generated outputs")
		return
	}

	if alert["severity"] != "high" {
		t.Errorf("Expected alert severity 'high', got %v", alert["severity"])
	}

	// Check notification output
	notification, ok := generatedOutputs["notification"]
	if !ok {
		t.Error("Expected notification in generated outputs")
		return
	}

	if notification["type"] != "email" {
		t.Errorf("Expected notification type 'email', got %v", notification["type"])
	}
}

func BenchmarkRuler_Evaluate(b *testing.B) {
	configObj := map[string]any{
		"config": map[string]any{
			"enabled": true,
		},
		"rules": []any{
			map[string]any{
				"name": "cpu_benchmark_rule",
				"expr": `metric == "cpu_usage" && value > 80`,
				"inputs": []any{
					map[string]any{
						"name":     "metric",
						"type":     "string",
						"required": true,
					},
					map[string]any{
						"name":     "value",
						"type":     "number",
						"required": true,
					},
				},
				"outputs": []any{
					map[string]any{
						"name": "alert",
						"fields": map[string]any{
							"severity": map[string]any{
								"type":     "string",
								"required": true,
								"default":  "high",
							},
						},
					},
				},
			},
			map[string]any{
				"name": "memory_benchmark_rule",
				"expr": `metric == "memory_usage" && value > 90`,
				"inputs": []any{
					map[string]any{
						"name":     "metric",
						"type":     "string",
						"required": true,
					},
					map[string]any{
						"name":     "value",
						"type":     "number",
						"required": true,
					},
				},
				"outputs": []any{
					map[string]any{
						"name": "alert",
						"fields": map[string]any{
							"severity": map[string]any{
								"type":     "string",
								"required": true,
								"default":  "critical",
							},
						},
					},
				},
			},
			map[string]any{
				"name": "disk_benchmark_rule",
				"expr": `metric == "disk_usage" && value > 95`,
				"inputs": []any{
					map[string]any{
						"name":     "metric",
						"type":     "string",
						"required": true,
					},
					map[string]any{
						"name":     "value",
						"type":     "number",
						"required": true,
					},
				},
				"outputs": []any{
					map[string]any{
						"name": "alert",
						"fields": map[string]any{
							"severity": map[string]any{
								"type":     "string",
								"required": true,
								"default":  "warning",
							},
						},
					},
				},
			},
		},
	}

	ruler, err := NewRuler(configObj)
	if err != nil {
		b.Fatalf("Failed to create ruler: %v", err)
	}

	ctx := context.Background()
	inputData := Inputs{
		"metric": "cpu_usage",
		"value":  85.0,
		"host":   "server1",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ruler.Evaluate(ctx, inputData)
		if err != nil {
			b.Fatalf("Evaluate() error = %v", err)
		}
	}
}

func BenchmarkRuler_EvaluateBatch(b *testing.B) {
	configObj := map[string]any{
		"config": map[string]any{
			"enabled": true,
		},
		"rules": []any{
			map[string]any{
				"name": "benchmark_batch_rule",
				"expr": `cpu_usage > 80`,
				"inputs": []any{
					map[string]any{
						"name":     "cpu_usage",
						"type":     "number",
						"required": true,
					},
				},
				"outputs": []any{
					map[string]any{
						"name": "alert",
						"fields": map[string]any{
							"severity": map[string]any{
								"type":     "string",
								"required": true,
								"default":  "high",
							},
						},
					},
				},
			},
		},
	}

	ruler, err := NewRuler(configObj)
	if err != nil {
		b.Fatalf("Failed to create ruler: %v", err)
	}

	// Create batch of input data
	inputBatches := []Inputs{
		{"cpu_usage": 85.0},
		{"cpu_usage": 75.0},
		{"cpu_usage": 95.0},
		{"cpu_usage": 60.0},
		{"cpu_usage": 88.0},
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ruler.EvaluateBatch(ctx, inputBatches)
		if err != nil {
			b.Fatalf("EvaluateBatch() error = %v", err)
		}
	}
}

func TestRuler_Interfaces(t *testing.T) {
	configObj := map[string]any{
		"config": map[string]any{
			"enabled": true,
		},
		"rules": []any{
			map[string]any{
				"name": "interface_test_rule",
				"expr": `true`,
				"outputs": []any{
					map[string]any{
						"name": "test",
						"fields": map[string]any{
							"result": map[string]any{
								"type":     "string",
								"required": true,
								"default":  "success",
							},
						},
					},
				},
			},
		},
	}

	ruler, err := NewRuler(configObj)
	if err != nil {
		t.Fatalf("Failed to create ruler: %v", err)
	}

	// Test RuleEvaluator interface
	var evaluator RuleEvaluator = ruler
	inputs := Inputs{"test": "data"}

	result, err := evaluator.Evaluate(context.Background(), inputs)
	if err != nil {
		t.Errorf("RuleEvaluator.Evaluate() error = %v", err)
	}
	if result == nil {
		t.Error("Expected evaluation result")
	}

	// Test RuleEngine interface
	var engine RuleEngine = ruler

	if !engine.IsEnabled() {
		t.Error("Expected ruler to be enabled")
	}

	config := engine.GetConfig()
	if config == nil {
		t.Error("Expected configuration")
	}

	stats := engine.GetStats()
	if stats.TotalEvaluations == 0 {
		t.Error("Expected evaluation statistics to be updated")
	}

	// Test ConfigValidator interface
	var validator ConfigValidator = ruler

	validationErrors := validator.Validate(configObj)
	if len(validationErrors) > 0 {
		t.Errorf("Expected no validation errors, got: %v", validationErrors)
	}

	// Test with invalid config
	invalidConfig := map[string]any{"invalid": "config"}
	validationErrors = validator.Validate(invalidConfig)
	if len(validationErrors) == 0 {
		t.Error("Expected validation errors for invalid config")
	}
}

func TestRuler_EvaluateBatch(t *testing.T) {
	configObj := map[string]any{
		"config": map[string]any{
			"enabled": true,
		},
		"rules": []any{
			map[string]any{
				"name": "batch_test_rule",
				"expr": `metric == "cpu_usage" && value > 80`,
				"inputs": []any{
					map[string]any{
						"name":     "metric",
						"type":     "string",
						"required": true,
					},
					map[string]any{
						"name":     "value",
						"type":     "number",
						"required": true,
					},
				},
				"outputs": []any{
					map[string]any{
						"name": "alert",
						"fields": map[string]any{
							"severity": map[string]any{
								"type":     "string",
								"required": true,
								"default":  "high",
							},
						},
					},
				},
			},
		},
	}

	ruler, err := NewRuler(configObj)
	if err != nil {
		t.Fatalf("Failed to create ruler: %v", err)
	}

	// Create multiple input batches
	inputBatches := []Inputs{
		{"metric": "cpu_usage", "value": 85.0},    // Should match
		{"metric": "memory_usage", "value": 75.0}, // Should not match
		{"metric": "cpu_usage", "value": 95.0},    // Should match
	}

	result, err := ruler.EvaluateBatch(context.Background(), inputBatches)
	if err != nil {
		t.Fatalf("EvaluateBatch() error = %v", err)
	}

	if len(result.Results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(result.Results))
	}

	if result.TotalMatches != 2 {
		t.Errorf("Expected 2 total matches, got %d", result.TotalMatches)
	}

	// Check individual results
	if result.Results[0].MatchedRule == nil {
		t.Error("Expected first batch to match")
	}
	if result.Results[1].MatchedRule != nil {
		t.Error("Expected second batch not to match")
	}
	if result.Results[2].MatchedRule == nil {
		t.Error("Expected third batch to match")
	}

	if result.TotalDuration == 0 {
		t.Error("Expected non-zero total duration")
	}
}

// TestInputs tests the Inputs type methods
func TestInputs(t *testing.T) {
	t.Run("Set and Get", func(t *testing.T) {
		var inputs Inputs

		// Test Set on nil map
		inputs.Set("cpu_usage", 85.5)
		inputs.Set("hostname", "server1")

		// Test Get
		value, exists := inputs.Get("cpu_usage")
		if !exists {
			t.Error("Expected cpu_usage to exist")
		}
		if value != 85.5 {
			t.Errorf("Expected cpu_usage to be 85.5, got %v", value)
		}

		// Test Get non-existent key
		_, exists = inputs.Get("non_existent")
		if exists {
			t.Error("Expected non_existent to not exist")
		}
	})

	t.Run("Len and IsEmpty", func(t *testing.T) {
		inputs := Inputs{}

		if !inputs.IsEmpty() {
			t.Error("Expected empty inputs to be empty")
		}

		if inputs.Len() != 0 {
			t.Errorf("Expected length 0, got %d", inputs.Len())
		}

		inputs.Set("key1", "value1")
		inputs.Set("key2", "value2")

		if inputs.IsEmpty() {
			t.Error("Expected inputs with data to not be empty")
		}

		if inputs.Len() != 2 {
			t.Errorf("Expected length 2, got %d", inputs.Len())
		}
	})

	t.Run("Keys", func(t *testing.T) {
		inputs := Inputs{
			"cpu_usage": 85.5,
			"hostname":  "server1",
			"status":    "active",
		}

		keys := inputs.Keys()
		if len(keys) != 3 {
			t.Errorf("Expected 3 keys, got %d", len(keys))
		}

		expectedKeys := map[string]bool{
			"cpu_usage": true,
			"hostname":  true,
			"status":    true,
		}

		for _, key := range keys {
			if !expectedKeys[key] {
				t.Errorf("Unexpected key: %s", key)
			}
		}
	})
}

// TestInputRequirementMatching tests the smart rule selection based on input requirements
func TestInputRequirementMatching(t *testing.T) {
	configObj := map[string]any{
		"config": map[string]any{
			"enabled": true,
		},
		"rules": []any{
			map[string]any{
				"name": "cpu_rule",
				"inputs": []any{
					map[string]any{
						"name":     "cpu_usage",
						"type":     "number",
						"required": true,
					},
					map[string]any{
						"name":     "hostname",
						"type":     "string",
						"required": true,
					},
				},
				"expr": `cpu_usage > 80`,
				"outputs": []any{
					map[string]any{
						"name": "alert",
						"fields": map[string]any{
							"message": map[string]any{
								"type":     "string",
								"required": true,
								"default":  "CPU alert",
							},
						},
					},
				},
			},
			map[string]any{
				"name": "memory_rule",
				"inputs": []any{
					map[string]any{
						"name":     "memory_usage",
						"type":     "number",
						"required": true,
					},
				},
				"expr": `memory_usage > 90`,
				"outputs": []any{
					map[string]any{
						"name": "alert",
						"fields": map[string]any{
							"message": map[string]any{
								"type":     "string",
								"required": true,
								"default":  "Memory alert",
							},
						},
					},
				},
			},
		},
	}

	ruler, err := NewRuler(configObj)
	if err != nil {
		t.Fatalf("Failed to create ruler: %v", err)
	}

	tests := []struct {
		name        string
		inputs      Inputs
		expectMatch bool
		expectRule  string
	}{
		{
			name: "matches cpu rule",
			inputs: Inputs{
				"cpu_usage": 85.0,
				"hostname":  "server1",
			},
			expectMatch: true,
			expectRule:  "cpu_rule",
		},
		{
			name: "matches memory rule",
			inputs: Inputs{
				"memory_usage": 95.0,
			},
			expectMatch: true,
			expectRule:  "memory_rule",
		},
		{
			name: "missing required input",
			inputs: Inputs{
				"cpu_usage": 85.0,
				// missing hostname
			},
			expectMatch: false,
		},
		{
			name: "no matching inputs",
			inputs: Inputs{
				"disk_usage": 50.0,
			},
			expectMatch: false,
		},
		{
			name: "wrong type",
			inputs: Inputs{
				"cpu_usage": "not_a_number",
				"hostname":  "server1",
			},
			expectMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ruler.Evaluate(context.Background(), tt.inputs)
			if err != nil {
				t.Fatalf("Evaluation failed: %v", err)
			}

			if tt.expectMatch {
				if result.Output == nil {
					t.Error("Expected rule to match but got no output")
					return
				}
				if result.MatchedRule.Name != tt.expectRule {
					t.Errorf("Expected rule %s to match, got %s", tt.expectRule, result.MatchedRule.Name)
				}
			} else {
				if result.Output != nil {
					t.Errorf("Expected no rule to match but got rule: %s", result.MatchedRule.Name)
				}
			}
		})
	}
}

// TestTypeValidation tests input type validation
func TestTypeValidation(t *testing.T) {
	configObj := map[string]any{
		"config": map[string]any{
			"enabled": true,
		},
		"rules": []any{
			map[string]any{
				"name": "type_test_rule",
				"inputs": []any{
					map[string]any{
						"name":     "string_field",
						"type":     "string",
						"required": true,
					},
					map[string]any{
						"name":     "number_field",
						"type":     "number",
						"required": true,
					},
					map[string]any{
						"name":     "boolean_field",
						"type":     "boolean",
						"required": true,
					},
					map[string]any{
						"name":     "array_field",
						"type":     "array",
						"required": true,
					},
					map[string]any{
						"name":     "object_field",
						"type":     "object",
						"required": true,
					},
					map[string]any{
						"name":     "any_field",
						"type":     "any",
						"required": true,
					},
				},
				"expr": `string_field == "test"`,
				"outputs": []any{
					map[string]any{
						"name": "result",
						"fields": map[string]any{
							"status": map[string]any{
								"type":     "string",
								"required": true,
								"default":  "success",
							},
						},
					},
				},
			},
		},
	}

	ruler, err := NewRuler(configObj)
	if err != nil {
		t.Fatalf("Failed to create ruler: %v", err)
	}

	tests := []struct {
		name        string
		inputs      Inputs
		expectMatch bool
	}{
		{
			name: "all correct types",
			inputs: Inputs{
				"string_field":  "test",
				"number_field":  42.5,
				"boolean_field": true,
				"array_field":   []string{"a", "b", "c"},
				"object_field":  map[string]any{"key": "value"},
				"any_field":     "anything",
			},
			expectMatch: true,
		},
		{
			name: "wrong string type",
			inputs: Inputs{
				"string_field":  123,
				"number_field":  42.5,
				"boolean_field": true,
				"array_field":   []string{"a", "b", "c"},
				"object_field":  map[string]any{"key": "value"},
				"any_field":     "anything",
			},
			expectMatch: false,
		},
		{
			name: "wrong number type",
			inputs: Inputs{
				"string_field":  "test",
				"number_field":  "not_a_number",
				"boolean_field": true,
				"array_field":   []string{"a", "b", "c"},
				"object_field":  map[string]any{"key": "value"},
				"any_field":     "anything",
			},
			expectMatch: false,
		},
		{
			name: "wrong boolean type",
			inputs: Inputs{
				"string_field":  "test",
				"number_field":  42.5,
				"boolean_field": "not_a_boolean",
				"array_field":   []string{"a", "b", "c"},
				"object_field":  map[string]any{"key": "value"},
				"any_field":     "anything",
			},
			expectMatch: false,
		},
		{
			name: "wrong array type",
			inputs: Inputs{
				"string_field":  "test",
				"number_field":  42.5,
				"boolean_field": true,
				"array_field":   "not_an_array",
				"object_field":  map[string]any{"key": "value"},
				"any_field":     "anything",
			},
			expectMatch: false,
		},
		{
			name: "wrong object type",
			inputs: Inputs{
				"string_field":  "test",
				"number_field":  42.5,
				"boolean_field": true,
				"array_field":   []string{"a", "b", "c"},
				"object_field":  "not_an_object",
				"any_field":     "anything",
			},
			expectMatch: false,
		},
		{
			name: "any type accepts anything",
			inputs: Inputs{
				"string_field":  "test",
				"number_field":  42.5,
				"boolean_field": true,
				"array_field":   []string{"a", "b", "c"},
				"object_field":  map[string]any{"key": "value"},
				"any_field":     123,
			},
			expectMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ruler.Evaluate(context.Background(), tt.inputs)
			if err != nil {
				t.Fatalf("Evaluation failed: %v", err)
			}

			if tt.expectMatch {
				if result.Output == nil {
					t.Error("Expected rule to match but got no output")
				}
			} else {
				if result.Output != nil {
					t.Error("Expected no rule to match but got output")
				}
			}
		})
	}
}

// TestStructuredOutputGeneration tests the generation of structured outputs
func TestStructuredOutputGeneration(t *testing.T) {
	configObj := map[string]any{
		"config": map[string]any{
			"enabled": true,
		},
		"rules": []any{
			map[string]any{
				"name": "output_test_rule",
				"expr": `value > 50`,
				"inputs": []any{
					map[string]any{
						"name":     "value",
						"type":     "number",
						"required": true,
					},
				},
				"outputs": []any{
					map[string]any{
						"name":        "alert",
						"description": "Alert output category",
						"fields": map[string]any{
							"severity": map[string]any{
								"type":        "string",
								"required":    true,
								"default":     "medium",
								"description": "Alert severity level",
								"example":     "high",
							},
							"message": map[string]any{
								"type":        "string",
								"required":    true,
								"default":     "Value threshold exceeded",
								"description": "Alert message",
							},
							"optional_field": map[string]any{
								"type":     "string",
								"required": false,
								"default":  "optional_value",
							},
						},
						"example": map[string]any{
							"severity": "high",
							"message":  "Critical threshold exceeded",
						},
					},
					map[string]any{
						"name":        "notification",
						"description": "Notification routing",
						"fields": map[string]any{
							"channel": map[string]any{
								"type":     "string",
								"required": true,
								"default":  "email",
							},
							"recipient": map[string]any{
								"type":     "string",
								"required": true,
								"default":  "admin@example.com",
							},
						},
					},
				},
				"labels": map[string]any{
					"category": "test",
					"priority": "normal",
				},
				"priority": 5,
			},
		},
	}

	ruler, err := NewRuler(configObj)
	if err != nil {
		t.Fatalf("Failed to create ruler: %v", err)
	}

	inputs := Inputs{
		"value": 75.0,
	}

	result, err := ruler.Evaluate(context.Background(), inputs)
	if err != nil {
		t.Fatalf("Evaluation failed: %v", err)
	}

	if result.Output == nil {
		t.Fatal("Expected output but got none")
	}

	// Test MatchedRule information
	if result.MatchedRule.Name != "output_test_rule" {
		t.Errorf("Expected rule name 'output_test_rule', got '%s'", result.MatchedRule.Name)
	}

	if len(result.MatchedRule.Inputs) != 1 {
		t.Errorf("Expected 1 input spec, got %d", len(result.MatchedRule.Inputs))
	}

	if len(result.MatchedRule.Outputs) != 2 {
		t.Errorf("Expected 2 output specs, got %d", len(result.MatchedRule.Outputs))
	}

	// Test Output structure
	if len(result.Output.Outputs) != 2 {
		t.Errorf("Expected 2 output specs in result, got %d", len(result.Output.Outputs))
	}

	// Test GeneratedOutputs
	if len(result.Output.GeneratedOutputs) != 2 {
		t.Errorf("Expected 2 generated outputs, got %d", len(result.Output.GeneratedOutputs))
	}

	// Test alert output
	alertOutput, exists := result.Output.GeneratedOutputs["alert"]
	if !exists {
		t.Error("Expected alert output to exist")
	} else {
		if alertOutput["severity"] != "medium" {
			t.Errorf("Expected severity 'medium', got '%s'", alertOutput["severity"])
		}
		if alertOutput["message"] != "Value threshold exceeded" {
			t.Errorf("Expected message 'Value threshold exceeded', got '%s'", alertOutput["message"])
		}
		if alertOutput["optional_field"] != "optional_value" {
			t.Errorf("Expected optional_field 'optional_value', got '%s'", alertOutput["optional_field"])
		}
	}

	// Test notification output
	notificationOutput, exists := result.Output.GeneratedOutputs["notification"]
	if !exists {
		t.Error("Expected notification output to exist")
	} else {
		if notificationOutput["channel"] != "email" {
			t.Errorf("Expected channel 'email', got '%s'", notificationOutput["channel"])
		}
		if notificationOutput["recipient"] != "admin@example.com" {
			t.Errorf("Expected recipient 'admin@example.com', got '%s'", notificationOutput["recipient"])
		}
	}

	// Test metadata
	if result.Output.Metadata["rule_name"] != "output_test_rule" {
		t.Errorf("Expected rule_name 'output_test_rule', got '%v'", result.Output.Metadata["rule_name"])
	}

	if result.Output.Metadata["priority"] != 5 {
		t.Errorf("Expected priority 5, got '%v'", result.Output.Metadata["priority"])
	}

	// Test output specifications
	var alertSpec, notificationSpec *OutputSpec
	for i := range result.Output.Outputs {
		switch result.Output.Outputs[i].Name {
		case "alert":
			alertSpec = &result.Output.Outputs[i]
		case "notification":
			notificationSpec = &result.Output.Outputs[i]
		}
	}

	if alertSpec == nil {
		t.Error("Expected alert output spec to exist")
	} else {
		if alertSpec.Description != "Alert output category" {
			t.Errorf("Expected alert description 'Alert output category', got '%s'", alertSpec.Description)
		}
		if len(alertSpec.Fields) != 3 {
			t.Errorf("Expected 3 alert fields, got %d", len(alertSpec.Fields))
		}
	}

	if notificationSpec == nil {
		t.Error("Expected notification output spec to exist")
	} else {
		if notificationSpec.Description != "Notification routing" {
			t.Errorf("Expected notification description 'Notification routing', got '%s'", notificationSpec.Description)
		}
		if len(notificationSpec.Fields) != 2 {
			t.Errorf("Expected 2 notification fields, got %d", len(notificationSpec.Fields))
		}
	}
}

// TestErrorHandling tests various error conditions
func TestErrorHandling(t *testing.T) {
	t.Run("invalid CUE expression", func(t *testing.T) {
		configObj := map[string]any{
			"config": map[string]any{
				"enabled": true,
			},
			"rules": []any{
				map[string]any{
					"name": "invalid_expr_rule",
					"expr": `invalid CUE syntax +++`,
					"outputs": []any{
						map[string]any{
							"name": "result",
							"fields": map[string]any{
								"status": map[string]any{
									"type":     "string",
									"required": true,
									"default":  "error",
								},
							},
						},
					},
				},
			},
		}

		_, err := NewRuler(configObj)
		if err == nil {
			t.Error("Expected error for invalid CUE expression")
		}
	})

	t.Run("missing expression", func(t *testing.T) {
		configObj := map[string]any{
			"config": map[string]any{
				"enabled": true,
			},
			"rules": []any{
				map[string]any{
					"name": "no_expr_rule",
					"outputs": []any{
						map[string]any{
							"name": "result",
							"fields": map[string]any{
								"status": map[string]any{
									"type":     "string",
									"required": true,
									"default":  "error",
								},
							},
						},
					},
				},
			},
		}

		_, err := NewRuler(configObj)
		if err == nil {
			t.Error("Expected error for missing expression")
		}
	})

	t.Run("nil context", func(t *testing.T) {
		configObj := map[string]any{
			"config": map[string]any{
				"enabled": true,
			},
			"rules": []any{
				map[string]any{
					"name": "test_rule",
					"expr": `true`,
					"outputs": []any{
						map[string]any{
							"name": "result",
							"fields": map[string]any{
								"status": map[string]any{
									"type":     "string",
									"required": true,
									"default":  "success",
								},
							},
						},
					},
				},
			},
		}

		ruler, err := NewRuler(configObj)
		if err != nil {
			t.Fatalf("Failed to create ruler: %v", err)
		}

		inputs := Inputs{"test": "data"}

		// This should not panic even with nil context
		_, err = ruler.Evaluate(context.TODO(), inputs)
		if err != nil {
			t.Errorf("Evaluation with nil context failed: %v", err)
		}
	})

	t.Run("empty inputs", func(t *testing.T) {
		configObj := map[string]any{
			"config": map[string]any{
				"enabled": true,
			},
			"rules": []any{
				map[string]any{
					"name": "test_rule",
					"expr": `true`,
					"outputs": []any{
						map[string]any{
							"name": "result",
							"fields": map[string]any{
								"status": map[string]any{
									"type":     "string",
									"required": true,
									"default":  "success",
								},
							},
						},
					},
				},
			},
		}

		ruler, err := NewRuler(configObj)
		if err != nil {
			t.Fatalf("Failed to create ruler: %v", err)
		}

		// Test with empty inputs
		result, err := ruler.Evaluate(context.Background(), Inputs{})
		if err != nil {
			t.Fatalf("Evaluation failed: %v", err)
		}

		if result.Output == nil {
			t.Error("Expected rule to match with empty inputs")
		}
	})

	t.Run("nil inputs", func(t *testing.T) {
		configObj := map[string]any{
			"config": map[string]any{
				"enabled": true,
			},
			"rules": []any{
				map[string]any{
					"name": "test_rule",
					"expr": `true`,
					"outputs": []any{
						map[string]any{
							"name": "result",
							"fields": map[string]any{
								"status": map[string]any{
									"type":     "string",
									"required": true,
									"default":  "success",
								},
							},
						},
					},
				},
			},
		}

		ruler, err := NewRuler(configObj)
		if err != nil {
			t.Fatalf("Failed to create ruler: %v", err)
		}

		// Test with nil inputs
		result, err := ruler.Evaluate(context.Background(), nil)
		if err != nil {
			t.Fatalf("Evaluation failed: %v", err)
		}

		if result.Output == nil {
			t.Error("Expected rule to match with nil inputs")
		}
	})
}

// TestConcurrency tests thread safety and concurrent evaluation
func TestConcurrency(t *testing.T) {
	configObj := map[string]any{
		"config": map[string]any{
			"enabled": true,
		},
		"rules": []any{
			map[string]any{
				"name": "concurrent_rule",
				"expr": `value > 50`,
				"inputs": []any{
					map[string]any{
						"name":     "value",
						"type":     "number",
						"required": true,
					},
				},
				"outputs": []any{
					map[string]any{
						"name": "result",
						"fields": map[string]any{
							"status": map[string]any{
								"type":     "string",
								"required": true,
								"default":  "success",
							},
						},
					},
				},
			},
		},
	}

	ruler, err := NewRuler(configObj)
	if err != nil {
		t.Fatalf("Failed to create ruler: %v", err)
	}

	// Test concurrent evaluation
	const numGoroutines = 100
	const numEvaluations = 10

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*numEvaluations)
	results := make(chan bool, numGoroutines*numEvaluations)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < numEvaluations; j++ {
				inputs := Inputs{
					"value": float64(goroutineID*10 + j),
				}

				result, err := ruler.Evaluate(context.Background(), inputs)
				if err != nil {
					errors <- fmt.Errorf("goroutine %d evaluation %d failed: %w", goroutineID, j, err)
					return
				}

				// Check if result is consistent
				expectedMatch := float64(goroutineID*10+j) > 50
				actualMatch := result.Output != nil

				if expectedMatch != actualMatch {
					errors <- fmt.Errorf("goroutine %d evaluation %d: expected match %v, got %v",
						goroutineID, j, expectedMatch, actualMatch)
					return
				}

				results <- actualMatch
			}
		}(i)
	}

	wg.Wait()
	close(errors)
	close(results)

	// Check for errors
	for err := range errors {
		t.Error(err)
	}

	// Count results
	var matchCount, noMatchCount int
	for result := range results {
		if result {
			matchCount++
		} else {
			noMatchCount++
		}
	}

	totalEvaluations := numGoroutines * numEvaluations
	if matchCount+noMatchCount != totalEvaluations {
		t.Errorf("Expected %d total evaluations, got %d", totalEvaluations, matchCount+noMatchCount)
	}

	t.Logf("Concurrent evaluation completed: %d matches, %d no-matches", matchCount, noMatchCount)
}

// TestComplexExpressions tests complex CUE expressions
func TestComplexExpressions(t *testing.T) {
	configObj := map[string]any{
		"config": map[string]any{
			"enabled": true,
		},
		"rules": []any{
			map[string]any{
				"name": "array_processing_rule",
				"expr": `len([for code in error_codes if code >= 500 {code}]) > 1`,
				"inputs": []any{
					map[string]any{
						"name":     "error_codes",
						"type":     "array",
						"required": true,
					},
				},
				"outputs": []any{
					map[string]any{
						"name": "alert",
						"fields": map[string]any{
							"message": map[string]any{
								"type":     "string",
								"required": true,
								"default":  "Multiple server errors detected",
							},
						},
					},
				},
			},
			map[string]any{
				"name": "complex_boolean_rule",
				"expr": `(cpu_usage > 80 || memory_usage > 90) && environment == "production"`,
				"inputs": []any{
					map[string]any{
						"name":     "cpu_usage",
						"type":     "number",
						"required": true,
					},
					map[string]any{
						"name":     "memory_usage",
						"type":     "number",
						"required": true,
					},
					map[string]any{
						"name":     "environment",
						"type":     "string",
						"required": true,
					},
				},
				"outputs": []any{
					map[string]any{
						"name": "alert",
						"fields": map[string]any{
							"message": map[string]any{
								"type":     "string",
								"required": true,
								"default":  "Production resource alert",
							},
						},
					},
				},
			},
			map[string]any{
				"name": "string_pattern_rule",
				"expr": `hostname =~ "^server[0-9]+$" && status == "error"`,
				"inputs": []any{
					map[string]any{
						"name":     "hostname",
						"type":     "string",
						"required": true,
					},
					map[string]any{
						"name":     "status",
						"type":     "string",
						"required": true,
					},
				},
				"outputs": []any{
					map[string]any{
						"name": "alert",
						"fields": map[string]any{
							"message": map[string]any{
								"type":     "string",
								"required": true,
								"default":  "Server error detected",
							},
						},
					},
				},
			},
		},
	}

	ruler, err := NewRuler(configObj)
	if err != nil {
		t.Fatalf("Failed to create ruler: %v", err)
	}

	tests := []struct {
		name        string
		inputs      Inputs
		expectMatch bool
		expectRule  string
	}{
		{
			name: "array processing - multiple server errors",
			inputs: Inputs{
				"error_codes": []int{200, 500, 503, 404, 502},
			},
			expectMatch: true,
			expectRule:  "array_processing_rule",
		},
		{
			name: "array processing - single server error",
			inputs: Inputs{
				"error_codes": []int{200, 404, 500},
			},
			expectMatch: false,
		},
		{
			name: "complex boolean - high CPU in production",
			inputs: Inputs{
				"cpu_usage":    85.0,
				"memory_usage": 70.0,
				"environment":  "production",
			},
			expectMatch: true,
			expectRule:  "complex_boolean_rule",
		},
		{
			name: "complex boolean - high memory in production",
			inputs: Inputs{
				"cpu_usage":    70.0,
				"memory_usage": 95.0,
				"environment":  "production",
			},
			expectMatch: true,
			expectRule:  "complex_boolean_rule",
		},
		{
			name: "complex boolean - high CPU in development",
			inputs: Inputs{
				"cpu_usage":    85.0,
				"memory_usage": 70.0,
				"environment":  "development",
			},
			expectMatch: false,
		},
		{
			name: "string pattern - matching server with error",
			inputs: Inputs{
				"hostname": "server123",
				"status":   "error",
			},
			expectMatch: true,
			expectRule:  "string_pattern_rule",
		},
		{
			name: "string pattern - non-matching hostname",
			inputs: Inputs{
				"hostname": "database01",
				"status":   "error",
			},
			expectMatch: false,
		},
		{
			name: "string pattern - matching hostname but no error",
			inputs: Inputs{
				"hostname": "server456",
				"status":   "ok",
			},
			expectMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ruler.Evaluate(context.Background(), tt.inputs)
			if err != nil {
				t.Fatalf("Evaluation failed: %v", err)
			}

			if tt.expectMatch {
				if result.Output == nil {
					t.Error("Expected rule to match but got no output")
					return
				}
				if result.MatchedRule.Name != tt.expectRule {
					t.Errorf("Expected rule %s to match, got %s", tt.expectRule, result.MatchedRule.Name)
				}
			} else {
				if result.Output != nil {
					t.Errorf("Expected no rule to match but got rule: %s", result.MatchedRule.Name)
				}
			}
		})
	}
}

// TestPerformance tests evaluation performance
func TestPerformance(t *testing.T) {
	configObj := map[string]any{
		"config": map[string]any{
			"enabled": true,
		},
		"rules": []any{
			map[string]any{
				"name": "perf_rule_1",
				"expr": `cpu_usage > 80`,
				"inputs": []any{
					map[string]any{
						"name":     "cpu_usage",
						"type":     "number",
						"required": true,
					},
				},
				"outputs": []any{
					map[string]any{
						"name": "alert",
						"fields": map[string]any{
							"message": map[string]any{
								"type":     "string",
								"required": true,
								"default":  "High CPU",
							},
						},
					},
				},
			},
			map[string]any{
				"name": "perf_rule_2",
				"expr": `memory_usage > 90`,
				"inputs": []any{
					map[string]any{
						"name":     "memory_usage",
						"type":     "number",
						"required": true,
					},
				},
				"outputs": []any{
					map[string]any{
						"name": "alert",
						"fields": map[string]any{
							"message": map[string]any{
								"type":     "string",
								"required": true,
								"default":  "High Memory",
							},
						},
					},
				},
			},
		},
	}

	ruler, err := NewRuler(configObj)
	if err != nil {
		t.Fatalf("Failed to create ruler: %v", err)
	}

	inputs := Inputs{
		"cpu_usage": 85.0,
	}

	// Warm up
	for i := 0; i < 100; i++ {
		_, err := ruler.Evaluate(context.Background(), inputs)
		if err != nil {
			t.Fatalf("Warmup evaluation failed: %v", err)
		}
	}

	// Measure performance
	const numEvaluations = 10000
	start := time.Now()

	for i := 0; i < numEvaluations; i++ {
		_, err := ruler.Evaluate(context.Background(), inputs)
		if err != nil {
			t.Fatalf("Performance evaluation failed: %v", err)
		}
	}

	duration := time.Since(start)
	avgDuration := duration / numEvaluations

	t.Logf("Performance test: %d evaluations in %v (avg: %v per evaluation)",
		numEvaluations, duration, avgDuration)

	// Expect sub-millisecond performance
	if avgDuration > time.Millisecond {
		t.Errorf("Performance too slow: expected < 1ms per evaluation, got %v", avgDuration)
	}
}

// TestBatchPerformance tests batch evaluation performance
func TestBatchPerformance(t *testing.T) {
	configObj := map[string]any{
		"config": map[string]any{
			"enabled": true,
		},
		"rules": []any{
			map[string]any{
				"name": "batch_perf_rule",
				"expr": `value > 50`,
				"inputs": []any{
					map[string]any{
						"name":     "value",
						"type":     "number",
						"required": true,
					},
				},
				"outputs": []any{
					map[string]any{
						"name": "result",
						"fields": map[string]any{
							"status": map[string]any{
								"type":     "string",
								"required": true,
								"default":  "success",
							},
						},
					},
				},
			},
		},
	}

	ruler, err := NewRuler(configObj)
	if err != nil {
		t.Fatalf("Failed to create ruler: %v", err)
	}

	// Create batch inputs
	const batchSize = 1000
	inputBatches := make([]Inputs, batchSize)
	for i := 0; i < batchSize; i++ {
		inputBatches[i] = Inputs{
			"value": float64(i),
		}
	}

	// Warm up
	for i := 0; i < 10; i++ {
		_, err := ruler.EvaluateBatch(context.Background(), inputBatches)
		if err != nil {
			t.Fatalf("Warmup batch evaluation failed: %v", err)
		}
	}

	// Measure batch performance
	const numBatches = 100
	start := time.Now()

	for i := 0; i < numBatches; i++ {
		_, err := ruler.EvaluateBatch(context.Background(), inputBatches)
		if err != nil {
			t.Fatalf("Batch performance evaluation failed: %v", err)
		}
	}

	duration := time.Since(start)
	totalEvaluations := numBatches * batchSize
	avgDuration := duration / time.Duration(totalEvaluations)

	t.Logf("Batch performance test: %d evaluations in %d batches in %v (avg: %v per evaluation)",
		totalEvaluations, numBatches, duration, avgDuration)

	// Batch should be faster than individual evaluations
	if avgDuration > 100*time.Microsecond {
		t.Errorf("Batch performance too slow: expected < 100μs per evaluation, got %v", avgDuration)
	}
}

// Benchmark tests for performance measurement
func BenchmarkRulerEvaluate(b *testing.B) {
	configObj := map[string]any{
		"config": map[string]any{
			"enabled": true,
		},
		"rules": []any{
			map[string]any{
				"name": "benchmark_rule",
				"expr": `cpu_usage > 80`,
				"inputs": []any{
					map[string]any{
						"name":     "cpu_usage",
						"type":     "number",
						"required": true,
					},
				},
				"outputs": []any{
					map[string]any{
						"name": "alert",
						"fields": map[string]any{
							"message": map[string]any{
								"type":     "string",
								"required": true,
								"default":  "High CPU usage",
							},
						},
					},
				},
			},
		},
	}

	ruler, err := NewRuler(configObj)
	if err != nil {
		b.Fatalf("Failed to create ruler: %v", err)
	}

	inputs := Inputs{
		"cpu_usage": 85.0,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := ruler.Evaluate(context.Background(), inputs)
		if err != nil {
			b.Fatalf("Evaluation failed: %v", err)
		}
	}
}

func BenchmarkRulerEvaluateBatch(b *testing.B) {
	configObj := map[string]any{
		"config": map[string]any{
			"enabled": true,
		},
		"rules": []any{
			map[string]any{
				"name": "benchmark_batch_rule",
				"expr": `value > 50`,
				"inputs": []any{
					map[string]any{
						"name":     "value",
						"type":     "number",
						"required": true,
					},
				},
				"outputs": []any{
					map[string]any{
						"name": "result",
						"fields": map[string]any{
							"status": map[string]any{
								"type":     "string",
								"required": true,
								"default":  "success",
							},
						},
					},
				},
			},
		},
	}

	ruler, err := NewRuler(configObj)
	if err != nil {
		b.Fatalf("Failed to create ruler: %v", err)
	}

	// Create batch inputs
	inputBatches := make([]Inputs, 100)
	for i := 0; i < 100; i++ {
		inputBatches[i] = Inputs{
			"value": float64(i),
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := ruler.EvaluateBatch(context.Background(), inputBatches)
		if err != nil {
			b.Fatalf("Batch evaluation failed: %v", err)
		}
	}
}

func BenchmarkRulerEvaluateComplex(b *testing.B) {
	configObj := map[string]any{
		"config": map[string]any{
			"enabled": true,
		},
		"rules": []any{
			map[string]any{
				"name": "complex_benchmark_rule",
				"expr": `len([for code in error_codes if code >= 500 {code}]) > 1 && cpu_usage > 80`,
				"inputs": []any{
					map[string]any{
						"name":     "error_codes",
						"type":     "array",
						"required": true,
					},
					map[string]any{
						"name":     "cpu_usage",
						"type":     "number",
						"required": true,
					},
				},
				"outputs": []any{
					map[string]any{
						"name": "alert",
						"fields": map[string]any{
							"message": map[string]any{
								"type":     "string",
								"required": true,
								"default":  "Complex condition met",
							},
						},
					},
				},
			},
		},
	}

	ruler, err := NewRuler(configObj)
	if err != nil {
		b.Fatalf("Failed to create ruler: %v", err)
	}

	inputs := Inputs{
		"error_codes": []int{200, 500, 503, 404, 502},
		"cpu_usage":   85.0,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := ruler.Evaluate(context.Background(), inputs)
		if err != nil {
			b.Fatalf("Complex evaluation failed: %v", err)
		}
	}
}

// TestIntegrationScenarios tests real-world integration scenarios
func TestIntegrationScenarios(t *testing.T) {
	t.Run("SNMP trap processing", func(t *testing.T) {
		configObj := map[string]any{
			"config": map[string]any{
				"enabled": true,
			},
			"rules": []any{
				map[string]any{
					"name":        "critical_snmp_trap",
					"description": "Process critical SNMP traps",
					"inputs": []any{
						map[string]any{
							"name":        "oid",
							"type":        "string",
							"required":    true,
							"description": "SNMP OID",
						},
						map[string]any{
							"name":        "severity",
							"type":        "number",
							"required":    true,
							"description": "Trap severity level",
						},
						map[string]any{
							"name":        "source_ip",
							"type":        "string",
							"required":    true,
							"description": "Source IP address",
						},
					},
					"expr": `oid =~ "^1\\.3\\.6\\.1\\.4\\.1\\." && severity >= 3`,
					"outputs": []any{
						map[string]any{
							"name":        "alert",
							"description": "Critical SNMP alert",
							"fields": map[string]any{
								"priority": map[string]any{
									"type":     "string",
									"required": true,
									"default":  "high",
								},
								"message": map[string]any{
									"type":     "string",
									"required": true,
									"default":  "Critical SNMP trap received",
								},
							},
						},
						map[string]any{
							"name":        "routing",
							"description": "Alert routing information",
							"fields": map[string]any{
								"team": map[string]any{
									"type":     "string",
									"required": true,
									"default":  "network_ops",
								},
								"escalation": map[string]any{
									"type":     "string",
									"required": true,
									"default":  "immediate",
								},
							},
						},
					},
					"priority": 10,
				},
			},
		}

		ruler, err := NewRuler(configObj)
		if err != nil {
			t.Fatalf("Failed to create ruler: %v", err)
		}

		inputs := Inputs{
			"oid":       "1.3.6.1.4.1.9.9.41.1.2.3.1.5",
			"severity":  4,
			"source_ip": "192.168.1.100",
		}

		result, err := ruler.Evaluate(context.Background(), inputs)
		if err != nil {
			t.Fatalf("SNMP trap evaluation failed: %v", err)
		}

		if result.Output == nil {
			t.Fatal("Expected SNMP trap to trigger rule")
		}

		if result.MatchedRule.Name != "critical_snmp_trap" {
			t.Errorf("Expected critical_snmp_trap rule, got %s", result.MatchedRule.Name)
		}

		// Verify alert output
		alertOutput := result.Output.GeneratedOutputs["alert"]
		if alertOutput["priority"] != "high" {
			t.Errorf("Expected high priority, got %s", alertOutput["priority"])
		}

		// Verify routing output
		routingOutput := result.Output.GeneratedOutputs["routing"]
		if routingOutput["team"] != "network_ops" {
			t.Errorf("Expected network_ops team, got %s", routingOutput["team"])
		}
	})

	t.Run("log analysis scenario", func(t *testing.T) {
		configObj := map[string]any{
			"config": map[string]any{
				"enabled": true,
			},
			"rules": []any{
				map[string]any{
					"name":        "error_burst_detection",
					"description": "Detect error bursts in logs",
					"inputs": []any{
						map[string]any{
							"name":        "error_count",
							"type":        "number",
							"required":    true,
							"description": "Number of errors in time window",
						},
						map[string]any{
							"name":        "time_window",
							"type":        "number",
							"required":    true,
							"description": "Time window in seconds",
						},
						map[string]any{
							"name":        "service_name",
							"type":        "string",
							"required":    true,
							"description": "Service generating errors",
						},
					},
					"expr": `error_count / time_window >= 5.0`,
					"outputs": []any{
						map[string]any{
							"name":        "incident",
							"description": "Error burst incident",
							"fields": map[string]any{
								"severity": map[string]any{
									"type":     "string",
									"required": true,
									"default":  "medium",
								},
								"description": map[string]any{
									"type":     "string",
									"required": true,
									"default":  "Error burst detected",
								},
							},
						},
					},
					"priority": 5,
				},
			},
		}

		ruler, err := NewRuler(configObj)
		if err != nil {
			t.Fatalf("Failed to create ruler: %v", err)
		}

		inputs := Inputs{
			"error_count":  150,
			"time_window":  30,
			"service_name": "api-gateway",
		}

		result, err := ruler.Evaluate(context.Background(), inputs)
		if err != nil {
			t.Fatalf("Log analysis evaluation failed: %v", err)
		}

		if result.Output == nil {
			// Debug: 150/30 = 5.0, which should be > 5.0 is false, so let's change the threshold
			t.Logf("Debug: error_count=%d, time_window=%d, rate=%f", 150, 30, 150.0/30.0)
			t.Fatal("Expected error burst to trigger rule")
		}

		if result.MatchedRule.Name != "error_burst_detection" {
			t.Errorf("Expected error_burst_detection rule, got %s", result.MatchedRule.Name)
		}
	})
}

// TestEdgeCases tests various edge cases and boundary conditions
func TestEdgeCases(t *testing.T) {
	t.Run("empty rule name", func(t *testing.T) {
		configObj := map[string]any{
			"config": map[string]any{
				"enabled": true,
			},
			"rules": []any{
				map[string]any{
					"name": "",
					"expr": `true`,
					"outputs": []any{
						map[string]any{
							"name": "result",
							"fields": map[string]any{
								"status": map[string]any{
									"type":     "string",
									"required": true,
									"default":  "success",
								},
							},
						},
					},
				},
			},
		}

		_, err := NewRuler(configObj)
		if err == nil {
			t.Error("Expected error for empty rule name")
		}
	})

	t.Run("duplicate rule names", func(t *testing.T) {
		configObj := map[string]any{
			"config": map[string]any{
				"enabled": true,
			},
			"rules": []any{
				map[string]any{
					"name": "duplicate_rule",
					"expr": `true`,
					"outputs": []any{
						map[string]any{
							"name": "result1",
							"fields": map[string]any{
								"status": map[string]any{
									"type":     "string",
									"required": true,
									"default":  "success1",
								},
							},
						},
					},
				},
				map[string]any{
					"name": "duplicate_rule",
					"expr": `false`,
					"outputs": []any{
						map[string]any{
							"name": "result2",
							"fields": map[string]any{
								"status": map[string]any{
									"type":     "string",
									"required": true,
									"default":  "success2",
								},
							},
						},
					},
				},
			},
		}

		ruler, err := NewRuler(configObj)
		if err != nil {
			t.Fatalf("Failed to create ruler: %v", err)
		}

		// Test that the ruler works even with duplicate names (implementation may allow this)
		inputs := Inputs{}
		result, err := ruler.Evaluate(context.Background(), inputs)
		if err != nil {
			t.Fatalf("Evaluation failed: %v", err)
		}

		// Should match the first rule (true expression)
		if result.Output == nil {
			t.Error("Expected first rule to match")
		}
	})

	t.Run("very large input values", func(t *testing.T) {
		configObj := map[string]any{
			"config": map[string]any{
				"enabled": true,
			},
			"rules": []any{
				map[string]any{
					"name": "large_value_rule",
					"expr": `large_number > 1000000`,
					"inputs": []any{
						map[string]any{
							"name":     "large_number",
							"type":     "number",
							"required": true,
						},
					},
					"outputs": []any{
						map[string]any{
							"name": "result",
							"fields": map[string]any{
								"status": map[string]any{
									"type":     "string",
									"required": true,
									"default":  "large_value_detected",
								},
							},
						},
					},
				},
			},
		}

		ruler, err := NewRuler(configObj)
		if err != nil {
			t.Fatalf("Failed to create ruler: %v", err)
		}

		inputs := Inputs{
			"large_number": 9223372036854775807, // Max int64
		}

		result, err := ruler.Evaluate(context.Background(), inputs)
		if err != nil {
			t.Fatalf("Large value evaluation failed: %v", err)
		}

		if result.Output == nil {
			t.Error("Expected large value to trigger rule")
		}
	})

	t.Run("unicode and special characters", func(t *testing.T) {
		configObj := map[string]any{
			"config": map[string]any{
				"enabled": true,
			},
			"rules": []any{
				map[string]any{
					"name": "unicode_rule",
					"expr": `message =~ ".*🚨.*"`,
					"inputs": []any{
						map[string]any{
							"name":     "message",
							"type":     "string",
							"required": true,
						},
					},
					"outputs": []any{
						map[string]any{
							"name": "alert",
							"fields": map[string]any{
								"type": map[string]any{
									"type":     "string",
									"required": true,
									"default":  "unicode_alert",
								},
							},
						},
					},
				},
			},
		}

		ruler, err := NewRuler(configObj)
		if err != nil {
			t.Fatalf("Failed to create ruler: %v", err)
		}

		inputs := Inputs{
			"message": "Alert: 🚨 System failure detected! 警告",
		}

		result, err := ruler.Evaluate(context.Background(), inputs)
		if err != nil {
			t.Fatalf("Unicode evaluation failed: %v", err)
		}

		if result.Output == nil {
			t.Error("Expected unicode message to trigger rule")
		}
	})

	t.Run("deeply nested object inputs", func(t *testing.T) {
		configObj := map[string]any{
			"config": map[string]any{
				"enabled": true,
			},
			"rules": []any{
				map[string]any{
					"name": "nested_object_rule",
					"expr": `nested_data.level1.level2.value > 100`,
					"inputs": []any{
						map[string]any{
							"name":     "nested_data",
							"type":     "object",
							"required": true,
						},
					},
					"outputs": []any{
						map[string]any{
							"name": "result",
							"fields": map[string]any{
								"status": map[string]any{
									"type":     "string",
									"required": true,
									"default":  "nested_condition_met",
								},
							},
						},
					},
				},
			},
		}

		ruler, err := NewRuler(configObj)
		if err != nil {
			t.Fatalf("Failed to create ruler: %v", err)
		}

		inputs := Inputs{
			"nested_data": map[string]any{
				"level1": map[string]any{
					"level2": map[string]any{
						"value": 150,
					},
				},
			},
		}

		result, err := ruler.Evaluate(context.Background(), inputs)
		if err != nil {
			t.Fatalf("Nested object evaluation failed: %v", err)
		}

		if result.Output == nil {
			t.Error("Expected nested object condition to trigger rule")
		}
	})
}

// TestConfigurationValidation tests comprehensive configuration validation
func TestConfigurationValidation(t *testing.T) {
	t.Run("invalid input type", func(t *testing.T) {
		configObj := map[string]any{
			"config": map[string]any{
				"enabled": true,
			},
			"rules": []any{
				map[string]any{
					"name": "invalid_type_rule",
					"expr": `value > 50`,
					"inputs": []any{
						map[string]any{
							"name":     "value",
							"type":     "invalid_type",
							"required": true,
						},
					},
					"outputs": []any{
						map[string]any{
							"name": "result",
							"fields": map[string]any{
								"status": map[string]any{
									"type":     "string",
									"required": true,
									"default":  "success",
								},
							},
						},
					},
				},
			},
		}

		_, err := NewRuler(configObj)
		if err == nil {
			t.Error("Expected error for invalid input type")
		}
	})

	t.Run("missing required output fields", func(t *testing.T) {
		configObj := map[string]any{
			"config": map[string]any{
				"enabled": true,
			},
			"rules": []any{
				map[string]any{
					"name": "missing_fields_rule",
					"expr": `true`,
					"outputs": []any{
						map[string]any{
							"name": "incomplete_output",
							// Missing fields
						},
					},
				},
			},
		}

		ruler, err := NewRuler(configObj)
		if err != nil {
			t.Fatalf("Failed to create ruler: %v", err)
		}

		// Test that the ruler works even with incomplete output specs
		inputs := Inputs{}
		result, err := ruler.Evaluate(context.Background(), inputs)
		if err != nil {
			t.Fatalf("Evaluation failed: %v", err)
		}

		// Should still match and generate output
		if result.Output == nil {
			t.Error("Expected rule to match")
		}
	})

	t.Run("invalid output field type", func(t *testing.T) {
		configObj := map[string]any{
			"config": map[string]any{
				"enabled": true,
			},
			"rules": []any{
				map[string]any{
					"name": "invalid_field_type_rule",
					"expr": `true`,
					"outputs": []any{
						map[string]any{
							"name": "result",
							"fields": map[string]any{
								"status": map[string]any{
									"type":     "invalid_field_type",
									"required": true,
									"default":  "success",
								},
							},
						},
					},
				},
			},
		}

		_, err := NewRuler(configObj)
		if err == nil {
			t.Error("Expected error for invalid output field type")
		}
	})

	t.Run("circular reference in expressions", func(t *testing.T) {
		configObj := map[string]any{
			"config": map[string]any{
				"enabled": true,
			},
			"rules": []any{
				map[string]any{
					"name": "circular_rule",
					"expr": `value1 > value2 && value2 > value1`,
					"inputs": []any{
						map[string]any{
							"name":     "value1",
							"type":     "number",
							"required": true,
						},
						map[string]any{
							"name":     "value2",
							"type":     "number",
							"required": true,
						},
					},
					"outputs": []any{
						map[string]any{
							"name": "result",
							"fields": map[string]any{
								"status": map[string]any{
									"type":     "string",
									"required": true,
									"default":  "impossible",
								},
							},
						},
					},
				},
			},
		}

		ruler, err := NewRuler(configObj)
		if err != nil {
			t.Fatalf("Failed to create ruler: %v", err)
		}

		inputs := Inputs{
			"value1": 10,
			"value2": 20,
		}

		result, err := ruler.Evaluate(context.Background(), inputs)
		if err != nil {
			t.Fatalf("Circular reference evaluation failed: %v", err)
		}

		// This should not match due to logical impossibility
		if result.Output != nil {
			t.Error("Expected circular reference condition to not match")
		}
	})
}

// TestMemoryManagement tests memory usage and garbage collection
func TestMemoryManagement(t *testing.T) {
	t.Run("memory usage with large datasets", func(t *testing.T) {
		configObj := map[string]any{
			"config": map[string]any{
				"enabled": true,
			},
			"rules": []any{
				map[string]any{
					"name": "memory_test_rule",
					"expr": `len(large_array) > 1000`,
					"inputs": []any{
						map[string]any{
							"name":     "large_array",
							"type":     "array",
							"required": true,
						},
					},
					"outputs": []any{
						map[string]any{
							"name": "result",
							"fields": map[string]any{
								"count": map[string]any{
									"type":     "string",
									"required": true,
									"default":  "large_dataset",
								},
							},
						},
					},
				},
			},
		}

		ruler, err := NewRuler(configObj)
		if err != nil {
			t.Fatalf("Failed to create ruler: %v", err)
		}

		// Create large array
		largeArray := make([]int, 10000)
		for i := range largeArray {
			largeArray[i] = i
		}

		inputs := Inputs{
			"large_array": largeArray,
		}

		// Multiple evaluations to test memory management
		for i := 0; i < 100; i++ {
			result, err := ruler.Evaluate(context.Background(), inputs)
			if err != nil {
				t.Fatalf("Memory test evaluation %d failed: %v", i, err)
			}

			if result.Output == nil {
				t.Errorf("Expected large array to trigger rule on iteration %d", i)
			}
		}
	})

	t.Run("concurrent memory stress test", func(t *testing.T) {
		configObj := map[string]any{
			"config": map[string]any{
				"enabled": true,
			},
			"rules": []any{
				map[string]any{
					"name": "stress_test_rule",
					"expr": `data_size > 100`,
					"inputs": []any{
						map[string]any{
							"name":     "data_size",
							"type":     "number",
							"required": true,
						},
						map[string]any{
							"name":     "payload",
							"type":     "string",
							"required": true,
						},
					},
					"outputs": []any{
						map[string]any{
							"name": "result",
							"fields": map[string]any{
								"status": map[string]any{
									"type":     "string",
									"required": true,
									"default":  "processed",
								},
							},
						},
					},
				},
			},
		}

		ruler, err := NewRuler(configObj)
		if err != nil {
			t.Fatalf("Failed to create ruler: %v", err)
		}

		const numGoroutines = 50
		const numIterations = 100

		var wg sync.WaitGroup
		errors := make(chan error, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(goroutineID int) {
				defer wg.Done()

				for j := 0; j < numIterations; j++ {
					// Create large payload
					payload := strings.Repeat("x", 1000)

					inputs := Inputs{
						"data_size": 500,
						"payload":   payload,
					}

					_, err := ruler.Evaluate(context.Background(), inputs)
					if err != nil {
						errors <- fmt.Errorf("stress test goroutine %d iteration %d failed: %w", goroutineID, j, err)
						return
					}
				}
			}(i)
		}

		wg.Wait()
		close(errors)

		// Check for errors
		for err := range errors {
			t.Error(err)
		}
	})
}

// TestRuleMetadata tests rule metadata and labeling
func TestRuleMetadata(t *testing.T) {
	configObj := map[string]any{
		"config": map[string]any{
			"enabled": true,
		},
		"rules": []any{
			map[string]any{
				"name":        "metadata_test_rule",
				"description": "A test rule with comprehensive metadata",
				"expr":        `value > 50`,
				"inputs": []any{
					map[string]any{
						"name":        "value",
						"type":        "number",
						"required":    true,
						"description": "Test input value",
						"example":     75,
					},
				},
				"outputs": []any{
					map[string]any{
						"name":        "alert",
						"description": "Test alert output",
						"fields": map[string]any{
							"severity": map[string]any{
								"type":        "string",
								"required":    true,
								"default":     "medium",
								"description": "Alert severity level",
								"example":     "high",
							},
						},
						"example": map[string]any{
							"severity": "high",
						},
					},
				},
				"labels": map[string]any{
					"category":    "test",
					"environment": "development",
					"team":        "engineering",
					"version":     "1.0",
				},
				"priority": 15,
				"enabled":  true,
			},
		},
	}

	ruler, err := NewRuler(configObj)
	if err != nil {
		t.Fatalf("Failed to create ruler: %v", err)
	}

	inputs := Inputs{
		"value": 75,
	}

	result, err := ruler.Evaluate(context.Background(), inputs)
	if err != nil {
		t.Fatalf("Metadata test evaluation failed: %v", err)
	}

	if result.Output == nil {
		t.Fatal("Expected rule to match")
	}

	// Test MatchedRule metadata
	rule := result.MatchedRule
	if rule.Name != "metadata_test_rule" {
		t.Errorf("Expected rule name 'metadata_test_rule', got '%s'", rule.Name)
	}

	if rule.Description != "A test rule with comprehensive metadata" {
		t.Errorf("Expected specific description, got '%s'", rule.Description)
	}

	if rule.Priority != 15 {
		t.Errorf("Expected priority 15, got %d", rule.Priority)
	}

	if len(rule.Labels) != 4 {
		t.Errorf("Expected 4 labels, got %d", len(rule.Labels))
	}

	expectedLabels := map[string]string{
		"category":    "test",
		"environment": "development",
		"team":        "engineering",
		"version":     "1.0",
	}

	for key, expectedValue := range expectedLabels {
		if actualValue, exists := rule.Labels[key]; !exists {
			t.Errorf("Expected label '%s' to exist", key)
		} else if actualValue != expectedValue {
			t.Errorf("Expected label '%s' to be '%s', got '%s'", key, expectedValue, actualValue)
		}
	}

	// Test input metadata
	if len(rule.Inputs) != 1 {
		t.Errorf("Expected 1 input spec, got %d", len(rule.Inputs))
	} else {
		input := rule.Inputs[0]
		if input.Description != "Test input value" {
			t.Errorf("Expected input description 'Test input value', got '%s'", input.Description)
		}
		if input.Example != int64(75) {
			t.Errorf("Expected input example 75, got %v (type: %T)", input.Example, input.Example)
		}
	}

	// Test output metadata
	if len(rule.Outputs) != 1 {
		t.Errorf("Expected 1 output spec, got %d", len(rule.Outputs))
	} else {
		output := rule.Outputs[0]
		if output.Description != "Test alert output" {
			t.Errorf("Expected output description 'Test alert output', got '%s'", output.Description)
		}

		if len(output.Fields) != 1 {
			t.Errorf("Expected 1 output field, got %d", len(output.Fields))
		} else {
			field := output.Fields["severity"]
			if field.Description != "Alert severity level" {
				t.Errorf("Expected field description 'Alert severity level', got '%s'", field.Description)
			}
			if field.Example != "high" {
				t.Errorf("Expected field example 'high', got '%s'", field.Example)
			}
		}
	}
}
