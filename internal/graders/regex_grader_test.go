package graders

import (
	"context"
	"testing"

	"github.com/microsoft/waza/internal/models"
	"github.com/stretchr/testify/require"
)

func TestRegexGrader_Basic(t *testing.T) {
	g, err := NewRegexGrader("test", models.RegexGraderParameters{
		MustMatch: []string{`he.*`, `world`},
	})
	require.NoError(t, err)
	require.Equal(t, models.GraderKindRegex, g.Kind())
	require.Equal(t, "test", g.Name())
}

func TestRegexGrader_MustMatch(t *testing.T) {
	tests := []struct {
		name             string
		mustMatch        []string
		output           string
		wantPassed       bool
		wantScore        float64
		wantFeedbackEq   string
		wantFeedbackPart string
	}{
		{
			name:           "all must_match patterns match",
			mustMatch:      []string{`he.*`, `world`},
			output:         "hello world",
			wantPassed:     true,
			wantScore:      1.0,
			wantFeedbackEq: "All regex checks passed",
		},
		{
			name:             "must_match pattern missing",
			mustMatch:        []string{`hello`, `missing`},
			output:           "hello world",
			wantPassed:       false,
			wantScore:        0.5,
			wantFeedbackPart: "Missing expected pattern: missing",
		},
		{
			name:             "invalid must_match pattern reports failure",
			mustMatch:        []string{`[invalid`},
			output:           "anything",
			wantPassed:       false,
			wantFeedbackPart: "Invalid must_match pattern \"[invalid\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, err := NewRegexGrader("test", models.RegexGraderParameters{MustMatch: tt.mustMatch})
			require.NoError(t, err)

			results, err := g.Grade(context.Background(), &Context{Output: tt.output})
			require.NoError(t, err)
			require.Equal(t, tt.wantPassed, results.Passed)
			if tt.wantScore != 0 || tt.wantPassed {
				require.Equal(t, tt.wantScore, results.Score)
			}
			if tt.wantFeedbackEq != "" {
				require.Equal(t, tt.wantFeedbackEq, results.Feedback)
			}
			if tt.wantFeedbackPart != "" {
				require.Contains(t, results.Feedback, tt.wantFeedbackPart)
			}
		})
	}
}

func TestRegexGrader_MustNotMatch(t *testing.T) {
	tests := []struct {
		name             string
		mustNotMatch     []string
		output           string
		wantPassed       bool
		wantScore        float64
		wantFeedbackPart string
	}{
		{
			name:         "passes when pattern absent",
			mustNotMatch: []string{`err.*`, `fail`},
			output:       "all good here",
			wantPassed:   true,
			wantScore:    1.0,
		},
		{
			name:             "fails when forbidden pattern found",
			mustNotMatch:     []string{`error`, `warning`},
			output:           "found an error in output",
			wantPassed:       false,
			wantScore:        0.5,
			wantFeedbackPart: "Found forbidden pattern: error",
		},
		{
			name:             "invalid must_not_match reports failure",
			mustNotMatch:     []string{`[invalid`},
			output:           "anything",
			wantPassed:       false,
			wantFeedbackPart: "Invalid must_not_match pattern \"[invalid\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, err := NewRegexGrader("test", models.RegexGraderParameters{MustNotMatch: tt.mustNotMatch})
			require.NoError(t, err)

			results, err := g.Grade(context.Background(), &Context{Output: tt.output})
			require.NoError(t, err)
			require.Equal(t, tt.wantPassed, results.Passed)
			if tt.wantScore != 0 || tt.wantPassed {
				require.Equal(t, tt.wantScore, results.Score)
			}
			if tt.wantFeedbackPart != "" {
				require.Contains(t, results.Feedback, tt.wantFeedbackPart)
			}
		})
	}
}

func TestRegexGrader_Combined(t *testing.T) {
	t.Run("both fields pass together", func(t *testing.T) {
		g, err := NewRegexGrader("test", models.RegexGraderParameters{
			MustMatch:    []string{`He.*ld`},
			MustNotMatch: []string{`panic`},
		})
		require.NoError(t, err)

		results, err := g.Grade(context.Background(), &Context{Output: "Hello World"})
		require.NoError(t, err)
		require.True(t, results.Passed)
		require.Equal(t, 1.0, results.Score)
		require.Equal(t, "All regex checks passed", results.Feedback)
	})

	t.Run("mixed failures across field types", func(t *testing.T) {
		g, err := NewRegexGrader("test", models.RegexGraderParameters{
			MustMatch:    []string{`hello`, `missing`},  // 1 pass, 1 fail
			MustNotMatch: []string{`world`, `notfound`}, // 1 fail, 1 pass
		})
		require.NoError(t, err)

		results, err := g.Grade(context.Background(), &Context{Output: "hello world"})
		require.NoError(t, err)
		require.False(t, results.Passed)
		require.Equal(t, 0.5, results.Score) // 2 of 4 pass
	})
}

func TestRegexGrader_EdgeCases(t *testing.T) {
	t.Run("no fields yields score 1 and passes", func(t *testing.T) {
		g, err := NewRegexGrader("test", models.RegexGraderParameters{})
		require.NoError(t, err)

		results, err := g.Grade(context.Background(), &Context{Output: "anything"})
		require.NoError(t, err)
		require.True(t, results.Passed)
		require.Equal(t, 1.0, results.Score)
	})

	t.Run("empty output fails must_match", func(t *testing.T) {
		g, err := NewRegexGrader("test", models.RegexGraderParameters{MustMatch: []string{`something`}})
		require.NoError(t, err)

		results, err := g.Grade(context.Background(), &Context{Output: ""})
		require.NoError(t, err)
		require.False(t, results.Passed)
		require.Equal(t, 0.0, results.Score)
	})

	t.Run("result details contains expected fields", func(t *testing.T) {
		g, err := NewRegexGrader("detail-test", models.RegexGraderParameters{
			MustMatch:    []string{`a`},
			MustNotMatch: []string{`z`},
		})
		require.NoError(t, err)

		results, err := g.Grade(context.Background(), &Context{Output: "abc"})
		require.NoError(t, err)
		require.Equal(t, "detail-test", results.Name)
		require.Equal(t, models.GraderKindRegex, results.Type)
		require.Equal(t, []string{"a"}, results.Details["must_match"])
		require.Equal(t, []string{"z"}, results.Details["must_not_match"])
	})

	t.Run("duration is recorded", func(t *testing.T) {
		g, err := NewRegexGrader("test", models.RegexGraderParameters{MustMatch: []string{`ok`}})
		require.NoError(t, err)

		results, err := g.Grade(context.Background(), &Context{Output: "ok"})
		require.NoError(t, err)
		require.GreaterOrEqual(t, results.DurationMs, int64(0))
	})
}

func TestRegexGrader_ViaCreate(t *testing.T) {
	t.Run("Create with RegexGraderParameters works", func(t *testing.T) {
		g, err := Create("from-create", models.RegexGraderParameters{
			MustMatch:    []string{`hello`},
			MustNotMatch: []string{`bye`},
		})
		require.NoError(t, err)
		require.Equal(t, "from-create", g.Name())
		require.Equal(t, models.GraderKindRegex, g.Kind())

		results, err := g.Grade(context.Background(), &Context{Output: "hello world"})
		require.NoError(t, err)
		require.True(t, results.Passed)
		require.Equal(t, 1.0, results.Score)
	})
}

// Ensure RegexGrader satisfies the Grader interface at compile time.
var _ Grader = (*RegexGrader)(nil)
