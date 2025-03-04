package github

import (
	"context"
	"net/http"
	"regexp"
	"testing"
	"time"

	gh "github.com/google/go-github/v45/github"
)

const (
	openshift  = "openshift"
	kubernetes = "kubernetes"
)

func TestClient_GetPRSHAMerged(t *testing.T) {
	now := time.Now()
	mergedSha := "96dcf2b704502a0b05c4bbff5e8c9bb836449fa6"
	unmergedSha1 := "aff4434f177142ff6ae2e4df895be5173700cbbe"
	unmergedSha2 := "aff4434f177142ff6ae2e4df895be5173700cbbf"

	pr1Title := "pr1"
	pr1URL := "link/to/pr/1"

	// We want to minimize the number of API calls to GitHub, this verifies
	// we only called GitHub once for each PR, not each SHA, Title or URL.
	prFetchCalls := 0
	expectedCalls := 3

	prFetch := func(org, repo string, number int) (*gh.PullRequest, error) {
		prFetchCalls++
		switch {
		case org == openshift && repo == kubernetes && number == 1:
			return &gh.PullRequest{
				MergedAt: &now,
				Head: &gh.PullRequestBranch{
					SHA: &mergedSha,
				},
				Title:   &pr1Title,
				HTMLURL: &pr1URL,
			}, nil
		case org == openshift && repo == kubernetes && number == 2:
			return &gh.PullRequest{}, nil
		case org == openshift && repo == "not-exist":
			return nil, &gh.ErrorResponse{
				Response: &http.Response{
					StatusCode: 404,
					Status:     "Not Found",
				},
			}
		}
		return nil, nil
	}

	client := &Client{
		ctx:     context.TODO(),
		prFetch: prFetch,
		cache:   make(map[prlocator]*PREntry),
	}

	tests := []struct {
		name       string
		org        string
		repo       string
		number     int
		sha        string
		title      string
		url        string
		wantMerged bool
		wantErr    bool
	}{
		{
			name:       "merged pr with matching sha",
			org:        openshift,
			repo:       kubernetes,
			sha:        mergedSha,
			title:      pr1Title,
			url:        pr1URL,
			number:     1,
			wantMerged: true,
		},
		{
			name:       "merged pr with other sha",
			org:        openshift,
			repo:       kubernetes,
			sha:        unmergedSha1,
			title:      pr1Title,
			url:        pr1URL,
			number:     1,
			wantMerged: false,
		},
		{
			name:       "unmerged pr",
			org:        openshift,
			repo:       kubernetes,
			sha:        unmergedSha1,
			number:     2,
			wantMerged: false,
		},
		{
			name:       "unmerged pr other sha",
			org:        openshift,
			repo:       kubernetes,
			sha:        unmergedSha2,
			number:     2,
			wantMerged: false,
		},
		{
			name:       "not found pr",
			org:        openshift,
			repo:       "not-exist",
			sha:        unmergedSha1,
			number:     2,
			wantMerged: false,
		},
		{
			name:       "not found pr other sha",
			org:        openshift,
			repo:       "not-exist",
			sha:        unmergedSha2,
			number:     2,
			wantMerged: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := client.GetPRSHAMerged(tt.org, tt.repo, tt.number, tt.sha)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetPRSHAMerged() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantMerged && got == nil {
				t.Errorf("GetPRSHAMerged() want merged, got unmerged")
				return
			}

			title, err := client.GetPRTitle(tt.org, tt.repo, tt.number)

			if err != nil {
				t.Errorf("GetPRTitle() error = %v", err)
				return
			}
			if title == nil && tt.title != "" {
				t.Errorf("GetPRTitle() want : %s, got nil", tt.title)
				return
			} else if title != nil && *title != tt.title {
				t.Errorf("GetPRTitle() want : %s, got: %s", tt.title, *title)
				return
			}

			url, err := client.GetPRURL(tt.org, tt.repo, tt.number)
			if err != nil {
				t.Errorf("GetPRURL() error = %v", err)
				return
			}
			if url == nil && tt.url != "" {
				t.Errorf("GetPRURL() want : %s, got nil", tt.url)
				return
			} else if url != nil && *url != tt.url {
				t.Errorf("GetPRURL() want : %s, got: %s", tt.url, *url)
				return
			}

		})
	}

	t.Run("github API calls matched expected times", func(t *testing.T) {
		if prFetchCalls != expectedCalls {
			t.Errorf("GetPRSHAMerged() error, expected %d github api calls, got %d", expectedCalls, prFetchCalls)
			return
		}
	})

}

func TestClient_IsCommentIdMatch(t *testing.T) {
	client := &Client{commentMetaRegEx: regexp.MustCompile(commentIDRegex)}

	tests := []struct {
		name          string
		comment       string
		commentKey    string
		commentID     string
		expectedMatch bool
	}{
		{
			name:          "match key and id",
			comment:       "<!-- META={\"trt_comment_id\": \"sha1\"} -->\ncomment\ntext",
			commentKey:    "trt_comment_id",
			commentID:     "sha1",
			expectedMatch: true,
		},
		{
			name:          "match id not key",
			comment:       "<!-- META={\"trt_alt_comment_id\": \"sha1\"} -->\ncomment\ntext",
			commentKey:    "trt_comment_id",
			commentID:     "sha1",
			expectedMatch: false,
		},
		{
			name:          "match key not id",
			comment:       "<!-- META={\"trt_comment_id\": \"sha1\"} -->\ncomment\ntext",
			commentKey:    "trt_comment_id",
			commentID:     "sha11",
			expectedMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.expectedMatch != client.isCommentIDMatch(tt.comment, tt.commentKey, tt.commentID) {
				t.Errorf("isCommentIdMatch did not match expected: %v for:%s, key: %s, id: %s, comment: %s", tt.expectedMatch, tt.name, tt.commentKey, tt.commentID, tt.comment)
				return
			}
		})
	}

}
