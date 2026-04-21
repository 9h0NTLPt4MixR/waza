package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/microsoft/waza/internal/models"
	"github.com/microsoft/waza/internal/projectconfig"
)

// --- buildMetricDeltas ---

func TestBuildMetricDeltas_Basic(t *testing.T) {
	o1 := &models.EvaluationOutcome{
		Measures: map[string]models.MeasureResult{
			"accuracy": {Value: 0.6},
			"speed":    {Value: 100},
		},
	}
	o2 := &models.EvaluationOutcome{
		Measures: map[string]models.MeasureResult{
			"accuracy": {Value: 0.9},
			"speed":    {Value: 80},
		},
	}

	deltas := buildMetricDeltas(o1, o2)
	if len(deltas) != 2 {
		t.Fatalf("expected 2 deltas, got %d", len(deltas))
	}
	if deltas["accuracy"].Delta != 0.3 {
		t.Errorf("accuracy delta = %v, want 0.3", deltas["accuracy"].Delta)
	}
	if deltas["speed"].Delta != -20.0 {
		t.Errorf("speed delta = %v, want -20", deltas["speed"].Delta)
	}
}

func TestBuildMetricDeltas_DisjointMetrics(t *testing.T) {
	o1 := &models.EvaluationOutcome{
		Measures: map[string]models.MeasureResult{
			"only_in_o1": {Value: 5},
		},
	}
	o2 := &models.EvaluationOutcome{
		Measures: map[string]models.MeasureResult{
			"only_in_o2": {Value: 10},
		},
	}

	deltas := buildMetricDeltas(o1, o2)
	if len(deltas) != 2 {
		t.Fatalf("expected 2 deltas, got %d", len(deltas))
	}
	// only_in_o1: o1=5, o2=0 → delta = -5
	if deltas["only_in_o1"].Value1 != 5 || deltas["only_in_o1"].Value2 != 0 {
		t.Errorf("only_in_o1: got v1=%v v2=%v", deltas["only_in_o1"].Value1, deltas["only_in_o1"].Value2)
	}
	// only_in_o2: o1=0, o2=10 → delta = 10
	if deltas["only_in_o2"].Value1 != 0 || deltas["only_in_o2"].Value2 != 10 {
		t.Errorf("only_in_o2: got v1=%v v2=%v", deltas["only_in_o2"].Value1, deltas["only_in_o2"].Value2)
	}
}

func TestBuildMetricDeltas_Empty(t *testing.T) {
	o1 := &models.EvaluationOutcome{Measures: map[string]models.MeasureResult{}}
	o2 := &models.EvaluationOutcome{Measures: map[string]models.MeasureResult{}}

	deltas := buildMetricDeltas(o1, o2)
	if len(deltas) != 0 {
		t.Errorf("expected 0 deltas, got %d", len(deltas))
	}
}

func TestBuildMetricDeltas_Rounding(t *testing.T) {
	// Ensure rounding to 3 decimal places works.
	o1 := &models.EvaluationOutcome{
		Measures: map[string]models.MeasureResult{
			"x": {Value: 0.1},
		},
	}
	o2 := &models.EvaluationOutcome{
		Measures: map[string]models.MeasureResult{
			"x": {Value: 0.3},
		},
	}

	deltas := buildMetricDeltas(o1, o2)
	// 0.3 - 0.1 = 0.19999... should round to 0.2
	if deltas["x"].Delta != 0.2 {
		t.Errorf("delta = %v, want 0.2 (rounded)", deltas["x"].Delta)
	}
}

// --- NewStore edge cases ---

func TestNewStore_NilConfig(t *testing.T) {
	store, err := NewStore(nil, t.TempDir())
	if err != nil {
		t.Fatalf("NewStore(nil) error: %v", err)
	}
	if _, ok := store.(*LocalStore); !ok {
		t.Error("expected *LocalStore for nil config")
	}
}

