package integration

import (
	"testing"
	"time"

	"cloud.google.com/go/civil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/sippy/pkg/api"
	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db"
	intutil "github.com/openshift/sippy/test/integration/util"
)

var lookbackDate = civil.Date{Year: 2024, Month: 7, Day: 15}

func lookbackCountDB(t *testing.T) *db.DB {
	t.Helper()
	return intutil.NewTestDB(t, pgContainer)
}

type lookbackFixtures struct {
	jobA, jobB                     uint
	suiteA                         uint
	testAlpha, testBeta, testGamma uint
}

func seedLookbackFixtures(t *testing.T, dbc *db.DB) lookbackFixtures {
	t.Helper()

	intutil.CreateReleaseDefinition(t, dbc, "4.18", 4, 18)
	intutil.CreateReleaseDefinition(t, dbc, "4.19", 4, 19)

	jobA := intutil.CreateProwJob(t, dbc, "periodic-e2e-aws", "4.18", []string{"aws"})
	jobB := intutil.CreateProwJob(t, dbc, "periodic-e2e-gcp", "4.19", []string{"gcp"})

	suite := intutil.CreateSuite(t, dbc, "openshift-tests")

	testAlpha := intutil.CreateTest(t, dbc, "alpha-test")
	testBeta := intutil.CreateTest(t, dbc, "beta-test")
	testGamma := intutil.CreateTest(t, dbc, "gamma-test")

	return lookbackFixtures{
		jobA:      jobA.ID,
		jobB:      jobB.ID,
		suiteA:    suite.ID,
		testAlpha: testAlpha.ID,
		testBeta:  testBeta.ID,
		testGamma: testGamma.ID,
	}
}

func TestLookbackCount_OnlyCountsActiveRelease(t *testing.T) {
	dbc := lookbackCountDB(t)
	f := seedLookbackFixtures(t, dbc)

	today := lookbackDate
	startMinusOne := today.AddDays(-15)

	// testAlpha and testBeta in the active release (4.18).
	intutil.CreateCumulativeSummary(t, dbc, today, "4.18", f.testAlpha, f.jobA, f.suiteA, 100, 90, 5)
	intutil.CreateCumulativeSummary(t, dbc, startMinusOne, "4.18", f.testAlpha, f.jobA, f.suiteA, 80, 70, 3)

	intutil.CreateCumulativeSummary(t, dbc, today, "4.18", f.testBeta, f.jobA, f.suiteA, 60, 55, 3)
	intutil.CreateCumulativeSummary(t, dbc, startMinusOne, "4.18", f.testBeta, f.jobA, f.suiteA, 40, 35, 2)

	// testGamma only in 4.19 (not the active release, should be ignored).
	intutil.CreateCumulativeSummary(t, dbc, today, "4.19", f.testGamma, f.jobB, f.suiteA, 50, 45, 2)
	intutil.CreateCumulativeSummary(t, dbc, startMinusOne, "4.19", f.testGamma, f.jobB, f.suiteA, 30, 25, 1)

	_, testIDsCount, err := api.GetJobRunTestsCountByLookbackAt(dbc, 14, lookbackDate)
	require.NoError(t, err)
	assert.Equal(t, int64(2), testIDsCount, "should only count tests from the active release")
}

func TestLookbackCount_ZeroAndNegativeDeltaExcluded(t *testing.T) {
	dbc := lookbackCountDB(t)
	f := seedLookbackFixtures(t, dbc)

	today := lookbackDate
	startMinusOne := today.AddDays(-15)

	// testAlpha has runs in the window (delta > 0).
	intutil.CreateCumulativeSummary(t, dbc, today, "4.18", f.testAlpha, f.jobA, f.suiteA, 100, 90, 5)
	intutil.CreateCumulativeSummary(t, dbc, startMinusOne, "4.18", f.testAlpha, f.jobA, f.suiteA, 80, 70, 3)

	// testBeta has zero delta (same prefix sum at both dates).
	intutil.CreateCumulativeSummary(t, dbc, today, "4.18", f.testBeta, f.jobA, f.suiteA, 50, 45, 2)
	intutil.CreateCumulativeSummary(t, dbc, startMinusOne, "4.18", f.testBeta, f.jobA, f.suiteA, 50, 45, 2)

	// testGamma has negative delta (end prefix sum < start prefix sum).
	intutil.CreateCumulativeSummary(t, dbc, today, "4.18", f.testGamma, f.jobA, f.suiteA, 30, 25, 1)
	intutil.CreateCumulativeSummary(t, dbc, startMinusOne, "4.18", f.testGamma, f.jobA, f.suiteA, 40, 35, 2)

	_, testIDsCount, err := api.GetJobRunTestsCountByLookbackAt(dbc, 14, lookbackDate)
	require.NoError(t, err)
	assert.Equal(t, int64(1), testIDsCount, "only testAlpha (positive delta) should be counted")
}

