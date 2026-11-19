package sippyserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	delegatorA string = "system:serviceaccount:sippy:sippy-mcp"
	delegatorB string = "other-trusted-bot"
)

func TestDelegationResolverResolve(t *testing.T) {
	twoDelegators := []string{delegatorA, delegatorB}
	tests := []struct {
		name          string
		allowlist     []string
		authenticated string
		// method defaults to POST, a mutating request.
		method        string
		actingFor     string
		wantUser      string
		wantDelegated bool
		wantErr       bool
	}{
		{name: "first delegator acts for user", allowlist: twoDelegators, authenticated: delegatorA, actingFor: "alice", wantUser: "alice", wantDelegated: true},
		{name: "second delegator acts for user", allowlist: twoDelegators, authenticated: delegatorB, actingFor: "bob", wantUser: "bob", wantDelegated: true},
		{name: "delegated identity is trimmed", allowlist: twoDelegators, authenticated: delegatorA, actingFor: "  alice\t", wantUser: "alice", wantDelegated: true},
		{name: "delegator without acting for is rejected", allowlist: twoDelegators, authenticated: delegatorA, wantErr: true},
		{name: "delegator with blank acting for is rejected", allowlist: twoDelegators, authenticated: delegatorB, actingFor: "   ", wantErr: true},
		{name: "delegator put without acting for is rejected", allowlist: twoDelegators, authenticated: delegatorA, method: http.MethodPut, wantErr: true},
		{name: "delegator delete without acting for is rejected", allowlist: twoDelegators, authenticated: delegatorA, method: http.MethodDelete, wantErr: true},
		{name: "delegator patch without acting for is rejected", allowlist: twoDelegators, authenticated: delegatorA, method: http.MethodPatch, wantErr: true},
		{name: "delegator get without acting for keeps its identity", allowlist: twoDelegators, authenticated: delegatorA, method: http.MethodGet, wantUser: delegatorA},
		{name: "delegator head with blank acting for keeps its identity", allowlist: twoDelegators, authenticated: delegatorA, method: http.MethodHead, actingFor: " ", wantUser: delegatorA},
		{name: "delegator options without acting for keeps its identity", allowlist: twoDelegators, authenticated: delegatorB, method: http.MethodOptions, wantUser: delegatorB},
		{name: "delegator get with acting for is delegated", allowlist: twoDelegators, authenticated: delegatorA, method: http.MethodGet, actingFor: "alice", wantUser: "alice", wantDelegated: true},
		{name: "nil allowlist enables no delegation", allowlist: nil, authenticated: delegatorA, actingFor: "alice", wantUser: delegatorA},
		{name: "empty allowlist enables no delegation", allowlist: []string{}, authenticated: delegatorA, actingFor: "alice", wantUser: delegatorA},
		{name: "unlisted chai-bot principal cannot delegate", allowlist: twoDelegators, authenticated: "chai-bot", actingFor: "mallory", wantUser: "chai-bot"},
		{name: "direct user ignores acting for", allowlist: twoDelegators, authenticated: "alice", actingFor: "mallory", wantUser: "alice"},
		{name: "direct user without acting for is unchanged", allowlist: twoDelegators, authenticated: "alice", wantUser: "alice"},
		{name: "unauthenticated request cannot acquire identity", allowlist: twoDelegators, authenticated: "", actingFor: "alice", wantUser: ""},
		{name: "empty allowlist entry does not trust unauthenticated requests", allowlist: []string{""}, authenticated: "", actingFor: "alice", wantUser: ""},
		{name: "suffix of a delegator is not trusted", allowlist: twoDelegators, authenticated: "sippy-mcp", actingFor: "alice", wantUser: "sippy-mcp"},
		{name: "same namespace service account is not trusted", allowlist: twoDelegators, authenticated: "system:serviceaccount:sippy:other", actingFor: "alice", wantUser: "system:serviceaccount:sippy:other"},
		{name: "superstring of a delegator is not trusted", allowlist: twoDelegators, authenticated: delegatorB + "-2", actingFor: "alice", wantUser: delegatorB + "-2"},
		{name: "case differences are not trusted", allowlist: twoDelegators, authenticated: strings.ToUpper(delegatorB), actingFor: "alice", wantUser: strings.ToUpper(delegatorB)},
		{name: "padded principal is not trusted", allowlist: twoDelegators, authenticated: " " + delegatorB, actingFor: "alice", wantUser: " " + delegatorB},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			method := tc.method
			if method == "" {
				method = http.MethodPost
			}
			user, delegated, err := newDelegationResolver(tc.allowlist).resolve(tc.authenticated, method, tc.actingFor)
			if tc.wantErr {
				require.ErrorIs(t, err, errMissingActingFor)
				assert.Empty(t, user, "a rejected request must not fall back to the delegator identity")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantUser, user)
			assert.Equal(t, tc.wantDelegated, delegated)
		})
	}
}

