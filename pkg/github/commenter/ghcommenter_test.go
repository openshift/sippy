package commenter

import (
	"testing"
)

func TestGitHubCommenter_IsRepoIncluded(t *testing.T) {

	tests := []struct {
		name           string
		include        []string
		exclude        []string
		org            string
		repo           string
		expectIncluded bool
		expectError    bool
	}{
		{
			name:           "test excluded",
			include:        []string{`org1/repo2`},
			exclude:        []string{`org1/repo1`},
			org:            "org1",
			repo:           "repo1",
			expectIncluded: false,
		},
		{
			name:           "test included AND excluded",
			include:        []string{`org1/repo1`},
			exclude:        []string{`org1/repo1`},
			org:            "org1",
			repo:           "repo1",
			expectIncluded: false,
		},
		{
			name:           "test included",
			include:        []string{`org1/repo2`},
			exclude:        []string{`org1/repo1`},
			org:            "org1",
			repo:           "repo2",
			expectIncluded: true,
		},
		{
			name:           "test NOT included with include",
			include:        []string{`org1/repo2`},
			exclude:        []string{`org1/repo1`},
			org:            "org1",
			repo:           "repo3",
			expectIncluded: false,
		},
		{
			name:           "test NOT included WITHOUT include",
			exclude:        []string{`org1/repo1`},
			org:            "org1",
			repo:           "repo3",
			expectIncluded: true,
		},
		{
			name:           "test default org not included",
			include:        []string{`repo1`},
			org:            "org1",
			repo:           "repo1",
			expectIncluded: false,
		},
		{
			name:           "test default org included",
			include:        []string{`repo1`},
			org:            "openshift",
			repo:           "repo1",
			expectIncluded: true,
		},
		{
			name:           "test default org not excluded",
			exclude:        []string{`repo1`},
			org:            "org1",
			repo:           "repo1",
			expectIncluded: true,
		},
		{
			name:           "test default org excluded",
			exclude:        []string{`repo1`},
			org:            "openshift",
			repo:           "repo1",
			expectIncluded: false,
		},
		{
			name:           "test multi excluded not excluded",
			exclude:        []string{`repo1`, `org1/repo2`},
			org:            "org1",
			repo:           "repo1",
			expectIncluded: true,
		},
		{
			name:           "test multi excluded excluded",
			exclude:        []string{`repo1`, `org1/repo2`},
			org:            "openshift",
			repo:           "repo1",
			expectIncluded: false,
		},
		{
			name:        "test bad excluded",
			exclude:     []string{`repo1`, `org1/repo2`, `org2/repo1/bad`},
			org:         "openshift",
			repo:        "repo1",
			expectError: true,
		},
		{
			name:        "test bad included",
			include:     []string{`repo1`, `org1/repo2`, `org2/repo1/bad`},
			org:         "openshift",
			repo:        "repo1",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			ghCommenter, err := NewGitHubCommenter(nil, nil, tt.exclude, tt.include)

			// invalid ghCommenter at this point
			// we expected the error
			if err != nil && tt.expectError {
				return
			}

			if (err != nil) != tt.expectError {
				t.Fatalf("Test: %s error did not match expected error: %t", tt.name, tt.expectError)
			}

			if tt.expectIncluded != ghCommenter.IsRepoIncluded(tt.org, tt.repo) {
				t.Fatalf("Test: %s - Org: %s, Repo: %s did not match expected inclusion check: %t", tt.name, tt.org, tt.repo, tt.expectIncluded)
			}
		})
	}
}
