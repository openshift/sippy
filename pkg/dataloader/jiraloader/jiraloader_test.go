package jiraloader

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

	"github.com/openshift/sippy/pkg/jiraauth"
)

func TestJiraLoaderReads(t *testing.T) {
	for _, prefix := range []string{"", "/ex/jira/cloud-id"} {
		t.Run("base path="+prefix, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, "application/json", r.Header.Get("Accept"))
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case prefix + "/rest/api/2/project/OCPBUGS/components":
					_, err := w.Write([]byte(`[{"id":"42","name":"Networking","description":"network component","lead":{"displayName":"Lead"}}]`))
					assert.NoError(t, err)
				case prefix + "/rest/api/3/search/jql":
					q := r.URL.Query()
					assert.Equal(t, `labels = "trt-incident" AND updated >= -60d`, q.Get("jql"))
					assert.Equal(t, "summary,created,labels,resolutiondate", q.Get("fields"))
					assert.Equal(t, "changelog", q.Get("expand"))
					if q.Get("nextPageToken") == "" {
						_, err := w.Write([]byte(`{"issues":[{"id":"42","key":"TRT-42","fields":{"summary":"Incident","labels":["trt-incident"]}}],"nextPageToken":"a+b/c=&","isLast":false}`))
						assert.NoError(t, err)
					} else {
						assert.Equal(t, "a+b/c=&", q.Get("nextPageToken"))
						_, err := w.Write([]byte(`{"issues":[],"isLast":true}`))
						assert.NoError(t, err)
					}
				case prefix + "/rest/api/2/issue/TRT-42":
					assert.Equal(t, "changelog", r.URL.Query().Get("expand"))
					_, err := w.Write([]byte(`{"id":"42","key":"TRT-42","fields":{"summary":"Incident"},"changelog":{"histories":[{"items":[{"field":"labels","fromString":"trt-incident","toString":""}]}]}}`))
					assert.NoError(t, err)
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			loader := NewWithClient(context.Background(), nil, server.Client(), server.URL+prefix)
			components, err := loader.queryComponents()
			require.NoError(t, err)
			require.Len(t, components, 1)
			assert.Equal(t, "Networking", components[0].Name)
			assert.Equal(t, "Lead", components[0].Lead.DisplayName)
			page, err := loader.queryIncidentPage("")
			require.NoError(t, err)
			require.Len(t, page.Issues, 1)
			assert.Equal(t, "TRT-42", page.Issues[0].Key)
			assert.False(t, page.IsLast)
			last, err := loader.queryIncidentPage(page.NextPageToken)
			require.NoError(t, err)
			assert.True(t, last.IsLast)
			assert.Empty(t, last.Issues)
			issue, err := loader.queryJiraAPI("TRT-42")
			require.NoError(t, err)
			require.Len(t, issue.Changelog.Histories, 1)
			assert.Equal(t, "labels", issue.Changelog.Histories[0].Items[0].Field)
			assert.Equal(t, 4, requests)
		})
	}
}

// loaderTestTransport redirects the OAuth library's fixed endpoints locally.
type loaderTestTransport func(*http.Request) (*http.Response, error)

func (f loaderTestTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestJiraLoaderServiceAccountAuthentication(t *testing.T) {
	const cloudID = "11111111-2222-3333-4444-555555555555"
	t.Setenv("JIRA_TOKEN", "unused-pat")
	t.Setenv("JIRA_TOKEN_BASIC", "unused-basic")
	grants, reads := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/oauth/token" {
			grants++
			assert.Equal(t, http.MethodPost, r.Method)
			assert.NoError(t, r.ParseForm())
			assert.Equal(t, "client_credentials", r.Form.Get("grant_type"))
			assert.Equal(t, "loader-client", r.Form.Get("client_id"))
			assert.Equal(t, "loader-secret", r.Form.Get("client_secret"))
			_, err := w.Write([]byte(`{"access_token":"loader-token","token_type":"Bearer","expires_in":3600}`))
			assert.NoError(t, err)
			return
		}
		reads++
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "Bearer loader-token", r.Header.Get("Authorization"))
		switch r.URL.Path {
		case "/ex/jira/" + cloudID + "/rest/api/2/project/OCPBUGS/components":
			_, err := w.Write([]byte(`[]`))
			assert.NoError(t, err)
		case "/ex/jira/" + cloudID + "/rest/api/3/search/jql":
			_, err := w.Write([]byte(`{"issues":[],"isLast":true}`))
			assert.NoError(t, err)
		case "/ex/jira/" + cloudID + "/rest/api/2/issue/TRT-42":
			_, err := w.Write([]byte(`{"id":"42","key":"TRT-42"}`))
			assert.NoError(t, err)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	target, err := url.Parse(server.URL)
	require.NoError(t, err)
	transport := loaderTestTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "auth.atlassian.com" && r.URL.Host != "api.atlassian.com" {
			return nil, fmt.Errorf("unexpected host %s", r.URL.Host)
		}
		clone := r.Clone(r.Context())
		clone.URL.Scheme, clone.URL.Host = target.Scheme, target.Host
		return server.Client().Transport.RoundTrip(clone)
	})
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, &http.Client{Transport: transport})
	client, baseURL, err := (jiraauth.Config{ClientID: "loader-client", ClientSecret: "loader-secret", CloudID: cloudID}).NewClient(ctx)
	require.NoError(t, err)
	loader := NewWithClient(ctx, nil, client, baseURL)
	_, err = loader.queryComponents()
	require.NoError(t, err)
	_, err = loader.queryIncidentPage("")
	require.NoError(t, err)
	_, err = loader.queryJiraAPI("TRT-42")
	require.NoError(t, err)
	assert.Equal(t, 1, grants)
	assert.Equal(t, 3, reads)
}

