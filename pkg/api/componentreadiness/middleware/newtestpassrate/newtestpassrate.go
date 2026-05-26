package newtestpassrate

import (
	"context"
	"sync"

	"github.com/openshift/sippy/pkg/api/componentreadiness/middleware/analysis"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crstatus"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/testdetails"
)

type NewTestPassRate struct {
	reqOptions reqopts.RequestOptions
}

func NewNewTestPassRateMiddleware(reqOptions reqopts.RequestOptions) *NewTestPassRate {
	return &NewTestPassRate{reqOptions: reqOptions}
}

func (n *NewTestPassRate) Query(_ context.Context, _ *sync.WaitGroup, _ crtest.JobVariants,
	_, _ chan map[string]crstatus.TestStatus, _ chan error) {
}

func (n *NewTestPassRate) QueryTestDetails(_ context.Context, _ *sync.WaitGroup, _ chan error, _ crtest.JobVariants) {
}

func (n *NewTestPassRate) PreAnalysis(_ crtest.Identification, _ *testdetails.TestComparison) error {
	return nil
}

func (n *NewTestPassRate) PostAnalysis(_ crtest.Identification, _ *testdetails.TestComparison) error {
	return nil
}

func (n *NewTestPassRate) PreTestDetailsAnalysis(_ crtest.KeyWithVariants, _ *crstatus.TestJobRunStatuses) error {
	return nil
}

// Analyze claims tests that have no base stats (new tests) when PassRateRequiredNewTests is configured.
func (n *NewTestPassRate) Analyze(_ crtest.Identification, testStats *testdetails.TestComparison) (bool, error) {
	opts := n.reqOptions.AdvancedOption
	if opts.PassRateRequiredNewTests == 0 {
		return false, nil
	}
	if testStats.BaseStats != nil && testStats.BaseStats.Total() > 0 {
		return false, nil
	}

	analysis.BuildPassRateTestStats(testStats, float64(opts.PassRateRequiredNewTests), opts)
	if testStats.ReportStatus == crtest.NotSignificant && opts.PassRateRequiredAllTests == 0 {
		testStats.ReportStatus = crtest.MissingBasis
	}
	return true, nil
}
