// Tests for the ADC execution engine.
//
// The adc package cannot currently compile because engine.go imports
// github.com/coreai-microsoft/adc-sdk-go which is not yet published.
// These tests are placed in an external test package (adc_test) and use
// a build constraint so they don't block `go vet ./...` until the SDK
// is available. Remove the build tag once the ADC SDK is in go.mod.
//
//go:build adcsdk

package adc_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Stub types – mirrors what the ADC engine exposes. These will be replaced
// by real imports once the ADC SDK is available.
// ---------------------------------------------------------------------------

type SandboxConfig struct {
	Image   string
	Timeout time.Duration
}

type Sandbox struct {
	ID     string
	Status string
}

type ExecutionRequest struct {
	SandboxID string
	Command   string
	Resources map[string][]byte
	Timeout   time.Duration
}

type ExecutionResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Duration time.Duration
}

type ADCClient interface {
	CreateSandbox(ctx context.Context, cfg SandboxConfig) (*Sandbox, error)
	UploadFiles(ctx context.Context, sandboxID string, files map[string][]byte) error
	Execute(ctx context.Context, req ExecutionRequest) (*ExecutionResult, error)
	DeleteSandbox(ctx context.Context, sandboxID string) error
}

type ADCEngineConfig struct {
	APIKey         string
	Endpoint       string
	MaxSandboxes   int
	DefaultTimeout time.Duration
	SandboxImage   string
}

type ADCEngine struct {
	client  ADCClient
	config  ADCEngineConfig
	mu      sync.Mutex
	active  map[string]*Sandbox
	perUser map[string]int
}

// ---------------------------------------------------------------------------
// Mock ADC client
// ---------------------------------------------------------------------------

type mockADCClient struct {
	mu              sync.Mutex
	sandboxes       map[string]*Sandbox
	createErr       error
	uploadErr       error
	executeResult   *ExecutionResult
	executeErr      error
	deleteErr       error
	createCallCount atomic.Int32
	deleteCallCount atomic.Int32
}

func newMockADCClient() *mockADCClient {
	return &mockADCClient{sandboxes: make(map[string]*Sandbox)}
}

