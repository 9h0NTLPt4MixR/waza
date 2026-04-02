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
// Integration test helpers — uses httptest.Server with the real router
// ---------------------------------------------------------------------------

// setupIntegrationServer creates an httptest.Server wired through RegisterRoutes
// and returns the server, a mock auth provider, and a mock store.
func setupIntegrationServer(t *testing.T) (*httptest.Server, *mockAuthProvider, *mockStore) {
	t.Helper()
	mux, ap, store := setupTestRouter()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, ap, store
}

// doRequest is a convenience wrapper for making HTTP requests to the integration server.
func doRequest(t *testing.T, srv *httptest.Server, method, path, token string, body any) *http.Response {
	t.Helper()
	var bodyReader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		bodyReader = bytes.NewReader(b)
	} else {
		bodyReader = bytes.NewReader(nil)
	}

	req, err := http.NewRequestWithContext(context.Background(), method, srv.URL+path, bodyReader)
	require.NoError(t, err)

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

// decodeJSON decodes resp.Body into v and closes it.
func decodeJSON(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	defer resp.Body.Close()
	require.NoError(t, json.NewDecoder(resp.Body).Decode(v))
}

// ===========================================================================
// Test: Full HTTP request/response cycle through httptest.Server
// ===========================================================================

func TestIntegration_FullHTTPCycle_Me(t *testing.T) {
	srv, ap, _ := setupIntegrationServer(t)

	user := &auth.User{GitHubID: 42, Login: "integration-user", Name: "Int Test"}
	addTestUser(ap, "int-tok", user)

	resp := doRequest(t, srv, http.MethodGet, "/api/auth/me", "int-tok", nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// handleMe returns camelCase JSON matching the frontend User interface.
	var got map[string]any
	decodeJSON(t, resp, &got)
	assert.Equal(t, "integration-user", got["login"])
	assert.Equal(t, float64(42), got["githubId"])
}

func TestIntegration_FullHTTPCycle_ListRunsEmpty(t *testing.T) {
	srv, ap, _ := setupIntegrationServer(t)

	user := &auth.User{GitHubID: 42, Login: "alice"}
	addTestUser(ap, "tok", user)

	resp := doRequest(t, srv, http.MethodGet, "/api/runs/queue", "tok", nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var runs []runQueueItem
	decodeJSON(t, resp, &runs)
	assert.Empty(t, runs)
}

func TestIntegration_FullHTTPCycle_ListConnectionsEmpty(t *testing.T) {
	srv, ap, _ := setupIntegrationServer(t)

	user := &auth.User{GitHubID: 42, Login: "alice"}
	addTestUser(ap, "tok", user)

	resp := doRequest(t, srv, http.MethodGet, "/api/connections", "tok", nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var conns []db.Connection
	decodeJSON(t, resp, &conns)
	assert.Empty(t, conns)
}

// ===========================================================================
// Test: Connection CRUD flow end-to-end (create → list → test → delete)
// ===========================================================================

func TestIntegration_ConnectionCRUD_EndToEnd(t *testing.T) {
	srv, ap, _ := setupIntegrationServer(t)

	user := &auth.User{GitHubID: 100, Login: "crud-user"}
	addTestUser(ap, "crud-tok", user)

	// --- Step 1: Create connection ---
	createBody := map[string]any{
		"type":   "azure-storage",
		"config": map[string]any{"account_name": "myaccount", "container_name": "results"},
	}
	resp := doRequest(t, srv, http.MethodPost, "/api/connections", "crud-tok", createBody)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var created db.Connection
	decodeJSON(t, resp, &created)
	assert.Equal(t, db.AzureStorage, created.Type)
	assert.Equal(t, int64(100), created.UserID)
	assert.NotEmpty(t, created.ID)
	connID := created.ID

	// --- Step 2: List — should have one connection ---
	resp = doRequest(t, srv, http.MethodGet, "/api/connections", "crud-tok", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var conns []db.Connection
	decodeJSON(t, resp, &conns)
	require.Len(t, conns, 1)
	assert.Equal(t, connID, conns[0].ID)

	// --- Step 3: Test connection (validates request, even though real Azure isn't reachable) ---
	testBody := map[string]any{
		"type":   "azure-storage",
		"config": map[string]any{"account_name": "myaccount", "container_name": "results"},
	}
	resp = doRequest(t, srv, http.MethodPost, "/api/connections/test", "crud-tok", testBody)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var testResult testConnectionResponse
	decodeJSON(t, resp, &testResult)
	// We don't assert OK=true because we're not hitting real Azure; we just check the
	// handler accepted the request and returned a valid response shape.
	assert.NotEmpty(t, testResult.Message)

	// --- Step 4: Delete ---
	resp = doRequest(t, srv, http.MethodDelete, "/api/connections/"+connID, "crud-tok", nil)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	// --- Step 5: Verify list is now empty ---
	resp = doRequest(t, srv, http.MethodGet, "/api/connections", "crud-tok", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var after []db.Connection
	decodeJSON(t, resp, &after)
	assert.Empty(t, after)
}

func TestIntegration_CreateConnection_InvalidType(t *testing.T) {
	srv, ap, _ := setupIntegrationServer(t)

	user := &auth.User{GitHubID: 100, Login: "alice"}
	addTestUser(ap, "tok", user)

	body := map[string]any{
		"type":   "unknown_type",
		"config": map[string]any{"key": "val"},
	}
	resp := doRequest(t, srv, http.MethodPost, "/api/connections", "tok", body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()
}

func TestIntegration_CreateConnection_EmptyConfig(t *testing.T) {
	srv, ap, _ := setupIntegrationServer(t)

	user := &auth.User{GitHubID: 100, Login: "alice"}
	addTestUser(ap, "tok", user)

	body := map[string]any{
		"type":   "azure_storage",
		"config": map[string]any{},
	}
	resp := doRequest(t, srv, http.MethodPost, "/api/connections", "tok", body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()
}

func TestIntegration_TestConnection_MissingFields(t *testing.T) {
	srv, ap, _ := setupIntegrationServer(t)

	user := &auth.User{GitHubID: 100, Login: "alice"}
	addTestUser(ap, "tok", user)

	// Missing account_name
	body := map[string]any{
		"type":   "azure-storage",
		"config": map[string]any{"container_name": "results"},
	}
	resp := doRequest(t, srv, http.MethodPost, "/api/connections/test", "tok", body)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result testConnectionResponse
	decodeJSON(t, resp, &result)
	assert.False(t, result.OK)
	assert.Contains(t, result.Message, "account_name")
}

func TestIntegration_TestConnection_GitHubRepo_MissingFields(t *testing.T) {
	srv, ap, _ := setupIntegrationServer(t)

	user := &auth.User{GitHubID: 100, Login: "alice"}
	addTestUser(ap, "tok", user)

	body := map[string]any{
		"type":   "github-repo",
		"config": map[string]any{"owner": "myorg"},
	}
	resp := doRequest(t, srv, http.MethodPost, "/api/connections/test", "tok", body)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result testConnectionResponse
	decodeJSON(t, resp, &result)
	assert.False(t, result.OK)
	assert.Contains(t, result.Message, "repo")
}

func TestIntegration_TestConnection_UnsupportedType(t *testing.T) {
	srv, ap, _ := setupIntegrationServer(t)

	user := &auth.User{GitHubID: 100, Login: "alice"}
	addTestUser(ap, "tok", user)

	body := map[string]any{
		"type":   "redis",
		"config": map[string]any{"host": "localhost"},
	}
	resp := doRequest(t, srv, http.MethodPost, "/api/connections/test", "tok", body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()
}

// ===========================================================================
// Test: Run trigger → queue → cancel flow
// ===========================================================================

func TestIntegration_RunLifecycle_TriggerQueueCancel(t *testing.T) {
	srv, ap, _ := setupIntegrationServer(t)

	user := &auth.User{GitHubID: 200, Login: "runner"}
	addTestUser(ap, "run-tok", user)

	// --- Step 1: Trigger a run ---
	triggerBody := map[string]string{
		"repo":      "myorg/myrepo",
		"eval_spec": "evals/test.yaml",
		"model":     "gpt-4o",
	}
	resp := doRequest(t, srv, http.MethodPost, "/api/runs/trigger", "run-tok", triggerBody)
	require.Equal(t, http.StatusAccepted, resp.StatusCode)

	var triggerResp triggerRunResponse
	decodeJSON(t, resp, &triggerResp)
	assert.NotEmpty(t, triggerResp.RunID, "response must include runId")
	assert.Equal(t, db.Queued, triggerResp.Status)
	runID := triggerResp.RunID

	// --- Step 2: List queue — should contain the run ---
	resp = doRequest(t, srv, http.MethodGet, "/api/runs/queue", "run-tok", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var runs []runQueueItem
	decodeJSON(t, resp, &runs)
	require.Len(t, runs, 1)
	assert.Equal(t, runID, runs[0].ID)

	// --- Step 3: Get run by ID ---
	resp = doRequest(t, srv, http.MethodGet, "/api/runs/queue/"+runID, "run-tok", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var detail runQueueItem
	decodeJSON(t, resp, &detail)
	assert.Equal(t, runID, detail.ID)

	// --- Step 4: Cancel the run ---
	resp = doRequest(t, srv, http.MethodPost, "/api/runs/cancel/"+runID, "run-tok", nil)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	// --- Step 5: Verify cancelled status ---
	resp = doRequest(t, srv, http.MethodGet, "/api/runs/queue/"+runID, "run-tok", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var cancelled runQueueItem
	decodeJSON(t, resp, &cancelled)
	assert.Equal(t, db.Cancelled, cancelled.Status)
	assert.NotNil(t, cancelled.CompletedAt)
}

func TestIntegration_RunTrigger_DefaultsApplied(t *testing.T) {
	srv, ap, _ := setupIntegrationServer(t)

	user := &auth.User{GitHubID: 100, Login: "alice"}
	addTestUser(ap, "tok", user)

	// Trigger with no model or workers — defaults should apply
	body := map[string]string{
		"repo":      "org/repo",
		"eval_spec": "evals/test.yaml",
	}
	resp := doRequest(t, srv, http.MethodPost, "/api/runs/trigger", "tok", body)
	require.Equal(t, http.StatusAccepted, resp.StatusCode)

	var trigResp triggerRunResponse
	decodeJSON(t, resp, &trigResp)
	assert.NotEmpty(t, trigResp.RunID, "response must include runId")

	// Verify defaults were applied by fetching the run from the queue
	resp = doRequest(t, srv, http.MethodGet, "/api/runs/queue/"+trigResp.RunID, "tok", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var run runQueueItem
	decodeJSON(t, resp, &run)
	assert.Equal(t, "gpt-4o", run.Model, "default model should be gpt-4o")
	assert.Equal(t, 1, run.Workers, "default workers should be 1")
}

func TestIntegration_RunCancel_AlreadyTerminal(t *testing.T) {
	srv, ap, store := setupIntegrationServer(t)

	user := &auth.User{GitHubID: 100, Login: "alice"}
	addTestUser(ap, "tok", user)

	now := time.Now().UTC()
	store.runs["done-run"] = &db.RunRequest{
		ID:          "done-run",
		UserID:      100,
		Repo:        "org/repo",
		Status:      db.Complete,
		CompletedAt: &now,
	}

	resp := doRequest(t, srv, http.MethodPost, "/api/runs/cancel/done-run", "tok", nil)
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
	resp.Body.Close()
}

func TestIntegration_GetRun_NotFound(t *testing.T) {
	srv, ap, _ := setupIntegrationServer(t)

	user := &auth.User{GitHubID: 100, Login: "alice"}
	addTestUser(ap, "tok", user)

	resp := doRequest(t, srv, http.MethodGet, "/api/runs/queue/nonexistent", "tok", nil)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// ===========================================================================
// Test: User isolation — user A can't access user B's data
// ===========================================================================

func TestIntegration_UserIsolation_Connections(t *testing.T) {
	srv, ap, store := setupIntegrationServer(t)

	userA := &auth.User{GitHubID: 100, Login: "alice"}
	userB := &auth.User{GitHubID: 200, Login: "bob"}
	addTestUser(ap, "tok-a", userA)
	addTestUser(ap, "tok-b", userB)

	// Pre-populate connections
	store.connections["conn-a"] = &db.Connection{ID: "conn-a", UserID: 100, Type: db.AzureStorage, Config: map[string]any{"account_name": "alice-storage"}}
	store.connections["conn-b"] = &db.Connection{ID: "conn-b", UserID: 200, Type: db.AzureStorage, Config: map[string]any{"account_name": "bob-storage"}}

	// Alice lists connections — sees only hers
	resp := doRequest(t, srv, http.MethodGet, "/api/connections", "tok-a", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var connsA []db.Connection
	decodeJSON(t, resp, &connsA)
	require.Len(t, connsA, 1)
	assert.Equal(t, "conn-a", connsA[0].ID)

	// Bob lists connections — sees only his
	resp = doRequest(t, srv, http.MethodGet, "/api/connections", "tok-b", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var connsB []db.Connection
	decodeJSON(t, resp, &connsB)
	require.Len(t, connsB, 1)
	assert.Equal(t, "conn-b", connsB[0].ID)

	// Alice cannot delete Bob's connection
	resp = doRequest(t, srv, http.MethodDelete, "/api/connections/conn-b", "tok-a", nil)
	// The mock returns error because conn-b belongs to user 200 not 100
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	resp.Body.Close()

	// Bob's connection still exists
	resp = doRequest(t, srv, http.MethodGet, "/api/connections", "tok-b", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var stillThere []db.Connection
	decodeJSON(t, resp, &stillThere)
	assert.Len(t, stillThere, 1)
}

func TestIntegration_UserIsolation_Runs(t *testing.T) {
	srv, ap, store := setupIntegrationServer(t)

	userA := &auth.User{GitHubID: 100, Login: "alice"}
	userB := &auth.User{GitHubID: 200, Login: "bob"}
	addTestUser(ap, "tok-a", userA)
	addTestUser(ap, "tok-b", userB)

	store.runs["run-a"] = &db.RunRequest{ID: "run-a", UserID: 100, Repo: "a/repo", Status: db.Queued}
	store.runs["run-b"] = &db.RunRequest{ID: "run-b", UserID: 200, Repo: "b/repo", Status: db.Running}

	// Alice sees only her runs
	resp := doRequest(t, srv, http.MethodGet, "/api/runs/queue", "tok-a", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var runsA []runQueueItem
	decodeJSON(t, resp, &runsA)
	require.Len(t, runsA, 1)
	assert.Equal(t, "run-a", runsA[0].ID)

	// Bob sees only his runs
	resp = doRequest(t, srv, http.MethodGet, "/api/runs/queue", "tok-b", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var runsB []runQueueItem
	decodeJSON(t, resp, &runsB)
	require.Len(t, runsB, 1)
	assert.Equal(t, "run-b", runsB[0].ID)

	// Alice cannot access Bob's run by ID
	resp = doRequest(t, srv, http.MethodGet, "/api/runs/queue/run-b", "tok-a", nil)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()

	// Alice cannot cancel Bob's run
	resp = doRequest(t, srv, http.MethodPost, "/api/runs/cancel/run-b", "tok-a", nil)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}

// ===========================================================================
// Test: Auth middleware rejection on all protected routes
// ===========================================================================

func TestIntegration_AuthMiddleware_AllProtectedRoutes(t *testing.T) {
	srv, _, _ := setupIntegrationServer(t)

	protectedEndpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/auth/me"},
		{http.MethodPost, "/api/auth/logout"},
		{http.MethodGet, "/api/connections"},
		{http.MethodPost, "/api/connections"},
		{http.MethodDelete, "/api/connections/some-id"},
		{http.MethodPost, "/api/connections/test"},
		{http.MethodPost, "/api/runs/trigger"},
		{http.MethodGet, "/api/runs/queue"},
		{http.MethodGet, "/api/runs/queue/some-id"},
		{http.MethodPost, "/api/runs/cancel/some-id"},
		{http.MethodGet, "/api/repos"},
	}

	for _, ep := range protectedEndpoints {
		t.Run(ep.method+" "+ep.path+" no_token", func(t *testing.T) {
			resp := doRequest(t, srv, ep.method, ep.path, "", nil)
			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
				"%s %s should return 401 without token", ep.method, ep.path)
			resp.Body.Close()
		})

		t.Run(ep.method+" "+ep.path+" bad_token", func(t *testing.T) {
			resp := doRequest(t, srv, ep.method, ep.path, "invalid-garbage-token", nil)
			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
				"%s %s should return 401 with invalid token", ep.method, ep.path)
			resp.Body.Close()
		})
	}
}

func TestIntegration_AuthMiddleware_RevokedToken(t *testing.T) {
	srv, ap, _ := setupIntegrationServer(t)

	user := &auth.User{GitHubID: 100, Login: "alice"}
	addTestUser(ap, "tok-revokable", user)

	// Verify works before revocation
	resp := doRequest(t, srv, http.MethodGet, "/api/auth/me", "tok-revokable", nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Revoke session
	ap.revoked["tok-revokable"] = true

	// Verify rejected after revocation
	resp = doRequest(t, srv, http.MethodGet, "/api/auth/me", "tok-revokable", nil)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()
}

func TestIntegration_AuthMiddleware_CookieAuth(t *testing.T) {
	mux, ap, _ := setupTestRouter()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	user := &auth.User{GitHubID: 42, Login: "cookie-user"}
	addTestUser(ap, "cookie-tok", user)

	// Use a cookie instead of Bearer header
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/api/auth/me", nil)
	require.NoError(t, err)
	req.AddCookie(&http.Cookie{Name: "waza_session", Value: "cookie-tok"})

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var got auth.User
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Equal(t, "cookie-user", got.Login)
}

// ===========================================================================
// Test: Public endpoints don't require auth
// ===========================================================================

func TestIntegration_PublicEndpoints_NoAuth(t *testing.T) {
	srv, _, _ := setupIntegrationServer(t)

	publicEndpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/auth/github"},
		{http.MethodGet, "/api/auth/callback"},
	}

	for _, ep := range publicEndpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			resp := doRequest(t, srv, ep.method, ep.path, "", nil)
			assert.NotEqual(t, http.StatusUnauthorized, resp.StatusCode,
				"%s %s must not require auth", ep.method, ep.path)
			resp.Body.Close()
		})
	}
}

// ===========================================================================
// Test: Logout clears session
// ===========================================================================

func TestIntegration_Logout_ClearsSession(t *testing.T) {
	srv, ap, _ := setupIntegrationServer(t)

	user := &auth.User{GitHubID: 100, Login: "alice"}
	addTestUser(ap, "session-tok", user)

	resp := doRequest(t, srv, http.MethodPost, "/api/auth/logout", "session-tok", nil)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	// Check Set-Cookie header clears the session
	cookies := resp.Cookies()
	var cleared bool
	for _, c := range cookies {
		if c.Name == "waza_session" && c.MaxAge < 0 {
			cleared = true
		}
	}
	assert.True(t, cleared, "logout should clear waza_session cookie")
	resp.Body.Close()
}

// ===========================================================================
// Test: Repos endpoint returns connected repos
// ===========================================================================

func TestIntegration_ListRepos(t *testing.T) {
	srv, ap, store := setupIntegrationServer(t)

	user := &auth.User{GitHubID: 100, Login: "alice"}
	addTestUser(ap, "tok", user)

	store.connections["gh-1"] = &db.Connection{
		ID:     "gh-1",
		UserID: 100,
		Type:   db.GitHubRepo,
		Config: map[string]any{"owner": "myorg", "repo": "myrepo", "description": "Test repo"},
	}

	resp := doRequest(t, srv, http.MethodGet, "/api/repos", "tok", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var repos []repoInfo
	decodeJSON(t, resp, &repos)
	require.Len(t, repos, 1)
	assert.Equal(t, "myorg/myrepo", repos[0].FullName)
	assert.Equal(t, "https://github.com/myorg/myrepo", repos[0].HTMLURL)
}

func TestIntegration_ListRepos_Empty(t *testing.T) {
	srv, ap, _ := setupIntegrationServer(t)

	user := &auth.User{GitHubID: 100, Login: "alice"}
	addTestUser(ap, "tok", user)

	resp := doRequest(t, srv, http.MethodGet, "/api/repos", "tok", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var repos []repoInfo
	decodeJSON(t, resp, &repos)
	assert.Empty(t, repos)
}

// ===========================================================================
// Test: Multiple runs trigger/list
// ===========================================================================

func TestIntegration_MultipleRuns(t *testing.T) {
	srv, ap, _ := setupIntegrationServer(t)

	user := &auth.User{GitHubID: 100, Login: "alice"}
	addTestUser(ap, "tok", user)

	// Trigger 3 runs
	for i := 0; i < 3; i++ {
		body := map[string]string{
			"repo":      "org/repo",
			"eval_spec": "evals/test.yaml",
			"model":     "gpt-4o",
		}
		resp := doRequest(t, srv, http.MethodPost, "/api/runs/trigger", "tok", body)
		require.Equal(t, http.StatusAccepted, resp.StatusCode)
		resp.Body.Close()
	}

	// List should return all 3
	resp := doRequest(t, srv, http.MethodGet, "/api/runs/queue", "tok", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var runs []runQueueItem
	decodeJSON(t, resp, &runs)
	assert.Len(t, runs, 3)
}

// ===========================================================================
// Test: Create GitHub repo connection
// ===========================================================================

func TestIntegration_CreateConnection_GitHubRepo(t *testing.T) {
	srv, ap, _ := setupIntegrationServer(t)

	user := &auth.User{GitHubID: 100, Login: "alice"}
	addTestUser(ap, "tok", user)

	body := map[string]any{
		"type":   "github-repo",
		"config": map[string]any{"owner": "myorg", "repo": "myrepo"},
	}
	resp := doRequest(t, srv, http.MethodPost, "/api/connections", "tok", body)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var conn db.Connection
	decodeJSON(t, resp, &conn)
	assert.Equal(t, db.GitHubRepo, conn.Type)
}

// ===========================================================================
// Test: Malformed JSON body
// ===========================================================================

func TestIntegration_MalformedJSON(t *testing.T) {
	mux, ap, _ := setupTestRouter()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	user := &auth.User{GitHubID: 100, Login: "alice"}
	addTestUser(ap, "tok", user)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL+"/api/runs/trigger",
		bytes.NewReader([]byte("not json at all")))
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}