func TestZeroValueDelegationResolverTrustsNobody(t *testing.T) {
	user, delegated, err := delegationResolver{}.resolve(delegatorA, http.MethodPost, "alice")
	require.NoError(t, err)
	assert.Equal(t, delegatorA, user)
	assert.False(t, delegated)
}

// TestDelegationMiddlewareEffectiveUser verifies the user seen by handlers through the shared identity
// helper, including that DEV_MODE mock authentication is unchanged and never delegates.
func TestDelegationMiddlewareEffectiveUser(t *testing.T) {
	tests := []struct {
		name    string
		devMode string
		// method defaults to POST, a mutating request.
		method        string
		forwardedUser string
		actingFor     string
		wantStatus    int
		wantUser      string
	}{
		{name: "delegator acts for user", forwardedUser: delegatorA, actingFor: "alice", wantStatus: http.StatusOK, wantUser: "alice"},
		{name: "second delegator acts for user", forwardedUser: delegatorB, actingFor: "bob", wantStatus: http.StatusOK, wantUser: "bob"},
		{name: "delegator without acting for never reaches handler", forwardedUser: delegatorA, wantStatus: http.StatusBadRequest},
		{name: "delegator with blank acting for never reaches handler", forwardedUser: delegatorA, actingFor: " ", wantStatus: http.StatusBadRequest},
		{name: "delegator read without acting for keeps its identity", method: http.MethodGet, forwardedUser: delegatorA, wantStatus: http.StatusOK, wantUser: delegatorA},
		{name: "delegator read with acting for is delegated", method: http.MethodGet, forwardedUser: delegatorA, actingFor: "alice", wantStatus: http.StatusOK, wantUser: "alice"},
		{name: "unlisted principal keeps its own identity", forwardedUser: "chai-bot", actingFor: "alice", wantStatus: http.StatusOK, wantUser: "chai-bot"},
		{name: "direct user is unchanged", forwardedUser: "alice", wantStatus: http.StatusOK, wantUser: "alice"},
		{name: "unauthenticated production request stays unauthenticated", actingFor: "alice", wantStatus: http.StatusOK, wantUser: ""},
		{name: "DEV_MODE mock user does not delegate", devMode: "1", actingFor: "alice", wantStatus: http.StatusOK, wantUser: MockUser},
		{name: "DEV_MODE still enforces delegation for a trusted header", devMode: "1", forwardedUser: delegatorA, wantStatus: http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(devModeEnvVar, tc.devMode)
			var called bool
			var gotUser string
			next := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				called = true
				gotUser = getUserForRequest(req)
				w.WriteHeader(http.StatusOK)
			})
			method := tc.method
			if method == "" {
				method = http.MethodPost
			}
			req := httptest.NewRequest(method, "/api/component_readiness/triages", nil)
			if tc.forwardedUser != "" {
				req.Header.Set(forwardedUserHeader, tc.forwardedUser)
			}
			if tc.actingFor != "" {
				req.Header.Set(actingForHeader, tc.actingFor)
			}
			rec := httptest.NewRecorder()

			newDelegationResolver([]string{delegatorA, delegatorB}).middleware(next).ServeHTTP(rec, req)

			assert.Equal(t, tc.wantStatus, rec.Code, rec.Body.String())
			if tc.wantStatus != http.StatusOK {
				assert.False(t, called, "rejected request must not reach a handler")
				assert.Contains(t, rec.Body.String(), actingForHeader)
				return
			}
			require.True(t, called)
			assert.Equal(t, tc.wantUser, gotUser)
		})
	}
}

