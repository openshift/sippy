package integration

import (
	"context"
	"testing"
	"time"

	"cloud.google.com/go/civil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	processingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db/models"
	dbverify "github.com/openshift/sippy/pkg/db/verify"
	intutil "github.com/openshift/sippy/test/integration/util"
)

func TestVerifyReleaseEnumerationUsesActiveReleaseDefinitions(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	intutil.CreateReleaseDefinition(t, dbc, "4.20", 4, 20)
	intutil.CreateProwJob(t, dbc, "historical-job", "stale-pseudo", nil)
	deleted := intutil.CreateProwJob(t, dbc, "deleted-job", "deleted-pseudo", nil)
	require.NoError(t, dbc.DB.Delete(&deleted).Error)
	intutil.CreateProwJob(t, dbc, "empty-release-job", "", nil)

	store := dbverify.NewPostgreSQL(dbc)
	releases, err := store.Releases(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"4.20"}, releases)

	date := civil.Date{Year: 2026, Month: 8, Day: 25}
	result := (&dbverify.Runner{PostgreSQL: store}).Run(context.Background(), dbverify.Options{
		Date: date, Checks: []dbverify.Check{dbverify.CheckDailyTotals, dbverify.CheckCumulativeSummaries}, Release: "stale-pseudo",
	})
	require.Len(t, result.Summaries, 2)
	for _, summary := range result.Summaries {
		assert.Equal(t, "stale-pseudo", summary.Release)
		assert.Contains(t, summary.Error, "was not found in active release definitions")
	}
}

func TestVerifyPostgreSQLProwStartHalfOpenUTCBoundaries(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	job := intutil.CreateProwJob(t, dbc, "boundary-job", "4.20", nil)
	date := civil.Date{Year: 2026, Month: 8, Day: 25}
	start := date.In(time.UTC)
	end := date.AddDays(1).In(time.UTC)
	before := intutil.CreateProwJobRun(t, dbc, job.ID, "4.20", start.Add(-time.Nanosecond), true, processingv1.JobSucceeded)
	atStart := intutil.CreateProwJobRun(t, dbc, job.ID, "4.20", start, true, processingv1.JobSucceeded)
	atEnd := intutil.CreateProwJobRun(t, dbc, job.ID, "4.20", end, true, processingv1.JobSucceeded)
	insideFromOffset := intutil.CreateProwJobRun(t, dbc, job.ID, "4.20", time.Date(2026, 8, 24, 20, 30, 0, 0, time.FixedZone("minus-four", -4*60*60)), true, processingv1.JobSucceeded)
	otherJob := intutil.CreateProwJob(t, dbc, "other-release-job", "4.19", nil)
	otherRelease := intutil.CreateProwJobRun(t, dbc, otherJob.ID, "4.19", start.Add(time.Hour), true, processingv1.JobSucceeded)

	ids, err := dbverify.NewPostgreSQL(dbc).ProwJobRunIDs(context.Background(), "4.20", start, end)
	require.NoError(t, err)
	assert.Contains(t, ids, dbverify.BuildID(atStart.ID))
	assert.Contains(t, ids, dbverify.BuildID(insideFromOffset.ID))
	assert.NotContains(t, ids, dbverify.BuildID(before.ID))
	assert.NotContains(t, ids, dbverify.BuildID(atEnd.ID))
	assert.NotContains(t, ids, dbverify.BuildID(otherRelease.ID))
}

