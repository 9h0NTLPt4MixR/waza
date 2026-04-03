package db

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/microsoft/waza/internal/platform/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// In-memory mock store — satisfies db.Store without a real Cosmos DB.
// ---------------------------------------------------------------------------

var errNotFound = errors.New("not found")

type mockStore struct {
	mu          sync.RWMutex
	users       map[int64]*auth.User
	connections map[string]*Connection // keyed by ID
	runs        map[string]*RunRequest
	settings    map[string]string
}

func newMockStore() *mockStore {
	return &mockStore{
		users:       make(map[int64]*auth.User),
		connections: make(map[string]*Connection),
		runs:        make(map[string]*RunRequest),
		settings:    make(map[string]string),
	}
}

func (m *mockStore) CreateUser(_ context.Context, user *auth.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if user.CreatedAt.IsZero() {
		user.CreatedAt = time.Now().UTC()
	}
	m.users[user.GitHubID] = user
	return nil
}

func (m *mockStore) GetUser(_ context.Context, githubID int64) (*auth.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	u, ok := m.users[githubID]
	if !ok {
		return nil, nil // matches Cosmos contract: nil, nil for not found
	}
	return u, nil
}

func (m *mockStore) SaveConnection(_ context.Context, conn *Connection) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connections[conn.ID] = conn
	return nil
}

func (m *mockStore) ListConnections(_ context.Context, userID int64, connType ConnectionType) ([]*Connection, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*Connection
	for _, c := range m.connections {
		if c.UserID != userID {
			continue
		}
		if connType != "" && c.Type != connType {
			continue
		}
		result = append(result, c)
	}
	return result, nil
}

func (m *mockStore) DeleteConnection(_ context.Context, userID int64, connectionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.connections[connectionID]
	if !ok || c.UserID != userID {
		return errNotFound
	}
	delete(m.connections, connectionID)
	return nil
}

func (m *mockStore) CreateRunRequest(_ context.Context, run *RunRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	run.CreatedAt = time.Now().UTC()
	run.Status = Queued
	cp := *run
	m.runs[run.ID] = &cp
	return nil
}

func (m *mockStore) UpdateRunRequest(_ context.Context, run *RunRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.runs[run.ID]
	if !ok {
		return errNotFound
	}
	if existing.Status.Terminal() {
		return errors.New("cannot update terminal run")
	}
	cp := *run
	m.runs[run.ID] = &cp
	return nil
}

func (m *mockStore) ListRunRequests(_ context.Context, userID int64, limit int) ([]*RunRequest, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*RunRequest
	for _, r := range m.runs {
		if r.UserID != userID {
			continue
		}
		result = append(result, r)
		if limit > 0 && len(result) >= limit {
			break
		}
	}
	return result, nil
}

func (m *mockStore) GetRunRequest(_ context.Context, userID int64, runID string) (*RunRequest, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.runs[runID]
	if !ok || r.UserID != userID {
		return nil, errNotFound
	}
	return r, nil
}

func (m *mockStore) GetSetting(_ context.Context, key string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.settings[key], nil
}

func (m *mockStore) SetSetting(_ context.Context, key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settings[key] = value
	return nil
}

func (m *mockStore) Close() error                                          { return nil }
func (m *mockStore) RecoverOrphanedRuns(_ context.Context) (int, error)     { return 0, nil }

func (m *mockStore) SaveResult(_ context.Context, _ int64, _ string, _ json.RawMessage) error {
	return nil
}

func (m *mockStore) GetResult(_ context.Context, _ int64, _ string) (json.RawMessage, error) {
	return nil, errNotFound
}

func (m *mockStore) ListResults(_ context.Context, _ int64, _ int) ([]ResultSummary, error) {
	return nil, nil
}

// Compile-time check
var _ Store = (*mockStore)(nil)

// ---------------------------------------------------------------------------
// Tests – User CRUD
// ---------------------------------------------------------------------------

func TestCreateUser(t *testing.T) {
	store := newMockStore()
	ctx := context.Background()

	user := &auth.User{
		GitHubID:  12345,
		Login:     "testuser",
		Name:      "Test User",
	}

	err := store.CreateUser(ctx, user)
	require.NoError(t, err)
	assert.Equal(t, int64(12345), user.GitHubID)
	assert.Equal(t, "testuser", user.Login)
	assert.False(t, user.CreatedAt.IsZero(), "CreatedAt must be set")

	got, err := store.GetUser(ctx, 12345)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, user.Login, got.Login)
}

func TestGetUser_Exists(t *testing.T) {
	store := newMockStore()
	ctx := context.Background()

	_ = store.CreateUser(ctx, &auth.User{
		GitHubID: 99999,
		Login:    "existinguser",
		Name:     "Existing",
	})

	got, err := store.GetUser(ctx, 99999)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "existinguser", got.Login)
	assert.Equal(t, int64(99999), got.GitHubID)
}