// TestUserAttributedHandlersWithDelegation runs handlers that share getUserForRequest behind the
// delegation middleware. Requests that pass authentication use an empty body so they stop at body
// validation (400) before any database access; the status therefore shows which identity check ran.
func TestUserAttributedHandlersWithDelegation(t *testing.T) {
	s := &Server{delegation: newDelegationResolver([]string{delegatorA})}
	handlers := []struct {
		name    string
		method  string
		path    string
		vars    map[string]string
		handler http.HandlerFunc
		// authenticatedStatus is the status for an authenticated request with an empty body.
		authenticatedStatus int
	}{
		{name: "create triage", method: http.MethodPost, path: "/api/component_readiness/triages", handler: s.jsonCreateTriage, authenticatedStatus: http.StatusBadRequest},
		{name: "update triage", method: http.MethodPut, path: "/api/component_readiness/triages/1", vars: map[string]string{"id": "1"}, handler: s.jsonUpdateTriage, authenticatedStatus: http.StatusBadRequest},
		{name: "force close", method: http.MethodPost, path: "/api/component_readiness/triages/1/force_close_regressions", vars: map[string]string{"id": "1"}, handler: s.jsonForceCloseRegressions, authenticatedStatus: http.StatusBadRequest},
		{name: "file jira bug", method: http.MethodPost, path: "/api/component_readiness/bugs", handler: s.jsonFileJiraBug, authenticatedStatus: http.StatusBadRequest},
	}
	identities := []struct {
		name          string
		forwardedUser string
		actingFor     string
		authenticated bool
		wantStatus    int
	}{
		{name: "delegated user", forwardedUser: delegatorA, actingFor: "alice", authenticated: true},
		{name: "direct user", forwardedUser: "alice", authenticated: true},
		{name: "unlisted principal with acting for", forwardedUser: "chai-bot", actingFor: "alice", authenticated: true},
		{name: "delegator without acting for", forwardedUser: delegatorA, wantStatus: http.StatusBadRequest},
		{name: "unauthenticated with acting for", actingFor: "alice", wantStatus: http.StatusUnauthorized},
	}
	for _, h := range handlers {
		for _, id := range identities {
			t.Run(h.name+"/"+id.name, func(t *testing.T) {
				t.Setenv(devModeEnvVar, "")
				req := httptest.NewRequest(h.method, h.path, strings.NewReader(""))
				if h.vars != nil {
					req = mux.SetURLVars(req, h.vars)
				}
				if id.forwardedUser != "" {
					req.Header.Set(forwardedUserHeader, id.forwardedUser)
				}
				if id.actingFor != "" {
					req.Header.Set(actingForHeader, id.actingFor)
				}
				rec := httptest.NewRecorder()

				s.delegation.middleware(h.handler).ServeHTTP(rec, req)

				want := id.wantStatus
				if id.authenticated {
					want = h.authenticatedStatus
				}
				assert.Equal(t, want, rec.Code, rec.Body.String())
			})
		}
	}
}

// TestDeleteTriageRequiresUser verifies delete refuses an unauthenticated request before touching
// the database, with or without an X-Acting-For header.
func TestDeleteTriageRequiresUser(t *testing.T) {
	for _, actingFor := range []string{"", "alice"} {
		t.Run("acting for "+actingFor, func(t *testing.T) {
			t.Setenv(devModeEnvVar, "")
			s := &Server{delegation: newDelegationResolver([]string{delegatorA})}
			req := httptest.NewRequest(http.MethodDelete, "/api/component_readiness/triages/1", nil)
			req = mux.SetURLVars(req, map[string]string{"id": "1"})
			if actingFor != "" {
				req.Header.Set(actingForHeader, actingFor)
			}
			rec := httptest.NewRecorder()

			s.delegation.middleware(http.HandlerFunc(s.jsonDeleteTriage)).ServeHTTP(rec, req)

			assert.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
		})
	}
}

func envFrom(vars map[string]string) func(string) string {
	return func(key string) string { return vars[key] }
}

func TestMockAuthEnabled(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{name: "production leaves DEV_MODE unset", env: map[string]string{}, want: false},
		{name: "DEV_MODE enabled", env: map[string]string{devModeEnvVar: "1"}, want: true},
		{name: "DEV_MODE other than 1", env: map[string]string{devModeEnvVar: "true"}, want: false},
		{name: "DEV_MODE disabled", env: map[string]string{devModeEnvVar: "0"}, want: false},
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
		{name: "DEV_MODE without header uses mock user", env: map[string]string{devModeEnvVar: "1"}, want: MockUser},
		{name: "real header wins over mock user", header: "alice", env: map[string]string{devModeEnvVar: "1"}, want: "alice"},
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
// production but accepts them as the mock user when DEV_MODE is enabled. An empty body is sent so an
// authenticated request stops at body validation (400) before any database access.
func TestForceCloseRegressionsAuth(t *testing.T) {
	tests := []struct {
		name       string
		devMode    string
		wantStatus int
	}{
		{name: "production rejects missing user", wantStatus: http.StatusUnauthorized},
		{name: "DEV_MODE authenticates as mock user", devMode: "1", wantStatus: http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(devModeEnvVar, tc.devMode)

			req := httptest.NewRequest(http.MethodPost, "/api/component_readiness/triages/1/force_close_regressions", strings.NewReader(""))
			req = mux.SetURLVars(req, map[string]string{"id": "1"})
			rec := httptest.NewRecorder()

			(&Server{}).jsonForceCloseRegressions(rec, req)

			assert.Equal(t, tc.wantStatus, rec.Code, rec.Body.String())
		})
	}
}
