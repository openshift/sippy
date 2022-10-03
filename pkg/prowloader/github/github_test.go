package github

import (
	"context"
	"net/http"
	"testing"
	"time"

	gh "github.com/google/go-github/v45/github"
)

func TestClient_GetPRSHAMerged(t *testing.T) {
	now := time.Now()
	mergedSha := "96dcf2b704502a0b05c4bbff5e8c9bb836449fa6"
	unmergedSha1 := "aff4434f177142ff6ae2e4df895be5173700cbbe"
	unmergedSha2 := "aff4434f177142ff6ae2e4df895be5173700cbbf"

	// We want to minimize the number of API calls to GitHub, this verifies
	// we only called GitHub once for each PR, not each SHA.
	prFetchCalls := 0
	expectedCalls := 3

	PRFetch = func(org, repo string, number int) (*gh.PullRequest, error) {
		prFetchCalls++
		if org == "openshift" && repo == "kubernetes" && number == 1 {
			return &gh.PullRequest{
				MergedAt: &now,
				Head: &gh.PullRequestBranch{
					SHA: &mergedSha,
				},
			}, nil
		} else if org == "openshift" && repo == "kubernetes" && number == 2 {
			return &gh.PullRequest{}, nil
		} else if org == "openshift" && repo == "not-exist" {
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
		ctx:   context.TODO(),
		cache: make(map[prlocator]*prentry),
	}

	tests := []struct {
		name       string
		org        string
		repo       string
		number     int
		sha        string
		wantMerged bool
		wantErr    bool
	}{
		{
			name:       "merged pr with matching sha",
			org:        "openshift",
			repo:       "kubernetes",
			sha:        mergedSha,
			number:     1,
			wantMerged: true,
		},
		{
			name:       "merged pr with other sha",
			org:        "openshift",
			repo:       "kubernetes",
			sha:        unmergedSha1,
			number:     1,
			wantMerged: false,
		},
		{
			name:       "unmerged pr",
			org:        "openshift",
			repo:       "kubernetes",
			sha:        unmergedSha1,
			number:     2,
			wantMerged: false,
		},
		{
			name:       "unmerged pr other sha",
			org:        "openshift",
			repo:       "kubernetes",
			sha:        unmergedSha2,
			number:     2,
			wantMerged: false,
		},
		{
			name:       "not found pr",
			org:        "openshift",
			repo:       "not-exist",
			sha:        unmergedSha1,
			number:     2,
			wantMerged: false,
		},
		{
			name:       "not found pr other sha",
			org:        "openshift",
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
		})
	}

	t.Run("github API calls matched expected times", func(t *testing.T) {
		if prFetchCalls != expectedCalls {
			t.Errorf("GetPRSHAMerged() error, expected %d github api calls, got %d", expectedCalls, prFetchCalls)
			return
		}
	})

}
