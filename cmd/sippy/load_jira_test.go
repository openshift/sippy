package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/openshift/sippy/pkg/api/componentreadiness"
	"github.com/openshift/sippy/pkg/db/models"
)

func TestJiraCommandsServiceAccountIsOptIn(t *testing.T) {
	t.Setenv("JIRA_CLIENT_ID", "client")
	t.Setenv("JIRA_CLIENT_SECRET", "secret")
	t.Setenv("JIRA_CLOUD_ID", "11111111-2222-3333-4444-555555555555")
	for _, tc := range []struct {
		name    string
		command func() *cobra.Command
	}{
		{"automate-jira", NewAutomateJiraCommand},
		{"load", NewLoadCommand},
		{"serve", NewServeCommand},
		{"component-readiness", NewComponentReadinessCommand},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := tc.command()
			flag := cmd.Flags().Lookup("jira-service-account")
			require.NotNil(t, flag)
			assert.Equal(t, "false", flag.DefValue)
			require.NoError(t, cmd.Flags().Parse(nil))
			enabled, err := cmd.Flags().GetBool("jira-service-account")
			require.NoError(t, err)
			assert.False(t, enabled)
		})
	}
}

func TestLoadJiraAuthenticationSelection(t *testing.T) {
	for _, serviceAccount := range []bool{false, true} {
		t.Run(fmt.Sprintf("service account=%t", serviceAccount), func(t *testing.T) {
			t.Setenv("JIRA_TOKEN", "existing-pat")
			t.Setenv("JIRA_CLIENT_ID", "")
			t.Setenv("JIRA_CLIENT_SECRET", "")
			flags := NewLoadFlags()
			flags.JiraFlags.ServiceAccount = serviceAccount
			clients := &loadJiraClients{flags: flags.JiraFlags, ctx: context.Background()}
			loader, err := clients.loader(nil)
			if serviceAccount {
				require.ErrorContains(t, err, "configure Jira data loader")
				require.ErrorContains(t, err, "client ID is required")
				assert.Nil(t, loader)
			} else {
				require.NoError(t, err)
				assert.Equal(t, "jira", loader.Name())
			}
			client, err := clients.jiraClient()
			if serviceAccount {
				require.ErrorContains(t, err, "client ID is required")
				assert.Nil(t, client)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, client)
			}
		})
	}
}

// loadJiraTestTransport routes fixed OAuth endpoints to a local test server.
type loadJiraTestTransport func(*http.Request) (*http.Response, error)

func (f loadJiraTestTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestLoadJiraServiceAccountForRegressionTracking(t *testing.T) {
	for _, loaderFirst := range []bool{true, false} {
		t.Run(fmt.Sprintf("loader first=%t", loaderFirst), func(t *testing.T) {
			const cloudID = "11111111-2222-3333-4444-555555555555"
			t.Setenv("JIRA_CLIENT_ID", "loader-client")
			t.Setenv("JIRA_CLIENT_SECRET", "loader-secret")
			t.Setenv("JIRA_CLOUD_ID", cloudID)
			t.Setenv("JIRA_TOKEN", "existing-pat")
			grants, comments := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/oauth/token":
					grants++
					assert.NoError(t, r.ParseForm())
					assert.Equal(t, "loader-client", r.Form.Get("client_id"))
					assert.Equal(t, "loader-secret", r.Form.Get("client_secret"))
					_, err := w.Write([]byte(`{"access_token":"loader-token","token_type":"Bearer","expires_in":3600}`))
					assert.NoError(t, err)
				case "/ex/jira/" + cloudID + "/rest/api/2/myself":
					assert.Equal(t, "Bearer loader-token", r.Header.Get("Authorization"))
					_, err := w.Write([]byte(`{"name":"service-account"}`))
					assert.NoError(t, err)
				case "/ex/jira/" + cloudID + "/rest/api/2/issue/OCPBUGS-42/comment":
					comments++
					assert.Equal(t, http.MethodPost, r.Method)
					assert.Equal(t, "Bearer loader-token", r.Header.Get("Authorization"))
					var comment struct {
						Body string `json:"body"`
					}
					assert.NoError(t, json.NewDecoder(r.Body).Decode(&comment))
					assert.Contains(t, comment.Body, "All regressions associated with this triage record have been resolved.")
					w.WriteHeader(http.StatusCreated)
					_, err := w.Write([]byte(`{"id":"1"}`))
					assert.NoError(t, err)
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			target, err := url.Parse(server.URL)
			require.NoError(t, err)
			transport := loadJiraTestTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host != "auth.atlassian.com" && r.URL.Host != "api.atlassian.com" {
					return nil, fmt.Errorf("unexpected host %s", r.URL.Host)
				}
				clone := r.Clone(r.Context())
				clone.URL.Scheme, clone.URL.Host = target.Scheme, target.Host
				return server.Client().Transport.RoundTrip(clone)
			})
			ctx := context.WithValue(context.Background(), oauth2.HTTPClient, &http.Client{Transport: transport})
			flags := NewLoadFlags()
			flags.JiraFlags.ServiceAccount = true
			flags.JiraFlags.JiraURL = server.URL
			clients := &loadJiraClients{flags: flags.JiraFlags, ctx: ctx}
			if !loaderFirst {
				_, err := clients.jiraClient()
				require.NoError(t, err)
			}
			loader, err := clients.loader(nil)
			require.NoError(t, err)
			assert.Equal(t, "jira", loader.Name())
			client, err := clients.jiraClient()
			require.NoError(t, err)
			require.NotNil(t, client)
			user, _, err := client.User.GetSelf()
			require.NoError(t, err)
			assert.Equal(t, "service-account", user.Name)
			componentreadiness.ReportTriageResolved(client, models.Triage{URL: "https://redhat.atlassian.net/browse/OCPBUGS-42"})
			assert.Equal(t, 1, comments)
			sharedClient, baseURL, err := clients.serviceAccountHTTPClient()
			require.NoError(t, err)
			assert.Equal(t, "https://api.atlassian.com/ex/jira/"+cloudID+"/", baseURL)
			response, err := sharedClient.Get(baseURL + "rest/api/2/myself") //nolint:gosec // G704: fixed OAuth gateway URL redirected to the local test server.
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			assert.Equal(t, http.StatusOK, response.StatusCode)
			assert.Equal(t, 1, grants)
		})
	}
}
