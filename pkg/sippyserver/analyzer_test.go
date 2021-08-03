package sippyserver_test

import (
	"testing"
	"time"

	testgridv1 "github.com/openshift/sippy/pkg/apis/testgrid/v1"
	"github.com/openshift/sippy/pkg/buganalysis"
	"github.com/openshift/sippy/pkg/sippyserver"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridconversion"
	"github.com/openshift/sippy/pkg/testgridanalysis/testidentification"
	"github.com/openshift/sippy/pkg/util/sets"
)

func TestAnalyzer(t *testing.T) {
	testGridData, _ := getTestGridData()

	testCases := []struct {
		name               string
		testGridJobDetails []testgridv1.JobDetails
		expectedWarnings   []string
	}{
		{
			name:               "missing overall test warning",
			testGridJobDetails: testGridData,
			expectedWarnings: []string{
				"could not prepare test report from data: missing Overall test in job periodic-ci-openshift-release-master-ci-4.9-e2e-aws",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			config := sippyserver.TestReportGeneratorConfig{
				TestGridLoadingConfig:       getLoadingConfig(testCase.testGridJobDetails, time.Now()),
				RawJobResultsAnalysisConfig: getAnalysisConfig(),
				DisplayDataConfig:           getDisplayConfig(),
			}

			report := config.PrepareTestReport(
				getDashboardCoordinates()[0],
				testgridconversion.NewOpenshiftSyntheticTestManager(),
				testidentification.NewOpenshiftVariantManager(),
				buganalysis.NewNoOpBugCache(),
			)

			expectedWarnings := sets.NewString(testCase.expectedWarnings...)
			actualWarnings := sets.NewString(report.AnalysisWarnings...)
			if !expectedWarnings.Equal(actualWarnings) {
				t.Errorf("expected warnings: %v, got: %v", expectedWarnings.List(), actualWarnings.List())
			}
		})
	}
}
