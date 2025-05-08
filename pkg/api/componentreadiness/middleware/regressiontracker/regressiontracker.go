package regressiontracker

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/openshift/sippy/pkg/api/componentreadiness/middleware"
	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/db/query"
	log "github.com/sirupsen/logrus"
)

const (
	// openRegressionConfidenceAdjustment is subtracted from the requested confidence for regressed tests that have
	// an open regression.
	openRegressionConfidenceAdjustment = 5
)

var _ middleware.Middleware = &RegressionTracker{}

func NewRegressionTrackerMiddleware(dbc *db.DB, reqOptions crtype.RequestOptions) *RegressionTracker {
	return &RegressionTracker{
		log:        log.WithField("middleware", "RegressionTracker"),
		reqOptions: reqOptions,
		dbc:        dbc,
	}
}

// RegressionTracker middleware loads all known regressions for this release from the db, and will
// inject details onto regressed test stats if they match known regressions.
// It also handles adjustments if those regressions are triaged to bugs.
type RegressionTracker struct {
	log             log.FieldLogger
	reqOptions      crtype.RequestOptions
	dbc             *db.DB
	openRegressions []*models.TestRegression
}

func (r *RegressionTracker) Query(ctx context.Context, wg *sync.WaitGroup, allJobVariants crtype.JobVariants, baseStatusCh, sampleStatusCh chan map[string]crtype.TestStatus, errCh chan error) {
	// Load all known regressions for this release:
	var err error
	r.openRegressions, err = query.ListRegressions(r.dbc, r.reqOptions.SampleRelease.Release)
	if err != nil {
		errCh <- err
		return
	}
	r.log.Infof("Found %d open regressions", len(r.openRegressions))
}

func (r *RegressionTracker) QueryTestDetails(ctx context.Context, wg *sync.WaitGroup, errCh chan error, allJobVariants crtype.JobVariants) {
}

func (r *RegressionTracker) PreAnalysis(testKey crtype.ReportTestIdentification, testStats *crtype.ReportTestStats) error {
	if len(r.openRegressions) > 0 {
		view := r.openRegressions[0].View // grab view from first regression, they were queried only for sample release
		or := FindOpenRegression(view, testKey.TestID, testKey.Variants, r.openRegressions)
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
		}
	}
	return nil
}

func (r *RegressionTracker) PostAnalysis(testKey crtype.ReportTestIdentification, testStats *crtype.ReportTestStats) error {
	if len(r.openRegressions) > 0 {
		view := r.openRegressions[0].View // grab view from first regression, they were queried only for sample release
		or := FindOpenRegression(view, testKey.TestID, testKey.Variants, r.openRegressions)
		if or == nil {
			return nil
		}

		if len(or.Triages) > 0 {

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
			case allTriagesResolved && testStats.LastFailure != nil && lastResolution.Before(*testStats.LastFailure):
				// claimed fixed but does not appear to be
				// aka liar liar pants on fire
				testStats.ReportStatus = crtype.FailedFixedRegression
				testStats.Explanations = append(testStats.Explanations, fmt.Sprintf(
					"Regression was triaged and believed fixed as of %s but failures have been observed as late as %s",
					lastResolution.Format(time.RFC3339), testStats.LastFailure.Format(time.RFC3339)))
			case allTriagesResolved:
				// claimed fixed, no failures since resolution date
				testStats.ReportStatus = crtype.FixedRegression
				testStats.Explanations = append(testStats.Explanations, fmt.Sprintf(
					"Regression was triaged and believed fixed as of %s",
					lastResolution.Format(time.RFC3339)))
			case testStats.ReportStatus == crtype.SignificantRegression:
				testStats.ReportStatus = crtype.SignificantTriagedRegression
				testStats.Explanations = append(testStats.Explanations,
					"Regression has been triaged to one or more bugs")
			case testStats.ReportStatus == crtype.ExtremeRegression:
				testStats.ReportStatus = crtype.ExtremeTriagedRegression
				testStats.Explanations = append(testStats.Explanations,
					"Regression has been triaged to one or more bugs")
			}
		}
	}
	return nil
}

// FindOpenRegression scans the list of open regressions for any that match the given test summary.
func FindOpenRegression(view string,
	testID string,
	variants map[string]string,
	regressions []*models.TestRegression) *models.TestRegression {

	for _, tr := range regressions {
		if tr.View != view {
			continue
		}

		// We compare test ID not name, as names can change.
		if tr.TestID != testID {
			continue
		}
		found := true
		for key, value := range variants {
			if value != findVariant(key, tr) {
				found = false
				break
			}
		}
		if !found {
			continue
		}
		// If we made it this far, this appears to be a match:
		return tr
	}
	return nil
}

func findVariant(variantName string, testReg *models.TestRegression) string {
	for _, v := range testReg.Variants {
		keyVal := strings.Split(v, ":")
		if keyVal[0] == variantName {
			return keyVal[1]
		}
	}
	return ""
}

func (r *RegressionTracker) PreTestDetailsAnalysis(status *crtype.JobRunTestReportStatus) error {
	return nil
}
