package integration

import (
	"context"
	"testing"
	"time"

	"cloud.google.com/go/civil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/openshift/sippy/pkg/dataloader/prowloader/pgwriter"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/infrafailure"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/db/query"
	intutil "github.com/openshift/sippy/test/integration/util"
)

// TestRecordInfraFailureSubtractsFromSummaries writes two runs of the same test
// on the same day, marks one as an infrastructure failure, and verifies its
// contribution is removed from both the daily totals and the cumulative
// summaries while the other run remains counted.
func TestRecordInfraFailureSubtractsFromSummaries(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")
	today := testDate
	ts := time.Date(today.Year, today.Month, today.Day, 10, 0, 0, 0, time.UTC)

	const infraRunID = 40001
	const keepRunID = 40002

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{
		{
			Run: pgwriter.RunRow{ID: infraRunID, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: infraRunID, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "infra-sub-test", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 1.0},
			},
		},
		{
			Run: pgwriter.RunRow{ID: keepRunID, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: keepRunID, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "infra-sub-test", SuiteName: "junit_e2e", Status: statusFailure, Duration: 2.0},
			},
		},
	})

	var test models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "infra-sub-test").First(&test).Error)

	// Precondition: both runs counted (1 success + 1 failure = 2 runs).
	var dt models.TestDailyTotal
	require.NoError(t, dbc.DB.Where("test_id = ? AND prow_job_id = ? AND release = ? AND date = ?", test.ID, jobID, "4.18", today).First(&dt).Error)
	require.Equal(t, int32(1), dt.Successes)
	require.Equal(t, int32(1), dt.Failures)
	require.Equal(t, int32(2), dt.Runs)

	// Record the infra failure for the first run. RecordInfraFailure opens its
	// own transaction on the supplied connection.
	require.NoError(t, infrafailure.RecordInfraFailure(context.Background(), dbc.DB, infraRunID))

	// The InfraFailure label is applied to the infra run (exercising the
	// NULL-safe gate, since the run had no labels).
	var infraRun models.ProwJobRun
	require.NoError(t, dbc.DB.First(&infraRun, infraRunID).Error)
	assert.Contains(t, []string(infraRun.Labels), infrafailure.LabelInfraFailure)

	// Daily totals now reflect only the retained run's failure.
	require.NoError(t, dbc.DB.Where("test_id = ? AND prow_job_id = ? AND release = ? AND date = ?", test.ID, jobID, "4.18", today).First(&dt).Error)
	assert.Equal(t, int32(0), dt.Successes, "infra run's success should be subtracted")
	assert.Equal(t, int32(1), dt.Failures, "retained run's failure should remain")
	assert.Equal(t, int32(1), dt.Runs)

	// Cumulative summaries cascade the subtraction from the affected date onward
	// (today and the carried-forward tomorrow row).
	tomorrow := today.AddDays(1)
	for _, d := range []civil.Date{today, tomorrow} {
		var cs models.TestCumulativeSummary
		require.NoError(t, dbc.DB.Where("test_id = ? AND prow_job_id = ? AND release = ? AND date = ?", test.ID, jobID, "4.18", d).First(&cs).Error, "date %s", d)
		assert.Equal(t, int64(0), cs.PrefixSumSuccesses, "date %s", d)
		assert.Equal(t, int64(1), cs.PrefixSumFailures, "date %s", d)
		assert.Equal(t, int64(1), cs.PrefixSumRuns, "date %s", d)
	}
}

