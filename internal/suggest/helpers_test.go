package suggest

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/microsoft/waza/internal/scaffold"
	"github.com/stretchr/testify/assert"
)

// --- orDefault ---

func TestOrDefault_NonEmpty(t *testing.T) {
	assert.Equal(t, "value", orDefault("value", "fallback"))
}

func TestOrDefault_Empty(t *testing.T) {
	assert.Equal(t, "fallback", orDefault("", "fallback"))
}

func TestOrDefault_Whitespace(t *testing.T) {
	assert.Equal(t, "fallback", orDefault("   ", "fallback"))
}

func TestOrDefault_BothEmpty(t *testing.T) {
	assert.Equal(t, "", orDefault("", ""))
}

// --- phrasesToText ---

func TestPhrasesToText_Empty(t *testing.T) {
	assert.Equal(t, "none", phrasesToText(nil))
}

func TestPhrasesToText_EmptySlice(t *testing.T) {
	assert.Equal(t, "none", phrasesToText([]scaffold.TriggerPhrase{}))
}

func TestPhrasesToText_SinglePhrase(t *testing.T) {
	phrases := []scaffold.TriggerPhrase{{Prompt: "summarize"}}
	assert.Equal(t, "summarize", phrasesToText(phrases))
}

func TestPhrasesToText_MultiplePhrases(t *testing.T) {
	phrases := []scaffold.TriggerPhrase{
		{Prompt: "summarize"},
		{Prompt: "explain"},
		{Prompt: "translate"},
	}
	assert.Equal(t, "summarize, explain, translate", phrasesToText(phrases))
}

func TestPhrasesToText_WhitespaceOnlyPrompts(t *testing.T) {
	phrases := []scaffold.TriggerPhrase{
		{Prompt: "   "},
		{Prompt: ""},
	}
	assert.Equal(t, "none", phrasesToText(phrases))
}

func TestPhrasesToText_MixedEmptyAndValid(t *testing.T) {
	phrases := []scaffold.TriggerPhrase{
		{Prompt: ""},
		{Prompt: "valid"},
		{Prompt: "  "},
	}
	assert.Equal(t, "valid", phrasesToText(phrases))
}

// --- summarizeBody ---

func TestSummarizeBody_Empty(t *testing.T) {
	assert.Equal(t, "No body content", summarizeBody(""))
}

func TestSummarizeBody_WhitespaceOnly(t *testing.T) {
	assert.Equal(t, "No body content", summarizeBody("  \n  \n  "))
}

func TestSummarizeBody_SingleHeading(t *testing.T) {
	result := summarizeBody("# Overview")
	assert.Equal(t, "# Overview", result)
}

func TestSummarizeBody_HeadingsAndText(t *testing.T) {
	body := "# Overview\nThis does things.\n## Details\nMore info here."
	result := summarizeBody(body)
	assert.Contains(t, result, "# Overview")
	assert.Contains(t, result, "This does things.")
	assert.Contains(t, result, "## Details")
	assert.Contains(t, result, "More info here.")
}

func TestSummarizeBody_MaxLines(t *testing.T) {
	// 10+ content lines — should cap at 8
	body := "# H1\nLine1\nLine2\nLine3\nLine4\nLine5\nLine6\nLine7\nLine8\nLine9\nLine10"
	result := summarizeBody(body)
	// Headings always included, plus lines up to 8 total
	assert.NotContains(t, result, "Line9")
	assert.NotContains(t, result, "Line10")
}

func TestSummarizeBody_SkipsBlankLines(t *testing.T) {
	body := "# Title\n\n\nSome content\n\nMore content"
	result := summarizeBody(body)
	assert.Contains(t, result, "# Title")
	assert.Contains(t, result, "Some content")
	assert.Contains(t, result, "More content")
}

// --- extractYAML ---

func TestExtractYAML_Empty(t *testing.T) {
	assert.Equal(t, "", extractYAML(""))
}

func TestExtractYAML_WhitespaceOnly(t *testing.T) {
	assert.Equal(t, "", extractYAML("   \n  \n  "))
}

func TestExtractYAML_NoCodeFence(t *testing.T) {
	input := "name: test\nvalue: 42"
	assert.Equal(t, input, extractYAML(input))
}

func TestExtractYAML_WithCodeFence(t *testing.T) {
	input := "```yaml\nname: test\nvalue: 42\n```"
	assert.Equal(t, "name: test\nvalue: 42", extractYAML(input))
}

