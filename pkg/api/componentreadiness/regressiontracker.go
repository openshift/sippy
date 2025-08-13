package componentreadiness

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/openshift/sippy/pkg/api/componentreadiness/middleware/regressiontracker"
	"github.com/openshift/sippy/pkg/api/componentreadiness/utils"
	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crview"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/apis/cache"
	configv1 "github.com/openshift/sippy/pkg/apis/config/v1"
	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	sippybigquery "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
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
	ListCurrentRegressionsForRelease(release string) ([]*models.TestRegression, error)
	OpenRegression(view crview.View, newRegressedTest crtype.ReportTestSummary) (*models.TestRegression, error)
	UpdateRegression(reg *models.TestRegression) error
	// ResolveTriages sets the resolution time on any triages that no longer have active regressions
	ResolveTriages() error
}

type PostgresRegressionStore struct {
	dbc *db.DB
}

func NewPostgresRegressionStore(dbc *db.DB) RegressionStore {
	return &PostgresRegressionStore{dbc: dbc}
}

func (prs *PostgresRegressionStore) ListCurrentRegressionsForRelease(release string) ([]*models.TestRegression, error) {
	// List open regressions (no closed date), or those that closed within the last few days. This is to prevent flapping
	// and return more accurate opened dates when a test is falling in / out of the report.
	regressions := make([]*models.TestRegression, 0)
	q := prs.dbc.DB.Table(testRegressionsTable).
		Where("release = ?", release).
		Where("closed IS NULL OR closed > ?", time.Now().Add(-regressionHysteresisDays*24*time.Hour))
	res := q.Scan(&regressions)
	return regressions, res.Error
}

func (prs *PostgresRegressionStore) OpenRegression(view crview.View, newRegressedTest crtype.ReportTestSummary) (*models.TestRegression, error) {

	variants := utils.VariantsMapToStringSlice(newRegressedTest.Variants)

	newRegression := &models.TestRegression{
		View:        view.Name,
		Release:     view.SampleRelease.Name,
		TestID:      newRegressedTest.TestID,
		TestName:    newRegressedTest.TestName,
		Opened:      time.Now(),
		Variants:    variants,
		MaxFailures: newRegressedTest.SampleStats.FailureCount,
	}

	// Store the base release
	// so we can generate accurate test_details API links.
	// Start with the view's base release, but if the test got a base release override to a prior release, we use that instead.
	newRegression.BaseRelease = view.BaseRelease.Name
	if newRegressedTest.BaseStats != nil {
		newRegression.BaseRelease = newRegressedTest.BaseStats.Release
	}

	newRegression.Capability = newRegressedTest.Capability
	newRegression.Component = newRegressedTest.Component

	if newRegressedTest.LastFailure != nil {
		newRegression.LastFailure = sql.NullTime{Valid: true, Time: *newRegressedTest.LastFailure}
	}
	res := prs.dbc.DB.Create(newRegression)
	if res.Error != nil {
		return &models.TestRegression{}, res.Error
	}
	log.Infof("opened a new regression: %v", newRegression)
	return newRegression, nil

}

func (prs *PostgresRegressionStore) UpdateRegression(reg *models.TestRegression) error {
	res := prs.dbc.DB.Save(&reg)
	return res.Error
}

// ResolveTriages sets the resolution time on any triages that no longer have active regressions
// It only does so when all the regressions have been closed for at least regressionHysteresisDays (5) days
func (prs *PostgresRegressionStore) ResolveTriages() error {
	hysteresisTime := time.Now().Add(-regressionHysteresisDays * 24 * time.Hour)
	var triagesToResolve []models.Triage
	subQuery := prs.dbc.DB.Table("triage_regressions tr").
		Joins("JOIN test_regressions r ON tr.test_regression_id = r.id").
		Where("tr.triage_id = triages.id").
		Where("r.closed IS NULL OR r.closed > ?", hysteresisTime).
		Select("1")

	res := prs.dbc.DB.Table("triages").
		Where("resolved IS NULL").
		Where("NOT EXISTS (?)", subQuery).
		Preload("Regressions").
		Find(&triagesToResolve)

	if res.Error != nil {
		return fmt.Errorf("error finding triages to resolve: %v", res.Error)
	}

	log.Infof("Found %d triages to resolve", len(triagesToResolve))

	for _, triage := range triagesToResolve {
		var mostRecentClosedRegression models.TestRegression

		// Find the latest, closed regression in order to get the resolution time
		regQuery := prs.dbc.DB.Table("test_regressions").
			Joins("JOIN triage_regressions ON triage_regressions.test_regression_id = test_regressions.id").
			Where("triage_regressions.triage_id = ?", triage.ID).
			Where("test_regressions.closed IS NOT NULL").
			Order("test_regressions.closed DESC").
			Limit(1)

		res := regQuery.First(&mostRecentClosedRegression)
		if res.Error != nil {
			log.WithError(res.Error).Errorf("error finding most recent closed regression for triage %d", triage.ID)
			continue
		}

		triage.Resolved = mostRecentClosedRegression.Closed
		dbWithContext := prs.dbc.DB.WithContext(context.WithValue(context.Background(), models.CurrentUserKey, "regression-tracker"))
		res = dbWithContext.Save(&triage)
		if res.Error != nil {
			log.WithError(res.Error).Errorf("error resolving triage %d", triage.ID)
			continue
		}

		log.Infof("Resolved triage %d with resolution time %v", triage.ID, triage.Resolved.Time)
	}

	return nil
}

