package execution

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/microsoft/waza/internal/models"
	"github.com/stretchr/testify/require"
)

type stubUsageEngine struct {
	usage map[string]*models.UsageStats
}

func (e *stubUsageEngine) Initialize(context.Context) error { return nil }

func (e *stubUsageEngine) Execute(context.Context, *ExecutionRequest) (*ExecutionResponse, error) {
	return nil, nil
}

func (e *stubUsageEngine) Shutdown(context.Context) error { return nil }

func (e *stubUsageEngine) SessionUsage(sessionID string) *models.UsageStats {
	return e.usage[sessionID]
}

func TestUpdateOutcomeUsage_NilOutcome(t *testing.T) {
	UpdateOutcomeUsage(nil, NewMockEngine("test"))
}

func TestUpdateOutcomeUsage_ReplacesAndReaggregates(t *testing.T) {
	outcome := &models.EvaluationOutcome{
		TestOutcomes: []models.TestOutcome{
			{
				Runs: []models.RunResult{
					{
						SessionDigest: models.SessionDigest{SessionID: "session-1"},
					},
					{
						SessionDigest: models.SessionDigest{SessionID: "session-2"},
					},
				},
			},
		},
	}

	engine := &stubUsageEngine{
		usage: map[string]*models.UsageStats{
			"session-1": {
				InputTokens:     100,
				OutputTokens:    25,
				CacheReadTokens: 4,
				PremiumRequests: 1,
			},
			"session-2": {
				InputTokens:      50,
				OutputTokens:     75,
				CacheWriteTokens: 3,
				Turns:            2,
			},
		},
	}

	UpdateOutcomeUsage(outcome, engine)

	run1 := outcome.TestOutcomes[0].Runs[0]
	run2 := outcome.TestOutcomes[0].Runs[1]

	require.NotNil(t, run1.Usage)
	require.NotNil(t, run2.Usage)
	require.Same(t, run1.SessionDigest.Usage, run1.Usage)
	require.Same(t, run2.SessionDigest.Usage, run2.Usage)
	require.Equal(t, 100, run1.Usage.InputTokens)
	require.Equal(t, 50, run2.Usage.InputTokens)

	require.NotNil(t, outcome.Digest.Usage)
	require.Equal(t, 150, outcome.Digest.Usage.InputTokens)
	require.Equal(t, 100, outcome.Digest.Usage.OutputTokens)
	require.Equal(t, 4, outcome.Digest.Usage.CacheReadTokens)
	require.Equal(t, 3, outcome.Digest.Usage.CacheWriteTokens)
	require.Equal(t, 1.0, outcome.Digest.Usage.PremiumRequests)
	require.Equal(t, 2, outcome.Digest.Usage.Turns)
}

func TestUpdateOutcomeUsage_SkipsEmptySessionID(t *testing.T) {
	originalUsage := &models.UsageStats{InputTokens: 100}
	outcome := &models.EvaluationOutcome{
		TestOutcomes: []models.TestOutcome{
			{
				Runs: []models.RunResult{
					{
						SessionDigest: models.SessionDigest{
							SessionID: "",
							Usage:     originalUsage,
						},
					},
				},
			},
		},
	}

	UpdateOutcomeUsage(outcome, NewMockEngine("test"))

	require.Equal(t, originalUsage, outcome.TestOutcomes[0].Runs[0].SessionDigest.Usage)
	require.Equal(t, originalUsage, outcome.TestOutcomes[0].Runs[0].Usage)
}

func TestUpdateOutcomeUsage_JSONIncludesPerRunAndSummaryUsage(t *testing.T) {
	outcome := &models.EvaluationOutcome{
		TestOutcomes: []models.TestOutcome{
			{
				TestID: "task-1",
				Runs: []models.RunResult{
					{RunNumber: 1, SessionDigest: models.SessionDigest{SessionID: "session-1"}},
					{RunNumber: 2, SessionDigest: models.SessionDigest{SessionID: "session-2"}},
				},
			},
		},
	}

	UpdateOutcomeUsage(outcome, &stubUsageEngine{
		usage: map[string]*models.UsageStats{
			"session-1": {InputTokens: 12, OutputTokens: 34},
			"session-2": {InputTokens: 8, OutputTokens: 9},
		},
	})

	data, err := json.Marshal(outcome)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(data, &payload))

	tasks := payload["tasks"].([]any)
	runs := tasks[0].(map[string]any)["runs"].([]any)
	run1Usage := runs[0].(map[string]any)["usage"].(map[string]any)
	run2Usage := runs[1].(map[string]any)["usage"].(map[string]any)
	summaryUsage := payload["summary"].(map[string]any)["usage"].(map[string]any)

	require.Equal(t, float64(12), run1Usage["input_tokens"])
	require.Equal(t, float64(34), run1Usage["output_tokens"])
	require.Equal(t, float64(8), run2Usage["input_tokens"])
	require.Equal(t, float64(9), run2Usage["output_tokens"])
	require.Equal(t, float64(20), summaryUsage["input_tokens"])
	require.Equal(t, float64(43), summaryUsage["output_tokens"])
}
