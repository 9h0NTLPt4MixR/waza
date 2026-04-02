// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License. See LICENSE in the project root for license information.

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/microsoft/waza/internal/platform/adc"
	"github.com/microsoft/waza/internal/platform/auth"
	"github.com/microsoft/waza/internal/platform/db"
	"github.com/microsoft/waza/internal/platform/execution"
)

// maxRequestBody is the maximum body size accepted by mutation endpoints.
const maxRequestBody = 1 << 20 // 1 MiB

// --- Auth handlers ---

// handleMe returns the authenticated user from the request context.
func handleMe(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	// Return camelCase JSON matching the frontend User interface.
	resp := map[string]any{
		"githubId":  user.GitHubID,
		"login":     user.Login,
		"name":      user.Name,
		"avatarUrl": user.AvatarURL,
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleLogout clears the session cookie and revokes the server-side session.
func handleLogout(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract token so we can revoke it server-side.
		token := extractBearerOrCookie(r)
		if token != "" {
			if err := deps.Auth.RevokeSession(r.Context(), token); err != nil {
				slog.Warn("failed to revoke session", "error", err)
			}
		}

		// Clear the session cookie.
		http.SetCookie(w, &http.Cookie{
			Name:     "waza_session",
			Value:    "",
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
			Path:     "/",
		})
		w.WriteHeader(http.StatusNoContent)
	}
}

// --- Connection handlers ---

// handleListConnections returns the authenticated user's connections.
func handleListConnections(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.UserFromContext(r.Context())
		if user == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		typeFilter := db.ConnectionType(r.URL.Query().Get("type"))
		connections, err := deps.Store.ListConnections(r.Context(), user.GitHubID, typeFilter)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list connections")
			slog.Error("ListConnections", "error", err, "user", user.Login)
			return
		}
		if connections == nil {
			connections = []*db.Connection{}
		}
		writeJSON(w, http.StatusOK, connections)
	}
}

// createConnectionRequest is the JSON body for POST /api/connections.
type createConnectionRequest struct {
	Type   db.ConnectionType `json:"type"`
	Config map[string]any    `json:"config"`
}

// handleCreateConnection validates and saves a new connection.
func handleCreateConnection(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.UserFromContext(r.Context())
		if user == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		var req createConnectionRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if req.Type != db.AzureStorage && req.Type != db.GitHubRepo {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("unsupported connection type: %s", req.Type))
			return
		}
		if len(req.Config) == 0 {
			writeError(w, http.StatusBadRequest, "config is required")
			return
		}

		conn := &db.Connection{
			ID:     fmt.Sprintf("conn-%d", time.Now().UnixNano()),
			UserID: user.GitHubID,
			Type:   req.Type,
			Config: req.Config,
		}

		if err := deps.Store.SaveConnection(r.Context(), conn); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to save connection")
			slog.Error("SaveConnection", "error", err, "user", user.Login)
			return
		}
		writeJSON(w, http.StatusCreated, conn)
	}
}

// handleDeleteConnection removes a connection by ID after verifying ownership.
func handleDeleteConnection(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.UserFromContext(r.Context())
		if user == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		connID := r.PathValue("id")
		if connID == "" {
			writeError(w, http.StatusBadRequest, "connection id is required")
			return
		}

		if err := deps.Store.DeleteConnection(r.Context(), user.GitHubID, connID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to delete connection")
			slog.Error("DeleteConnection", "error", err, "user", user.Login, "id", connID)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// testConnectionRequest is the JSON body for POST /api/connections/test.
type testConnectionRequest struct {
	Type   db.ConnectionType `json:"type"`
	Config map[string]any    `json:"config"`
}

// testConnectionResponse is returned by the test endpoint.
type testConnectionResponse struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}

// handleTestConnection verifies connectivity for a connection config.
func handleTestConnection(deps *Dependencies) http.HandlerFunc {
	_ = deps // deps available for future use (e.g., proxy through store's decrypt)
	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.UserFromContext(r.Context())
		if user == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		var req testConnectionRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		switch req.Type {
		case db.AzureStorage:
			ok, msg := testAzureStorage(r, req.Config)
			writeJSON(w, http.StatusOK, testConnectionResponse{OK: ok, Message: msg})

		case db.GitHubRepo:
			ok, msg := testGitHubRepo(r, req.Config)
			writeJSON(w, http.StatusOK, testConnectionResponse{OK: ok, Message: msg})

		default:
			writeError(w, http.StatusBadRequest, fmt.Sprintf("unsupported connection type: %s", req.Type))
		}
	}
}

