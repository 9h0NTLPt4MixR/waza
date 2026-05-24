package adc

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// DeploymentStatus represents the current state of a deployment.
type DeploymentStatus string

const (
	DeploymentStatusPending   DeploymentStatus = "pending"
	DeploymentStatusRunning   DeploymentStatus = "running"
	DeploymentStatusSucceeded DeploymentStatus = "succeeded"
	DeploymentStatusFailed    DeploymentStatus = "failed"
	DeploymentStatusCancelled DeploymentStatus = "cancelled"
)

// DeploymentInstance represents a single deployment resource with its metadata and status.
type DeploymentInstance struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	AppID       string            `json:"appId"`
	WorkspaceID string            `json:"workspaceId"`
	Status      DeploymentStatus  `json:"status"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	CreatedAt   time.Time         `json:"createdAt"`
	UpdatedAt   time.Time         `json:"updatedAt"`
	Replicas    int               `json:"replicas"`
	Image       string            `json:"image"`
	EnvVars     map[string]string `json:"envVars,omitempty"`

	client *Client
}

// NewDeploymentInstance creates a new DeploymentInstance bound to the given client.
func NewDeploymentInstance(client *Client) *DeploymentInstance {
	return &DeploymentInstance{
		client: client,
	}
}

// Refresh fetches the latest state of the deployment from the API and updates
// the instance fields in place.
func (d *DeploymentInstance) Refresh(ctx context.Context) error {
	if d.ID == "" {
		return fmt.Errorf("deployment instance has no ID set")
	}

	path := fmt.Sprintf("/deployments/%s", d.ID)
	resp, err := d.client.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return fmt.Errorf("refreshing deployment %s: %w", d.ID, err)
	}
	defer resp.Body.Close()

	if err := decodeJSON(resp.Body, d); err != nil {
		return fmt.Errorf("decoding deployment response: %w", err)
	}
	return nil
}

// Scale updates the replica count for the deployment.
func (d *DeploymentInstance) Scale(ctx context.Context, replicas int) error {
	if d.ID == "" {
		return fmt.Errorf("deployment instance has no ID set")
	}
	if replicas < 0 {
		return fmt.Errorf("replica count must be non-negative, got %d", replicas)
	}

	payload := map[string]int{"replicas": replicas}
	path := fmt.Sprintf("/deployments/%s/scale", d.ID)

	resp, err := d.client.doRequest(ctx, http.MethodPatch, path, payload)
	if err != nil {
		return fmt.Errorf("scaling deployment %s: %w", d.ID, err)
	}
	defer resp.Body.Close()

	if err := decodeJSON(resp.Body, d); err != nil {
		return fmt.Errorf("decoding scale response: %w", err)
	}
	return nil
}

// Delete removes the deployment from the API.
func (d *DeploymentInstance) Delete(ctx context.Context) error {
	if d.ID == "" {
		return fmt.Errorf("deployment instance has no ID set")
	}

	path := fmt.Sprintf("/deployments/%s", d.ID)
	resp, err := d.client.doRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return fmt.Errorf("deleting deployment %s: %w", d.ID, err)
	}
	defer resp.Body.Close()
	return nil
}

// IsTerminal returns true when the deployment has reached a final, non-retryable state.
func (d *DeploymentInstance) IsTerminal() bool {
	switch d.Status {
	case DeploymentStatusSucceeded, DeploymentStatusFailed, DeploymentStatusCancelled:
		return true
	default:
		return false
	}
}

// WaitUntilReady polls the deployment status until it reaches a terminal state
// or the provided context is cancelled. The poll interval defaults to 5 seconds.
func (d *DeploymentInstance) WaitUntilReady(ctx context.Context, pollInterval time.Duration) error {
	if pollInterval <= 0 {
		pollInterval = 5 * time.Second
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := d.Refresh(ctx); err != nil {
				return err
			}
			if d.IsTerminal() {
				if d.Status == DeploymentStatusFailed {
					return fmt.Errorf("deployment %s reached failed state", d.ID)
				}
				return nil
			}
		}
	}
}
