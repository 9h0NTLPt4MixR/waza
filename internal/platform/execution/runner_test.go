// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License. See LICENSE in the project root for license information.

package execution

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/microsoft/waza/internal/platform/auth"
	"github.com/microsoft/waza/internal/platform/db"
	"gopkg.in/yaml.v3"
)

// --- in-memory mock store ---

type mockStore struct {
	runs    map[string]*db.RunRequest
	results map[string]json.RawMessage
}

func newMockStore() *mockStore {
	return &mockStore{
		runs:    make(map[string]*db.RunRequest),
		results: make(map[string]json.RawMessage),
	}
}

func (m *mockStore) CreateUser(_ context.Context, _ *auth.User) error   { return nil }
func (m *mockStore) GetUser(_ context.Context, _ int64) (*auth.User, error) {
	return nil, nil
}
func (m *mockStore) SaveConnection(_ context.Context, _ *db.Connection) error { return nil }
func (m *mockStore) ListConnections(_ context.Context, _ int64, _ db.ConnectionType) ([]*db.Connection, error) {
	return nil, nil
}
func (m *mockStore) DeleteConnection(_ context.Context, _ int64, _ string) error { return nil }
func (m *mockStore) CreateRunRequest(_ context.Context, run *db.RunRequest) error {
	run.Status = db.Queued
	run.CreatedAt = time.Now()
	m.runs[run.ID] = run
	return nil
}
func (m *mockStore) UpdateRunRequest(_ context.Context, run *db.RunRequest) error {
	m.runs[run.ID] = run
	return nil
}
func (m *mockStore) ListRunRequests(_ context.Context, _ int64, _ int) ([]*db.RunRequest, error) {
	return nil, nil
}
func (m *mockStore) GetRunRequest(_ context.Context, _ int64, id string) (*db.RunRequest, error) {
	r, ok := m.runs[id]
	if !ok {
		return nil, nil
	}
	return r, nil
}
func (m *mockStore) SaveResult(_ context.Context, _ int64, runID string, data json.RawMessage) error {
	m.results[runID] = data
	return nil
}
func (m *mockStore) GetResult(_ context.Context, _ int64, runID string) (json.RawMessage, error) {
	return m.results[runID], nil
}
func (m *mockStore) ListResults(_ context.Context, _ int64, _ int) ([]db.ResultSummary, error) {
	return nil, nil
}
func (m *mockStore) GetSetting(_ context.Context, _ string) (string, error) { return "", nil }
func (m *mockStore) SetSetting(_ context.Context, _, _ string) error        { return nil }
func (m *mockStore) RecoverOrphanedRuns(_ context.Context) (int, error)     { return 0, nil }
func (m *mockStore) Close() error                                            { return nil }

// --- tests ---

func TestRunEval_CloneFails(t *testing.T) {
	store := newMockStore()
	run := &db.RunRequest{
		ID:       "run-test-1",
		UserID:   42,
		Repo:     "owner/nonexistent-repo",
		EvalSpec: "eval.yaml",
		Model:    "gpt-4o",
		Status:   db.Queued,
	}
	_ = store.CreateRunRequest(context.Background(), run)

	cfg := RunConfig{
		Store:       store,
		Run:         run,
		GitHubToken: "fake-token",
		Timeout:     10 * time.Second,
	}

	err := RunEval(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error from RunEval with fake repo")
	}

	got := store.runs["run-test-1"]
	if got.Status != db.Failed {
		t.Errorf("expected status Failed, got %s", got.Status)
	}
	if got.Error == "" {
		t.Error("expected non-empty error message")
	}
	if got.CompletedAt == nil {
		t.Error("expected CompletedAt to be set")
	}
}

func TestRunEval_ContextCancelled(t *testing.T) {
	store := newMockStore()
	run := &db.RunRequest{
		ID:       "run-timeout",
		UserID:   42,
		Repo:     "owner/repo",
		EvalSpec: "eval.yaml",
		Model:    "gpt-4o",
		Status:   db.Queued,
	}
	_ = store.CreateRunRequest(context.Background(), run)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cfg := RunConfig{
		Store:       store,
		Run:         run,
		GitHubToken: "fake-token",
		Timeout:     1 * time.Millisecond,
	}

	err := RunEval(ctx, cfg)
	if err == nil {
		t.Fatal("expected error from RunEval with cancelled context")
	}

	got := store.runs["run-timeout"]
	if got.Status != db.Failed {
		t.Errorf("expected status Failed, got %s", got.Status)
	}
}