// testAzureStorage attempts to list blobs in the configured container.
func testAzureStorage(r *http.Request, config map[string]any) (bool, string) {
	account, _ := config["account_name"].(string)
	container, _ := config["container_name"].(string)
	sasToken, _ := config["sas_token"].(string)

	if account == "" || container == "" {
		return false, "account_name and container_name are required"
	}

	// Build a lightweight HTTP probe — list blobs with maxresults=1.
	url := fmt.Sprintf("https://%s.blob.core.windows.net/%s?restype=container&comp=list&maxresults=1",
		account, container)
	if sasToken != "" {
		url += "&" + sasToken
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, url, nil)
	if err != nil {
		return false, fmt.Sprintf("building request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, fmt.Sprintf("connection failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true, "connected successfully"
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return false, fmt.Sprintf("storage returned %d: %s", resp.StatusCode, string(body))
}

// testGitHubRepo attempts to list the repo contents via the GitHub API.
func testGitHubRepo(r *http.Request, config map[string]any) (bool, string) {
	owner, _ := config["owner"].(string)
	repo, _ := config["repo"].(string)
	token, _ := config["token"].(string)

	if owner == "" || repo == "" {
		return false, "owner and repo are required"
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s", owner, repo)
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, url, nil)
	if err != nil {
		return false, fmt.Sprintf("building request: %v", err)
	}

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "waza-platform")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, fmt.Sprintf("connection failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return true, "connected successfully"
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return false, fmt.Sprintf("GitHub API returned %d: %s", resp.StatusCode, string(body))
}

// --- Run handlers ---

// triggerRunRequest is the JSON body for POST /api/runs/trigger.
type triggerRunRequest struct {
	Owner              string `json:"owner"`                        // repo owner (frontend sends separately)
	Repo               string `json:"repo"`                        // repo name or "owner/repo"
	EvalPath           string `json:"evalPath"`                    // path to eval YAML within the repo
	EvalSpec           string `json:"eval_spec"`                   // alias for evalPath
	Model              string `json:"model"`
	Workers            int    `json:"workers"`
	StorageDestination string `json:"storageDestination"`          // "cosmos" (default) or connection ID
	Executor           string `json:"executor,omitempty"`          // executor override; defaults to "copilot-sdk"
}

// triggerRunResponse is returned by POST /api/runs/trigger so the frontend
// can redirect to the run status page.
type triggerRunResponse struct {
	RunID  string       `json:"runId"`
	Status db.RunStatus `json:"status"`
}

// runQueueItem is the camelCase JSON representation of a RunRequest sent to
// the frontend for list and detail views.
type runQueueItem struct {
	ID                 string       `json:"id"`
	Status             db.RunStatus `json:"status"`
	Repo               string       `json:"repo"`
	EvalSpec           string       `json:"evalSpec"`
	Model              string       `json:"model"`
	Workers            int          `json:"workers"`
	StorageDestination string       `json:"storageDestination"`
	Executor           string       `json:"executor,omitempty"`
	ADCSandboxIDs      []string     `json:"adcSandboxIds,omitempty"`
	LogTail            string       `json:"logTail,omitempty"`
	CreatedAt          time.Time    `json:"createdAt"`
	CompletedAt        *time.Time   `json:"completedAt,omitempty"`
	Error              string       `json:"error,omitempty"`
}

// toRunQueueItem maps a db.RunRequest to its camelCase API representation.
func toRunQueueItem(r *db.RunRequest) runQueueItem {
	return runQueueItem{
		ID:                 r.ID,
		Status:             r.Status,
		Repo:               r.Repo,
		EvalSpec:           r.EvalSpec,
		Model:              r.Model,
		Workers:            r.Workers,
		StorageDestination: r.StorageDestination,
		Executor:           r.Executor,
		ADCSandboxIDs:      r.ADCSandboxIDs,
		LogTail:            r.LogTail,
		CreatedAt:          r.CreatedAt,
		CompletedAt:        r.CompletedAt,
		Error:              r.Error,
	}
}

// handleTriggerRun creates a RunRequest and dispatches it to ADC asynchronously.
func handleTriggerRun(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.UserFromContext(r.Context())
		if user == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		var req triggerRunRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if req.Owner != "" && !strings.Contains(req.Repo, "/") {
			req.Repo = req.Owner + "/" + req.Repo
		}
		if req.EvalPath != "" && req.EvalSpec == "" {
			req.EvalSpec = req.EvalPath
		}

		if req.Repo == "" {
			writeError(w, http.StatusBadRequest, "repo is required")
			return
		}
		if req.EvalSpec == "" {
			writeError(w, http.StatusBadRequest, "eval_spec is required")
			return
		}
		if req.Model == "" {
			req.Model = "gpt-4o"
		}
		if req.Workers <= 0 {
			req.Workers = 1
		}
		if req.StorageDestination == "" {
			req.StorageDestination = "cosmos"
		}
		if req.Executor == "" {
			req.Executor = "copilot-sdk"
		}

		slog.Info("triggering run",
			"user", user.Login,
			"repo", req.Repo,
			"eval", req.EvalSpec,
			"model", req.Model,
			"executor", req.Executor,
			"storageDestination", req.StorageDestination,
		)

		run := &db.RunRequest{
			ID:                 fmt.Sprintf("run-%d", time.Now().UnixNano()),
			UserID:             user.GitHubID,
			Repo:               req.Repo,
			EvalSpec:           req.EvalSpec,
			Model:              req.Model,
			Workers:            req.Workers,
			StorageDestination: req.StorageDestination,
			Executor:           req.Executor,
		}

		if err := deps.Store.CreateRunRequest(r.Context(), run); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create run request")
			slog.Error("CreateRunRequest", "error", err, "user", user.Login)
			return
		}

		// Dispatch execution asynchronously. Uses local subprocess for now;
		// can be swapped to ADC sandbox later without changing this wiring.
		go dispatchRun(deps, run, user.GitHubToken)

		writeJSON(w, http.StatusAccepted, triggerRunResponse{
			RunID:  run.ID,
			Status: run.Status,
		})
	}
}

