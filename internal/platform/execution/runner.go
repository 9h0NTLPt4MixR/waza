// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License. See LICENSE in the project root for license information.

// Package execution runs waza evaluations as local subprocesses or inside
// ADC sandboxes. When RunConfig.ADCEngine is set, the eval is dispatched
// to an ADC sandbox instead of running locally.
package execution

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/microsoft/waza/internal/platform/adc"
	"github.com/microsoft/waza/internal/platform/db"
)

// DefaultTimeout is the maximum wall-clock time for a single eval run.
// Real Copilot SDK evals with multiple tasks and trials can take 45+ minutes.
const DefaultTimeout = 60 * time.Minute

// RunConfig carries everything RunEval needs to execute an evaluation.
type RunConfig struct {
	Store       db.Store
	Run         *db.RunRequest
	GitHubToken string // user's OAuth token for repo clone
	WazaBinary  string // path to waza binary; empty = "waza" (in $PATH)
	Timeout     time.Duration
	Executor    string      // executor engine override (e.g., "copilot-sdk", "mock"); empty = use eval YAML default
	ADCEngine   *adc.Engine // non-nil = run in ADC sandbox instead of local subprocess
}

// RunEval clones a repo, locates the eval spec, executes `waza run`, and
// persists the results to Cosmos. It is designed to be called in a goroutine
// from the trigger handler.
//
// When ADCEngine is set, the eval runs inside an ADC sandbox. Otherwise
// it uses a local subprocess.
//
// Status transitions: queued → running → complete | failed.
func RunEval(ctx context.Context, cfg RunConfig) (retErr error) {
	if cfg.ADCEngine != nil {
		return runViaADC(ctx, cfg)
	}
	return runLocal(ctx, cfg)
}

