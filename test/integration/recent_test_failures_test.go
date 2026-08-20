package integration

import (
	"testing"
	"time"

	"cloud.google.com/go/civil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"

	"github.com/openshift/sippy/pkg/api"
	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/filter"
	intutil "github.com/openshift/sippy/test/integration/util"
)

func recentFailuresTestDB(t *testing.T) *db.DB {
	t.Helper()
	return intutil.NewTestDB(t, pgContainer)
}

func TestRecentTestFailures_ReturnsFailedTestsWithCounts(t *testing.T) {
	dbc := recentFailuresTestDB(t)
	release := "4.19"

	testObj := intutil.CreateTest(t, dbc, "test-basic-failure")
	job := intutil.CreateProwJob(t, dbc, "job-1", release, nil)

	ts1 := time.Date(2024, 6, 8, 10, 0, 0, 0, time.UTC)
	ts2 := time.Date(2024, 6, 9, 14, 0, 0, 0, time.UTC)
	run1 := intutil.CreateProwJobRun(t, dbc, job.ID, release, ts1, false, v1.JobTestFailure)
	run2 := intutil.CreateProwJobRun(t, dbc, job.ID, release, ts2, false, v1.JobTestFailure)
	intutil.CreateProwJobRunTest(t, dbc, run1.ID, job.ID, testObj.ID, release, ts1, statusFailure)
	intutil.CreateProwJobRunTest(t, dbc, run2.ID, job.ID, testObj.ID, release, ts2, statusFailure)

	reportEnd := time.Date(2024, 6, 11, 0, 0, 0, 0, time.UTC)
	period := 7 * 24 * time.Hour

	result, err := api.GetRecentTestFailures(dbc, release, period, nil, false,
		&filter.FilterOptions{Filter: &filter.Filter{}}, nil, reportEnd)
	require.NoError(t, err)
	require.NotNil(t, result)

	rows := result.Rows.([]apitype.RecentTestFailure)
	require.Len(t, rows, 1)
	assert.Equal(t, "test-basic-failure", rows[0].TestName)
	assert.Equal(t, 2, rows[0].FailureCount)
	assert.Equal(t, ts1, rows[0].FirstFailure, "first_failure")
	assert.Equal(t, ts2, rows[0].LastFailure, "last_failure")
}

func TestRecentTestFailures_ExcludesTestsAlsoFailingInPreviousPeriod(t *testing.T) {
	dbc := recentFailuresTestDB(t)
	release := "4.19"

	testObj := intutil.CreateTest(t, dbc, "test-also-failed-before")
	job := intutil.CreateProwJob(t, dbc, "job-1", release, nil)

	// Failure in previous period.
	prevTs := time.Date(2024, 6, 2, 10, 0, 0, 0, time.UTC)
	prevRun := intutil.CreateProwJobRun(t, dbc, job.ID, release, prevTs, false, v1.JobTestFailure)
	intutil.CreateProwJobRunTest(t, dbc, prevRun.ID, job.ID, testObj.ID, release, prevTs, statusFailure)

	// Failure in current period.
	curTs := time.Date(2024, 6, 8, 10, 0, 0, 0, time.UTC)
	curRun := intutil.CreateProwJobRun(t, dbc, job.ID, release, curTs, false, v1.JobTestFailure)
	intutil.CreateProwJobRunTest(t, dbc, curRun.ID, job.ID, testObj.ID, release, curTs, statusFailure)

	reportEnd := time.Date(2024, 6, 11, 0, 0, 0, 0, time.UTC)
	period := 7 * 24 * time.Hour
	previousPeriod := 7 * 24 * time.Hour

	result, err := api.GetRecentTestFailures(dbc, release, period, &previousPeriod, false,
		&filter.FilterOptions{Filter: &filter.Filter{}}, nil, reportEnd)
	require.NoError(t, err)
	require.NotNil(t, result)

	rows := result.Rows.([]apitype.RecentTestFailure)
	assert.Empty(t, rows, "test that also failed in previous period should be excluded")
}

