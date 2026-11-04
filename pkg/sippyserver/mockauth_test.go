package sippyserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
)

func envFrom(vars map[string]string) func(string) string {
	return func(key string) string { return vars[key] }
}

func TestMockAuthEnabled(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{name: "production leaves environment unset", env: map[string]string{}, want: false},
		{name: "local environment", env: map[string]string{EnvironmentEnvVar: "local"}, want: true},
		{name: "agentic-staging environment", env: map[string]string{EnvironmentEnvVar: "agentic-staging"}, want: true},
		{name: "environment value is case and whitespace insensitive", env: map[string]string{EnvironmentEnvVar: " Agentic-Staging "}, want: true},
		{name: "unrecognized environment", env: map[string]string{EnvironmentEnvVar: "production"}, want: false},
		{name: "staging without agentic prefix is not mocked", env: map[string]string{EnvironmentEnvVar: "staging"}, want: false},
		{name: "legacy DEV_MODE", env: map[string]string{devModeEnvVar: "1"}, want: true},
		{name: "DEV_MODE other than 1", env: map[string]string{devModeEnvVar: "true"}, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, mockAuthEnabled(envFrom(tc.env)))
		})
	}
}

func TestUserForRequest(t *testing.T) {
	tests := []struct {
		name   string
		header string
		env    map[string]string
		want   string
	}{
		{name: "production with header", header: "alice", env: map[string]string{}, want: "alice"},
		{name: "production without header is unauthenticated", env: map[string]string{}, want: ""},
		{name: "local without header uses mock user", env: map[string]string{EnvironmentEnvVar: EnvironmentLocal}, want: MockUser},
		{name: "agentic-staging without header uses mock user", env: map[string]string{EnvironmentEnvVar: EnvironmentAgenticStaging}, want: MockUser},
		{name: "real header wins over mock user", header: "alice", env: map[string]string{EnvironmentEnvVar: EnvironmentAgenticStaging}, want: "alice"},
		{name: "legacy DEV_MODE without header uses mock user", env: map[string]string{devModeEnvVar: "1"}, want: MockUser},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.header != "" {
				req.Header.Set("X-Forwarded-User", tc.header)
			}
			assert.Equal(t, tc.want, userForRequest(req, envFrom(tc.env)))
		})
	}
}

// TestForceCloseRegressionsAuth verifies the force close endpoint refuses unauthenticated requests in
// production but accepts them as the mock user in local and agentic-staging. An empty body is sent so
// an authenticated request stops at body validation (400) before any database access.
func TestForceCloseRegressionsAuth(t *testing.T) {
	tests := []struct {
		name        string
		environment string
		devMode     string
		wantStatus  int
	}{
		{name: "production rejects missing user", wantStatus: http.StatusUnauthorized},
		{name: "local authenticates as mock user", environment: EnvironmentLocal, wantStatus: http.StatusBadRequest},
		{name: "agentic-staging authenticates as mock user", environment: EnvironmentAgenticStaging, wantStatus: http.StatusBadRequest},
		{name: "legacy DEV_MODE authenticates as mock user", devMode: "1", wantStatus: http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(EnvironmentEnvVar, tc.environment)
			t.Setenv(devModeEnvVar, tc.devMode)

			req := httptest.NewRequest(http.MethodPost, "/api/component_readiness/triages/1/force_close_regressions", strings.NewReader(""))
			req = mux.SetURLVars(req, map[string]string{"id": "1"})
			rec := httptest.NewRecorder()

			(&Server{}).jsonForceCloseRegressions(rec, req)

			assert.Equal(t, tc.wantStatus, rec.Code, rec.Body.String())
		})
	}
}
