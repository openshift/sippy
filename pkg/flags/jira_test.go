package flags

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

// jiraTestTransport redirects the fixed OAuth and gateway endpoints to a local server.
type jiraTestTransport func(*http.Request) (*http.Response, error)

func (f jiraTestTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestGetJiraClientServiceAccount(t *testing.T) {
	for _, tc := range []struct {
		name     string
		env      map[string]string
		clientID string
		secret   string
	}{
		{
			name: "generic credentials",
			env: map[string]string{
				"JIRA_CLIENT_ID": "generic-client", "JIRA_CLIENT_SECRET": "generic-secret",
			},
			clientID: "generic-client", secret: "generic-secret",
		},
		{
			name: "generic credentials ignore legacy variables",
			env: map[string]string{
				"JIRA_CLIENT_ID": "generic-client", "JIRA_CLIENT_SECRET": "generic-secret",
				"JIRA_READ_ONLY_CLIENT_ID": "legacy-client", "JIRA_READ_ONLY_CLIENT_SECRET": "legacy-secret",
			},
			clientID: "generic-client", secret: "generic-secret",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, variable := range []string{"JIRA_CLIENT_ID", "JIRA_CLIENT_SECRET", "JIRA_READ_ONLY_CLIENT_ID", "JIRA_READ_ONLY_CLIENT_SECRET"} {
				t.Setenv(variable, "")
				require.NoError(t, os.Unsetenv(variable))
			}
			for name, value := range tc.env {
				t.Setenv(name, value)
			}
			const cloudID = "11111111-2222-3333-4444-555555555555"
			t.Setenv("JIRA_CLOUD_ID", cloudID)
			t.Setenv("JIRA_TOKEN", "unused-pat")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/oauth/token":
					assert.NoError(t, r.ParseForm())
					assert.Equal(t, tc.clientID, r.Form.Get("client_id"))
					assert.Equal(t, tc.secret, r.Form.Get("client_secret"))
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
			fs := pflag.NewFlagSet("jira", pflag.ContinueOnError)
			flags.BindFlags(fs)
			require.NoError(t, fs.Parse([]string{"--jira-service-account"}))
			flags.JiraURL = "https://ignored.invalid/"
			flags.JiraTokenFile = "/nonexistent/pat-file"
			client, err := flags.GetJiraClient(ctx)
			require.NoError(t, err)
			user, _, err := client.User.GetSelf()
			require.NoError(t, err)
			assert.Equal(t, "service-account", user.Name)
		})
	}
}

func TestGetJiraClientServiceAccountMissingEnvironment(t *testing.T) {
	for _, variable := range []string{"JIRA_CLIENT_ID", "JIRA_CLIENT_SECRET", "JIRA_CLOUD_ID"} {
		t.Run(variable, func(t *testing.T) {
			t.Setenv("JIRA_CLIENT_ID", "client")
			t.Setenv("JIRA_CLIENT_SECRET", "secret")
			t.Setenv("JIRA_CLOUD_ID", "11111111-2222-3333-4444-555555555555")
			t.Setenv(variable, "")
			t.Setenv("JIRA_TOKEN", "unused-pat")
			flags := NewJiraFlags()
			flags.ServiceAccount = true
			client, err := flags.GetJiraClient(context.Background())
			require.ErrorContains(t, err, "configure Jira service-account client")
			assert.Nil(t, client)
		})
	}
}

func TestGetJiraClientServiceAccountRequiresGenericCredentials(t *testing.T) {
	for _, tc := range []struct {
		name      string
		env       map[string]string
		wantError string
	}{
		{"legacy pair without generic credentials", nil, "client ID is required"},
		{"generic ID without secret", map[string]string{"JIRA_CLIENT_ID": "generic-client"}, "client secret is required"},
		{"generic secret without ID", map[string]string{"JIRA_CLIENT_SECRET": "generic-secret"}, "client ID is required"},
		{"empty generic pair", map[string]string{"JIRA_CLIENT_ID": "", "JIRA_CLIENT_SECRET": ""}, "client ID is required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, variable := range []string{"JIRA_CLIENT_ID", "JIRA_CLIENT_SECRET"} {
				t.Setenv(variable, "")
				require.NoError(t, os.Unsetenv(variable))
			}
			for name, value := range tc.env {
				t.Setenv(name, value)
			}
			t.Setenv("JIRA_READ_ONLY_CLIENT_ID", "legacy-client")
			t.Setenv("JIRA_READ_ONLY_CLIENT_SECRET", "legacy-secret")
			t.Setenv("JIRA_CLOUD_ID", "11111111-2222-3333-4444-555555555555")
			flags := NewJiraFlags()
			flags.ServiceAccount = true
			ctx := context.WithValue(context.Background(), oauth2.HTTPClient, &http.Client{
				Transport: jiraTestTransport(func(r *http.Request) (*http.Response, error) {
					return nil, fmt.Errorf("unexpected OAuth request with incomplete credentials")
				}),
			})
			client, err := flags.GetJiraClient(ctx)
			require.ErrorContains(t, err, tc.wantError)
			assert.Nil(t, client)
		})
	}
}

func TestGetJiraClientKeepsPATAuthentication(t *testing.T) {
	for _, tc := range []struct {
		name                string
		bearer, basic, want string
		file                bool
		fileContents        string
		missingFile         bool
	}{
		{name: "token file takes precedence", bearer: "env-bearer", basic: "env-basic", file: true, fileContents: "file-token", want: "Bearer file-token"},
		{name: "empty file falls back to environment", bearer: "env-bearer", basic: "env-basic", file: true, want: "Bearer env-bearer"},
		{name: "bearer takes precedence over basic", bearer: "env-bearer", basic: "env-basic", want: "Bearer env-bearer"},
		{name: "basic", basic: "env-basic", want: "Basic env-basic"},
		{name: "no token still returns nil client"},
		{name: "missing token file still fails", bearer: "env-bearer", missingFile: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("JIRA_CLIENT_ID", "client")
			t.Setenv("JIRA_CLIENT_SECRET", "secret")
			t.Setenv("JIRA_CLOUD_ID", "11111111-2222-3333-4444-555555555555")
			t.Setenv("JIRA_TOKEN", tc.bearer)
			t.Setenv("JIRA_TOKEN_BASIC", tc.basic)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/rest/api/2/myself", r.URL.Path)
				assert.Equal(t, tc.want, r.Header.Get("Authorization"))
				w.Header().Set("Content-Type", "application/json")
				_, err := w.Write([]byte(`{"name":"existing-account"}`))
				assert.NoError(t, err)
			}))
			defer server.Close()
			flags := NewJiraFlags()
			flags.JiraURL = server.URL
			if tc.file || tc.missingFile {
				flags.JiraTokenFile = filepath.Join(t.TempDir(), "jira-token")
				if tc.file {
					require.NoError(t, os.WriteFile(flags.JiraTokenFile, []byte(tc.fileContents), 0o600))
				}
			}
			fs := pflag.NewFlagSet("jira", pflag.ContinueOnError)
			flags.BindFlags(fs)
			// PAT selection must ignore SA credentials and their token context.
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			client, err := flags.GetJiraClient(ctx)
			if tc.missingFile {
				require.ErrorIs(t, err, os.ErrNotExist)
				assert.Nil(t, client)
				return
			}
			require.NoError(t, err)
			if tc.want == "" {
				assert.Nil(t, client)
				return
			}
			require.NotNil(t, client)
			user, _, err := client.User.GetSelf()
			require.NoError(t, err)
			assert.Equal(t, "existing-account", user.Name)
		})
	}
}