func NewRegressionTracker(
	bigqueryClient *sippybigquery.Client,
	dbc *db.DB,
	cacheOptions cache.RequestOptions,
	releases []v1.Release,
	backend RegressionStore,
	views []crview.View,
	overrides []configv1.VariantJunitTableOverride,
	dryRun bool) *RegressionTracker {

	return &RegressionTracker{
		bigqueryClient:             bigqueryClient,
		dbc:                        dbc,
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
	dbc                        *db.DB
	cacheOpts                  cache.RequestOptions
	releases                   []v1.Release
	dryRun                     bool
	views                      []crview.View
	logger                     log.FieldLogger
	variantJunitTableOverrides []configv1.VariantJunitTableOverride
	errors                     []error
}

func (rt *RegressionTracker) Name() string {
	return "regression-tracker"
}

// Load iterates all views with regression tracking enabled and syncs the results of its
// component report to the regression tables.
func (rt *RegressionTracker) Load() {
	for _, view := range rt.views {
		if view.RegressionTracking.Enabled {
			err := rt.SyncRegressionsForView(context.Background(), view)
			if err != nil {
				log.WithError(err).WithField("view", view.Name).Error("error refreshing regressions for view")
				rt.errors = append(rt.errors, err)
				// keep processing other views
			}
		}
	}
}

func (rt *RegressionTracker) Errors() []error {
	return rt.errors
}

func (rt *RegressionTracker) SyncRegressionsForView(ctx context.Context, view crview.View) error {
	rLog := rt.logger.WithField("view", view.Name)

	baseRelease, err := utils.GetViewReleaseOptions(
		rt.releases, "basis", view.BaseRelease, rt.cacheOpts.CRTimeRoundingFactor)
	if err != nil {
		return err
	}

	sampleRelease, err := utils.GetViewReleaseOptions(
		rt.releases, "sample", view.SampleRelease, rt.cacheOpts.CRTimeRoundingFactor)
	if err != nil {
		return err
	}

	variantOption := view.VariantOptions
	advancedOption := view.AdvancedOptions

	// Get component readiness report
	reportOpts := reqopts.RequestOptions{
		BaseRelease:    baseRelease,
		SampleRelease:  sampleRelease,
		VariantOption:  variantOption,
		AdvancedOption: advancedOption,
		CacheOption:    rt.cacheOpts,
	}

	report, errs := GetComponentReportFromBigQuery(
		ctx, rt.bigqueryClient, rt.dbc, reportOpts, rt.variantJunitTableOverrides, "")
	if len(errs) > 0 {
		var strErrors []string
		for _, err := range errs {
			strErrors = append(strErrors, err.Error())
		}
		return fmt.Errorf("component report generation encountered errors: %s", strings.Join(strErrors, "; "))
	}

	return rt.SyncRegressionsForReport(ctx, view, rLog, &report)
}

func (rt *RegressionTracker) SyncRegressionsForReport(ctx context.Context, view crview.View, rLog *log.Entry, report *crtype.ComponentReport) error {
	regressions, err := rt.backend.ListCurrentRegressionsForRelease(view.SampleRelease.Name)
	if err != nil {
		return err
	}
	rLog.Infof("loaded %d regressions from db for release %s", len(regressions), view.SampleRelease.Name)

	// All regressed tests, both triaged and not:
	allRegressedTests := []crtype.ReportTestSummary{}
	for _, row := range report.Rows {
		for _, col := range row.Columns {
			allRegressedTests = append(allRegressedTests, col.RegressedTests...)
		}
	}

	var openedRegs, reopenedRegs, ongoingRegs, closedRegs, statsUpdatedRegs int
	matchedOpenRegressions := []*models.TestRegression{} // all the matches we found, used to determine what had no match
	rLog.Infof("syncing %d open regressions", len(allRegressedTests))
	for _, regTest := range allRegressedTests {
		if openReg := regressiontracker.FindOpenRegression(view.Name, regTest.TestID, regTest.Variants, regressions); openReg != nil {

			// Update any tracking params on the regression if we see better values:
			var modifiedRegression bool
			if regTest.SampleStats.FailureCount > openReg.MaxFailures {
				openReg.MaxFailures = regTest.SampleStats.FailureCount
				modifiedRegression = true
			}
			if regTest.LastFailure != nil {
				if !openReg.LastFailure.Valid || regTest.LastFailure.After(openReg.LastFailure.Time) {
					openReg.LastFailure = sql.NullTime{Valid: true, Time: *regTest.LastFailure}
					modifiedRegression = true
				}
			}

			// BaseRelease was added to test_regressions later, this block allows us to set it for any pre-existing
			// regressions as soon as the reg tracker runs.
			// TODO: remove this block and make the field non-nullable once the db is updated
			if regTest.BaseStats.Release != openReg.BaseRelease {
				openReg.BaseRelease = regTest.BaseStats.Release
				modifiedRegression = true
			}

			// Technically component and capability could get remapped during the time the regression is open,
			// and we need this to roll out the storing of these fields initially:
			if regTest.Component != openReg.Component {
				openReg.Component = regTest.Component
				modifiedRegression = true
			}
			if regTest.Capability != openReg.Capability {
				openReg.Capability = regTest.Capability
				modifiedRegression = true
			}

			if modifiedRegression {
				statsUpdatedRegs++
				err := rt.backend.UpdateRegression(openReg)
				if err != nil {
					rLog.WithError(err).Errorf("error updating regression: %v", openReg)
					return errors.Wrapf(err, "error updating regression: %v", openReg)
				}
			}

			if openReg.Closed.Valid {
				// if the regression returned has a closedRegs date, we found a recently closedRegs
				// regression for this test. We'll re-use it to limit churn as sometimes tests may drop
				// in / out of the report depending on the data available in the sample/basis.
				rLog.Infof("re-opening existing regression: %v", openReg)
				if !rt.dryRun {
					reopenedRegs++
					openReg.Closed = sql.NullTime{Valid: false}
					err := rt.backend.UpdateRegression(openReg)
					if err != nil {
						rLog.WithError(err).Errorf("error re-opening regression: %v", openReg)
						return errors.Wrapf(err, "error re-opening regression: %v", openReg)
					}
				}
			} else {
				// Still consider untouched even if we bumped the max failures count
				ongoingRegs++
				rLog.WithFields(log.Fields{
					"test": regTest.TestName,
				}).Debugf("reusing already opened regression: %v", openReg)
			}
			matchedOpenRegressions = append(matchedOpenRegressions, openReg)
		} else {
			openedRegs++
			rLog.Infof("opening new regression: %v", regTest)
			if !rt.dryRun {
				// Open a new regression:
				newReg, err := rt.backend.OpenRegression(view, regTest)
				if err != nil {
					rLog.WithError(err).Errorf("error opening new regression for: %v", regTest)
					return errors.Wrapf(err, "error opening new regression: %v", regTest)
				}
				rLog.Infof("new regression opened with id: %d", newReg.ID)
			}
		}
	}

	// Now we want to close any open regressions that are not appearing in the latest report:
	now := time.Now()
	for _, regression := range regressions {
		var matched bool
		// We don't want to reject a regression as unmatched because it's max failures count is different.
		// We also do not want to wipe out the max failures value when closing.
		for _, m := range matchedOpenRegressions {
			if m.ID == regression.ID {
				matched = true
				break
			}
		}
		// If we didn't match to an active test regression, and this record isn't already closedRegs, close it.
		if !matched && !regression.Closed.Valid {
			rLog.Infof("found a regression no longer appearing in the report which should be closedRegs: %v", regression)
			closedRegs++
			if !rt.dryRun {
				regression.Closed = sql.NullTime{Valid: true, Time: now}
				err := rt.backend.UpdateRegression(regression)
				if err != nil {
					rLog.WithError(err).Errorf("error closing regression: %v", regression)
					return errors.Wrap(err, "error closing regression")
				}
			}
		}

	}

	rLog.Infof("resolving triages that have had all of their regressions closed")
	err = rt.backend.ResolveTriages()
	if err != nil {
		rLog.WithError(err).Error("error resolving triages")
		return errors.Wrap(err, "error resolving triages")
	}

	rLog.Infof("regression tracking sync completed, opened=%d, reopened=%d, closed=%d ongoing=%d statsUpdated=%d",
		openedRegs, reopenedRegs, closedRegs, ongoingRegs, statsUpdatedRegs)

	return nil
}
