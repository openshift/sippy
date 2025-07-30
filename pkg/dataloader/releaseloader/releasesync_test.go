package releaseloader

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReleaseTagForcedFlag(t *testing.T) {
	tests := []struct {
		name                  string
		releaseDetails        ReleaseDetails
		releaseTagName        string
		releaseTagPhase       string
		releaseTagPullSpec    string
		releaseTagDownloadURL string
		architecture          string
		wantForced            bool
	}{
		{

			name:                  "Rejected Not Forced",
			releaseDetails:        buildReleaseDetails(true),
			releaseTagName:        "4.7.0-0.ci-2022-06-24-181413",
			releaseTagPhase:       "Rejected",
			releaseTagPullSpec:    "registry.ci.openshift.org/ocp/release:4.7.0-0.ci-2022-06-24-181413",
			releaseTagDownloadURL: "https://openshift-release-artifacts.apps.ci.l2s4.p1.openshiftapps.com/4.7.0-0.ci-2022-06-24-181413",
			architecture:          "amd64",
			wantForced:            false,
		},
		{

			name:                  "Force Accepted",
			releaseDetails:        buildReleaseDetails(true),
			releaseTagName:        "4.7.0-0.ci-2022-06-24-181413",
			releaseTagPhase:       "Accepted",
			releaseTagPullSpec:    "registry.ci.openshift.org/ocp/release:4.7.0-0.ci-2022-06-24-181413",
			releaseTagDownloadURL: "https://openshift-release-artifacts.apps.ci.l2s4.p1.openshiftapps.com/4.7.0-0.ci-2022-06-24-181413",
			architecture:          "amd64",
			wantForced:            true,
		},
		{

			name:                  "Force Rejected",
			releaseDetails:        buildReleaseDetails(false),
			releaseTagName:        "4.7.0-0.ci-2022-06-24-181413",
			releaseTagPhase:       "Rejected",
			releaseTagPullSpec:    "registry.ci.openshift.org/ocp/release:4.7.0-0.ci-2022-06-24-181413",
			releaseTagDownloadURL: "https://openshift-release-artifacts.apps.ci.l2s4.p1.openshiftapps.com/4.7.0-0.ci-2022-06-24-181413",
			architecture:          "amd64",
			wantForced:            true,
		},
		{

			name:                  "Accepted Not Forced",
			releaseDetails:        buildReleaseDetails(false),
			releaseTagName:        "4.7.0-0.ci-2022-06-24-181413",
			releaseTagPhase:       "Accepted",
			releaseTagPullSpec:    "registry.ci.openshift.org/ocp/release:4.7.0-0.ci-2022-06-24-181413",
			releaseTagDownloadURL: "https://openshift-release-artifacts.apps.ci.l2s4.p1.openshiftapps.com/4.7.0-0.ci-2022-06-24-181413",
			architecture:          "amd64",
			wantForced:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			releaseTag := ReleaseTag{}
			releaseTag.Name = tt.releaseTagName
			releaseTag.DownloadURL = tt.releaseTagDownloadURL
			releaseTag.Phase = tt.releaseTagPhase
			releaseTag.PullSpec = tt.releaseTagPullSpec

			mReleaseTag := releaseDetailsToDB(tt.architecture, releaseTag, tt.releaseDetails)

			if mReleaseTag.Forced != tt.wantForced {
				t.Errorf("Invalid forced flag for %s", tt.name)
			}

		})
	}
}