// TestSubtractInfraFailureFromSummaries verifies the exported subtraction entry
// point removes a run's contribution from the summary tables WITHOUT touching the
// InfraFailure label. This is the half of the invariant the
// /api/job/run/labels applier relies on: the applier appends the label itself
// and then calls this to subtract in the same transaction. The run here is never
// labeled, proving the function does only the subtraction.
func TestSubtractInfraFailureFromSummaries(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")
	today := testDate
	ts := time.Date(today.Year, today.Month, today.Day, 10, 0, 0, 0, time.UTC)

	const infraRunID = 48001
	const keepRunID = 48002

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{
		{
			Run: pgwriter.RunRow{ID: infraRunID, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: infraRunID, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "subtract-only-test", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 1.0},
			},
		},
		{
			Run: pgwriter.RunRow{ID: keepRunID, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: keepRunID, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "subtract-only-test", SuiteName: "junit_e2e", Status: statusFailure, Duration: 2.0},
			},
		},
	})

	var test models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "subtract-only-test").First(&test).Error)

	// Precondition: both runs counted (1 success + 1 failure = 2 runs).
	var dt models.TestDailyTotal
	require.NoError(t, dbc.DB.Where("test_id = ? AND prow_job_id = ? AND release = ? AND date = ?", test.ID, jobID, "4.18", today).First(&dt).Error)
	require.Equal(t, int32(1), dt.Successes)
	require.Equal(t, int32(1), dt.Failures)
	require.Equal(t, int32(2), dt.Runs)

	// Subtract the infra run's contribution directly. The function expects a
	// transaction (its temp table is ON COMMIT DROP), so wrap it; unlike
	// RecordInfraFailure it does not set the InfraFailure label.
	require.NoError(t, dbc.DB.Transaction(func(tx *gorm.DB) error {
		return infrafailure.SubtractInfraFailureFromSummaries(tx, infraRunID)
	}))

	// Daily totals now reflect only the retained run's failure.
	require.NoError(t, dbc.DB.Where("test_id = ? AND prow_job_id = ? AND release = ? AND date = ?", test.ID, jobID, "4.18", today).First(&dt).Error)
	assert.Equal(t, int32(0), dt.Successes, "infra run's success should be subtracted")
	assert.Equal(t, int32(1), dt.Failures, "retained run's failure should remain")
	assert.Equal(t, int32(1), dt.Runs)

	// Cumulative summaries cascade the subtraction from the affected date onward
	// (today and the carried-forward tomorrow row).
	tomorrow := today.AddDays(1)
	for _, d := range []civil.Date{today, tomorrow} {
		var cs models.TestCumulativeSummary
		require.NoError(t, dbc.DB.Where("test_id = ? AND prow_job_id = ? AND release = ? AND date = ?", test.ID, jobID, "4.18", d).First(&cs).Error, "date %s", d)
		assert.Equal(t, int64(0), cs.PrefixSumSuccesses, "date %s", d)
		assert.Equal(t, int64(1), cs.PrefixSumFailures, "date %s", d)
		assert.Equal(t, int64(1), cs.PrefixSumRuns, "date %s", d)
	}

	// The function must NOT have set the InfraFailure label: it does only the
	// subtraction and leaves the label to the caller.
	var infraRun models.ProwJobRun
	require.NoError(t, dbc.DB.First(&infraRun, infraRunID).Error)
	assert.NotContains(t, []string(infraRun.Labels), infrafailure.LabelInfraFailure, "SubtractInfraFailureFromSummaries must not set the label")
}

// TestRecordInfraFailureIsIdempotent verifies that repeated calls for the same
// run subtract exactly once and never drive the totals negative or duplicate
// the label.
func TestRecordInfraFailureIsIdempotent(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")
	today := testDate
	ts := time.Date(today.Year, today.Month, today.Day, 10, 0, 0, 0, time.UTC)

	const runID = 41001

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{ID: runID, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
		Tests: []pgwriter.TestRow{
			{ProwJobRunID: runID, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "infra-idem-test", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 1.0},
		},
	}})

	var test models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "infra-idem-test").First(&test).Error)

	call := func() {
		require.NoError(t, infrafailure.RecordInfraFailure(context.Background(), dbc.DB, runID))
	}

	// First call subtracts the single run down to zero.
	call()
	var dt models.TestDailyTotal
	require.NoError(t, dbc.DB.Where("test_id = ? AND prow_job_id = ? AND release = ? AND date = ?", test.ID, jobID, "4.18", today).First(&dt).Error)
	assert.Equal(t, int32(0), dt.Successes)
	assert.Equal(t, int32(0), dt.Runs)

	// Subsequent calls must be no-ops.
	call()
	call()
	require.NoError(t, dbc.DB.Where("test_id = ? AND prow_job_id = ? AND release = ? AND date = ?", test.ID, jobID, "4.18", today).First(&dt).Error)
	assert.Equal(t, int32(0), dt.Successes, "idempotent: no double subtraction")
	assert.Equal(t, int32(0), dt.Runs, "idempotent: totals not driven negative")

	// Cumulative summaries must also settle at zero and stay there across the
	// carried-forward rows (today and tomorrow): the repeated calls must not
	// drive the running totals negative.
	tomorrow := today.AddDays(1)
	for _, d := range []civil.Date{today, tomorrow} {
		var cs models.TestCumulativeSummary
		require.NoError(t, dbc.DB.Where("test_id = ? AND prow_job_id = ? AND release = ? AND date = ?", test.ID, jobID, "4.18", d).First(&cs).Error, "date %s", d)
		assert.Equal(t, int64(0), cs.PrefixSumSuccesses, "idempotent: cumulative successes at zero, date %s", d)
		assert.Equal(t, int64(0), cs.PrefixSumRuns, "idempotent: cumulative runs not driven negative, date %s", d)
	}

	// Label present exactly once.
	var run models.ProwJobRun
	require.NoError(t, dbc.DB.First(&run, runID).Error)
	labelCount := 0
	for _, l := range run.Labels {
		if l == infrafailure.LabelInfraFailure {
			labelCount++
		}
	}
	assert.Equal(t, 1, labelCount, "InfraFailure label should be applied exactly once")
}

