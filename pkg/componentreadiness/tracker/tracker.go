package tracker

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/google/uuid"
	"github.com/openshift/sippy/pkg/apis/api"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
)

const (
	testRegressionsDataSet = "ci_analysis_us"
	testRegressionsTable   = "test_regressions"
)

type Storage interface {
	ListCurrentRegressions(release string) ([]api.TestRegression, error)
	OpenRegression(release string, newRegressedTest api.ComponentReportTestSummary) (*api.TestRegression, error)
	CloseRegression(regressionID string, closedAt time.Time) error
}

// InMemoryRegressionTracker is purely for testing purposes.
type InMemoryStorage struct{}

// BigQueryStorage is the primary implementation for real world usage, tracking when we detect test
// regressions opening/closing in the Bigquery test_regressions table.
type BigQueryStorage struct {
	client *bigquery.Client
}

func NewBigQueryStorage(client *bigquery.Client) Storage {
	return &BigQueryStorage{client: client}
}

func (bqs *BigQueryStorage) ListCurrentRegressions(release string) ([]api.TestRegression, error) {
	// TODO: list only open regressions, or possibly in future also those closed within last few days to
	// detect churn. For now we'll keep it simple and only list open.
	queryString := fmt.Sprintf("SELECT * FROM %s.%s WHERE release = @SampleRelease AND closed IS NULL",
		testRegressionsDataSet, testRegressionsTable)

	params := make([]bigquery.QueryParameter, 0)
	params = append(params, []bigquery.QueryParameter{
		{
			Name:  "SampleRelease",
			Value: release,
		},
	}...)

	sampleQuery := bqs.client.Query(queryString)
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
func (bqs *BigQueryStorage) OpenRegression(release string, newRegressedTest api.ComponentReportTestSummary) (*api.TestRegression, error) {
	id := uuid.New()
	newRegression := &api.TestRegression{
		Release:      release,
		TestID:       newRegressedTest.TestID,
		TestName:     bigquery.NullString{StringVal: newRegressedTest.TestName, Valid: true},
		RegressionID: id.String(),
		Opened:       time.Now(),
		Variants: []api.TriagedVariant{
			{
				Key:   "Network",
				Value: newRegressedTest.Network,
			},
			{
				Key:   "Upgrade",
				Value: newRegressedTest.Upgrade,
			},
			{
				Key:   "Platform",
				Value: newRegressedTest.Platform,
			},
			{
				Key:   "Architecture",
				Value: newRegressedTest.Arch,
			},
			{
				Key:   "Variant",
				Value: newRegressedTest.Variant,
			},
		},
	}
	inserter := bqs.client.Dataset(testRegressionsDataSet).Table(testRegressionsTable).Inserter()
	items := []*api.TestRegression{
		newRegression,
	}
	if err := inserter.Put(context.TODO(), items); err != nil {
		return nil, err
	}
	return newRegression, nil

}
func (bqs *BigQueryStorage) CloseRegression(regressionID string, closedAt time.Time) error {
	queryString := fmt.Sprintf("UPDATE %s.%s SET closed = '%s' WHERE regression_id = '%s'",
		testRegressionsDataSet, testRegressionsTable, closedAt.Format("2006-01-02 15:04:05.999999"), regressionID)

	query := bqs.client.Query(queryString)

	// Execute the query
	job, err := query.Run(context.TODO())
	if err != nil {
		return err
	}

	// Wait for the query to complete and retrieve the job status
	status, err := job.Wait(context.TODO())
	if err != nil {
		return err
	}

	if err := status.Err(); err != nil {
		return err
	}
	return nil

}

func NewRegressionTracker(storage Storage) *RegressionTracker {
	return &RegressionTracker{
		storage: storage,
	}
}

// RegressionTracker is the primary object for managing regression tracking logic.
type RegressionTracker struct {
	storage Storage
}

func (rt *RegressionTracker) SyncComponentReport(release string, report *api.ComponentReport) error {
	regressions, err := rt.storage.ListCurrentRegressions(release)
	if err != nil {
		return err
	}
	rLog := log.WithField("func", "SyncComponentReport")
	rLog.Infof("loaded %d regressions from db", len(regressions))

	// All regressions, both traiged and not:
	allRegressedTests := []api.ComponentReportTestSummary{}
	for _, row := range report.Rows {
		for _, col := range row.Columns {
			for _, regTest := range col.RegressedTests {
				allRegressedTests = append(allRegressedTests, regTest)
			}
			// Once triaged, regressions move to this list, we want to still consider them an open regression until
			// the report says they're cleared and they disappear fully. Triaged does not imply fixed or no longer
			// a regression.
			for _, triaged := range col.TriagedIncidents {
				allRegressedTests = append(allRegressedTests, triaged.ComponentReportTestSummary)
			}
		}
	}

	matchedOpenRegressions := []api.TestRegression{} // all the matches we found, used to determine what had no match
	for _, regTest := range allRegressedTests {
		if openReg := FindOpenRegression(release, regTest, regressions); openReg != nil {
			rLog.WithFields(log.Fields{
				"test": regTest.TestName,
			}).Infof("found open regression: %+v", openReg)
			matchedOpenRegressions = append(matchedOpenRegressions, *openReg)
		} else {
			// Open a new regression:
			newReg, err := rt.storage.OpenRegression(release, regTest)
			if err != nil {
				rLog.WithError(err).Errorf("error opening new regression for: %+v", regTest)
				return errors.Wrapf(err, "error opening new regression: %+v", regTest)
			}
			rLog.Infof("opened new regression: %+v", newReg)
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
		if !matched {
			rLog.Infof("found a regression no longer appearing in the report which should be closed: %+v", regression)
			err := rt.storage.CloseRegression(regression.RegressionID, now)
			if err != nil {
				rLog.WithError(err).Errorf("error closing regression: %+v", regression)
				return errors.Wrap(err, "error closing regression")
			}
		}

	}

	return nil
}

// FindOpenRegression scans the list of open regressions for any that match the given test summary.
func FindOpenRegression(release string,
	regTest api.ComponentReportTestSummary,
	regressions []api.TestRegression) *api.TestRegression {

	for _, tr := range regressions {
		if tr.Release != release {
			continue
		}
		// We compare test ID not name, as names can change.
		if tr.TestID != regTest.TestID {
			continue
		}
		// TODO: needs updating when we're ready for dynamic variants
		if regTest.Network != findVariant("Network", tr) {
			continue
		}
		if regTest.Upgrade != findVariant("Upgrade", tr) {
			continue
		}
		if regTest.Platform != findVariant("Platform", tr) {
			continue
		}
		if regTest.Variant != findVariant("Variant", tr) {
			continue
		}
		if regTest.Arch != findVariant("Architecture", tr) {
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
