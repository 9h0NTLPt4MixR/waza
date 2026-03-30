// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License. See LICENSE in the project root for license information.

//go:build adcsdk

package adc

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	adcsdk "github.com/coreai-microsoft/adc-sdk-go"
	"github.com/microsoft/waza/internal/execution"
	"github.com/microsoft/waza/internal/models"
)

// Engine implements execution.AgentEngine using Azure Dev Compute sandboxes.
// Each Execute call creates an isolated sandbox, uploads fixtures, runs the
// waza CLI inside it, and collects the results.
type Engine struct {
	cfg    ADCConfig
	client *adcsdk.Client

	mu        sync.Mutex
	sandboxes map[string]*adcsdk.Sandbox // tracked for cleanup
	shutdown  bool
}

// NewEngine creates an ADC engine with the given configuration.
func NewEngine(cfg ADCConfig) *Engine {
	return &Engine{
		cfg:       cfg.WithDefaults(),
		sandboxes: make(map[string]*adcsdk.Sandbox),
	}
}

// Initialize creates the ADC client and validates credentials.
func (e *Engine) Initialize(ctx context.Context) error {
	client, err := adcsdk.New(adcsdk.Config{
		APIKey: e.cfg.APIKey,
		APIURL: e.cfg.APIURL,
	})
	if err != nil {
		return fmt.Errorf("adc: creating client: %w", err)
	}
	e.client = client
	return nil
}

// Execute runs a single evaluation task in an isolated ADC sandbox.
//
// Flow: create sandbox → upload fixture files → execute waza CLI →
// read results JSON → parse into ExecutionResponse → delete sandbox.
func (e *Engine) Execute(ctx context.Context, req *execution.ExecutionRequest) (*execution.ExecutionResponse, error) {
	start := time.Now()

	// Create sandbox from disk image
	sandbox, err := e.client.Sandboxes.CreateFromDiskImage(ctx, adcsdk.CreateSandboxRequest{
		DiskImageID:    e.cfg.DiskImage,
		SandboxGroupID: e.cfg.SandboxGroupID,
		CPU:            strconv.Itoa(e.cfg.CPU),
		Memory:         strconv.Itoa(e.cfg.MemoryMB) + "Mi",
	})
	if err != nil {
		return nil, fmt.Errorf("adc: creating sandbox: %w", err)
	}

	e.trackSandbox(sandbox)
	defer e.cleanupSandbox(ctx, sandbox)

	// Upload fixture files into the sandbox workspace
	for _, res := range req.Resources {
		if err := sandbox.WriteFileText(ctx, "/workspace/"+res.Path, string(res.Content)); err != nil {
			return nil, fmt.Errorf("adc: uploading file %s: %w", res.Path, err)
		}
	}

	// Build and execute the waza CLI command inside the sandbox
	cmd := fmt.Sprintf(
		"cd /workspace && waza run eval.yaml --context-dir fixtures -o results.json --model %s 2>&1",
		req.ModelID,
	)

	timeout := req.Timeout
	if timeout == 0 {
		timeout = e.cfg.SandboxTimeout
	}

	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	output, err := sandbox.ExecuteShellCommand(execCtx, cmd)
	if err != nil {
		return &execution.ExecutionResponse{
			FinalOutput: output,
			ErrorMsg:    fmt.Sprintf("sandbox execution failed: %v", err),
			Success:     false,
			DurationMs:  time.Since(start).Milliseconds(),
		}, nil
	}

	// Read and parse results
	resultsJSON, err := sandbox.ReadFileText(ctx, "/workspace/results.json")
	if err != nil {
		return &execution.ExecutionResponse{
			FinalOutput: output,
			ErrorMsg:    fmt.Sprintf("reading results: %v", err),
			Success:     false,
			DurationMs:  time.Since(start).Milliseconds(),
		}, nil
	}

	resp := &execution.ExecutionResponse{
		FinalOutput: output,
		Success:     true,
		DurationMs:  time.Since(start).Milliseconds(),
		ModelID:     req.ModelID,
	}

	// Try to extract final output from results JSON
	var results struct {
		Output string `json:"output"`
	}
	if json.Unmarshal([]byte(resultsJSON), &results) == nil && results.Output != "" {
		resp.FinalOutput = results.Output
	}

	return resp, nil
}

// Shutdown deletes all tracked sandboxes. Safe to call multiple times.
func (e *Engine) Shutdown(ctx context.Context) error {
	e.mu.Lock()
	if e.shutdown {
		e.mu.Unlock()
		return nil
	}
	e.shutdown = true
	sandboxes := make(map[string]*adcsdk.Sandbox, len(e.sandboxes))
	for k, v := range e.sandboxes {
		sandboxes[k] = v
	}
	e.mu.Unlock()

	var lastErr error
	for id, sb := range sandboxes {
		if err := sb.Delete(ctx); err != nil {
			lastErr = fmt.Errorf("adc: deleting sandbox %s: %w", id, err)
		}
	}

	e.mu.Lock()
	e.sandboxes = make(map[string]*adcsdk.Sandbox)
	e.mu.Unlock()

	return lastErr
}

// SessionUsage returns nil — ADC sandboxes don't track per-session token usage.
func (e *Engine) SessionUsage(_ string) *models.UsageStats {
	return nil
}

// trackSandbox registers a sandbox for cleanup tracking.
func (e *Engine) trackSandbox(sb *adcsdk.Sandbox) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.sandboxes[sb.ID] = sb
}

// cleanupSandbox removes and deletes a single sandbox.
func (e *Engine) cleanupSandbox(ctx context.Context, sb *adcsdk.Sandbox) {
	e.mu.Lock()
	delete(e.sandboxes, sb.ID)
	e.mu.Unlock()
	// Best-effort delete; Shutdown will catch stragglers.
	_ = sb.Delete(ctx)
}
