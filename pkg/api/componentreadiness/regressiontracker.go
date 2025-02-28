package componentreadiness

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"
	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/cache"
	configv1 "github.com/openshift/sippy/pkg/apis/config/v1"
	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	sippybigquery "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/db"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
)

const (
	testRegressionsTable = "test_regressions"

	// regressionHysteresisDays is the number of days a closed regression can be closed but still
	// reused if the test appears regressed again in that timeframe. It allows us to reuse the regression
	// record, including its start date, if the regression is coming and going.
	regressionHysteresisDays = 5
)

// RegressionStore is an underlying interface for where we store/load data on open test regressions.
type RegressionStore interface {
	// ListCurrentRegressionsForRelease returns *all* regressions for the given release. We operate on the assumption that
	// only one view is allowed to have regression tracking enabled (i.e. 4.18-main) per release, which is validated
	// when the views file is loaded. This is because we want to display regression tracking data on any report that shows
	// a regressed test, so people using custom reporting can see what is regressed in main as well.
	ListCurrentRegressionsForRelease(ctx context.Context, release string) ([]*crtype.TestRegression, error)
	OpenRegression(ctx context.Context, view crtype.View, newRegressedTest crtype.ReportTestSummary) (*crtype.TestRegression, error)
	ReOpenRegression(ctx context.Context, reg *crtype.TestRegression) error
	CloseRegression(ctx context.Context, reg *crtype.TestRegression, closedAt time.Time) error
}

// TODO: temporary, just being used for migration code on initial rollout, can be deleted as soon as postgres imports
// the current regressions.
//
// ListCurrentRegressionsForRelease returns *all* regressions for all releases. We operate on the assumption that
// only one view is allowed to have regression tracking enabled (i.e. 4.18-main) per release, which is validated
// when the views file is loaded. This is because we want to display regression tracking data on any report that shows
// a regressed test, so users using custom reporting can see what is regressed in main as well.
func ListCurrentRegressions(ctx context.Context, client *sippybigquery.Client) ([]*crtype.TestRegressionBigQuery, error) {
	// Use max snapshot date to get the most recently appended view of the regressions.
	queryString := fmt.Sprintf("SELECT * FROM %s.%s WHERE snapshot = (SELECT MAX(snapshot) FROM %s.%s)",
		client.Dataset, testRegressionsTable, client.Dataset, testRegressionsTable)

	sampleQuery := client.BQ.Query(queryString)

	regressions := make([]*crtype.TestRegressionBigQuery, 0)
	log.Infof("Fetching current test regressions with: %s", sampleQuery.Q)

	it, err := sampleQuery.Read(ctx)
	if err != nil {
		log.WithError(err).Error("error querying triaged incidents from bigquery")
		return regressions, err
	}

	for {
		var regression crtype.TestRegressionBigQuery
		err := it.Next(&regression)
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.WithError(err).Error("error parsing triaged incident from bigquery")
			return nil, errors.Wrap(err, "error parsing triaged incident from bigquery")
		}
		regressions = append(regressions, &regression)
	}
	return regressions, nil
}

type PostgresRegressionStore struct {
	dbc *db.DB
}

func NewPostgresRegressionStore(dbc *db.DB) RegressionStore {
	return &PostgresRegressionStore{dbc: dbc}
}

func (prs *PostgresRegressionStore) ListCurrentRegressionsForRelease(ctx context.Context, release string) ([]*crtype.TestRegression, error) {
	// List open regressions (no closed date), or those that closed within the last few days. This is to prevent flapping
	// and return more accurate opened dates when a test is falling in / out of the report.
	regressions := make([]*crtype.TestRegression, 0)
	q := prs.dbc.DB.Table("test_regressions").
		Where("release = ?", release).
		Where("closed IS NULL OR closed > ?", time.Now().Add(-regressionHysteresisDays*24*time.Hour))
	res := q.Scan(&regressions)
	return regressions, res.Error

}
func (prs *PostgresRegressionStore) OpenRegression(ctx context.Context, view crtype.View,
	newRegressedTest crtype.ReportTestSummary) (*crtype.TestRegression, error) {

	variants := []string{}
	for k, v := range newRegressedTest.Variants {
		variants = append(variants, fmt.Sprintf("%s:%s", k, v))
	}
	log.Infof("variants: %s", strings.Join(variants, ","))

	id := uuid.New()
	newRegression := &crtype.TestRegression{
		View:         view.Name,
		Release:      view.SampleRelease.Release,
		TestID:       newRegressedTest.TestID,
		TestName:     newRegressedTest.TestName,
		RegressionID: id.String(),
		Opened:       time.Now(),
		Variants:     variants,
	}
	res := prs.dbc.DB.Create(newRegression)
	if res.Error != nil {
		return &crtype.TestRegression{}, res.Error
	}
	log.Infof("opened a new regression: %v", newRegression)
	return newRegression, nil

}

