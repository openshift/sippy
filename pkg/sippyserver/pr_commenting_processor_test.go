package sippyserver

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/gcs"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/util"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestMatchPriorRiskAnalysisTest(t *testing.T) {

	tests := map[string]struct {
		priorRiskAnalysisJSON    string
		riskAnalysisJSON         string
		expectedSummaryTestCount int
		expectedRiskLevel        api.RiskLevel
	}{
		"MatchAll": {
			priorRiskAnalysisJSON:    `{"ProwJobName":"pull-ci-openshift-origin-master-e2e-aws-ovn-upgrade","ProwJobRunID":1684917114550358016,"Release":"Presubmits","CompareRelease":"4.14","Tests":[{"Name":"[sig-arch] Only known images used by tests","Risk":{"Level":{"Name":"High","Level":100},"Reasons":["This test has passed 99.04% of 3767 runs on release 4.14 [Overall] in the last week."]},"OpenBugs":[]},{"Name":"Cluster upgrade.[sig-apps] job-upgrade","Risk":{"Level":{"Name":"High","Level":100},"Reasons":["This test has passed 99.14% of 1280 runs on release 4.14 [Overall] in the last week."]},"OpenBugs":[]}],"OverallRisk":{"Level":{"Name":"High","Level":100},"Reasons":["Maximum failed test risk:High"]},"OpenBugs":[]}`,
			riskAnalysisJSON:         `{"ProwJobName":"pull-ci-openshift-origin-master-e2e-aws-ovn-upgrade","ProwJobRunID":1684985307247677440,"Release":"Presubmits","CompareRelease":"4.14","Tests":[{"Name":"[sig-arch] Only known images used by tests","Risk":{"Level":{"Name":"High","Level":100},"Reasons":["This test has passed 99.04% of 3767 runs on release 4.14 [Overall] in the last week."]},"OpenBugs":[]},{"Name":"Cluster upgrade.[sig-apps] job-upgrade","Risk":{"Level":{"Name":"High","Level":100},"Reasons":["This test has passed 99.14% of 1280 runs on release 4.14 [Overall] in the last week."]},"OpenBugs":[]}],"OverallRisk":{"Level":{"Name":"High","Level":100},"Reasons":["Maximum failed test risk:High"]},"OpenBugs":[]}`,
			expectedSummaryTestCount: 2,
			expectedRiskLevel:        api.FailureRiskLevelHigh,
		},
		"MatchOnePrior": {
			priorRiskAnalysisJSON:    `{"ProwJobName":"pull-ci-openshift-origin-master-e2e-aws-ovn-upgrade","ProwJobRunID":1684917114550358016,"Release":"Presubmits","CompareRelease":"4.14","Tests":[{"Name":"[sig-arch] Only known images used by tests","Risk":{"Level":{"Name":"High","Level":100},"Reasons":["This test has passed 99.04% of 3767 runs on release 4.14 [Overall] in the last week."]},"OpenBugs":[]}],"OverallRisk":{"Level":{"Name":"High","Level":100},"Reasons":["Maximum failed test risk:High"]},"OpenBugs":[]}`,
			riskAnalysisJSON:         `{"ProwJobName":"pull-ci-openshift-origin-master-e2e-aws-ovn-upgrade","ProwJobRunID":1684985307247677440,"Release":"Presubmits","CompareRelease":"4.14","Tests":[{"Name":"[sig-arch] Only known images used by tests","Risk":{"Level":{"Name":"High","Level":100},"Reasons":["This test has passed 99.04% of 3767 runs on release 4.14 [Overall] in the last week."]},"OpenBugs":[]},{"Name":"Cluster upgrade.[sig-apps] job-upgrade","Risk":{"Level":{"Name":"High","Level":100},"Reasons":["This test has passed 99.14% of 1280 runs on release 4.14 [Overall] in the last week."]},"OpenBugs":[]}],"OverallRisk":{"Level":{"Name":"High","Level":100},"Reasons":["Maximum failed test risk:High"]},"OpenBugs":[]}`,
			expectedSummaryTestCount: 1,
			expectedRiskLevel:        api.FailureRiskLevelHigh,
		},
		"MatchOneCurrent": {
			priorRiskAnalysisJSON:    `{"ProwJobName":"pull-ci-openshift-origin-master-e2e-aws-ovn-upgrade","ProwJobRunID":1684917114550358016,"Release":"Presubmits","CompareRelease":"4.14","Tests":[{"Name":"[sig-arch] Only known images used by tests","Risk":{"Level":{"Name":"High","Level":100},"Reasons":["This test has passed 99.04% of 3767 runs on release 4.14 [Overall] in the last week."]},"OpenBugs":[]},{"Name":"Cluster upgrade.[sig-apps] job-upgrade","Risk":{"Level":{"Name":"High","Level":100},"Reasons":["This test has passed 99.14% of 1280 runs on release 4.14 [Overall] in the last week."]},"OpenBugs":[]}],"OverallRisk":{"Level":{"Name":"High","Level":100},"Reasons":["Maximum failed test risk:High"]},"OpenBugs":[]}`,
			riskAnalysisJSON:         `{"ProwJobName":"pull-ci-openshift-origin-master-e2e-aws-ovn-upgrade","ProwJobRunID":1684985307247677440,"Release":"Presubmits","CompareRelease":"4.14","Tests":[{"Name":"Cluster upgrade.[sig-apps] job-upgrade","Risk":{"Level":{"Name":"High","Level":100},"Reasons":["This test has passed 99.14% of 1280 runs on release 4.14 [Overall] in the last week."]},"OpenBugs":[]}],"OverallRisk":{"Level":{"Name":"High","Level":100},"Reasons":["Maximum failed test risk:High"]},"OpenBugs":[]}`,
			expectedSummaryTestCount: 1,
			expectedRiskLevel:        api.FailureRiskLevelHigh,
		},
		"MatchNone": {
			priorRiskAnalysisJSON:    `{"ProwJobName":"pull-ci-openshift-origin-master-e2e-aws-ovn-upgrade","ProwJobRunID":1684917114550358016,"Release":"Presubmits","CompareRelease":"4.14","Tests":[{"Name":"[sig-arch] Only known images used by tests","Risk":{"Level":{"Name":"High","Level":100},"Reasons":["This test has passed 99.04% of 3767 runs on release 4.14 [Overall] in the last week."]},"OpenBugs":[]}],"OverallRisk":{"Level":{"Name":"High","Level":100},"Reasons":["Maximum failed test risk:High"]},"OpenBugs":[]}`,
			riskAnalysisJSON:         `{"ProwJobName":"pull-ci-openshift-origin-master-e2e-aws-ovn-upgrade","ProwJobRunID":1684985307247677440,"Release":"Presubmits","CompareRelease":"4.14","Tests":[{"Name":"Cluster upgrade.[sig-apps] job-upgrade","Risk":{"Level":{"Name":"High","Level":100},"Reasons":["This test has passed 99.14% of 1280 runs on release 4.14 [Overall] in the last week."]},"OpenBugs":[]}],"OverallRisk":{"Level":{"Name":"High","Level":100},"Reasons":["Maximum failed test risk:High"]},"OpenBugs":[]}`,
			expectedSummaryTestCount: 0,
			expectedRiskLevel:        api.FailureRiskLevelNone,
		},
		"MatchAllHighsNoPrior": {
			riskAnalysisJSON:         `{"ProwJobName":"pull-ci-openshift-origin-master-e2e-aws-ovn-upgrade","ProwJobRunID":1684985307247677440,"Release":"Presubmits","CompareRelease":"4.14","Tests":[{"Name":"[sig-arch] Only known images used by tests","Risk":{"Level":{"Name":"High","Level":100},"Reasons":["This test has passed 99.04% of 3767 runs on release 4.14 [Overall] in the last week."]},"OpenBugs":[]},{"Name":"[sig-autoscaling] [Feature:HPA] Horizontal pod autoscaling (scale resource:CPU) CustomResourceDefinition Should scale with a CRD targetRef [Suite:openshift/conformance/parallel] [Suite:k8s]","Risk":{"Level":{"Name":"Medium","Level":50},"Reasons":["This test has passed 91.34% of 127 runs on release 4.14 [amd64 aws ha ovn] in the last week."]},"OpenBugs":[]},{"Name":"Cluster upgrade.[sig-apps] job-upgrade","Risk":{"Level":{"Name":"High","Level":100},"Reasons":["This test has passed 99.14% of 1280 runs on release 4.14 [Overall] in the last week."]},"OpenBugs":[]},{"Name":"[bz-kube-apiserver][invariant] alert/KubeAPIErrorBudgetBurn should not be at or above info","Risk":{"Level":{"Name":"High","Level":100},"Reasons":["This test has passed 99.20% of 3764 runs on release 4.14 [Overall] in the last week."]},"OpenBugs":[{"id":15394692,"key":"TRT-1167","created_at":"2023-08-10T02:59:03.979473-04:00","updated_at":"2023-09-14T11:02:57.748888-04:00","deleted_at":null,"status":"In Progress","last_change_time":"2023-09-13T08:35:17-04:00","summary":"Investigate Opportunity For Risk Analysis Tuning","affects_versions":[],"fix_versions":[],"components":[],"labels":[],"url":"https://issues.redhat.com/browse/TRT-1167"}]}],"OverallRisk":{"Level":{"Name":"High","Level":100},"Reasons":["Maximum failed test risk:High"]},"OpenBugs":[]}`,
			expectedSummaryTestCount: 3,
			expectedRiskLevel:        api.FailureRiskLevelHigh,
		},
		"MatchIncompletes": {
			priorRiskAnalysisJSON:    `{"ProwJobName":"pull-ci-openshift-origin-master-e2e-aws-ovn-serial","ProwJobRunID":1684917111224274944,"Release":"Presubmits","CompareRelease":"4.14","Tests":[],"OverallRisk":{"Level":{"Name":"IncompleteTests","Level":75},"Reasons":["Tests for this run (100) are below the historical average (709):IncompleteTests"]},"OpenBugs":[]}`,
			riskAnalysisJSON:         `{"ProwJobName":"pull-ci-openshift-origin-master-e2e-aws-ovn-serial","ProwJobRunID":1684985307130236928,"Release":"Presubmits","CompareRelease":"4.14","Tests":[],"OverallRisk":{"Level":{"Name":"IncompleteTests","Level":75},"Reasons":["Tests for this run (57) are below the historical average (709):IncompleteTests"]},"OpenBugs":[]}`,
			expectedRiskLevel:        api.FailureRiskLevelIncompleteTests,
			expectedSummaryTestCount: 0,
		},
		"NoMatchIncompletes": {
			priorRiskAnalysisJSON:    `{"ProwJobName":"pull-ci-openshift-origin-master-e2e-aws-ovn-serial","ProwJobRunID":1684917111224274944,"Release":"Presubmits","CompareRelease":"4.14","Tests":[],"OverallRisk":{"Level":{"Name":"MissingData","Level":75},"Reasons":["Tests for this run (100) are below the historical average (709):IncompleteTests"]},"OpenBugs":[]}`,
			riskAnalysisJSON:         `{"ProwJobName":"pull-ci-openshift-origin-master-e2e-aws-ovn-serial","ProwJobRunID":1684985307130236928,"Release":"Presubmits","CompareRelease":"4.14","Tests":[],"OverallRisk":{"Level":{"Name":"IncompleteTests","Level":75},"Reasons":["Tests for this run (57) are below the historical average (709):IncompleteTests"]},"OpenBugs":[]}`,
			expectedRiskLevel:        api.FailureRiskLevelNone,
			expectedSummaryTestCount: 0,
		},
		"NoMatchIncompletesNoPrior": {
			riskAnalysisJSON:         `{"ProwJobName":"pull-ci-openshift-origin-master-e2e-aws-ovn-serial","ProwJobRunID":1684985307130236928,"Release":"Presubmits","CompareRelease":"4.14","Tests":[],"OverallRisk":{"Level":{"Name":"IncompleteTests","Level":75},"Reasons":["Tests for this run (57) are below the historical average (709):IncompleteTests"]},"OpenBugs":[]}`,
			expectedRiskLevel:        api.FailureRiskLevelNone,
			expectedSummaryTestCount: 0,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {

			var priorRiskAnalysis, riskAnalysis api.ProwJobRunRiskAnalysis
			var priorRiskAnalysisPTR *api.ProwJobRunRiskAnalysis

			if len(tc.priorRiskAnalysisJSON) > 0 {
				err := json.Unmarshal([]byte(tc.priorRiskAnalysisJSON), &priorRiskAnalysis)
				assert.Nil(t, err, "Failed to unmarshal prior risk analysis: %v", err)
				priorRiskAnalysisPTR = &priorRiskAnalysis
			}
			err := json.Unmarshal([]byte(tc.riskAnalysisJSON), &riskAnalysis)
			assert.Nil(t, err, "Failed to unmarshal prior risk analysis: %v", err)

			summary := buildRiskSummary(&riskAnalysis, priorRiskAnalysisPTR)

			assert.NotNil(t, summary, "Nil summary")
			assert.Equal(t, tc.expectedSummaryTestCount, len(summary.Tests), "Invalid summary test count")
			assert.Equal(t, tc.expectedRiskLevel, summary.OverallRisk.Level, "Invalid summary risk level")

		})
	}
}

