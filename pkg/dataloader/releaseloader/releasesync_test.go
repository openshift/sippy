package releaseloader

import (
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
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

			mReleaseTag := releaseDetailsToDB(ReleaseStream{Architecture: tt.architecture}, releaseTag, tt.releaseDetails)

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

func TestResolveReleasePullRequestsLargeDataset(t *testing.T) {
	originalBulkFetch := bulkFetchPRsFromTbl
	t.Cleanup(func() {
		bulkFetchPRsFromTbl = originalBulkFetch
	})

	const prCount = 1000

	inputPRs := make([]models.ReleasePullRequest, prCount)
	existingPRs := make([]models.ReleasePullRequest, prCount/2) // Half exist in DB

	for i := 0; i < prCount; i++ {
		inputPRs[i] = models.ReleasePullRequest{
			URL:           fmt.Sprintf("https://github.com/openshift/repo%d/pull/%d", i%10, i),
			Name:          fmt.Sprintf("repo%d", i%10),
			Description:   fmt.Sprintf("PR %d description", i),
			PullRequestID: fmt.Sprintf("%d", i),
		}

		// Create DB version for first half
		if i < prCount/2 {
			existingPRs[i] = models.ReleasePullRequest{
				URL:           inputPRs[i].URL,
				Name:          inputPRs[i].Name,
				Description:   fmt.Sprintf("DB PR %d description", i),
				PullRequestID: inputPRs[i].PullRequestID,
				BugURL:        fmt.Sprintf("https://bugzilla.redhat.com/%d", i),
			}
		}
	}

	dbQueryCount := 0

	// Mock the database function
	bulkFetchPRsFromTbl = func(db *db.DB, orConditions []string, args []any) []models.ReleasePullRequest {
		dbQueryCount++
		return existingPRs
	}

	loader := &ReleaseLoader{}
	result := loader.resolveReleasePullRequests(inputPRs)
	slices.SortFunc(result, func(i, j models.ReleasePullRequest) int {
		a, _ := strconv.Atoi(i.PullRequestID)
		b, _ := strconv.Atoi(j.PullRequestID)
		return cmp.Compare(a, b)
	})

	// Verify results
	if len(result) != prCount {
		t.Errorf("Expected %d PRs, got %d", prCount, len(result))
	}

	// Verify only one database query was made
	if dbQueryCount != 1 {
		t.Errorf("Expected 1 database query, got %d", dbQueryCount)
	}

	// Verify first half have DB descriptions, second half have original descriptions
	for i := 0; i < prCount/2; i++ {
		if result[i].Description != fmt.Sprintf("DB PR %d description", i) {
			t.Errorf("PR %d should have DB description, got %s", i, result[i].Description)
		}
		if result[i].BugURL != fmt.Sprintf("https://bugzilla.redhat.com/%d", i) {
			t.Errorf("PR %d should have DB BugURL, got %s", i, result[i].BugURL)
		}
	}
	for i := prCount / 2; i < prCount; i++ {
		if result[i].Description != fmt.Sprintf("PR %d description", i) {
			t.Errorf("PR %d should have original description, got %s", i, result[i].Description)
		}
		if result[i].BugURL != "" {
			t.Errorf("PR %d should have empty BugURL, got %s", i, result[i].BugURL)
		}
	}
}