// runViaADC creates an ADC sandbox, clones the repo inside it, runs waza,
// reads results.json, saves to Cosmos, and deletes the sandbox.
func runViaADC(ctx context.Context, cfg RunConfig) (retErr error) {
	run := cfg.Run
	logger := slog.With("run", run.ID, "repo", run.Repo, "eval", run.EvalSpec, "mode", "adc")

	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("panic during ADC eval: %v", r)
			logger.Error("recovered panic in runViaADC", "panic", r)
			markFailed(cfg.Store, run, retErr.Error())
		}
	}()

	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	// 1. Mark running.
	run.Status = db.Running
	if err := cfg.Store.UpdateRunRequest(ctx, run); err != nil {
		logger.Error("failed to update status to running", "error", err)
		return fmt.Errorf("update run status: %w", err)
	}
	logger.Info("ADC run started")

	// 2. Create sandbox with the user's GitHub token for Copilot SDK auth.
	env := map[string]string{
		"GITHUB_TOKEN": cfg.GitHubToken,
	}
	sandbox, err := cfg.ADCEngine.CreateSandbox(ctx, cfg.GitHubToken, env)
	if err != nil {
		msg := fmt.Sprintf("ADC sandbox creation failed: %v", err)
		logger.Error(msg, "error", err)
		markFailed(cfg.Store, run, msg)
		return fmt.Errorf("adc sandbox: %w", err)
	}
	defer func() {
		if delErr := cfg.ADCEngine.DeleteSandbox(ctx, sandbox); delErr != nil {
			logger.Error("failed to delete ADC sandbox", "sandbox", sandbox.ID(), "error", delErr)
		}
	}()
	logger.Info("ADC sandbox created", "sandbox", sandbox.ID())

	// Store sandbox ID on the run for UI visibility.
	run.ADCSandboxIDs = append(run.ADCSandboxIDs, sandbox.ID())
	if err := cfg.Store.UpdateRunRequest(ctx, run); err != nil {
		logger.Warn("failed to persist sandbox ID", "error", err)
	}

	// 3. Clone repo inside the sandbox.
	cloneCmd := fmt.Sprintf("git clone --depth=1 https://x-access-token:%s@github.com/%s.git /workspace/repo", cfg.GitHubToken, run.Repo)
	cloneResult, err := sandbox.ExecuteShellCommand(ctx, cloneCmd, "", nil, "/workspace")
	if err != nil || (cloneResult != nil && cloneResult.ExitCode != 0) {
		stderr := ""
		if cloneResult != nil {
			stderr = cloneResult.Stderr
		}
		msg := fmt.Sprintf("git clone failed in sandbox: %s", sanitizeToken(stderr, cfg.GitHubToken))
		logger.Error(msg, "error", err)
		markFailed(cfg.Store, run, truncate(msg, 500))
		return fmt.Errorf("adc git clone: %w", err)
	}
	logger.Info("repo cloned in sandbox")

	// 4. Run waza inside the sandbox (background + poll pattern).
	// ADC's ExecuteShellCommand is synchronous HTTP — the server-side timeout
	// kills long-running requests (~5 min). We launch waza in the background
	// and poll for completion instead.
	executor := cfg.Executor
	if executor == "" {
		executor = "copilot-sdk"
	}
	wazaCmd := fmt.Sprintf(
		"cd /workspace/repo && nohup waza run %s -o /workspace/results.json --executor %s",
		run.EvalSpec, executor)
	if run.Model != "" {
		wazaCmd += " --model " + run.Model
	}
	wazaCmd += " > /workspace/waza.log 2>&1 & echo $!"

	// Start waza in background — returns PID immediately.
	startResult, err := sandbox.ExecuteShellCommand(ctx, wazaCmd, "", nil, "/workspace/repo")
	if err != nil {
		msg := fmt.Sprintf("failed to start waza in sandbox: %v", err)
		logger.Error(msg, "error", err)
		markFailed(cfg.Store, run, truncate(msg, 500))
		return fmt.Errorf("adc waza start: %w", err)
	}
	pid := strings.TrimSpace(startResult.Stdout)
	logger.Info("waza started in sandbox", "pid", pid)

	// Poll for completion: check if PID is still running, with 15s intervals.
	pollCmd := fmt.Sprintf("kill -0 %s 2>/dev/null && echo RUNNING || echo DONE", pid)
	for {
		select {
		case <-ctx.Done():
			msg := "ADC eval timed out"
			logger.Error(msg)
			markFailed(cfg.Store, run, msg)
			return fmt.Errorf("adc eval: %w", ctx.Err())
		case <-time.After(15 * time.Second):
		}

		pollResult, err := sandbox.ExecuteShellCommand(ctx, pollCmd, "", nil, "")
		if err != nil {
			logger.Warn("poll failed, retrying", "error", err)
			continue
		}
		status := strings.TrimSpace(pollResult.Stdout)
		if status == "DONE" {
			break
		}
		logger.Debug("waza still running in sandbox", "pid", pid)

		// Tail the waza log for live progress visibility.
		logResult, logErr := sandbox.ExecuteShellCommand(ctx, "tail -30 /workspace/waza.log 2>/dev/null", "", nil, "")
		if logErr == nil && logResult != nil && logResult.ExitCode == 0 {
			run.LogTail = logResult.Stdout
			_ = cfg.Store.UpdateRunRequest(ctx, run) // best-effort
		}
	}

	// Read exit code from waza log.
	exitResult, err := sandbox.ExecuteShellCommand(ctx, "wait "+pid+" 2>/dev/null; cat /workspace/waza.log | tail -5", "", nil, "")
	exitCode := 0
	if exitResult != nil {
		exitCode = exitResult.ExitCode
	}

	// Exit code 1 = tests failed (eval completed), exit code 2+ = real error.
	if exitCode > 1 {
		logTail := ""
		if exitResult != nil {
			logTail = exitResult.Stdout
		}
		msg := fmt.Sprintf("waza run exited with code %d: %s", exitCode, sanitizeToken(logTail, cfg.GitHubToken))
		logger.Error(msg)
		markFailed(cfg.Store, run, truncate(msg, 500))
		return fmt.Errorf("adc waza exit code %d", exitCode)
	}
	logger.Info("waza run completed in sandbox", "exitCode", exitCode)

	// 5. Read results.json from sandbox.
	resultsJSON, err := sandbox.ReadFileText(ctx, "/workspace/results.json")
	if err != nil {
		msg := "failed to read results.json from sandbox"
		logger.Error(msg, "error", err)
		markFailed(cfg.Store, run, msg)
		return fmt.Errorf("adc read results: %w", err)
	}

	if !json.Valid([]byte(resultsJSON)) {
		msg := "results.json from sandbox is not valid JSON"
		logger.Error(msg)
		markFailed(cfg.Store, run, msg)
		return fmt.Errorf("invalid results JSON from sandbox")
	}

	// 6. Save to Cosmos.
	if err := cfg.Store.SaveResult(ctx, run.UserID, run.ID, json.RawMessage(resultsJSON)); err != nil {
		msg := "failed to save results to Cosmos"
		logger.Error(msg, "error", err)
		markFailed(cfg.Store, run, msg)
		return fmt.Errorf("save result: %w", err)
	}
	logger.Info("ADC results saved to Cosmos")

	// 7. Mark complete.
	now := time.Now()
	run.Status = db.Complete
	run.CompletedAt = &now
	if err := cfg.Store.UpdateRunRequest(ctx, run); err != nil {
		logger.Error("failed to mark run complete", "error", err)
		return fmt.Errorf("update run complete: %w", err)
	}

	logger.Info("ADC run completed successfully",
		"sandbox", sandbox.ID(),
		"duration", time.Since(run.CreatedAt).Round(time.Second),
	)
	return nil
}