// dispatchRun executes the eval in a background goroutine. It wraps
// execution.RunEval with panic recovery so a failed run never crashes
// the server process.
func dispatchRun(deps *Dependencies, run *db.RunRequest, githubToken string) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("dispatchRun: recovered panic", "run", run.ID, "panic", r)
			now := time.Now()
			run.Status = db.Failed
			run.Error = fmt.Sprintf("internal panic: %v", r)
			run.CompletedAt = &now
			_ = deps.Store.UpdateRunRequest(context.Background(), run)
		}
	}()

	cfg := execution.RunConfig{
		Store:       deps.Store,
		Run:         run,
		GitHubToken: githubToken,
		Timeout:     execution.DefaultTimeout,
		Executor:    run.Executor,
	}

	// When ADC is configured, create a per-request ADC engine using the
	// platform-level config. The engine authenticates with the user's
	// GitHub token — no platform-level API key needed.
	if deps.ADCConfig != nil {
		cfg.ADCEngine = adc.NewEngine(*deps.ADCConfig)
	}

	if err := execution.RunEval(context.Background(), cfg); err != nil {
		slog.Error("dispatchRun: eval execution failed",
			"run", run.ID,
			"error", err,
		)
		// RunEval already marked the run as failed; nothing more to do.
	}
}

// handleListRuns returns the authenticated user's run queue.
func handleListRuns(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.UserFromContext(r.Context())
		if user == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		runs, err := deps.Store.ListRunRequests(r.Context(), user.GitHubID, 50)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list runs")
			slog.Error("ListRunRequests", "error", err, "user", user.Login)
			return
		}
		if runs == nil {
			runs = []*db.RunRequest{}
		}
		items := make([]runQueueItem, len(runs))
		for i, r := range runs {
			items[i] = toRunQueueItem(r)
		}
		writeJSON(w, http.StatusOK, items)
	}
}

