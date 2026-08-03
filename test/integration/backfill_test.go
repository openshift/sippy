package integration

import (
	"testing"
	"time"

	"cloud.google.com/go/civil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/cumulativesummary"
	"github.com/openshift/sippy/pkg/db/dailysummary"
	"github.com/openshift/sippy/pkg/db/models"
	intutil "github.com/openshift/sippy/test/integration/util"
)

const (
	statusSuccess = int(v1.TestStatusSuccess)
	statusFailure = int(v1.TestStatusFailure)
	statusFlake   = int(v1.TestStatusFlake)
	statusRunning = int(v1.TestStatusRunning)
)

type dailyTotalOpts struct {
	suiteID              *uint
	lifecycle            string
	lastFailureTimestamp *time.Time
	lastSuccessTimestamp *time.Time
}

type dailyTotalOption func(*dailyTotalOpts)

func withDailySuiteID(id uint) dailyTotalOption {
	return func(o *dailyTotalOpts) { o.suiteID = &id }
}

func withDailyLifecycle(lifecycle string) dailyTotalOption {
	return func(o *dailyTotalOpts) { o.lifecycle = lifecycle }
}

func withDailyLastFailure(ts time.Time) dailyTotalOption {
	return func(o *dailyTotalOpts) { o.lastFailureTimestamp = &ts }
}

func withDailyLastSuccess(ts time.Time) dailyTotalOption {
	return func(o *dailyTotalOpts) { o.lastSuccessTimestamp = &ts }
}

// createDailyTotal seeds a test_daily_totals row directly, bypassing
// dailysummary.Backfill, so cumulative-summary tests can be scoped to just
// that package's behavior.
func createDailyTotal(t *testing.T, dbc *db.DB, date civil.Date, release string, testID, prowJobID uint, successes, failures, flakes, runs int32, options ...dailyTotalOption) {
	t.Helper()
	o := dailyTotalOpts{lifecycle: "blocking"}
	for _, fn := range options {
		fn(&o)
	}
	dt := models.TestDailyTotal{
		TestID:               testID,
		ProwJobID:            prowJobID,
		Release:              release,
		Date:                 date,
		Lifecycle:            o.lifecycle,
		Successes:            successes,
		Failures:             failures,
		Flakes:               flakes,
		Runs:                 runs,
		LastFailureTimestamp: o.lastFailureTimestamp,
		LastSuccessTimestamp: o.lastSuccessTimestamp,
	}
	if o.suiteID != nil {
		dt.SuiteID = *o.suiteID
	}
	require.NoError(t, dbc.DB.Create(&dt).Error, "creating TestDailyTotal")
}

// --- dailysummary.Backfill ---

func TestDailyTotalsBackfill_AggregatesCountsFromRawResults(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	intutil.CreateReleaseDefinition(t, dbc, "4.18", 4, 18)
	job := intutil.CreateProwJob(t, dbc, "periodic-e2e-aws", "4.18", nil)
	ts := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	run := intutil.CreateProwJobRun(t, dbc, job.ID, "4.18", ts, true, v1.JobSucceeded)

	passTest := intutil.CreateTest(t, dbc, "backfill-count-pass")
	failTest := intutil.CreateTest(t, dbc, "backfill-count-fail")
	flakeTest := intutil.CreateTest(t, dbc, "backfill-count-flake")
	intutil.CreateProwJobRunTest(t, dbc, run.ID, job.ID, passTest.ID, "4.18", ts, statusSuccess)
	intutil.CreateProwJobRunTest(t, dbc, run.ID, job.ID, failTest.ID, "4.18", ts, statusFailure)
	intutil.CreateProwJobRunTest(t, dbc, run.ID, job.ID, flakeTest.ID, "4.18", ts, statusFlake)

	day := civil.Date{Year: 2026, Month: 7, Day: 20}
	require.NoError(t, dailysummary.Backfill(dbc, day, day))

	var pass models.TestDailyTotal
	require.NoError(t, dbc.DB.Where("test_id = ?", passTest.ID).First(&pass).Error)
	assert.Equal(t, int32(1), pass.Successes)
	assert.Equal(t, int32(0), pass.Failures)
	assert.Equal(t, int32(0), pass.Flakes)
	assert.Equal(t, int32(1), pass.Runs)

	var fail models.TestDailyTotal
	require.NoError(t, dbc.DB.Where("test_id = ?", failTest.ID).First(&fail).Error)
	assert.Equal(t, int32(0), fail.Successes)
	assert.Equal(t, int32(1), fail.Failures)
	assert.Equal(t, int32(0), fail.Flakes)
	assert.Equal(t, int32(1), fail.Runs)

	var flake models.TestDailyTotal
	require.NoError(t, dbc.DB.Where("test_id = ?", flakeTest.ID).First(&flake).Error)
	assert.Equal(t, int32(0), flake.Successes)
	assert.Equal(t, int32(0), flake.Failures)
	assert.Equal(t, int32(1), flake.Flakes)
	assert.Equal(t, int32(1), flake.Runs)
}