// TestCreateBatchDeltasExcludesInfraFailureRuns verifies the write-time
// exclusion: a run that already carries the InfraFailure label when the batch
// is written never contributes to the summary tables.
func TestCreateBatchDeltasExcludesInfraFailureRuns(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")
	today := testDate
	ts := time.Date(today.Year, today.Month, today.Day, 10, 0, 0, 0, time.UTC)

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{
		{
			Run: pgwriter.RunRow{ID: 42001, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts, Labels: []string{infrafailure.LabelInfraFailure}},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: 42001, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "write-exclude-test", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 1.0},
			},
		},
		{
			Run: pgwriter.RunRow{ID: 42002, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: 42002, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "write-exclude-test", SuiteName: "junit_e2e", Status: statusFailure, Duration: 2.0},
			},
		},
	})

	var test models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "write-exclude-test").First(&test).Error)

	var dt models.TestDailyTotal
	require.NoError(t, dbc.DB.Where("test_id = ? AND prow_job_id = ? AND release = ? AND date = ?", test.ID, jobID, "4.18", today).First(&dt).Error)
	// Only the non-infra run (a failure) is counted; the infra-labeled run's
	// success is excluded.
	assert.Equal(t, int32(0), dt.Successes, "infra-labeled run's success should be excluded")
	assert.Equal(t, int32(1), dt.Failures)
	assert.Equal(t, int32(1), dt.Runs)
}

// TestRecordInfraFailureSubtractsFlakes writes two runs whose test flaked on the
// same day, marks one as an infrastructure failure, and verifies the flake
// contribution is removed from both the daily totals and the cumulative
// summaries while the retained run's flake remains counted. Flakes are status 13
// and are counted by a dedicated column, so this exercises the flakes /
// prefix_sum_flakes subtraction path specifically.
func TestRecordInfraFailureSubtractsFlakes(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")
	today := testDate
	ts := time.Date(today.Year, today.Month, today.Day, 10, 0, 0, 0, time.UTC)

	const infraRunID = 43001
	const keepRunID = 43002

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{
		{
			Run: pgwriter.RunRow{ID: infraRunID, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: infraRunID, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "infra-flake-test", SuiteName: "junit_e2e", Status: statusFlake, Duration: 1.0},
			},
		},
		{
			Run: pgwriter.RunRow{ID: keepRunID, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: keepRunID, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "infra-flake-test", SuiteName: "junit_e2e", Status: statusFlake, Duration: 1.0},
			},
		},
	})

	var test models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "infra-flake-test").First(&test).Error)

	// Precondition: both flakes counted (2 flakes = 2 runs, no successes/failures).
	var dt models.TestDailyTotal
	require.NoError(t, dbc.DB.Where("test_id = ? AND prow_job_id = ? AND release = ? AND date = ?", test.ID, jobID, "4.18", today).First(&dt).Error)
	require.Equal(t, int32(2), dt.Flakes)
	require.Equal(t, int32(2), dt.Runs)
	require.Equal(t, int32(0), dt.Successes)
	require.Equal(t, int32(0), dt.Failures)

	require.NoError(t, infrafailure.RecordInfraFailure(context.Background(), dbc.DB, infraRunID))

	// Daily totals now reflect only the retained run's flake.
	require.NoError(t, dbc.DB.Where("test_id = ? AND prow_job_id = ? AND release = ? AND date = ?", test.ID, jobID, "4.18", today).First(&dt).Error)
	assert.Equal(t, int32(1), dt.Flakes, "infra run's flake should be subtracted")
	assert.Equal(t, int32(1), dt.Runs)

	// Cumulative summaries cascade the flake subtraction from the affected date
	// onward (today and the carried-forward tomorrow row).
	tomorrow := today.AddDays(1)
	for _, d := range []civil.Date{today, tomorrow} {
		var cs models.TestCumulativeSummary
		require.NoError(t, dbc.DB.Where("test_id = ? AND prow_job_id = ? AND release = ? AND date = ?", test.ID, jobID, "4.18", d).First(&cs).Error, "date %s", d)
		assert.Equal(t, int64(1), cs.PrefixSumFlakes, "date %s", d)
		assert.Equal(t, int64(1), cs.PrefixSumRuns, "date %s", d)
	}
}