func (prs *PostgresRegressionStore) ReOpenRegression(ctx context.Context, reg *crtype.TestRegression) error {
	reg.Closed = sql.NullTime{Valid: false}
	res := prs.dbc.DB.Save(&reg)
	return res.Error
}

func (prs *PostgresRegressionStore) CloseRegression(ctx context.Context, reg *crtype.TestRegression, closedAt time.Time) error {
	reg.Closed = sql.NullTime{Valid: true, Time: closedAt}
	res := prs.dbc.DB.Save(&reg)
	return res.Error
}

func NewRegressionTracker(
	bigqueryClient *sippybigquery.Client,
	cacheOptions cache.RequestOptions,
	releases []v1.Release,
	backend RegressionStore,
	views []crtype.View,
	overrides []configv1.VariantJunitTableOverride,
	dryRun bool) *RegressionTracker {

	return &RegressionTracker{
		bigqueryClient:             bigqueryClient,
		cacheOpts:                  cacheOptions,
		releases:                   releases,
		backend:                    backend,
		views:                      views,
		variantJunitTableOverrides: overrides,
		dryRun:                     dryRun,
		logger:                     log.WithField("daemon", "regression-tracker"),
	}
}

// RegressionTracker is the primary object for managing regression tracking logic.
type RegressionTracker struct {
	backend                    RegressionStore
	bigqueryClient             *sippybigquery.Client
	cacheOpts                  cache.RequestOptions
	releases                   []v1.Release
	dryRun                     bool
	views                      []crtype.View
	logger                     log.FieldLogger
	variantJunitTableOverrides []configv1.VariantJunitTableOverride
}

// Run iterates all views with regression tracking enabled and syncs the results of it's
// component report to the regression tables in bigquery.
func (rt *RegressionTracker) Run(ctx context.Context) error {
	// Run the existing logic
	var err error
	for _, view := range rt.views {
		if view.RegressionTracking.Enabled {
			err = rt.SyncRegressionsForView(ctx, view)
			if err != nil {
				log.WithError(err).WithField("view", view.Name).Error("error refreshing regressions for view")
				// keep processing other views
			}
		}
	}
	return err // return last error

}

func (rt *RegressionTracker) SyncRegressionsForView(ctx context.Context, view crtype.View) error {
	rLog := rt.logger.WithField("view", view.Name)

	baseRelease, err := GetViewReleaseOptions(
		rt.releases, "basis", view.BaseRelease, rt.cacheOpts.CRTimeRoundingFactor)
	if err != nil {
		return err
	}

	sampleRelease, err := GetViewReleaseOptions(
		rt.releases, "sample", view.SampleRelease, rt.cacheOpts.CRTimeRoundingFactor)
	if err != nil {
		return err
	}

	variantOption := view.VariantOptions
	advancedOption := view.AdvancedOptions

	// Get component readiness report
	reportOpts := crtype.RequestOptions{
		BaseRelease:    baseRelease,
		SampleRelease:  sampleRelease,
		TestIDOption:   crtype.RequestTestIdentificationOptions{},
		VariantOption:  variantOption,
		AdvancedOption: advancedOption,
		CacheOption:    rt.cacheOpts,
	}

	// Passing empty gcs bucket and prow URL, they are not needed outside test details reports
	report, errs := GetComponentReportFromBigQuery(
		ctx, rt.bigqueryClient, "", "", reportOpts, rt.variantJunitTableOverrides)
	if len(errs) > 0 {
		var strErrors []string
		for _, err := range errs {
			strErrors = append(strErrors, err.Error())
		}
		return fmt.Errorf("component report generation encountered errors: " + strings.Join(strErrors, "; "))
	}

	return rt.SyncRegressionsForReport(ctx, view, rLog, &report)
}

