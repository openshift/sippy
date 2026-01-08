package jobrunscan

import (
	"context"
	"fmt"

	api "github.com/openshift/sippy/pkg/api/jobrunscan"
	"github.com/openshift/sippy/pkg/db/models/jobrunscan"
	"github.com/openshift/sippy/pkg/sippyclient"
)

// SymptomsClient provides methods for interacting with the symptoms API
type SymptomsClient struct {
	client *sippyclient.Client
}

// NewSymptomsClient creates a new symptoms client
func NewSymptomsClient(client *sippyclient.Client) *SymptomsClient {
	return &SymptomsClient{
		client: client,
	}
}

// List retrieves all symptoms from the API
func (sc *SymptomsClient) List(ctx context.Context) ([]jobrunscan.Symptom, error) {
	var symptoms []jobrunscan.Symptom
	if err := sc.client.Get(ctx, "/api/jobs/symptoms", &symptoms); err != nil {
		return nil, fmt.Errorf("failed to list symptoms: %w", err)
	}
	return symptoms, nil
}

// Get retrieves a single symptom by ID
func (sc *SymptomsClient) Get(ctx context.Context, id string) (*jobrunscan.Symptom, error) {
	if !api.ValidIdentifierRegex.MatchString(id) {
		return nil, fmt.Errorf("invalid symptom ID: '%s'", id)
	}

	var symptom jobrunscan.Symptom
	path := fmt.Sprintf("/api/jobs/symptoms/%s", id)
	if err := sc.client.Get(ctx, path, &symptom); err != nil {
		return nil, fmt.Errorf("failed to get symptom %s: %w", id, err)
	}
	return &symptom, nil
}

// Create creates a new symptom
func (sc *SymptomsClient) Create(ctx context.Context, symptom jobrunscan.Symptom) (*jobrunscan.Symptom, error) {
	if !api.ValidIdentifierRegex.MatchString(symptom.ID) {
		return nil, fmt.Errorf("invalid symptom ID: '%s'", symptom.ID)
	}

	var result jobrunscan.Symptom
	if err := sc.client.Post(ctx, "/api/jobs/symptoms", symptom, &result); err != nil {
		return nil, fmt.Errorf("failed to create symptom: %w", err)
	}
	return &result, nil
}

// Update updates an existing symptom
func (sc *SymptomsClient) Update(ctx context.Context, id string, symptom jobrunscan.Symptom) (*jobrunscan.Symptom, error) {
	if !api.ValidIdentifierRegex.MatchString(id) {
		return nil, fmt.Errorf("invalid lookup symptom ID: '%s'", id)
	}
	if !api.ValidIdentifierRegex.MatchString(symptom.ID) {
		return nil, fmt.Errorf("invalid update symptom ID: '%s'", symptom.ID)
	}

	var result jobrunscan.Symptom
	path := fmt.Sprintf("/api/jobs/symptoms/%s", id)
	if err := sc.client.Put(ctx, path, symptom, &result); err != nil {
		return nil, fmt.Errorf("failed to update symptom %s: %w", id, err)
	}
	return &result, nil
}

// Delete deletes a symptom by ID
func (sc *SymptomsClient) Delete(ctx context.Context, id string) error {
	if !api.ValidIdentifierRegex.MatchString(id) {
		return fmt.Errorf("invalid symptom ID: '%s'", id)
	}

	path := fmt.Sprintf("/api/jobs/symptoms/%s", id)
	if err := sc.client.Delete(ctx, path); err != nil {
		return fmt.Errorf("failed to delete symptom %s: %w", id, err)
	}
	return nil
}