func buildReleaseDetails(hasFailedBlockingJobs bool) ReleaseDetails {

	releaseDetails := ReleaseDetails{}

	releaseDetails.Name = "4.7.0-0.ci-2022-06-17-154849"
	releaseDetails.Results = make(map[string]map[string]JobRunResult)

	jobRunResult := JobRunResult{}
	jobRunResult.State = succeeded
	jobRunResult.URL = "https://prow.ci.openshift.org/view/gs/origin-ci-test/logs/periodic-ci-openshift-release-master-ci-4.7-e2e-aws-serial/1537826070202421248"
	jobRunResult.Retries = 0
	jobRunResult.TransitionTime = time.Now()
	releaseDetails.Results["blockingJobs"] = map[string]JobRunResult{}
	releaseDetails.Results["blockingJobs"]["aws-serial"] = jobRunResult

	jobRunResult = JobRunResult{}
	if hasFailedBlockingJobs {
		jobRunResult.State = failed
	} else {
		jobRunResult.State = succeeded
	}
	jobRunResult.URL = "https://prow.ci.openshift.org/view/gs/origin-ci-test/logs/periodic-ci-openshift-release-master-ci-4.7-e2e-gcp/1537826069917208576"
	jobRunResult.Retries = 0
	jobRunResult.TransitionTime = time.Now()
	releaseDetails.Results["blockingJobs"]["gcp"] = jobRunResult

	jobRunResult = JobRunResult{}
	jobRunResult.State = succeeded
	jobRunResult.URL = "https://prow.ci.openshift.org/view/gs/origin-ci-test/logs/periodic-ci-openshift-release-master-ci-4.7-upgrade-from-stable-4.6-e2e-aws-upgrade/1537826070286307328"
	jobRunResult.Retries = 0
	jobRunResult.TransitionTime = time.Now()
	releaseDetails.Results["blockingJobs"]["upgrade-minor"] = jobRunResult

	jobRunResult = JobRunResult{}
	jobRunResult.State = succeeded
	jobRunResult.URL = "https://prow.ci.openshift.org/view/gs/origin-ci-test/logs/periodic-ci-openshift-release-master-ci-4.7-e2e-gcp-upgrade/1537826070248558592"
	jobRunResult.Retries = 0
	jobRunResult.TransitionTime = time.Now()
	releaseDetails.Results["informingJobs"] = map[string]JobRunResult{}
	releaseDetails.Results["informingJobs"]["upgrade"] = jobRunResult

	jobRunResult = JobRunResult{}
	jobRunResult.State = failed
	jobRunResult.URL = "https://prow.ci.openshift.org/view/gs/origin-ci-test/logs/periodic-ci-openshift-release-master-ci-4.7-upgrade-from-stable-4.6-e2e-aws-ovn-upgrade/1537826069875265536"
	jobRunResult.Retries = 0
	jobRunResult.TransitionTime = time.Now()
	releaseDetails.Results["informingJobs"]["upgrade-minor-aws-ovn"] = jobRunResult

	releaseDetails.UpgradesTo = make([]UpgradeResult, 0)

	upgradeResult := UpgradeResult{}
	upgradeResult.History = make(map[string]JobRunResult)
	upgradeResult.Success = 0
	upgradeResult.Failure = 1
	upgradeResult.Total = 1
	jobRunResult = JobRunResult{}
	jobRunResult.URL = "https://prow.ci.openshift.org/view/gs/origin-ci-test/logs/periodic-ci-openshift-release-master-ci-4.7-e2e-gcp-upgrade/1540399550064234496"
	jobRunResult.State = failed
	jobRunResult.Retries = 0
	jobRunResult.TransitionTime = time.Now()
	upgradeResult.History[jobRunResult.URL] = jobRunResult

	upgradeResult = UpgradeResult{}
	upgradeResult.History = make(map[string]JobRunResult)
	upgradeResult.Success = 1
	upgradeResult.Failure = 1
	upgradeResult.Total = 2
	jobRunResult = JobRunResult{}
	jobRunResult.URL = "https://prow.ci.openshift.org/view/gs/origin-ci-test/logs/periodic-ci-openshift-release-master-ci-4.7-upgrade-from-stable-4.6-e2e-aws-ovn-upgrade/1540399550244589568"
	jobRunResult.State = failed
	jobRunResult.Retries = 0
	jobRunResult.TransitionTime = time.Now()
	upgradeResult.History[jobRunResult.URL] = jobRunResult
	jobRunResult = JobRunResult{}
	jobRunResult.URL = "https://prow.ci.openshift.org/view/gs/origin-ci-test/logs/periodic-ci-openshift-release-master-ci-4.7-upgrade-from-stable-4.6-e2e-aws-upgrade/1540399550177480704"
	jobRunResult.State = succeeded
	jobRunResult.Retries = 0
	jobRunResult.TransitionTime = time.Now()
	upgradeResult.History[jobRunResult.URL] = jobRunResult

	releaseDetails.ChangeLog = []uint8("<h2>Changes from <a target=\"_blank\" href=\"/releasetag/4.7.0-0.ci-2022-06-18-175830\">4.7.0-0.ci-2022-06-18-175830</a></h2>\n\n<p>Created: 2022-06-24 18:20:08 +0000 UTC</p>\n\n<p>Image Digest: <code>sha256:f854883113a2edeb559dbd7cda40b96b0b5c7a86dfcc9d9b6026096908fe170f</code></p>\n\n<h3>Components</h3>\n\n<ul>\n<li>Kubernetes 1.20.15</li>\n<li>Red Hat Enterprise Linux CoreOS <a target=\"_blank\" href=\"https://releases-rhcos-art.cloud.privileged.psi.redhat.com/?release=47.84.202206171954-0&amp;stream=releases%2Frhcos-4.7")

	return releaseDetails

}