func TestDailyTotalsBackfill_UnrecognizedStatusCountsTowardRunsOnly(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	intutil.CreateReleaseDefinition(t, dbc, "4.18", 4, 18)
	job := intutil.CreateProwJob(t, dbc, "periodic-e2e-aws", "4.18", nil)
	ts := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	run := intutil.CreateProwJobRun(t, dbc, job.ID, "4.18", ts, true, v1.JobSucceeded)
	test := intutil.CreateTest(t, dbc, "unrecognized-status-test")

	// A result outside {Success, Failure, Flake} (e.g. captured mid-run)
	// should still be counted as a run so pass-rate denominators stay
	// accurate, without being miscategorized into any outcome bucket.
	intutil.CreateProwJobRunTest(t, dbc, run.ID, job.ID, test.ID, "4.18", ts, statusRunning)

	day := civil.Date{Year: 2026, Month: 7, Day: 20}
	require.NoError(t, dailysummary.Backfill(dbc, day, day))

	var dt models.TestDailyTotal
	require.NoError(t, dbc.DB.Where("test_id = ?", test.ID).First(&dt).Error)
	assert.Equal(t, int32(0), dt.Successes)
	assert.Equal(t, int32(0), dt.Failures)
	assert.Equal(t, int32(0), dt.Flakes)
	assert.Equal(t, int32(1), dt.Runs, "an uncategorized result should still count toward total runs")
}

func TestDailyTotalsBackfill_ScopedBySuite(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	intutil.CreateReleaseDefinition(t, dbc, "4.18", 4, 18)
	job := intutil.CreateProwJob(t, dbc, "periodic-e2e-aws", "4.18", nil)
	suiteA := intutil.CreateSuite(t, dbc, "suite-a")
	suiteB := intutil.CreateSuite(t, dbc, "suite-b")
	test := intutil.CreateTest(t, dbc, "suite-scoped-test")
	ts := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	run := intutil.CreateProwJobRun(t, dbc, job.ID, "4.18", ts, true, v1.JobSucceeded)

	// The same test, job, release, and day, but run as part of two
	// different suites - each suite should get its own daily total row
	// rather than being merged into one.
	intutil.CreateProwJobRunTest(t, dbc, run.ID, job.ID, test.ID, "4.18", ts, statusSuccess, intutil.WithSuiteID(suiteA.ID))
	intutil.CreateProwJobRunTest(t, dbc, run.ID, job.ID, test.ID, "4.18", ts, statusFailure, intutil.WithSuiteID(suiteB.ID))

	day := civil.Date{Year: 2026, Month: 7, Day: 20}
	require.NoError(t, dailysummary.Backfill(dbc, day, day))

	var totals []models.TestDailyTotal
	require.NoError(t, dbc.DB.Where("test_id = ?", test.ID).Find(&totals).Error)
	require.Len(t, totals, 2)

	bySuite := make(map[uint]models.TestDailyTotal)
	for _, dt := range totals {
		bySuite[dt.SuiteID] = dt
	}
	assert.Equal(t, int32(1), bySuite[suiteA.ID].Successes)
	assert.Equal(t, int32(0), bySuite[suiteA.ID].Failures)
	assert.Equal(t, int32(0), bySuite[suiteB.ID].Successes)
	assert.Equal(t, int32(1), bySuite[suiteB.ID].Failures)
}

func TestDailyTotalsBackfill_ScopedByLifecycle(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	intutil.CreateReleaseDefinition(t, dbc, "4.18", 4, 18)
	job := intutil.CreateProwJob(t, dbc, "periodic-e2e-aws", "4.18", nil)
	test := intutil.CreateTest(t, dbc, "lifecycle-scoped-test")
	ts := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	run := intutil.CreateProwJobRun(t, dbc, job.ID, "4.18", ts, true, v1.JobSucceeded)

	// The same test, job, release, and day, but one run tagged blocking
	// and the other informing - they must not be aggregated together.
	intutil.CreateProwJobRunTest(t, dbc, run.ID, job.ID, test.ID, "4.18", ts, statusSuccess, intutil.WithLifecycle("blocking"))
	intutil.CreateProwJobRunTest(t, dbc, run.ID, job.ID, test.ID, "4.18", ts, statusFailure, intutil.WithLifecycle("informing"))

	day := civil.Date{Year: 2026, Month: 7, Day: 20}
	require.NoError(t, dailysummary.Backfill(dbc, day, day))

	var totals []models.TestDailyTotal
	require.NoError(t, dbc.DB.Where("test_id = ?", test.ID).Find(&totals).Error)
	require.Len(t, totals, 2)

	byLifecycle := make(map[string]models.TestDailyTotal)
	for _, dt := range totals {
		byLifecycle[dt.Lifecycle] = dt
	}
	assert.Equal(t, int32(1), byLifecycle["blocking"].Successes)
	assert.Equal(t, int32(0), byLifecycle["blocking"].Failures)
	assert.Equal(t, int32(0), byLifecycle["informing"].Successes)
	assert.Equal(t, int32(1), byLifecycle["informing"].Failures)
}

