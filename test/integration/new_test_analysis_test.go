package integration

import (
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/sippy/pkg/api"
	apitype "github.com/openshift/sippy/pkg/apis/api"
	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/filter"
	intutil "github.com/openshift/sippy/test/integration/util"
)

// setupNewTestAnalysisData creates test fixtures for verifying FetchJobRun
// with onlyNewTests=true. It creates:
//   - A target job run with two tests: one that appeared in a merged PR
//     (within 90 days), and one that is genuinely new.
//   - A merged PR job run with the "old" test to establish prior history.
//   - A test_ownerships row for a third test to verify that path as well.
func setupNewTestAnalysisData(t *testing.T, dbc *db.DB) newTestAnalysisData {
	t.Helper()
	var td newTestAnalysisData

	td.job = intutil.CreateProwJob(t, dbc, "pull-ci-test-e2e-aws", "4.16", []string{"aws"})

	now := time.Date(2024, 7, 10, 12, 0, 0, 0, time.UTC)

	// Create the target job run (the one we're analyzing for new tests)
	td.targetRun = intutil.CreateProwJobRun(t, dbc, td.job.ID, "4.16", now, false, v1.JobTestFailure)

	// Create a merged PR and a prior job run associated with it
	td.mergedPR = models.ProwPullRequest{
		Org:      "openshift",
		Repo:     "origin",
		Number:   12345,
		Author:   "testauthor",
		MergedAt: timePtr(now.Add(-30 * 24 * time.Hour)),
	}
	require.NoError(t, dbc.DB.Create(&td.mergedPR).Error)

	prJobRunTime := now.Add(-30 * 24 * time.Hour)
	td.mergedPRRun = intutil.CreateProwJobRun(t, dbc, td.job.ID, "4.16", prJobRunTime, true, v1.JobSucceeded)

	// Link the merged PR to its job run
	prLink := models.ProwJobRunProwPullRequest{
		ProwJobRunID:        td.mergedPRRun.ID,
		ProwPullRequestID:   td.mergedPR.ID,
		ProwJobRunRelease:   "4.16",
		ProwJobRunTimestamp: prJobRunTime,
	}
	require.NoError(t, dbc.DB.Create(&prLink).Error)

	// Create tests
	td.oldTest = intutil.CreateTest(t, dbc, "test-that-existed-in-merged-pr")
	td.newTest = intutil.CreateTest(t, dbc, "test-never-seen-before")
	td.ownedTest = intutil.CreateTest(t, dbc, "test-in-ownership-registry")

	suite := intutil.CreateSuite(t, dbc, "e2e-suite")

	// Add test results to the target run
	intutil.CreateProwJobRunTest(t, dbc, td.targetRun.ID, td.job.ID, td.oldTest.ID,
		"4.16", now, int(v1.TestStatusFailure), intutil.WithSuiteID(suite.ID))
	intutil.CreateProwJobRunTest(t, dbc, td.targetRun.ID, td.job.ID, td.newTest.ID,
		"4.16", now, int(v1.TestStatusFailure), intutil.WithSuiteID(suite.ID))
	intutil.CreateProwJobRunTest(t, dbc, td.targetRun.ID, td.job.ID, td.ownedTest.ID,
		"4.16", now, int(v1.TestStatusFailure), intutil.WithSuiteID(suite.ID))

	// Add the "old" test to the merged PR's run (makes it not-new)
	intutil.CreateProwJobRunTest(t, dbc, td.mergedPRRun.ID, td.job.ID, td.oldTest.ID,
		"4.16", prJobRunTime, int(v1.TestStatusSuccess), intutil.WithSuiteID(suite.ID))

	// Register the owned test in test_ownerships (makes it not-new)
	intutil.CreateTestOwnership(t, dbc, td.ownedTest.ID, &suite.ID, "owned-test-id", "TestComponent")

	return td
}

type newTestAnalysisData struct {
	job         models.ProwJob
	targetRun   models.ProwJobRun
	mergedPRRun models.ProwJobRun
	mergedPR    models.ProwPullRequest
	oldTest     models.Test
	newTest     models.Test
	ownedTest   models.Test
}

func timePtr(t time.Time) *time.Time {
	return &t
}

func fetchJobRunInt64(dbc *db.DB, id uint, onlyNewTests bool, preloads []string, logger *log.Entry) (*models.ProwJobRun, error) {
	return api.FetchJobRun(dbc, int64(id), onlyNewTests, preloads, logger) //nolint:gosec // G115: test ID from serial, within int64 range
}

func TestFetchJobRunOnlyNewTests(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	td := setupNewTestAnalysisData(t, dbc)
	logger := log.WithField("test", t.Name())

	t.Run("only returns tests not seen in merged PRs or ownership registry", func(t *testing.T) {
		jobRun, err := fetchJobRunInt64(dbc, td.targetRun.ID, true, nil, logger)
		require.NoError(t, err)
		require.NotNil(t, jobRun)

		var testNames []string
		for _, test := range jobRun.Tests {
			testNames = append(testNames, test.Test.Name)
		}

		assert.Contains(t, testNames, "test-never-seen-before",
			"genuinely new test should be returned")
		assert.NotContains(t, testNames, "test-that-existed-in-merged-pr",
			"test seen in a recent merged PR should be excluded")
		assert.NotContains(t, testNames, "test-in-ownership-registry",
			"test in ownership registry should be excluded")
	})

	t.Run("failed tests returned when onlyNewTests is false", func(t *testing.T) {
		jobRun, err := fetchJobRunInt64(dbc, td.targetRun.ID, false, nil, logger)
		require.NoError(t, err)
		require.NotNil(t, jobRun)

		var testNames []string
		for _, test := range jobRun.Tests {
			testNames = append(testNames, test.Test.Name)
		}

		assert.Contains(t, testNames, "test-that-existed-in-merged-pr",
			"all failed tests should be returned when onlyNewTests is false")
		assert.Contains(t, testNames, "test-never-seen-before")
		assert.Contains(t, testNames, "test-in-ownership-registry")
	})
}

func TestPrintJobAnalysisEmptyJobs(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	now := time.Now().UTC()
	start := now.Add(-14 * 24 * time.Hour)
	boundary := now.Add(-7 * 24 * time.Hour)

	// Use a filter that matches no jobs
	noMatchFilter := &filter.Filter{
		Items: []filter.FilterItem{
			{Field: "name", Value: "nonexistent-job-name-that-matches-nothing", Operator: filter.OperatorEquals},
		},
	}

	result, err := api.PrintJobAnalysisJSONFromDB(
		dbc, "4.99", noMatchFilter, &filter.Filter{},
		start, boundary, now, 10, "name", apitype.SortDescending, api.PeriodDay, now,
	)
	require.NoError(t, err)
	assert.Empty(t, result.ByPeriod,
		"empty job list should return empty result without querying partitioned tables")
}
