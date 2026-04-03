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
	"sync"
	"time"

	adcsdk "github.com/coreai-microsoft/adc-sdk-go"
	"github.com/microsoft/waza/internal/platform/adc"
	"github.com/microsoft/waza/internal/platform/db"
	"gopkg.in/yaml.v3"
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

// runViaADC creates ADC sandbox(es), clones the repo inside, runs waza,
// reads results.json, saves to Cosmos, and deletes the sandbox(es).
//
// When RunConfig.Run.Workers > 1, it creates multiple sandboxes via the
// batch API and distributes tasks across them for parallel execution.
func runViaADC(ctx context.Context, cfg RunConfig) (retErr error) {
	if cfg.Run.Workers > 1 {
		return runViaADCParallel(ctx, cfg)
	}
	return runViaADCSingle(ctx, cfg)
}

// runViaADCSingle is the original single-sandbox ADC execution path.
func runViaADCSingle(ctx context.Context, cfg RunConfig) (retErr error) {
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
		msg := fmt.Sprintf("failed to save results to Cosmos: %v", err)
		logger.Error(msg, "error", err, "resultSizeBytes", len(resultsJSON))
		markFailed(cfg.Store, run, truncate(msg, 500))
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

// ---------------------------------------------------------------------------
// Parallel ADC execution — multiple sandboxes with task sharding
// ---------------------------------------------------------------------------

// sandboxResult holds the results from a single sandbox worker.
type sandboxResult struct {
	SandboxID  string
	ResultJSON json.RawMessage
	LogTail    string
	Err        error
}

// runViaADCParallel creates N sandboxes via the batch API, distributes tasks
// across them, runs waza in each concurrently, merges results, and saves.
func runViaADCParallel(ctx context.Context, cfg RunConfig) (retErr error) {
	run := cfg.Run
	workers := run.Workers
	logger := slog.With("run", run.ID, "repo", run.Repo, "eval", run.EvalSpec, "mode", "adc-parallel", "workers", workers)

	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("panic during parallel ADC eval: %v", r)
			logger.Error("recovered panic in runViaADCParallel", "panic", r)
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
	logger.Info("parallel ADC run started")

	// 2. Create batch sandboxes.
	env := map[string]string{
		"GITHUB_TOKEN": cfg.GitHubToken,
	}

	// Clamp workers to MaxSandboxesPerUser.
	if workers > adc.MaxSandboxesPerUser {
		workers = adc.MaxSandboxesPerUser
		logger.Warn("clamped workers to max", "max", adc.MaxSandboxesPerUser)
	}

	sandboxes, err := cfg.ADCEngine.CreateBatchSandboxes(ctx, workers, cfg.GitHubToken, env)
	if err != nil {
		msg := fmt.Sprintf("ADC batch sandbox creation failed: %v", err)
		logger.Error(msg, "error", err)
		markFailed(cfg.Store, run, msg)
		return fmt.Errorf("adc batch: %w", err)
	}
	defer func() {
		if delErr := cfg.ADCEngine.DeleteBatchSandboxes(ctx, sandboxes); delErr != nil {
			logger.Error("failed to delete ADC sandboxes", "error", delErr)
		}
	}()

	// Track all sandbox IDs on the run for UI visibility.
	for _, sb := range sandboxes {
		run.ADCSandboxIDs = append(run.ADCSandboxIDs, sb.ID())
	}
	if err := cfg.Store.UpdateRunRequest(ctx, run); err != nil {
		logger.Warn("failed to persist sandbox IDs", "error", err)
	}
	logger.Info("batch sandboxes created", "count", len(sandboxes))

	// Actual worker count may be less than requested if some failed.
	actualWorkers := len(sandboxes)

	// 3. Clone repo in ALL sandboxes concurrently (tolerant of individual failures).
	cloneCmd := fmt.Sprintf("git clone --depth=1 https://x-access-token:%s@github.com/%s.git /workspace/repo", cfg.GitHubToken, run.Repo)

	cloneOK := make([]bool, len(sandboxes))
	var wgClone sync.WaitGroup
	for i, sb := range sandboxes {
		wgClone.Add(1)
		go func() {
			defer wgClone.Done()
			result, err := sb.ExecuteShellCommand(ctx, cloneCmd, "", nil, "/workspace")
			if err != nil || (result != nil && result.ExitCode != 0) {
				stderr := ""
				if result != nil {
					stderr = result.Stderr
				}
				logger.Warn("clone failed in sandbox, will skip",
					"worker", i, "sandbox", sb.ID(),
					"error", sanitizeToken(stderr, cfg.GitHubToken))
				return
			}
			cloneOK[i] = true
			logger.Debug("repo cloned in sandbox", "sandbox", sb.ID(), "worker", i)
		}()
	}
	wgClone.Wait()

	// Keep only sandboxes that cloned successfully.
	var healthySandboxes []*adcsdk.Sandbox
	for i, sb := range sandboxes {
		if cloneOK[i] {
			healthySandboxes = append(healthySandboxes, sb)
		}
	}
	if len(healthySandboxes) == 0 {
		msg := "all sandboxes failed to clone repo"
		logger.Error(msg)
		markFailed(cfg.Store, run, msg)
		return fmt.Errorf("adc parallel clone: %s", msg)
	}
	if len(healthySandboxes) < len(sandboxes) {
		logger.Warn("some sandboxes failed clone, continuing with healthy ones",
			"healthy", len(healthySandboxes), "total", len(sandboxes))
	}
	sandboxes = healthySandboxes
	actualWorkers = len(sandboxes)
	logger.Info("repo cloned", "healthySandboxes", actualWorkers)

	// 4. Discover task files from sandbox 0 by reading the eval spec.
	taskFiles, err := discoverTaskFiles(ctx, sandboxes[0], run.EvalSpec, logger)
	if err != nil {
		msg := fmt.Sprintf("task discovery failed: %v", err)
		logger.Error(msg)
		markFailed(cfg.Store, run, truncate(msg, 500))
		return fmt.Errorf("adc task discovery: %w", err)
	}
	logger.Info("discovered tasks", "count", len(taskFiles))

	// Clamp workers to task count — no point having more sandboxes than tasks.
	if actualWorkers > len(taskFiles) {
		actualWorkers = len(taskFiles)
		sandboxes = sandboxes[:actualWorkers]
		logger.Info("reduced workers to match task count", "workers", actualWorkers)
	}

	// 5. Distribute tasks round-robin and create shard evals.
	shards := distributeTaskFiles(taskFiles, actualWorkers)

	// Track which tasks each worker is responsible for.
	run.WorkerTasks = make(map[string][]string, actualWorkers)
	for i, sb := range sandboxes {
		run.WorkerTasks[sb.ID()] = shards[i]
	}
	_ = cfg.Store.UpdateRunRequest(ctx, run) // best-effort persist

	// Read the original eval YAML for shard creation.
	evalYAML, err := readSandboxFile(ctx, sandboxes[0], "/workspace/repo/"+run.EvalSpec)
	if err != nil {
		msg := fmt.Sprintf("failed to read eval spec: %v", err)
		logger.Error(msg)
		markFailed(cfg.Store, run, truncate(msg, 500))
		return fmt.Errorf("adc read eval: %w", err)
	}

	executor := cfg.Executor
	if executor == "" {
		executor = "copilot-sdk"
	}

	// 6. Write shard evals and run waza in each sandbox concurrently.
	results := make([]sandboxResult, actualWorkers)
	var wg sync.WaitGroup

	for i, sb := range sandboxes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = runSandboxShard(ctx, sb, runShardConfig{
				workerIdx:   i,
				evalYAML:    evalYAML,
				taskFiles:   shards[i],
				evalSpec:    run.EvalSpec,
				model:       run.Model,
				executor:    executor,
				githubToken: cfg.GitHubToken,
				logger:      logger.With("worker", i, "sandbox", sb.ID()),
			})
		}()
	}

	// 7. Poll log tails while workers are running.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	pollLogTails(ctx, cfg, sandboxes, done, logger)

	// 8. Collect and merge results.
	var validResults []json.RawMessage
	var allLogs []string
	var firstErr error
	for i, r := range results {
		if r.Err != nil {
			logger.Error("sandbox worker failed", "worker", i, "sandbox", r.SandboxID, "error", r.Err)
			if firstErr == nil {
				firstErr = r.Err
			}
			continue
		}
		if r.ResultJSON != nil {
			validResults = append(validResults, r.ResultJSON)
		}
		if r.LogTail != "" {
			allLogs = append(allLogs, fmt.Sprintf("=== Worker %d (%s) ===\n%s", i, r.SandboxID, r.LogTail))
		}
	}

	if len(validResults) == 0 {
		msg := "all sandbox workers failed"
		if firstErr != nil {
			msg = fmt.Sprintf("all workers failed, first error: %v", firstErr)
		}
		logger.Error(msg)
		markFailed(cfg.Store, run, truncate(msg, 500))
		return fmt.Errorf("adc parallel: %s", msg)
	}

	// Persist combined log tails.
	run.LogTail = strings.Join(allLogs, "\n\n")
	_ = cfg.Store.UpdateRunRequest(ctx, run) // best-effort

	// 9. Merge results from all sandboxes.
	merged, err := mergeResults(validResults)
	if err != nil {
		msg := fmt.Sprintf("failed to merge results: %v", err)
		logger.Error(msg)
		markFailed(cfg.Store, run, truncate(msg, 500))
		return fmt.Errorf("adc merge: %w", err)
	}
	logger.Info("merged results from workers", "shards", len(validResults), "totalTasks", len(taskFiles))

	// 10. Save to Cosmos.
	logger.Info("saving merged results", "size", len(merged))
	if err := cfg.Store.SaveResult(ctx, run.UserID, run.ID, merged); err != nil {
		msg := fmt.Sprintf("failed to save merged results to Cosmos: %v", err)
		logger.Error(msg, "error", err, "resultSizeBytes", len(merged))
		markFailed(cfg.Store, run, truncate(msg, 500))
		return fmt.Errorf("save result: %w", err)
	}
	logger.Info("parallel ADC results saved to Cosmos")

	// 11. Mark complete.
	now := time.Now()
	run.Status = db.Complete
	run.CompletedAt = &now
	if err := cfg.Store.UpdateRunRequest(ctx, run); err != nil {
		logger.Error("failed to mark run complete", "error", err)
		return fmt.Errorf("update run complete: %w", err)
	}

	logger.Info("parallel ADC run completed",
		"sandboxes", actualWorkers,
		"tasks", len(taskFiles),
		"duration", time.Since(run.CreatedAt).Round(time.Second),
	)
	return nil
}

