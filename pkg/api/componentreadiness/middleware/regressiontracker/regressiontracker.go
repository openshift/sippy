package regressiontracker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/openshift/sippy/pkg/api/componentreadiness/middleware"
	"github.com/openshift/sippy/pkg/api/componentreadiness/utils"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crstatus"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/testdetails"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/db/query"
	sippyutil "github.com/openshift/sippy/pkg/util"
	log "github.com/sirupsen/logrus"
)

const (
	// openRegressionConfidenceAdjustment is subtracted from the requested confidence for regressed tests that have
	// an open regression.
	openRegressionConfidenceAdjustment = 5
	// openRegressionPityAdjustment is used to adjust pity for regressed tests that have an open regression. The goal is
	// to adjust this to a smaller value so that, if a rate improvement is smaller than the openRegressionPityAdjustment,
	// we still consider it regressed.
	openRegressionPityAdjustment = -2
	// keyTestMinFailuresForFailedFix is the minimum number of sample failures
	// required before marking a key test as FailedFixedRegression ("pants on
	// fire"). Key tests like "install should succeed" are highly sensitive and
	// a single failure can be noise.
	keyTestMinFailuresForFailedFix = 2
)

var _ middleware.Middleware = &RegressionTracker{}

// failureCounterFunc counts post-resolution failures for a regression.
// It is a field on RegressionTracker so tests can inject a stub without
// needing a real database.
type failureCounterFunc func(regressionID uint, after time.Time) (int, error)

func NewRegressionTrackerMiddleware(dbc *db.DB, reqOptions reqopts.RequestOptions) *RegressionTracker {
	return &RegressionTracker{
		log:        log.WithField("middleware", "RegressionTracker"),
		reqOptions: reqOptions,
		dbc:        dbc,
		failureCounter: func(regressionID uint, after time.Time) (int, error) {
			return query.CountRegressionFailuresAfter(dbc, regressionID, after)
		},
	}
}

// RegressionTracker middleware loads all known regressions for this release from the db, and will
// inject details onto regressed test stats if they match known regressions.
// It also handles adjustments if those regressions are triaged to bugs.
type RegressionTracker struct {
	log                 log.FieldLogger
	reqOptions          reqopts.RequestOptions
	dbc                 *db.DB
	failureCounter      failureCounterFunc
	openRegressions     []*models.TestRegression
	regressionsByTestID map[string][]*models.TestRegression
	// hasLoadedRegressions will be true once we've loaded regression data
	hasLoadedRegressions bool
}

func (r *RegressionTracker) Query(ctx context.Context, wg *sync.WaitGroup, baseStatusCh, sampleStatusCh chan map[string]crstatus.TestStatus, errCh chan error) {
	err := r.ensureRegressionsLoaded()
	if err != nil {
		utils.EnqueueAsync(wg, errCh, err)
	}
}

func (r *RegressionTracker) QueryTestDetails(ctx context.Context, wg *sync.WaitGroup, errCh chan error) {
	err := r.ensureRegressionsLoaded()
	if err != nil {
		utils.EnqueueAsync(wg, errCh, err)
	}
}

func (r *RegressionTracker) ensureRegressionsLoaded() error {
	if r.hasLoadedRegressions {
		return nil
	}

	// Load all known regressions for this release:
	var err error
	r.openRegressions, err = query.ListOpenRegressions(r.dbc, r.reqOptions.SampleRelease.Name)
	if err != nil {
		return err
	}
	r.regressionsByTestID = BuildRegressionIndex(r.openRegressions)
	r.log.Infof("Found %d open regressions", len(r.openRegressions))
	r.hasLoadedRegressions = true
	return nil
}

// BuildRegressionIndex groups regressions by TestID for O(1) lookup.
func BuildRegressionIndex(regressions []*models.TestRegression) map[string][]*models.TestRegression {
	idx := make(map[string][]*models.TestRegression, len(regressions))
	for _, reg := range regressions {
		idx[reg.TestID] = append(idx[reg.TestID], reg)
	}
	return idx
}

func (r *RegressionTracker) PreAnalysis(testKey crtest.Identification, testStats *testdetails.TestComparison) error {
	if len(r.openRegressions) > 0 {
		or := FindOpenRegression(r.reqOptions.SampleRelease.Name, testKey.TestID, len(r.reqOptions.VariantOption.VariantCrossCompare) > 0, testKey.Variants, r.regressionsByTestID)
		if or != nil {
			testStats.Regression = or

			// Adjust the required certainty of a regression before we include it in the report as a
			// regressed test. This is to introduce some hysteresis into the process so once a regression creeps over the 95%
			// confidence we typically use, dropping to 94.9% should not make the cell immediately green.
			//
			// Instead, once you cross the confidence threshold and a regression begins tracking in the openRegressions list,
			// we'll require less confidence for that test until the regression is closed. (-5%) Once the certainty drops below that
			// modified confidence, the regression will be closed and the -5% adjuster is gone.
			//
			// ie. if the request was for 95% confidence, but we see that a test has an open regression (meaning at some point recently
			// we were over 95% certain of a regression), we're going to only require 90% certainty to mark that test red.
			testStats.RequiredConfidence = r.reqOptions.AdvancedOption.Confidence - openRegressionConfidenceAdjustment
			testStats.PityAdjustment = openRegressionPityAdjustment
		}
	}
	return nil
}

