package adc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// WorkspaceInstance represents a single workspace resource and provides
// methods to interact with it.
type WorkspaceInstance struct {
	client    *Client
	workspace *Workspace
}

// Workspace holds the data model for a workspace object returned by the API.
type Workspace struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Labels      map[string]string `json:"labels,omitempty"`
	Status      string            `json:"status"`
	CreatedAt   string            `json:"createdAt"`
	UpdatedAt   string            `json:"updatedAt"`
}

// NewWorkspaceInstance creates a new WorkspaceInstance wrapping the given
// Workspace data and the shared API client.
func NewWorkspaceInstance(client *Client, workspace *Workspace) *WorkspaceInstance {
	return &WorkspaceInstance{
		client:    client,
		workspace: workspace,
	}
}

// Get returns the underlying Workspace data held by this instance.
func (wi *WorkspaceInstance) Get() *Workspace {
	return wi.workspace
}

// Refresh fetches the latest state of the workspace from the API and updates
// the local copy. Returns an error if the request fails.
func (wi *WorkspaceInstance) Refresh(ctx context.Context) error {
	path := fmt.Sprintf("/workspaces/%s", wi.workspace.ID)

	resp, err := wi.client.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return fmt.Errorf("workspace refresh: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("workspace refresh: unexpected status %d", resp.StatusCode)
	}

	var updated Workspace
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		return fmt.Errorf("workspace refresh: decode response: %w", err)
	}

	wi.workspace = &updated
	return nil
}

// Update applies a partial update (name and/or description) to the workspace
// and refreshes the local copy with the response from the API.
func (wi *WorkspaceInstance) Update(ctx context.Context, name, description string) error {
	patch := map[string]string{}
	if name != "" {
		patch["name"] = name
	}
	if description != "" {
		patch["description"] = description
	}

	path := fmt.Sprintf("/workspaces/%s", wi.workspace.ID)

	resp, err := wi.client.doRequest(ctx, http.MethodPatch, path, patch)
	if err != nil {
		return fmt.Errorf("workspace update: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("workspace update: unexpected status %d", resp.StatusCode)
	}

	var updated Workspace
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		return fmt.Errorf("workspace update: decode response: %w", err)
	}

	wi.workspace = &updated
	return nil
}

// Delete removes the workspace from the API. After a successful deletion the
// local instance should be discarded.
func (wi *WorkspaceInstance) Delete(ctx context.Context) error {
	path := fmt.Sprintf("/workspaces/%s", wi.workspace.ID)

	resp, err := wi.client.doRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return fmt.Errorf("workspace delete: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("workspace delete: unexpected status %d", resp.StatusCode)
	}

	return nil
}

// SetLabels replaces the label set on the workspace with the provided map and
// refreshes the local copy.
func (wi *WorkspaceInstance) SetLabels(ctx context.Context, labels map[string]string) error {
	patch := map[string]interface{}{
		"labels": labels,
	}

	path := fmt.Sprintf("/workspaces/%s", wi.workspace.ID)

	resp, err := wi.client.doRequest(ctx, http.MethodPatch, path, patch)
	if err != nil {
		return fmt.Errorf("workspace set labels: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("workspace set labels: unexpected status %d", resp.StatusCode)
	}

	var updated Workspace
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		return fmt.Errorf("workspace set labels: decode response: %w", err)
	}

	wi.workspace = &updated
	return nil
}