// runShardConfig configures a single sandbox worker's execution.
type runShardConfig struct {
	workerIdx   int
	evalYAML    string
	taskFiles   []string
	evalSpec    string
	model       string
	executor    string
	githubToken string
	logger      *slog.Logger
}

// runSandboxShard clones, writes a shard eval, runs waza, and reads results
// for a single sandbox worker.
func runSandboxShard(ctx context.Context, sb *adcsdk.Sandbox, cfg runShardConfig) sandboxResult {
	result := sandboxResult{SandboxID: sb.ID()}

	// Create shard eval YAML with only this worker's tasks.
	shardYAML, err := createShardEval(cfg.evalYAML, cfg.taskFiles)
	if err != nil {
		result.Err = fmt.Errorf("worker %d: create shard eval: %w", cfg.workerIdx, err)
		return result
	}

	// Write shard eval to sandbox.
	shardPath := fmt.Sprintf("/workspace/repo/shard-%d-eval.yaml", cfg.workerIdx)
	_, err = sb.WriteFileText(ctx, shardPath, shardYAML, nil)
	if err != nil {
		result.Err = fmt.Errorf("worker %d: write shard eval: %w", cfg.workerIdx, err)
		return result
	}
	cfg.logger.Debug("shard eval written", "tasks", len(cfg.taskFiles))

	// Build waza command using the shard eval.
	wazaCmd := fmt.Sprintf(
		"cd /workspace/repo && nohup waza run shard-%d-eval.yaml -o /workspace/results-%d.json --executor %s",
		cfg.workerIdx, cfg.workerIdx, cfg.executor)
	if cfg.model != "" {
		wazaCmd += " --model " + cfg.model
	}
	wazaCmd += fmt.Sprintf(" > /workspace/waza-%d.log 2>&1 & echo $!", cfg.workerIdx)

	// Start waza in background.
	startResult, err := sb.ExecuteShellCommand(ctx, wazaCmd, "", nil, "/workspace/repo")
	if err != nil {
		result.Err = fmt.Errorf("worker %d: start waza: %w", cfg.workerIdx, err)
		return result
	}
	pid := strings.TrimSpace(startResult.Stdout)
	cfg.logger.Info("waza started", "pid", pid)

	// Poll for completion.
	pollCmd := fmt.Sprintf("kill -0 %s 2>/dev/null && echo RUNNING || echo DONE", pid)
	for {
		select {
		case <-ctx.Done():
			result.Err = fmt.Errorf("worker %d: timed out", cfg.workerIdx)
			return result
		case <-time.After(15 * time.Second):
		}

		pollResult, err := sb.ExecuteShellCommand(ctx, pollCmd, "", nil, "")
		if err != nil {
			cfg.logger.Warn("poll failed, retrying", "error", err)
			continue
		}
		if strings.TrimSpace(pollResult.Stdout) == "DONE" {
			break
		}
		cfg.logger.Debug("waza still running", "pid", pid)
	}

	// Check exit code.
	logFile := fmt.Sprintf("/workspace/waza-%d.log", cfg.workerIdx)
	exitResult, _ := sb.ExecuteShellCommand(ctx, "wait "+pid+" 2>/dev/null; tail -5 "+logFile, "", nil, "")
	exitCode := 0
	if exitResult != nil {
		exitCode = exitResult.ExitCode
		result.LogTail = exitResult.Stdout
	}

	if exitCode > 1 {
		result.Err = fmt.Errorf("worker %d: waza exited with code %d", cfg.workerIdx, exitCode)
		return result
	}
	cfg.logger.Info("waza completed", "exitCode", exitCode)

	// Read results.
	resultsPath := fmt.Sprintf("/workspace/results-%d.json", cfg.workerIdx)
	resultsJSON, err := sb.ReadFileText(ctx, resultsPath)
	if err != nil {
		result.Err = fmt.Errorf("worker %d: read results: %w", cfg.workerIdx, err)
		return result
	}

	if !json.Valid([]byte(resultsJSON)) {
		result.Err = fmt.Errorf("worker %d: invalid results JSON", cfg.workerIdx)
		return result
	}

	result.ResultJSON = json.RawMessage(resultsJSON)
	return result
}

