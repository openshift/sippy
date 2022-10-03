package github

import (
	"context"
	"net/http"
	"os"
	"time"

	gh "github.com/google/go-github/v45/github"
	log "github.com/sirupsen/logrus"
	"github.com/tcnksm/go-gitconfig"
	"golang.org/x/oauth2"
)

type prlocator struct {
	org    string
	repo   string
	number int
}

type prentry struct {
	mergedAt *time.Time
	sha      string
}

type Client struct {
	ctx     context.Context
	cache   map[prlocator]*prentry
	prFetch func(org, repo string, number int) (*gh.PullRequest, error)
}

func New(ctx context.Context) *Client {
	client := &Client{
		ctx:   ctx,
		cache: make(map[prlocator]*prentry),
	}
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		log.Infof("No GitHub token environment variable, checking git config")
		var err error
		token, err = gitconfig.GithubToken()
		if err != nil {
			log.WithError(err).Warningf("unable to retrieve GitHub token from git config")
		}
	}

	var ghc *gh.Client

	if token != "" {
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{
				AccessToken: token,
			},
		)
		tc := oauth2.NewClient(client.ctx, ts)
		ghc = gh.NewClient(tc)
	} else {
		log.Warningf("using unathenticated GitHub client, requests will be rate-limited")
		ghc = gh.NewClient(nil)
	}

	client.prFetch = func(org, repo string, number int) (*gh.PullRequest, error) {
		pr, _, err := ghc.PullRequests.Get(client.ctx, org, repo, number)
		return pr, err
	}

	return client
}

// GetPRSHAMerged returns the merge time for a PR/SHA combination. The caching is designed
// to minimize queries to GitHub. We basically have to handle these cases:
//   - the PR doesn't exist (cache as nil)
//   - the PR is unmerged (cache as nil)
//   - the PR is merged with a different SHA (cache with the merged sha, return nil)
//   - the PR is merged with the same SHA (cache with the merged sha, return merged time)
func (c *Client) GetPRSHAMerged(org, repo string, number int, sha string) (*time.Time, error) {
	prl := prlocator{org: org, repo: repo, number: number}
	if val, ok := c.cache[prl]; ok && val != nil && val.sha == sha {
		// If it's in the cache, and sha matches, this sha was merged.
		return val.mergedAt, nil
	} else if ok {
		// If it's in the cache, then either the PR merged with a different
		// SHA, or has not merged at all.
		return nil, nil
	}

	// Get PR from GitHub
	pr, err := c.prFetch(org, repo, number)
	if err != nil {
		log.WithError(err).
			WithField("org", org).
			WithField("repo", repo).
			WithField("number", number).
			WithField("sha", sha).
			Errorf("error retrieving pull request")

		if resp, ok := err.(*gh.ErrorResponse); ok && resp.Response != nil && resp.Response.StatusCode == http.StatusNotFound {
			c.cache[prl] = nil
			return nil, nil
		}
		return nil, err
	}

	var state *prentry
	if pr != nil && pr.Head != nil && pr.Head.SHA != nil && pr.MergedAt != nil {
		// If PR was merged, store merged sha in cache
		state = &prentry{
			sha:      *pr.Head.SHA,
			mergedAt: pr.MergedAt,
		}
		c.cache[prl] = state
	} else if pr != nil && pr.MergedAt == nil {
		// If PR was not merged yet, store that in cache
		c.cache[prl] = nil
	}

	if state != nil && state.sha == sha {
		return state.mergedAt, nil
	}

	return nil, nil
}