func TestDailyTotalsBackfill_ScopedByRelease(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	intutil.CreateReleaseDefinition(t, dbc, "4.18", 4, 18)
	intutil.CreateReleaseDefinition(t, dbc, "4.17", 4, 17)
	job18 := intutil.CreateProwJob(t, dbc, "periodic-e2e-aws-418", "4.18", nil)
	job17 := intutil.CreateProwJob(t, dbc, "periodic-e2e-aws-417", "4.17", nil)
	ts := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	run18 := intutil.CreateProwJobRun(t, dbc, job18.ID, "4.18", ts, true, v1.JobSucceeded)
	run17 := intutil.CreateProwJobRun(t, dbc, job17.ID, "4.17", ts, false, v1.JobTestFailure)
	test := intutil.CreateTest(t, dbc, "release-scoped-test")

	intutil.CreateProwJobRunTest(t, dbc, run18.ID, job18.ID, test.ID, "4.18", ts, statusSuccess)
	intutil.CreateProwJobRunTest(t, dbc, run17.ID, job17.ID, test.ID, "4.17", ts, statusFailure)

	day := civil.Date{Year: 2026, Month: 7, Day: 20}
	require.NoError(t, dailysummary.Backfill(dbc, day, day))

	var totals []models.TestDailyTotal
	require.NoError(t, dbc.DB.Where("test_id = ?", test.ID).Find(&totals).Error)
	require.Len(t, totals, 2)

	byRelease := make(map[string]models.TestDailyTotal)
	for _, dt := range totals {
		byRelease[dt.Release] = dt
	}
	assert.Equal(t, int32(1), byRelease["4.18"].Successes)
	assert.Equal(t, int32(0), byRelease["4.18"].Failures)
	assert.Equal(t, int32(0), byRelease["4.17"].Successes)
	assert.Equal(t, int32(1), byRelease["4.17"].Failures)
}

func TestDailyTotalsBackfill_MultiDayRangeProducesOneRowPerDay(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	intutil.CreateReleaseDefinition(t, dbc, "4.18", 4, 18)
	job := intutil.CreateProwJob(t, dbc, "periodic-e2e-aws", "4.18", nil)
	test := intutil.CreateTest(t, dbc, "multi-day-test")

	day1Ts := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	day2Ts := time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC)
	run1 := intutil.CreateProwJobRun(t, dbc, job.ID, "4.18", day1Ts, true, v1.JobSucceeded)
	run2 := intutil.CreateProwJobRun(t, dbc, job.ID, "4.18", day2Ts, false, v1.JobTestFailure)
	intutil.CreateProwJobRunTest(t, dbc, run1.ID, job.ID, test.ID, "4.18", day1Ts, statusSuccess)
	intutil.CreateProwJobRunTest(t, dbc, run2.ID, job.ID, test.ID, "4.18", day2Ts, statusFailure)

	start := civil.Date{Year: 2026, Month: 7, Day: 20}
	end := civil.Date{Year: 2026, Month: 7, Day: 21}
	require.NoError(t, dailysummary.Backfill(dbc, start, end))

	var totals []models.TestDailyTotal
	require.NoError(t, dbc.DB.Where("test_id = ?", test.ID).Order("date").Find(&totals).Error)
	require.Len(t, totals, 2)
	assert.Equal(t, start, totals[0].Date)
	assert.Equal(t, int32(1), totals[0].Successes)
	assert.Equal(t, end, totals[1].Date)
	assert.Equal(t, int32(1), totals[1].Failures)
}