// handleGetRun returns a single run request by ID.
func handleGetRun(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.UserFromContext(r.Context())
		if user == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		runID := r.PathValue("id")
		if runID == "" {
			writeError(w, http.StatusBadRequest, "run id is required")
			return
		}

		run, err := deps.Store.GetRunRequest(r.Context(), user.GitHubID, runID)
		if err != nil {
			writeError(w, http.StatusNotFound, "run not found")
			return
		}
		writeJSON(w, http.StatusOK, toRunQueueItem(run))
	}
}

// handleCancelRun cancels a running or queued eval.
func handleCancelRun(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.UserFromContext(r.Context())
		if user == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		runID := r.PathValue("id")
		if runID == "" {
			writeError(w, http.StatusBadRequest, "run id is required")
			return
		}

		run, err := deps.Store.GetRunRequest(r.Context(), user.GitHubID, runID)
		if err != nil {
			writeError(w, http.StatusNotFound, "run not found")
			return
		}

		if run.Status.Terminal() {
			writeError(w, http.StatusConflict, fmt.Sprintf("run is already %s", run.Status))
			return
		}

		now := time.Now().UTC()
		run.Status = db.Cancelled
		run.CompletedAt = &now

		if err := deps.Store.UpdateRunRequest(r.Context(), run); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to cancel run")
			slog.Error("CancelRun", "error", err, "user", user.Login, "run", runID)
			return
		}

		// Best-effort ADC sandbox cleanup.
		if deps.ADCConfig != nil && len(run.ADCSandboxIDs) > 0 {
			go func() {
				slog.Info("cleaning up ADC sandboxes for cancelled run", "run", runID)
				// Sandbox cleanup is handled by the engine's Shutdown or individual sandbox delete.
			}()
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// handleRerun clones the configuration from an existing run into a new queued
// run and dispatches it for execution. The original run must belong to the
// authenticated user (user isolation).
func handleRerun(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.UserFromContext(r.Context())
		if user == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		runID := r.PathValue("id")
		if runID == "" {
			writeError(w, http.StatusBadRequest, "run id is required")
			return
		}

		originalRun, err := deps.Store.GetRunRequest(r.Context(), user.GitHubID, runID)
		if err != nil {
			writeError(w, http.StatusNotFound, "run not found")
			return
		}

		newRun := &db.RunRequest{
			ID:                 fmt.Sprintf("run-%d", time.Now().UnixNano()),
			UserID:             user.GitHubID,
			Repo:               originalRun.Repo,
			EvalSpec:           originalRun.EvalSpec,
			Model:              originalRun.Model,
			Executor:           originalRun.Executor,
			Workers:            originalRun.Workers,
			StorageDestination: originalRun.StorageDestination,
			Status:             db.Queued,
			CreatedAt:          time.Now().UTC(),
		}

		if err := deps.Store.CreateRunRequest(r.Context(), newRun); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create rerun")
			slog.Error("Rerun:CreateRunRequest", "error", err, "user", user.Login, "original", runID)
			return
		}

		slog.Info("rerun created",
			"user", user.Login,
			"original", runID,
			"new", newRun.ID,
		)

		go dispatchRun(deps, newRun, user.GitHubToken)

		writeJSON(w, http.StatusCreated, triggerRunResponse{
			RunID:  newRun.ID,
			Status: newRun.Status,
		})
	}
}

// --- Repo handlers ---

// repoInfo is a slim representation of a GitHub repo.
type repoInfo struct {
	FullName    string `json:"fullName"`
	Description string `json:"description"`
	Private     bool   `json:"private"`
	HTMLURL     string `json:"htmlUrl"`
}

