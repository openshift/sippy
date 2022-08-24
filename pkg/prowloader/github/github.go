package github

import (
	"context"
	"os"
	"time"

	gh "github.com/google/go-github/v45/github"
	"github.com/sirupsen/logrus"
	"github.com/tcnksm/go-gitconfig"
	"golang.org/x/oauth2"
)

type prlocator struct {
	org    string
	repo   string
	number int
	sha    string
}

type Client struct {
	ctx    context.Context
	client *gh.Client
	cache  map[prlocator]*time.Time
}

func New(ctx context.Context) *Client {
	client := &Client{
		ctx:   ctx,
		cache: make(map[prlocator]*time.Time),
	}
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		logrus.Infof("No GitHub token environment variable, checking git config")
		var err error
		token, err = gitconfig.GithubToken()
		if err != nil {
			logrus.WithError(err).Warningf("unable to retrieve GitHub token from git config")
		}
	}

	if token != "" {
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{
				AccessToken: token,
			},
		)
		tc := oauth2.NewClient(client.ctx, ts)
		client.client = gh.NewClient(tc)
	} else {
		logrus.Warningf("using unathenticated GitHub client, requests will be rate-limited")
		client.client = gh.NewClient(nil)
	}

	return client
}

func (c *Client) GetPRMerged(org, repo string, number int, sha string) (*time.Time, error) {
	prl := prlocator{org: org, repo: repo, number: number, sha: sha}
	if val, ok := c.cache[prl]; ok {
		return val, nil
	}

	pr, _, err := c.client.PullRequests.Get(c.ctx, org, repo, number)
	if err != nil {
		return nil, err
	}

	// see if PR was merged yet
	state := pr.MergedAt
	if pr.Head != nil && pr.Head.SHA != nil && *pr.Head.SHA != sha {
		state = nil
	}

	c.cache[prl] = state
	return state, nil
}
