package adc

import (
	"context"
	"fmt"
	"net/http"
)

// ServiceInstance represents a single service resource with operations.
type ServiceInstance struct {
	client    *Client
	namespace string
	name      string
}

// NewServiceInstance creates a new ServiceInstance for the given namespace and name.
func NewServiceInstance(client *Client, namespace, name string) *ServiceInstance {
	return &ServiceInstance{
		client:    client,
		namespace: namespace,
		name:      name,
	}
}

// Service represents the API response structure for a service resource.
type Service struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Spec        ServiceSpec       `json:"spec"`
	Status      ServiceStatus     `json:"status"`
}

// ServiceSpec defines the desired state of a service.
type ServiceSpec struct {
	Image    string            `json:"image"`
	Replicas int               `json:"replicas"`
	Ports    []ServicePort     `json:"ports,omitempty"`
	Env      map[string]string `json:"env,omitempty"`
	Resources ResourceRequirements `json:"resources,omitempty"`
}

// ServicePort describes a port exposed by a service.
type ServicePort struct {
	Name       string `json:"name,omitempty"`
	Port       int    `json:"port"`
	TargetPort int    `json:"targetPort,omitempty"`
	Protocol   string `json:"protocol,omitempty"`
}

// ResourceRequirements describes compute resource requirements.
type ResourceRequirements struct {
	CPU    string `json:"cpu,omitempty"`
	Memory string `json:"memory,omitempty"`
}

// ServiceStatus reflects the observed state of a service.
type ServiceStatus struct {
	State     string `json:"state"`
	Replicas  int    `json:"replicas"`
	ReadyReplicas int `json:"readyReplicas"`
	Endpoint  string `json:"endpoint,omitempty"`
	Message   string `json:"message,omitempty"`
}

// Get retrieves the current state of the service.
func (s *ServiceInstance) Get(ctx context.Context) (*Service, error) {
	path := fmt.Sprintf("/namespaces/%s/services/%s", s.namespace, s.name)

	resp, err := s.client.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("getting service %s/%s: %w", s.namespace, s.name, err)
	}
	defer resp.Body.Close()

	var svc Service
	if err := decodeResponse(resp, &svc); err != nil {
		return nil, fmt.Errorf("decoding service response: %w", err)
	}
	return &svc, nil
}

// Update applies the provided service spec as an update to the existing service.
func (s *ServiceInstance) Update(ctx context.Context, spec ServiceSpec) (*Service, error) {
	path := fmt.Sprintf("/namespaces/%s/services/%s", s.namespace, s.name)

	resp, err := s.client.doRequest(ctx, http.MethodPut, path, spec)
	if err != nil {
		return nil, fmt.Errorf("updating service %s/%s: %w", s.namespace, s.name, err)
	}
	defer resp.Body.Close()

	var svc Service
	if err := decodeResponse(resp, &svc); err != nil {
		return nil, fmt.Errorf("decoding updated service response: %w", err)
	}
	return &svc, nil
}

// Delete removes the service from the namespace.
func (s *ServiceInstance) Delete(ctx context.Context) error {
	path := fmt.Sprintf("/namespaces/%s/services/%s", s.namespace, s.name)

	resp, err := s.client.doRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return fmt.Errorf("deleting service %s/%s: %w", s.namespace, s.name, err)
	}
	defer resp.Body.Close()

	if err := expectNoContent(resp); err != nil {
		return fmt.Errorf("unexpected response deleting service: %w", err)
	}
	return nil
}

// Scale updates the replica count for the service.
func (s *ServiceInstance) Scale(ctx context.Context, replicas int) (*Service, error) {
	path := fmt.Sprintf("/namespaces/%s/services/%s/scale", s.namespace, s.name)

	body := map[string]int{"replicas": replicas}
	resp, err := s.client.doRequest(ctx, http.MethodPost, path, body)
	if err != nil {
		return nil, fmt.Errorf("scaling service %s/%s to %d replicas: %w", s.namespace, s.name, replicas, err)
	}
	defer resp.Body.Close()

	var svc Service
	if err := decodeResponse(resp, &svc); err != nil {
		return nil, fmt.Errorf("decoding scale response: %w", err)
	}
	return &svc, nil
}
