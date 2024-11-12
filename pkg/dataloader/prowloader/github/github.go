package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"sync"
	"time"

	gh "github.com/google/go-github/v45/github"
	ghauth "github.com/jferrl/go-githubauth"
	log "github.com/sirupsen/logrus"
	"github.com/tcnksm/go-gitconfig"
	"golang.org/x/oauth2"
)

const commentIDRegex = `META\s*=\s*{(?P<meta>[^}]*)`

// if we have fewer than this threshold remaining we will report rate limited
const rateLimitThreshold = 500

const GitHubAppID = 1046118

// openshift github orgs that our app has access to
type GitHubOrg string

const OpenshiftOrg = GitHubOrg("openshift")
const OpenshiftEngOrg = GitHubOrg("openshift-eng")

type prlocator struct {
	org    string
	repo   string
	number int
}

type PREntry struct {
	MergedAt *time.Time
	SHA      string
	Title    *string
	URL      *string
	Login    *string
	State    *string
}

type Client struct {
	ctx                 context.Context
	cache               map[prlocator]*PREntry
	cacheLock           sync.RWMutex
	closedCache         map[string]map[string]map[int]*gh.PullRequest
	closedCacheLock     sync.RWMutex
	prFetch             func(org, repo string, number int) (*gh.PullRequest, error)
	prCommentsFetch     func(org, repo string, number int) ([]*gh.IssueComment, error)
	prCommentCreate     func(org, repo string, number int, comment string) (*gh.IssueComment, error)
	prCommentDelete     func(org, repo string, updateID int64) error
	gitHubCoreRateFetch func() (*gh.Rate, error)
	gitHubListClosedPRs func(org, repo string) (map[int]*gh.PullRequest, error)
	commentMetaRegEx    *regexp.Regexp
}

func New(ctx context.Context, org GitHubOrg) *Client {
	client := &Client{
		ctx:         ctx,
		cache:       make(map[prlocator]*PREntry),
		closedCache: make(map[string]map[string]map[int]*gh.PullRequest),
	}
	ghc := gh.NewClient(newGHAuthClient(client.ctx, org))

	client.prFetch = func(org, repo string, number int) (*gh.PullRequest, error) {
		pr, _, err := ghc.PullRequests.Get(client.ctx, org, repo, number)
		return pr, err
	}

	client.prCommentCreate = func(org, repo string, number int, comment string) (*gh.IssueComment, error) {
		ghComment := &gh.IssueComment{Body: &comment}
		commentResponse, _, err := ghc.Issues.CreateComment(client.ctx, org, repo, number, ghComment)
		return commentResponse, err
	}

	client.prCommentDelete = func(org, repo string, updateID int64) error {
		_, err := ghc.Issues.DeleteComment(client.ctx, org, repo, updateID)
		return err
	}

	client.prCommentsFetch = func(org, repo string, number int) ([]*gh.IssueComment, error) {
		issueCommentOptions := &gh.IssueListCommentsOptions{}
		issueComments, _, err := ghc.Issues.ListComments(client.ctx, org, repo, number, issueCommentOptions)
		return issueComments, err
	}

	client.gitHubCoreRateFetch = func() (*gh.Rate, error) {
		rateLimits, _, err := ghc.RateLimits(client.ctx)
		if err != nil {
			return nil, err
		}
		if rateLimits == nil {
			return nil, nil
		}
		return rateLimits.Core, nil
	}

	client.gitHubListClosedPRs = func(org, repo string) (map[int]*gh.PullRequest, error) {
		since := time.Now().Add(-time.Hour * 48)
		response := make(map[int]*gh.PullRequest)
		// larger page size fewer requests counting against our api rate
		pageSize := 50
		currentPage := 0

		for {

			prs, _, err := ghc.PullRequests.List(ctx, org, repo, &gh.PullRequestListOptions{State: "closed", Sort: "updated", Direction: "desc", ListOptions: gh.ListOptions{Page: currentPage, PerPage: pageSize}})

			if err != nil {
				return response, err
			}

			currentPage += len(prs)
			lastPage := len(prs) < pageSize

			for _, pr := range prs {
				if pr != nil && pr.Number != nil {
					response[*pr.Number] = pr

					if pr.UpdatedAt != nil && pr.UpdatedAt.Before(since) {
						lastPage = true
					}
				}
			}

			if lastPage {
				return response, nil
			}
		}
	}

	client.commentMetaRegEx = regexp.MustCompile(commentIDRegex)

	return client
}

// we could use the app token to look up github app installation ids at https://api.github.com/app/installations
// but it's not like they will change, so we can just hard code them, for one less thing to go wrong
var installationIdForOrg = map[GitHubOrg]int64{
	OpenshiftOrg:    56889436,
	OpenshiftEngOrg: 56889451,
}

func newGHAuthClient(ctx context.Context, org GitHubOrg) *http.Client {
	if tokenSource := newAppTokenSource(); tokenSource != nil {
		// create an org-specific self-renewing token source
		installationTokenSource := ghauth.NewInstallationTokenSource(installationIdForOrg[org], tokenSource, ghauth.WithContext(ctx))
		log.Infof("using GitHub App credentials for org %s", org)
		return oauth2.NewClient(ctx, installationTokenSource)
	}

	// no app creds, try to use a personal access token
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		log.Infof("No GitHub token environment variable, checking git config")
		var err error
		token, err = gitconfig.GithubToken()
		if err != nil {
			log.WithError(err).Warningf("unable to retrieve GitHub token from git config")
		}
	}
	if token != "" {
		log.Infof("using GitHub access token for org %s", org)
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: token},
		)
		return oauth2.NewClient(ctx, ts)
	}

	// make a no-auth client if no token is available
	log.Warningf("using unathenticated GitHub client, requests will be rate-limited")
	return nil
}

