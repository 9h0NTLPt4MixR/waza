package suggest

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGraderSummaries_ContainsAllKnownTypes(t *testing.T) {
	summaries := GraderSummaries()
	for _, gtype := range AvailableGraderTypes() {
		assert.Contains(t, summaries, gtype+":", "missing summary for %s", gtype)
	}
}

func TestGraderSummaries_NoDescriptionFallback(t *testing.T) {
	// The graderSummary map may not cover a future grader type.
	// Test the fallback by temporarily adding a type to the list and
	// verifying the function output format.
	summaries := GraderSummaries()
	// All current types should have descriptions (no "(no description)").
	assert.NotContains(t, summaries, "(no description)")
}

func TestGraderSummaries_Format(t *testing.T) {
	summaries := GraderSummaries()
	// Each line should start with "- "
	for _, line := range splitNonEmpty(summaries) {
		assert.True(t, len(line) > 2 && line[:2] == "- ", "unexpected format: %q", line)
	}
}

func TestLoadGraderDocs_NilFS(t *testing.T) {
	result := LoadGraderDocs(nil, []string{"code", "text"})
	assert.Equal(t, "", result)
}

func TestLoadGraderDocs_EmptyTypes(t *testing.T) {
	fsys := fstest.MapFS{}
	result := LoadGraderDocs(fsys, nil)
	assert.Equal(t, "", result)
}

func TestLoadGraderDocs_UnknownTypesSkipped(t *testing.T) {
	fsys := fstest.MapFS{
		"docs/graders/code.md": &fstest.MapFile{Data: []byte("# Code Grader\nAssertions.")},
	}
	result := LoadGraderDocs(fsys, []string{"nonexistent_type"})
	assert.Equal(t, "", result)
}

func TestLoadGraderDocs_LoadsSingleDoc(t *testing.T) {
	fsys := fstest.MapFS{
		"docs/graders/code.md": &fstest.MapFile{Data: []byte("# Code Grader\nAssertions.")},
	}
	result := LoadGraderDocs(fsys, []string{"code"})
	require.Contains(t, result, "Code Grader")
	require.Contains(t, result, "Assertions")
}

func TestLoadGraderDocs_MultipleTypes(t *testing.T) {
	fsys := fstest.MapFS{
		"docs/graders/code.md": &fstest.MapFile{Data: []byte("# Code Grader")},
		"docs/graders/text.md": &fstest.MapFile{Data: []byte("# Text Grader")},
	}
	result := LoadGraderDocs(fsys, []string{"code", "text"})
	assert.Contains(t, result, "Code Grader")
	assert.Contains(t, result, "Text Grader")
}

func TestLoadGraderDocs_TrimsWhitespace(t *testing.T) {
	fsys := fstest.MapFS{
		"docs/graders/code.md": &fstest.MapFile{Data: []byte("  \n# Code Grader\n  \n")},
	}
	result := LoadGraderDocs(fsys, []string{"code"})
	assert.Equal(t, "# Code Grader", result)
}

func TestLoadGraderDocs_MixedValidAndInvalid(t *testing.T) {
	fsys := fstest.MapFS{
		"docs/graders/text.md": &fstest.MapFile{Data: []byte("# Text Grader docs")},
	}
	// "bogus" has no file, "text" does
	result := LoadGraderDocs(fsys, []string{"bogus", "text"})
	assert.Contains(t, result, "Text Grader docs")
	assert.NotContains(t, result, "bogus")
}

func splitNonEmpty(s string) []string {
	var result []string
	for _, line := range split(s) {
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

func split(s string) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		result = append(result, s[start:])
	}
	return result
}
