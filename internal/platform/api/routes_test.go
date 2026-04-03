package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/microsoft/waza/internal/platform/auth"
	"github.com/microsoft/waza/internal/platform/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Mock auth provider — satisfies auth.AuthProvider
// ---------------------------------------------------------------------------

type mockAuthProvider struct {
	users    map[string]*auth.User // token → user
	revoked  map[string]bool
}

func newMockAuthProvider() *mockAuthProvider {
	return &mockAuthProvider{
		users:   make(map[string]*auth.User),
		revoked: make(map[string]bool),
	}
}

func (m *mockAuthProvider) HandleLogin(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "https://github.com/login/oauth/authorize", http.StatusTemporaryRedirect)
}

func (m *mockAuthProvider) HandleCallback(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (m *mockAuthProvider) ValidateSession(_ context.Context, token string) (*auth.User, error) {
	if m.revoked[token] {
		return nil, http.ErrNoCookie
	}
	u, ok := m.users[token]
	if !ok {
		return nil, http.ErrNoCookie
	}
	return u, nil
}

func (m *mockAuthProvider) GetUser(_ context.Context, githubID int64) (*auth.User, error) {
	for _, u := range m.users {
		if u.GitHubID == githubID {
			return u, nil
		}
	}
	return nil, nil
}

func (m *mockAuthProvider) RevokeSession(_ context.Context, token string) error {
	m.revoked[token] = true
	return nil
}

var _ auth.AuthProvider = (*mockAuthProvider)(nil)

// ---------------------------------------------------------------------------
// Mock store — satisfies db.Store
// ---------------------------------------------------------------------------

type mockStore struct {
	users       map[int64]*auth.User
	connections map[string]*db.Connection
	runs        map[string]*db.RunRequest
	settings    map[string]string
}

func newMockStore() *mockStore {
	return &mockStore{
		users:       make(map[int64]*auth.User),
		connections: make(map[string]*db.Connection),
		runs:        make(map[string]*db.RunRequest),
		settings:    make(map[string]string),
	}
}

func (m *mockStore) CreateUser(_ context.Context, u *auth.User) error {
	m.users[u.GitHubID] = u
	return nil
}

func (m *mockStore) GetUser(_ context.Context, id int64) (*auth.User, error) {
	u := m.users[id]
	return u, nil
}

func (m *mockStore) SaveConnection(_ context.Context, c *db.Connection) error {
	m.connections[c.ID] = c
	return nil
}

func (m *mockStore) ListConnections(_ context.Context, userID int64, _ db.ConnectionType) ([]*db.Connection, error) {
	var result []*db.Connection
	for _, c := range m.connections {
		if c.UserID == userID {
			result = append(result, c)
		}
	}
	return result, nil
}

func (m *mockStore) DeleteConnection(_ context.Context, userID int64, connID string) error {
	c, ok := m.connections[connID]
	if !ok || c.UserID != userID {
		return http.ErrNoCookie // placeholder
	}
	delete(m.connections, connID)
	return nil
}

func (m *mockStore) CreateRunRequest(_ context.Context, r *db.RunRequest) error {
	r.CreatedAt = time.Now()
	r.Status = db.Queued
	m.runs[r.ID] = r
	return nil
}

func (m *mockStore) UpdateRunRequest(_ context.Context, r *db.RunRequest) error {
	m.runs[r.ID] = r
	return nil
}

func (m *mockStore) ListRunRequests(_ context.Context, userID int64, _ int) ([]*db.RunRequest, error) {
	var result []*db.RunRequest
	for _, r := range m.runs {
		if r.UserID == userID {
			result = append(result, r)
		}
	}
	return result, nil
}

func (m *mockStore) GetRunRequest(_ context.Context, userID int64, runID string) (*db.RunRequest, error) {
	r, ok := m.runs[runID]
	if !ok || r.UserID != userID {
		return nil, http.ErrNoCookie
	}
	return r, nil
}

func (m *mockStore) GetSetting(_ context.Context, key string) (string, error) {
	return m.settings[key], nil
}

func (m *mockStore) SetSetting(_ context.Context, key, value string) error {
	m.settings[key] = value
	return nil
}

func (m *mockStore) Close() error                                          { return nil }
func (m *mockStore) RecoverOrphanedRuns(_ context.Context) (int, error)     { return 0, nil }

func (m *mockStore) SaveResult(_ context.Context, _ int64, _ string, _ json.RawMessage) error {
	return nil
}

func (m *mockStore) GetResult(_ context.Context, _ int64, _ string) (json.RawMessage, error) {
	return nil, nil
}

func (m *mockStore) ListResults(_ context.Context, _ int64, _ int) ([]db.ResultSummary, error) {
	return nil, nil
}

var _ db.Store = (*mockStore)(nil)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func setupTestRouter() (*http.ServeMux, *mockAuthProvider, *mockStore) {
	ap := newMockAuthProvider()
	store := newMockStore()
	middleware := auth.NewAuthMiddleware(ap)

	mux := http.NewServeMux()
	RegisterRoutes(mux, &Dependencies{
		Auth:           ap,
		Store:          store,
		AuthMiddleware: middleware,
	})
	return mux, ap, store
}

func addTestUser(ap *mockAuthProvider, token string, user *auth.User) {
	ap.users[token] = user
}

// ---------------------------------------------------------------------------
// Tests – Auth enforcement
// ---------------------------------------------------------------------------

func TestAuthRequired_ProtectedEndpoints(t *testing.T) {
	mux, _, _ := setupTestRouter()

	protectedPaths := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/connections"},
		{http.MethodPost, "/api/connections"},
		{http.MethodDelete, "/api/connections/conn-1"},
		{http.MethodPost, "/api/connections/test"},
		{http.MethodPost, "/api/runs/trigger"},
		{http.MethodGet, "/api/runs/queue"},
		{http.MethodGet, "/api/runs/queue/run-1"},
		{http.MethodPost, "/api/runs/cancel/run-1"},
		{http.MethodGet, "/api/runs/batch/batch-1"},
		{http.MethodGet, "/api/repos"},
		{http.MethodGet, "/api/auth/me"},
		{http.MethodPost, "/api/auth/logout"},
	}

	for _, ep := range protectedPaths {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusUnauthorized, rec.Code,
				"%s %s must return 401 without auth", ep.method, ep.path)
		})
	}
}