// discoverTaskFiles reads the eval YAML from the sandbox, parses out the task
// globs, resolves them against the repo, and returns individual task file paths.
func discoverTaskFiles(ctx context.Context, sb *adcsdk.Sandbox, evalSpec string, logger *slog.Logger) ([]string, error) {
	// Read the eval YAML.
	content, err := sb.ReadFileText(ctx, "/workspace/repo/"+evalSpec)
	if err != nil {
		return nil, fmt.Errorf("reading eval spec: %w", err)
	}

	// Parse just the tasks field.
	var spec struct {
		Tasks []string `yaml:"tasks"`
	}
	if err := yaml.Unmarshal([]byte(content), &spec); err != nil {
		return nil, fmt.Errorf("parsing eval YAML: %w", err)
	}

	if len(spec.Tasks) == 0 {
		return nil, fmt.Errorf("eval spec has no tasks defined")
	}

	// Resolve globs inside the sandbox to get actual file paths.
	// The eval dir is the directory containing the eval spec.
	evalDir := filepath.Dir(evalSpec)
	var globExprs []string
	for _, pattern := range spec.Tasks {
		globExprs = append(globExprs, pattern)
	}

	// Use bash glob expansion in the sandbox.
	lsCmd := fmt.Sprintf("cd /workspace/repo/%s && ls -1 %s 2>/dev/null | sort",
		evalDir, strings.Join(globExprs, " "))

	result, err := sb.ExecuteShellCommand(ctx, lsCmd, "", nil, "")
	if err != nil {
		return nil, fmt.Errorf("resolving task globs: %w", err)
	}

	var taskFiles []string
	for _, line := range strings.Split(strings.TrimSpace(result.Stdout), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			// Prefix with evalDir so paths work from repo root (shard evals are at repo root).
			taskFiles = append(taskFiles, filepath.Join(evalDir, line))
		}
	}

	if len(taskFiles) == 0 {
		return nil, fmt.Errorf("no task files matched globs: %v", spec.Tasks)
	}

	logger.Debug("resolved task globs", "patterns", spec.Tasks, "files", taskFiles)
	return taskFiles, nil
}