func TestAnalysisWorker(t *testing.T) {
	// initialize AnalysisWorker
	dbc := util.GetDbHandle(t)
	gcsBucket := util.GetGcsBucket(t)
	logrus.SetLevel(logrus.InfoLevel)

	preparedComments := make(chan PreparedComment, 5)
	defer close(preparedComments)
	pendingWork := make(chan models.PullRequestComment, 1)
	defer close(pendingWork)

	ntw := StandardNewTestsWorker(dbc)
	analysisWorker := AnalysisWorker{
		riskAnalysisLocator: gcs.GetDefaultRiskAnalysisSummaryFile(),
		dbc:                 dbc,
		gcsBucket:           gcsBucket,
		prCommentProspects:  pendingWork,
		preparedComments:    preparedComments,
		newTestsWorker:      ntw,
	}

	prPendingComment := models.PullRequestComment{Org: "openshift", Repo: "origin", PullNumber: 29512, SHA: "8849ed78d4c51e2add729a68a2cbf8551c6d60c9", ProwJobRoot: "pr-logs/pull/29512/"} // PR constructed for testing new-test analysis
	analysisWorker.determinePrComment(context.TODO(), prPendingComment)

	pc := <-preparedComments
	logrus.Infof("Pending comment: %+v", pc)
	assert.Contains(t, pc.comment, "New Test Risks for", "comment should report on risks")
	assert.Contains(t, pc.comment, "new test that failed 2 time(s)", "comment should report on risks")
	assert.Contains(t, pc.comment, "- *\"a passed test that has never been seen before\"* [Total:", "comment should summarize new tests")
}

