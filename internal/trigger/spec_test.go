package trigger

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTriggerTestSpec_ParseSpec(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string // empty means expect success
		check   func(t *testing.T, spec *TestSpec)
	}{
		// ---- positive cases ----
		{
			name: "valid spec with both trigger and non-trigger prompts",
			yaml: `
skill: code-explainer
should_trigger_prompts:
  - prompt: "Explain this code"
  - prompt: "What does this code do?"
should_not_trigger_prompts:
  - prompt: "Write me a sort function"
    reason: "Code writing, not explaining"
`,
			check: func(t *testing.T, spec *TestSpec) {
				require.Equal(t, "code-explainer", spec.Skill)
				require.Len(t, spec.ShouldTriggerPrompts, 2)
				require.Len(t, spec.ShouldNotTriggerPrompts, 1)
				require.Equal(t, "Explain this code", spec.ShouldTriggerPrompts[0].Prompt)
				require.Equal(t, "What does this code do?", spec.ShouldTriggerPrompts[1].Prompt)
				require.Equal(t, "Write me a sort function", spec.ShouldNotTriggerPrompts[0].Prompt)
				require.Equal(t, "Code writing, not explaining", spec.ShouldNotTriggerPrompts[0].Reason)
			},
		},
		{
			name: "valid spec with only should_trigger_prompts",
			yaml: `
skill: my-skill
should_trigger_prompts:
  - prompt: "Do the thing"
`,
			check: func(t *testing.T, spec *TestSpec) {
				require.Equal(t, "my-skill", spec.Skill)
				require.Len(t, spec.ShouldTriggerPrompts, 1)
				require.Empty(t, spec.ShouldNotTriggerPrompts)
			},
		},
		{
			name: "valid spec with only should_not_trigger_prompts",
			yaml: `
skill: my-skill
should_not_trigger_prompts:
  - prompt: "Unrelated request"
    reason: "Not in scope"
`,
			check: func(t *testing.T, spec *TestSpec) {
				require.Equal(t, "my-skill", spec.Skill)
				require.Empty(t, spec.ShouldTriggerPrompts)
				require.Len(t, spec.ShouldNotTriggerPrompts, 1)
			},
		},
		{
			name: "valid spec with high confidence",
			yaml: `
skill: my-skill
should_trigger_prompts:
  - prompt: "Explain this"
    confidence: high
`,
			check: func(t *testing.T, spec *TestSpec) {
				require.Equal(t, "high", spec.ShouldTriggerPrompts[0].Confidence)
			},
		},
		{
			name: "valid spec with medium confidence",
			yaml: `
skill: my-skill
should_trigger_prompts:
  - prompt: "Explain this"
    confidence: medium
`,
			check: func(t *testing.T, spec *TestSpec) {
				require.Equal(t, "medium", spec.ShouldTriggerPrompts[0].Confidence)
			},
		},
		{
			name: "valid spec with reason and confidence on both lists",
			yaml: `
skill: code-explainer
should_trigger_prompts:
  - prompt: "Explain this code"
    reason: "Directly asks for explanation"
    confidence: high
should_not_trigger_prompts:
  - prompt: "Write a sort"
    reason: "Code generation"
    confidence: medium
`,
			check: func(t *testing.T, spec *TestSpec) {
				require.Equal(t, "Directly asks for explanation", spec.ShouldTriggerPrompts[0].Reason)
				require.Equal(t, "high", spec.ShouldTriggerPrompts[0].Confidence)
				require.Equal(t, "Code generation", spec.ShouldNotTriggerPrompts[0].Reason)
				require.Equal(t, "medium", spec.ShouldNotTriggerPrompts[0].Confidence)
			},
		},
		// ---- negative cases ----
		{
			name:    "missing skill field",
			yaml:    "should_trigger_prompts:\n  - prompt: \"hello\"\n",
			wantErr: "missing required 'skill' field",
		},
		{
			name:    "no prompts at all",
			yaml:    "skill: test-skill\n",
			wantErr: "at least one prompt",
		},
		{
			name:    "empty prompt in should_trigger_prompts",
			yaml:    "skill: test-skill\nshould_trigger_prompts:\n  - prompt: \"\"\n",
			wantErr: "missing required 'prompt' field",
		},
		{
			name:    "empty prompt in should_not_trigger_prompts",
			yaml:    "skill: test-skill\nshould_not_trigger_prompts:\n  - prompt: \"\"\n",
			wantErr: "missing required 'prompt' field",
		},
		{
			name:    "unknown field rejected by strict parsing",
			yaml:    "skill: test-skill\nunknown_field: oops\nshould_trigger_prompts:\n  - prompt: \"hello\"\n",
			wantErr: "parsing trigger tests",
		},
		{
			name:    "unknown nested field rejected",
			yaml:    "skill: test-skill\nshould_trigger_prompts:\n  - prompt: \"hello\"\n    bogus: true\n",
			wantErr: "parsing trigger tests",
		},
		{
			name:    "invalid confidence value",
			yaml:    "skill: test-skill\nshould_trigger_prompts:\n  - prompt: \"hello\"\n    confidence: low\n",
			wantErr: "unrecognized confidence",
		},
		{
			name:    "invalid confidence on should_not_trigger prompt",
			yaml:    "skill: test-skill\nshould_not_trigger_prompts:\n  - prompt: \"hello\"\n    confidence: extreme\n",
			wantErr: "unrecognized confidence",
		},
		{
			name:    "invalid yaml syntax",
			yaml:    "skill: test-skill\nshould_trigger_prompts:\n  - prompt: [unclosed\n",
			wantErr: "parsing trigger tests",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, err := ParseSpec([]byte(tt.yaml))
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			if tt.check != nil {
				tt.check(t, spec)
			}
		})
	}
}