func TestNewStore_EmptyProvider(t *testing.T) {
	cfg := &projectconfig.StorageConfig{
		Provider: "",
		Enabled:  true,
	}
	store, err := NewStore(cfg, t.TempDir())
	if err != nil {
		t.Fatalf("NewStore(empty provider) error: %v", err)
	}
	if _, ok := store.(*LocalStore); !ok {
		t.Error("expected *LocalStore for empty provider")
	}
}

func TestNewStore_AzureBlobMissingAccountName(t *testing.T) {
	cfg := &projectconfig.StorageConfig{
		Provider:      "azure-blob",
		Enabled:       true,
		AccountName:   "",
		ContainerName: "test-container",
	}
	_, err := NewStore(cfg, t.TempDir())
	if err == nil {
		t.Fatal("expected error for azure-blob with empty accountName")
	}
}

func TestNewStore_AzureBlobMissingContainerName(t *testing.T) {
	cfg := &projectconfig.StorageConfig{
		Provider:      "azure-blob",
		Enabled:       true,
		AccountName:   "myaccount",
		ContainerName: "",
	}
	_, err := NewStore(cfg, t.TempDir())
	if err == nil {
		t.Fatal("expected error for azure-blob with empty containerName")
	}
}

func TestNewStore_UnknownProvider(t *testing.T) {
	cfg := &projectconfig.StorageConfig{
		Provider: "unknown-provider",
		Enabled:  true,
	}
	store, err := NewStore(cfg, t.TempDir())
	if err != nil {
		t.Fatalf("NewStore(unknown) error: %v", err)
	}
	// Unknown provider with enabled=true but not "azure-blob" falls through to local.
	if _, ok := store.(*LocalStore); !ok {
		t.Error("expected *LocalStore for unknown provider")
	}
}

// --- outcomeToResultSummary ---

func TestOutcomeToResultSummary_ZeroTests(t *testing.T) {
	o := &models.EvaluationOutcome{
		RunID:       "run-zero",
		SkillTested: "skill-x",
		Setup:       models.OutcomeSetup{ModelID: "model-y"},
		Timestamp:   time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		Digest:      models.OutcomeDigest{TotalTests: 0, Succeeded: 0},
	}
	s := outcomeToResultSummary(o, "/results")
	if s.PassRate != 0.0 {
		t.Errorf("PassRate = %v, want 0.0 for zero tests", s.PassRate)
	}
	if s.RunID != "run-zero" {
		t.Errorf("RunID = %q, want run-zero", s.RunID)
	}
}

func TestOutcomeToResultSummary_PassRate(t *testing.T) {
	o := &models.EvaluationOutcome{
		RunID:       "run-pass",
		SkillTested: "s",
		Setup:       models.OutcomeSetup{ModelID: "m"},
		Digest:      models.OutcomeDigest{TotalTests: 4, Succeeded: 3},
	}
	s := outcomeToResultSummary(o, "/dir")
	want := 75.0
	if s.PassRate != want {
		t.Errorf("PassRate = %v, want %v", s.PassRate, want)
	}
}

// --- matchesFilter ---

func TestMatchesFilter_AllEmpty(t *testing.T) {
	o := &models.EvaluationOutcome{}
	if !matchesFilter(o, ListOptions{}) {
		t.Error("empty options should match any outcome")
	}
}

func TestMatchesFilter_SkillMismatch(t *testing.T) {
	o := &models.EvaluationOutcome{SkillTested: "skill-a"}
	if matchesFilter(o, ListOptions{Skill: "skill-b"}) {
		t.Error("should not match different skill")
	}
}

func TestMatchesFilter_ModelMismatch(t *testing.T) {
	o := &models.EvaluationOutcome{Setup: models.OutcomeSetup{ModelID: "gpt-4o"}}
	if matchesFilter(o, ListOptions{Model: "claude-sonnet"}) {
		t.Error("should not match different model")
	}
}

