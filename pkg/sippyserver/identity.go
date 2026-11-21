package sippyserver

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	// forwardedUserHeader carries the principal authenticated by the oauth proxy in front of Sippy.
	forwardedUserHeader string = "X-Forwarded-User"

	// actingForHeader lets a trusted delegator (for example the Sippy MCP server's service account)
	// name the user it is acting for, so authorization and audit records reflect that user.
	actingForHeader string = "X-Acting-For"

	// MockUser is the identity attributed to requests that arrive without an authenticated user when
	// DEV_MODE is enabled. It lets developers exercise user-attributed write flows (triage audit logs,
	// regression force close) without the oauth proxy that sets X-Forwarded-User in production.
	MockUser string = "developer"

	// devModeEnvVar is the existing development toggle; non-production deployments such as local
	// development and agentic-staging (via hack/agentic_setup.sh) set it to "1". Production leaves
	// it unset.
	devModeEnvVar string = "DEV_MODE"
)

// identityContextKey is the request context key holding the effective user resolved by
// delegationResolver, kept unexported so no other package can forge it.
type identityContextKey struct{}

// errMissingActingFor is returned when a trusted delegator omits the delegated identity on a
// mutating request. It maps to 400 Bad Request, matching ship-status, and the request never falls back
// to the delegator identity.
var errMissingActingFor = fmt.Errorf("%s is required for requests from a trusted delegator", actingForHeader)

// delegationResolver determines the effective requesting user for a request. Principals in the
// trusted delegator allowlist must name the user they act for on mutating requests; everyone else is
// unaffected.
type delegationResolver struct {
	// trustedDelegators holds exact authenticated principal strings allowed to delegate. An empty
	// set enables no delegation.
	trustedDelegators sets.Set[string]
}

// newDelegationResolver builds a resolver from the configured allowlist. Entries are matched exactly.
func newDelegationResolver(trustedDelegators []string) delegationResolver {
	return delegationResolver{trustedDelegators: sets.New[string](trustedDelegators...)}
}

// safeMethods are read-only HTTP methods. A trusted delegator may send them without X-Acting-For
// because they change nothing that needs attribution.
var safeMethods = sets.New[string](http.MethodGet, http.MethodHead, http.MethodOptions)

// resolve returns the effective user for an authenticated principal, the request method, and the raw
// X-Acting-For value. delegated reports whether the identity came from X-Acting-For. Only a non-empty
// authenticated principal that exactly matches an allowlist entry can delegate, so an unauthenticated
// request can never acquire an identity from the header. A trusted delegator must name a user on
// mutating requests; on safe methods without one it keeps its own identity.
func (r delegationResolver) resolve(authenticatedUser, method, actingFor string) (user string, delegated bool, err error) {
	if authenticatedUser == "" || !r.trustedDelegators.Has(authenticatedUser) {
		return authenticatedUser, false, nil
	}
	actingFor = strings.TrimSpace(actingFor)
	if actingFor != "" {
		return actingFor, true, nil
	}
	if safeMethods.Has(method) {
		return authenticatedUser, false, nil
	}
	return "", false, errMissingActingFor
}

// middleware resolves delegation before any handler runs. Mutating requests from trusted delegators
// without a delegated identity are rejected with 400 so no mutation can proceed or be attributed to
// the delegator.
// Resolved identities are stored in the request context for getUserForRequest.
func (r delegationResolver) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		authenticatedUser := req.Header.Get(forwardedUserHeader)
		user, delegated, err := r.resolve(authenticatedUser, req.Method, req.Header.Get(actingForHeader))
		if err != nil {
			log.WithFields(log.Fields{
				"principal": authenticatedUser,
				"method":    req.Method,
				"path":      req.URL.Path,
			}).Warn("trusted delegator request rejected: missing delegated identity")
			failureResponse(w, http.StatusBadRequest, err.Error())
			return
		}
		if !delegated {
			next.ServeHTTP(w, req)
			return
		}
		log.WithFields(log.Fields{
			"principal":  authenticatedUser,
			"acting_for": user,
			"method":     req.Method,
			"path":       req.URL.Path,
		}).Info("delegated request")
		next.ServeHTTP(w, req.WithContext(context.WithValue(req.Context(), identityContextKey{}, user)))
	})
}

// delegatedUserFromContext returns the identity resolved by the delegation middleware, if any. The
// middleware stores only trimmed, non-empty identities, so presence alone means delegation applied.
func delegatedUserFromContext(ctx context.Context) (string, bool) {
	user, ok := ctx.Value(identityContextKey{}).(string)
	return user, ok
}

// mockAuthEnabled reports whether requests without an authenticated user should be attributed to
// MockUser. getenv is injected so the decision can be tested without mutating process state.
func mockAuthEnabled(getenv func(string) string) bool {
	return getenv(devModeEnvVar) == "1"
}

// userForRequest returns the effective requesting user. A delegated identity resolved by the
// delegation middleware wins; otherwise it is the authenticated user from the X-Forwarded-User
// header, falling back to MockUser only when mock authentication is enabled. A real header always
// takes precedence over the mock so that an environment fronted by the oauth proxy still records
// the real user.
func userForRequest(req *http.Request, getenv func(string) string) string {
	if user, ok := delegatedUserFromContext(req.Context()); ok {
		return user
	}
	if user := req.Header.Get(forwardedUserHeader); user != "" {
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
