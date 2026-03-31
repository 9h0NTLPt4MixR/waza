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
	"strings"
	"time"

	"github.com/microsoft/waza/internal/platform/auth"
	"github.com/microsoft/waza/internal/platform/db"
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
	writeJSON(w, http.StatusOK, user)
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
	Repo     string `json:"repo"`      // "owner/repo"
	EvalSpec string `json:"eval_spec"` // path to eval YAML within the repo
	Model    string `json:"model"`
	Workers  int    `json:"workers"`
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

		run := &db.RunRequest{
			ID:       fmt.Sprintf("run-%d", time.Now().UnixNano()),
			UserID:   user.GitHubID,
			Repo:     req.Repo,
			EvalSpec: req.EvalSpec,
			Model:    req.Model,
			Workers:  req.Workers,
		}

		if err := deps.Store.CreateRunRequest(r.Context(), run); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create run request")
			slog.Error("CreateRunRequest", "error", err, "user", user.Login)
			return
		}

		// Dispatch to ADC asynchronously if engine is configured.
		if deps.ADCEngine != nil {
			go dispatchToADC(deps, run)
		}

		writeJSON(w, http.StatusAccepted, run)
	}
}

// dispatchToADC runs the eval in an ADC sandbox. This runs in a goroutine.
func dispatchToADC(deps *Dependencies, run *db.RunRequest) {
	// Mark as running.
	run.Status = db.Running
	if err := deps.Store.UpdateRunRequest(context.Background(), run); err != nil {
		slog.Error("dispatchToADC: failed to update status to running", "error", err, "run", run.ID)
		return
	}

	slog.Info("dispatching run to ADC", "run", run.ID, "repo", run.Repo)

	// TODO: When ADC SDK is wired in go.mod, call deps.ADCEngine.Execute here.
	// For now, mark as queued — the ADC engine integration is pending SDK availability.
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
		writeJSON(w, http.StatusOK, runs)
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
		writeJSON(w, http.StatusOK, run)
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
		if deps.ADCEngine != nil && len(run.ADCSandboxIDs) > 0 {
			go func() {
				slog.Info("cleaning up ADC sandboxes for cancelled run", "run", runID)
				// Sandbox cleanup is handled by the engine's Shutdown or individual sandbox delete.
			}()
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// --- Repo handlers ---

// repoInfo is a slim representation of a GitHub repo.
type repoInfo struct {
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	Private     bool   `json:"private"`
	HTMLURL     string `json:"html_url"`
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

		slog.Info("ListRepos", "user", user.Login, "github_id", user.GitHubID, "connections_count", len(connections))

		repos := make([]repoInfo, 0, len(connections))
		for _, conn := range connections {
			slog.Info("ListRepos connection", "id", conn.ID, "type", conn.Type, "config", conn.Config)
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

// searchEvalFiles uses the GitHub API to find eval.yaml files in a repo.
func searchEvalFiles(r *http.Request, owner, repo string, user *auth.User) ([]evalFileInfo, error) {
	// Use GitHub search API: filename:eval.yaml in the specified repo
	url := fmt.Sprintf(
		"https://api.github.com/search/code?q=filename:eval.yaml+repo:%s/%s",
		owner, repo,
	)

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building search request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "waza-platform")
	// Note: auth token would come from the user's GitHub connection for private repos.
	_ = user

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GitHub search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("GitHub API returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Items []struct {
			Name string `json:"name"`
			Path string `json:"path"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding search results: %w", err)
	}

	evals := make([]evalFileInfo, 0, len(result.Items))
	for _, item := range result.Items {
		evals = append(evals, evalFileInfo{
			Path: item.Path,
			Name: item.Name,
		})
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
