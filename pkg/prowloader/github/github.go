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
	title    *string
	url      *string
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

func (c *Client) GetPRURL(org, repo string, number int) (*string, error) {
	prEntry, err := c.getPREntry(org, repo, number)
	if err != nil {
		return nil, err
	}
	if prEntry != nil {
		return prEntry.url, nil
	}
	return nil, nil
}

func (c *Client) GetPRTitle(org, repo string, number int) (*string, error) {
	prEntry, err := c.getPREntry(org, repo, number)
	if err != nil {
		return nil, err
	}
	if prEntry != nil {
		return prEntry.title, nil
	}
	return nil, nil
}

// GetPRSHAMerged returns the merge time for a PR/SHA combination. The caching is designed
// to minimize queries to GitHub. We basically have to handle these cases:
//   - the PR doesn't exist (cache as nil)
//   - the PR is unmerged (cache with nil mergedAt)
//   - the PR is merged with a different SHA (cache with the merged sha, return nil)
//   - the PR is merged with the same SHA (cache with the merged sha, return merged time)
func (c *Client) GetPRSHAMerged(org, repo string, number int, sha string) (*time.Time, error) {
	pr, err := c.getPREntry(org, repo, number)
	if err != nil {
		return nil, err
	}
	if pr != nil && pr.sha == sha {
		return pr.mergedAt, nil
	}
	// if it isn't in the cache or the sha doesn't match then return nil
	return nil, nil
}

func (c *Client) getPREntry(org, repo string, number int) (*prentry, error) {
	prl := prlocator{org: org, repo: repo, number: number}
	if val, ok := c.cache[prl]; ok {
		// If it's in the cache return it
		return val, nil
	}
	pr, err := c.fetchPR(prl)
	if err != nil {
		return nil, err
	}
	if pr != nil {
		return pr, nil
	}
	return nil, nil
}

func (c *Client) fetchPR(prl prlocator) (*prentry, error) {
	// Get PR from GitHub
	pr, err := c.prFetch(prl.org, prl.repo, prl.number)
	if err != nil {
		log.WithError(err).
			WithField("org", prl.org).
			WithField("repo", prl.repo).
			WithField("number", prl.number).
			Errorf("error retrieving pull request")

		if resp, ok := err.(*gh.ErrorResponse); ok && resp.Response != nil && resp.Response.StatusCode == http.StatusNotFound {
			// cache nil record to prevent additional fetching
			c.cache[prl] = nil
			return nil, nil
		}
		return nil, err
	}

	var state *prentry
	if pr != nil {
		// Store any pr data we have, so we don't fetch again
		state = &prentry{
			mergedAt: pr.MergedAt,
			title:    pr.Title,
			url:      pr.URL,
		}

		if pr.Head != nil && pr.Head.SHA != nil {
			state.sha = *pr.Head.SHA
		}

		c.cache[prl] = state
	}
	return state, nil
}
