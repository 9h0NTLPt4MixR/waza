// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License. See LICENSE in the project root for license information.

// Package execution runs waza evaluations as local subprocesses.
// This is the "local engine" — it clones the user's repo, finds the eval
// spec, and shells out to `waza run`. When ADC sandbox support is ready,
// the subprocess call can be swapped for sandbox creation without changing
// the handler wiring.
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

	"github.com/microsoft/waza/internal/platform/db"
)

// DefaultTimeout is the maximum wall-clock time for a single eval run.
const DefaultTimeout = 5 * time.Minute

// RunConfig carries everything RunEval needs to execute an evaluation.
type RunConfig struct {
	Store       db.Store
	Run         *db.RunRequest
	GitHubToken string // user's OAuth token for repo clone
	WazaBinary  string // path to waza binary; empty = "waza" (in $PATH)
	Timeout     time.Duration
}

// RunEval clones a repo, locates the eval spec, executes `waza run`, and
// persists the results to Cosmos. It is designed to be called in a goroutine
// from the trigger handler.
//
// Status transitions: queued → running → complete | failed.
func RunEval(ctx context.Context, cfg RunConfig) (retErr error) {
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
