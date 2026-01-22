package jobrunscan

import (
	"context"
	"fmt"

	api "github.com/openshift/sippy/pkg/api/jobrunscan"
	"github.com/openshift/sippy/pkg/db/models/jobrunscan"
	"github.com/openshift/sippy/pkg/sippyclient"
)

// LabelsClient provides methods for interacting with the labels API
type LabelsClient struct {
	client *sippyclient.Client
}

// NewLabelsClient creates a new labels client
func NewLabelsClient(client *sippyclient.Client) *LabelsClient {
	return &LabelsClient{
		client: client,
	}
}

// List retrieves all labels from the API
func (sc *LabelsClient) List(ctx context.Context) ([]jobrunscan.Label, error) {
	var labels []jobrunscan.Label
	if err := sc.client.Get(ctx, "/api/jobs/labels", &labels); err != nil {
		return nil, fmt.Errorf("failed to list labels: %w", err)
	}
	return labels, nil
}

// Get retrieves a single label by ID
func (sc *LabelsClient) Get(ctx context.Context, id string) (*jobrunscan.Label, error) {
	if !api.ValidIdentifierRegex.MatchString(id) {
		return nil, fmt.Errorf("invalid label ID: '%s'", id)
	}

	var label jobrunscan.Label
	path := fmt.Sprintf("/api/jobs/labels/%s", id)
	if err := sc.client.Get(ctx, path, &label); err != nil {
		return nil, fmt.Errorf("failed to get label %s: %w", id, err)
	}
	return &label, nil
}

// Create creates a new label
func (sc *LabelsClient) Create(ctx context.Context, label jobrunscan.Label) (*jobrunscan.Label, error) {
	if !api.ValidIdentifierRegex.MatchString(label.ID) {
		return nil, fmt.Errorf("invalid label ID: '%s'", label.ID)
	}

	var result jobrunscan.Label
	if err := sc.client.Post(ctx, "/api/jobs/labels", label, &result); err != nil {
		return nil, fmt.Errorf("failed to create label: %w", err)
	}
	return &result, nil
}

// Update updates an existing label
func (sc *LabelsClient) Update(ctx context.Context, id string, label jobrunscan.Label) (*jobrunscan.Label, error) {
	if !api.ValidIdentifierRegex.MatchString(id) {
		return nil, fmt.Errorf("invalid lookup label ID: '%s'", id)
	}
	if !api.ValidIdentifierRegex.MatchString(label.ID) {
		return nil, fmt.Errorf("invalid update label ID: '%s'", label.ID)
	}

	var result jobrunscan.Label
	path := fmt.Sprintf("/api/jobs/labels/%s", id)
	if err := sc.client.Put(ctx, path, label, &result); err != nil {
		return nil, fmt.Errorf("failed to update label %s: %w", id, err)
	}
	return &result, nil
}

// Delete deletes a label by ID
func (sc *LabelsClient) Delete(ctx context.Context, id string) error {
	if !api.ValidIdentifierRegex.MatchString(id) {
		return fmt.Errorf("invalid label ID: '%s'", id)
	}

	path := fmt.Sprintf("/api/jobs/labels/%s", id)
	if err := sc.client.Delete(ctx, path); err != nil {
		return fmt.Errorf("failed to delete label %s: %w", id, err)
	}
	return nil
}
