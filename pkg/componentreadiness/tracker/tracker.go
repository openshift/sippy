package tracker

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/google/uuid"
	"github.com/openshift/sippy/pkg/apis/api"
	sippybigquery "github.com/openshift/sippy/pkg/bigquery"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
)

const (
	// TODO: don't commit
	testRegressionsTable = "test_regressions_dgoodwin_temp"
)

// RegressionStore is an underlying interface for where we store/load data on open test regressions.
type RegressionStore interface {
	ListCurrentRegressions(release string) ([]api.TestRegression, error)
	CloseRegression(regressionID string, closedAt time.Time, closeStats api.ComponentReportTestStats) error

	// TODO: can these be made private?
	OpenRegression(release, testID, testName string, variants map[string]string, testStats api.ComponentReportTestStats) (*api.TestRegression, error)
	ReOpenRegression(regressionID string) error
}

// BigQueryRegressionStore is the primary implementation for real world usage, storing when regressions appear/disappear in BigQuery.
type BigQueryRegressionStore struct {
	client *sippybigquery.Client
}

func NewBigQueryRegressionStore(client *sippybigquery.Client) RegressionStore {
	return &BigQueryRegressionStore{client: client}
}

func (bq *BigQueryRegressionStore) ListCurrentRegressions(release string) ([]api.TestRegression, error) {
	// List open regressions (no closed date), or those that closed within the last two days. This is to prevent flapping
	// and return more accurate opened dates when a test is falling in / out of the report.
	queryString := fmt.Sprintf("SELECT * FROM %s.%s WHERE release = @SampleRelease AND (closed IS NULL or closed > TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 2 DAY))",
		bq.client.Dataset, testRegressionsTable)

	params := make([]bigquery.QueryParameter, 0)
	params = append(params, []bigquery.QueryParameter{
		{
			Name:  "SampleRelease",
			Value: release,
		},
	}...)

	sampleQuery := bq.client.BQ.Query(queryString)
	sampleQuery.Parameters = append(sampleQuery.Parameters, params...)

	regressions := make([]api.TestRegression, 0)
	log.Infof("Fetching current test regressions with:\n%s\nParameters:\n%+v\n",
		sampleQuery.Q, sampleQuery.Parameters)

	it, err := sampleQuery.Read(context.TODO())
	if err != nil {
		log.WithError(err).Error("error querying triaged incidents from bigquery")
		return regressions, err
	}

	for {
		var regression api.TestRegression
		err := it.Next(&regression)
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.WithError(err).Error("error parsing triaged incident from bigquery")
			return nil, errors.Wrap(err, "error parsing triaged incident from bigquery")
		}
		regressions = append(regressions, regression)
	}
	return regressions, nil

}
func (bq *BigQueryRegressionStore) OpenRegression(
	release, testID, testName string,
	variants map[string]string,
	testStats api.ComponentReportTestStats) (*api.TestRegression, error) {
	id := uuid.New()
	newRegression := &api.TestRegression{
		Release:               release,
		TestID:                testID,
		TestName:              testName,
		RegressionID:          id.String(),
		Opened:                time.Now(),
		OpenedFisherExact:     testStats.FisherExact,
		OpenedSampleSuccesses: testStats.SampleStats.SuccessCount,
		OpenedSampleFailures:  testStats.SampleStats.FailureCount,
		OpenedSampleFlakes:    testStats.SampleStats.FlakeCount,
		OpenedSamplePassRate:  testStats.SampleStats.SuccessRate,
		OpenedBaseSuccesses:   testStats.BaseStats.SuccessCount,
		OpenedBaseFailures:    testStats.BaseStats.FailureCount,
		OpenedBaseFlakes:      testStats.BaseStats.FlakeCount,
		OpenedBasePassRate:    testStats.BaseStats.SuccessRate,
	}
	for key, value := range variants {
		newRegression.Variants = append(newRegression.Variants, api.ComponentReportVariant{
			Key: key, Value: value,
		})
	}
	inserter := bq.client.BQ.Dataset(bq.client.Dataset).Table(testRegressionsTable).Inserter()
	items := []*api.TestRegression{
		newRegression,
	}
	if err := inserter.Put(context.TODO(), items); err != nil {
		return nil, err
	}
	return newRegression, nil

}

func (bq *BigQueryRegressionStore) ReOpenRegression(regressionID string) error {
	return bq.updateClosed(regressionID, "NULL", api.ComponentReportTestStats{})
}

func (bq *BigQueryRegressionStore) CloseRegression(regressionID string, closedAt time.Time, closeStats api.ComponentReportTestStats) error {
	return bq.updateClosed(regressionID,
		fmt.Sprintf("'%s'", closedAt.Format("2006-01-02 15:04:05.999999")), closeStats)
}

