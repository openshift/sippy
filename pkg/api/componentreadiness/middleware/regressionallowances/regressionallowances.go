package regressionallowances

import (
	"context"
	"sync"

	"github.com/openshift/sippy/pkg/api/componentreadiness/middleware"
	"github.com/openshift/sippy/pkg/api/componentreadiness/utils"
	"github.com/openshift/sippy/pkg/regressionallowances"
	log "github.com/sirupsen/logrus"

	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
)

var _ middleware.Middleware = &RegressionAllowances{}

func NewRegressionAllowancesMiddleware(reqOptions crtype.RequestOptions) *RegressionAllowances {
	return &RegressionAllowances{
		log:                  log.WithField("middleware", "RegressionAllowances"),
		reqOptions:           reqOptions,
		regressionGetterFunc: regressionallowances.IntentionalRegressionFor,
	}
}

// RegressionAllowances middleware checks if there was an accepted intentional regression in the basis release, and if so
// overrides the test pass data to what was specified in the allowance. (typically the data from the prior release)
// This allows us to make sure the current branch compares against the good data from two releases ago, instead of the
// prior release which had a regression in the window prior to GA.
type RegressionAllowances struct {
	cachedFallbackTestStatuses *crtype.FallbackReleases
	log                        log.FieldLogger
	reqOptions                 crtype.RequestOptions

	// regressionGetterFunc allows us to unit test without relying on real regression data
	regressionGetterFunc func(releaseString string, variant crtype.ColumnIdentification, testID string) *regressionallowances.IntentionalRegression
}

func (r *RegressionAllowances) Query(_ context.Context, _ *sync.WaitGroup, _ crtype.JobVariants,
	_, _ chan map[string]crtype.TestStatus, _ chan error) {
	// unused
}

// PreAnalysis iterates the base status looking for any with an accepted regression in the basis release, and if found
// swaps out the stats with the better pass rate data specified in the intentional regression allowance.
func (r *RegressionAllowances) PreAnalysis(testKey crtype.ReportTestIdentification, testStats *crtype.ReportTestStats) error {

	r.matchBaseRegression(testKey, r.reqOptions.BaseRelease.Release, testStats)

	return nil
}

func (r *RegressionAllowances) PostAnalysis(testKey crtype.ReportTestIdentification, testStats *crtype.ReportTestStats) error {
	return nil
}

// matchBaseRegression returns a testStatus that reflects the allowances specified
// in an intentional regression that accepted a lower threshold but maintains the higher
// threshold when used as a basis.
// It will return the original testStatus if there is no intentional regression.
func (r *RegressionAllowances) matchBaseRegression(testID crtype.ReportTestIdentification, baseRelease string, testStats *crtype.ReportTestStats) {
	// Nothing to do for tests with no basis. (i.e. new tests)
	if testStats.BaseStats == nil {
		return
	}

	var baseRegression *regressionallowances.IntentionalRegression
	if len(r.reqOptions.VariantOption.VariantCrossCompare) == 0 {
		// only really makes sense when not cross-comparing variants:
		// look for corresponding regressions we can account for in the analysis
		// only if we are ignoring fallback, otherwise we will let fallback determine the threshold
		baseRegression = r.regressionGetterFunc(baseRelease, testID.ColumnIdentification, testID.TestID)

		baseStats := testStats.BaseStats

		success := baseStats.SuccessCount
		fail := baseStats.FailureCount
		flake := baseStats.FlakeCount
		basePassRate := utils.CalculatePassRate(success, fail, flake, r.reqOptions.AdvancedOption.FlakeAsFailure)
		if baseRegression != nil && baseRegression.PreviousPassPercentage(r.reqOptions.AdvancedOption.FlakeAsFailure) > basePassRate {
			// override with  the basis regression previous values
			// testStats will reflect the expected threshold, not the computed values from the release with the allowed regression
			baseRegressionPreviousRelease, err := utils.PreviousRelease(r.reqOptions.BaseRelease.Release)
			if err != nil {
				log.WithError(err).Error("Failed to determine the previous release for baseRegression")
			} else {
				testStats.BaseStats.Release = baseRegressionPreviousRelease
				testStats.BaseStats.TestDetailsTestStats = crtype.TestDetailsTestStats{
					SuccessCount: baseRegression.PreviousSuccesses,
					FailureCount: baseRegression.PreviousFailures,
					FlakeCount:   baseRegression.PreviousFlakes,
					SuccessRate: utils.CalculatePassRate(baseRegression.PreviousSuccesses, baseRegression.PreviousFailures,
						baseRegression.PreviousFlakes, r.reqOptions.AdvancedOption.FlakeAsFailure),
				}

				log.Infof("BaseRegression - PreviousPassPercentage overrides baseStats.  Release: %s, Successes: %d, Flakes: %d",
					baseRegressionPreviousRelease, baseStats.SuccessCount, baseStats.FlakeCount)
			}
		}
	}

}

func (r *RegressionAllowances) QueryTestDetails(ctx context.Context, wg *sync.WaitGroup, errCh chan error, allJobVariants crtype.JobVariants) {
}

func (r *RegressionAllowances) PreTestDetailsAnalysis(status *crtype.TestJobRunStatuses) error {
	return nil
}