// handleListRepos lists the user's connected GitHub repos.
func handleListRepos(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.UserFromContext(r.Context())
		if user == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		connections, err := deps.Store.ListConnections(r.Context(), user.GitHubID, db.GitHubRepo)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list repos")
			slog.Error("ListRepos", "error", err, "user", user.Login)
			return
		}

		repos := make([]repoInfo, 0, len(connections))
		for _, conn := range connections {
			owner, _ := conn.Config["owner"].(string)
			repo, _ := conn.Config["repo"].(string)
			desc, _ := conn.Config["description"].(string)
			private, _ := conn.Config["private"].(bool)
			if owner != "" && repo != "" {
				repos = append(repos, repoInfo{
					FullName:    owner + "/" + repo,
					Description: desc,
					Private:     private,
					HTMLURL:     fmt.Sprintf("https://github.com/%s/%s", owner, repo),
				})
			}
		}
		writeJSON(w, http.StatusOK, repos)
	}
}

// evalFileInfo represents an eval.yaml file found in a repo.
type evalFileInfo struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

// handleListEvals discovers eval.yaml files in a GitHub repo.
func handleListEvals(deps *Dependencies) http.HandlerFunc {
	_ = deps
	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.UserFromContext(r.Context())
		if user == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		owner := r.PathValue("owner")
		repo := r.PathValue("repo")
		if owner == "" || repo == "" {
			writeError(w, http.StatusBadRequest, "owner and repo are required")
			return
		}

		// Search for eval.yaml files using the GitHub code search API.
		evals, err := searchEvalFiles(r, owner, repo, user)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to search for eval files")
			slog.Error("ListEvals", "error", err, "user", user.Login, "repo", owner+"/"+repo)
			return
		}
		writeJSON(w, http.StatusOK, evals)
	}
}