func (m *mockADCClient) CreateSandbox(_ context.Context, _ SandboxConfig) (*Sandbox, error) {
	m.createCallCount.Add(1)
	if m.createErr != nil {
		return nil, m.createErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	id := fmt.Sprintf("sb-%d", len(m.sandboxes)+1)
	sb := &Sandbox{ID: id, Status: "running"}
	m.sandboxes[id] = sb
	return sb, nil
}

func (m *mockADCClient) UploadFiles(_ context.Context, sandboxID string, _ map[string][]byte) error {
	if m.uploadErr != nil {
		return m.uploadErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sandboxes[sandboxID]; !ok {
		return errors.New("sandbox not found")
	}
	return nil
}

func (m *mockADCClient) Execute(ctx context.Context, _ ExecutionRequest) (*ExecutionResult, error) {
	if m.executeErr != nil {
		return nil, m.executeErr
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	if m.executeResult != nil {
		return m.executeResult, nil
	}
	return &ExecutionResult{ExitCode: 0, Stdout: "ok", Duration: 100 * time.Millisecond}, nil
}

func (m *mockADCClient) DeleteSandbox(_ context.Context, sandboxID string) error {
	m.deleteCallCount.Add(1)
	if m.deleteErr != nil {
		return m.deleteErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if sb, ok := m.sandboxes[sandboxID]; ok {
		sb.Status = "deleted"
	}
	return nil
}

var _ ADCClient = (*mockADCClient)(nil)

func newTestEngine(client ADCClient) *ADCEngine {
	return &ADCEngine{
		client: client,
		config: ADCEngineConfig{
			APIKey:         "test-api-key",
			Endpoint:       "https://adc.example.com",
			MaxSandboxes:   10,
			DefaultTimeout: 30 * time.Second,
			SandboxImage:   "mcr.microsoft.com/waza/sandbox:latest",
		},
		active:  make(map[string]*Sandbox),
		perUser: make(map[string]int),
	}
}

// ---------------------------------------------------------------------------
// Tests – Initialization
// ---------------------------------------------------------------------------

func TestADCEngine_Initialize_ValidConfig(t *testing.T) {
	t.Skip("waiting for ADC engine — blocked on adc-sdk-go")

	client := newMockADCClient()
	engine := newTestEngine(client)

	assert.NotNil(t, engine.client)
	assert.Equal(t, "test-api-key", engine.config.APIKey)
	assert.Equal(t, "https://adc.example.com", engine.config.Endpoint)
	assert.Equal(t, 10, engine.config.MaxSandboxes)
}

func TestADCEngine_Initialize_MissingAPIKey(t *testing.T) {
	t.Skip("waiting for ADC engine — blocked on adc-sdk-go")

	engine := &ADCEngine{
		config: ADCEngineConfig{APIKey: "", Endpoint: "https://adc.example.com"},
	}
	_ = engine
	// err := engine.Initialize(context.Background())
	// assert.Error(t, err)
	// assert.Contains(t, err.Error(), "API key")
}

// ---------------------------------------------------------------------------
// Tests – Execute lifecycle
// ---------------------------------------------------------------------------

func TestADCEngine_Execute_HappyPath(t *testing.T) {
	t.Skip("waiting for ADC engine — blocked on adc-sdk-go")

	client := newMockADCClient()
	client.executeResult = &ExecutionResult{
		ExitCode: 0,
		Stdout:   "Hello, World!",
		Duration: 200 * time.Millisecond,
	}
	engine := newTestEngine(client)
	_ = engine

	// result, err := engine.Execute(ctx, req)
	// require.NoError(t, err)
	// assert.Equal(t, 0, result.ExitCode)

	assert.Equal(t, int32(1), client.createCallCount.Load())
	assert.Equal(t, int32(1), client.deleteCallCount.Load())
}

func TestADCEngine_Execute_SandboxCreateFails(t *testing.T) {
	t.Skip("waiting for ADC engine — blocked on adc-sdk-go")

	client := newMockADCClient()
	client.createErr = errors.New("quota exceeded: no capacity")
	engine := newTestEngine(client)

	_ = engine
	assert.Empty(t, engine.active)
}

func TestADCEngine_Execute_CommandFails(t *testing.T) {
	t.Skip("waiting for ADC engine — blocked on adc-sdk-go")

	client := newMockADCClient()
	client.executeResult = &ExecutionResult{
		ExitCode: 1,
		Stderr:   "error: file not found",
		Duration: 50 * time.Millisecond,
	}
	engine := newTestEngine(client)
	_ = engine

	assert.Equal(t, int32(1), client.deleteCallCount.Load())
}

func TestADCEngine_Execute_Timeout(t *testing.T) {
	t.Skip("waiting for ADC engine — blocked on adc-sdk-go")

	client := newMockADCClient()
	engine := newTestEngine(client)
	engine.config.DefaultTimeout = 50 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_ = engine
	_ = ctx
}

// ---------------------------------------------------------------------------
// Tests – Shutdown & cleanup
// ---------------------------------------------------------------------------

func TestADCEngine_Shutdown_CleansUpSandboxes(t *testing.T) {
	t.Skip("waiting for ADC engine — blocked on adc-sdk-go")

	client := newMockADCClient()
	engine := newTestEngine(client)

	for i := range 3 {
		sb := &Sandbox{ID: fmt.Sprintf("sb-%d", i), Status: "running"}
		engine.active[sb.ID] = sb
	}

	// engine.Shutdown(context.Background())
	assert.Equal(t, int32(3), client.deleteCallCount.Load())
	assert.Empty(t, engine.active)
}

// ---------------------------------------------------------------------------
// Tests – Quota enforcement
// ---------------------------------------------------------------------------

func TestADCEngine_MaxSandboxLimit(t *testing.T) {
	t.Skip("waiting for ADC engine — blocked on adc-sdk-go")

	client := newMockADCClient()
	engine := newTestEngine(client)
	engine.config.MaxSandboxes = 2
	engine.perUser["u-1"] = 2
	_ = engine
}

// ---------------------------------------------------------------------------
// Tests – Concurrency
// ---------------------------------------------------------------------------

func TestADCEngine_ConcurrentExecutions(t *testing.T) {
	t.Skip("waiting for ADC engine — blocked on adc-sdk-go")

	client := newMockADCClient()
	client.executeResult = &ExecutionResult{ExitCode: 0, Stdout: "ok", Duration: 10 * time.Millisecond}
	engine := newTestEngine(client)

	const numGoroutines = 20
	var wg sync.WaitGroup
	errs := make([]error, numGoroutines)

	wg.Add(numGoroutines)
	for i := range numGoroutines {
		go func(idx int) {
			defer wg.Done()
			_ = engine
			errs[idx] = nil
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		assert.NoError(t, err, "goroutine %d should not error", i)
	}
	assert.Empty(t, engine.active)
}

// ---------------------------------------------------------------------------
// Tests – Mock client verification
// ---------------------------------------------------------------------------

func TestMockADCClient_SandboxLifecycle(t *testing.T) {
	client := newMockADCClient()
	ctx := context.Background()

	sb, err := client.CreateSandbox(ctx, SandboxConfig{Image: "test"})
	require.NoError(t, err)
	assert.Equal(t, "running", sb.Status)

	err = client.UploadFiles(ctx, sb.ID, map[string][]byte{"test.txt": []byte("hello")})
	require.NoError(t, err)

	result, err := client.Execute(ctx, ExecutionRequest{SandboxID: sb.ID, Command: "cat test.txt"})
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)

	err = client.DeleteSandbox(ctx, sb.ID)
	require.NoError(t, err)

	client.mu.Lock()
	assert.Equal(t, "deleted", client.sandboxes[sb.ID].Status)
	client.mu.Unlock()
}

func TestMockADCClient_UploadToNonexistentSandbox(t *testing.T) {
	client := newMockADCClient()
	err := client.UploadFiles(context.Background(), "nonexistent", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "sandbox not found")
}