func TestDailyTotalsBackfill_TracksFirstAndLastFailureTimestamps(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	intutil.CreateReleaseDefinition(t, dbc, "4.18", 4, 18)
	job := intutil.CreateProwJob(t, dbc, "periodic-e2e-aws", "4.18", nil)
	test := intutil.CreateTest(t, dbc, "failure-timestamp-test")

	early := time.Date(2026, 7, 20, 6, 0, 0, 0, time.UTC)
	late := time.Date(2026, 7, 20, 18, 0, 0, 0, time.UTC)
	run1 := intutil.CreateProwJobRun(t, dbc, job.ID, "4.18", early, false, v1.JobTestFailure)
	run2 := intutil.CreateProwJobRun(t, dbc, job.ID, "4.18", late, false, v1.JobTestFailure)
	intutil.CreateProwJobRunTest(t, dbc, run1.ID, job.ID, test.ID, "4.18", early, statusFailure)
	intutil.CreateProwJobRunTest(t, dbc, run2.ID, job.ID, test.ID, "4.18", late, statusFailure)

	day := civil.Date{Year: 2026, Month: 7, Day: 20}
	require.NoError(t, dailysummary.Backfill(dbc, day, day))

	var dt models.TestDailyTotal
	require.NoError(t, dbc.DB.Where("test_id = ?", test.ID).First(&dt).Error)
	require.NotNil(t, dt.FirstFailureTimestamp)
	require.NotNil(t, dt.LastFailureTimestamp)
	assert.True(t, dt.FirstFailureTimestamp.Equal(early))
	assert.True(t, dt.LastFailureTimestamp.Equal(late))
}

func TestDailyTotalsBackfill_TracksFirstAndLastSuccessTimestamps(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	intutil.CreateReleaseDefinition(t, dbc, "4.18", 4, 18)
	job := intutil.CreateProwJob(t, dbc, "periodic-e2e-aws", "4.18", nil)
	test := intutil.CreateTest(t, dbc, "success-timestamp-test")

	early := time.Date(2026, 7, 20, 6, 0, 0, 0, time.UTC)
	late := time.Date(2026, 7, 20, 18, 0, 0, 0, time.UTC)
	run1 := intutil.CreateProwJobRun(t, dbc, job.ID, "4.18", early, true, v1.JobSucceeded)
	run2 := intutil.CreateProwJobRun(t, dbc, job.ID, "4.18", late, true, v1.JobSucceeded)
	intutil.CreateProwJobRunTest(t, dbc, run1.ID, job.ID, test.ID, "4.18", early, statusSuccess)
	intutil.CreateProwJobRunTest(t, dbc, run2.ID, job.ID, test.ID, "4.18", late, statusSuccess)

	day := civil.Date{Year: 2026, Month: 7, Day: 20}
	require.NoError(t, dailysummary.Backfill(dbc, day, day))

	var dt models.TestDailyTotal
	require.NoError(t, dbc.DB.Where("test_id = ?", test.ID).First(&dt).Error)
	require.NotNil(t, dt.FirstSuccessTimestamp)
	require.NotNil(t, dt.LastSuccessTimestamp)
	assert.True(t, dt.FirstSuccessTimestamp.Equal(early))
	assert.True(t, dt.LastSuccessTimestamp.Equal(late))
}

func TestDailyTotalsBackfill_RerunAfterRawDataRemovedDropsStaleRow(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	intutil.CreateReleaseDefinition(t, dbc, "4.18", 4, 18)
	job := intutil.CreateProwJob(t, dbc, "periodic-e2e-aws", "4.18", nil)
	ts := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	run := intutil.CreateProwJobRun(t, dbc, job.ID, "4.18", ts, false, v1.JobTestFailure)
	test := intutil.CreateTest(t, dbc, "reprocessed-away-test")
	pjrt := intutil.CreateProwJobRunTest(t, dbc, run.ID, job.ID, test.ID, "4.18", ts, statusFailure)

	day := civil.Date{Year: 2026, Month: 7, Day: 20}
	require.NoError(t, dailysummary.Backfill(dbc, day, day))

	var before models.TestDailyTotal
	require.NoError(t, dbc.DB.Where("test_id = ?", test.ID).First(&before).Error)
	assert.Equal(t, int32(1), before.Failures)

	// Simulate the underlying job run being reprocessed and removed
	// entirely - the raw result this daily total was built from is gone.
	// ON CONFLICT DO UPDATE alone could never notice this; the cleanup
	// delete is what removes the now-unsupported row.
	require.NoError(t, dbc.DB.Unscoped().Delete(&pjrt).Error)

	require.NoError(t, dailysummary.Backfill(dbc, day, day))

	var count int64
	require.NoError(t, dbc.DB.Model(&models.TestDailyTotal{}).Where("test_id = ?", test.ID).Count(&count).Error)
	assert.Equal(t, int64(0), count, "stale daily total should be removed, not left behind, once its source data is gone")
}

