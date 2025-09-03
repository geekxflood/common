package ruler

import "list"

// InputSpec defines expected input structure for a rule
#InputSpec: {
	// name of the input field (required)
	name: string & =~"^[a-zA-Z_][a-zA-Z0-9_]*$"

	// type constraint for the input value
	type?: "string" | "number" | "boolean" | "array" | "object" | *"any"

	// required indicates if this input must be present for rule to be considered
	required?: bool | *true

	// description of what this input represents
	description?: string

	// example value for documentation and testing
	example?: _
}

// OutputSpec defines the structure of rule outputs
#OutputSpec: {
	// name of the output category (required)
	name: string & =~"^[a-zA-Z_][a-zA-Z0-9_-]*$"

	// description of what this output represents
	description?: string

	// fields defines the structure of this output category
	fields: {[string]: #OutputField}

	// example output for documentation and testing
	example?: {[string]: string}
}

// OutputField defines a single field in an output
#OutputField: {
	// type of the output field value
	type?: "string" | "number" | "boolean" | *"string"

	// description of what this field contains
	description?: string

	// required indicates if this field must be present in the output
	required?: bool | *true

	// default value if not specified in the rule
	default?: string

	// example value for documentation
	example?: string
}

// Rule defines the structure of a single rule
#Rule: {
	// name is a unique identifier for the rule (required)
	name: string & =~"^[a-zA-Z_][a-zA-Z0-9_-]*$"

	// description explains what this rule does
	description?: string

	// inputs defines the expected input structure for this rule
	// This allows for rule selection based on available input data
	inputs?: [...#InputSpec] & list.MinItems(1)

	// expr is the evaluation expression (required)
	// The expression has access to input data for evaluation
	// Input fields are available as variables by their names
	expr: string & =~"^.+$"

	// outputs defines the structured outputs this rule can generate (required)
	// Can be either a simple map structure or a structured OutputSpec list
	outputs: [...#OutputSpec] & list.MinItems(1)

	// Optional metadata
	labels?: {[string]: string}

	// enabled controls whether this rule is active
	enabled?: bool | *true

	// priority for rule execution order (higher numbers execute first)
	priority?: int | *0
}

// Config defines the overall configuration structure
#Config: {
	// config contains operational settings for the ruler
	config: {
		// enabled controls whether the ruler is active
		enabled?: bool | *true

		// default_message when no rules match
		default_message?: string | *"no rule found for contents"

		// max_concurrent_rules limits parallel rule execution
		max_concurrent_rules?: int & >0 | *10

		// timeout for rule evaluation
		timeout?: string | *"20ms"
	}

	// rules is the list of rules to evaluate
	rules: [...#Rule] & list.MinItems(0)
}