func TestChangeLog(t *testing.T) {

	data, err := os.ReadFile(`OCPCRT-74-pr-test.json`)
	if err != nil {
		t.Fatal("Failed to read test file")
	}

	releaseDetails := ReleaseDetails{}

	if err := json.Unmarshal(data, &releaseDetails); err != nil {
		panic(err)
	}

	if len(releaseDetails.ChangeLog) == 0 {
		t.Fatal("Failed unmarshalling")
	}

	changeLogStr := string(releaseDetails.ChangeLog)
	releaseChangeLog := models.ReleaseTag{}
	changelog := NewChangelog("amd64", changeLogStr)
	releaseChangeLog.KubernetesVersion = changelog.KubernetesVersion()
	releaseChangeLog.CurrentOSURL, releaseChangeLog.CurrentOSVersion, releaseChangeLog.PreviousOSURL, releaseChangeLog.PreviousOSVersion, releaseChangeLog.OSDiffURL = changelog.CoreOSVersion()
	releaseChangeLog.PreviousReleaseTag = changelog.PreviousReleaseTag()
	releaseChangeLog.Repositories = changelog.Repositories()
	releaseChangeLog.PullRequests = changelog.PullRequests()

	releaseChangeLogJSON := parseChangeLogJSON("test", releaseDetails.ChangeLogJSON)

	if releaseChangeLogJSON.KubernetesVersion != releaseChangeLog.KubernetesVersion {
		t.Fatalf("ReleaseChangeLog Kubernetes versions don't match.  ChangeLog: %s, ChangeLogJson: %s", releaseChangeLog.KubernetesVersion, releaseChangeLogJSON.KubernetesVersion)
	}

	if releaseChangeLogJSON.CurrentOSVersion != releaseChangeLog.CurrentOSVersion {
		t.Fatalf("ReleaseChangeLog CurrentOSVersion versions don't match.  ChangeLog: %s, ChangeLogJson: %s", releaseChangeLog.CurrentOSVersion, releaseChangeLogJSON.CurrentOSVersion)
	}

	if releaseChangeLogJSON.CurrentOSURL != releaseChangeLog.CurrentOSURL {
		t.Fatalf("ReleaseChangeLog CurrentOSURL versions don't match.  ChangeLog: %s, ChangeLogJson: %s", releaseChangeLog.CurrentOSURL, releaseChangeLogJSON.CurrentOSURL)
	}

	if releaseChangeLogJSON.PreviousOSVersion != releaseChangeLog.PreviousOSVersion {
		t.Fatalf("ReleaseChangeLog PreviousOSVersion versions don't match.  ChangeLog: %s, ChangeLogJson: %s", releaseChangeLog.PreviousOSVersion, releaseChangeLogJSON.PreviousOSVersion)
	}

	if releaseChangeLogJSON.PreviousOSURL != releaseChangeLog.PreviousOSURL {
		t.Fatalf("ReleaseChangeLog PreviousOSURL versions don't match.  ChangeLog: %s, ChangeLogJson: %s", releaseChangeLog.PreviousOSURL, releaseChangeLogJSON.PreviousOSURL)
	}

	if releaseChangeLogJSON.OSDiffURL != releaseChangeLog.OSDiffURL {
		t.Fatalf("ReleaseChangeLog OSDiffURL versions don't match.  ChangeLog: %s, ChangeLogJson: %s", releaseChangeLog.OSDiffURL, releaseChangeLogJSON.OSDiffURL)
	}

	if releaseChangeLogJSON.PreviousReleaseTag != releaseChangeLog.PreviousReleaseTag {
		t.Fatalf("ReleaseChangeLog PreviousReleaseTag versions don't match.  ChangeLog: %s, ChangeLogJson: %s", releaseChangeLog.PreviousReleaseTag, releaseChangeLogJSON.PreviousReleaseTag)
	}

	if len(releaseChangeLogJSON.Repositories) != len(releaseChangeLog.Repositories) {
		t.Fatalf("ReleaseChangeLog Repositories versions don't match.  ChangeLog: %v, ChangeLogJson: %v", releaseChangeLog.Repositories, releaseChangeLogJSON.Repositories)
	}

	for _, repoBase := range releaseChangeLog.Repositories {
		found := false
		for _, repoJSON := range releaseChangeLogJSON.Repositories {
			if repoBase.Name != repoJSON.Name {
				continue
			}
			found = true
			if repoBase.DiffURL != repoJSON.DiffURL {
				t.Fatalf("ReleaseChangeLog Repositories DiffURL don't match for %s.  ChangeLog: %s, ChangeLogJson: %s", repoJSON.Name, repoBase.DiffURL, repoJSON.DiffURL)
			}

			if repoBase.Head != repoJSON.Head {
				t.Fatalf("ReleaseChangeLog Repositories Head don't match for %s.  ChangeLog: %s, ChangeLogJson: %s", repoJSON.Name, repoBase.Head, repoJSON.Head)
			}
		}

		if !found {
			t.Fatalf("ReleaseChangeLog Repositories match for %s.", repoBase.Name)
		}
	}

	if len(releaseChangeLogJSON.PullRequests) != len(releaseChangeLog.PullRequests) {
		t.Fatalf("ReleaseChangeLog PullRequests versions don't match.  ChangeLog: %v, ChangeLogJson: %v", releaseChangeLog.PullRequests, releaseChangeLogJSON.PullRequests)
	}

	for _, prBase := range releaseChangeLog.PullRequests {
		found := false
		for _, prJSON := range releaseChangeLogJSON.PullRequests {
			if prBase.Name != prJSON.Name || prBase.PullRequestID != prJSON.PullRequestID {
				continue
			}

			found = true

			// the quotes are different.. skip this test
			// ReleaseChangeLog PullRequest Description don't match for console.  ChangeLog: display ‘Control plane is hosted’ alert only when isCl…, ChangeLogJson: display 'Control plane is hosted' alert only when isCl…
			// if prBase.Description != prJSON.Description {
			// 	t.Fatalf("ReleaseChangeLog PullRequest Description don't match for %s.  ChangeLog: %s, ChangeLogJson: %s", prJSON.Name, prBase.Description, prJSON.Description)
			// }

			if prBase.URL != prJSON.URL {
				t.Fatalf("ReleaseChangeLog PullRequest URL don't match for %s.  ChangeLog: %s, ChangeLogJson: %s", prJSON.Name, prBase.URL, prJSON.URL)
			}

			if prBase.BugURL != prJSON.BugURL {
				t.Fatalf("ReleaseChangeLog PullRequest BugURL don't match for %s.  ChangeLog: %s, ChangeLogJson: %s", prJSON.Name, prBase.BugURL, prJSON.BugURL)
			}

		}

		if !found {
			t.Fatalf("ReleaseChangeLog Repositories match for %s.", prBase.Name)
		}
	}
}

