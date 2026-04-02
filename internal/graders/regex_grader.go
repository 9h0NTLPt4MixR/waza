package graders

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/microsoft/waza/internal/models"
)

// RegexGrader validates output using regex patterns.
type RegexGrader struct {
	name           string
	mustMatch      []string
	mustNotMatch   []string
	skipIfNoMatch  bool
}

// NewRegexGrader creates a [RegexGrader] that checks for regex pattern
// presence/absence in the agent output.
func NewRegexGrader(name string, args models.RegexGraderParameters) (*RegexGrader, error) {
	return &RegexGrader{
		name:          name,
		mustMatch:     args.MustMatch,
		mustNotMatch:  args.MustNotMatch,
		skipIfNoMatch: args.SkipIfNoMatch,
	}, nil
}

func (rg *RegexGrader) Name() string            { return rg.name }
func (rg *RegexGrader) Kind() models.GraderKind { return models.GraderKindRegex }

func (rg *RegexGrader) Grade(ctx context.Context, gradingContext *Context) (*models.GraderResults, error) {
	return measureTime(func() (*models.GraderResults, error) {
		var failures []string

		// must_match: all patterns must match
		mustMatchFailures := 0
		for _, pattern := range rg.mustMatch {
			re, err := regexp.Compile(pattern)
			if err != nil {
				failures = append(failures, fmt.Sprintf("Invalid must_match pattern %q: %v", pattern, err))
				continue
			}
			if !re.MatchString(gradingContext.Output) {
				if rg.skipIfNoMatch {
					// skip_if_no_match: treat as pass, don't record failure
					continue
				}
				failures = append(failures, fmt.Sprintf("Missing expected pattern: %s", pattern))
				mustMatchFailures++
			}
		}

		// must_not_match: no pattern should match
		for _, pattern := range rg.mustNotMatch {
			re, err := regexp.Compile(pattern)
			if err != nil {
				failures = append(failures, fmt.Sprintf("Invalid must_not_match pattern %q: %v", pattern, err))
				continue
			}
			if re.MatchString(gradingContext.Output) {
				failures = append(failures, fmt.Sprintf("Found forbidden pattern: %s", pattern))
			}
		}

		totalChecks := len(rg.mustMatch) + len(rg.mustNotMatch)
		passedChecks := totalChecks - len(failures)

		score := 1.0
		if totalChecks > 0 {
			score = float64(passedChecks) / float64(totalChecks)
		}

		feedback := "All regex checks passed"
		if len(failures) > 0 {
			feedback = strings.Join(failures, "; ")
		}

		return &models.GraderResults{
			Name:     rg.name,
			Type:     models.GraderKindRegex,
			Score:    score,
			Passed:   len(failures) == 0,
			Feedback: feedback,
			Details: map[string]any{
				"must_match":     rg.mustMatch,
				"must_not_match": rg.mustNotMatch,
				"failures":       failures,
			},
		}, nil
	})
}