func (bq *BigQueryRegressionStore) updateClosed(regressionID, closed string, closeStats api.ComponentReportTestStats) error {
	queryString := fmt.Sprintf(`
UPDATE %s.%s SET 
	closed = %s, 
	closed_sample_successes = %d, 
	closed_sample_failures = %d, 
	closed_sample_flakes = %d, 
	closed_sample_pass_rate = %.2f 
	closed_fisher_exact = %.2f 
WHERE regression_id = '%s'`,
		bq.client.Dataset,
		testRegressionsTable,
		closed,
		closeStats.SampleStats.SuccessCount,
		closeStats.SampleStats.FailureCount,
		closeStats.SampleStats.FlakeCount,
		closeStats.SampleStats.SuccessRate,
		closeStats.FisherExact,
		regressionID)

	query := bq.client.BQ.Query(queryString)

	job, err := query.Run(context.TODO())
	if err != nil {
		return err
	}

	status, err := job.Wait(context.TODO())
	if err != nil {
		return err
	}

	err = status.Err()
	return err
}

func NewRegressionTracker(backend RegressionStore, dryRun bool) *RegressionTracker {
	return &RegressionTracker{
		backend: backend,
		dryRun:  dryRun,
	}
}

// RegressionTracker is the primary object for managing regression tracking logic.
type RegressionTracker struct {
	backend RegressionStore
	dryRun  bool
}

func (rt *RegressionTracker) CloseRegression(rLog log.FieldLogger, regressionID string,
	testStats api.ComponentReportTestStats, closeTime time.Time) error {
	rLog.Infof("found a regression which should be closed: %+v currentStats: %+v", regressionID, testStats)
	if !rt.dryRun {
		err := rt.backend.CloseRegression(regressionID, closeTime, testStats)
		if err != nil {
			rLog.WithError(err).Errorf("error closing regression: %s", regressionID)
			return errors.Wrapf(err, "error closing regression: %s", regressionID)
		}
	}
	return nil
}

// ReuseOrOpenRegression will check the list of open regressions for a match or a recent closure within the last few days.
// If a regression is already open it will no-op. If a recent closure is found, it will be re-used and re-opened.
// Otherwise we'll open a new regression.
//
// Return the regression opened/reused, a bool true if it was a new regression, and optionally an error.
func (rt *RegressionTracker) ReuseOrOpenRegression(
	rLog log.FieldLogger,
	release, testID, testName string,
	variants map[string]string,
	testStats api.ComponentReportTestStats, openRegressions []api.TestRegression) (*api.TestRegression, bool, error) {
	tLog := rLog.WithFields(log.Fields{
		"release":  release,
		"testID":   testID,
		"testName": testName,
		"variants": variants,
	})

	if openReg := FindOpenRegression(release, testID, variants, openRegressions); openReg != nil {
		if openReg.Closed.Valid {
			// if the regression returned has a closed date, we found a recently closed
			// regression for this test. We'll re-use it to limit churn as sometimes tests may drop
			// in / out of the report depending on the data available in the sample/basis.
			tLog.Infof("re-opening existing regression: %v", openReg)
			if !rt.dryRun {
				err := rt.backend.ReOpenRegression(openReg.RegressionID)
				if err != nil {
					tLog.WithError(err).Errorf("error re-opening regression: %v", openReg)
					return nil, false, errors.Wrapf(err, "error re-opening regression: %v", openReg)
				}
			}
		} else {
			tLog.Infof("reusing already opened regression: %+v", openReg)

		}
		return openReg, false, nil
	}

	tLog.Infof("opening new regression with stats: %+v", testStats)
	if !rt.dryRun {
		// Open a new regression:
		newReg, err := rt.backend.OpenRegression(release, testID, testName, variants, testStats)
		if err != nil {
			tLog.WithError(err).Error("error opening new regression")
			return nil, false, errors.Wrap(err, "error opening new regression")
		}
		tLog.Infof("new regression opened with id: %s", newReg.RegressionID)
		return newReg, true, nil
	}

	// No open regression, but we were in dry-run mode.
	return nil, false, nil
}

// FindOpenRegression scans the list of open regressions for any that match the given test summary.
func FindOpenRegression(release string,
	testID string,
	variants map[string]string,
	regressions []api.TestRegression) *api.TestRegression {

	for _, tr := range regressions {
		if tr.Release != release {
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
		return &tr
	}
	return nil
}

func findVariant(variantName string, testReg api.TestRegression) string {
	for _, v := range testReg.Variants {
		if v.Key == variantName {
			return v.Value
		}
	}
	return ""
}