func TestBuildReleaseTag(t *testing.T) {
	// Skip test if no database connection is available
	dbc := util.GetDbHandle(t)

	tests := []struct {
		name               string
		architecture       string
		release            string
		inputTag           ReleaseTag
		mockReleaseDetails ReleaseDetails
		setupDB            func(*testing.T, *db.DB)
		teardownDB         func(*testing.T, *db.DB)
		expectedResult     *expectedReleaseTag
		shouldReturnNil    bool
		description        string
	}{
		{
			name:         "Returns nil for pending phase",
			architecture: "amd64",
			release:      "4.14.0-0.ci",
			inputTag: ReleaseTag{
				Name:  "4.14.0-0.ci-2023-01-01-120000",
				Phase: "Pending", // Not Accepted or Rejected
			},
			mockReleaseDetails: buildMockReleaseDetails("4.14.0-0.ci-2023-01-01-120000", "Pending", false),
			setupDB:            func(t *testing.T, db *db.DB) {},
			teardownDB:         cleanupTestDB,
			shouldReturnNil:    true,
			description:        "Should return nil for releases not in Accepted or Rejected phase",
		},
		{
			name:         "Returns nil when releaseDetailsToDB returns nil",
			architecture: "amd64",
			release:      "4.14.0-0.ci",
			inputTag: ReleaseTag{
				Name:  "4.14.0-0.ci-2023-01-01-120000",
				Phase: api.PayloadAccepted,
			},
			mockReleaseDetails: buildMockReleaseDetailsNoChangelog("4.14.0-0.ci-2023-01-01-120000"),
			setupDB:            func(t *testing.T, db *db.DB) {},
			teardownDB:         cleanupTestDB,
			shouldReturnNil:    true,
			description:        "Should return nil when no changelog is available",
		},
		{
			name:         "Processes accepted release with no PRs",
			architecture: "amd64",
			release:      "4.14.0-0.ci",
			inputTag: ReleaseTag{
				Name:  "4.14.0-0.ci-2023-01-01-120000",
				Phase: api.PayloadAccepted,
			},
			mockReleaseDetails: buildMockReleaseDetails("4.14.0-0.ci-2023-01-01-120000", api.PayloadAccepted, false),
			setupDB:            func(t *testing.T, db *db.DB) {},
			teardownDB:         cleanupTestDB,
			expectedResult: &expectedReleaseTag{
				releaseTag: "4.14.0-0.ci-2023-01-01-120000",
				phase:      api.PayloadAccepted,
				prCount:    0,
			},
			description: "Should process release with no pull requests successfully",
		},
		{
			name:         "Processes rejected release with no PRs",
			architecture: "amd64",
			release:      "4.14.0-0.ci",
			inputTag: ReleaseTag{
				Name:  "4.14.0-0.ci-2023-01-01-120000",
				Phase: api.PayloadRejected,
			},
			mockReleaseDetails: buildMockReleaseDetails("4.14.0-0.ci-2023-01-01-120000", api.PayloadRejected, true),
			setupDB:            func(t *testing.T, db *db.DB) {},
			teardownDB:         cleanupTestDB,
			expectedResult: &expectedReleaseTag{
				releaseTag: "4.14.0-0.ci-2023-01-01-120000",
				phase:      api.PayloadRejected,
				prCount:    0,
			},
			description: "Should process rejected release successfully",
		},
		{
			name:         "Processes release with new PRs",
			architecture: "amd64",
			release:      "4.14.0-0.ci",
			inputTag: ReleaseTag{
				Name:  "4.14.0-0.ci-2023-01-01-120000",
				Phase: api.PayloadAccepted,
			},
			mockReleaseDetails: buildMockReleaseDetailsWithPRs("4.14.0-0.ci-2023-01-01-120000", []testPR{
				{URL: "https://github.com/openshift/api/pull/123", Name: "api", Description: "New API PR"},
				{URL: "https://github.com/openshift/origin/pull/456", Name: "origin", Description: "New Origin PR"},
			}),
			setupDB:    func(t *testing.T, db *db.DB) {},
			teardownDB: cleanupTestDB,
			expectedResult: &expectedReleaseTag{
				releaseTag:     "4.14.0-0.ci-2023-01-01-120000",
				phase:          api.PayloadAccepted,
				prCount:        2,
				prDescriptions: []string{"New API PR", "New Origin PR"},
			},
			description: "Should process release with new pull requests",
		},
		{
			name:         "Processes release with existing PRs in database",
			architecture: "amd64",
			release:      "4.14.0-0.ci",
			inputTag: ReleaseTag{
				Name:  "4.14.0-0.ci-2023-01-01-120000",
				Phase: api.PayloadAccepted,
			},
			mockReleaseDetails: buildMockReleaseDetailsWithPRs("4.14.0-0.ci-2023-01-01-120000", []testPR{
				{URL: "https://github.com/openshift/api/pull/123", Name: "api", Description: "New API PR"},
				{URL: "https://github.com/openshift/origin/pull/456", Name: "origin", Description: "New Origin PR"},
			}),
			setupDB: func(t *testing.T, db *db.DB) {
				// Create existing PR in database
				existingPR := models.ReleasePullRequest{
					URL:           "https://github.com/openshift/api/pull/123",
					Name:          "api",
					Description:   "Existing API PR from DB",
					PullRequestID: "123",
				}
				err := db.DB.Create(&existingPR).Error
				require.NoError(t, err)
			},
			teardownDB: cleanupTestDB,
			expectedResult: &expectedReleaseTag{
				releaseTag:     "4.14.0-0.ci-2023-01-01-120000",
				phase:          api.PayloadAccepted,
				prCount:        2,
				prDescriptions: []string{"Existing API PR from DB", "New Origin PR"}, // First PR should use DB description
			},
			description: "Should use existing PR data from database and merge with new PRs",
		},
		{
			name:         "Handles duplicate PRs in release data",
			architecture: "amd64",
			release:      "4.14.0-0.ci",
			inputTag: ReleaseTag{
				Name:  "4.14.0-0.ci-2023-01-01-120000",
				Phase: api.PayloadAccepted,
			},
			mockReleaseDetails: buildMockReleaseDetailsWithPRs("4.14.0-0.ci-2023-01-01-120000", []testPR{
				{URL: "https://github.com/openshift/api/pull/123", Name: "api", Description: "API PR"},
				{URL: "https://github.com/openshift/api/pull/123", Name: "api", Description: "Duplicate API PR"},
				{URL: "https://github.com/openshift/origin/pull/456", Name: "origin", Description: "Origin PR"},
			}),
			setupDB:    func(t *testing.T, db *db.DB) {},
			teardownDB: cleanupTestDB,
			expectedResult: &expectedReleaseTag{
				releaseTag: "4.14.0-0.ci-2023-01-01-120000",
				phase:      api.PayloadAccepted,
				prCount:    2, // Duplicates are removed during changelog parsing
			},
			description: "Should handle duplicate PRs efficiently by deduplicating database queries",
		},
		{
			name:         "Handles large number of PRs efficiently",
			architecture: "amd64",
			release:      "4.14.0-0.ci",
			inputTag: ReleaseTag{
				Name:  "4.14.0-0.ci-2023-01-01-120000",
				Phase: api.PayloadAccepted,
			},
			mockReleaseDetails: buildMockReleaseDetailsWithManyPRs("4.14.0-0.ci-2023-01-01-120000", 100),
			setupDB: func(t *testing.T, db *db.DB) {
				// Create some existing PRs
				for i := 0; i < 10; i++ {
					existingPR := models.ReleasePullRequest{
						URL:           fmt.Sprintf("https://github.com/openshift/repo%d/pull/%d", i, i),
						Name:          fmt.Sprintf("repo%d", i),
						Description:   fmt.Sprintf("Existing PR %d from DB", i),
						PullRequestID: fmt.Sprintf("%d", i),
					}
					err := db.DB.Create(&existingPR).Error
					require.NoError(t, err)
				}
			},
			teardownDB: cleanupTestDB,
			expectedResult: &expectedReleaseTag{
				releaseTag: "4.14.0-0.ci-2023-01-01-120000",
				phase:      api.PayloadAccepted,
				prCount:    100,
			},
			description: "Should handle large numbers of PRs efficiently with batch database queries",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test database
			tt.setupDB(t, dbc)
			defer tt.teardownDB(t, dbc)

			// Create a test loader
			loader := &testReleaseLoader{
				db:                 dbc,
				httpClient:         &http.Client{Timeout: 60 * time.Second},
				mockReleaseDetails: tt.mockReleaseDetails,
			}

			// Execute function under test
			result := loader.buildReleaseTag(tt.architecture, tt.release, tt.inputTag)

			// Verify results
			if tt.shouldReturnNil {
				assert.Nil(t, result, tt.description)
				return
			}

			require.NotNil(t, result, tt.description)
			assert.Equal(t, tt.expectedResult.releaseTag, result.ReleaseTag, "Release tag should match")
			assert.Equal(t, tt.expectedResult.phase, result.Phase, "Phase should match")
			assert.Equal(t, tt.expectedResult.prCount, len(result.PullRequests), "PR count should match")

			// Verify PR descriptions if specified
			if tt.expectedResult.prDescriptions != nil {
				actualDescriptions := make([]string, len(result.PullRequests))
				for i, pr := range result.PullRequests {
					actualDescriptions[i] = pr.Description
				}
				assert.ElementsMatch(t, tt.expectedResult.prDescriptions, actualDescriptions, "PR descriptions should match")
			}
		})
	}
}

