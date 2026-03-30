// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License. See LICENSE in the project root for license information.

// Package api defines HTTP route registration and handlers for Waza Platform.
package api

import (
	"net/http"
)

// RegisterRoutes mounts all platform API routes on the given mux.
// Auth endpoints are public; all others require authentication.
func RegisterRoutes(mux *http.ServeMux, deps *Dependencies) {
	// --- Auth (public) ---
	mux.HandleFunc("GET /api/auth/github", deps.Auth.HandleLogin)
	mux.HandleFunc("GET /api/auth/callback", deps.Auth.HandleCallback)
	mux.Handle("GET /api/auth/me", deps.AuthMiddleware(http.HandlerFunc(handleMe)))
	mux.Handle("POST /api/auth/logout", deps.AuthMiddleware(http.HandlerFunc(handleLogout(deps))))

	// --- Connections (authenticated) ---
	mux.Handle("GET /api/connections", deps.AuthMiddleware(http.HandlerFunc(handleListConnections(deps))))
	mux.Handle("POST /api/connections", deps.AuthMiddleware(http.HandlerFunc(handleCreateConnection(deps))))
	mux.Handle("DELETE /api/connections/{id}", deps.AuthMiddleware(http.HandlerFunc(handleDeleteConnection(deps))))
	mux.Handle("POST /api/connections/test", deps.AuthMiddleware(http.HandlerFunc(handleTestConnection(deps))))

	// --- Runs (authenticated) ---
	mux.Handle("POST /api/runs/trigger", deps.AuthMiddleware(http.HandlerFunc(handleTriggerRun(deps))))
	mux.Handle("GET /api/runs/queue", deps.AuthMiddleware(http.HandlerFunc(handleListRuns(deps))))
	mux.Handle("GET /api/runs/{id}", deps.AuthMiddleware(http.HandlerFunc(handleGetRun(deps))))
	mux.Handle("POST /api/runs/cancel/{id}", deps.AuthMiddleware(http.HandlerFunc(handleCancelRun(deps))))

	// --- Repos (authenticated) ---
	mux.Handle("GET /api/repos", deps.AuthMiddleware(http.HandlerFunc(handleListRepos(deps))))
	mux.Handle("GET /api/repos/{owner}/{repo}/evals", deps.AuthMiddleware(http.HandlerFunc(handleListEvals(deps))))
}
