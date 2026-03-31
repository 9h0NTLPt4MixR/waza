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
