package wizard

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateSkillMD_BasicSpec(t *testing.T) {
	spec := &SkillSpec{
		Name:         "code-reviewer",
		Description:  "Reviews code for quality and best practices.",
		Triggers:     []string{"review code", "check quality"},
		AntiTriggers: []string{"deploy code", "run tests"},
	}

	result, err := GenerateSkillMD(spec)
	require.NoError(t, err)

	assert.Contains(t, result, "name: code-reviewer")
	assert.NotContains(t, result, "type:")
	assert.Contains(t, result, "Reviews code for quality and best practices.")
	assert.Contains(t, result, "# code-reviewer")
	assert.Contains(t, result, "**USE FOR:**")
	assert.Contains(t, result, "- review code")
	assert.Contains(t, result, "- check quality")
	assert.Contains(t, result, "**DO NOT USE FOR:**")
	assert.Contains(t, result, "- deploy code")
	assert.Contains(t, result, "- run tests")
}

func TestGenerateSkillMD_EmptyTriggers(t *testing.T) {
	spec := &SkillSpec{
		Name:        "minimal-skill",
		Description: "A minimal skill with no triggers.",
	}

	result, err := GenerateSkillMD(spec)
	require.NoError(t, err)

	assert.Contains(t, result, "name: minimal-skill")
	assert.Contains(t, result, "**USE FOR:**")
	assert.Contains(t, result, "**DO NOT USE FOR:**")
}

func TestSplitAndTrim(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"empty", "", nil},
		{"single", "hello", []string{"hello"}},
		{"multiple", "a, b, c", []string{"a", "b", "c"}},
		{"with blanks", "a,, b, ,c", []string{"a", "b", "c"}},
		{"whitespace only", "  ,  ,  ", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitAndTrim(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// pipeInput feeds lines to an io.Pipe with small delays so bubbletea
// processes each field before the next line arrives.
func pipeInput(t *testing.T, lines ...string) io.Reader {
	t.Helper()
	r, w := io.Pipe()
	go func() {
		defer w.Close() //nolint:errcheck
		for _, line := range lines {
			w.Write([]byte(line + "\n")) //nolint:errcheck
			time.Sleep(50 * time.Millisecond)
		}
	}()
	return r
}

func TestRunSkillWizard_ValidInput(t *testing.T) {
	in := pipeInput(t, "my-skill", "A great skill", "trigger1, trigger2", "anti1, anti2")
	out := &bytes.Buffer{}

	spec, err := RunSkillWizard(in, out, "")
	require.NoError(t, err)

	assert.Equal(t, "my-skill", spec.Name)
	assert.Equal(t, "A great skill", spec.Description)
	assert.Equal(t, []string{"trigger1", "trigger2"}, spec.Triggers)
	assert.Equal(t, []string{"anti1", "anti2"}, spec.AntiTriggers)
}

func TestRunSkillWizard_EmptyInput(t *testing.T) {
	in := strings.NewReader("")
	out := &bytes.Buffer{}

	spec, err := RunSkillWizard(in, out, "")
	require.Error(t, err)
	assert.Nil(t, spec)
	assert.ErrorContains(t, err, "invalid skill name")
}

func TestRunSkillWizard_IncompleteInput(t *testing.T) {
	in := pipeInput(t, "my-skill")
	out := &bytes.Buffer{}

	spec, err := RunSkillWizard(in, out, "")
	require.Error(t, err)
	assert.Nil(t, spec)
	assert.ErrorContains(t, err, "description is required")
}

func TestRunSkillWizard_InitialName(t *testing.T) {
	// When initialName is provided, the name field is pre-populated.
	// User submits the pre-populated name and fills in other fields.
	in := pipeInput(t, "azure-deploy", "My pre-named skill", "use for testing", "")
	out := &bytes.Buffer{}

	spec, err := RunSkillWizard(in, out, "azure-deploy")
	require.NoError(t, err)
	assert.Equal(t, "azure-deploy", spec.Name)
	assert.Equal(t, "My pre-named skill", spec.Description)
}