func TestMatchesFilter_SinceFilter(t *testing.T) {
	o := &models.EvaluationOutcome{
		Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	since := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if matchesFilter(o, ListOptions{Since: since}) {
		t.Error("should not match outcome before Since")
	}
}

func TestMatchesFilter_AllMatch(t *testing.T) {
	o := &models.EvaluationOutcome{
		SkillTested: "skill-a",
		Timestamp:   time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		Setup:       models.OutcomeSetup{ModelID: "gpt-4o"},
	}
	opts := ListOptions{
		Skill: "skill-a",
		Model: "gpt-4o",
		Since: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	if !matchesFilter(o, opts) {
		t.Error("should match when all filters satisfy")
	}
}

// --- load() edge cases ---

func TestLocalStore_LoadSkipsNonJSON(t *testing.T) {
	dir := t.TempDir()
	// Write a non-JSON file — load should skip it.
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hello"), 0o644)
	ls := NewLocalStore(dir)
	results, err := ls.List(t.Context(), ListOptions{})
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestLocalStore_LoadSkipsInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{not valid json}"), 0o644)
	ls := NewLocalStore(dir)
	results, err := ls.List(t.Context(), ListOptions{})
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for invalid JSON, got %d", len(results))
	}
}

func TestLocalStore_LoadSkipsNonOutcomeJSON(t *testing.T) {
	dir := t.TempDir()
	// Valid JSON but not an EvaluationOutcome (no BenchName, TotalTests==0).
	os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"key":"value"}`), 0o644)
	ls := NewLocalStore(dir)
	results, err := ls.List(t.Context(), ListOptions{})
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for non-outcome JSON, got %d", len(results))
	}
}

func TestLocalStore_LoadGeneratesRunIDFromPath(t *testing.T) {
	dir := t.TempDir()
	// Outcome without RunID — load should derive RunID from file path.
	data, _ := json.Marshal(models.EvaluationOutcome{
		BenchName: "bench",
		Digest:    models.OutcomeDigest{TotalTests: 1, Succeeded: 1},
	})
	os.WriteFile(filepath.Join(dir, "derived-run.json"), data, 0o644)

	ls := NewLocalStore(dir)
	results, err := ls.List(t.Context(), ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].RunID != "derived-run" {
		t.Errorf("RunID = %q, want derived-run", results[0].RunID)
	}
}

func TestLocalStore_ListWithLimit(t *testing.T) {
	dir := t.TempDir()
	// Create 3 outcomes.
	for i := 1; i <= 3; i++ {
		o := makeOutcome(fmt.Sprintf("run-%d", i), "s", "m", i, 10)
		o.Timestamp = time.Date(2026, time.Month(i), 1, 0, 0, 0, 0, time.UTC)
		data, _ := json.Marshal(o)
		os.WriteFile(filepath.Join(dir, fmt.Sprintf("run-%d.json", i)), data, 0o644)
	}

	ls := NewLocalStore(dir)
	results, err := ls.List(t.Context(), ListOptions{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results with Limit=2, got %d", len(results))
	}
}

func TestLocalStore_CompareFirstDownloadError(t *testing.T) {
	dir := t.TempDir()
	ls := NewLocalStore(dir)
	_, err := ls.Compare(t.Context(), "nonexistent-1", "nonexistent-2")
	if err == nil {
		t.Error("expected error when first download fails")
	}
}

func TestLocalStore_CompareSecondDownloadError(t *testing.T) {
	dir := t.TempDir()
	o1 := makeOutcome("exists", "s", "m", 5, 10)
	data, _ := json.Marshal(o1)
	os.WriteFile(filepath.Join(dir, "exists.json"), data, 0o644)

	ls := NewLocalStore(dir)
	_, err := ls.Compare(t.Context(), "exists", "nonexistent")
	if err == nil {
		t.Error("expected error when second download fails")
	}
}

// --- ErrNotFound sentinel ---

func TestErrNotFound_Message(t *testing.T) {
	if ErrNotFound.Error() != "result not found" {
		t.Errorf("ErrNotFound = %q, want %q", ErrNotFound.Error(), "result not found")
	}
}
