package regressionallowances

import (
	"context"
	"fmt"
	"sync"

	"github.com/openshift/sippy/pkg/api/componentreadiness/middleware"
	"github.com/openshift/sippy/pkg/api/componentreadiness/utils"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/bq"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/testdetails"
	"github.com/openshift/sippy/pkg/regressionallowances"
	log "github.com/sirupsen/logrus"
)

var _ middleware.Middleware = &RegressionAllowances{}

func NewRegressionAllowancesMiddleware(reqOptions reqopts.RequestOptions) *RegressionAllowances {
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
	log        log.FieldLogger
	reqOptions reqopts.RequestOptions

	// regressionGetterFunc allows us to unit test without relying on real regression data
	regressionGetterFunc func(releaseString string, variant crtest.ColumnIdentification, testID string) *regressionallowances.IntentionalRegression
}

func (r *RegressionAllowances) Query(_ context.Context, _ *sync.WaitGroup, _ crtest.JobVariants,
	_, _ chan map[string]bq.TestStatus, _ chan error) {
	// unused
}

// PreAnalysis iterates the base status looking for any with an accepted regression in the basis release, and if found
// swaps out the stats with the better pass rate data specified in the intentional regression allowance.
// It also iterates the sample looking for intentional regressions and adjusts the analysis parameters accordingly.
func (r *RegressionAllowances) PreAnalysis(testKey crtest.Identification, testStats *testdetails.ReportTestStats) error {

	// for intentional regression in the base
	r.matchBaseRegression(testKey, r.reqOptions.BaseRelease.Name, testStats)

	if ir := r.regressionGetterFunc(testStats.SampleStats.Release, testKey.ColumnIdentification, testKey.TestID); ir != nil {
		// for intentional regression in the sample
		r.adjustAnalysisParameters(testStats, ir)
	}

	return nil
}

func (r *RegressionAllowances) PostAnalysis(testKey crtest.Identification, testStats *testdetails.ReportTestStats) error {
	return nil
}

// matchBaseRegression returns a testStatus that reflects the allowances specified
// in an intentional regression that accepted a lower threshold but maintains the higher
// threshold when used as a basis.
// It will return the original testStatus if there is no intentional regression.
func (r *RegressionAllowances) matchBaseRegression(testID crtest.Identification, baseRelease string, testStats *testdetails.ReportTestStats) {
	opts := r.reqOptions.AdvancedOption
	// Nothing to do for tests with no basis. (i.e. new tests)
	if testStats.BaseStats == nil {
		return
	}

	// with fallback enabled and a fallback release found, let that determine the threshold across bases without the munging done below.
	if opts.IncludeMultiReleaseAnalysis && reqopts.AnyAreBaseOverrides(r.reqOptions.TestIDOptions) {
		return
	}

	// nothing to do for cross variant compares
	if len(r.reqOptions.VariantOption.VariantCrossCompare) != 0 {
		return
	}

	// look for corresponding regressions we can account for in the analysis
	baseRegression := r.regressionGetterFunc(baseRelease, testID.ColumnIdentification, testID.TestID)
	if baseRegression == nil {
		return
	}
	r.log.Infof("found a base regression for %s", testID.TestName)

	baseStats := testStats.BaseStats
	overrideTestStats := crtest.NewTestStats(baseRegression.PreviousSuccesses, baseRegression.PreviousFailures, baseRegression.PreviousFlakes, opts.FlakeAsFailure)
	if overrideTestStats.SuccessRate > baseStats.PassRate(opts.FlakeAsFailure) {
		// override with  the basis regression previous values
		// testStats will reflect the expected threshold, not the computed values from the release with the allowed regression
		baseRegressionPreviousRelease, err := utils.PreviousRelease(r.reqOptions.BaseRelease.Name)
		if err != nil {
			r.log.WithError(err).Error("Failed to determine the previous release for base regression")
		} else if overrideTestStats.Total() > 0 { // only override if there is history to override with
			testStats.BaseStats.Release = baseRegressionPreviousRelease
			testStats.BaseStats.Stats = overrideTestStats
			r.log.Infof("BaseRegression - PreviousPassPercentage overrides baseStats.  Release: %s, Successes: %d, Flakes: %d",
				baseRegressionPreviousRelease, baseStats.SuccessCount, baseStats.FlakeCount)
		}
	}
}

func (r *RegressionAllowances) adjustAnalysisParameters(testStats *testdetails.ReportTestStats, ir *regressionallowances.IntentionalRegression) {
	// nothing to do for cross variant compares
	if len(r.reqOptions.VariantOption.VariantCrossCompare) != 0 {
		return
	}

	opts := r.reqOptions.AdvancedOption
	if testStats.BaseStats == nil || testStats.BaseStats.Total() == 0 {
		// for regressions on new tests, adjust the required pass rate
		requiredSuccessRate := ir.RegressedPassPercentage(opts.FlakeAsFailure) * 100
		if requiredSuccessRate > float64(opts.PassRateRequiredNewTests) {
			log.Warnf("%+v allows pass rate %.1f, higher than the normal required pass rate for new tests %d; ignoring",
				ir, requiredSuccessRate, opts.PassRateRequiredNewTests)
		} else {
			testStats.RequiredPassRateAdjustment = requiredSuccessRate - float64(opts.PassRateRequiredNewTests)
			testStats.Explanations = append(testStats.Explanations,
				fmt.Sprintf("Intentional regression applied to allow a %.1f%% pass rate: %q %s",
					requiredSuccessRate, ir.ReasonToAllowInsteadOfFix, ir.JiraBug))
		}
	} else {
		// for regressions on existing tests, adjust what Fisher's Exact will consider a pass
		basisPassPercentage := testStats.BaseStats.PassRate(opts.FlakeAsFailure)
		regressedPassPercentage := ir.RegressedPassPercentage(opts.FlakeAsFailure)
		if regressedPassPercentage < basisPassPercentage {
			// adjust pity to cover product-owner-approved leniency for pass percentage
			if regressedPassPercentage > basisPassPercentage {
				log.Warnf("%+v allows pass rate %.1f, higher than the actual basis pass rate %.1f; ignoring",
					ir, regressedPassPercentage, basisPassPercentage)
			} else {
				testStats.PityAdjustment = (basisPassPercentage - regressedPassPercentage) * 100
				testStats.Explanations = append(testStats.Explanations,
					fmt.Sprintf("Intentional regression applied to allow a %.1f%% pass rate: %q %s",
						regressedPassPercentage*100, ir.ReasonToAllowInsteadOfFix, ir.JiraBug))
			}
		}
	}
}

func (r *RegressionAllowances) QueryTestDetails(ctx context.Context, wg *sync.WaitGroup, errCh chan error, allJobVariants crtest.JobVariants) {
}

func (r *RegressionAllowances) PreTestDetailsAnalysis(testKey crtest.KeyWithVariants, status *bq.TestJobRunStatuses) error {
	return nil
}