func TestDailyTotalsBackfill_RerunAfterLifecycleReclassificationMovesRowToNewKey(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	intutil.CreateReleaseDefinition(t, dbc, "4.18", 4, 18)
	job := intutil.CreateProwJob(t, dbc, "periodic-e2e-aws", "4.18", nil)
	ts := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	run := intutil.CreateProwJobRun(t, dbc, job.ID, "4.18", ts, false, v1.JobTestFailure)
	test := intutil.CreateTest(t, dbc, "reclassified-test")
	pjrt := intutil.CreateProwJobRunTest(t, dbc, run.ID, job.ID, test.ID, "4.18", ts, statusFailure, intutil.WithLifecycle("blocking"))

	day := civil.Date{Year: 2026, Month: 7, Day: 20}
	require.NoError(t, dailysummary.Backfill(dbc, day, day))

	var blockingBefore models.TestDailyTotal
	require.NoError(t, dbc.DB.Where("test_id = ? AND lifecycle = ?", test.ID, "blocking").First(&blockingBefore).Error)
	assert.Equal(t, int32(1), blockingBefore.Failures)

	// The test is reclassified from blocking to informing (e.g. the
	// classification logic is refined) and the raw row is updated in
	// place to reflect it.
	require.NoError(t, dbc.DB.Model(&pjrt).Update("lifecycle", "informing").Error)

	require.NoError(t, dailysummary.Backfill(dbc, day, day))

	var blockingCount int64
	require.NoError(t, dbc.DB.Model(&models.TestDailyTotal{}).Where("test_id = ? AND lifecycle = ?", test.ID, "blocking").Count(&blockingCount).Error)
	assert.Equal(t, int64(0), blockingCount, "the old blocking-keyed row should be cleaned up, not left behind")

	var informing models.TestDailyTotal
	require.NoError(t, dbc.DB.Where("test_id = ? AND lifecycle = ?", test.ID, "informing").First(&informing).Error)
	assert.Equal(t, int32(1), informing.Failures)
}

// --- cumulativesummary.Backfill ---

func TestCumulativeSummariesBackfill_FirstDayEqualsThatDaysTotals(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	intutil.CreateReleaseDefinition(t, dbc, "4.18", 4, 18)
	job := intutil.CreateProwJob(t, dbc, "periodic-e2e-aws", "4.18", nil)
	test := intutil.CreateTest(t, dbc, "first-day-cumulative-test")

	day := civil.Date{Year: 2026, Month: 7, Day: 20}
	createDailyTotal(t, dbc, day, "4.18", test.ID, job.ID, 3, 1, 0, 4)

	require.NoError(t, cumulativesummary.Backfill(dbc, day, day))

	var cs models.TestCumulativeSummary
	require.NoError(t, dbc.DB.Where("date = ? AND test_id = ?", day, test.ID).First(&cs).Error)
	assert.Equal(t, int64(3), cs.PrefixSumSuccesses)
	assert.Equal(t, int64(1), cs.PrefixSumFailures)
	assert.Equal(t, int64(4), cs.PrefixSumRuns)
}

func TestCumulativeSummariesBackfill_AccumulatesOnTopOfPreviousDay(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	intutil.CreateReleaseDefinition(t, dbc, "4.18", 4, 18)
	job := intutil.CreateProwJob(t, dbc, "periodic-e2e-aws", "4.18", nil)
	test := intutil.CreateTest(t, dbc, "accumulate-test")

	yesterday := civil.Date{Year: 2026, Month: 7, Day: 19}
	today := civil.Date{Year: 2026, Month: 7, Day: 20}

	createCumulativeSummary(t, dbc, yesterday, "4.18", test.ID, job.ID, 0, 10, 8, 0, withFailures(2))
	createDailyTotal(t, dbc, today, "4.18", test.ID, job.ID, 3, 1, 0, 4)

	require.NoError(t, cumulativesummary.Backfill(dbc, today, today))

	var cs models.TestCumulativeSummary
	require.NoError(t, dbc.DB.Where("date = ? AND test_id = ?", today, test.ID).First(&cs).Error)
	assert.Equal(t, int64(11), cs.PrefixSumSuccesses)
	assert.Equal(t, int64(3), cs.PrefixSumFailures)
	assert.Equal(t, int64(14), cs.PrefixSumRuns)
}

