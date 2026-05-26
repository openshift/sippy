package alltestspassrate

import (
	"context"
	"sync"

	"github.com/openshift/sippy/pkg/api/componentreadiness/middleware/analysis"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crstatus"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/testdetails"
)

type AllTestsPassRate struct {
	reqOptions reqopts.RequestOptions
}

func NewAllTestsPassRateMiddleware(reqOptions reqopts.RequestOptions) *AllTestsPassRate {
	return &AllTestsPassRate{reqOptions: reqOptions}
}

func (a *AllTestsPassRate) Query(_ context.Context, _ *sync.WaitGroup, _ crtest.JobVariants,
	_, _ chan map[string]crstatus.TestStatus, _ chan error) {
}

func (a *AllTestsPassRate) QueryTestDetails(_ context.Context, _ *sync.WaitGroup, _ chan error, _ crtest.JobVariants) {
}

func (a *AllTestsPassRate) PreAnalysis(_ crtest.Identification, _ *testdetails.TestComparison) error {
	return nil
}

func (a *AllTestsPassRate) PostAnalysis(_ crtest.Identification, _ *testdetails.TestComparison) error {
	return nil
}

func (a *AllTestsPassRate) PreTestDetailsAnalysis(_ crtest.KeyWithVariants, _ *crstatus.TestJobRunStatuses) error {
	return nil
}

// Analyze claims all tests when PassRateRequiredAllTests is configured, applying a raw
// pass rate comparison instead of Fisher's exact test.
func (a *AllTestsPassRate) Analyze(_ crtest.Identification, testStats *testdetails.TestComparison) (bool, error) {
	opts := a.reqOptions.AdvancedOption
	if opts.PassRateRequiredAllTests == 0 {
		return false, nil
	}

	analysis.BuildPassRateTestStats(testStats, float64(opts.PassRateRequiredAllTests), opts)
	return true, nil
}