func TestRecentTestFailures_IncludesNewRegressionsWhenPreviousPeriodSet(t *testing.T) {
	dbc := recentFailuresTestDB(t)
	release := "4.19"

	testObj := intutil.CreateTest(t, dbc, "test-only-current-period")
	job := intutil.CreateProwJob(t, dbc, "job-1", release, nil)

	// Success in previous period (no failure).
	prevTs := time.Date(2024, 6, 2, 10, 0, 0, 0, time.UTC)
	prevRun := intutil.CreateProwJobRun(t, dbc, job.ID, release, prevTs, true, v1.JobSucceeded)
	intutil.CreateProwJobRunTest(t, dbc, prevRun.ID, job.ID, testObj.ID, release, prevTs, statusSuccess)

	// Failure in current period.
	curTs := time.Date(2024, 6, 8, 10, 0, 0, 0, time.UTC)
	curRun := intutil.CreateProwJobRun(t, dbc, job.ID, release, curTs, false, v1.JobTestFailure)
	intutil.CreateProwJobRunTest(t, dbc, curRun.ID, job.ID, testObj.ID, release, curTs, statusFailure)

	reportEnd := time.Date(2024, 6, 11, 0, 0, 0, 0, time.UTC)
	period := 7 * 24 * time.Hour
	previousPeriod := 7 * 24 * time.Hour

	result, err := api.GetRecentTestFailures(dbc, release, period, &previousPeriod, false,
		&filter.FilterOptions{Filter: &filter.Filter{}}, nil, reportEnd)
	require.NoError(t, err)
	require.NotNil(t, result)

	rows := result.Rows.([]apitype.RecentTestFailure)
	require.Len(t, rows, 1)
	assert.Equal(t, "test-only-current-period", rows[0].TestName)
	assert.Equal(t, 1, rows[0].FailureCount)
}

func TestRecentTestFailures_PopulatesLastPassFromCumulativeSummary(t *testing.T) {
	dbc := recentFailuresTestDB(t)
	release := "4.19"

	testObj := intutil.CreateTest(t, dbc, "test-with-last-pass")
	job := intutil.CreateProwJob(t, dbc, "job-1", release, nil)

	// Failure run for the main query.
	failTs := time.Date(2024, 6, 9, 14, 0, 0, 0, time.UTC)
	failRun := intutil.CreateProwJobRun(t, dbc, job.ID, release, failTs, false, v1.JobTestFailure)
	intutil.CreateProwJobRunTest(t, dbc, failRun.ID, job.ID, testObj.ID, release, failTs, statusFailure)

	// Cumulative summary with prefix_max_last_success for findLastPass.
	// reportEnd date = 2024-06-11, so cumulative summary at that date.
	lastSuccess := time.Date(2024, 6, 8, 10, 0, 0, 0, time.UTC)
	createCumulativeSummary(t, dbc,
		civil.Date{Year: 2024, Month: 6, Day: 11}, release,
		testObj.ID, job.ID, 0, 100, 90, 0,
		withLastSuccess(lastSuccess),
	)

	reportEnd := time.Date(2024, 6, 11, 0, 0, 0, 0, time.UTC)
	period := 7 * 24 * time.Hour

	result, err := api.GetRecentTestFailures(dbc, release, period, nil, false,
		&filter.FilterOptions{Filter: &filter.Filter{}}, nil, reportEnd)
	require.NoError(t, err)

	rows := result.Rows.([]apitype.RecentTestFailure)
	require.Len(t, rows, 1)
	require.NotNil(t, rows[0].LastPass, "LastPass should be populated from cumulative summary")
	assert.Equal(t, lastSuccess, *rows[0].LastPass, "last_pass")
}