// runLocal runs the eval as a local subprocess (existing behavior).
func runLocal(ctx context.Context, cfg RunConfig) (retErr error) {
	run := cfg.Run
	logger := slog.With("run", run.ID, "repo", run.Repo, "eval", run.EvalSpec)

	// Panic safety — a crashed goroutine must not take down the server.
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("panic during eval: %v", r)
			logger.Error("recovered panic in RunEval", "panic", r)
			markFailed(cfg.Store, run, retErr.Error())
		}
	}()

	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	wazaBin := cfg.WazaBinary
	if wazaBin == "" {
		wazaBin = "/app/waza"
	}

	// 1. Mark running.
	run.Status = db.Running
	if err := cfg.Store.UpdateRunRequest(ctx, run); err != nil {
		logger.Error("failed to update status to running", "error", err)
		return fmt.Errorf("update run status: %w", err)
	}
	logger.Info("run started")

	// 2. Create workspace directory.
	workDir, err := os.MkdirTemp("", "waza-run-*")
	if err != nil {
		markFailed(cfg.Store, run, "failed to create workspace")
		return fmt.Errorf("create workspace: %w", err)
	}
	defer os.RemoveAll(workDir)

	// 3. Clone repo using the user's GitHub token.
	cloneURL := fmt.Sprintf("https://x-access-token:%s@github.com/%s.git", cfg.GitHubToken, run.Repo)
	cloneCmd := exec.CommandContext(ctx, "git", "clone", "--depth=1", cloneURL, "repo")
	cloneCmd.Dir = workDir
	// Suppress token from logs by capturing stderr.
	var cloneErr bytes.Buffer
	cloneCmd.Stderr = &cloneErr
	if err := cloneCmd.Run(); err != nil {
		msg := fmt.Sprintf("git clone failed: %s", sanitizeToken(cloneErr.String(), cfg.GitHubToken))
		logger.Error(msg, "error", err)
		markFailed(cfg.Store, run, msg)
		return fmt.Errorf("git clone: %w", err)
	}
	logger.Info("repo cloned")

	repoDir := filepath.Join(workDir, "repo")

	// 4. Verify eval spec exists.
	evalPath := filepath.Join(repoDir, run.EvalSpec)
	if _, err := os.Stat(evalPath); err != nil {
		msg := fmt.Sprintf("eval spec not found: %s", run.EvalSpec)
		logger.Error(msg, "error", err)
		markFailed(cfg.Store, run, msg)
		return fmt.Errorf("eval spec: %w", err)
	}

	// 5. Run waza.
	resultsPath := filepath.Join(workDir, "results.json")
	args := []string{"run", evalPath, "--output", resultsPath}

	// Override executor engine — platform defaults to copilot-sdk so evals
	// from external repos (which may specify executor: mock) actually call the LLM.
	executor := cfg.Executor
	if executor == "" {
		executor = "copilot-sdk"
	}
	args = append(args, "--executor", executor)

	if run.Model != "" {
		args = append(args, "--model", run.Model)
	}
	wazaCmd := exec.CommandContext(ctx, wazaBin, args...)
	wazaCmd.Dir = repoDir
	// Pass GitHub token so the Copilot SDK can authenticate.
	wazaCmd.Env = append(os.Environ(), "GITHUB_TOKEN="+cfg.GitHubToken)

	var stdout, stderr bytes.Buffer
	wazaCmd.Stdout = &stdout
	wazaCmd.Stderr = &stderr

	logger.Info("executing waza run", "args", args)
	if err := wazaCmd.Run(); err != nil {
		// Exit code 1 = tests failed (normal eval result, not a platform error).
		// Exit code 2 = configuration/runtime error.
		// Only treat exit code 2+ as a real failure. For exit code 1,
		// continue to read results.json — the eval completed.
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			logger.Info("waza run completed with test failures (exit code 1)")
		} else {
			msg := fmt.Sprintf("waza run failed: %s", sanitizeToken(stderr.String(), cfg.GitHubToken))
			logger.Error(msg, "error", err, "stderr", sanitizeToken(stderr.String(), cfg.GitHubToken))
			markFailed(cfg.Store, run, truncate(msg, 500))
			return fmt.Errorf("waza run: %w", err)
		}
	} else {
		logger.Info("waza run completed (all tests passed)")
	}

	// 6. Read and persist results.
	resultsData, err := os.ReadFile(resultsPath)
	if err != nil {
		msg := "failed to read results.json"
		logger.Error(msg, "error", err)
		markFailed(cfg.Store, run, msg)
		return fmt.Errorf("read results: %w", err)
	}

	// Validate it's valid JSON before saving.
	if !json.Valid(resultsData) {
		msg := "results.json is not valid JSON"
		logger.Error(msg)
		markFailed(cfg.Store, run, msg)
		return fmt.Errorf("invalid results JSON")
	}

	if err := cfg.Store.SaveResult(ctx, run.UserID, run.ID, json.RawMessage(resultsData)); err != nil {
		msg := "failed to save results to Cosmos"
		logger.Error(msg, "error", err)
		markFailed(cfg.Store, run, msg)
		return fmt.Errorf("save result: %w", err)
	}
	logger.Info("results saved to Cosmos")

	// 7. Mark complete.
	now := time.Now()
	run.Status = db.Complete
	run.CompletedAt = &now
	if err := cfg.Store.UpdateRunRequest(ctx, run); err != nil {
		logger.Error("failed to mark run complete", "error", err)
		return fmt.Errorf("update run complete: %w", err)
	}

	logger.Info("run completed successfully",
		"duration", time.Since(run.CreatedAt).Round(time.Second),
	)
	return nil
}

// markFailed sets the run status to Failed with the given error message.
func markFailed(store db.Store, run *db.RunRequest, errMsg string) {
	now := time.Now()
	run.Status = db.Failed
	run.Error = errMsg
	run.CompletedAt = &now
	if err := store.UpdateRunRequest(context.Background(), run); err != nil {
		slog.Error("markFailed: failed to update run", "run", run.ID, "error", err)
	}
}

// sanitizeToken removes the GitHub token from log output to prevent leaks.
func sanitizeToken(s, token string) string {
	if token == "" || s == "" {
		return s
	}
	return strings.ReplaceAll(s, token, "***")
}

// truncate limits s to maxLen bytes, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen < 4 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