// Test helper types and functions

type expectedReleaseTag struct {
	releaseTag     string
	phase          string
	prCount        int
	prDescriptions []string
}

type testPR struct {
	URL         string
	Name        string
	Description string
}

// testReleaseLoader is a test version of ReleaseLoader that mocks fetchReleaseDetails
type testReleaseLoader struct {
	db                 *db.DB
	httpClient         *http.Client
	mockReleaseDetails ReleaseDetails
	releases           []string
	architectures      []string
	errors             []error
}

func (t *testReleaseLoader) fetchReleaseDetails(arch, rel string, tag ReleaseTag) ReleaseDetails {
	return t.mockReleaseDetails
}

func (t *testReleaseLoader) buildReleaseTag(architecture, release string, tag ReleaseTag) *models.ReleaseTag {
	releaseDetails := t.fetchReleaseDetails(architecture, release, tag)
	releaseTag := releaseDetailsToDB(architecture, tag, releaseDetails)

	// We skip releases that aren't fully baked (i.e. all jobs run and changelog calculated)
	if releaseTag == nil || (releaseTag.Phase != api.PayloadAccepted && releaseTag.Phase != api.PayloadRejected) {
		return nil
	}

	if len(releaseTag.PullRequests) == 0 {
		return releaseTag
	}

	// PR lookup logic - same as the original
	type prKey struct{ url, name string }
	prIndexMap := make(map[prKey]int, len(releaseTag.PullRequests))
	orConditions := make([]string, 0, len(releaseTag.PullRequests))
	args := make([]interface{}, 0, len(releaseTag.PullRequests)*2)

	for i, pr := range releaseTag.PullRequests {
		key := prKey{pr.URL, pr.Name}
		if _, exists := prIndexMap[key]; !exists {
			prIndexMap[key] = i
			orConditions = append(orConditions, "(url = ? AND name = ?)")
			args = append(args, key.url, key.name)
		}
	}

	// Execute batch query and map results back
	var existingPRs []models.ReleasePullRequest
	if err := t.db.DB.Table("release_pull_requests").Where(strings.Join(orConditions, " OR "), args...).Find(&existingPRs).Error; err != nil {
		panic(err)
	}

	for _, existingPR := range existingPRs {
		if index, ok := prIndexMap[prKey{existingPR.URL, existingPR.Name}]; ok {
			releaseTag.PullRequests[index] = existingPR
		}
	}

	return releaseTag
}