// TestRecordInfraFailureSubtractsEachTestIndependently writes a single run
// containing two different tests, marks the run as an infrastructure failure,
// and verifies each test's daily total row is subtracted on its own rather than
// aggregated into a single combined row. The two tests deliberately land in
// different status columns (one success, one failure) so an accidental
// cross-test aggregation would be visible.
func TestRecordInfraFailureSubtractsEachTestIndependently(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")
	today := testDate
	ts := time.Date(today.Year, today.Month, today.Day, 10, 0, 0, 0, time.UTC)

	const infraRunID = 44001

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{ID: infraRunID, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
		Tests: []pgwriter.TestRow{
			{ProwJobRunID: infraRunID, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "infra-multi-test-a", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 1.0},
			{ProwJobRunID: infraRunID, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "infra-multi-test-b", SuiteName: "junit_e2e", Status: statusFailure, Duration: 2.0},
		},
	}})

	var testA, testB models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "infra-multi-test-a").First(&testA).Error)
	require.NoError(t, dbc.DB.Where("name = ?", "infra-multi-test-b").First(&testB).Error)

	fetch := func(testID uint) models.TestDailyTotal {
		var dt models.TestDailyTotal
		require.NoError(t, dbc.DB.Where("test_id = ? AND prow_job_id = ? AND release = ? AND date = ?", testID, jobID, "4.18", today).First(&dt).Error)
		return dt
	}

	// Precondition: two distinct tests produce two separate rows, not one
	// aggregated row.
	var rows []models.TestDailyTotal
	require.NoError(t, dbc.DB.Where("prow_job_id = ? AND release = ? AND date = ?", jobID, "4.18", today).Find(&rows).Error)
	require.Len(t, rows, 2, "each test must have its own daily total row")
	require.Equal(t, int32(1), fetch(testA.ID).Successes)
	require.Equal(t, int32(1), fetch(testB.ID).Failures)

	require.NoError(t, infrafailure.RecordInfraFailure(context.Background(), dbc.DB, infraRunID))

	// Each test's row is subtracted independently by exactly that test's own
	// contribution from the run.
	dtA := fetch(testA.ID)
	assert.Equal(t, int32(0), dtA.Successes, "test A success subtracted independently")
	assert.Equal(t, int32(0), dtA.Runs)
	assert.Equal(t, int32(0), dtA.Failures)

	dtB := fetch(testB.ID)
	assert.Equal(t, int32(0), dtB.Failures, "test B failure subtracted independently")
	assert.Equal(t, int32(0), dtB.Runs)
	assert.Equal(t, int32(0), dtB.Successes)

	// The cumulative summaries mirror the per-test scoping across the
	// carried-forward rows (today and tomorrow): both tests belonged to the same
	// infra run, so each test's running totals are subtracted to zero on its own.
	tomorrow := today.AddDays(1)
	fetchCum := func(testID uint, d civil.Date) models.TestCumulativeSummary {
		var cs models.TestCumulativeSummary
		require.NoError(t, dbc.DB.Where("test_id = ? AND prow_job_id = ? AND release = ? AND date = ?", testID, jobID, "4.18", d).First(&cs).Error, "test %d date %s", testID, d)
		return cs
	}
	for _, d := range []civil.Date{today, tomorrow} {
		csA := fetchCum(testA.ID, d)
		assert.Equal(t, int64(0), csA.PrefixSumSuccesses, "test A cumulative success subtracted, date %s", d)
		assert.Equal(t, int64(0), csA.PrefixSumRuns, "test A cumulative runs subtracted, date %s", d)

		csB := fetchCum(testB.ID, d)
		assert.Equal(t, int64(0), csB.PrefixSumFailures, "test B cumulative failure subtracted, date %s", d)
		assert.Equal(t, int64(0), csB.PrefixSumRuns, "test B cumulative runs subtracted, date %s", d)
	}
}