func TestAuthNotRequired_LoginEndpoints(t *testing.T) {
	mux, _, _ := setupTestRouter()

	publicPaths := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/auth/github"},
		{http.MethodGet, "/api/auth/callback"},
	}

	for _, ep := range publicPaths {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			assert.NotEqual(t, http.StatusUnauthorized, rec.Code,
				"%s %s must not require auth", ep.method, ep.path)
		})
	}
}

// ---------------------------------------------------------------------------
// Tests – Authenticated endpoint: /api/auth/me
// ---------------------------------------------------------------------------

func TestAuthMe_ReturnsUser(t *testing.T) {
	mux, ap, _ := setupTestRouter()
	user := &auth.User{GitHubID: 12345, Login: "testuser", Name: "Test User"}
	addTestUser(ap, "valid-token", user)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var got auth.User
	err := json.Unmarshal(rec.Body.Bytes(), &got)
	require.NoError(t, err)
	assert.Equal(t, "testuser", got.Login)
}

// ---------------------------------------------------------------------------
// Tests – Connections CRUD
// ---------------------------------------------------------------------------

func TestConnectionsCRUD(t *testing.T) {
	mux, ap, store := setupTestRouter()
	user := &auth.User{GitHubID: 100, Login: "alice"}
	addTestUser(ap, "tok", user)

	// POST /api/connections — create
	body, _ := json.Marshal(map[string]any{
		"type":   "azure-storage",
		"config": map[string]any{"account_name": "myaccount", "container_name": "results"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/connections", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var created db.Connection
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	assert.Equal(t, db.AzureStorage, created.Type)
	assert.Equal(t, int64(100), created.UserID)

	// GET /api/connections — list
	req = httptest.NewRequest(http.MethodGet, "/api/connections", nil)
	req.Header.Set("Authorization", "Bearer tok")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	var conns []db.Connection
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &conns))
	assert.Len(t, conns, 1)

	// DELETE /api/connections/{id}
	req = httptest.NewRequest(http.MethodDelete, "/api/connections/"+created.ID, nil)
	req.Header.Set("Authorization", "Bearer tok")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Verify deletion in store
	remaining, _ := store.ListConnections(context.Background(), 100, "")
	assert.Empty(t, remaining)
}

// ---------------------------------------------------------------------------
// Tests – Run trigger & lifecycle
// ---------------------------------------------------------------------------

func TestRunTrigger_ValidRequest(t *testing.T) {
	mux, ap, store := setupTestRouter()
	user := &auth.User{GitHubID: 100, Login: "alice"}
	addTestUser(ap, "tok", user)

	body, _ := json.Marshal(map[string]string{
		"repo":      "testorg/testrepo",
		"eval_spec": "evals/test.yaml",
		"model":     "gpt-4o",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/runs/trigger", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusAccepted, rec.Code)

	var resp struct {
		RunID  string `json:"runId"`
		Status string `json:"status"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.RunID, "response must include runId")
	assert.Equal(t, "queued", resp.Status)

	// Verify persisted in store
	runs, _ := store.ListRunRequests(context.Background(), 100, 10)
	assert.Len(t, runs, 1)
	assert.Equal(t, "testorg/testrepo", runs[0].Repo)
	assert.Equal(t, "gpt-4o", runs[0].Model)
}

func TestRunTrigger_InvalidEvalSpec(t *testing.T) {
	mux, ap, _ := setupTestRouter()
	user := &auth.User{GitHubID: 100, Login: "alice"}
	addTestUser(ap, "tok", user)

	body, _ := json.Marshal(map[string]string{
		"repo": "testorg/testrepo",
		// missing eval_spec
	})
	req := httptest.NewRequest(http.MethodPost, "/api/runs/trigger", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRunTrigger_EmptyBody(t *testing.T) {
	mux, ap, _ := setupTestRouter()
	user := &auth.User{GitHubID: 100, Login: "alice"}
	addTestUser(ap, "tok", user)

	req := httptest.NewRequest(http.MethodPost, "/api/runs/trigger", bytes.NewReader([]byte("{}")))
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRunCancel(t *testing.T) {
	mux, ap, store := setupTestRouter()
	user := &auth.User{GitHubID: 100, Login: "alice"}
	addTestUser(ap, "tok", user)

	// Create a run first
	run := &db.RunRequest{
		ID:     "run-cancel-test",
		UserID: 100,
		Repo:   "testorg/testrepo",
		Status: db.Running,
	}
	store.runs[run.ID] = run

	req := httptest.NewRequest(http.MethodPost, "/api/runs/cancel/run-cancel-test", nil)
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Verify status changed
	updated := store.runs["run-cancel-test"]
	assert.Equal(t, db.Cancelled, updated.Status)
	assert.NotNil(t, updated.CompletedAt)
}

// ---------------------------------------------------------------------------
// Tests – User isolation
// ---------------------------------------------------------------------------

func TestRunQueue_UserIsolation(t *testing.T) {
	mux, ap, store := setupTestRouter()

	userA := &auth.User{GitHubID: 100, Login: "alice"}
	userB := &auth.User{GitHubID: 200, Login: "bob"}
	addTestUser(ap, "tok-a", userA)
	addTestUser(ap, "tok-b", userB)

	// Create runs for both users
	store.runs["run-a"] = &db.RunRequest{ID: "run-a", UserID: 100, Repo: "a/repo", Status: db.Queued}
	store.runs["run-b"] = &db.RunRequest{ID: "run-b", UserID: 200, Repo: "b/repo", Status: db.Queued}

	// User A sees only their runs
	req := httptest.NewRequest(http.MethodGet, "/api/runs/queue", nil)
	req.Header.Set("Authorization", "Bearer tok-a")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	var runsA []struct {
		ID     string `json:"id"`
		Repo   string `json:"repo"`
		Status string `json:"status"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &runsA))
	assert.Len(t, runsA, 1)
	assert.Equal(t, "run-a", runsA[0].ID)

	// User B sees only their runs
	req = httptest.NewRequest(http.MethodGet, "/api/runs/queue", nil)
	req.Header.Set("Authorization", "Bearer tok-b")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	var runsB []struct {
		ID     string `json:"id"`
		Repo   string `json:"repo"`
		Status string `json:"status"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &runsB))
	assert.Len(t, runsB, 1)
	assert.Equal(t, "run-b", runsB[0].ID)
}

// ---------------------------------------------------------------------------
// Tests – Stub handlers return expected defaults
// ---------------------------------------------------------------------------

func TestListConnections_EmptyDefault(t *testing.T) {
	mux, ap, _ := setupTestRouter()
	user := &auth.User{GitHubID: 100, Login: "alice"}
	addTestUser(ap, "tok", user)

	req := httptest.NewRequest(http.MethodGet, "/api/connections", nil)
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var conns []json.RawMessage
	err := json.Unmarshal(rec.Body.Bytes(), &conns)
	require.NoError(t, err)
	assert.Empty(t, conns, "stub should return empty array")
}

func TestTriggerRun_Returns202(t *testing.T) {
	mux, ap, _ := setupTestRouter()
	user := &auth.User{GitHubID: 100, Login: "alice"}
	addTestUser(ap, "tok", user)

	body, _ := json.Marshal(map[string]string{
		"repo":      "testorg/testrepo",
		"eval_spec": "evals/test.yaml",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/runs/trigger", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusAccepted, rec.Code)
}

func TestLogout_Returns204(t *testing.T) {
	mux, ap, _ := setupTestRouter()
	user := &auth.User{GitHubID: 100, Login: "alice"}
	addTestUser(ap, "tok", user)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestGetRunFromQueue_ReturnsRun(t *testing.T) {
	mux, ap, store := setupTestRouter()
	user := &auth.User{GitHubID: 100, Login: "alice"}
	addTestUser(ap, "tok", user)

	store.runs["run-detail"] = &db.RunRequest{
		ID:       "run-detail",
		UserID:   100,
		Repo:     "org/repo",
		EvalSpec: "evals/test.yaml",
		Model:    "gpt-4o",
		Status:   db.Queued,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/runs/queue/run-detail", nil)
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var got struct {
		ID       string `json:"id"`
		Repo     string `json:"repo"`
		EvalSpec string `json:"evalSpec"`
		Model    string `json:"model"`
		Status   string `json:"status"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, "run-detail", got.ID)
	assert.Equal(t, "org/repo", got.Repo)
	assert.Equal(t, "evals/test.yaml", got.EvalSpec)
	assert.Equal(t, "queued", got.Status)
}

func TestGetRunFromQueue_NotFound(t *testing.T) {
	mux, ap, _ := setupTestRouter()
	user := &auth.User{GitHubID: 100, Login: "alice"}
	addTestUser(ap, "tok", user)

	req := httptest.NewRequest(http.MethodGet, "/api/runs/queue/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestGetRunFromQueue_UserIsolation(t *testing.T) {
	mux, ap, store := setupTestRouter()

	userA := &auth.User{GitHubID: 100, Login: "alice"}
	userB := &auth.User{GitHubID: 200, Login: "bob"}
	addTestUser(ap, "tok-a", userA)
	addTestUser(ap, "tok-b", userB)

	store.runs["run-a-only"] = &db.RunRequest{ID: "run-a-only", UserID: 100, Repo: "a/repo", Status: db.Queued}

	// User B cannot see user A's run
	req := httptest.NewRequest(http.MethodGet, "/api/runs/queue/run-a-only", nil)
	req.Header.Set("Authorization", "Bearer tok-b")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// ---------------------------------------------------------------------------
// Tests – Multi-model batch trigger
// ---------------------------------------------------------------------------

func TestRunTrigger_MultiModel_Batch(t *testing.T) {
	mux, ap, store := setupTestRouter()
	user := &auth.User{GitHubID: 100, Login: "alice"}
	addTestUser(ap, "tok", user)

	body, _ := json.Marshal(map[string]any{
		"repo":      "testorg/testrepo",
		"eval_spec": "evals/test.yaml",
		"models":    []string{"gpt-4o", "claude-sonnet-4", "o3-mini"},
		"workers":   2,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/runs/trigger", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusAccepted, rec.Code)

	var resp struct {
		BatchID string   `json:"batchId"`
		RunIDs  []string `json:"runIds"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.BatchID, "batch response must include batchId")
	assert.Len(t, resp.RunIDs, 3, "should create one run per model")

	// All runs should be in the store with the same BatchID
	runs, _ := store.ListRunRequests(nil, 100, 0)
	assert.Len(t, runs, 3)

	models := make(map[string]bool)
	for _, r := range runs {
		assert.Equal(t, resp.BatchID, r.BatchID)
		assert.Equal(t, "testorg/testrepo", r.Repo)
		assert.Equal(t, "evals/test.yaml", r.EvalSpec)
		assert.Equal(t, 2, r.Workers)
		models[r.Model] = true
	}
	assert.True(t, models["gpt-4o"])
	assert.True(t, models["claude-sonnet-4"])
	assert.True(t, models["o3-mini"])
}

func TestRunTrigger_MultiModel_EmptyModelsArray(t *testing.T) {
	mux, ap, _ := setupTestRouter()
	user := &auth.User{GitHubID: 100, Login: "alice"}
	addTestUser(ap, "tok", user)

	// Empty models array should fall through to single-model path with default
	body, _ := json.Marshal(map[string]any{
		"repo":      "testorg/testrepo",
		"eval_spec": "evals/test.yaml",
		"models":    []string{},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/runs/trigger", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusAccepted, rec.Code)

	// Should get single-run response (backward compat)
	var resp struct {
		RunID  string `json:"runId"`
		Status string `json:"status"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.RunID)
	assert.Equal(t, "queued", resp.Status)
}

func TestRunTrigger_SingleModel_BackwardCompat(t *testing.T) {
	mux, ap, store := setupTestRouter()
	user := &auth.User{GitHubID: 100, Login: "alice"}
	addTestUser(ap, "tok", user)

	// Old-style single model request — must still work
	body, _ := json.Marshal(map[string]string{
		"repo":      "testorg/testrepo",
		"eval_spec": "evals/test.yaml",
		"model":     "gpt-4o",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/runs/trigger", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusAccepted, rec.Code)

	var resp struct {
		RunID  string `json:"runId"`
		Status string `json:"status"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.RunID)
	assert.Equal(t, "queued", resp.Status)

	runs, _ := store.ListRunRequests(nil, 100, 0)
	assert.Len(t, runs, 1)
	assert.Empty(t, runs[0].BatchID, "single-model run should have no BatchID")
}

func TestRunTrigger_MultiModel_AllEmptyStrings(t *testing.T) {
	mux, ap, _ := setupTestRouter()
	user := &auth.User{GitHubID: 100, Login: "alice"}
	addTestUser(ap, "tok", user)

	body, _ := json.Marshal(map[string]any{
		"repo":      "testorg/testrepo",
		"eval_spec": "evals/test.yaml",
		"models":    []string{"", ""},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/runs/trigger", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// ---------------------------------------------------------------------------
// Tests – Batch status endpoint
// ---------------------------------------------------------------------------

func TestGetBatchRuns_ReturnsGroupedRuns(t *testing.T) {
	mux, ap, store := setupTestRouter()
	user := &auth.User{GitHubID: 100, Login: "alice"}
	addTestUser(ap, "tok", user)

	// Seed runs with a common BatchID
	store.runs["run-a"] = &db.RunRequest{
		ID: "run-a", UserID: 100, BatchID: "batch-123",
		Repo: "org/repo", Model: "gpt-4o", Status: db.Complete,
	}
	store.runs["run-b"] = &db.RunRequest{
		ID: "run-b", UserID: 100, BatchID: "batch-123",
		Repo: "org/repo", Model: "claude-sonnet-4", Status: db.Running,
	}
	store.runs["run-c"] = &db.RunRequest{
		ID: "run-c", UserID: 100, BatchID: "batch-other",
		Repo: "org/repo", Model: "o3-mini", Status: db.Queued,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/runs/batch/batch-123", nil)
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		BatchID string         `json:"batchId"`
		Runs    []runQueueItem `json:"runs"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "batch-123", resp.BatchID)
	assert.Len(t, resp.Runs, 2, "should only return runs in the requested batch")

	ids := map[string]bool{}
	for _, r := range resp.Runs {
		ids[r.ID] = true
		assert.Equal(t, "batch-123", r.BatchID)
	}
	assert.True(t, ids["run-a"])
	assert.True(t, ids["run-b"])
}

func TestGetBatchRuns_NotFound(t *testing.T) {
	mux, ap, _ := setupTestRouter()
	user := &auth.User{GitHubID: 100, Login: "alice"}
	addTestUser(ap, "tok", user)

	req := httptest.NewRequest(http.MethodGet, "/api/runs/batch/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestGetBatchRuns_UserIsolation(t *testing.T) {
	mux, ap, store := setupTestRouter()

	userA := &auth.User{GitHubID: 100, Login: "alice"}
	userB := &auth.User{GitHubID: 200, Login: "bob"}
	addTestUser(ap, "tok-a", userA)
	addTestUser(ap, "tok-b", userB)

	store.runs["run-a"] = &db.RunRequest{
		ID: "run-a", UserID: 100, BatchID: "batch-private",
		Repo: "org/repo", Model: "gpt-4o", Status: db.Queued,
	}

	// User B should not see user A's batch
	req := httptest.NewRequest(http.MethodGet, "/api/runs/batch/batch-private", nil)
	req.Header.Set("Authorization", "Bearer tok-b")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// ---------------------------------------------------------------------------
// Tests – BatchID in queue items
// ---------------------------------------------------------------------------

func TestRunQueueItem_IncludesBatchID(t *testing.T) {
	mux, ap, store := setupTestRouter()
	user := &auth.User{GitHubID: 100, Login: "alice"}
	addTestUser(ap, "tok", user)

	store.runs["run-batch"] = &db.RunRequest{
		ID: "run-batch", UserID: 100, BatchID: "batch-456",
		Repo: "org/repo", Model: "gpt-4o", Status: db.Queued,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/runs/queue/run-batch", nil)
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var got struct {
		ID      string `json:"id"`
		BatchID string `json:"batchId"`
		Model   string `json:"model"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, "run-batch", got.ID)
	assert.Equal(t, "batch-456", got.BatchID)
}