func TestGetUser_NotFound(t *testing.T) {
	store := newMockStore()
	ctx := context.Background()

	got, err := store.GetUser(ctx, 00000)
	assert.NoError(t, err, "not found should return nil, nil per Cosmos contract")
	assert.Nil(t, got)
}

// ---------------------------------------------------------------------------
// Tests – Connection CRUD
// ---------------------------------------------------------------------------

func TestSaveConnection_New(t *testing.T) {
	store := newMockStore()
	ctx := context.Background()

	conn := &Connection{
		ID:     "conn-1",
		UserID: 12345,
		Type:   AzureStorage,
		Config: map[string]any{"account": "myaccount"},
	}

	err := store.SaveConnection(ctx, conn)
	require.NoError(t, err)

	conns, err := store.ListConnections(ctx, 12345, "")
	require.NoError(t, err)
	assert.Len(t, conns, 1)
	assert.Equal(t, "conn-1", conns[0].ID)
	assert.Equal(t, AzureStorage, conns[0].Type)
}

func TestSaveConnection_Update(t *testing.T) {
	store := newMockStore()
	ctx := context.Background()

	conn := &Connection{
		ID:     "conn-1",
		UserID: 12345,
		Type:   AzureStorage,
		Config: map[string]any{"key": "old"},
	}
	_ = store.SaveConnection(ctx, conn)

	conn.Config = map[string]any{"key": "new"}
	err := store.SaveConnection(ctx, conn)
	require.NoError(t, err)

	conns, err := store.ListConnections(ctx, 12345, "")
	require.NoError(t, err)
	require.Len(t, conns, 1)
	assert.Equal(t, "new", conns[0].Config["key"])
}

func TestListConnections(t *testing.T) {
	store := newMockStore()
	ctx := context.Background()

	// User A connections
	_ = store.SaveConnection(ctx, &Connection{ID: "a1", UserID: 100, Type: AzureStorage})
	_ = store.SaveConnection(ctx, &Connection{ID: "a2", UserID: 100, Type: GitHubRepo})
	// User B connection
	_ = store.SaveConnection(ctx, &Connection{ID: "b1", UserID: 200, Type: AzureStorage})

	// User A — all types
	conns, err := store.ListConnections(ctx, 100, "")
	require.NoError(t, err)
	assert.Len(t, conns, 2, "user A should see 2 connections")
	for _, c := range conns {
		assert.Equal(t, int64(100), c.UserID)
	}

	// User A — filtered by type
	conns, err = store.ListConnections(ctx, 100, AzureStorage)
	require.NoError(t, err)
	assert.Len(t, conns, 1)

	// User B — isolation check
	conns, err = store.ListConnections(ctx, 200, "")
	require.NoError(t, err)
	assert.Len(t, conns, 1)

	// Unknown user — empty, no error
	conns, err = store.ListConnections(ctx, 999, "")
	require.NoError(t, err)
	assert.Empty(t, conns)
}

func TestDeleteConnection(t *testing.T) {
	store := newMockStore()
	ctx := context.Background()

	_ = store.SaveConnection(ctx, &Connection{ID: "del", UserID: 100, Type: AzureStorage})

	err := store.DeleteConnection(ctx, 100, "del")
	require.NoError(t, err)

	conns, _ := store.ListConnections(ctx, 100, "")
	assert.Empty(t, conns, "connection should be gone")

	// Double-delete
	err = store.DeleteConnection(ctx, 100, "del")
	assert.ErrorIs(t, err, errNotFound)
}

func TestDeleteConnection_WrongUser(t *testing.T) {
	store := newMockStore()
	ctx := context.Background()

	_ = store.SaveConnection(ctx, &Connection{ID: "x", UserID: 100, Type: AzureStorage})

	err := store.DeleteConnection(ctx, 200, "x")
	assert.ErrorIs(t, err, errNotFound, "should not allow cross-user deletion")
}

// ---------------------------------------------------------------------------
// Tests – Connection model helpers
// ---------------------------------------------------------------------------

func TestConnection_Verified(t *testing.T) {
	now := time.Now()
	assert.True(t, (&Connection{VerifiedAt: &now}).Verified())
	assert.False(t, (&Connection{}).Verified())
}

// ---------------------------------------------------------------------------
// Tests – RunRequest CRUD
// ---------------------------------------------------------------------------

func TestCreateRunRequest(t *testing.T) {
	store := newMockStore()
	ctx := context.Background()

	run := &RunRequest{
		ID:       "run-1",
		UserID:   100,
		Repo:     "testorg/testrepo",
		EvalSpec: "evals/code-review.yaml",
		Model:    "gpt-4o",
		Workers:  2,
	}

	err := store.CreateRunRequest(ctx, run)
	require.NoError(t, err)
	assert.Equal(t, Queued, run.Status, "new run must be Queued")
	assert.False(t, run.CreatedAt.IsZero())
}