// distributeTaskFiles splits task files round-robin across N workers.
func distributeTaskFiles(tasks []string, workers int) [][]string {
	shards := make([][]string, workers)
	for i, task := range tasks {
		idx := i % workers
		shards[idx] = append(shards[idx], task)
	}
	return shards
}

// createShardEval takes the original eval YAML and replaces the tasks field
// with explicit task file paths for a single shard.
func createShardEval(originalYAML string, taskFiles []string) (string, error) {
	var spec map[string]any
	if err := yaml.Unmarshal([]byte(originalYAML), &spec); err != nil {
		return "", fmt.Errorf("parsing eval YAML: %w", err)
	}

	// Replace the tasks field with explicit file paths (no globs).
	spec["tasks"] = taskFiles

	out, err := yaml.Marshal(spec)
	if err != nil {
		return "", fmt.Errorf("marshaling shard eval: %w", err)
	}

	return string(out), nil
}

// readSandboxFile reads a text file from a sandbox.
func readSandboxFile(ctx context.Context, sb *adcsdk.Sandbox, path string) (string, error) {
	return sb.ReadFileText(ctx, path)
}

// pollLogTails periodically tails waza logs from all sandboxes and persists
// combined log output to the run record. Stops when done channel is closed.
func pollLogTails(ctx context.Context, cfg RunConfig, sandboxes []*adcsdk.Sandbox, done <-chan struct{}, logger *slog.Logger) {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		var parts []string
		for i, sb := range sandboxes {
			logFile := fmt.Sprintf("/workspace/waza-%d.log", i)
			tailCmd := fmt.Sprintf("tail -15 %s 2>/dev/null", logFile)
			result, err := sb.ExecuteShellCommand(ctx, tailCmd, "", nil, "")
			if err == nil && result != nil && result.ExitCode == 0 && result.Stdout != "" {
				parts = append(parts, fmt.Sprintf("[worker-%d] %s", i, result.Stdout))
			}
		}

		if len(parts) > 0 {
			cfg.Run.LogTail = strings.Join(parts, "\n---\n")
			_ = cfg.Store.UpdateRunRequest(ctx, cfg.Run) // best-effort
		}
	}
}