// TestRecordInfraFailureScopedBySuite writes two runs whose same-named test ran
// in two different suites, marks only the junit_e2e run as an infrastructure
// failure, and verifies the junit_serial suite's daily total is left untouched.
// The daily total key includes suite_id, so the subtraction must be scoped per
// suite.
func TestRecordInfraFailureScopedBySuite(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")
	today := testDate
	ts := time.Date(today.Year, today.Month, today.Day, 10, 0, 0, 0, time.UTC)

	const e2eRunID = 45001
	const serialRunID = 45002

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{
		{
			Run: pgwriter.RunRow{ID: e2eRunID, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: e2eRunID, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "infra-suite-test", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 1.0},
			},
		},
		{
			Run: pgwriter.RunRow{ID: serialRunID, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: serialRunID, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "infra-suite-test", SuiteName: "junit_serial", Status: statusSuccess, Duration: 1.0},
			},
		},
	})

	var test models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "infra-suite-test").First(&test).Error)
	var suiteE2E, suiteSerial models.Suite
	require.NoError(t, dbc.DB.Where("name = ?", "junit_e2e").First(&suiteE2E).Error)
	require.NoError(t, dbc.DB.Where("name = ?", "junit_serial").First(&suiteSerial).Error)

	fetch := func(suiteID uint) models.TestDailyTotal {
		var dt models.TestDailyTotal
		require.NoError(t, dbc.DB.Where("test_id = ? AND suite_id = ? AND prow_job_id = ? AND release = ? AND date = ?", test.ID, suiteID, jobID, "4.18", today).First(&dt).Error)
		return dt
	}

	// Precondition: each suite has its own row, each with one success.
	require.Equal(t, int32(1), fetch(suiteE2E.ID).Successes)
	require.Equal(t, int32(1), fetch(suiteSerial.ID).Successes)

	// Record the infra failure for the junit_e2e run only.
	require.NoError(t, infrafailure.RecordInfraFailure(context.Background(), dbc.DB, e2eRunID))

	// The junit_e2e suite is subtracted; junit_serial is untouched.
	e2eDT := fetch(suiteE2E.ID)
	assert.Equal(t, int32(0), e2eDT.Successes, "junit_e2e suite should be subtracted")
	assert.Equal(t, int32(0), e2eDT.Runs)

	serialDT := fetch(suiteSerial.ID)
	assert.Equal(t, int32(1), serialDT.Successes, "junit_serial suite must be unaffected")
	assert.Equal(t, int32(1), serialDT.Runs)

	// The cumulative summaries mirror the per-suite scoping across the
	// carried-forward rows (today and tomorrow): junit_e2e is subtracted to zero
	// while junit_serial's running totals remain untouched.
	tomorrow := today.AddDays(1)
	fetchCum := func(suiteID uint, d civil.Date) models.TestCumulativeSummary {
		var cs models.TestCumulativeSummary
		require.NoError(t, dbc.DB.Where("test_id = ? AND suite_id = ? AND prow_job_id = ? AND release = ? AND date = ?", test.ID, suiteID, jobID, "4.18", d).First(&cs).Error, "suite %d date %s", suiteID, d)
		return cs
	}
	for _, d := range []civil.Date{today, tomorrow} {
		e2eCum := fetchCum(suiteE2E.ID, d)
		assert.Equal(t, int64(0), e2eCum.PrefixSumSuccesses, "junit_e2e cumulative subtracted, date %s", d)
		assert.Equal(t, int64(0), e2eCum.PrefixSumRuns, "junit_e2e cumulative subtracted, date %s", d)

		serialCum := fetchCum(suiteSerial.ID, d)
		assert.Equal(t, int64(1), serialCum.PrefixSumSuccesses, "junit_serial cumulative unaffected, date %s", d)
		assert.Equal(t, int64(1), serialCum.PrefixSumRuns, "junit_serial cumulative unaffected, date %s", d)
	}
}