func TestUpdateRunRequest_Status(t *testing.T) {
	store := newMockStore()
	ctx := context.Background()

	run := &RunRequest{ID: "run-s", UserID: 100, Repo: "o/r", EvalSpec: "e.yaml"}
	_ = store.CreateRunRequest(ctx, run)

	// queued → running
	run.Status = Running
	run.ADCSandboxIDs = []string{"sb-1"}
	err := store.UpdateRunRequest(ctx, run)
	require.NoError(t, err)

	got, _ := store.GetRunRequest(ctx, 100, "run-s")
	assert.Equal(t, Running, got.Status)
}

func TestUpdateRunRequest_StatusToComplete(t *testing.T) {
	store := newMockStore()
	ctx := context.Background()

	run := &RunRequest{ID: "run-c", UserID: 100, Repo: "o/r", EvalSpec: "e.yaml"}
	_ = store.CreateRunRequest(ctx, run)

	// queued → running first
	run.Status = Running
	err := store.UpdateRunRequest(ctx, run)
	require.NoError(t, err)

	// running → complete
	now := time.Now()
	run.Status = Complete
	run.CompletedAt = &now
	err = store.UpdateRunRequest(ctx, run)
	require.NoError(t, err)

	got, _ := store.GetRunRequest(ctx, 100, "run-c")
	assert.Equal(t, Complete, got.Status)
	assert.NotNil(t, got.CompletedAt)
}

func TestUpdateRunRequest_TerminalStateBlocked(t *testing.T) {
	store := newMockStore()
	ctx := context.Background()

	run := &RunRequest{ID: "run-t", UserID: 100}
	_ = store.CreateRunRequest(ctx, run)

	// queued → running (non-terminal, should succeed)
	run.Status = Running
	_ = store.UpdateRunRequest(ctx, run)

	// running → complete (terminal)
	run.Status = Complete
	_ = store.UpdateRunRequest(ctx, run)

	// complete → running (should fail: terminal state)
	run.Status = Running
	err := store.UpdateRunRequest(ctx, run)
	assert.Error(t, err, "should not allow updating terminal run")
}

func TestListRunRequests_FilterByUser(t *testing.T) {
	store := newMockStore()
	ctx := context.Background()

	_ = store.CreateRunRequest(ctx, &RunRequest{ID: "r1", UserID: 100})
	_ = store.CreateRunRequest(ctx, &RunRequest{ID: "r2", UserID: 100})
	_ = store.CreateRunRequest(ctx, &RunRequest{ID: "r3", UserID: 200})

	runs, err := store.ListRunRequests(ctx, 100, 0)
	require.NoError(t, err)
	assert.Len(t, runs, 2)
	for _, r := range runs {
		assert.Equal(t, int64(100), r.UserID)
	}

	// User isolation
	runs, err = store.ListRunRequests(ctx, 200, 0)
	require.NoError(t, err)
	assert.Len(t, runs, 1)

	// With limit
	runs, err = store.ListRunRequests(ctx, 100, 1)
	require.NoError(t, err)
	assert.Len(t, runs, 1)
}

func TestRunStatus_Terminal(t *testing.T) {
	tests := []struct {
		status   RunStatus
		terminal bool
	}{
		{Queued, false},
		{Running, false},
		{Complete, true},
		{Failed, true},
		{Cancelled, true},
	}
	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			assert.Equal(t, tt.terminal, tt.status.Terminal())
		})
	}
}

// ---------------------------------------------------------------------------
// Tests – Settings
// ---------------------------------------------------------------------------

func TestSettings_GetSet(t *testing.T) {
	store := newMockStore()
	ctx := context.Background()

	// Get non-existent → empty string
	val, err := store.GetSetting(ctx, "missing")
	require.NoError(t, err)
	assert.Empty(t, val)

	// Set and get
	err = store.SetSetting(ctx, "feature_flag", "enabled")
	require.NoError(t, err)

	val, err = store.GetSetting(ctx, "feature_flag")
	require.NoError(t, err)
	assert.Equal(t, "enabled", val)
}

// ---------------------------------------------------------------------------
// Tests – Encryption (integration, requires Cosmos DB)
// ---------------------------------------------------------------------------

func TestConnectionEncryption(t *testing.T) {
	t.Skip("requires Cosmos DB — integration test for verifying at-rest encryption")
}

// ---------------------------------------------------------------------------
// Tests – edge cases
// ---------------------------------------------------------------------------

func TestUpdateRunRequest_NotFound(t *testing.T) {
	store := newMockStore()
	ctx := context.Background()

	err := store.UpdateRunRequest(ctx, &RunRequest{ID: "nonexistent", Status: Running})
	assert.ErrorIs(t, err, errNotFound)
}

func TestGetRunRequest_WrongUser(t *testing.T) {
	store := newMockStore()
	ctx := context.Background()

	_ = store.CreateRunRequest(ctx, &RunRequest{ID: "r1", UserID: 100})

	got, err := store.GetRunRequest(ctx, 200, "r1")
	assert.Nil(t, got)
	assert.ErrorIs(t, err, errNotFound, "wrong user should not see the run")
}
