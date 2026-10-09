// Package jiraauth provides service-account authentication for Jira SDK and HTTP callers.
package jiraauth

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

// Config identifies a service-account credential and the Jira cloud it accesses.
// The credential's scopes are assigned in Atlassian Administration.
type Config struct {
	ClientID     string
	ClientSecret string `json:"-"`
	CloudID      string
}

// NewClient obtains an initial access token and returns an authenticated client
// and Jira gateway URL. ctx must remain valid for the client's lifetime because
// the OAuth library uses it to obtain replacement tokens when they expire.
func (c Config) NewClient(ctx context.Context) (*http.Client, string, error) {
	if c.ClientID == "" {
		return nil, "", fmt.Errorf("jira service-account client ID is required")
	}
	if c.ClientSecret == "" {
		return nil, "", fmt.Errorf("jira service-account client secret is required")
	}
	cloudID, err := uuid.Parse(c.CloudID)
	if err != nil {
		return nil, "", fmt.Errorf("jira service-account cloud ID must be a UUID: %w", err)
	}

	oauthConfig := clientcredentials.Config{ // #nosec G101 -- Credentials come from the caller; the token endpoint is public.
		ClientID:     c.ClientID,
		ClientSecret: c.ClientSecret,
		TokenURL:     "https://auth.atlassian.com/oauth/token",
		AuthStyle:    oauth2.AuthStyleInParams,
	}
	source := oauthConfig.TokenSource(ctx)
	if _, err := source.Token(); err != nil {
		return nil, "", fmt.Errorf("obtain Jira service-account access token: %w", err)
	}
	return oauth2.NewClient(ctx, source), "https://api.atlassian.com/ex/jira/" + cloudID.String() + "/", nil
}
