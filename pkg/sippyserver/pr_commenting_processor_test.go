package sippyserver

import (
	"encoding/json"
	"testing"

	"github.com/openshift/sippy/pkg/apis/api"
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
