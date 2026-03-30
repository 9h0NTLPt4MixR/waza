// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License. See LICENSE in the project root for license information.

// Package db defines the data persistence contracts for Waza Platform.
// The primary implementation targets Cosmos DB (serverless), but the
// interface-based design allows for alternative backends (e.g., in-memory
// for testing).
package db

import (
	"context"
	"time"

	"github.com/microsoft/waza/internal/platform/auth"
)

// ConnectionType identifies the kind of external service connection.
type ConnectionType string

const (
	// AzureStorage represents a user's Azure Blob Storage account (BYOS).
	AzureStorage ConnectionType = "azure_storage"
	// GitHubRepo represents a connected GitHub repository.
	GitHubRepo ConnectionType = "github_repo"
)

// RunStatus tracks the lifecycle of an evaluation run.
type RunStatus string

const (
	Queued    RunStatus = "queued"
	Running   RunStatus = "running"
	Complete  RunStatus = "complete"
	Failed    RunStatus = "failed"
	Cancelled RunStatus = "cancelled"
)

// Terminal reports whether the status represents a final state.
func (s RunStatus) Terminal() bool {
	return s == Complete || s == Failed || s == Cancelled
}

// Connection represents an external service linked by a user (e.g., Azure
// Storage for artifacts, GitHub repo for eval sources).
type Connection struct {
	ID         string         `json:"id"`
	UserID     int64          `json:"user_id"`
	Type       ConnectionType `json:"type"`
	Config     map[string]any `json:"config"` // provider-specific configuration
	VerifiedAt *time.Time     `json:"verified_at,omitempty"`
}

// Verified reports whether this connection has been successfully tested.
func (c *Connection) Verified() bool {
	return c.VerifiedAt != nil
}

// RunRequest represents a queued or in-progress evaluation run.
type RunRequest struct {
	ID            string    `json:"id"`
	UserID        int64     `json:"user_id"`
	Repo          string    `json:"repo"`           // "owner/repo" format
	EvalSpec      string    `json:"eval_spec"`      // path to eval YAML within the repo
	Model         string    `json:"model"`          // target model for evaluation
	Workers       int       `json:"workers"`        // parallel worker count
	Status        RunStatus `json:"status"`         // current lifecycle state
	ADCSandboxIDs []string  `json:"adc_sandbox_ids"` // allocated sandbox identifiers
	Error         string    `json:"error,omitempty"` // error message if failed
	CreatedAt     time.Time `json:"created_at"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
}

// Store defines the data persistence contract for Waza Platform.
// All methods are user-scoped — there is no cross-user data access in v1.
type Store interface {
	// --- Users ---

	// CreateUser persists a new user record. If the user already exists
	// (matched by GitHubID), the existing record is updated.
	CreateUser(ctx context.Context, user *auth.User) error

	// GetUser retrieves a user by their GitHub ID. Returns nil and no error
	// if the user does not exist.
	GetUser(ctx context.Context, githubID int64) (*auth.User, error)

	// --- Connections ---

	// SaveConnection creates or updates a connection for the given user.
	SaveConnection(ctx context.Context, conn *Connection) error

	// ListConnections returns all connections for a user, optionally filtered
	// by type. Pass empty string to list all types.
	ListConnections(ctx context.Context, userID int64, connType ConnectionType) ([]*Connection, error)

	// DeleteConnection removes a connection by ID. Returns an error if the
	// connection does not exist or does not belong to the specified user.
	DeleteConnection(ctx context.Context, userID int64, connectionID string) error

	// --- Runs ---

	// CreateRunRequest persists a new evaluation run request in Queued state.
	CreateRunRequest(ctx context.Context, run *RunRequest) error

	// UpdateRunRequest updates the status and metadata of an existing run.
	// Only non-terminal runs can be updated (enforced by implementation).
	UpdateRunRequest(ctx context.Context, run *RunRequest) error

	// ListRunRequests returns runs for a user, ordered by creation time
	// descending. The limit parameter caps the result count (0 = no limit).
	ListRunRequests(ctx context.Context, userID int64, limit int) ([]*RunRequest, error)

	// GetRunRequest retrieves a single run by ID. Returns an error if the
	// run does not exist or does not belong to the specified user.
	GetRunRequest(ctx context.Context, userID int64, runID string) (*RunRequest, error)

	// --- Settings ---

	// GetSetting retrieves a platform setting by key. Returns empty string
	// and no error if the key does not exist.
	GetSetting(ctx context.Context, key string) (string, error)

	// SetSetting creates or updates a platform setting.
	SetSetting(ctx context.Context, key, value string) error

	// --- Lifecycle ---

	// Close releases any resources held by the store (connection pools, etc.).
	Close() error
}