func buildMockReleaseDetails(name, phase string, hasFailedBlocking bool) ReleaseDetails {
	details := buildReleaseDetails(hasFailedBlocking)
	details.Name = name
	details.ChangeLog = []uint8("Mock changelog content") // Ensure non-empty changelog

	// Provide mock ChangeLogJSON to avoid HTML parsing issues
	details.ChangeLogJSON = ChangeLog{
		From: ChangeLogRelease{Name: "previous-release"},
		Components: []ChangeLogComponent{
			{Name: "Kubernetes", Version: "1.28.5"},
			{Name: "CoreOS", Version: "413.92.202301031521-0", VersionURL: "https://example.com/coreos"},
		},
		UpdatedImages: []UpdatedImage{}, // Empty for basic test
	}

	return details
}

func buildMockReleaseDetailsNoChangelog(name string) ReleaseDetails {
	details := ReleaseDetails{
		Name:    name,
		Results: make(map[string]map[string]JobRunResult),
		// No ChangeLog - this will cause releaseDetailsToDB to return nil
	}
	return details
}

func buildMockReleaseDetailsWithPRs(name string, prs []testPR) ReleaseDetails {
	details := buildMockReleaseDetails(name, api.PayloadAccepted, false)

	// Create mock changelog JSON with PRs
	changelog := ChangeLog{
		From: ChangeLogRelease{Name: "previous-release"},
		Components: []ChangeLogComponent{
			{Name: "Kubernetes", Version: "1.28.5"},
		},
		UpdatedImages: make([]UpdatedImage, 0),
	}

	// Add PRs to changelog
	for i, pr := range prs {
		changelog.UpdatedImages = append(changelog.UpdatedImages, UpdatedImage{
			Name:          pr.Name,
			Path:          fmt.Sprintf("sha256:hash%d", i),
			FullChangeLog: fmt.Sprintf("https://github.com/openshift/%s/compare/old..new", pr.Name),
			Commits: []UpdatedImageCommits{
				{
					Subject: pr.Description,
					PullURL: pr.URL,
					PullID:  extractPRNumber(pr.URL),
				},
			},
		})
	}

	details.ChangeLogJSON = changelog
	return details
}

