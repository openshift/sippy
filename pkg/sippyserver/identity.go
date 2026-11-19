package sippyserver

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	// forwardedUserHeader carries the principal authenticated by the oauth proxy in front of Sippy.
	forwardedUserHeader = "X-Forwarded-User"

	// actingForHeader lets a trusted delegator (for example the production MCP server's service
	// account) name the user it is acting for, so authorization and audit records reflect that user.
	actingForHeader = "X-Acting-For"
)

// identityContextKey is the request context key holding the effective user resolved by
// delegationResolver, kept unexported so no other package can forge it.
type identityContextKey struct{}

// errMissingActingFor is returned when a trusted delegator omits the delegated identity. It maps to
// 400 Bad Request, matching ship-status, and the request never falls back to the delegator identity.
var errMissingActingFor = fmt.Errorf("%s is required for requests from a trusted delegator", actingForHeader)

// delegationResolver determines the effective requesting user for a request. Principals in the
// trusted delegator allowlist must name the user they act for; everyone else is unaffected.
type delegationResolver struct {
	// trustedDelegators holds exact authenticated principal strings allowed to delegate. An empty
	// set enables no delegation.
	trustedDelegators sets.Set[string]
}

// newDelegationResolver builds a resolver from the configured allowlist. Entries are matched exactly.
func newDelegationResolver(trustedDelegators []string) delegationResolver {
	return delegationResolver{trustedDelegators: sets.New[string](trustedDelegators...)}
}

// resolve returns the effective user for an authenticated principal and the raw X-Acting-For value.
// delegated reports whether the identity came from X-Acting-For. Only a non-empty authenticated
// principal that exactly matches an allowlist entry can delegate, so an unauthenticated request can
// never acquire an identity from the header.
func (r delegationResolver) resolve(authenticatedUser, actingFor string) (user string, delegated bool, err error) {
	if authenticatedUser == "" || !r.trustedDelegators.Has(authenticatedUser) {
		return authenticatedUser, false, nil
	}
	actingFor = strings.TrimSpace(actingFor)
	if actingFor == "" {
		return "", false, errMissingActingFor
	}
	return actingFor, true, nil
}

// middleware resolves delegation before any handler runs. Requests from trusted delegators without a
// delegated identity are rejected with 400 so no mutation can proceed or be attributed to the delegator.
// Resolved identities are stored in the request context for getUserForRequest.
func (r delegationResolver) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		authenticatedUser := req.Header.Get(forwardedUserHeader)
		user, delegated, err := r.resolve(authenticatedUser, req.Header.Get(actingForHeader))
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

// delegatedUserFromContext returns the identity resolved by the delegation middleware, if any.
func delegatedUserFromContext(ctx context.Context) (string, bool) {
	user, ok := ctx.Value(identityContextKey{}).(string)
	return user, ok && user != ""
}