func TestSanitizeToken(t *testing.T) {
	cases := []struct {
		input, token, want string
	}{
		{"clone failed: ghp_abc123", "ghp_abc123", "clone failed: ***"},
		{"no token here", "xyz987", "no token here"},
		{"", "tok", ""},
		{"has token", "", "has token"},
		{"ghp_XY ghp_XY end", "ghp_XY", "*** *** end"},
	}
	for _, tc := range cases {
		got := sanitizeToken(tc.input, tc.token)
		if got != tc.want {
			t.Errorf("sanitizeToken(%q, %q) = %q, want %q", tc.input, tc.token, got, tc.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Errorf("expected 'short', got %q", got)
	}
	long := "this is a very long message that exceeds the limit"
	got := truncate(long, 20)
	if len(got) != 20 {
		t.Errorf("truncated string length %d, want 20, got %q", len(got), got)
	}
	if got[len(got)-3:] != "..." {
		t.Errorf("expected trailing '...', got %q", got)
	}
}

func TestMarkFailed(t *testing.T) {
	store := newMockStore()
	run := &db.RunRequest{
		ID:     "run-fail",
		UserID: 1,
		Status: db.Running,
	}
	store.runs[run.ID] = run

	markFailed(store, run, "something broke")

	got := store.runs["run-fail"]
	if got.Status != db.Failed {
		t.Errorf("expected Failed, got %s", got.Status)
	}
	if got.Error != "something broke" {
		t.Errorf("expected error message, got %q", got.Error)
	}
	if got.CompletedAt == nil {
		t.Error("expected CompletedAt to be set")
	}
}

// ---------------------------------------------------------------------------
// Tests – Parallel ADC helpers
// ---------------------------------------------------------------------------

func TestDistributeTaskFiles(t *testing.T) {
	tests := []struct {
		name    string
		tasks   []string
		workers int
		want    [][]string
	}{
		{
			name:    "even split",
			tasks:   []string{"a.yaml", "b.yaml", "c.yaml", "d.yaml"},
			workers: 2,
			want:    [][]string{{"a.yaml", "c.yaml"}, {"b.yaml", "d.yaml"}},
		},
		{
			name:    "uneven split",
			tasks:   []string{"a.yaml", "b.yaml", "c.yaml"},
			workers: 2,
			want:    [][]string{{"a.yaml", "c.yaml"}, {"b.yaml"}},
		},
		{
			name:    "single worker gets all",
			tasks:   []string{"a.yaml", "b.yaml"},
			workers: 1,
			want:    [][]string{{"a.yaml", "b.yaml"}},
		},
		{
			name:    "more workers than tasks",
			tasks:   []string{"a.yaml"},
			workers: 3,
			want:    [][]string{{"a.yaml"}, nil, nil},
		},
		{
			name:    "5 tasks 3 workers",
			tasks:   []string{"1", "2", "3", "4", "5"},
			workers: 3,
			want:    [][]string{{"1", "4"}, {"2", "5"}, {"3"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := distributeTaskFiles(tt.tasks, tt.workers)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d shards, want %d", len(got), len(tt.want))
			}
			for i := range tt.want {
				if len(got[i]) != len(tt.want[i]) {
					t.Errorf("shard %d: got %v, want %v", i, got[i], tt.want[i])
					continue
				}
				for j := range tt.want[i] {
					if got[i][j] != tt.want[i][j] {
						t.Errorf("shard %d[%d]: got %q, want %q", i, j, got[i][j], tt.want[i][j])
					}
				}
			}
		})
	}
}

func TestCreateShardEval(t *testing.T) {
	originalYAML := `name: test-eval
description: A test evaluation
config:
  workers: 3
  model: gpt-4o
tasks:
  - "tasks/*.yaml"
graders:
  - type: code
    name: validator
`
	taskFiles := []string{"tasks/task-1.yaml", "tasks/task-3.yaml"}

	result, err := createShardEval(originalYAML, taskFiles)
	if err != nil {
		t.Fatalf("createShardEval: %v", err)
	}

	// Parse result and verify tasks were replaced.
	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("parsing result: %v", err)
	}

	tasks, ok := parsed["tasks"].([]any)
	if !ok {
		t.Fatal("tasks field not found or wrong type")
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
	if tasks[0] != "tasks/task-1.yaml" {
		t.Errorf("task 0: got %q, want %q", tasks[0], "tasks/task-1.yaml")
	}
	if tasks[1] != "tasks/task-3.yaml" {
		t.Errorf("task 1: got %q, want %q", tasks[1], "tasks/task-3.yaml")
	}

	// Verify other fields preserved.
	if parsed["name"] != "test-eval" {
		t.Errorf("name field lost: got %v", parsed["name"])
	}
	config, _ := parsed["config"].(map[string]any)
	if config["model"] != "gpt-4o" {
		t.Errorf("config.model lost: got %v", config["model"])
	}
}

func TestMergeResults_SingleResult(t *testing.T) {
	input := json.RawMessage(`{"tasks": [{"test_id": "t1", "status": "passed"}], "summary": {"total_tests": 1}}`)
	got, err := mergeResults([]json.RawMessage{input})
	if err != nil {
		t.Fatalf("mergeResults: %v", err)
	}
	// Single result should pass through unchanged.
	if string(got) != string(input) {
		t.Errorf("single result should pass through, got %s", got)
	}
}

func TestMergeResults_MultipleResults(t *testing.T) {
	r1 := json.RawMessage(`{
		"eval_id": "test-run",
		"tasks": [
			{"test_id": "t1", "status": "passed", "stats": {"avg_score": 0.8}},
			{"test_id": "t2", "status": "failed", "stats": {"avg_score": 0.3}}
		],
		"summary": {"total_tests": 2, "succeeded": 1, "failed": 1, "errors": 0, "skipped": 0, "success_rate": 0.5}
	}`)

	r2 := json.RawMessage(`{
		"eval_id": "test-run",
		"tasks": [
			{"test_id": "t3", "status": "passed", "stats": {"avg_score": 0.9}},
			{"test_id": "t4", "status": "passed", "stats": {"avg_score": 1.0}}
		],
		"summary": {"total_tests": 2, "succeeded": 2, "failed": 0, "errors": 0, "skipped": 0, "success_rate": 1.0}
	}`)

	merged, err := mergeResults([]json.RawMessage{r1, r2})
	if err != nil {
		t.Fatalf("mergeResults: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(merged, &result); err != nil {
		t.Fatalf("parsing merged: %v", err)
	}

	// Verify tasks are combined.
	tasks, ok := result["tasks"].([]any)
	if !ok {
		t.Fatal("tasks not found in merged result")
	}
	if len(tasks) != 4 {
		t.Errorf("expected 4 tasks, got %d", len(tasks))
	}

	// Verify summary is recalculated.
	summary, ok := result["summary"].(map[string]any)
	if !ok {
		t.Fatal("summary not found in merged result")
	}
	if summary["total_tests"] != float64(4) {
		t.Errorf("total_tests: got %v, want 4", summary["total_tests"])
	}
	if summary["succeeded"] != float64(3) {
		t.Errorf("succeeded: got %v, want 3", summary["succeeded"])
	}
	if summary["failed"] != float64(1) {
		t.Errorf("failed: got %v, want 1", summary["failed"])
	}
	if summary["success_rate"] != 0.75 {
		t.Errorf("success_rate: got %v, want 0.75", summary["success_rate"])
	}
}

func TestMergeResults_Empty(t *testing.T) {
	_, err := mergeResults(nil)
	if err == nil {
		t.Error("expected error for empty results")
	}
}

func TestMergeResults_WithUsageStats(t *testing.T) {
	r1 := json.RawMessage(`{
		"tasks": [{"test_id": "t1", "status": "passed"}],
		"summary": {"total_tests": 1, "succeeded": 1, "failed": 0, "errors": 0, "skipped": 0,
			"usage": {"turns": 5, "input_tokens": 1000, "output_tokens": 500}}
	}`)

	r2 := json.RawMessage(`{
		"tasks": [{"test_id": "t2", "status": "passed"}],
		"summary": {"total_tests": 1, "succeeded": 1, "failed": 0, "errors": 0, "skipped": 0,
			"usage": {"turns": 3, "input_tokens": 800, "output_tokens": 400}}
	}`)

	merged, err := mergeResults([]json.RawMessage{r1, r2})
	if err != nil {
		t.Fatalf("mergeResults: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(merged, &result); err != nil {
		t.Fatalf("parsing: %v", err)
	}

	summary := result["summary"].(map[string]any)
	usage := summary["usage"].(map[string]any)

	if usage["turns"] != float64(8) {
		t.Errorf("turns: got %v, want 8", usage["turns"])
	}
	if usage["input_tokens"] != float64(1800) {
		t.Errorf("input_tokens: got %v, want 1800", usage["input_tokens"])
	}
	if usage["output_tokens"] != float64(900) {
		t.Errorf("output_tokens: got %v, want 900", usage["output_tokens"])
	}
}

func TestRunViaADC_DispatchesSingleWhenWorkersZero(t *testing.T) {
	// When Workers is 0 or 1, should go through single-sandbox path (not parallel).
	// We verify this indirectly: with no ADCEngine, RunEval returns the local error path.
	store := newMockStore()
	run := &db.RunRequest{
		ID:       "run-dispatch",
		UserID:   42,
		Repo:     "owner/repo",
		EvalSpec: "eval.yaml",
		Workers:  0, // single mode
		Status:   db.Queued,
	}
	_ = store.CreateRunRequest(context.Background(), run)

	// This should use runLocal (no ADCEngine set) and fail at clone.
	cfg := RunConfig{
		Store:       store,
		Run:         run,
		GitHubToken: "fake",
		Timeout:     5 * time.Second,
	}
	err := RunEval(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error")
	}
	// Verify it went through local path (not parallel ADC).
	got := store.runs["run-dispatch"]
	if got.Status != db.Failed {
		t.Errorf("expected Failed, got %s", got.Status)
	}
}