func TestVerifyDailyRowsProductionSemanticsAndMismatches(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	release := "pseudo-with-data"
	date := civil.Date{Year: 2026, Month: 8, Day: 25}
	start := date.In(time.UTC)
	intutil.CreateReleaseDefinition(t, dbc, release, 4, 20)
	job := intutil.CreateProwJob(t, dbc, "daily-job", release, nil)
	test := intutil.CreateTest(t, dbc, "daily test")
	suite := intutil.CreateSuite(t, dbc, "suite")

	statuses := []int{
		int(processingv1.TestStatusSuccess),
		int(processingv1.TestStatusFailure),
		int(processingv1.TestStatusFlake),
		int(processingv1.TestStatusRunning),
	}
	for i, status := range statuses {
		timestamp := start.Add(time.Duration(i) * time.Hour)
		run := intutil.CreateProwJobRun(t, dbc, job.ID, release, timestamp, status == int(processingv1.TestStatusSuccess), processingv1.JobSucceeded)
		intutil.CreateProwJobRunTest(t, dbc, run.ID, job.ID, test.ID, release, timestamp, status)
	}

	// A distinct suite/lifecycle proves they are both part of the key.
	informingTime := start.Add(5 * time.Hour)
	informingRun := intutil.CreateProwJobRun(t, dbc, job.ID, release, informingTime, true, processingv1.JobSucceeded)
	intutil.CreateProwJobRunTest(t, dbc, informingRun.ID, job.ID, test.ID, release, informingTime, int(processingv1.TestStatusSuccess), intutil.WithSuiteID(suite.ID), intutil.WithLifecycle("informing"))

	// InfraFailure results are excluded.
	infraTime := start.Add(6 * time.Hour)
	infraRun := intutil.CreateProwJobRun(t, dbc, job.ID, release, infraTime, false, processingv1.JobInternalInfrastructureFailure, intutil.WithLabels("InfraFailure"))
	intutil.CreateProwJobRunTest(t, dbc, infraRun.ID, job.ID, test.ID, release, infraTime, int(processingv1.TestStatusFailure))

	// The day is half-open: both joined rows are outside the target day.
	beforeTime := start.Add(-time.Nanosecond)
	beforeRun := intutil.CreateProwJobRun(t, dbc, job.ID, release, beforeTime, true, processingv1.JobSucceeded)
	intutil.CreateProwJobRunTest(t, dbc, beforeRun.ID, job.ID, test.ID, release, beforeTime, int(processingv1.TestStatusSuccess))
	endTime := date.AddDays(1).In(time.UTC)
	endRun := intutil.CreateProwJobRun(t, dbc, job.ID, release, endTime, true, processingv1.JobSucceeded)
	intutil.CreateProwJobRunTest(t, dbc, endRun.ID, job.ID, test.ID, release, endTime, int(processingv1.TestStatusSuccess))

	nullSuite := models.TestDailyTotal{
		Release: release, Date: date, TestID: test.ID, ProwJobID: job.ID, SuiteID: 0, Lifecycle: "blocking",
		Successes: 1, Failures: 1, Flakes: 1, Runs: 4,
	}
	informing := models.TestDailyTotal{
		Release: release, Date: date, TestID: test.ID, ProwJobID: job.ID, SuiteID: suite.ID, Lifecycle: "informing",
		Successes: 1, Runs: 1,
	}
	require.NoError(t, dbc.DB.Create(&nullSuite).Error)
	require.NoError(t, dbc.DB.Create(&informing).Error)

	store := dbverify.NewPostgreSQL(dbc)
	raw, stored, err := store.DailyRows(context.Background(), release, date)
	require.NoError(t, err)
	summary, discrepancies := dbverify.CompareDaily(release, date, raw, stored)
	assert.True(t, summary.Passed)
	assert.Empty(t, discrepancies)
	require.Len(t, raw, 2)
	assert.Equal(t, uint64(0), raw[0].Key.SuiteID, "null suite is normalized to zero")
	runnerResult := (&dbverify.Runner{PostgreSQL: store}).Run(context.Background(), dbverify.Options{
		Date: date, Checks: []dbverify.Check{dbverify.CheckDailyTotals}, Release: release,
	})
	require.Len(t, runnerResult.Summaries, 1)
	assert.True(t, runnerResult.Passed(), "data-bearing pseudo-release is applicable")

	// All four counters are independently detected.
	require.NoError(t, dbc.DB.Model(&nullSuite).Where(
		"release = ? AND date = ? AND test_id = ? AND prow_job_id = ? AND suite_id = ? AND lifecycle = ?",
		nullSuite.Release, nullSuite.Date, nullSuite.TestID, nullSuite.ProwJobID, nullSuite.SuiteID, nullSuite.Lifecycle,
	).Updates(map[string]any{
		"successes": 2, "failures": 2, "flakes": 2, "runs": 5,
	}).Error)
	_, stored, err = store.DailyRows(context.Background(), release, date)
	require.NoError(t, err)
	_, discrepancies = dbverify.CompareDaily(release, date, raw, stored)
	fields := make([]string, 0)
	for _, discrepancy := range discrepancies {
		if discrepancy.Kind == "count-mismatch" {
			fields = append(fields, discrepancy.Field)
		}
	}
	assert.ElementsMatch(t, []string{"successes", "failures", "flakes", "runs"}, fields)

	// Summary-only and raw-only keys are compared in both directions.
	extraSummary := models.TestDailyTotal{
		Release: release, Date: date, TestID: test.ID + 100, ProwJobID: job.ID, SuiteID: 0, Lifecycle: "blocking", Runs: 1,
	}
	require.NoError(t, dbc.DB.Create(&extraSummary).Error)
	rawOnlyTest := intutil.CreateTest(t, dbc, "raw only")
	rawOnlyTime := start.Add(8 * time.Hour)
	rawOnlyRun := intutil.CreateProwJobRun(t, dbc, job.ID, release, rawOnlyTime, true, processingv1.JobSucceeded)
	intutil.CreateProwJobRunTest(t, dbc, rawOnlyRun.ID, job.ID, rawOnlyTest.ID, release, rawOnlyTime, int(processingv1.TestStatusSuccess))
	raw, stored, err = store.DailyRows(context.Background(), release, date)
	require.NoError(t, err)
	_, discrepancies = dbverify.CompareDaily(release, date, raw, stored)
	kinds := make([]string, len(discrepancies))
	for i := range discrepancies {
		kinds[i] = discrepancies[i].Kind
	}
	assert.Contains(t, kinds, "unexpected-row")
	assert.Contains(t, kinds, "missing-row")
}

