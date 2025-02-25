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

func (r *RegressionAllowances) Query(ctx context.Context, wg *sync.WaitGroup, allJobVariants crtype.JobVariants) error {
	return nil
}

// Transform iterates the base status looking for any with an accepted regression in the basis release, and if found
// swaps out the stats with the better pass rate data specified in the intentional regression allowance.
func (r *RegressionAllowances) Transform(baseStatus, sampleStatus map[string]crtype.TestStatus) (map[string]crtype.TestStatus, map[string]crtype.TestStatus, error) {
	for testKeyStr, baseStats := range baseStatus {
		testKey, err := utils.DeserializeTestKey(baseStats, testKeyStr)
		if err != nil {
			return nil, nil, err
		}

		newBaseStatus, newBaseRelease := r.matchBaseRegression(testKey, r.reqOptions.BaseRelease.Release, baseStats)
		if newBaseRelease != r.reqOptions.BaseRelease.Release {
			baseStatus[testKeyStr] = newBaseStatus
		}
	}

	return baseStatus, sampleStatus, nil
}

// matchBaseRegression returns a testStatus that reflects the allowances specified
// in an intentional regression that accepted a lower threshold but maintains the higher
// threshold when used as a basis.
// It will return the original testStatus if there is no intentional regression.
func (r *RegressionAllowances) matchBaseRegression(testID crtype.ReportTestIdentification, baseRelease string, baseStats crtype.TestStatus) (crtype.TestStatus, string) {
	var baseRegression *regressionallowances.IntentionalRegression

	// TODO: knowledge of variant cross compare here would be nice to eliminate
	if len(r.reqOptions.VariantOption.VariantCrossCompare) == 0 {
		// only really makes sense when not cross-comparing variants:
		// look for corresponding regressions we can account for in the analysis
		// only if we are ignoring fallback, otherwise we will let fallback determine the threshold
		baseRegression = r.regressionGetterFunc(baseRelease, testID.ColumnIdentification, testID.TestID)

		_, success, fail, flake := baseStats.GetTotalSuccessFailFlakeCounts()
		basePassRate := utils.CalculatePassRate(r.reqOptions, success, fail, flake)
		if baseRegression != nil && baseRegression.PreviousPassPercentage(r.reqOptions.AdvancedOption.FlakeAsFailure) > basePassRate {
			// override with  the basis regression previous values
			// testStats will reflect the expected threshold, not the computed values from the release with the allowed regression
			baseRegressionPreviousRelease, err := utils.PreviousRelease(r.reqOptions.BaseRelease.Release)
			if err != nil {
				log.WithError(err).Error("Failed to determine the previous release for baseRegression")
			} else {
				// create a clone since we might be updating a cached item though the same regression would likely apply each time...
				updatedStats := crtype.TestStatus{TestName: baseStats.TestName, TestSuite: baseStats.TestSuite, Capabilities: baseStats.Capabilities,
					Component: baseStats.Component, Variants: baseStats.Variants,
					FlakeCount:   baseRegression.PreviousFlakes,
					SuccessCount: baseRegression.PreviousSuccesses,
					TotalCount:   baseRegression.PreviousFailures + baseRegression.PreviousFlakes + baseRegression.PreviousSuccesses,
				}
				baseStats = updatedStats
				baseRelease = baseRegressionPreviousRelease
				log.Infof("BaseRegression - PreviousPassPercentage overrides baseStats.  Release: %s, Successes: %d, Flakes: %d, Total: %d",
					baseRelease, baseStats.SuccessCount, baseStats.FlakeCount, baseStats.TotalCount)
			}
		}
	}

	return baseStats, baseRelease
}
