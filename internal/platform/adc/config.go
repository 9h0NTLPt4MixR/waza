// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License. See LICENSE in the project root for license information.

// Package adc defines configuration types for Azure Dev Compute (ADC)
// sandbox management. ADC provides isolated execution environments for
// running waza evaluations in the hosted platform.
package adc

import "time"

// Quota and resource defaults. These are intentionally constants, not
// configuration — changing sandbox limits requires a code change and review.
const (
	// MaxSandboxesPerUser is the hard cap on concurrent sandboxes per user.
	// Agreed in platform design: 10 concurrent sandboxes per user.
	MaxSandboxesPerUser = 10

	// DefaultCPU is the default vCPU allocation per sandbox in millicores.
	DefaultCPU = 2000

	// DefaultMemoryMB is the default memory allocation per sandbox in MB.
	DefaultMemoryMB = 4096

	// DefaultSandboxTimeout is how long a sandbox can run before forced termination.
	DefaultSandboxTimeout = 60 * time.Minute

	// DefaultAPIURL is the default ADC management API endpoint.
	DefaultAPIURL = "https://management.azuredevcompute.io"
)

// ADCConfig holds the ADC connection and resource settings. In platform mode
// these are loaded from environment variables at startup. The ADC SDK
// authenticates per-request using the user's GitHub OAuth token — no
// platform-level API key is needed.
type ADCConfig struct {
	// APIKey authenticates requests to the ADC management API.
	// If set, used for all sandbox operations (platform-level credential).
	// If empty, falls back to per-user GitHub token auth.
	APIKey string `yaml:"api_key" json:"-"` // never serialize to JSON

	// APIURL is the ADC management API base URL.
	APIURL string `yaml:"api_url" json:"api_url"`

	// DiskImage is the container image used for sandboxes (e.g.,
	// "mcr.microsoft.com/waza/sandbox:latest").
	DiskImage string `yaml:"disk_image" json:"disk_image"`

	// CPU is the vCPU allocation per sandbox in millicores. Defaults to DefaultCPU.
	CPU int `yaml:"cpu" json:"cpu"`

	// MemoryMB is the memory allocation per sandbox in MB. Defaults to DefaultMemoryMB.
	MemoryMB int `yaml:"memory_mb" json:"memory_mb"`

	// SandboxGroupID is the ADC sandbox group for resource isolation.
	SandboxGroupID string `yaml:"sandbox_group_id" json:"sandbox_group_id"`

	// MaxSandboxesPerUser overrides the default concurrent sandbox limit.
	// Clamped to MaxSandboxesPerUser constant — this field cannot exceed it.
	MaxSandboxes int `yaml:"max_sandboxes_per_user" json:"max_sandboxes_per_user"`

	// SandboxTimeout overrides the default sandbox execution timeout.
	SandboxTimeout time.Duration `yaml:"sandbox_timeout" json:"sandbox_timeout"`
}

// WithDefaults returns a copy of the config with zero-value fields set to defaults.
func (c ADCConfig) WithDefaults() ADCConfig {
	if c.APIURL == "" {
		c.APIURL = DefaultAPIURL
	}
	if c.CPU <= 0 {
		c.CPU = DefaultCPU
	}
	if c.MemoryMB <= 0 {
		c.MemoryMB = DefaultMemoryMB
	}
	if c.MaxSandboxes <= 0 || c.MaxSandboxes > MaxSandboxesPerUser {
		c.MaxSandboxes = MaxSandboxesPerUser
	}
	if c.SandboxTimeout <= 0 {
		c.SandboxTimeout = DefaultSandboxTimeout
	}
	return c
}

// CanAllocate reports whether the user has capacity for n additional sandboxes
// given their current active count.
func (c ADCConfig) CanAllocate(activeSandboxes, requested int) bool {
	return activeSandboxes+requested <= c.MaxSandboxes
}