func TestLookbackCount_NewTestWithNoPriorRow(t *testing.T) {
	dbc := lookbackCountDB(t)
	f := seedLookbackFixtures(t, dbc)

	today := lookbackDate

	// testAlpha has a row at today but no row at startMinusOne.
	// COALESCE treats the missing start row as zero, so the delta
	// equals the full end prefix sum and should be counted.
	intutil.CreateCumulativeSummary(t, dbc, today, "4.18", f.testAlpha, f.jobA, f.suiteA, 10, 8, 1)

	_, testIDsCount, err := api.GetJobRunTestsCountByLookbackAt(dbc, 14, lookbackDate)
	require.NoError(t, err)
	assert.Equal(t, int64(1), testIDsCount, "test with no prior row should be counted")
}

func TestLookbackCount_JobRunsCountExcludesDeleted(t *testing.T) {
	dbc := lookbackCountDB(t)
	seedLookbackFixtures(t, dbc)

	withinWindow := lookbackDate.AddDays(-1).In(time.UTC)
	outsideWindow := lookbackDate.AddDays(-15).In(time.UTC)

	// Two runs within the 14-day window.
	intutil.CreateProwJobRun(t, dbc, 1, "4.18", withinWindow, true, v1.JobSucceeded)
	intutil.CreateProwJobRun(t, dbc, 1, "4.18", withinWindow.Add(-time.Hour), false, v1.JobTestFailure)

	// One run outside the window.
	intutil.CreateProwJobRun(t, dbc, 1, "4.18", outsideWindow, true, v1.JobSucceeded)

	// One soft-deleted run within the window.
	deletedRun := intutil.CreateProwJobRun(t, dbc, 1, "4.18", withinWindow.Add(-2*time.Hour), true, v1.JobSucceeded)
	require.NoError(t, dbc.DB.Delete(&deletedRun).Error)

	jobRunsCount, _, err := api.GetJobRunTestsCountByLookbackAt(dbc, 14, lookbackDate)
	require.NoError(t, err)
	assert.Equal(t, int64(2), jobRunsCount, "should count only non-deleted runs within the window")
}

func TestLookbackCount_MultipleJobsPerTestAggregated(t *testing.T) {
	dbc := lookbackCountDB(t)
	f := seedLookbackFixtures(t, dbc)

	today := lookbackDate
	startMinusOne := today.AddDays(-15)

	// testAlpha has zero delta in jobA but positive delta in jobB,
	// both in the same release. The SUM across jobs should yield a
	// positive total, so testAlpha is counted.
	intutil.CreateCumulativeSummary(t, dbc, today, "4.18", f.testAlpha, f.jobA, f.suiteA, 50, 45, 2)
	intutil.CreateCumulativeSummary(t, dbc, startMinusOne, "4.18", f.testAlpha, f.jobA, f.suiteA, 50, 45, 2)

	intutil.CreateCumulativeSummary(t, dbc, today, "4.18", f.testAlpha, f.jobB, f.suiteA, 30, 25, 1)
	intutil.CreateCumulativeSummary(t, dbc, startMinusOne, "4.18", f.testAlpha, f.jobB, f.suiteA, 20, 18, 0)

	_, testIDsCount, err := api.GetJobRunTestsCountByLookbackAt(dbc, 14, lookbackDate)
	require.NoError(t, err)
	assert.Equal(t, int64(1), testIDsCount, "test with positive aggregate delta across jobs should be counted")
}

func TestLookbackCount_NoReleasesReturnsError(t *testing.T) {
	dbc := lookbackCountDB(t)

	_, _, err := api.GetJobRunTestsCountByLookbackAt(dbc, 14, lookbackDate)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no OCP release found")
}

func TestLookbackCount_InvalidLookbackDays(t *testing.T) {
	dbc := lookbackCountDB(t)

	_, _, err := api.GetJobRunTestsCountByLookbackAt(dbc, 0, lookbackDate)
	assert.Error(t, err)

	_, _, err = api.GetJobRunTestsCountByLookbackAt(dbc, -1, lookbackDate)
	assert.Error(t, err)
}

func TestLookbackCount_NilDatabase(t *testing.T) {
	_, _, err := api.GetJobRunTestsCountByLookbackAt(nil, 14, lookbackDate)
	assert.Error(t, err)
}
