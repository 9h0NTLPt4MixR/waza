// Tests for the ADC execution engine.
//
// These tests verify the Engine's configuration, lifecycle, and quota logic
// using the real ADC types. Network-dependent tests (sandbox creation,
// execution) are skipped when ADC_API_URL is not set.

package adc_test

import (
	"testing"
	"time"

	"github.com/microsoft/waza/internal/platform/adc"
	"github.com/stretchr/testify/assert"
)

// ---------------------------------------------------------------------------
// Tests – Configuration and defaults
// ---------------------------------------------------------------------------

func TestADCConfig_WithDefaults(t *testing.T) {
	cfg := adc.ADCConfig{}
	got := cfg.WithDefaults()

	assert.Equal(t, adc.DefaultAPIURL, got.APIURL)
	assert.Equal(t, adc.DefaultCPU, got.CPU)
	assert.Equal(t, adc.DefaultMemoryMB, got.MemoryMB)
	assert.Equal(t, adc.MaxSandboxesPerUser, got.MaxSandboxes)
	assert.Equal(t, adc.DefaultSandboxTimeout, got.SandboxTimeout)
}

func TestADCConfig_WithDefaults_PreservesExplicit(t *testing.T) {
	cfg := adc.ADCConfig{
		APIURL:   "https://custom.example.com",
		CPU:      4000,
		MemoryMB: 8192,
	}
	got := cfg.WithDefaults()

	assert.Equal(t, "https://custom.example.com", got.APIURL)
	assert.Equal(t, 4000, got.CPU)
	assert.Equal(t, 8192, got.MemoryMB)
}

func TestADCConfig_CanAllocate(t *testing.T) {
	cfg := adc.ADCConfig{MaxSandboxes: 5}
	assert.True(t, cfg.CanAllocate(3, 2))
	assert.True(t, cfg.CanAllocate(5, 0))
	assert.False(t, cfg.CanAllocate(4, 2))
	assert.False(t, cfg.CanAllocate(5, 1))
}

func TestADCConfig_MaxSandboxes_Clamped(t *testing.T) {
	cfg := adc.ADCConfig{MaxSandboxes: 999}
	got := cfg.WithDefaults()
	assert.Equal(t, adc.MaxSandboxesPerUser, got.MaxSandboxes)
}

// ---------------------------------------------------------------------------
// Tests – Engine creation
// ---------------------------------------------------------------------------

func TestNewEngine(t *testing.T) {
	cfg := adc.ADCConfig{
		APIURL:    "https://management.azuredevcompute.io",
		DiskImage: "test-image",
		CPU:       2000,
		MemoryMB:  4096,
	}
	engine := adc.NewEngine(cfg)

	assert.NotNil(t, engine)
	assert.Equal(t, "https://management.azuredevcompute.io", engine.Config().APIURL)
	assert.Equal(t, "test-image", engine.Config().DiskImage)
	assert.Equal(t, 2000, engine.Config().CPU)
	assert.Equal(t, 4096, engine.Config().MemoryMB)
}

func TestNewEngine_AppliesDefaults(t *testing.T) {
	engine := adc.NewEngine(adc.ADCConfig{})
	assert.Equal(t, adc.DefaultAPIURL, engine.Config().APIURL)
	assert.Equal(t, adc.DefaultCPU, engine.Config().CPU)
	assert.Equal(t, adc.DefaultMemoryMB, engine.Config().MemoryMB)
}

func TestNewClient(t *testing.T) {
	engine := adc.NewEngine(adc.ADCConfig{
		APIURL: "https://test.example.com",
	})
	client := engine.NewClient("ghp_testtoken123")
	assert.NotNil(t, client)
}

func TestEngine_Shutdown_Idempotent(t *testing.T) {
	engine := adc.NewEngine(adc.ADCConfig{})
	// Shutdown with no sandboxes should be safe.
	err := engine.Shutdown(t.Context())
	assert.NoError(t, err)
	// Second call should also be safe.
	err = engine.Shutdown(t.Context())
	assert.NoError(t, err)
}

func TestEngine_SessionUsage_ReturnsNil(t *testing.T) {
	engine := adc.NewEngine(adc.ADCConfig{})
	assert.Nil(t, engine.SessionUsage("any-session"))
}

// ---------------------------------------------------------------------------
// Tests – Default URL is the real ADC endpoint
// ---------------------------------------------------------------------------

func TestDefaultAPIURL(t *testing.T) {
	assert.Equal(t, "https://management.azuredevcompute.io", adc.DefaultAPIURL)
}

func TestDefaultCPU(t *testing.T) {
	assert.Equal(t, 2000, adc.DefaultCPU)
}

func TestDefaultSandboxTimeout(t *testing.T) {
	assert.Equal(t, 60*time.Minute, adc.DefaultSandboxTimeout)
}

// ---------------------------------------------------------------------------
// Tests – Batch sandbox support
// ---------------------------------------------------------------------------

func TestADCConfig_CanAllocate_BatchSizes(t *testing.T) {
	cfg := adc.ADCConfig{MaxSandboxes: 10}

	// Batch of 5 with 0 active → OK
	assert.True(t, cfg.CanAllocate(0, 5))

	// Batch of 10 with 0 active → OK (at limit)
	assert.True(t, cfg.CanAllocate(0, 10))

	// Batch of 11 with 0 active → exceeds max
	assert.False(t, cfg.CanAllocate(0, 11))

	// Batch of 5 with 6 active → exceeds max
	assert.False(t, cfg.CanAllocate(6, 5))

	// Batch of 3 with 7 active → at limit, OK
	assert.True(t, cfg.CanAllocate(7, 3))
}

func TestMaxSandboxesPerUser_Constant(t *testing.T) {
	// MaxSandboxesPerUser is the hard cap agreed in platform design.
	assert.Equal(t, 10, adc.MaxSandboxesPerUser)
}