func TestRecentTestFailures_SumsFailuresAcrossMultipleJobs(t *testing.T) {
	dbc := recentFailuresTestDB(t)
	release := "4.19"

	testObj := intutil.CreateTest(t, dbc, "test-multi-job")
	job1 := intutil.CreateProwJob(t, dbc, "job-a", release, nil)
	job2 := intutil.CreateProwJob(t, dbc, "job-b", release, nil)

	// Two failures from job1.
	ts1 := time.Date(2024, 6, 8, 10, 0, 0, 0, time.UTC)
	ts2 := time.Date(2024, 6, 9, 10, 0, 0, 0, time.UTC)
	run1 := intutil.CreateProwJobRun(t, dbc, job1.ID, release, ts1, false, v1.JobTestFailure)
	run2 := intutil.CreateProwJobRun(t, dbc, job1.ID, release, ts2, false, v1.JobTestFailure)
	intutil.CreateProwJobRunTest(t, dbc, run1.ID, job1.ID, testObj.ID, release, ts1, statusFailure)
	intutil.CreateProwJobRunTest(t, dbc, run2.ID, job1.ID, testObj.ID, release, ts2, statusFailure)

	// One failure from job2.
	ts3 := time.Date(2024, 6, 10, 16, 0, 0, 0, time.UTC)
	run3 := intutil.CreateProwJobRun(t, dbc, job2.ID, release, ts3, false, v1.JobTestFailure)
	intutil.CreateProwJobRunTest(t, dbc, run3.ID, job2.ID, testObj.ID, release, ts3, statusFailure)

	reportEnd := time.Date(2024, 6, 11, 0, 0, 0, 0, time.UTC)
	period := 7 * 24 * time.Hour

	result, err := api.GetRecentTestFailures(dbc, release, period, nil, false,
		&filter.FilterOptions{Filter: &filter.Filter{}}, nil, reportEnd)
	require.NoError(t, err)

	rows := result.Rows.([]apitype.RecentTestFailure)
	require.Len(t, rows, 1)
	assert.Equal(t, 3, rows[0].FailureCount, "should sum failures across both jobs")
	assert.Equal(t, ts1, rows[0].FirstFailure, "first_failure should be earliest across all jobs")
	assert.Equal(t, ts3, rows[0].LastFailure, "last_failure should be latest across all jobs")
}

func TestRecentTestFailures_ReportsSuitesSeparately(t *testing.T) {
	dbc := recentFailuresTestDB(t)
	release := "4.19"

	testObj := intutil.CreateTest(t, dbc, "test-with-suites")
	suiteA := intutil.CreateSuite(t, dbc, "suite-a")
	suiteB := intutil.CreateSuite(t, dbc, "suite-b")
	job := intutil.CreateProwJob(t, dbc, "job-1", release, nil)

	ts := time.Date(2024, 6, 9, 10, 0, 0, 0, time.UTC)
	run := intutil.CreateProwJobRun(t, dbc, job.ID, release, ts, false, v1.JobTestFailure)
	intutil.CreateProwJobRunTest(t, dbc, run.ID, job.ID, testObj.ID, release, ts, statusFailure, intutil.WithSuiteID(suiteA.ID))
	intutil.CreateProwJobRunTest(t, dbc, run.ID, job.ID, testObj.ID, release, ts, statusFailure, intutil.WithSuiteID(suiteB.ID))

	reportEnd := time.Date(2024, 6, 11, 0, 0, 0, 0, time.UTC)
	period := 7 * 24 * time.Hour

	result, err := api.GetRecentTestFailures(dbc, release, period, nil, false,
		&filter.FilterOptions{Filter: &filter.Filter{}}, nil, reportEnd)
	require.NoError(t, err)

	rows := result.Rows.([]apitype.RecentTestFailure)
	require.Len(t, rows, 2, "each suite should produce a separate row")

	suiteIDs := make(map[uint]bool)
	for _, r := range rows {
		require.NotNil(t, r.SuiteID)
		suiteIDs[*r.SuiteID] = true
		assert.Equal(t, 1, r.FailureCount)
	}
	assert.True(t, suiteIDs[suiteA.ID], "suite-a should be present")
	assert.True(t, suiteIDs[suiteB.ID], "suite-b should be present")
}