func TestJiraLoaderPATAuthentication(t *testing.T) {
	for _, tc := range []struct {
		name, bearer, basic, want string
	}{
		{"bearer takes precedence", "bearer-token", "basic-token", "Bearer bearer-token"},
		{"basic", "", "basic-token", "Basic basic-token"},
		{"unauthenticated", "", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("JIRA_TOKEN", tc.bearer)
			t.Setenv("JIRA_TOKEN_BASIC", tc.basic)
			t.Setenv("JIRA_CLIENT_ID", "unused-client")
			t.Setenv("JIRA_CLIENT_SECRET", "unused-secret")
			t.Setenv("JIRA_CLOUD_ID", "invalid-but-unused")
			t.Setenv("JIRA_READ_ONLY_CLIENT_ID", "unused-legacy-client")
			t.Setenv("JIRA_READ_ONLY_CLIENT_SECRET", "unused-legacy-secret")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, tc.want, r.Header.Get("Authorization"))
				_, err := w.Write([]byte(`[]`))
				assert.NoError(t, err)
			}))
			defer server.Close()
			loader := New(nil)
			assert.Equal(t, "https://redhat.atlassian.net/", loader.baseURL)
			loader.baseURL = server.URL + "/"
			loader.httpClient = server.Client()
			_, err := loader.queryComponents()
			require.NoError(t, err)
		})
	}
}

func TestJiraLoaderReadErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"unauthorized", http.StatusUnauthorized, `{"message":"denied","private":"confidential incident"}`, "401 Unauthorized"},
		{"forbidden", http.StatusForbidden, `{"message":"private issue details","email":"person@example.com"}`, "403 Forbidden"},
		{"malformed JSON", http.StatusOK, `{`, "decode Jira"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, err := w.Write([]byte(tc.body))
				assert.NoError(t, err)
			}))
			defer server.Close()
			loader := NewWithClient(context.Background(), nil, server.Client(), server.URL)
			_, componentsErr := loader.queryComponents()
			_, pageErr := loader.queryIncidentPage("")
			_, issueErr := loader.queryJiraAPI("TRT-42")
			for _, err := range []error{componentsErr, pageErr, issueErr} {
				require.ErrorContains(t, err, tc.want)
				if tc.status != http.StatusOK {
					assert.EqualError(t, err, "jira request failed: "+tc.want)
				}
			}
		})
	}
}

func TestJiraLoaderNilHTTPClient(t *testing.T) {
	loader := NewWithClient(context.Background(), nil, nil, "https://redhat.atlassian.net/")
	_, componentsErr := loader.queryComponents()
	_, pageErr := loader.queryIncidentPage("")
	_, issueErr := loader.queryJiraAPI("TRT-42")
	for _, err := range []error{componentsErr, pageErr, issueErr} {
		require.EqualError(t, err, "jira request failed: HTTP client is nil")
	}
}

func TestJiraLoaderCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("canceled loader made an HTTP request")
	}))
	defer server.Close()
	loader := NewWithClient(ctx, nil, server.Client(), server.URL)
	_, err := loader.queryComponents()
	require.ErrorIs(t, err, context.Canceled)
}
