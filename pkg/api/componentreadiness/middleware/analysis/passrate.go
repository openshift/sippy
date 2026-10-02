package analysis

import (
	"fmt"

	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/testdetails"
)

const ExplanationNoRegression = "No significant regressions found"

// BuildPassRateTestStats evaluates a test's sample pass rate against a required success rate
// and sets the report status accordingly.
func BuildPassRateTestStats(testStats *testdetails.TestComparison, requiredSuccessRate float64, opts reqopts.Advanced) {
	effectiveSuccessReq := requiredSuccessRate + testStats.RequiredPassRateAdjustment

	// Assume 2x our allowed failure rate = an extreme regression.
	severeRegressionSuccessRate := effectiveSuccessReq - (100 - requiredSuccessRate)

	sufficientRuns := testStats.SampleStats.Total() >= 6

	successRate := testStats.SampleStats.PassRate(opts.FlakeAsFailure)
	if sufficientRuns && successRate*100 < effectiveSuccessReq && testStats.SampleStats.FailureCount >= opts.MinimumFailure {
		rStatus := crtest.SignificantRegression
		if successRate*100 < severeRegressionSuccessRate {
			rStatus = crtest.ExtremeRegression
		}
		testStats.ReportStatus = rStatus
		testStats.Explanations = append(testStats.Explanations,
			fmt.Sprintf("Test has a %.2f%% pass rate, but %.2f%% is required.", successRate*100, effectiveSuccessReq))
		testStats.Comparison = crtest.PassRate
		testStats.SampleStats.SuccessRate = successRate
		return
	}

	testStats.ReportStatus = crtest.NotSignificant
	testStats.Explanations = append(testStats.Explanations, ExplanationNoRegression)
}