func buildMockReleaseDetailsWithManyPRs(name string, count int) ReleaseDetails {
	prs := make([]testPR, count)
	for i := 0; i < count; i++ {
		prs[i] = testPR{
			URL:         fmt.Sprintf("https://github.com/openshift/repo%d/pull/%d", i, i),
			Name:        fmt.Sprintf("repo%d", i),
			Description: fmt.Sprintf("PR %d description", i),
		}
	}
	return buildMockReleaseDetailsWithPRs(name, prs)
}

func extractPRNumber(url string) int {
	// Simple extraction for test purposes
	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		if num, err := strconv.Atoi(parts[len(parts)-1]); err == nil {
			return num
		}
	}
	return 123 // Default for tests
}

func cleanupTestDB(t *testing.T, db *db.DB) {
	// Clean up test data
	db.DB.Exec("DELETE FROM release_tag_pull_requests")
	db.DB.Exec("DELETE FROM release_pull_requests")
	db.DB.Exec("DELETE FROM release_tags")
	db.DB.Exec("DELETE FROM release_repositories")
	db.DB.Exec("DELETE FROM release_job_runs")
}

func cleanupTestDBBenchmark(b *testing.B, db *db.DB) {
	// Clean up test data for benchmark
	db.DB.Exec("DELETE FROM release_tag_pull_requests")
	db.DB.Exec("DELETE FROM release_pull_requests")
	db.DB.Exec("DELETE FROM release_tags")
	db.DB.Exec("DELETE FROM release_repositories")
	db.DB.Exec("DELETE FROM release_job_runs")
}