// searchEvalFiles uses the GitHub Git Trees API to find eval.yaml files in a repo.
// This is more reliable than the search API which can return intermittent 500s.
func searchEvalFiles(r *http.Request, owner, repo string, user *auth.User) ([]evalFileInfo, error) {
	// Use Git Trees API with recursive flag to get full file listing.
	url := fmt.Sprintf(
		"https://api.github.com/repos/%s/%s/git/trees/HEAD?recursive=1",
		owner, repo,
	)

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building tree request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "waza-platform")
	if user.GitHubToken != "" {
		req.Header.Set("Authorization", "Bearer "+user.GitHubToken)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GitHub tree request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("GitHub API returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Tree []struct {
			Path string `json:"path"`
			Type string `json:"type"`
		} `json:"tree"`
		Truncated bool `json:"truncated"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding tree response: %w", err)
	}

	var evals []evalFileInfo
	for _, entry := range result.Tree {
		if entry.Type != "blob" {
			continue
		}
		// Match files named eval.yaml or eval.yml
		base := filepath.Base(entry.Path)
		if base == "eval.yaml" || base == "eval.yml" {
			evals = append(evals, evalFileInfo{
				Path: entry.Path,
				Name: base,
			})
		}
	}
	return evals, nil
}

// --- Helpers ---

// writeJSON encodes v as JSON and writes it to w with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes a structured JSON error response.
func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// readJSON decodes a JSON request body into v with a size limit.
func readJSON(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, maxRequestBody)
	return json.NewDecoder(r.Body).Decode(v)
}

// extractBearerOrCookie reads the session token from Authorization header or cookie.
func extractBearerOrCookie(r *http.Request) string {
	if authHeader := r.Header.Get("Authorization"); len(authHeader) > 7 && strings.HasPrefix(authHeader, "Bearer ") {
		return authHeader[7:]
	}
	if cookie, err := r.Cookie("waza_session"); err == nil {
		return cookie.Value
	}
	return ""
}

// context is imported for the goroutine dispatch.
var _ = context.Background

// --- Result handlers ---

// handleListResults returns stored evaluation results for the authenticated user.
func handleListResults(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.UserFromContext(r.Context())
		if user == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		results, err := deps.Store.ListResults(r.Context(), user.GitHubID, 50)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list results")
			slog.Error("ListResults", "error", err, "user", user.Login)
			return
		}
		if results == nil {
			results = []db.ResultSummary{}
		}
		writeJSON(w, http.StatusOK, results)
	}
}

// handleGetResult returns a full evaluation result by run ID.
// It transforms the raw waza output into the RunDetail format the dashboard expects.
func handleGetResult(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.UserFromContext(r.Context())
		if user == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		runID := r.PathValue("id")
		if runID == "" {
			writeError(w, http.StatusBadRequest, "result id is required")
			return
		}

		result, err := deps.Store.GetResult(r.Context(), user.GitHubID, runID)
		if err != nil {
			writeError(w, http.StatusNotFound, "result not found")
			return
		}

		// Transform raw waza output to dashboard RunDetail format.
		var raw map[string]any
		if err := json.Unmarshal(result, &raw); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(result) //nolint:errcheck
			return
		}

		transformed := transformWazaResult(runID, raw)
		data, err := json.Marshal(transformed)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(result) //nolint:errcheck
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(data) //nolint:errcheck
	}
}

// transformWazaResult converts raw waza results.json into the RunDetail format.
func transformWazaResult(runID string, raw map[string]any) map[string]any {
	detail := map[string]any{
		"id":        runID,
		"spec":      stringVal(raw, "eval_name"),
		"model":     "",
		"outcome":   "unknown",
		"passCount": 0,
		"taskCount": 0,
		"tokens":    0,
		"cost":      0.0,
		"duration":  0.0,
		"timestamp": stringVal(raw, "timestamp"),
		"tasks":     []any{},
	}

	// Extract model from config.
	if config, ok := raw["config"].(map[string]any); ok {
		detail["model"] = stringVal(config, "model_id")
	}

	// Extract summary stats.
	if summary, ok := raw["summary"].(map[string]any); ok {
		totalTests := intVal(summary, "total_tests")
		succeeded := intVal(summary, "succeeded")
		failed := intVal(summary, "failed")
		detail["taskCount"] = totalTests
		detail["passCount"] = succeeded
		if totalTests > 0 && failed == 0 {
			detail["outcome"] = "pass"
		} else if totalTests > 0 {
			detail["outcome"] = "fail"
		}
		if dur, ok := summary["duration_ms"].(float64); ok {
			detail["duration"] = dur / 1000.0
		}
	}

	// Extract a composite pass threshold from metrics (used per-task when available).
	var passThreshold *float64
	if metrics, ok := raw["metrics"].(map[string]any); ok {
		// Use the task_completion threshold if present, otherwise pick the first metric's threshold.
		for _, key := range []string{"task_completion", "composite_score"} {
			if m, ok := metrics[key].(map[string]any); ok {
				if t, ok := m["threshold"].(float64); ok && t > 0 {
					passThreshold = &t
					break
				}
			}
		}
		if passThreshold == nil {
			for _, v := range metrics {
				if m, ok := v.(map[string]any); ok {
					if t, ok := m["threshold"].(float64); ok && t > 0 {
						passThreshold = &t
						break
					}
				}
			}
		}
	}

	// Transform tasks.
	if tasks, ok := raw["tasks"].([]any); ok {
		transformed := make([]map[string]any, 0, len(tasks))
		for _, t := range tasks {
			task, ok := t.(map[string]any)
			if !ok {
				continue
			}

			taskResult := map[string]any{
				"name":          stringVal(task, "display_name"),
				"outcome":       stringVal(task, "status"),
				"score":         0.0,
				"weightedScore": 0.0,
				"duration":      0.0,
				"graderResults": []any{},
			}

			// Extract stats.
			if stats, ok := task["stats"].(map[string]any); ok {
				if dur, ok := stats["avg_duration_ms"].(float64); ok {
					taskResult["duration"] = dur / 1000.0
				}
				if score, ok := stats["avg_score"].(float64); ok {
					taskResult["score"] = score
				}
				if ws, ok := stats["avg_weighted_score"].(float64); ok {
					taskResult["weightedScore"] = ws
				}
				if total, ok := stats["total_runs"].(float64); ok && total > 0 {
					taskResult["numTrials"] = int(total)
					if passed, ok := stats["passed_runs"].(float64); ok {
						taskResult["passedTrials"] = int(passed)
					}
					if failed, ok := stats["failed_runs"].(float64); ok {
						taskResult["failedTrials"] = int(failed)
					}
				}
			}

			// Extract grader results from first run's validations or error.
			if runs, ok := task["runs"].([]any); ok && len(runs) > 0 {
				if run, ok := runs[0].(map[string]any); ok {
					// Check for validations (grader results — stored as map[graderName]result).
					if validations, ok := run["validations"].(map[string]any); ok && len(validations) > 0 {
						graders := make([]map[string]any, 0, len(validations))
						for graderName, v := range validations {
							val, ok := v.(map[string]any)
							if !ok {
								continue
							}
							passed := false
							if p, ok := val["passed"].(bool); ok {
								passed = p
							}
							score := 0.0
							if s, ok := val["score"].(float64); ok {
								score = s
							}
							feedback := stringVal(val, "feedback")
							graderType := stringVal(val, "type")
							if graderType == "" {
								graderType = stringVal(val, "grader_type")
							}

							// Build details from the validation's details map.
							var details map[string]any
							if d, ok := val["details"].(map[string]any); ok {
								details = d
							}

							g := map[string]any{
								"name":    graderName,
								"type":    graderType,
								"passed":  passed,
								"score":   score,
								"message": feedback,
							}
							if details != nil {
								g["details"] = details
							}
							if w, ok := val["weight"].(float64); ok {
								g["weight"] = w
							}
							if dur, ok := val["duration_ms"].(float64); ok {
								g["durationMs"] = dur
							}
							graders = append(graders, g)
						}
						if len(graders) > 0 {
							taskResult["graderResults"] = graders
						}
					}
					// Fall back to error message if no validations.
					graders, hasGraders := taskResult["graderResults"].([]map[string]any)
					if (!hasGraders || len(graders) == 0) {
						if errMsg := stringVal(run, "error_msg"); errMsg != "" {
							taskResult["graderResults"] = []map[string]any{
								{
									"name":    "error",
									"type":    "error",
									"passed":  false,
									"score":   0,
									"message": errMsg,
								},
							}
						}
					}
				}
			}

			if passThreshold != nil {
				taskResult["passThreshold"] = *passThreshold
			}

			transformed = append(transformed, taskResult)
		}
		detail["tasks"] = transformed
	}

	return detail
}

// summaryResponse is the JSON payload for GET /api/summary — KPI cards on the dashboard.
type summaryResponse struct {
	TotalRuns   int     `json:"totalRuns"`
	TotalTasks  int     `json:"totalTasks"`
	PassRate    float64 `json:"passRate"`
	AvgTokens   float64 `json:"avgTokens"`
	AvgCost     float64 `json:"avgCost"`
	AvgDuration float64 `json:"avgDuration"`
}

// handleSummary aggregates KPI metrics from stored Cosmos results for the
// authenticated user.
func handleSummary(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.UserFromContext(r.Context())
		if user == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		results, err := deps.Store.ListResults(r.Context(), user.GitHubID, 0)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list results")
			slog.Error("handleSummary: ListResults", "error", err, "user", user.Login)
			return
		}

		resp := summaryResponse{}
		if len(results) == 0 {
			writeJSON(w, http.StatusOK, resp)
			return
		}

		resp.TotalRuns = len(results)
		var totalTokens, totalTasks, totalPass int
		var totalCost, totalDuration float64
		for _, rs := range results {
			totalTasks += rs.TaskCount
			totalPass += rs.PassCount
			totalTokens += rs.Tokens
			totalCost += rs.Cost
			totalDuration += rs.Duration
		}
		resp.TotalTasks = totalTasks
		if totalTasks > 0 {
			resp.PassRate = float64(totalPass) / float64(totalTasks)
		}
		n := float64(len(results))
		resp.AvgTokens = float64(totalTokens) / n
		resp.AvgCost = totalCost / n
		resp.AvgDuration = totalDuration / n

		writeJSON(w, http.StatusOK, resp)
	}
}

func intVal(m map[string]any, key string) int {
	if v, ok := m[key].(float64); ok {
		return int(v)
	}
	return 0
}

func stringVal(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