// TestRecordInfraFailureNonexistentRunIsNoOp calls RecordInfraFailure with a run
// ID that does not exist. The conditional label UPDATE matches no rows, so the
// function returns nil without error. This documents that a nonexistent run is
// conflated with an already-labeled run: both leave RowsAffected == 0 and are
// treated as an idempotent no-op.
func TestRecordInfraFailureNonexistentRunIsNoOp(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	const missingRunID = 46001

	// Sanity: the run genuinely does not exist.
	var count int64
	require.NoError(t, dbc.DB.Model(&models.ProwJobRun{}).Where("id = ?", missingRunID).Count(&count).Error)
	require.Equal(t, int64(0), count)

	// No error despite the run not existing (conflated with already-labeled).
	assert.NoError(t, infrafailure.RecordInfraFailure(context.Background(), dbc.DB, missingRunID))

	// The call did not create the run as a side effect.
	require.NoError(t, dbc.DB.Model(&models.ProwJobRun{}).Where("id = ?", missingRunID).Count(&count).Error)
	assert.Equal(t, int64(0), count, "nonexistent run must not be created")
}

// TestRecordInfraFailureScopedByRelease writes two runs of the same test in two
// different releases, marks only the 4.18 run as an infrastructure failure, and
// verifies the 4.19 release's daily total is left untouched. Release is a
// partition key of the summary tables, so the subtraction must be scoped per
// release.
func TestRecordInfraFailureScopedByRelease(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")
	today := testDate
	ts := time.Date(today.Year, today.Month, today.Day, 10, 0, 0, 0, time.UTC)

	const infraRunID = 47001 // release 4.18
	const otherRunID = 47002 // release 4.19

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{
		{
			Run: pgwriter.RunRow{ID: infraRunID, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: infraRunID, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "infra-release-test", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 1.0},
			},
		},
		{
			Run: pgwriter.RunRow{ID: otherRunID, ProwJobID: jobID, ProwJobRelease: "4.19", Timestamp: ts},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: otherRunID, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.19", TestName: "infra-release-test", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 1.0},
			},
		},
	})

	var test models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "infra-release-test").First(&test).Error)

	fetch := func(release string) models.TestDailyTotal {
		var dt models.TestDailyTotal
		require.NoError(t, dbc.DB.Where("test_id = ? AND prow_job_id = ? AND release = ? AND date = ?", test.ID, jobID, release, today).First(&dt).Error)
		return dt
	}

	// Precondition: each release has its own row with one success.
	require.Equal(t, int32(1), fetch("4.18").Successes)
	require.Equal(t, int32(1), fetch("4.19").Successes)

	// Record the infra failure for the 4.18 run only.
	require.NoError(t, infrafailure.RecordInfraFailure(context.Background(), dbc.DB, infraRunID))

	// The 4.18 release is subtracted; 4.19 is untouched.
	dt418 := fetch("4.18")
	assert.Equal(t, int32(0), dt418.Successes, "4.18 release should be subtracted")
	assert.Equal(t, int32(0), dt418.Runs)

	dt419 := fetch("4.19")
	assert.Equal(t, int32(1), dt419.Successes, "4.19 release must be unaffected")
	assert.Equal(t, int32(1), dt419.Runs)

	// The cumulative summaries mirror the per-release scoping across the
	// carried-forward rows (today and tomorrow): 4.18 is subtracted to zero while
	// 4.19's running totals remain untouched.
	tomorrow := today.AddDays(1)
	fetchCum := func(release string, d civil.Date) models.TestCumulativeSummary {
		var cs models.TestCumulativeSummary
		require.NoError(t, dbc.DB.Where("test_id = ? AND prow_job_id = ? AND release = ? AND date = ?", test.ID, jobID, release, d).First(&cs).Error, "release %s date %s", release, d)
		return cs
	}
	for _, d := range []civil.Date{today, tomorrow} {
		cum418 := fetchCum("4.18", d)
		assert.Equal(t, int64(0), cum418.PrefixSumSuccesses, "4.18 cumulative subtracted, date %s", d)
		assert.Equal(t, int64(0), cum418.PrefixSumRuns, "4.18 cumulative subtracted, date %s", d)

		cum419 := fetchCum("4.19", d)
		assert.Equal(t, int64(1), cum419.PrefixSumSuccesses, "4.19 cumulative unaffected, date %s", d)
		assert.Equal(t, int64(1), cum419.PrefixSumRuns, "4.19 cumulative unaffected, date %s", d)
	}
}

// readQueryRelease is the release the read-time query exclusion tests operate on. The
// TestOutputs and TestDurations queries filter on this release across the run, test, and
// output rows, so all seeded rows use it.
const readQueryRelease = "4.18"

