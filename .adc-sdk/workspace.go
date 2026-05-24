package adc

import (
	"context"
	"fmt"
	"net/http"
)

// Workspace represents an ADC workspace resource.
type Workspace struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	CreatedAt   string            `json:"createdAt,omitempty"`
	UpdatedAt   string            `json:"updatedAt,omitempty"`
}

// WorkspaceList holds a list of workspaces returned by the API.
type WorkspaceList struct {
	Items    []Workspace `json:"items"`
	NextPage string      `json:"nextPage,omitempty"`
}

// WorkspaceAPI provides methods for interacting with workspace resources.
type WorkspaceAPI struct {
	client *Client
}

// NewWorkspaceAPI creates a new WorkspaceAPI backed by the given Client.
func NewWorkspaceAPI(client *Client) *WorkspaceAPI {
	return &WorkspaceAPI{client: client}
}

// List returns all workspaces visible to the authenticated user,
// optionally filtered by the provided ListOptions.
func (w *WorkspaceAPI) List(ctx context.Context, opts *ListOptions) ([]Workspace, error) {
	path := "/workspaces"
	if opts != nil {
		if qs := buildListOptions(opts); qs != "" {
			path = fmt.Sprintf("%s?%s", path, qs)
		}
	}

	var result WorkspaceList
	if err := w.client.do(ctx, http.MethodGet, path, nil, &result); err != nil {
		return nil, fmt.Errorf("workspace list: %w", err)
	}
	return result.Items, nil
}

// Get retrieves a single workspace by its ID.
func (w *WorkspaceAPI) Get(ctx context.Context, id string) (*Workspace, error) {
	if id == "" {
		return nil, fmt.Errorf("workspace get: id must not be empty")
	}

	var result Workspace
	if err := w.client.do(ctx, http.MethodGet, fmt.Sprintf("/workspaces/%s", id), nil, &result); err != nil {
		return nil, fmt.Errorf("workspace get %q: %w", id, err)
	}
	return &result, nil
}

// Create creates a new workspace with the given name and optional fields.
func (w *WorkspaceAPI) Create(ctx context.Context, workspace *Workspace) (*Workspace, error) {
	if workspace == nil {
		return nil, fmt.Errorf("workspace create: workspace must not be nil")
	}
	if workspace.Name == "" {
		return nil, fmt.Errorf("workspace create: name must not be empty")
	}

	var result Workspace
	if err := w.client.do(ctx, http.MethodPost, "/workspaces", workspace, &result); err != nil {
		return nil, fmt.Errorf("workspace create %q: %w", workspace.Name, err)
	}
	return &result, nil
}

// Update updates an existing workspace identified by its ID.
func (w *WorkspaceAPI) Update(ctx context.Context, id string, workspace *Workspace) (*Workspace, error) {
	if id == "" {
		return nil, fmt.Errorf("workspace update: id must not be empty")
	}
	if workspace == nil {
		return nil, fmt.Errorf("workspace update: workspace must not be nil")
	}

	var result Workspace
	if err := w.client.do(ctx, http.MethodPut, fmt.Sprintf("/workspaces/%s", id), workspace, &result); err != nil {
		return nil, fmt.Errorf("workspace update %q: %w", id, err)
	}
	return &result, nil
}

// Delete removes the workspace with the given ID.
func (w *WorkspaceAPI) Delete(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("workspace delete: id must not be empty")
	}

	if err := w.client.do(ctx, http.MethodDelete, fmt.Sprintf("/workspaces/%s", id), nil, nil); err != nil {
		return fmt.Errorf("workspace delete %q: %w", id, err)
	}
	return nil
}
