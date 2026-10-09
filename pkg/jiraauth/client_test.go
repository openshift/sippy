package jiraauth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

const testCloudID = "11111111-2222-3333-4444-555555555555"

// redirectedTransport keeps the production endpoints while routing tests locally.
type redirectedTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func (r redirectedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host != "auth.atlassian.com" && req.URL.Host != "api.atlassian.com" {
		return nil, fmt.Errorf("unexpected host %s", req.URL.Host)
	}
	clone := req.Clone(req.Context())
	clone.URL.Scheme = r.target.Scheme
	clone.URL.Host = r.target.Host
	return r.base.RoundTrip(clone)
}

func TestNewClientTokenLifecycle(t *testing.T) {
	for _, expired := range []bool{false, true} {
		t.Run(fmt.Sprintf("initial token expired=%t", expired), func(t *testing.T) {
			var grants atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/oauth/token":
					assert.Equal(t, http.MethodPost, r.Method)
					assert.Empty(t, r.Header.Get("Authorization"))
					assert.NoError(t, r.ParseForm())
					assert.Equal(t, "client_credentials", r.Form.Get("grant_type"))
					assert.Equal(t, "test-client", r.Form.Get("client_id"))
					assert.Equal(t, "test-secret", r.Form.Get("client_secret"))
					assert.Empty(t, r.Form.Get("scope"))
					grant := grants.Add(1)
					expiry := 3600
					if expired && grant == 1 {
						// The SDK treats tokens within ten seconds of expiry as stale.
						expiry = 1
					}
					w.Header().Set("Content-Type", "application/json")
					_, err := fmt.Fprintf(w, `{"access_token":"token-%d","token_type":"Bearer","expires_in":%d}`, grant, expiry)
					assert.NoError(t, err)
				case "/ex/jira/" + testCloudID + "/rest/api/3/search/jql":
					want := "Bearer token-1"
					if expired {
						want = "Bearer token-2"
					}
					assert.Equal(t, want, r.Header.Get("Authorization"))
					w.WriteHeader(http.StatusOK)
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			target, err := url.Parse(server.URL)
			require.NoError(t, err)
			transport := redirectedTransport{target: target, base: server.Client().Transport}
			ctx := context.WithValue(context.Background(), oauth2.HTTPClient, &http.Client{Transport: transport})
			client, baseURL, err := (Config{ClientID: "test-client", ClientSecret: "test-secret", CloudID: testCloudID}).NewClient(ctx)
			require.NoError(t, err)
			assert.Equal(t, "https://api.atlassian.com/ex/jira/"+testCloudID+"/", baseURL)
			assert.EqualValues(t, 1, grants.Load(), "constructor fetches initial token")
			for range 2 {
				response, err := client.Get(baseURL + "rest/api/3/search/jql")
				require.NoError(t, err)
				assert.Equal(t, http.StatusOK, response.StatusCode)
				require.NoError(t, response.Body.Close())
			}
			wantGrants := int32(1)
			if expired {
				wantGrants = 2
			}
			assert.Equal(t, wantGrants, grants.Load())
		})
	}
}

func TestNewClientInvalidConfig(t *testing.T) {
	for _, tc := range []struct {
		name   string
		config Config
		want   string
	}{
		{"missing client ID", Config{ClientSecret: "secret", CloudID: testCloudID}, "client ID is required"},
		{"missing secret", Config{ClientID: "client", CloudID: testCloudID}, "client secret is required"},
		{"missing cloud ID", Config{ClientID: "client", ClientSecret: "secret"}, "cloud ID must be a UUID"},
		{"invalid cloud ID", Config{ClientID: "client", ClientSecret: "secret", CloudID: "../other"}, "cloud ID must be a UUID"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, baseURL, err := tc.config.NewClient(context.Background())
			require.ErrorContains(t, err, tc.want)
			assert.Nil(t, client)
			assert.Empty(t, baseURL)
		})
	}
}

func TestNewClientRejectedCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, err := w.Write([]byte(`{"error":"invalid_client"}`))
		assert.NoError(t, err)
	}))
	defer server.Close()
	target, err := url.Parse(server.URL)
	require.NoError(t, err)
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, &http.Client{
		Transport: redirectedTransport{target: target, base: server.Client().Transport},
	})
	client, baseURL, err := (Config{ClientID: "client", ClientSecret: "secret", CloudID: testCloudID}).NewClient(ctx)
	require.ErrorContains(t, err, "obtain Jira service-account access token")
	assert.ErrorContains(t, err, "invalid_client")
	assert.Nil(t, client)
	assert.Empty(t, baseURL)
}
