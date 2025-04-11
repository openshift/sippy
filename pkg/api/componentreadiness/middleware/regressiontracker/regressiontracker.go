package regressiontracker

import (
	"context"
	"strings"
	"sync"

	"github.com/openshift/sippy/pkg/api/componentreadiness/middleware"
	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	log "github.com/sirupsen/logrus"
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
type RegressionTracker struct {
	log             log.FieldLogger
	reqOptions      crtype.RequestOptions
	dbc             *db.DB
	openRegressions []*models.TestRegression
}

func (r *RegressionTracker) Query(ctx context.Context, wg *sync.WaitGroup, allJobVariants crtype.JobVariants, baseStatusCh, sampleStatusCh chan map[string]crtype.TestStatus, errCh chan error) {
	// Load all known regressions for this release:
	r.openRegressions = make([]*models.TestRegression, 0)
	q := r.dbc.DB.Table("test_regressions").
		Where("release = ?", r.reqOptions.SampleRelease.Release).
		Where("closed IS NULL")
	res := q.Scan(&r.openRegressions)
	if res.Error != nil {
		errCh <- res.Error
		return
	}
	r.log.Infof("Found %d open regressions", len(r.openRegressions))
	return
}

func (r *RegressionTracker) QueryTestDetails(ctx context.Context, wg *sync.WaitGroup, errCh chan error, allJobVariants crtype.JobVariants) {
	return
}

func (r *RegressionTracker) Transform(status *crtype.ReportTestStatus) error {
	return nil
}

func (r *RegressionTracker) TransformTestDetails(status *crtype.JobRunTestReportStatus) error {
	return nil
}

func (r *RegressionTracker) TestDetailsAnalyze(report *crtype.ReportTestDetails) error {
	return nil
}

func (r *RegressionTracker) Analyze(testID string, variants map[string]string, report *crtype.ReportTestStats) error {
	if len(r.openRegressions) > 0 {
		view := r.openRegressions[0].View // grab view from first regression, they were queried only for sample release
		or := FindOpenRegression(view, testID, variants, r.openRegressions)
		if or != nil {
			report.Regression = or
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