func TestCumulativeSummariesBackfill_CarriesForwardWhenNoNewDataToday(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	intutil.CreateReleaseDefinition(t, dbc, "4.18", 4, 18)
	job := intutil.CreateProwJob(t, dbc, "periodic-e2e-aws", "4.18", nil)
	test := intutil.CreateTest(t, dbc, "carry-forward-test")

	yesterday := civil.Date{Year: 2026, Month: 7, Day: 19}
	today := civil.Date{Year: 2026, Month: 7, Day: 20}
	lastFailure := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)

	createCumulativeSummary(t, dbc, yesterday, "4.18", test.ID, job.ID, 0, 6, 5, 0,
		withFailures(1), withLastFailure(lastFailure))
	// No daily total for `today` - the test simply didn't run today.

	require.NoError(t, cumulativesummary.Backfill(dbc, today, today))

	var cs models.TestCumulativeSummary
	require.NoError(t, dbc.DB.Where("date = ? AND test_id = ?", today, test.ID).First(&cs).Error)
	assert.Equal(t, int64(5), cs.PrefixSumSuccesses)
	assert.Equal(t, int64(1), cs.PrefixSumFailures)
	assert.Equal(t, int64(6), cs.PrefixSumRuns)
	require.NotNil(t, cs.PrefixMaxLastFailure, "last-failure timestamp should carry forward along with the counts")
	assert.True(t, cs.PrefixMaxLastFailure.Equal(lastFailure))
}

func TestCumulativeSummariesBackfill_NewTestAppearsAlongsideExistingAccumulation(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	intutil.CreateReleaseDefinition(t, dbc, "4.18", 4, 18)
	job := intutil.CreateProwJob(t, dbc, "periodic-e2e-aws", "4.18", nil)
	existing := intutil.CreateTest(t, dbc, "already-running-test")
	newTest := intutil.CreateTest(t, dbc, "newly-added-test")

	yesterday := civil.Date{Year: 2026, Month: 7, Day: 19}
	today := civil.Date{Year: 2026, Month: 7, Day: 20}

	createCumulativeSummary(t, dbc, yesterday, "4.18", existing.ID, job.ID, 0, 4, 4, 0)
	// `existing` keeps running today; `newTest` shows up for the first time.
	createDailyTotal(t, dbc, today, "4.18", existing.ID, job.ID, 1, 0, 0, 1)
	createDailyTotal(t, dbc, today, "4.18", newTest.ID, job.ID, 0, 1, 0, 1)

	require.NoError(t, cumulativesummary.Backfill(dbc, today, today))

	var summaries []models.TestCumulativeSummary
	require.NoError(t, dbc.DB.Where("date = ?", today).Find(&summaries).Error)
	require.Len(t, summaries, 2)

	byTest := make(map[uint]models.TestCumulativeSummary)
	for _, cs := range summaries {
		byTest[cs.TestID] = cs
	}
	assert.Equal(t, int64(5), byTest[existing.ID].PrefixSumRuns, "existing test should accumulate on top of yesterday's total")
	assert.Equal(t, int64(1), byTest[newTest.ID].PrefixSumRuns, "brand new test should start from just today's total, not inherit another test's history")
	assert.Equal(t, int64(1), byTest[newTest.ID].PrefixSumFailures)
}

func TestCumulativeSummariesBackfill_ScopedByRelease(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	intutil.CreateReleaseDefinition(t, dbc, "4.18", 4, 18)
	intutil.CreateReleaseDefinition(t, dbc, "4.17", 4, 17)
	job18 := intutil.CreateProwJob(t, dbc, "job-418", "4.18", nil)
	job17 := intutil.CreateProwJob(t, dbc, "job-417", "4.17", nil)
	test := intutil.CreateTest(t, dbc, "release-scoped-cumulative-test")

	day := civil.Date{Year: 2026, Month: 7, Day: 20}
	createDailyTotal(t, dbc, day, "4.18", test.ID, job18.ID, 2, 0, 0, 2)
	createDailyTotal(t, dbc, day, "4.17", test.ID, job17.ID, 0, 5, 0, 5)

	require.NoError(t, cumulativesummary.Backfill(dbc, day, day))

	var summaries []models.TestCumulativeSummary
	require.NoError(t, dbc.DB.Where("test_id = ?", test.ID).Find(&summaries).Error)
	require.Len(t, summaries, 2)
	byRelease := make(map[string]models.TestCumulativeSummary)
	for _, cs := range summaries {
		byRelease[cs.Release] = cs
	}
	assert.Equal(t, int64(2), byRelease["4.18"].PrefixSumSuccesses)
	assert.Equal(t, int64(0), byRelease["4.18"].PrefixSumFailures)
	assert.Equal(t, int64(0), byRelease["4.17"].PrefixSumSuccesses)
	assert.Equal(t, int64(5), byRelease["4.17"].PrefixSumFailures)
}