func TestExtractYAML_CodeFenceNoClosing(t *testing.T) {
	// No closing ```, should return the original
	input := "```yaml\nname: test\nvalue: 42"
	result := extractYAML(input)
	assert.Equal(t, input, result)
}

func TestExtractYAML_SurroundingText(t *testing.T) {
	input := "Here is the YAML:\n```yaml\nname: extracted\n```\nDone."
	assert.Equal(t, "name: extracted", extractYAML(input))
}

// --- normalizeGeneratedPath ---

func TestNormalizeGeneratedPath_ValidRelative(t *testing.T) {
	path, err := normalizeGeneratedPath("tasks/basic.yaml", "fallback.yaml")
	assert.NoError(t, err)
	assert.Equal(t, filepath.FromSlash("tasks/basic.yaml"), path)
}

func TestNormalizeGeneratedPath_EmptyUsesFallback(t *testing.T) {
	path, err := normalizeGeneratedPath("", "tasks/task-01.yaml")
	assert.NoError(t, err)
	assert.Equal(t, filepath.FromSlash("tasks/task-01.yaml"), path)
}

func TestNormalizeGeneratedPath_WhitespaceUsesFallback(t *testing.T) {
	path, err := normalizeGeneratedPath("   ", "fallback.yaml")
	assert.NoError(t, err)
	assert.Equal(t, "fallback.yaml", path)
}

func TestNormalizeGeneratedPath_AbsolutePathRejected(t *testing.T) {
	absPath := "/etc/passwd"
	if runtime.GOOS == "windows" {
		absPath = `C:\etc\passwd`
	}
	_, err := normalizeGeneratedPath(absPath, "fallback.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid generated path")
}

func TestNormalizeGeneratedPath_ParentTraversalRejected(t *testing.T) {
	_, err := normalizeGeneratedPath("../../etc/passwd", "fallback.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid generated path")
}

func TestNormalizeGeneratedPath_DotSlash(t *testing.T) {
	path, err := normalizeGeneratedPath("./tasks/basic.yaml", "fallback.yaml")
	assert.NoError(t, err)
	assert.Equal(t, filepath.FromSlash("tasks/basic.yaml"), path)
}

// --- filterValidGraderTypes ---

func TestFilterValidGraderTypes_AllValid(t *testing.T) {
	result := filterValidGraderTypes([]string{"code", "text", "file"})
	assert.Equal(t, []string{"code", "text", "file"}, result)
}

func TestFilterValidGraderTypes_MixedValidInvalid(t *testing.T) {
	result := filterValidGraderTypes([]string{"code", "not_a_grader", "text"})
	assert.Equal(t, []string{"code", "text"}, result)
}

func TestFilterValidGraderTypes_AllInvalid(t *testing.T) {
	result := filterValidGraderTypes([]string{"bogus", "fake"})
	assert.Nil(t, result)
}

func TestFilterValidGraderTypes_Empty(t *testing.T) {
	result := filterValidGraderTypes(nil)
	assert.Nil(t, result)
}

func TestFilterValidGraderTypes_WhitespaceTrimmed(t *testing.T) {
	result := filterValidGraderTypes([]string{"  code  ", " text "})
	assert.Equal(t, []string{"code", "text"}, result)
}

// --- AvailableGraderTypes ---

func TestAvailableGraderTypes_NonEmpty(t *testing.T) {
	types := AvailableGraderTypes()
	assert.True(t, len(types) > 0, "should have at least one grader type")
}

func TestAvailableGraderTypes_ContainsExpected(t *testing.T) {
	types := AvailableGraderTypes()
	expected := []string{"code", "prompt", "text", "file", "json_schema", "program", "behavior", "action_sequence", "skill_invocation", "trigger", "diff", "tool_constraint"}
	for _, exp := range expected {
		assert.Contains(t, types, exp, "missing expected grader type %q", exp)
	}
}

// --- parseGraderSelection ---

func TestParseGraderSelection_PlainLines(t *testing.T) {
	result := parseGraderSelection("code\ntext\nfile")
	assert.Equal(t, []string{"code", "text", "file"}, result)
}

func TestParseGraderSelection_WhitespaceAround(t *testing.T) {
	result := parseGraderSelection("  \n  code  \n  text  \n  ")
	assert.Equal(t, []string{"code", "text"}, result)
}

func TestParseGraderSelection_OnlyInvalid(t *testing.T) {
	result := parseGraderSelection("bogus\nfake\nnope")
	assert.Nil(t, result)
}
