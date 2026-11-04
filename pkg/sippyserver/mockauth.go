package sippyserver

import (
	"net/http"
	"os"
)

const (
	// MockUser is the identity attributed to requests that arrive without an authenticated user when
	// DEV_MODE is enabled. It lets developers exercise user-attributed write flows (triage audit logs,
	// regression force close) without the oauth proxy that sets X-Forwarded-User in production.
	MockUser = "developer"

	// devModeEnvVar is the existing development toggle; non-production deployments such as local
	// development and agentic-staging set it to "1". Production leaves it unset.
	devModeEnvVar = "DEV_MODE"
)

// mockAuthEnabled reports whether requests without an authenticated user should be attributed to
// MockUser. getenv is injected so the decision can be tested without mutating process state.
func mockAuthEnabled(getenv func(string) string) bool {
	return getenv(devModeEnvVar) == "1"
}

// userForRequest returns the authenticated user from the X-Forwarded-User header, falling back to
// MockUser only when mock authentication is enabled. A real header always takes precedence so that
// an environment fronted by the oauth proxy still records the real user.
func userForRequest(req *http.Request, getenv func(string) string) string {
	if user := req.Header.Get("X-Forwarded-User"); user != "" {
		return user
	}
	if mockAuthEnabled(getenv) {
		return MockUser
	}
	return ""
}

// getUserForRequest returns the user making the request, using the process environment to decide
// whether mock authentication applies.
func getUserForRequest(req *http.Request) string {
	return userForRequest(req, os.Getenv)
}
