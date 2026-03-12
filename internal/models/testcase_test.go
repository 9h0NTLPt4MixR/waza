package models

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadTestCase_ShouldTriggerField(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantNil bool
		wantVal bool
	}{
		{
			name: "should_trigger true",
			yaml: `id: tc-trigger-true
name: Trigger True
inputs:
  prompt: "test prompt"
expected:
  should_trigger: true
`,
			wantNil: false,
			wantVal: true,
		},
		{
			name: "should_trigger false",
			yaml: `id: tc-trigger-false
name: Trigger False
inputs:
  prompt: "test prompt"
expected:
  should_trigger: false
`,
			wantNil: false,
			wantVal: false,
		},
		{
			name: "should_trigger omitted",
			yaml: `id: tc-trigger-omit
name: Trigger Omitted
inputs:
  prompt: "test prompt"
`,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			p := filepath.Join(dir, "tc.yaml")
			if err := os.WriteFile(p, []byte(tt.yaml), 0o644); err != nil {
				t.Fatalf("write file: %v", err)
			}

			tc, err := LoadTestCase(p)
			if err != nil {
				t.Fatalf("LoadTestCase: %v", err)
			}

			if tt.wantNil {
				if tc.Expectation.ExpectedTrigger != nil {
					t.Errorf("expected ExpectedTrigger nil, got %v", *tc.Expectation.ExpectedTrigger)
				}
				return
			}

			if tc.Expectation.ExpectedTrigger == nil {
				t.Fatal("expected ExpectedTrigger non-nil, got nil")
			}
			if *tc.Expectation.ExpectedTrigger != tt.wantVal {
				t.Errorf("ExpectedTrigger = %v, want %v", *tc.Expectation.ExpectedTrigger, tt.wantVal)
			}
		})
	}
}

func TestLoadTestCase_ValidatorInline_AssertionsAtRootLevel_Error(t *testing.T) {
	// When a 'code' grader has assertions at root level (instead of under config:),
	// LoadTestCase should return an error.
	dir := t.TempDir()
	yamlContent := `id: tc-001
name: Test
inputs:
  prompt: "say hello"
graders:
  - name: my-grader
    type: code
    assertions:
      - "len(output) > 0"
`
	p := filepath.Join(dir, "tc.yaml")
	if err := os.WriteFile(p, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, err := LoadTestCase(p)
	if err == nil {
		t.Fatal("expected error when assertions are at root level for code grader, got nil")
	}
	if !strings.Contains(err.Error(), "config") {
		t.Errorf("expected error to mention 'config:', got: %v", err)
	}
}

func TestLoadTestCase_ValidatorInline_NoAssertions_Error(t *testing.T) {
	// A 'code' grader with no assertions (neither root nor config level) should fail.
	dir := t.TempDir()
	yamlContent := `id: tc-002
name: Test
inputs:
  prompt: "say hello"
graders:
  - name: empty-grader
    type: code
    config: {}
`
	p := filepath.Join(dir, "tc.yaml")
	if err := os.WriteFile(p, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, err := LoadTestCase(p)
	if err == nil {
		t.Fatal("expected error for code grader with no assertions, got nil")
	}
	if !strings.Contains(err.Error(), "assertions") {
		t.Errorf("expected error to mention 'assertions', got: %v", err)
	}
}

func TestLoadTestCase_ValidatorInline_UnknownField_Error(t *testing.T) {
	// Unknown top-level fields in a grader definition should cause an error.
	dir := t.TempDir()
	yamlContent := `id: tc-003
name: Test
inputs:
  prompt: "say hello"
graders:
  - name: my-grader
    type: text
    contains:
      - "hello"
`
	p := filepath.Join(dir, "tc.yaml")
	if err := os.WriteFile(p, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, err := LoadTestCase(p)
	if err == nil {
		t.Fatal("expected error for unknown field 'contains' at grader root level, got nil")
	}
	if !strings.Contains(err.Error(), "contains") || !strings.Contains(err.Error(), "config") {
		t.Errorf("expected error to mention 'contains' and 'config:', got: %v", err)
	}
}