// mergeResults combines EvaluationOutcome JSON from multiple sandbox workers
// into a single result. Tasks are concatenated and the summary is recalculated.
func mergeResults(results []json.RawMessage) (json.RawMessage, error) {
	if len(results) == 0 {
		return nil, fmt.Errorf("no results to merge")
	}
	if len(results) == 1 {
		return results[0], nil
	}

	// Parse all results as generic maps for flexible merging.
	var base map[string]any
	if err := json.Unmarshal(results[0], &base); err != nil {
		return nil, fmt.Errorf("parsing base result: %w", err)
	}

	// Collect all tasks from all shards.
	allTasks, _ := base["tasks"].([]any)

	for _, raw := range results[1:] {
		var shard map[string]any
		if err := json.Unmarshal(raw, &shard); err != nil {
			continue
		}
		if tasks, ok := shard["tasks"].([]any); ok {
			allTasks = append(allTasks, tasks...)
		}

		// Merge usage stats if present.
		mergeUsageStats(base, shard)
	}

	base["tasks"] = allTasks

	// Recalculate the summary from merged tasks.
	recalculateSummary(base, allTasks)

	out, err := json.Marshal(base)
	if err != nil {
		return nil, fmt.Errorf("marshaling merged result: %w", err)
	}

	return json.RawMessage(out), nil
}