// not a real test, this is just a way to run the analysis worker on specific PRs and see the result
func TestRunCommentAnalysis(t *testing.T) {
	preparedComments := make(chan PreparedComment, 5)
	defer close(preparedComments)
	pendingWork := make(chan models.PullRequestComment, 1)
	defer close(pendingWork)

	dbc := util.GetDbHandle(t)
	analysisWorker := AnalysisWorker{
		riskAnalysisLocator: gcs.GetDefaultRiskAnalysisSummaryFile(),
		dbc:                 dbc,
		gcsBucket:           util.GetGcsBucket(t),
		prCommentProspects:  pendingWork,
		preparedComments:    preparedComments,
		newTestsWorker:      StandardNewTestsWorker(dbc),
	}

	tests := map[string]struct {
		prCommentProspect models.PullRequestComment
		logLevel          logrus.Level
	}{
		"28075": {
			prCommentProspect: models.PullRequestComment{Org: "openshift", Repo: "origin", PullNumber: 28075, SHA: "79d237196d93eb92ed58c66497d8718259264226", ProwJobRoot: "pr-logs/pull/28075/"},
			logLevel:          logrus.DebugLevel,
		},
		"has RA comment already": {
			prCommentProspect: models.PullRequestComment{Org: "openshift", Repo: "origin", PullNumber: 29474, SHA: "58a8615189ebd164a1ce87ffe9b078965a9f4b14", ProwJobRoot: "pr-logs/pull/29474/"},
			logLevel:          logrus.InfoLevel,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			logrus.SetLevel(tc.logLevel)
			analysisWorker.determinePrComment(context.TODO(), tc.prCommentProspect)

			select {
			case pc := <-preparedComments:
				logrus.Infof("Pending comment: %+v", pc)
				print(pc.comment)
			case <-time.After(2 * time.Minute):
				logrus.Warn("Timed out waiting for pending comment (see debug logs)")
				// often a PR still has active jobs so no comment is coming
			}
		})
	}
}

func TestRASort(t *testing.T) {
	foo := []RiskAnalysisSummary{
		{Name: "foo"},
		{Name: "bar"},
		{Name: "baz"},
	}
	SortByJobNameRA(foo) // really a test of whether slices.SortFunc sorts a param in place
	assert.Equal(t, "bar", foo[0].Name)
}
