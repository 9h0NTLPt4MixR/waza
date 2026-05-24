package adc

import (
	"context"
	"fmt"
	"net/http"
)

// DeploymentAPI provides methods for managing deployments within a workspace.
type DeploymentAPI struct {
	client    *Client
	workspace string
}

// NewDeploymentAPI creates a new DeploymentAPI instance scoped to the given workspace.
func NewDeploymentAPI(client *Client, workspace string) *DeploymentAPI {
	return &DeploymentAPI{
		client:    client,
		workspace: workspace,
	}
}

// Deployment represents a deployed application instance within a workspace.
type Deployment struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	AppID       string            `json:"appId"`
	Workspace   string            `json:"workspace"`
	Status      string            `json:"status"`
	Replicas    int               `json:"replicas"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	CreatedAt   string            `json:"createdAt"`
	UpdatedAt   string            `json:"updatedAt"`
}

// DeploymentSpec defines the desired state of a deployment.
type DeploymentSpec struct {
	Name      string            `json:"name"`
	AppID     string            `json:"appId"`
	Replicas  int               `json:"replicas,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
	EnvVars   map[string]string `json:"envVars,omitempty"`
}

// ListDeployments returns all deployments within the workspace, optionally filtered by labels.
func (d *DeploymentAPI) ListDeployments(ctx context.Context, labels map[string]string) ([]Deployment, error) {
	path := fmt.Sprintf("/workspaces/%s/deployments", d.workspace)
	if len(labels) > 0 {
		path += "?" + labelsToQueryString(labels)
	}

	var deployments []Deployment
	if err := d.client.do(ctx, http.MethodGet, path, nil, &deployments); err != nil {
		return nil, fmt.Errorf("listing deployments: %w", err)
	}
	return deployments, nil
}

// GetDeployment retrieves a single deployment by name.
func (d *DeploymentAPI) GetDeployment(ctx context.Context, name string) (*Deployment, error) {
	path := fmt.Sprintf("/workspaces/%s/deployments/%s", d.workspace, name)

	var deployment Deployment
	if err := d.client.do(ctx, http.MethodGet, path, nil, &deployment); err != nil {
		return nil, fmt.Errorf("getting deployment %q: %w", name, err)
	}
	return &deployment, nil
}

// CreateDeployment creates a new deployment within the workspace.
func (d *DeploymentAPI) CreateDeployment(ctx context.Context, spec DeploymentSpec) (*Deployment, error) {
	path := fmt.Sprintf("/workspaces/%s/deployments", d.workspace)

	var deployment Deployment
	if err := d.client.do(ctx, http.MethodPost, path, spec, &deployment); err != nil {
		return nil, fmt.Errorf("creating deployment %q: %w", spec.Name, err)
	}
	return &deployment, nil
}

// UpdateDeployment updates an existing deployment with the provided spec.
func (d *DeploymentAPI) UpdateDeployment(ctx context.Context, name string, spec DeploymentSpec) (*Deployment, error) {
	path := fmt.Sprintf("/workspaces/%s/deployments/%s", d.workspace, name)

	var deployment Deployment
	if err := d.client.do(ctx, http.MethodPut, path, spec, &deployment); err != nil {
		return nil, fmt.Errorf("updating deployment %q: %w", name, err)
	}
	return &deployment, nil
}

// DeleteDeployment removes a deployment by name from the workspace.
func (d *DeploymentAPI) DeleteDeployment(ctx context.Context, name string) error {
	path := fmt.Sprintf("/workspaces/%s/deployments/%s", d.workspace, name)

	if err := d.client.do(ctx, http.MethodDelete, path, nil, nil); err != nil {
		return fmt.Errorf("deleting deployment %q: %w", name, err)
	}
	return nil
}

// ScaleDeployment updates the replica count for a given deployment.
func (d *DeploymentAPI) ScaleDeployment(ctx context.Context, name string, replicas int) (*Deployment, error) {
	path := fmt.Sprintf("/workspaces/%s/deployments/%s/scale", d.workspace, name)

	body := map[string]int{"replicas": replicas}
	var deployment Deployment
	if err := d.client.do(ctx, http.MethodPost, path, body, &deployment); err != nil {
		return nil, fmt.Errorf("scaling deployment %q to %d replicas: %w", name, replicas, err)
	}
	return &deployment, nil
}