func (rt *RegressionTracker) SyncRegressionsForReport(ctx context.Context, view crtype.View, rLog *log.Entry, report *crtype.ComponentReport) error {
	// TODO: could move to one query for all regressions across all views in the parent run function
	regressions, err := rt.backend.ListCurrentRegressionsForRelease(ctx, view.SampleRelease.Release)
	if err != nil {
		return err
	}
	rLog.Infof("loaded %d regressions from db", len(regressions))

	// All regressions, both triaged and not:
	allRegressedTests := []crtype.ReportTestSummary{}
	for _, row := range report.Rows {
		for _, col := range row.Columns {
			allRegressedTests = append(allRegressedTests, col.RegressedTests...)
			// Once triaged, regressions move to this list, we want to still consider them an open regression until
			// the report says they're cleared and they disappear fully. Triaged does not imply fixed or no longer
			// a regression.
			for _, triaged := range col.TriagedIncidents {
				allRegressedTests = append(allRegressedTests, triaged.ReportTestSummary)
			}
		}
	}

	matchedOpenRegressions := []*crtype.TestRegression{} // all the matches we found, used to determine what had no match
	rLog.Infof("syncing %d open regressions", len(allRegressedTests))
	for _, regTest := range allRegressedTests {
		if openReg := FindOpenRegression(view.Name, regTest.TestID, regTest.Variants, regressions); openReg != nil {
			if openReg.Closed.Valid {
				// if the regression returned has a closed date, we found a recently closed
				// regression for this test. We'll re-use it to limit churn as sometimes tests may drop
				// in / out of the report depending on the data available in the sample/basis.
				rLog.Infof("re-opening existing regression: %v", openReg)
				if !rt.dryRun {
					err := rt.backend.ReOpenRegression(ctx, openReg)
					if err != nil {
						rLog.WithError(err).Errorf("error re-opening regression: %v", openReg)
						return errors.Wrapf(err, "error re-opening regression: %v", openReg)
					}
				}
			} else {
				rLog.WithFields(log.Fields{
					"test": regTest.TestName,
				}).Infof("reusing already opened regression: %v", openReg)

			}
			matchedOpenRegressions = append(matchedOpenRegressions, openReg)
		} else {
			rLog.Infof("opening new regression: %v", regTest)
			if !rt.dryRun {
				// Open a new regression:
				newReg, err := rt.backend.OpenRegression(ctx, view, regTest)
				if err != nil {
					rLog.WithError(err).Errorf("error opening new regression for: %v", regTest)
					return errors.Wrapf(err, "error opening new regression: %v", regTest)
				}
				rLog.Infof("new regression opened with id: %s", newReg.RegressionID)
			}
		}
	}

	// Now we want to close any open regressions that are not appearing in the latest report:
	now := time.Now()
	for _, regression := range regressions {
		var matched bool
		for _, m := range matchedOpenRegressions {
			if reflect.DeepEqual(m, regression) {
				matched = true
				break
			}
		}
		// If we didn't match to an active test regression, and this record isn't already closed, close it.
		if !matched && !regression.Closed.Valid {
			rLog.Infof("found a regression no longer appearing in the report which should be closed: %v", regression)
			if !rt.dryRun {
				err := rt.backend.CloseRegression(ctx, regression, now)
				if err != nil {
					rLog.WithError(err).Errorf("error closing regression: %v", regression)
					return errors.Wrap(err, "error closing regression")
				}
			}
		}

	}

	return nil
}

// FindOpenRegression scans the list of open regressions for any that match the given test summary.
func FindOpenRegression(view string,
	testID string,
	variants map[string]string,
	regressions []*crtype.TestRegression) *crtype.TestRegression {

	for _, tr := range regressions {

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

func findVariant(variantName string, testReg *crtype.TestRegression) string {
	for _, v := range testReg.Variants {
		keyVal := strings.Split(v, ":")
		if keyVal[0] == variantName {
			return keyVal[1]
		}
	}
	return ""
}
