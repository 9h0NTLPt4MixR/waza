package models

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// TestCase represents a single evaluation test
type TestCase struct {
	Active      *bool             `yaml:"enabled,omitempty" json:"active,omitempty"`
	ContextRoot string            `yaml:"context_dir,omitempty" json:"context_root,omitempty"`
	DisplayName string            `yaml:"name" json:"display_name"`
	Expectation TestExpectation   `yaml:"expected,omitempty" json:"expectation,omitempty"`
	Stimulus    TestStimulus      `yaml:"inputs" json:"stimulus"`
	Summary     string            `yaml:"description,omitempty" json:"summary,omitempty"`
	Tags        []string          `yaml:"tags,omitempty" json:"labels,omitempty"`
	TestID      string            `yaml:"id" json:"test_id"`
	TimeoutSec  *int              `yaml:"timeout_seconds,omitempty" json:"timeout_sec,omitempty"`
	Validators  []ValidatorInline `yaml:"graders,omitempty" json:"validators,omitempty"`
}

// TestStimulus defines the input for a test
type TestStimulus struct {
	Message     string            `yaml:"prompt" json:"message"`
	Metadata    map[string]any    `yaml:"context,omitempty" json:"metadata,omitempty"`
	Resources   []ResourceRef     `yaml:"files,omitempty" json:"resources,omitempty"`
	Environment map[string]string `yaml:"environment,omitempty" json:"environment,omitempty"`
}

// ResourceRef points to a file or inline content
type ResourceRef struct {
	Location string `yaml:"path,omitempty" json:"location,omitempty"`
	Body     string `yaml:"content,omitempty" json:"body,omitempty"`
}

// TestExpectation defines expected outcomes
type TestExpectation struct {
	OutcomeSpecs    []OutcomeSpec  `yaml:"outcomes,omitempty" json:"outcome_specs,omitempty"`
	ToolPatterns    map[string]any `yaml:"tool_calls,omitempty" json:"tool_patterns,omitempty"`
	BehaviorRules   BehaviorRules  `yaml:"behavior,omitempty" json:"behavior_rules,omitempty"`
	MustInclude     []string       `yaml:"output_contains,omitempty" json:"must_include,omitempty"`
	MustExclude     []string       `yaml:"output_not_contains,omitempty" json:"must_exclude,omitempty"`
	ExpectedTrigger *bool          `yaml:"should_trigger,omitempty" json:"expected_trigger,omitempty"`
}

type OutcomeSpec struct {
	Category  string `yaml:"type" json:"category"`
	Value     any    `yaml:"value,omitempty" json:"value,omitempty"`
	Predicate string `yaml:"condition,omitempty" json:"predicate,omitempty"`
}

type BehaviorRules struct {
	MaxToolInvocations int      `yaml:"max_tool_calls,omitempty" json:"max_tool_invocations,omitempty"`
	MaxRounds          int      `yaml:"max_iterations,omitempty" json:"max_rounds,omitempty"`
	MaxTokens          int      `yaml:"max_tokens,omitempty" json:"max_tokens,omitempty"`
	MustUseTool        []string `yaml:"required_tools,omitempty" json:"must_use_tool,omitempty"`
	ForbidTool         []string `yaml:"forbidden_tools,omitempty" json:"forbid_tool,omitempty"`
}

// ValidatorInline is a validator embedded in a test case
type ValidatorInline struct {
	Identifier string           `yaml:"name" json:"identifier"`
	Kind       GraderKind       `yaml:"type,omitempty" json:"kind,omitempty"`
	Checks     []string         `yaml:"assertions,omitempty" json:"checks,omitempty"`
	Rubric     string           `yaml:"rubric,omitempty" json:"rubric,omitempty"`
	Weight     float64          `yaml:"weight,omitempty" json:"weight,omitempty"`
	Parameters GraderParameters `yaml:"config,omitempty" json:"parameters,omitempty"`
}

func (v *ValidatorInline) EffectiveWeight() float64 {
	if v.Weight <= 0 {
		return 1.0
	}
	return v.Weight
}

func (v *ValidatorInline) UnmarshalYAML(node *yaml.Node) error {
	// We need to unmarshal into a separate struct to apply KnownFields strict parsing, since ValidatorInline has flexible fields based on the Kind.
	type rawValidatorInline struct {
		Identifier string     `yaml:"name"`
		Kind       GraderKind `yaml:"type,omitempty"`
		Checks     []string   `yaml:"assertions,omitempty"`
		Rubric     string     `yaml:"rubric,omitempty"`
		Weight     float64    `yaml:"weight,omitempty"`
		Parameters yaml.Node  `yaml:"config,omitempty"`
	}

	var raw rawValidatorInline

	// Serialize the node back to bytes to leverage KnownFields strict parsing on the raw struct
	bytesData, err := yaml.Marshal(node)
	if err != nil {
		return fmt.Errorf("failed to marshal validator config: %w", err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(bytesData))
	decoder.KnownFields(true)
	if err := decoder.Decode(&raw); err != nil {
		return err
	}

	params, err := decodeGraderParameters(raw.Kind, &raw.Parameters)
	if err != nil {
		return fmt.Errorf("invalid grader config for %q (type %q): %w", raw.Identifier, raw.Kind, err)
	}

	v.Identifier = raw.Identifier
	v.Kind = raw.Kind
	v.Checks = raw.Checks
	v.Rubric = raw.Rubric
	v.Weight = raw.Weight
	v.Parameters = params

	return nil
}

// LoadTestCase loads a test case from YAML
func LoadTestCase(path string) (*TestCase, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var tc TestCase
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true) // Strict parsing to catch unknown fields
	if err := decoder.Decode(&tc); err != nil {
		return nil, fmt.Errorf("parsing test case YAML: %w", err)
	}

	// Note: Active field defaults to nil when not specified in YAML.
	// The runner treats nil as true (enabled by default).
	// Only explicitly set "enabled: false" will disable a test.

	return &tc, nil
}
