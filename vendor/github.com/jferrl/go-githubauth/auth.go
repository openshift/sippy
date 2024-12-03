// Package githubauth provides utilities for GitHub authentication,
// including generating and using GitHub App tokens and installation tokens.
// The package is based on the go-github and golang.org/x/oauth2 libraries.
// It implements a set of TokenSource interfaces for generating GitHub App and
// installation tokens.
package githubauth

import (
	"context"
	"crypto/rsa"
	"errors"
	"net/http"
	"strconv"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/google/go-github/v62/github"
	"golang.org/x/oauth2"
)

const (
	// DefaultApplicationTokenExpiration is the default expiration time for the GitHub App token.
	// The expiration time of the JWT, after which it can't be used to request an installation token.
	// The time must be no more than 10 minutes into the future.
	DefaultApplicationTokenExpiration = 10 * time.Minute

	bearerTokenType = "Bearer"
)

// applicationTokenSource represents a GitHub App token source.
// See: https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/generating-a-json-web-token-jwt-for-a-github-app
type applicationTokenSource struct {
	id         int64
	privateKey *rsa.PrivateKey
	expiration time.Duration
}

// ApplicationTokenOpt is a functional option for ApplicationTokenSource.
type ApplicationTokenOpt func(*applicationTokenSource)

// WithApplicationTokenExpiration sets the expiration for the GitHub App token.
// The expiration time of the JWT must be no more than 10 minutes into the future
// and greater than 0. If the provided expiration is invalid, the default expiration is used.
func WithApplicationTokenExpiration(expiration time.Duration) ApplicationTokenOpt {
	return func(a *applicationTokenSource) {
		if expiration > DefaultApplicationTokenExpiration || expiration <= 0 {
			expiration = DefaultApplicationTokenExpiration
		}
		a.expiration = expiration
	}
}

// NewApplicationTokenSource creates a new GitHub App token source using the provided
// application ID and private key. Functional options can be passed to customize the
// token source.
func NewApplicationTokenSource(id int64, privateKey []byte, opts ...ApplicationTokenOpt) (oauth2.TokenSource, error) {
	if id == 0 {
		return nil, errors.New("application id is required")
	}

	privKey, err := jwt.ParseRSAPrivateKeyFromPEM(privateKey)
	if err != nil {
		return nil, err
	}

	t := &applicationTokenSource{
		id:         id,
		privateKey: privKey,
		expiration: DefaultApplicationTokenExpiration,
	}

	for _, opt := range opts {
		opt(t)
	}

	return t, nil
}

// Token generates a new GitHub App token for authenticating as a GitHub App.
func (t *applicationTokenSource) Token() (*oauth2.Token, error) {
	// To protect against clock drift, set the issuance time 60 seconds in the past.
	now := time.Now().Add(-60 * time.Second)
	expiresAt := now.Add(t.expiration)

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.RegisteredClaims{
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(expiresAt),
		Issuer:    strconv.FormatInt(t.id, 10),
	})

	tokenString, err := token.SignedString(t.privateKey)
	if err != nil {
		return nil, err
	}

	return &oauth2.Token{
		AccessToken: tokenString,
		TokenType:   bearerTokenType,
		Expiry:      expiresAt,
	}, nil
}

// InstallationTokenSourceOpt is a functional option for InstallationTokenSource.
type InstallationTokenSourceOpt func(*installationTokenSource)

// WithInstallationTokenOptions sets the options for the GitHub App installation token.
func WithInstallationTokenOptions(opts *github.InstallationTokenOptions) InstallationTokenSourceOpt {
	return func(i *installationTokenSource) {
		i.opts = opts
	}
}

// WithHTTPClient sets the HTTP client for the GitHub App installation token source.
func WithHTTPClient(client *http.Client) InstallationTokenSourceOpt {
	return func(i *installationTokenSource) {
		client.Transport = &oauth2.Transport{
			Source: i.src,
			Base:   client.Transport,
		}

		i.client = github.NewClient(client)
	}
}

// WithEnterpriseURLs sets the base URL and upload URL for the GitHub App installation token source.
// This should passed after WithHTTPClient to ensure the HTTP client is updated with the new URLs.
// If the provided URLs are invalid, the default GitHub URLs are used.
func WithEnterpriseURLs(baseURL, uploadURL string) InstallationTokenSourceOpt {
	return func(i *installationTokenSource) {
		enterpriseClient, err := i.client.WithEnterpriseURLs(baseURL, uploadURL)
		if err != nil {
			return
		}

		i.client = enterpriseClient
	}
}

// WithContext sets the context for the GitHub App installation token source.
func WithContext(ctx context.Context) InstallationTokenSourceOpt {
	return func(i *installationTokenSource) {
		i.ctx = ctx
	}
}

// installationTokenSource represents a GitHub App installation token source.
type installationTokenSource struct {
	id     int64
	ctx    context.Context
	src    oauth2.TokenSource
	client *github.Client
	opts   *github.InstallationTokenOptions
}

// NewInstallationTokenSource creates a new GitHub App installation token source using the provided
// installation ID and token source. Functional options can be passed to customize the
// installation token source.
func NewInstallationTokenSource(id int64, src oauth2.TokenSource, opts ...InstallationTokenSourceOpt) oauth2.TokenSource {
	client := &http.Client{
		Transport: &oauth2.Transport{
			Source: src,
		},
	}

	i := &installationTokenSource{
		id:     id,
		ctx:    context.Background(),
		src:    src,
		client: github.NewClient(client),
	}

	for _, opt := range opts {
		opt(i)
	}

	return i
}

// Token generates a new GitHub App installation token for authenticating as a GitHub App installation.
func (t *installationTokenSource) Token() (*oauth2.Token, error) {
	token, _, err := t.client.Apps.CreateInstallationToken(t.ctx, t.id, t.opts)
	if err != nil {
		return nil, err
	}

	return &oauth2.Token{
		AccessToken: token.GetToken(),
		TokenType:   bearerTokenType,
		Expiry:      token.GetExpiresAt().Time,
	}, nil
}