func TestCumulativeSummariesBackfill_MultiDayRangeAccumulatesInSequence(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	intutil.CreateReleaseDefinition(t, dbc, "4.18", 4, 18)
	job := intutil.CreateProwJob(t, dbc, "periodic-e2e-aws", "4.18", nil)
	test := intutil.CreateTest(t, dbc, "multi-day-cumulative-test")

	day1 := civil.Date{Year: 2026, Month: 7, Day: 20}
	day2 := civil.Date{Year: 2026, Month: 7, Day: 21}
	day3 := civil.Date{Year: 2026, Month: 7, Day: 22}
	createDailyTotal(t, dbc, day1, "4.18", test.ID, job.ID, 1, 0, 0, 1)
	createDailyTotal(t, dbc, day2, "4.18", test.ID, job.ID, 0, 1, 0, 1)
	createDailyTotal(t, dbc, day3, "4.18", test.ID, job.ID, 0, 0, 1, 1)

	require.NoError(t, cumulativesummary.Backfill(dbc, day1, day3))

	var summaries []models.TestCumulativeSummary
	require.NoError(t, dbc.DB.Where("test_id = ?", test.ID).Order("date").Find(&summaries).Error)
	require.Len(t, summaries, 3)
	assert.Equal(t, int64(1), summaries[0].PrefixSumRuns)
	assert.Equal(t, int64(2), summaries[1].PrefixSumRuns)
	assert.Equal(t, int64(3), summaries[2].PrefixSumRuns)
	assert.Equal(t, int64(1), summaries[2].PrefixSumFlakes)
}

func TestCumulativeSummariesBackfill_TracksLatestFailureTimestamp(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	intutil.CreateReleaseDefinition(t, dbc, "4.18", 4, 18)
	job := intutil.CreateProwJob(t, dbc, "periodic-e2e-aws", "4.18", nil)
	test := intutil.CreateTest(t, dbc, "cumulative-timestamp-test")

	yesterday := civil.Date{Year: 2026, Month: 7, Day: 19}
	today := civil.Date{Year: 2026, Month: 7, Day: 20}
	earlierFailure := time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC)
	laterFailure := time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC)

	createCumulativeSummary(t, dbc, yesterday, "4.18", test.ID, job.ID, 0, 0, 0, 0, withLastFailure(earlierFailure))
	createDailyTotal(t, dbc, today, "4.18", test.ID, job.ID, 0, 1, 0, 1, withDailyLastFailure(laterFailure))

	require.NoError(t, cumulativesummary.Backfill(dbc, today, today))

	var cs models.TestCumulativeSummary
	require.NoError(t, dbc.DB.Where("date = ? AND test_id = ?", today, test.ID).First(&cs).Error)
	require.NotNil(t, cs.PrefixMaxLastFailure)
	assert.True(t, cs.PrefixMaxLastFailure.Equal(laterFailure))
}

func TestCumulativeSummariesBackfill_TracksLatestSuccessTimestamp(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	intutil.CreateReleaseDefinition(t, dbc, "4.18", 4, 18)
	job := intutil.CreateProwJob(t, dbc, "periodic-e2e-aws", "4.18", nil)
	test := intutil.CreateTest(t, dbc, "cumulative-success-timestamp-test")

	yesterday := civil.Date{Year: 2026, Month: 7, Day: 19}
	today := civil.Date{Year: 2026, Month: 7, Day: 20}
	earlierSuccess := time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC)
	laterSuccess := time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC)

	createCumulativeSummary(t, dbc, yesterday, "4.18", test.ID, job.ID, 0, 0, 0, 0, withLastSuccess(earlierSuccess))
	createDailyTotal(t, dbc, today, "4.18", test.ID, job.ID, 1, 0, 0, 1, withDailyLastSuccess(laterSuccess))

	require.NoError(t, cumulativesummary.Backfill(dbc, today, today))

	var cs models.TestCumulativeSummary
	require.NoError(t, dbc.DB.Where("date = ? AND test_id = ?", today, test.ID).First(&cs).Error)
	require.NotNil(t, cs.PrefixMaxLastSuccess)
	assert.True(t, cs.PrefixMaxLastSuccess.Equal(laterSuccess))
}

