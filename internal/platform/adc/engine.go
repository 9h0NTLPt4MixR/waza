// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License. See LICENSE in the project root for license information.

package adc

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	adcsdk "github.com/coreai-microsoft/adc-sdk-go"
	"github.com/coreai-microsoft/adc-sdk-go/models"
	"github.com/microsoft/waza/internal/execution"
	wazamodels "github.com/microsoft/waza/internal/models"
)

// Engine implements execution.AgentEngine using Azure Dev Compute sandboxes.
// Each Execute call creates an isolated sandbox, uploads fixtures, runs the
// waza CLI inside it, and collects the results.
//
// The ADC client is created per-request using the user's GitHub OAuth token.
// The Engine stores platform-level config (disk image, CPU, memory) but
// defers authentication to NewClient.
type Engine struct {
	cfg ADCConfig

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

// NewClient creates an ADC client authenticated with the user's GitHub token.
// Call this per-request — the SDK uses "Authorization: GitHub {token}" for auth.
func (e *Engine) NewClient(githubToken string) *adcsdk.Client {
	cfg := adcsdk.Config{
		APIURL:  e.cfg.APIURL,
		Timeout: e.cfg.SandboxTimeout, // match eval timeout (default 30m)
	}
	// Prefer platform-level API key. For GitHub tokens (gho_/ghp_), use
	// the GitHubToken field which sends "Authorization: GitHub {token}".
	// A real ADC API key uses the "x-ms-api-key" header instead.
	if e.cfg.APIKey != "" {
		if isGitHubToken(e.cfg.APIKey) {
			cfg.GitHubToken = e.cfg.APIKey
		} else {
			cfg.APIKey = e.cfg.APIKey
		}
	} else {
		cfg.GitHubToken = githubToken
	}
	return adcsdk.New(cfg)
}

// isGitHubToken returns true if the token looks like a GitHub OAuth/PAT token.
func isGitHubToken(token string) bool {
	return len(token) > 4 && (token[:4] == "gho_" || token[:4] == "ghp_" || token[:4] == "ghs_" || token[:4] == "ghr_")
}

// Execute runs a single evaluation task in an isolated ADC sandbox.
//
// Flow: create sandbox → upload fixture files → execute waza CLI →
// read results JSON → parse into ExecutionResponse → delete sandbox.
func (e *Engine) Execute(ctx context.Context, req *execution.ExecutionRequest, githubToken string) (*execution.ExecutionResponse, error) {
	start := time.Now()
	client := e.NewClient(githubToken)

	// Create sandbox from disk image
	sandbox, err := client.Sandboxes.CreateFromDiskImage(ctx, models.CreateFromDiskImageOptions{
		DiskImage:      models.SandboxSourceDiskImage{ID: e.cfg.DiskImage},
		CPU:            fmt.Sprintf("%dm", e.cfg.CPU),
		Memory:         fmt.Sprintf("%dMi", e.cfg.MemoryMB),
		SandboxGroupID: e.cfg.SandboxGroupID,
		Environment: map[string]string{
			"GITHUB_TOKEN": githubToken,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("adc: creating sandbox: %w", err)
	}

	e.trackSandbox(sandbox)
	defer e.cleanupSandbox(ctx, sandbox, githubToken)

	// Upload fixture files into the sandbox workspace
	for _, res := range req.Resources {
		if _, err := sandbox.WriteFileText(ctx, "/workspace/"+res.Path, string(res.Content), &models.WriteFileOptions{CreateDirs: true}); err != nil {
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

	result, err := sandbox.ExecuteShellCommand(execCtx, cmd, "", nil, "/workspace")
	if err != nil {
		return &execution.ExecutionResponse{
			FinalOutput: "",
			ErrorMsg:    fmt.Sprintf("sandbox execution failed: %v", err),
			Success:     false,
			DurationMs:  time.Since(start).Milliseconds(),
		}, nil
	}

	// Check command exit code
	if result.ExitCode != 0 {
		return &execution.ExecutionResponse{
			FinalOutput: result.Stdout,
			ErrorMsg:    fmt.Sprintf("sandbox command exited with code %d: %s", result.ExitCode, result.Stderr),
			Success:     false,
			DurationMs:  time.Since(start).Milliseconds(),
		}, nil
	}

	// Read and parse results
	resultsJSON, err := sandbox.ReadFileText(ctx, "/workspace/results.json")
	if err != nil {
		return &execution.ExecutionResponse{
			FinalOutput: result.Stdout,
			ErrorMsg:    fmt.Sprintf("reading results: %v", err),
			Success:     false,
			DurationMs:  time.Since(start).Milliseconds(),
		}, nil
	}

	resp := &execution.ExecutionResponse{
		FinalOutput: result.Stdout,
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

// CreateSandbox creates an ADC sandbox with the given GitHub token and returns
// the sandbox ID. Used by the platform runner for repo-level eval execution.
func (e *Engine) CreateSandbox(ctx context.Context, githubToken string, env map[string]string) (*adcsdk.Sandbox, error) {
	client := e.NewClient(githubToken)

	sandbox, err := client.Sandboxes.CreateFromDiskImage(ctx, models.CreateFromDiskImageOptions{
		DiskImage:      models.SandboxSourceDiskImage{ID: e.cfg.DiskImage},
		CPU:            fmt.Sprintf("%dm", e.cfg.CPU),
		Memory:         fmt.Sprintf("%dMi", e.cfg.MemoryMB),
		SandboxGroupID: e.cfg.SandboxGroupID,
		Environment:    env,
	})
	if err != nil {
		return nil, fmt.Errorf("adc: creating sandbox: %w", err)
	}

	e.trackSandbox(sandbox)
	return sandbox, nil
}

// CreateBatchSandboxes creates count sandboxes via the ADC batch API and
// returns them as ready-to-use Sandbox objects. Each sandbox gets the
// same disk image, CPU, memory, and environment configuration.
//
// The batch API creates all sandboxes in a single request for faster
// provisioning. Failed sandboxes are logged but not fatal — the caller
// gets back however many succeeded (minimum 1 or error).
func (e *Engine) CreateBatchSandboxes(ctx context.Context, count int, githubToken string, env map[string]string) ([]*adcsdk.Sandbox, error) {
	if count <= 0 {
		return nil, fmt.Errorf("adc: batch count must be > 0, got %d", count)
	}
	if count > MaxSandboxesPerUser {
		count = MaxSandboxesPerUser
	}

	client := e.NewClient(githubToken)

	batchReq := models.BatchSandboxRequest{
		Count: count,
		Sandbox: models.CreateSandboxRequest{
			SourcesRef: models.SandboxSource{
				DiskImage: &models.SandboxSourceDiskImage{ID: e.cfg.DiskImage},
			},
			Resources: &models.SandboxResources{
				CPU:    fmt.Sprintf("%dm", e.cfg.CPU),
				Memory: fmt.Sprintf("%dMi", e.cfg.MemoryMB),
			},
			SandboxGroupID: e.cfg.SandboxGroupID,
			Environment:    env,
		},
	}

	resp, err := client.Sandboxes.CreateBatch(ctx, batchReq)
	if err != nil {
		return nil, fmt.Errorf("adc: batch create: %w", err)
	}

	if resp.Succeeded == 0 {
		return nil, fmt.Errorf("adc: batch create failed — 0 of %d sandboxes created: %v", count, resp.Errors)
	}

	// Convert SandboxData → *Sandbox via Get (needed for Execute methods).
	sandboxes := make([]*adcsdk.Sandbox, 0, resp.Succeeded)
	for _, data := range resp.Sandboxes {
		sb, err := client.Sandboxes.Get(ctx, data.ID)
		if err != nil {
			// Log but continue — partial success is better than total failure.
			continue
		}
		e.trackSandbox(sb)
		sandboxes = append(sandboxes, sb)
	}

	if len(sandboxes) == 0 {
		return nil, fmt.Errorf("adc: batch create succeeded (%d) but failed to retrieve sandbox handles", resp.Succeeded)
	}

	return sandboxes, nil
}

// DeleteBatchSandboxes deletes a slice of sandboxes, returning the last
// error encountered. Best-effort: continues deleting even if one fails.
func (e *Engine) DeleteBatchSandboxes(ctx context.Context, sandboxes []*adcsdk.Sandbox) error {
	var lastErr error
	for _, sb := range sandboxes {
		e.mu.Lock()
		delete(e.sandboxes, sb.ID())
		e.mu.Unlock()
		if err := sb.Delete(ctx); err != nil {
			lastErr = fmt.Errorf("adc: deleting sandbox %s: %w", sb.ID(), err)
		}
	}
	return lastErr
}

// DeleteSandbox deletes a sandbox by reference and removes it from tracking.
func (e *Engine) DeleteSandbox(ctx context.Context, sandbox *adcsdk.Sandbox) error {
	e.mu.Lock()
	delete(e.sandboxes, sandbox.ID())
	e.mu.Unlock()
	return sandbox.Delete(ctx)
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
func (e *Engine) SessionUsage(_ string) *wazamodels.UsageStats {
	return nil
}

// Config returns the engine's ADC configuration.
func (e *Engine) Config() ADCConfig {
	return e.cfg
}

// trackSandbox registers a sandbox for cleanup tracking.
func (e *Engine) trackSandbox(sb *adcsdk.Sandbox) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.sandboxes[sb.ID()] = sb
}

// cleanupSandbox removes and deletes a single sandbox.
func (e *Engine) cleanupSandbox(ctx context.Context, sb *adcsdk.Sandbox, githubToken string) {
	e.mu.Lock()
	delete(e.sandboxes, sb.ID())
	e.mu.Unlock()
	// Best-effort delete; Shutdown will catch stragglers.
	_ = sb.Delete(ctx)
}
