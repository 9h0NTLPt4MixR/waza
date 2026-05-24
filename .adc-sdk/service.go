package adc

import (
	"context"
	"fmt"
	"net/http"
)

// ServiceAPI provides methods for managing services within the ADC platform.
type ServiceAPI struct {
	client *Client
}

// NewServiceAPI creates a new ServiceAPI instance backed by the provided client.
func NewServiceAPI(client *Client) *ServiceAPI {
	return &ServiceAPI{client: client}
}

// Service represents a deployed service resource in ADC.
type Service struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Spec        ServiceSpec       `json:"spec"`
	Status      ServiceStatus     `json:"status"`
}

// ServiceSpec defines the desired configuration for a service.
type ServiceSpec struct {
	Port       int    `json:"port"`
	TargetPort int    `json:"targetPort"`
	Protocol   string `json:"protocol"`
	Selector   map[string]string `json:"selector,omitempty"`
}

// ServiceStatus reflects the observed state of a service.
type ServiceStatus struct {
	State   string `json:"state"`
	Message string `json:"message,omitempty"`
	IP      string `json:"clusterIP,omitempty"`
}

// ListServicesOptions holds optional filters for listing services.
type ListServicesOptions struct {
	Namespace string
	Labels    map[string]string
}

// List retrieves all services, optionally filtered by namespace and labels.
func (s *ServiceAPI) List(ctx context.Context, opts *ListServicesOptions) ([]Service, error) {
	path := "/services"
	if opts != nil && opts.Namespace != "" {
		path = fmt.Sprintf("/namespaces/%s/services", opts.Namespace)
	}

	if opts != nil && len(opts.Labels) > 0 {
		path += "?" + labelsToQueryString(opts.Labels)
	}

	req, err := s.client.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("service list: build request: %w", err)
	}

	var services []Service
	if err := s.client.do(req, &services); err != nil {
		return nil, fmt.Errorf("service list: %w", err)
	}

	return services, nil
}

// Get retrieves a single service by namespace and name.
func (s *ServiceAPI) Get(ctx context.Context, namespace, name string) (*Service, error) {
	path := fmt.Sprintf("/namespaces/%s/services/%s", namespace, name)

	req, err := s.client.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("service get: build request: %w", err)
	}

	var svc Service
	if err := s.client.do(req, &svc); err != nil {
		return nil, fmt.Errorf("service get %s/%s: %w", namespace, name, err)
	}

	return &svc, nil
}

// Create registers a new service resource.
func (s *ServiceAPI) Create(ctx context.Context, svc *Service) (*Service, error) {
	path := fmt.Sprintf("/namespaces/%s/services", svc.Namespace)

	req, err := s.client.newRequest(ctx, http.MethodPost, path, svc)
	if err != nil {
		return nil, fmt.Errorf("service create: build request: %w", err)
	}

	var created Service
	if err := s.client.do(req, &created); err != nil {
		return nil, fmt.Errorf("service create %s: %w", svc.Name, err)
	}

	return &created, nil
}

// Delete removes a service by namespace and name.
func (s *ServiceAPI) Delete(ctx context.Context, namespace, name string) error {
	path := fmt.Sprintf("/namespaces/%s/services/%s", namespace, name)

	req, err := s.client.newRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return fmt.Errorf("service delete: build request: %w", err)
	}

	if err := s.client.do(req, nil); err != nil {
		return fmt.Errorf("service delete %s/%s: %w", namespace, name, err)
	}

	return nil
}
