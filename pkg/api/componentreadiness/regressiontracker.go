package componentreadiness

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/google/uuid"
	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/cache"
	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	sippybigquery "github.com/openshift/sippy/pkg/bigquery"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
)

const (
	testRegressionsTable = "test_regressions_append"
)

type RegressionStore interface {
	ListCurrentRegressions(ctx context.Context) ([]*crtype.TestRegression, error)
	SyncRegressions(ctx context.Context, allRegressions []*crtype.TestRegression) error
}

// BigQueryRegressionStore is the primary implementation for real world usage, storing when regressions appear/disappear in BigQuery.
type BigQueryRegressionStore struct {
	client *sippybigquery.Client
}

func NewBigQueryRegressionStore(client *sippybigquery.Client) RegressionStore {
	return &BigQueryRegressionStore{client: client}
}

// ListCurrentRegressionsForRelease returns *all* regressions for all releases. We operate on the assumption that
// only one view is allowed to have regression tracking enabled (i.e. 4.18-main) per release, which is validated
// when the views file is loaded. This is because we want to display regression tracking data on any report that shows
// a regressed test, so users using custom reporting can see what is regressed in main as well.
func (bq *BigQueryRegressionStore) ListCurrentRegressions(ctx context.Context) ([]*crtype.TestRegression, error) {
	// Use max snapshot date to get the most recently appended view of the regressions.
	queryString := fmt.Sprintf("SELECT * FROM %s.%s WHERE snapshot = (SELECT MAX(snapshot) FROM %s.%s)",
		bq.client.Dataset, testRegressionsTable, bq.client.Dataset, testRegressionsTable)

	sampleQuery := bq.client.BQ.Query(queryString)

	regressions := make([]*crtype.TestRegression, 0)
	log.Infof("Fetching current test regressions with: %s", sampleQuery.Q)

	it, err := sampleQuery.Read(ctx)
	if err != nil {
		log.WithError(err).Error("error querying triaged incidents from bigquery")
		return regressions, err
	}

	for {
		var regression crtype.TestRegression
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

// SyncRegressions appends all updated regressions to the table for all releases, with a new unique snapshot timestamp.
// This is done because of the problems we've encountered using BigQuery to keep data updated. (streaming buffer delays)
func (bq *BigQueryRegressionStore) SyncRegressions(ctx context.Context, allRegressions []*crtype.TestRegression) error {
	inserter := bq.client.BQ.Dataset(bq.client.Dataset).Table(testRegressionsTable).Inserter()
	return inserter.Put(ctx, allRegressions)
}

func NewRegressionTracker(
	bigqueryClient *sippybigquery.Client,
	cacheOptions cache.RequestOptions,
	releases []v1.Release,
	backend RegressionStore,
	views []crtype.View,
	dryRun bool) *RegressionTracker {

	return &RegressionTracker{
		bigqueryClient: bigqueryClient,
		cacheOpts:      cacheOptions,
		releases:       releases,
		backend:        backend,
		views:          views,
		dryRun:         dryRun,
		logger:         log.WithField("daemon", "regression-tracker"),
	}
}

// RegressionTracker is the primary object for managing regression tracking logic.
type RegressionTracker struct {
	backend        RegressionStore
	bigqueryClient *sippybigquery.Client
	cacheOpts      cache.RequestOptions
	releases       []v1.Release
	dryRun         bool
	views          []crtype.View
	logger         log.FieldLogger
}

// Run iterates all views with regression tracking enabled and syncs the results of it's
// component report to the regression tables in bigquery.
func (rt *RegressionTracker) Run(ctx context.Context) error {
	// Grab the latest regression snapshot in the db across all releases and filter + pass them into each run.
	allRegressions, err := rt.backend.ListCurrentRegressions(ctx)
	if err != nil {
		rt.logger.WithError(err).Error("error listing current regressions from bigquery")
		return err
	}

	// Build out an entirely new regression snapshot containing all releases.
	newRegressionSnapshot := []*crtype.TestRegression{}

	// Single snapshot time used for opened on new regressions, as well as every record in the new snapshot.
	snapshotTime := time.Now()

	rt.logger.Infof("loaded %d regressions from db for all releases from last snapshot", len(allRegressions))
	var updateNeeded bool
	for _, view := range rt.views {
		rLog := rt.logger.WithField("view", view.Name)
		if view.RegressionTracking.Enabled {
			releaseRegressions := FilterRegressionsForRelease(allRegressions, view.SampleRelease.Release)
			rLog.Infof("filtered down to %d regressions for release %s", len(releaseRegressions), view.SampleRelease.Release)
			newReleaseRegressions, modified, err := rt.syncRegressionsForView(ctx, rLog, snapshotTime, releaseRegressions, view)
			if err != nil {
				rLog.WithError(err).Error("error refreshing regressions for view")
				return err
			}
			rLog.WithField("before", len(releaseRegressions)).WithField("after", len(newReleaseRegressions)).Info("processed regressions for view")
			newRegressionSnapshot = append(newRegressionSnapshot, newReleaseRegressions...)
			if modified {
				updateNeeded = true
			}
		}
	}

	for i := range newRegressionSnapshot {
		newRegressionSnapshot[i].Snapshot = snapshotTime
	}

	if !updateNeeded {
		rt.logger.Info("no regression changed needed, skipping new snapshot")
	} else if !rt.dryRun {
		rt.logger.WithField("snapshot", snapshotTime).Infof(
			"syncing %d total regressions for all releases to bigquery", len(newRegressionSnapshot))
		err = rt.backend.SyncRegressions(ctx, newRegressionSnapshot)
		if err != nil {
			rt.logger.WithError(err).Error("unable to sync to bigquery")
			return err
		}
	} else {
		rt.logger.WithField("snapshot", snapshotTime).Warnf(
			"SKIPPED syncing %d total regressions for all releases to bigquery due to dry-run flag",
			len(newRegressionSnapshot))
	}

	return nil
}

func (rt *RegressionTracker) syncRegressionsForView(
	ctx context.Context,
	rLog log.FieldLogger,
	snapshotTime time.Time,
	currentReleaseRegressions []*crtype.TestRegression,
	view crtype.View) ([]*crtype.TestRegression, bool, error) {
	rLog.Info("syncing regressions for view")

	// The updated slice of regressions we'll return to send to the db for this release.
	releaseRegressions := []*crtype.TestRegression{}

	baseRelease, err := GetViewReleaseOptions(
		rt.releases, "basis", view.BaseRelease, rt.cacheOpts.CRTimeRoundingFactor)
	if err != nil {
		return releaseRegressions, false, err
	}

	sampleRelease, err := GetViewReleaseOptions(
		rt.releases, "sample", view.SampleRelease, rt.cacheOpts.CRTimeRoundingFactor)
	if err != nil {
		return releaseRegressions, false, err
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
		ctx, rt.bigqueryClient, "", "", reportOpts)
	if len(errs) > 0 {
		var strErrors []string
		for _, err := range errs {
			strErrors = append(strErrors, err.Error())
		}
		return releaseRegressions, false, fmt.Errorf("component report generation encountered errors: " + strings.Join(strErrors, "; "))
	}

	// All regressions in the component report, both triaged and not:
	regressedTestsReport := []crtype.ReportTestSummary{}
	for _, row := range report.Rows {
		for _, col := range row.Columns {
			regressedTestsReport = append(regressedTestsReport, col.RegressedTests...)
			// Once triaged, regressions move to this list, we want to still consider them an open regression until
			// the report says they're cleared and they disappear fully. Triaged does not imply fixed or no longer
			// a regression.
			for _, triaged := range col.TriagedIncidents {
				regressedTestsReport = append(regressedTestsReport, triaged.ReportTestSummary)
			}
		}
	}

	rLog.Infof("syncing %d open regressions", len(regressedTestsReport))
	var opened, closed, ongoing int
	for _, regTest := range regressedTestsReport {
		if openReg := FindOpenRegression(view.Name, regTest.TestID, regTest.Variants, currentReleaseRegressions); openReg != nil {
			// If we found an existing closed regression within the last two days, re-open it by nulling the closed
			// timestamp to prevent churn.
			if openReg.Closed.Valid && time.Until(openReg.Closed.Timestamp) <= 48*time.Hour {
				rLog.Infof("re-opening existing regression that was recently closed: %v", openReg)
				openReg.Closed = bigquery.NullTimestamp{}
				releaseRegressions = append(releaseRegressions, openReg)
				opened++
			} else {
				rLog.WithFields(log.Fields{
					"test": regTest.TestName,
				}).Debugf("regression already exists, no action required: %v", openReg)
				ongoing++

			}
			releaseRegressions = append(releaseRegressions, openReg)
		} else {
			// Open a new regression:
			id := uuid.New()
			newRegression := &crtype.TestRegression{
				View:         view.Name,
				Release:      view.SampleRelease.Release,
				TestID:       regTest.TestID,
				TestName:     regTest.TestName,
				RegressionID: id.String(),
				Opened:       snapshotTime,
			}
			for key, value := range regTest.Variants {
				newRegression.Variants = append(newRegression.Variants, crtype.Variant{
					Key: key, Value: value,
				})
			}
			rLog.Infof("created new regression: %+v", newRegression)
			releaseRegressions = append(releaseRegressions, newRegression)
			opened++
		}
	}

	// Now we want to close any open regressions that are not appearing in the latest report:
	for _, regression := range currentReleaseRegressions {
		var matched bool
		// Ignore those we've already processed because they were in the report:
		for _, m := range releaseRegressions {
			if reflect.DeepEqual(*m, *regression) {
				matched = true
				break
			}
		}
		// If we didn't match to an active test regression, and this record isn't already closed, close it.
		if !matched && !regression.Closed.Valid {
			rLog.Infof("found a regression no longer appearing in the report which should be closed: %+v", regression)
			regression.Closed = bigquery.NullTimestamp{Timestamp: snapshotTime, Valid: true}
			releaseRegressions = append(releaseRegressions, regression)
			closed++
		}
	}
	rLog.WithFields(log.Fields{
		"total":   len(releaseRegressions),
		"opened":  opened,
		"closed":  closed,
		"ongoing": ongoing,
	}).Infof("finished processing regressions for release")

	return releaseRegressions, opened > 0 || closed > 0, nil
}

// FindOpenRegression scans the list of open regressions for any that match the given test summary.
func FindOpenRegression(view string,
	testID string,
	variants map[string]string,
	regressions []*crtype.TestRegression) *crtype.TestRegression {

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

func findVariant(variantName string, testReg *crtype.TestRegression) string {
	for _, v := range testReg.Variants {
		if v.Key == variantName {
			return v.Value
		}
	}
	return ""
}

func FilterRegressionsForRelease(allRegressions []*crtype.TestRegression, release string) []*crtype.TestRegression {
	releaseRegressions := make([]*crtype.TestRegression, 0)
	for _, regression := range allRegressions {
		if regression.Release == release {
			releaseRegressions = append(releaseRegressions, regression)
		}
	}
	return releaseRegressions
}