// recalculateSummary updates the summary/digest fields based on merged tasks.
func recalculateSummary(result map[string]any, tasks []any) {
	total := len(tasks)
	succeeded := 0
	failed := 0
	errors := 0
	skipped := 0
	var totalScore float64
	var maxDuration int64

	for _, t := range tasks {
		task, ok := t.(map[string]any)
		if !ok {
			continue
		}
		status, _ := task["status"].(string)
		switch status {
		case "passed":
			succeeded++
		case "failed":
			failed++
		case "error":
			errors++
		case "skipped":
			skipped++
		}

		// Accumulate scores from runs/trials.
		if runs, ok := task["runs"].([]any); ok {
			for _, r := range runs {
				if run, ok := r.(map[string]any); ok {
					if dur, ok := run["duration_ms"].(float64); ok && int64(dur) > maxDuration {
						maxDuration = int64(dur)
					}
				}
			}
		}

		// Try extracting score from stats.
		if stats, ok := task["stats"].(map[string]any); ok {
			if avg, ok := stats["avg_score"].(float64); ok {
				totalScore += avg
			}
		}
	}

	successRate := 0.0
	avgScore := 0.0
	if total > 0 {
		successRate = float64(succeeded) / float64(total)
		avgScore = totalScore / float64(total)
	}

	// Update the summary object (handles both "summary" and "digest" keys).
	for _, key := range []string{"summary", "digest"} {
		if s, ok := result[key].(map[string]any); ok {
			s["total_tests"] = total
			s["succeeded"] = succeeded
			s["failed"] = failed
			s["errors"] = errors
			s["skipped"] = skipped
			s["success_rate"] = successRate
			s["pass_rate"] = successRate
			s["aggregate_score"] = avgScore
			s["total_tasks"] = total
			s["passed"] = succeeded
			break
		}
	}
}

// mergeUsageStats adds usage stats from a shard into the base result.
func mergeUsageStats(base, shard map[string]any) {
	// Look for usage in summary/digest.
	for _, key := range []string{"summary", "digest"} {
		baseDigest, ok1 := base[key].(map[string]any)
		shardDigest, ok2 := shard[key].(map[string]any)
		if !ok1 || !ok2 {
			continue
		}

		baseUsage, ok1 := baseDigest["usage"].(map[string]any)
		shardUsage, ok2 := shardDigest["usage"].(map[string]any)
		if !ok1 || !ok2 {
			continue
		}

		// Sum numeric fields.
		for _, field := range []string{"turns", "input_tokens", "output_tokens", "cache_read_tokens", "cache_write_tokens"} {
			bv, _ := baseUsage[field].(float64)
			sv, _ := shardUsage[field].(float64)
			baseUsage[field] = bv + sv
		}
		break
	}
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
