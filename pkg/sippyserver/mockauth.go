package sippyserver

import (
	"net/http"
	"os"
	"strings"

	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	// MockUser is the identity attributed to requests that arrive without an authenticated user in a
	// non-production environment. It lets developers exercise user-attributed write flows (triage
	// audit logs, regression force close) without the oauth proxy that sets X-Forwarded-User in production.
	MockUser = "developer"

	// EnvironmentEnvVar names the environment variable that identifies the deployment environment.
	// Production leaves it unset, so mock authentication can never activate there by default.
	EnvironmentEnvVar = "SIPPY_ENVIRONMENT"

	// EnvironmentLocal identifies a local development instance (such as the devcontainer).
	EnvironmentLocal = "local"

	// EnvironmentAgenticStaging identifies the agentic-staging environment, which runs against a
	// seeded database without the production auth model.
	EnvironmentAgenticStaging = "agentic-staging"

	// devModeEnvVar is the legacy local development toggle, still honored for mock authentication.
	devModeEnvVar = "DEV_MODE"
)

// mockAuthEnvironments lists the SIPPY_ENVIRONMENT values in which requests authenticate as MockUser.
var mockAuthEnvironments = sets.New[string](EnvironmentLocal, EnvironmentAgenticStaging)

// mockAuthEnabled reports whether requests without an authenticated user should be attributed to
// MockUser. getenv is injected so the decision can be tested without mutating process state.
func mockAuthEnabled(getenv func(string) string) bool {
	if getenv(devModeEnvVar) == "1" {
		return true
	}
	env := strings.ToLower(strings.TrimSpace(getenv(EnvironmentEnvVar)))
	return mockAuthEnvironments.Has(env)
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