func TestCumulativeSummariesBackfill_SuiteChangeStartsAnIndependentChain(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	intutil.CreateReleaseDefinition(t, dbc, "4.18", 4, 18)
	job := intutil.CreateProwJob(t, dbc, "periodic-e2e-aws", "4.18", nil)
	test := intutil.CreateTest(t, dbc, "suite-change-test")
	suiteA := intutil.CreateSuite(t, dbc, "suite-a-cumulative")
	suiteB := intutil.CreateSuite(t, dbc, "suite-b-cumulative")

	day1 := civil.Date{Year: 2026, Month: 7, Day: 20}
	day2 := civil.Date{Year: 2026, Month: 7, Day: 21}
	// The test runs under suite A on day 1, then is reclassified under
	// suite B on day 2 (and no longer reports under suite A). The
	// cumulative summary is keyed by (test, job, suite, lifecycle), so
	// suite A's chain keeps carrying forward unchanged on day 2 (same as
	// any test that simply stopped reporting) alongside a brand new
	// chain for suite B - they never merge into one row.
	createDailyTotal(t, dbc, day1, "4.18", test.ID, job.ID, 1, 0, 0, 1, withDailySuiteID(suiteA.ID))
	createDailyTotal(t, dbc, day2, "4.18", test.ID, job.ID, 0, 1, 0, 1, withDailySuiteID(suiteB.ID))

	require.NoError(t, cumulativesummary.Backfill(dbc, day1, day2))

	var summaries []models.TestCumulativeSummary
	require.NoError(t, dbc.DB.Where("test_id = ?", test.ID).Order("date, suite_id").Find(&summaries).Error)
	require.Len(t, summaries, 3, "suite A's day-1 row, its carried-forward day-2 row, and suite B's fresh day-2 row")

	var day1SuiteA, day2SuiteA, day2SuiteB models.TestCumulativeSummary
	for _, cs := range summaries {
		switch {
		case cs.Date == day1 && cs.SuiteID == suiteA.ID:
			day1SuiteA = cs
		case cs.Date == day2 && cs.SuiteID == suiteA.ID:
			day2SuiteA = cs
		case cs.Date == day2 && cs.SuiteID == suiteB.ID:
			day2SuiteB = cs
		default:
			t.Fatalf("unexpected cumulative summary row: %+v", cs)
		}
	}
	assert.Equal(t, int64(1), day1SuiteA.PrefixSumSuccesses)
	assert.Equal(t, int64(1), day2SuiteA.PrefixSumSuccesses, "suite A's chain should carry forward unchanged, not disappear because the suite reassignment happened")
	assert.Equal(t, int64(0), day2SuiteB.PrefixSumSuccesses, "suite B's chain should start fresh, not inherit suite A's history")
	assert.Equal(t, int64(1), day2SuiteB.PrefixSumFailures)
}

func TestCumulativeSummariesBackfill_LifecycleReclassificationDropsOrphanedRow(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	intutil.CreateReleaseDefinition(t, dbc, "4.18", 4, 18)
	job := intutil.CreateProwJob(t, dbc, "periodic-e2e-aws", "4.18", nil)
	testA := intutil.CreateTest(t, dbc, "cumulative-lifecycle-a")
	testB := intutil.CreateTest(t, dbc, "cumulative-lifecycle-b")

	day := civil.Date{Year: 2026, Month: 7, Day: 20}
	createDailyTotal(t, dbc, day, "4.18", testA.ID, job.ID, 1, 0, 0, 1, withDailyLifecycle("blocking"))
	require.NoError(t, cumulativesummary.Backfill(dbc, day, day))

	var countBefore int64
	require.NoError(t, dbc.DB.Model(&models.TestCumulativeSummary{}).Where("test_id = ?", testA.ID).Count(&countBefore).Error)
	require.Equal(t, int64(1), countBefore)

	// testA's daily total on this day is reprocessed away entirely (e.g.
	// dailysummary's own cleanup delete removed it after a reclassification
	// with no history to carry forward from), and testB's total appears
	// in its place. Since this cumulative row for testA has neither a
	// prior day to carry forward from nor a daily total to support it
	// anymore, it should be cleaned up rather than left behind forever.
	require.NoError(t, dbc.DB.Where("test_id = ? AND release = ? AND date = ?", testA.ID, "4.18", day).Delete(&models.TestDailyTotal{}).Error)
	createDailyTotal(t, dbc, day, "4.18", testB.ID, job.ID, 1, 0, 0, 1, withDailyLifecycle("blocking"))

	require.NoError(t, cumulativesummary.Backfill(dbc, day, day))

	var countA int64
	require.NoError(t, dbc.DB.Model(&models.TestCumulativeSummary{}).Where("test_id = ?", testA.ID).Count(&countA).Error)
	assert.Equal(t, int64(0), countA, "orphaned cumulative summary for the removed test should be gone")

	var countB int64
	require.NoError(t, dbc.DB.Model(&models.TestCumulativeSummary{}).Where("test_id = ?", testB.ID).Count(&countB).Error)
	assert.Equal(t, int64(1), countB)
}