func newAppTokenSource() oauth2.TokenSource {
	// check that the environment variables are set
	privateKey := os.Getenv("GITHUB_APP_CLIENT_KEY")
	if privateKey == "" {
		log.Warn("missing GITHUB_APP_CLIENT_KEY, will not authenticate as GitHub App")
		return nil
	}
	// create top-level token source for the application
	appTokenSource, err := ghauth.NewApplicationTokenSource(GitHubAppID, []byte(privateKey))
	if err != nil {
		log.Errorf("Error creating application token source: %s", err)
		return nil
	}
	return appTokenSource
}

func (c *Client) IsPrRecentlyMerged(org, repo string, number int) (*time.Time, *string, error) {
	c.closedCacheLock.Lock()
	defer c.closedCacheLock.Unlock()
	if c.closedCache[org] == nil {
		c.closedCache[org] = make(map[string]map[int]*gh.PullRequest)
	}

	var err error
	if c.closedCache[org][repo] == nil {
		c.closedCache[org][repo], err = c.gitHubListClosedPRs(org, repo)

		// we expect that gitHubListClosedPRs will return a map, possibly partially filled
		// so log the error for now and then we will return it once we check to see if we have data for this request or not
		if err != nil {
			log.WithError(err).Errorf("Error fetching closed PRs for %s/%s", org, repo)
		}
	}

	pr := c.closedCache[org][repo][number]
	if pr != nil && pr.Number != nil && *pr.Number == number {
		return pr.MergedAt, pr.MergeCommitSHA, err
	}
	// we didn't find it
	return nil, nil, err
}

func (c *Client) IsWithinRateLimitThreshold() bool {
	rate, err := c.gitHubCoreRateFetch()

	if err != nil {
		// presume we are rate limited if we can't even get the rate limit...
		return true
	}

	if rate == nil {
		// for now assume rate limited if we can't get the rate
		return true
	}

	log.Infof("Github Limit:%d, Remaining:%d", rate.Limit, rate.Remaining)

	return rate.Remaining < rateLimitThreshold
}

func (c *Client) GetPRURL(org, repo string, number int) (*string, error) {
	prEntry, err := c.GetPREntry(org, repo, number)
	if err != nil {
		return nil, err
	}
	if prEntry != nil {
		return prEntry.URL, nil
	}
	return nil, nil
}

func (c *Client) GetPRTitle(org, repo string, number int) (*string, error) {
	prEntry, err := c.GetPREntry(org, repo, number)
	if err != nil {
		return nil, err
	}
	if prEntry != nil {
		return prEntry.Title, nil
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
	pr, err := c.GetPREntry(org, repo, number)
	if err != nil {
		return nil, err
	}
	if pr != nil && pr.SHA == sha {
		return pr.MergedAt, nil
	}
	// if it isn't in the cache or the sha doesn't match then return nil
	return nil, nil
}

func (c *Client) GetPREntry(org, repo string, number int) (*PREntry, error) {
	c.cacheLock.Lock()
	defer c.cacheLock.Unlock()
	prl := prlocator{org: org, repo: repo, number: number}
	if val, ok := c.cache[prl]; ok {
		// If it's in the cache return it
		return val, nil
	}

	// Get PR from GitHub
	pr, err := c.PRFetch(prl.org, prl.repo, prl.number)
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

	c.cache[prl] = pr
	return pr, nil
}

// PRFetch is an uncached call to github to get the most up to date information
// on the PR.  Use cautiously and only when necessary
func (c *Client) PRFetch(org, repo string, number int) (prEntry *PREntry, err error) {
	// Get PR from GitHub
	pr, err := c.prFetch(org, repo, number)
	if err != nil {
		return nil, err
	}

	if pr != nil {
		// Store any pr data we have, so we don't fetch again
		prEntry = &PREntry{
			MergedAt: pr.MergedAt,
			Title:    pr.Title,
			URL:      pr.HTMLURL,
			State:    pr.State,
		}

		if pr.User != nil && pr.User.Login != nil {
			prEntry.Login = pr.User.Login
		}

		if pr.Head != nil && pr.Head.SHA != nil {
			prEntry.SHA = *pr.Head.SHA
		}
	}

	return prEntry, nil
}

func (c *Client) CreatePRComment(org, repo string, number int, comment string) error {
	_, err := c.prCommentCreate(org, repo, number, comment)
	return err
}

func (c *Client) DeletePRComment(org, repo string, updateID int64) error {
	err := c.prCommentDelete(org, repo, updateID)
	return err
}

func (c *Client) FindCommentID(org, repo string, number int, commentKey, commentID string) (*int64, *string, error) {
	comments, err := c.prCommentsFetch(org, repo, number)

	if err != nil {
		return nil, nil, err
	}

	for _, cmt := range comments {

		if c.isCommentIDMatch(*cmt.Body, commentKey, commentID) {
			return cmt.ID, cmt.Body, nil
		}
	}
	return nil, nil, nil
}

func (c *Client) isCommentIDMatch(comment, commentKey, commentID string) bool {
	match := c.commentMetaRegEx.FindStringSubmatch(comment)

	if match != nil {
		index := c.commentMetaRegEx.SubexpIndex("meta")

		if index > -1 {
			metaJSON := fmt.Sprintf("{%s}", match[index])

			var result map[string]interface{}
			err := json.Unmarshal([]byte(metaJSON), &result)

			if err != nil {
				log.WithError(err).Errorf("Error searching for commentId: %s, match", commentID)
			} else {
				if value, ok := result[commentKey]; ok {
					if value == commentID {
						return true
					}
				}
			}
		}
	}
	return false
}