func TestVerifyCumulativeComparisonStreamsDiscrepancies(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	release := "4.20"
	date := civil.Date{Year: 2026, Month: 8, Day: 25}
	job := intutil.CreateProwJob(t, dbc, "cumulative-job", release, nil)
	suite := intutil.CreateSuite(t, dbc, "cumulative suite")

	createDaily := func(testID, suiteID uint, lifecycle string, counts dbverify.Counts) {
		require.NoError(t, dbc.DB.Create(&models.TestDailyTotal{
			Release: release, Date: date, TestID: testID, ProwJobID: job.ID, SuiteID: suiteID, Lifecycle: lifecycle,
			Successes: fixtureInt32(counts.Successes), Failures: fixtureInt32(counts.Failures),
			Flakes: fixtureInt32(counts.Flakes), Runs: fixtureInt32(counts.Runs),
		}).Error)
	}
	createCumulative := func(targetDate civil.Date, testID, suiteID uint, lifecycle string, counts dbverify.Counts) {
		row := models.TestCumulativeSummary{
			Release: release, Date: targetDate, TestID: testID, ProwJobID: job.ID, SuiteID: suiteID, Lifecycle: lifecycle,
			PrefixSumSuccesses: counts.Successes, PrefixSumFailures: counts.Failures, PrefixSumFlakes: counts.Flakes, PrefixSumRuns: counts.Runs,
		}
		require.NoError(t, dbc.DB.Create(&row).Error)
	}
	comparisonKey := func(testID, suiteID uint, lifecycle string) dbverify.SummaryKey {
		return dbverify.SummaryKey{TestID: uint64(testID), ProwJobID: uint64(job.ID), SuiteID: uint64(suiteID), Lifecycle: lifecycle}
	}

	previousCounts := dbverify.Counts{Successes: 10, Failures: 2, Flakes: 1, Runs: 13}
	dailyCounts := dbverify.Counts{Successes: 2, Failures: 1, Flakes: 1, Runs: 4}
	accumulationTest := intutil.CreateTest(t, dbc, "accumulation")
	createCumulative(date.AddDays(-1), accumulationTest.ID, 0, "accumulation", previousCounts)
	createDaily(accumulationTest.ID, 0, "accumulation", dailyCounts)
	createCumulative(date, accumulationTest.ID, 0, "accumulation", previousCounts.Add(dailyCounts))

	carryTest := intutil.CreateTest(t, dbc, "carry forward")
	carryCounts := dbverify.Counts{Successes: 5, Failures: 1, Runs: 6}
	createCumulative(date.AddDays(-1), carryTest.ID, 0, "blocking", carryCounts)
	createCumulative(date, carryTest.ID, 0, "blocking", carryCounts)

	firstTest := intutil.CreateTest(t, dbc, "first day")
	firstCounts := dbverify.Counts{Successes: 1, Runs: 1}
	createDaily(firstTest.ID, suite.ID, "informing", firstCounts)
	createCumulative(date, firstTest.ID, suite.ID, "informing", firstCounts)

	mismatchTest := intutil.CreateTest(t, dbc, "count mismatch")
	mismatchPrevious := dbverify.Counts{Successes: 4, Failures: 1, Flakes: 2, Runs: 7}
	mismatchDaily := dbverify.Counts{Successes: 1, Failures: 2, Flakes: 3, Runs: 6}
	createCumulative(date.AddDays(-1), mismatchTest.ID, 0, "blocking", mismatchPrevious)
	createDaily(mismatchTest.ID, 0, "blocking", mismatchDaily)
	createCumulative(date, mismatchTest.ID, 0, "blocking", dbverify.Counts{Successes: 4, Failures: 4, Flakes: 6, Runs: 14})

	missingTest := intutil.CreateTest(t, dbc, "missing target")
	createDaily(missingTest.ID, 0, "blocking", dbverify.Counts{Successes: 3, Runs: 3})

	unexpectedTest := intutil.CreateTest(t, dbc, "unexpected target")
	createCumulative(date, unexpectedTest.ID, 0, "blocking", dbverify.Counts{Runs: 1})
	zeroOnlyTest := intutil.CreateTest(t, dbc, "zero-only unexpected target")
	createCumulative(date, zeroOnlyTest.ID, 0, "blocking", dbverify.Counts{})

	store := dbverify.NewPostgreSQL(dbc)
	var differences []dbverify.CumulativeComparisonDiscrepancy
	comparison, err := store.CumulativeComparison(context.Background(), release, date, func(discrepancy dbverify.CumulativeComparisonDiscrepancy) {
		differences = append(differences, discrepancy)
	})
	require.NoError(t, err)
	assert.Equal(t, 5, comparison.ExpectedRows)
	assert.Equal(t, 6, comparison.ActualRows)
	assert.Equal(t, 6, comparison.Discrepancies)
	assert.ElementsMatch(t, []dbverify.CumulativeComparisonDiscrepancy{
		{Key: comparisonKey(missingTest.ID, 0, "blocking"), Kind: "missing-row", Expected: "present", Actual: "missing"},
		{Key: comparisonKey(unexpectedTest.ID, 0, "blocking"), Kind: "unexpected-row", Expected: "missing", Actual: "present"},
		{Key: comparisonKey(mismatchTest.ID, 0, "blocking"), Kind: "count-mismatch", Field: "successes", Expected: "5", Actual: "4"},
		{Key: comparisonKey(mismatchTest.ID, 0, "blocking"), Kind: "count-mismatch", Field: "failures", Expected: "3", Actual: "4"},
		{Key: comparisonKey(mismatchTest.ID, 0, "blocking"), Kind: "count-mismatch", Field: "flakes", Expected: "5", Actual: "6"},
		{Key: comparisonKey(mismatchTest.ID, 0, "blocking"), Kind: "count-mismatch", Field: "runs", Expected: "13", Actual: "14"},
	}, differences)
}

func fixtureInt32(value int64) int32 {
	return int32(value) //nolint:gosec // Test fixtures only use small non-negative counter constants.
}
