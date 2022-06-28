package releasesync

import (
	"testing"
	"time"
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