// seedReadQueryRun creates a prow job run (optionally InfraFailure-labeled) with a single
// failing test result, its duration, prow job URL, and failure output. The three rows share
// the run timestamp and readQueryRelease because the read-time TestOutputs and TestDurations
// queries join prow_job_runs -> prow_job_run_tests -> prow_job_run_test_outputs on equal
// (timestamp, release), so they must line up for the run to be counted.
func seedReadQueryRun(t *testing.T, dbc *db.DB, jobID, testID uint, ts time.Time, duration float64, url, output string, infra bool) {
	t.Helper()
	runOpts := []intutil.ProwJobRunOption{intutil.WithURL(url)}
	if infra {
		runOpts = append(runOpts, intutil.WithLabels(infrafailure.LabelInfraFailure))
	}
	// OverallResult "F" (JobTestFailure); the read-time queries don't filter on it.
	run := intutil.CreateProwJobRun(t, dbc, jobID, readQueryRelease, ts, false, "F", runOpts...)
	pjrt := intutil.CreateProwJobRunTest(t, dbc, run.ID, jobID, testID, readQueryRelease, ts, statusFailure, intutil.WithDuration(duration))
	intutil.CreateProwJobRunTestOutput(t, dbc, pjrt, output)
}

// recentReadQueryDay returns a fixed time-of-day on a date two days in the past. The
// read-time queries constrain rows to current_date - interval '14' day, so fixtures for
// them must use timestamps relative to the real current date rather than the fixed testDate
// used by the summary-table tests.
func recentReadQueryDay() time.Time {
	base := time.Now().UTC().Add(-2 * 24 * time.Hour)
	return time.Date(base.Year(), base.Month(), base.Day(), 10, 0, 0, 0, time.UTC)
}

// TestTestOutputsExcludesInfraFailureRuns verifies the read-time exclusion in
// query.TestOutputs: two runs of the same failing test are written on the same day, one
// carrying the InfraFailure label, and only the clean run's output is returned.
func TestTestOutputsExcludesInfraFailureRuns(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", readQueryRelease)
	testRec := intutil.CreateTest(t, dbc, "read-exclude-outputs-test")

	day := recentReadQueryDay()
	infraTS := day
	cleanTS := day.Add(1 * time.Hour)

	seedReadQueryRun(t, dbc, jobID, testRec.ID, infraTS, 99.0, "https://prow/infra-run", "infra failure output", true)
	seedReadQueryRun(t, dbc, jobID, testRec.ID, cleanTS, 3.0, "https://prow/clean-run", "clean failure output", false)

	outputs, err := query.TestOutputs(dbc, readQueryRelease, "read-exclude-outputs-test", nil, nil, 10)
	require.NoError(t, err)
	require.Len(t, outputs, 1, "the InfraFailure-labeled run's output must be excluded")
	assert.Equal(t, "https://prow/clean-run", outputs[0].ProwJobURL, "only the clean run's output should be returned")
	assert.Equal(t, "clean failure output", outputs[0].Output)
}

// TestTestDurationsExcludesInfraFailureRuns verifies the read-time exclusion in
// query.TestDurations: two runs of the same test land in the same day's duration bucket, one
// carrying the InfraFailure label, and the returned average reflects only the clean run's
// duration rather than the blended average of both runs.
func TestTestDurationsExcludesInfraFailureRuns(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", readQueryRelease)
	testRec := intutil.CreateTest(t, dbc, "read-exclude-durations-test")

	day := recentReadQueryDay()
	infraTS := day
	cleanTS := day.Add(1 * time.Hour)

	// Both runs share the same UTC date, so without the exclusion the query would return the
	// blended average (99 + 3) / 2 = 51 for that date. With the exclusion only the clean run
	// remains, so the average must be exactly the clean run's 3.0.
	seedReadQueryRun(t, dbc, jobID, testRec.ID, infraTS, 99.0, "https://prow/infra-run", "infra failure output", true)
	seedReadQueryRun(t, dbc, jobID, testRec.ID, cleanTS, 3.0, "https://prow/clean-run", "clean failure output", false)

	durations, err := query.TestDurations(dbc, readQueryRelease, "read-exclude-durations-test", nil, nil)
	require.NoError(t, err)
	require.Len(t, durations, 1, "only the clean run's date bucket should be present")
	dateKey := civil.DateOf(day.UTC())
	require.Contains(t, durations, dateKey)
	assert.InDelta(t, 3.0, durations[dateKey], 1e-9, "duration must reflect only the clean run, not the blended average with the infra run")
}