// PostAnalysis adjusts triages and status code (and thus icons) based on the triaged state of open regressions.
func (r *RegressionTracker) PostAnalysis(testKey crtest.Identification, testStats *testdetails.TestComparison) error {
	if testStats.ReportStatus > crtest.SignificantTriagedRegression {
		// no need to adjust status for triage if this is no longer a regression
		return nil
	}
	err := r.ensureRegressionsLoaded()
	if err != nil {
		return err
	}
	if len(r.openRegressions) > 0 {
		or := FindOpenRegression(r.reqOptions.SampleRelease.Name, testKey.TestID, len(r.reqOptions.VariantOption.VariantCrossCompare) > 0, testKey.Variants, r.regressionsByTestID)
		r.log.Debugf("checking regressions for %+v", testKey)
		if or == nil {
			return nil
		}

		// rare circumstances result in the regression not being set during pre-analysis, but being present now
		// This also could happen when report is from cache and PreAnalysis was never run.
		if testStats.Regression == nil {
			testStats.Regression = or
		}
		if len(or.Triages) > 0 {
			// triages need to be included, in case they are not in the cache, in order to show the list on the report
			testStats.Regression.Triages = or.Triages

			allTriagesResolved := true
			var lastResolution time.Time
			for _, t := range or.Triages {
				if !t.Resolved.Valid {
					allTriagesResolved = false
				} else if t.Resolved.Time.After(lastResolution) {
					lastResolution = t.Resolved.Time
				}
			}

			switch {
			case allTriagesResolved && testStats.LastFailure != nil && lastResolution.Before(*testStats.LastFailure) &&
				sippyutil.StrSliceContains(r.reqOptions.AdvancedOption.KeyTestNames, testKey.TestName):
				failuresAfterFix, err := r.failureCounter(or.ID, lastResolution)
				if err != nil {
					r.log.WithError(err).WithField("regression_id", or.ID).Warn("failed to count post-resolution failures for key test, falling back to FailedFixedRegression")
					testStats.ReportStatus = crtest.FailedFixedRegression
					testStats.Explanations = append(testStats.Explanations, fmt.Sprintf(
						"Regression is triaged, and believed fixed as of %s, but failures have been observed as recently as %s.",
						lastResolution.Format(time.RFC3339), testStats.LastFailure.Format(time.RFC3339)))
					return nil
				}
				if failuresAfterFix < keyTestMinFailuresForFailedFix {
					testStats.ReportStatus = crtest.FixedRegression
					testStats.Explanations = append(testStats.Explanations, fmt.Sprintf(
						"Regression is triaged and believed fixed as of %s. Failures since resolution (%d) are below the key test threshold (%d) for a failed fix.",
						lastResolution.Format(time.RFC3339), failuresAfterFix, keyTestMinFailuresForFailedFix))
				} else {
					testStats.ReportStatus = crtest.FailedFixedRegression
					testStats.Explanations = append(testStats.Explanations, fmt.Sprintf(
						"Regression is triaged, and believed fixed as of %s, but failures have been observed as recently as %s.",
						lastResolution.Format(time.RFC3339), testStats.LastFailure.Format(time.RFC3339)))
				}
			case allTriagesResolved && testStats.LastFailure != nil && lastResolution.Before(*testStats.LastFailure):
				testStats.ReportStatus = crtest.FailedFixedRegression
				testStats.Explanations = append(testStats.Explanations, fmt.Sprintf(
					"Regression is triaged, and believed fixed as of %s, but failures have been observed as recently as %s.",
					lastResolution.Format(time.RFC3339), testStats.LastFailure.Format(time.RFC3339)))
			case allTriagesResolved:
				// claimed fixed, no failures since resolution date
				testStats.ReportStatus = crtest.FixedRegression
				testStats.Explanations = append(testStats.Explanations, fmt.Sprintf(
					"Regression is triaged and believed fixed as of %s.",
					lastResolution.Format(time.RFC3339)))
			case testStats.ReportStatus == crtest.SignificantRegression:
				testStats.ReportStatus = crtest.SignificantTriagedRegression
				testStats.Explanations = append(testStats.Explanations,
					"Regression has been triaged to one or more bugs.")
			case testStats.ReportStatus == crtest.ExtremeRegression:
				testStats.ReportStatus = crtest.ExtremeTriagedRegression
				testStats.Explanations = append(testStats.Explanations,
					"Extreme regression has been triaged to one or more bugs.")
			}
		}
	}
	return nil
}

// FindOpenRegression looks up open regressions matching the given test by
// testID, then checks release, crossCompare, and variant subset matching.
// The index is keyed by testID for O(1) lookup instead of a linear scan.
func FindOpenRegression(sampleRelease, testID string,
	crossCompare bool,
	variants map[string]string,
	regressionsByTestID map[string][]*models.TestRegression) *models.TestRegression {

	candidates := regressionsByTestID[testID]
	for _, tr := range candidates {
		if sampleRelease != tr.Release {
			continue
		}
		if tr.CrossCompare != crossCompare {
			continue
		}
		if !variantsMatch(tr.Variants, variants) {
			continue
		}
		return tr
	}
	return nil
}

// variantsMatch checks if ALL regression variants are present in the input
// variants (subset matching). This allows the input to have additional variants
// beyond what the regression has, supporting db_column_groupby modifications.
func variantsMatch(regressionVariants []string, inputVariants map[string]string) bool {
	for _, variant := range regressionVariants {
		key, value := crtest.VariantStringToKeyValue(variant)
		if key == "" || inputVariants[key] != value {
			return false
		}
	}
	return true
}

func (r *RegressionTracker) PreTestDetailsAnalysis(testKey crtest.KeyWithVariants, status *crstatus.TestJobRunStatuses) error {
	return nil
}
