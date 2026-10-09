package flags

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

// jiraTestTransport redirects the fixed OAuth and gateway endpoints to a local server.
type jiraTestTransport func(*http.Request) (*http.Response, error)

func (f jiraTestTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestGetReadOnlyServiceAccountClient(t *testing.T) {
	const cloudID = "11111111-2222-3333-4444-555555555555"
	t.Setenv("JIRA_READ_ONLY_CLIENT_ID", "read-only-client")
	t.Setenv("JIRA_READ_ONLY_CLIENT_SECRET", "read-only-secret")
	t.Setenv("JIRA_CLOUD_ID", cloudID)
	t.Setenv("JIRA_TOKEN", "unused-pat")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/oauth/token":
			assert.NoError(t, r.ParseForm())
			assert.Equal(t, "read-only-client", r.Form.Get("client_id"))
			assert.Equal(t, "read-only-secret", r.Form.Get("client_secret"))
			_, err := w.Write([]byte(`{"access_token":"sa-token","token_type":"Bearer","expires_in":3600}`))
			assert.NoError(t, err)
		case "/ex/jira/" + cloudID + "/rest/api/2/myself":
			assert.Equal(t, "Bearer sa-token", r.Header.Get("Authorization"))
			_, err := w.Write([]byte(`{"name":"service-account"}`))
			assert.NoError(t, err)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	target, err := url.Parse(server.URL)
	require.NoError(t, err)
	transport := jiraTestTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "auth.atlassian.com" && r.URL.Host != "api.atlassian.com" {
			return nil, fmt.Errorf("unexpected host %s", r.URL.Host)
		}
		clone := r.Clone(r.Context())
		clone.URL.Scheme, clone.URL.Host = target.Scheme, target.Host
		return server.Client().Transport.RoundTrip(clone)
	})
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, &http.Client{Transport: transport})
	flags := NewJiraFlags()
	flags.JiraURL = "https://ignored.invalid/"
	flags.JiraTokenFile = "/nonexistent/pat-file"
	client, err := flags.GetReadOnlyServiceAccountClient(ctx)
	require.NoError(t, err)
	user, _, err := client.User.GetSelf()
	require.NoError(t, err)
	assert.Equal(t, "service-account", user.Name)
}

func TestGetReadOnlyServiceAccountClientMissingEnvironment(t *testing.T) {
	for _, variable := range []string{"JIRA_READ_ONLY_CLIENT_ID", "JIRA_READ_ONLY_CLIENT_SECRET", "JIRA_CLOUD_ID"} {
		t.Run(variable, func(t *testing.T) {
			t.Setenv("JIRA_READ_ONLY_CLIENT_ID", "client")
			t.Setenv("JIRA_READ_ONLY_CLIENT_SECRET", "secret")
			t.Setenv("JIRA_CLOUD_ID", "11111111-2222-3333-4444-555555555555")
			t.Setenv(variable, "")
			t.Setenv("JIRA_TOKEN", "unused-pat")
			client, err := NewJiraFlags().GetReadOnlyServiceAccountClient(context.Background())
			require.ErrorContains(t, err, "configure read-only Jira service-account client")
			assert.Nil(t, client)
		})
	}
}

func TestGetJiraClientKeepsPATAuthentication(t *testing.T) {
	t.Setenv("JIRA_READ_ONLY_CLIENT_ID", "client")
	t.Setenv("JIRA_READ_ONLY_CLIENT_SECRET", "secret")
	t.Setenv("JIRA_CLOUD_ID", "11111111-2222-3333-4444-555555555555")
	t.Setenv("JIRA_TOKEN", "existing-pat")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/rest/api/2/myself", r.URL.Path)
		assert.Equal(t, "Bearer existing-pat", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"name":"existing-account"}`))
		assert.NoError(t, err)
	}))
	defer server.Close()
	flags := NewJiraFlags()
	flags.JiraURL = server.URL
	client, err := flags.GetJiraClient()
	require.NoError(t, err)
	require.NotNil(t, client)
	user, _, err := client.User.GetSelf()
	require.NoError(t, err)
	assert.Equal(t, "existing-account", user.Name)
}