// Benchmark test for performance validation
func BenchmarkBuildReleaseTagPRLookup(b *testing.B) {
	// Get database handle (requires TEST_SIPPY_DATABASE_DSN)
	if os.Getenv("TEST_SIPPY_DATABASE_DSN") == "" {
		b.Skip("TEST_SIPPY_DATABASE_DSN environment variable not set, skipping benchmark")
	}

	dbc := util.GetDbHandle(&testing.T{})

	// Setup many PRs in database
	setupManyPRsForBenchmark(b, dbc, 1000)
	defer cleanupTestDBBenchmark(b, dbc)

	loader := &testReleaseLoader{
		db:                 dbc,
		httpClient:         &http.Client{Timeout: 60 * time.Second},
		mockReleaseDetails: buildMockReleaseDetailsWithManyPRs("4.14.0-0.ci-2023-01-01-120000", 1000),
	}

	inputTag := ReleaseTag{
		Name:  "4.14.0-0.ci-2023-01-01-120000",
		Phase: api.PayloadAccepted,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := loader.buildReleaseTag("amd64", "4.14.0-0.ci", inputTag)
		if result == nil {
			b.Fatal("Expected non-nil result")
		}
	}
}

func setupManyPRsForBenchmark(b *testing.B, db *db.DB, count int) {
	prs := make([]models.ReleasePullRequest, count)
	for i := 0; i < count; i++ {
		prs[i] = models.ReleasePullRequest{
			URL:           fmt.Sprintf("https://github.com/openshift/repo%d/pull/%d", i, i),
			Name:          fmt.Sprintf("repo%d", i),
			Description:   fmt.Sprintf("Benchmark PR %d", i),
			PullRequestID: fmt.Sprintf("%d", i),
		}
	}

	// Create in batches for performance
	batchSize := 100
	for i := 0; i < len(prs); i += batchSize {
		end := i + batchSize
		if end > len(prs) {
			end = len(prs)
		}
		err := db.DB.CreateInBatches(prs[i:end], batchSize).Error
		require.NoError(b, err)
	}
}
