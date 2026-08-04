package integration

import (
	"context"
	"testing"
	"time"

	"cloud.google.com/go/civil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openshift/sippy/pkg/dataloader/prowloader/pgwriter"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	intutil "github.com/openshift/sippy/test/integration/util"
)

var testDate = civil.Date{Year: 2026, Month: 7, Day: 15}

func seedProwJob(t *testing.T, dbc *db.DB, name, release string) uint {
	t.Helper()
	job := models.ProwJob{Name: name, Release: release}
	require.NoError(t, dbc.DB.Create(&job).Error)
	return job.ID
}

func writeBatch(t *testing.T, dbc *db.DB, currentDate civil.Date, batch []pgwriter.JobRunResult) {
	t.Helper()
	require.NoError(t, pgwriter.Write(context.Background(), dbc, currentDate, batch))
}

func carryForward(t *testing.T, dbc *db.DB, currentDate civil.Date, releases []string) {
	t.Helper()
	require.NoError(t, pgwriter.CarryForwardCumulativeSummaries(context.Background(), dbc, currentDate, releases))
}

func TestJobRunsPersisted(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")
	ts := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{
			ID: 1001, Cluster: "build01", Duration: 30 * time.Minute,
			ProwJobID: jobID, ProwJobRelease: "4.18", URL: "https://prow/1001",
			GCSBucket: "origin-ci-test", Timestamp: ts,
			OverallResult: "S", TestFailures: 2, Succeeded: true,
			Labels: []string{"cloud:aws", "upgrade:none"},
		},
	}})

	var run models.ProwJobRun
	require.NoError(t, dbc.DB.First(&run, 1001).Error)
	assert.Equal(t, "build01", run.Cluster)
	assert.Equal(t, jobID, run.ProwJobID)
	assert.Equal(t, "https://prow/1001", run.URL)
	assert.Equal(t, "origin-ci-test", run.GCSBucket)
	assert.Equal(t, "S", string(run.OverallResult))
	assert.Equal(t, 2, run.TestFailures)
	assert.True(t, run.Succeeded)
}

func TestAnnotationsStored(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")
	ts := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{
			ID: 2001, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts,
		},
		Annotations: []pgwriter.AnnotationRow{
			{ProwJobRunID: 2001, Key: "risk", Value: "high", ProwJobRunRelease: "4.18", ProwJobRunTimestamp: ts},
			{ProwJobRunID: 2001, Key: "team", Value: "trt", ProwJobRunRelease: "4.18", ProwJobRunTimestamp: ts},
		},
	}})

	var anns []models.ProwJobRunAnnotation
	require.NoError(t, dbc.DB.Where("prow_job_run_id = ?", 2001).Find(&anns).Error)
	assert.Len(t, anns, 2)

	annMap := make(map[string]string)
	for _, a := range anns {
		annMap[a.Key] = a.Value
	}
	assert.Equal(t, "high", annMap["risk"])
	assert.Equal(t, "trt", annMap["team"])
}

func TestPullRequestsAssociated(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "pull-ci-e2e", "4.18")
	ts := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{
			ID: 3001, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts,
		},
		PullRequests: []pgwriter.PullRequestRow{
			{Org: "openshift", Repo: "origin", Link: "https://github.com/openshift/origin/pull/100", SHA: "abc123", Author: "dev1", Title: "Fix thing", Number: 100},
		},
		PullRequestAssoc: []pgwriter.PullRequestAssocRow{
			{ProwJobRunID: 3001, Link: "https://github.com/openshift/origin/pull/100", SHA: "abc123", ProwJobRunRelease: "4.18", ProwJobRunTimestamp: ts},
		},
	}})

	var pr models.ProwPullRequest
	require.NoError(t, dbc.DB.Where("link = ? AND sha = ?", "https://github.com/openshift/origin/pull/100", "abc123").First(&pr).Error)
	assert.Equal(t, "openshift", pr.Org)
	assert.Equal(t, "origin", pr.Repo)
	assert.Equal(t, "dev1", pr.Author)
	assert.Equal(t, "Fix thing", pr.Title)
	assert.Equal(t, 100, pr.Number)

	var assocCount int64
	require.NoError(t, dbc.DB.Model(&models.ProwJobRunProwPullRequest{}).Where("prow_job_run_id = ?", 3001).Count(&assocCount).Error)
	assert.Equal(t, int64(1), assocCount)
}

func TestPullRequestMetadataPreservedOnReload(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "pull-ci-e2e", "4.18")
	ts := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	mergedAt := time.Date(2026, 7, 14, 8, 0, 0, 0, time.UTC)

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{ID: 4001, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
		PullRequests: []pgwriter.PullRequestRow{
			{Org: "openshift", Repo: "origin", Link: "https://github.com/openshift/origin/pull/200", SHA: "def456", Author: "dev1", Title: "Original title", Number: 200, MergedAt: &mergedAt},
		},
		PullRequestAssoc: []pgwriter.PullRequestAssocRow{
			{ProwJobRunID: 4001, Link: "https://github.com/openshift/origin/pull/200", SHA: "def456", ProwJobRunRelease: "4.18", ProwJobRunTimestamp: ts},
		},
	}})

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{ID: 4002, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
		PullRequests: []pgwriter.PullRequestRow{
			{Org: "openshift", Repo: "origin", Link: "https://github.com/openshift/origin/pull/200", SHA: "def456", Author: "", Title: "", Number: 200, MergedAt: nil},
		},
		PullRequestAssoc: []pgwriter.PullRequestAssocRow{
			{ProwJobRunID: 4002, Link: "https://github.com/openshift/origin/pull/200", SHA: "def456", ProwJobRunRelease: "4.18", ProwJobRunTimestamp: ts},
		},
	}})

	var pr models.ProwPullRequest
	require.NoError(t, dbc.DB.Where("link = ? AND sha = ?", "https://github.com/openshift/origin/pull/200", "def456").First(&pr).Error)
	assert.Equal(t, "dev1", pr.Author, "original author should be preserved")
	assert.Equal(t, "Original title", pr.Title, "original title should be preserved")
	assert.NotNil(t, pr.MergedAt, "merged_at should be preserved")
}

func TestTestResultsPersisted(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")
	ts := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{ID: 5001, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
		Tests: []pgwriter.TestRow{
			{ProwJobRunID: 5001, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "[sig-api] pods should start", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 1.5},
			{ProwJobRunID: 5001, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "[sig-network] services should work", SuiteName: "junit_e2e", Status: statusFailure, Duration: 3.2},
		},
	}})

	var tests []models.Test
	require.NoError(t, dbc.DB.Find(&tests).Error)
	assert.GreaterOrEqual(t, len(tests), 2)

	testNames := sets.New[string]()
	for _, test := range tests {
		testNames.Insert(test.Name)
	}
	assert.True(t, testNames.Has("[sig-api] pods should start"))
	assert.True(t, testNames.Has("[sig-network] services should work"))

	var suite models.Suite
	require.NoError(t, dbc.DB.Where("name = ?", "junit_e2e").First(&suite).Error)

	var jrt []models.ProwJobRunTest
	require.NoError(t, dbc.DB.Where("prow_job_run_id = ?", 5001).Find(&jrt).Error)
	assert.Len(t, jrt, 2)
}

func TestTestOutputsStoredWhenPresent(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")
	ts := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	failOutput := "expected 200, got 500"

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{ID: 6001, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
		Tests: []pgwriter.TestRow{
			{ProwJobRunID: 6001, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "test-with-output", SuiteName: "junit_e2e", Status: statusFailure, Duration: 1.0, Output: &failOutput},
			{ProwJobRunID: 6001, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "test-without-output", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 0.5},
		},
	}})

	var outputs []models.ProwJobRunTestOutput
	require.NoError(t, dbc.DB.Find(&outputs).Error)
	assert.Len(t, outputs, 1)
	assert.Equal(t, "expected 200, got 500", outputs[0].Output)
}

func TestSoftDeletedTestsRestored(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")
	ts := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)

	deletedAt := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, dbc.DB.Exec(
		"INSERT INTO tests (name, created_at, updated_at, deleted_at) VALUES (?, NOW(), NOW(), ?)",
		"deleted-test", deletedAt,
	).Error)

	var softDeleted models.Test
	require.NoError(t, dbc.DB.Unscoped().Where("name = ?", "deleted-test").First(&softDeleted).Error)
	require.True(t, softDeleted.DeletedAt.Valid, "precondition: test should be soft-deleted")

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{ID: 7001, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
		Tests: []pgwriter.TestRow{
			{ProwJobRunID: 7001, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "deleted-test", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 1.0},
		},
	}})

	var restored models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "deleted-test").First(&restored).Error)
	assert.False(t, restored.DeletedAt.Valid, "soft-deleted test should be restored")
}

func TestDailyTotalsReflectCounts(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")
	ts := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{ID: 8001, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
		Tests: []pgwriter.TestRow{
			{ProwJobRunID: 8001, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "count-test", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 1.0},
			{ProwJobRunID: 8001, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "count-test-fail", SuiteName: "junit_e2e", Status: statusFailure, Duration: 2.0},
			{ProwJobRunID: 8001, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "count-test-flake", SuiteName: "junit_e2e", Status: statusFlake, Duration: 3.0},
		},
	}})

	var test models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "count-test").First(&test).Error)

	var dt models.TestDailyTotal
	require.NoError(t, dbc.DB.Where("test_id = ? AND prow_job_id = ? AND release = ?", test.ID, jobID, "4.18").First(&dt).Error)
	assert.Equal(t, int32(1), dt.Successes)
	assert.Equal(t, int32(0), dt.Failures)
	assert.Equal(t, int32(0), dt.Flakes)
	assert.Equal(t, int32(1), dt.Runs)

	var failTest models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "count-test-fail").First(&failTest).Error)
	var dtFail models.TestDailyTotal
	require.NoError(t, dbc.DB.Where("test_id = ? AND prow_job_id = ?", failTest.ID, jobID).First(&dtFail).Error)
	assert.Equal(t, int32(0), dtFail.Successes)
	assert.Equal(t, int32(1), dtFail.Failures)
	assert.Equal(t, int32(1), dtFail.Runs)

	var flakeTest models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "count-test-flake").First(&flakeTest).Error)
	var dtFlake models.TestDailyTotal
	require.NoError(t, dbc.DB.Where("test_id = ? AND prow_job_id = ?", flakeTest.ID, jobID).First(&dtFlake).Error)
	assert.Equal(t, int32(0), dtFlake.Successes)
	assert.Equal(t, int32(0), dtFlake.Failures)
	assert.Equal(t, int32(1), dtFlake.Flakes)
	assert.Equal(t, int32(1), dtFlake.Runs)
}

func TestDailyTotalsAccumulateAcrossBatches(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")
	ts := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{ID: 9001, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
		Tests: []pgwriter.TestRow{
			{ProwJobRunID: 9001, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "accum-test", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 1.0},
		},
	}})

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{ID: 9002, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
		Tests: []pgwriter.TestRow{
			{ProwJobRunID: 9002, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "accum-test", SuiteName: "junit_e2e", Status: statusFailure, Duration: 2.0},
		},
	}})

	var test models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "accum-test").First(&test).Error)

	var dt models.TestDailyTotal
	require.NoError(t, dbc.DB.Where("test_id = ? AND prow_job_id = ? AND release = ?", test.ID, jobID, "4.18").First(&dt).Error)
	assert.Equal(t, int32(1), dt.Successes)
	assert.Equal(t, int32(1), dt.Failures)
	assert.Equal(t, int32(2), dt.Runs)
}

func TestDailyTotalsScopedByRelease(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID418 := seedProwJob(t, dbc, "periodic-e2e-aws-418", "4.18")
	jobID417 := seedProwJob(t, dbc, "periodic-e2e-aws-417", "4.17")
	ts := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{
		{
			Run: pgwriter.RunRow{ID: 10001, ProwJobID: jobID418, ProwJobRelease: "4.18", Timestamp: ts},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: 10001, ProwJobID: jobID418, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "release-scoped-test", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 1.0},
			},
		},
		{
			Run: pgwriter.RunRow{ID: 10002, ProwJobID: jobID417, ProwJobRelease: "4.17", Timestamp: ts},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: 10002, ProwJobID: jobID417, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.17", TestName: "release-scoped-test", SuiteName: "junit_e2e", Status: statusFailure, Duration: 2.0},
			},
		},
	})

	var test models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "release-scoped-test").First(&test).Error)

	var totals []models.TestDailyTotal
	require.NoError(t, dbc.DB.Where("test_id = ?", test.ID).Find(&totals).Error)
	assert.Len(t, totals, 2)

	releaseMap := make(map[string]models.TestDailyTotal)
	for _, dt := range totals {
		releaseMap[dt.Release] = dt
	}
	assert.Equal(t, int32(1), releaseMap["4.18"].Successes)
	assert.Equal(t, int32(0), releaseMap["4.18"].Failures)
	assert.Equal(t, int32(0), releaseMap["4.17"].Successes)
	assert.Equal(t, int32(1), releaseMap["4.17"].Failures)
}

func TestDailyTotalsScopedByLifecycle(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")

	today := testDate
	ts := time.Date(today.Year, today.Month, today.Day, 10, 0, 0, 0, time.UTC)

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{ID: 35001, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
		Tests: []pgwriter.TestRow{
			{ProwJobRunID: 35001, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "lifecycle-test", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 1.0, Lifecycle: "blocking"},
			{ProwJobRunID: 35001, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "lifecycle-test", SuiteName: "junit_e2e", Status: statusFailure, Duration: 2.0, Lifecycle: "informing"},
		},
	}})

	var test models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "lifecycle-test").First(&test).Error)

	var totals []models.TestDailyTotal
	require.NoError(t, dbc.DB.Where("test_id = ? AND date = ? AND release = ?", test.ID, today, "4.18").Find(&totals).Error)
	require.Len(t, totals, 2, "should have separate daily total rows for blocking and informing")

	byLifecycle := make(map[string]models.TestDailyTotal)
	for _, dt := range totals {
		byLifecycle[dt.Lifecycle] = dt
	}
	assert.Equal(t, int32(1), byLifecycle["blocking"].Successes)
	assert.Equal(t, int32(0), byLifecycle["blocking"].Failures)
	assert.Equal(t, int32(0), byLifecycle["informing"].Successes)
	assert.Equal(t, int32(1), byLifecycle["informing"].Failures)
}

func TestCumulativeSummariesScopedByLifecycle(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")

	today := testDate
	ts := time.Date(today.Year, today.Month, today.Day, 10, 0, 0, 0, time.UTC)

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{ID: 36001, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
		Tests: []pgwriter.TestRow{
			{ProwJobRunID: 36001, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "lifecycle-cum-test", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 1.0, Lifecycle: "blocking"},
			{ProwJobRunID: 36001, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "lifecycle-cum-test", SuiteName: "junit_e2e", Status: statusFailure, Duration: 2.0, Lifecycle: "informing"},
		},
	}})

	var test models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "lifecycle-cum-test").First(&test).Error)

	tomorrow := today.AddDays(1)
	var summaries []models.TestCumulativeSummary
	require.NoError(t, dbc.DB.Where("test_id = ? AND date = ? AND release = ?", test.ID, tomorrow, "4.18").Find(&summaries).Error)
	require.Len(t, summaries, 2, "should have separate cumulative rows for blocking and informing")

	byLifecycle := make(map[string]models.TestCumulativeSummary)
	for _, cs := range summaries {
		byLifecycle[cs.Lifecycle] = cs
	}
	assert.Equal(t, int64(1), byLifecycle["blocking"].PrefixSumSuccesses)
	assert.Equal(t, int64(0), byLifecycle["blocking"].PrefixSumFailures)
	assert.Equal(t, int64(0), byLifecycle["informing"].PrefixSumSuccesses)
	assert.Equal(t, int64(1), byLifecycle["informing"].PrefixSumFailures)
}

func TestCarryForwardPreservesLifecycleSeparation(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")

	today := testDate
	ts := time.Date(today.Year, today.Month, today.Day, 10, 0, 0, 0, time.UTC)

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{ID: 37001, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
		Tests: []pgwriter.TestRow{
			{ProwJobRunID: 37001, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "lifecycle-carry-test", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 1.0, Lifecycle: "blocking"},
			{ProwJobRunID: 37001, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "lifecycle-carry-test", SuiteName: "junit_e2e", Status: statusFailure, Duration: 2.0, Lifecycle: "informing"},
		},
	}})

	var test models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "lifecycle-carry-test").First(&test).Error)

	tomorrow := today.AddDays(1)
	require.NoError(t, dbc.DB.Where("test_id = ? AND date = ? AND release = ?", test.ID, tomorrow, "4.18").Delete(&models.TestCumulativeSummary{}).Error)

	carryForward(t, dbc, testDate, []string{"4.18"})

	var summaries []models.TestCumulativeSummary
	require.NoError(t, dbc.DB.Where("test_id = ? AND date = ? AND release = ?", test.ID, tomorrow, "4.18").Find(&summaries).Error)
	require.Len(t, summaries, 2, "carry-forward should preserve both lifecycle rows")

	byLifecycle := make(map[string]models.TestCumulativeSummary)
	for _, cs := range summaries {
		byLifecycle[cs.Lifecycle] = cs
	}
	assert.Equal(t, int64(1), byLifecycle["blocking"].PrefixSumSuccesses)
	assert.Equal(t, int64(0), byLifecycle["blocking"].PrefixSumFailures)
	assert.Equal(t, int64(0), byLifecycle["informing"].PrefixSumSuccesses)
	assert.Equal(t, int64(1), byLifecycle["informing"].PrefixSumFailures)
}

func TestCumulativeSummariesCascadeThroughTomorrow(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")

	today := testDate
	ts := time.Date(today.Year, today.Month, today.Day, 10, 0, 0, 0, time.UTC)

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{ID: 11001, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
		Tests: []pgwriter.TestRow{
			{ProwJobRunID: 11001, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "cascade-test", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 1.0},
		},
	}})

	var test models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "cascade-test").First(&test).Error)

	var summaries []models.TestCumulativeSummary
	require.NoError(t, dbc.DB.Where("test_id = ? AND release = ?", test.ID, "4.18").Order("date").Find(&summaries).Error)

	tomorrow := today.AddDays(1)
	assert.GreaterOrEqual(t, len(summaries), 2, "should have rows for today and tomorrow")

	dateMap := make(map[civil.Date]models.TestCumulativeSummary)
	for _, s := range summaries {
		dateMap[s.Date] = s
	}
	assert.Equal(t, int64(1), dateMap[today].PrefixSumSuccesses)
	assert.Equal(t, int64(1), dateMap[today].PrefixSumRuns)
	assert.Equal(t, int64(1), dateMap[tomorrow].PrefixSumSuccesses)
	assert.Equal(t, int64(1), dateMap[tomorrow].PrefixSumRuns)
}

func TestCumulativeSummariesAccumulateAcrossBatches(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")

	today := testDate
	ts := time.Date(today.Year, today.Month, today.Day, 10, 0, 0, 0, time.UTC)

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{ID: 12001, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
		Tests: []pgwriter.TestRow{
			{ProwJobRunID: 12001, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "accum-cum-test", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 1.0},
		},
	}})

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{ID: 12002, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
		Tests: []pgwriter.TestRow{
			{ProwJobRunID: 12002, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "accum-cum-test", SuiteName: "junit_e2e", Status: statusFailure, Duration: 2.0},
		},
	}})

	var test models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "accum-cum-test").First(&test).Error)

	var summary models.TestCumulativeSummary
	require.NoError(t, dbc.DB.Where("test_id = ? AND date = ? AND release = ?", test.ID, today, "4.18").First(&summary).Error)
	assert.Equal(t, int64(1), summary.PrefixSumSuccesses)
	assert.Equal(t, int64(1), summary.PrefixSumFailures)
	assert.Equal(t, int64(2), summary.PrefixSumRuns)
}

func TestMultiDateBatch(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")

	today := testDate
	yesterday := today.AddDays(-1)

	tsToday := time.Date(today.Year, today.Month, today.Day, 10, 0, 0, 0, time.UTC)
	tsYesterday := time.Date(yesterday.Year, yesterday.Month, yesterday.Day, 22, 0, 0, 0, time.UTC)

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{
		{
			Run: pgwriter.RunRow{ID: 13001, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: tsYesterday},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: 13001, ProwJobID: jobID, ProwJobRunTimestamp: tsYesterday, ProwJobRunRelease: "4.18", TestName: "multi-date-test", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 1.0},
			},
		},
		{
			Run: pgwriter.RunRow{ID: 13002, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: tsToday},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: 13002, ProwJobID: jobID, ProwJobRunTimestamp: tsToday, ProwJobRunRelease: "4.18", TestName: "multi-date-test", SuiteName: "junit_e2e", Status: statusFailure, Duration: 2.0},
			},
		},
	})

	var test models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "multi-date-test").First(&test).Error)

	var totals []models.TestDailyTotal
	require.NoError(t, dbc.DB.Where("test_id = ? AND prow_job_id = ?", test.ID, jobID).Order("date").Find(&totals).Error)
	assert.Len(t, totals, 2, "should have separate daily totals for each date")

	dateMap := make(map[civil.Date]models.TestDailyTotal)
	for _, dt := range totals {
		dateMap[dt.Date] = dt
	}
	assert.Equal(t, int32(1), dateMap[yesterday].Successes)
	assert.Equal(t, int32(1), dateMap[today].Failures)

	tomorrow := today.AddDays(1)
	var cumSummary models.TestCumulativeSummary
	require.NoError(t, dbc.DB.Where("test_id = ? AND date = ? AND release = ?", test.ID, tomorrow, "4.18").First(&cumSummary).Error)
	assert.Equal(t, int64(1), cumSummary.PrefixSumSuccesses)
	assert.Equal(t, int64(1), cumSummary.PrefixSumFailures)
	assert.Equal(t, int64(2), cumSummary.PrefixSumRuns)
}

func TestMultiDateBatchCumulativeSummariesAtEachDate(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")

	today := testDate
	yesterday := today.AddDays(-1)
	tomorrow := today.AddDays(1)

	tsToday := time.Date(today.Year, today.Month, today.Day, 10, 0, 0, 0, time.UTC)
	tsYesterday := time.Date(yesterday.Year, yesterday.Month, yesterday.Day, 22, 0, 0, 0, time.UTC)

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{
		{
			Run: pgwriter.RunRow{ID: 22001, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: tsYesterday},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: 22001, ProwJobID: jobID, ProwJobRunTimestamp: tsYesterday, ProwJobRunRelease: "4.18", TestName: "multi-date-cum-test", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 1.0},
			},
		},
		{
			Run: pgwriter.RunRow{ID: 22002, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: tsToday},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: 22002, ProwJobID: jobID, ProwJobRunTimestamp: tsToday, ProwJobRunRelease: "4.18", TestName: "multi-date-cum-test", SuiteName: "junit_e2e", Status: statusFailure, Duration: 2.0},
			},
		},
	})

	var test models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "multi-date-cum-test").First(&test).Error)

	dateMap := make(map[civil.Date]models.TestCumulativeSummary)
	var summaries []models.TestCumulativeSummary
	require.NoError(t, dbc.DB.Where("test_id = ? AND release = ?", test.ID, "4.18").Find(&summaries).Error)
	for _, s := range summaries {
		dateMap[s.Date] = s
	}

	assert.Equal(t, int64(1), dateMap[yesterday].PrefixSumSuccesses, "yesterday: only yesterday's success")
	assert.Equal(t, int64(0), dateMap[yesterday].PrefixSumFailures, "yesterday: no failures yet")
	assert.Equal(t, int64(1), dateMap[yesterday].PrefixSumRuns, "yesterday: 1 run")

	assert.Equal(t, int64(1), dateMap[today].PrefixSumSuccesses, "today: yesterday's success cascaded")
	assert.Equal(t, int64(1), dateMap[today].PrefixSumFailures, "today: today's failure added")
	assert.Equal(t, int64(2), dateMap[today].PrefixSumRuns, "today: 2 runs total")

	assert.Equal(t, int64(1), dateMap[tomorrow].PrefixSumSuccesses, "tomorrow: same as today")
	assert.Equal(t, int64(1), dateMap[tomorrow].PrefixSumFailures, "tomorrow: same as today")
	assert.Equal(t, int64(2), dateMap[tomorrow].PrefixSumRuns, "tomorrow: same as today")
}

func TestCumulativeSummariesScopedByRelease(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID418 := seedProwJob(t, dbc, "periodic-e2e-aws-418", "4.18")
	jobID417 := seedProwJob(t, dbc, "periodic-e2e-aws-417", "4.17")

	today := testDate
	tomorrow := today.AddDays(1)
	ts := time.Date(today.Year, today.Month, today.Day, 10, 0, 0, 0, time.UTC)

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{
		{
			Run: pgwriter.RunRow{ID: 23001, ProwJobID: jobID418, ProwJobRelease: "4.18", Timestamp: ts},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: 23001, ProwJobID: jobID418, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "release-cum-test", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 1.0},
			},
		},
		{
			Run: pgwriter.RunRow{ID: 23002, ProwJobID: jobID417, ProwJobRelease: "4.17", Timestamp: ts},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: 23002, ProwJobID: jobID417, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.17", TestName: "release-cum-test", SuiteName: "junit_e2e", Status: statusFailure, Duration: 2.0},
			},
		},
	})

	var test models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "release-cum-test").First(&test).Error)

	var sum418 models.TestCumulativeSummary
	require.NoError(t, dbc.DB.Where("test_id = ? AND date = ? AND release = ?", test.ID, tomorrow, "4.18").First(&sum418).Error)
	assert.Equal(t, int64(1), sum418.PrefixSumSuccesses)
	assert.Equal(t, int64(0), sum418.PrefixSumFailures)
	assert.Equal(t, int64(1), sum418.PrefixSumRuns)

	var sum417 models.TestCumulativeSummary
	require.NoError(t, dbc.DB.Where("test_id = ? AND date = ? AND release = ?", test.ID, tomorrow, "4.17").First(&sum417).Error)
	assert.Equal(t, int64(0), sum417.PrefixSumSuccesses)
	assert.Equal(t, int64(1), sum417.PrefixSumFailures)
	assert.Equal(t, int64(1), sum417.PrefixSumRuns)
}

func TestCarryForwardCopiesTodayToTomorrow(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")

	today := testDate
	ts := time.Date(today.Year, today.Month, today.Day, 10, 0, 0, 0, time.UTC)

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{ID: 14001, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
		Tests: []pgwriter.TestRow{
			{ProwJobRunID: 14001, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "carry-test", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 1.0},
		},
	}})

	var test models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "carry-test").First(&test).Error)

	tomorrow := today.AddDays(1)
	require.NoError(t, dbc.DB.Where("test_id = ? AND date = ? AND release = ?", test.ID, tomorrow, "4.18").Delete(&models.TestCumulativeSummary{}).Error)

	carryForward(t, dbc, testDate, []string{"4.18"})

	var summary models.TestCumulativeSummary
	require.NoError(t, dbc.DB.Where("test_id = ? AND date = ? AND release = ?", test.ID, tomorrow, "4.18").First(&summary).Error)
	assert.Equal(t, int64(1), summary.PrefixSumSuccesses)
	assert.Equal(t, int64(1), summary.PrefixSumRuns)
}

func TestCarryForwardIsIdempotent(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")

	today := testDate
	ts := time.Date(today.Year, today.Month, today.Day, 10, 0, 0, 0, time.UTC)

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{ID: 15001, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
		Tests: []pgwriter.TestRow{
			{ProwJobRunID: 15001, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "idempotent-test", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 1.0},
		},
	}})

	var test models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "idempotent-test").First(&test).Error)

	tomorrow := today.AddDays(1)
	require.NoError(t, dbc.DB.Where("test_id = ? AND date = ? AND release = ?", test.ID, tomorrow, "4.18").Delete(&models.TestCumulativeSummary{}).Error)

	carryForward(t, dbc, testDate, []string{"4.18"})
	carryForward(t, dbc, testDate, []string{"4.18"})

	var count int64
	require.NoError(t, dbc.DB.Model(&models.TestCumulativeSummary{}).Where("test_id = ? AND date = ? AND release = ?", test.ID, tomorrow, "4.18").Count(&count).Error)
	assert.Equal(t, int64(1), count, "carry-forward should not create duplicate rows")
}

func TestCarryForwardIsReleaseScoped(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID418 := seedProwJob(t, dbc, "periodic-e2e-aws-418", "4.18")
	jobID417 := seedProwJob(t, dbc, "periodic-e2e-aws-417", "4.17")

	today := testDate
	ts := time.Date(today.Year, today.Month, today.Day, 10, 0, 0, 0, time.UTC)

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{
		{
			Run: pgwriter.RunRow{ID: 16001, ProwJobID: jobID418, ProwJobRelease: "4.18", Timestamp: ts},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: 16001, ProwJobID: jobID418, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "scoped-carry-test", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 1.0},
			},
		},
		{
			Run: pgwriter.RunRow{ID: 16002, ProwJobID: jobID417, ProwJobRelease: "4.17", Timestamp: ts},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: 16002, ProwJobID: jobID417, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.17", TestName: "scoped-carry-test", SuiteName: "junit_e2e", Status: statusFailure, Duration: 2.0},
			},
		},
	})

	var test models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "scoped-carry-test").First(&test).Error)

	tomorrow := today.AddDays(1)
	require.NoError(t, dbc.DB.Where("test_id = ? AND date = ? AND release IN (?)", test.ID, tomorrow, []string{"4.18", "4.17"}).Delete(&models.TestCumulativeSummary{}).Error)

	carryForward(t, dbc, testDate, []string{"4.18"})

	var count418 int64
	require.NoError(t, dbc.DB.Model(&models.TestCumulativeSummary{}).Where("test_id = ? AND date = ? AND release = ?", test.ID, tomorrow, "4.18").Count(&count418).Error)
	assert.Equal(t, int64(1), count418, "4.18 should be carried forward")

	var count417 int64
	require.NoError(t, dbc.DB.Model(&models.TestCumulativeSummary{}).Where("test_id = ? AND date = ? AND release = ?", test.ID, tomorrow, "4.17").Count(&count417).Error)
	assert.Equal(t, int64(0), count417, "4.17 should not be carried forward")
}

func TestCarryForwardMultipleReleasesInParallel(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID418 := seedProwJob(t, dbc, "periodic-e2e-aws-418", "4.18")
	jobID417 := seedProwJob(t, dbc, "periodic-e2e-aws-417", "4.17")

	today := testDate
	ts := time.Date(today.Year, today.Month, today.Day, 10, 0, 0, 0, time.UTC)

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{
		{
			Run: pgwriter.RunRow{ID: 34001, ProwJobID: jobID418, ProwJobRelease: "4.18", Timestamp: ts},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: 34001, ProwJobID: jobID418, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "parallel-carry-test", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 1.0},
			},
		},
		{
			Run: pgwriter.RunRow{ID: 34002, ProwJobID: jobID417, ProwJobRelease: "4.17", Timestamp: ts},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: 34002, ProwJobID: jobID417, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.17", TestName: "parallel-carry-test", SuiteName: "junit_e2e", Status: statusFailure, Duration: 2.0},
			},
		},
	})

	var test models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "parallel-carry-test").First(&test).Error)

	tomorrow := today.AddDays(1)
	require.NoError(t, dbc.DB.Where("test_id = ? AND date = ?", test.ID, tomorrow).Delete(&models.TestCumulativeSummary{}).Error)

	carryForward(t, dbc, testDate, []string{"4.18", "4.17"})

	var summary418 models.TestCumulativeSummary
	require.NoError(t, dbc.DB.Where("test_id = ? AND date = ? AND release = ?", test.ID, tomorrow, "4.18").First(&summary418).Error)
	assert.Equal(t, int64(1), summary418.PrefixSumSuccesses)
	assert.Equal(t, int64(1), summary418.PrefixSumRuns)

	var summary417 models.TestCumulativeSummary
	require.NoError(t, dbc.DB.Where("test_id = ? AND date = ? AND release = ?", test.ID, tomorrow, "4.17").First(&summary417).Error)
	assert.Equal(t, int64(1), summary417.PrefixSumFailures)
	assert.Equal(t, int64(1), summary417.PrefixSumRuns)
}

func TestCarryForwardCatchesUpMultipleMissedDays(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")

	seedDate := civil.Date{Year: 2026, Month: 7, Day: 13}
	ts := time.Date(seedDate.Year, seedDate.Month, seedDate.Day, 10, 0, 0, 0, time.UTC)

	writeBatch(t, dbc, seedDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{ID: 20001, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
		Tests: []pgwriter.TestRow{
			{ProwJobRunID: 20001, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "catchup-test", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 1.0},
		},
	}})

	var test models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "catchup-test").First(&test).Error)

	laterDate := civil.Date{Year: 2026, Month: 7, Day: 16}
	carryForward(t, dbc, laterDate, []string{"4.18"})

	laterTomorrow := laterDate.AddDays(1)
	for day := seedDate; !day.After(laterTomorrow); day = day.AddDays(1) {
		var summary models.TestCumulativeSummary
		require.NoError(t, dbc.DB.Where("test_id = ? AND date = ? AND release = ?", test.ID, day, "4.18").First(&summary).Error,
			"expected cumulative summary row for %s", day)
		assert.Equal(t, int64(1), summary.PrefixSumSuccesses, "date %s", day)
		assert.Equal(t, int64(1), summary.PrefixSumRuns, "date %s", day)
	}

	dayAfterTomorrow := laterTomorrow.AddDays(1)
	var count int64
	require.NoError(t, dbc.DB.Model(&models.TestCumulativeSummary{}).Where("test_id = ? AND date = ? AND release = ?", test.ID, dayAfterTomorrow, "4.18").Count(&count).Error)
	assert.Equal(t, int64(0), count, "should not have data beyond tomorrow")
}

func TestCarryForwardErrorsWhenNoDataWithinLookback(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	err := pgwriter.CarryForwardCumulativeSummaries(context.Background(), dbc, testDate, []string{"4.18"})
	assert.NoError(t, err, "should be a no-op when no data exists at all")

	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")
	oldDate := civil.Date{Year: 2026, Month: 5, Day: 1}
	ts := time.Date(oldDate.Year, oldDate.Month, oldDate.Day, 10, 0, 0, 0, time.UTC)

	writeBatch(t, dbc, oldDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{ID: 21001, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
		Tests: []pgwriter.TestRow{
			{ProwJobRunID: 21001, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "old-test", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 1.0},
		},
	}})

	farFuture := civil.Date{Year: 2026, Month: 8, Day: 15}
	err = pgwriter.CarryForwardCumulativeSummaries(context.Background(), dbc, farFuture, []string{"4.18"})
	require.Error(t, err, "should error when latest data is beyond the 30-day lookback window")
	assert.Contains(t, err.Error(), "no cumulative summary data found")
}

func TestBatchWithNoTestResults(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")
	ts := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{
			ID: 17001, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts,
			OverallResult: "S", Succeeded: true,
		},
	}})

	var run models.ProwJobRun
	require.NoError(t, dbc.DB.First(&run, 17001).Error)
	assert.True(t, run.Succeeded)

	var testCount int64
	require.NoError(t, dbc.DB.Model(&models.ProwJobRunTest{}).Where("prow_job_run_id = ?", 17001).Count(&testCount).Error)
	assert.Equal(t, int64(0), testCount)

	var dtCount int64
	require.NoError(t, dbc.DB.Model(&models.TestDailyTotal{}).Count(&dtCount).Error)
	assert.Equal(t, int64(0), dtCount)
}

func TestEmptySuiteNameGetsSuiteIDZero(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")
	ts := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{ID: 18001, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
		Tests: []pgwriter.TestRow{
			{ProwJobRunID: 18001, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "no-suite-test", SuiteName: "", Status: statusSuccess, Duration: 1.0},
		},
	}})

	var test models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "no-suite-test").First(&test).Error)

	var dt models.TestDailyTotal
	require.NoError(t, dbc.DB.Where("test_id = ?", test.ID).First(&dt).Error)
	assert.Equal(t, uint(0), dt.SuiteID)
}

func TestEmptyBatchIsNoOp(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	err := pgwriter.Write(context.Background(), dbc, testDate, []pgwriter.JobRunResult{})
	assert.NoError(t, err)

	var runCount int64
	require.NoError(t, dbc.DB.Model(&models.ProwJobRun{}).Count(&runCount).Error)
	assert.Equal(t, int64(0), runCount)
}

func TestDuplicateJobRunIDFailsBatch(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")
	ts := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{ID: 19001, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
	}})

	err := pgwriter.Write(context.Background(), dbc, testDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{ID: 19001, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
	}})
	assert.Error(t, err, "inserting a duplicate job run ID should fail")
}

func TestFutureDatedTestResultsRejectBatch(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")

	futureTS := time.Date(2026, 7, 17, 10, 0, 0, 0, time.UTC)

	err := pgwriter.Write(context.Background(), dbc, testDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{ID: 38001, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: futureTS},
		Tests: []pgwriter.TestRow{
			{ProwJobRunID: 38001, ProwJobID: jobID, ProwJobRunTimestamp: futureTS, ProwJobRunRelease: "4.18", TestName: "future-test", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 1.0},
		},
	}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "batch contains test results dated after")

	var dtCount int64
	require.NoError(t, dbc.DB.Model(&models.TestDailyTotal{}).Count(&dtCount).Error)
	assert.Equal(t, int64(0), dtCount, "transaction should have rolled back, no daily totals written")

	var runCount int64
	require.NoError(t, dbc.DB.Model(&models.ProwJobRun{}).Count(&runCount).Error)
	assert.Equal(t, int64(0), runCount, "transaction should have rolled back, no job runs written")

	var testResultCount int64
	require.NoError(t, dbc.DB.Model(&models.ProwJobRunTest{}).Count(&testResultCount).Error)
	assert.Equal(t, int64(0), testResultCount, "transaction should have rolled back, no test results written")
}

func TestDailyTotalsTrackFirstAndLastTimestamps(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")

	earlyTS := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)
	lateTS := time.Date(2026, 7, 15, 16, 0, 0, 0, time.UTC)

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{
		{
			Run: pgwriter.RunRow{ID: 30001, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: earlyTS},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: 30001, ProwJobID: jobID, ProwJobRunTimestamp: earlyTS, ProwJobRunRelease: "4.18", TestName: "ts-test", SuiteName: "junit_e2e", Status: statusFailure, Duration: 1.0},
			},
		},
		{
			Run: pgwriter.RunRow{ID: 30002, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: lateTS},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: 30002, ProwJobID: jobID, ProwJobRunTimestamp: lateTS, ProwJobRunRelease: "4.18", TestName: "ts-test", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 1.0},
			},
		},
	})

	var test models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "ts-test").First(&test).Error)

	var dt models.TestDailyTotal
	require.NoError(t, dbc.DB.Where("test_id = ? AND prow_job_id = ? AND release = ?", test.ID, jobID, "4.18").First(&dt).Error)

	require.NotNil(t, dt.FirstFailureTimestamp, "first failure timestamp should be set")
	assert.True(t, dt.FirstFailureTimestamp.Equal(earlyTS), "first failure should be the early timestamp")
	require.NotNil(t, dt.LastFailureTimestamp, "last failure timestamp should be set")
	assert.True(t, dt.LastFailureTimestamp.Equal(earlyTS), "last failure should also be early (only one failure)")

	require.NotNil(t, dt.FirstSuccessTimestamp, "first success timestamp should be set")
	assert.True(t, dt.FirstSuccessTimestamp.Equal(lateTS), "first success should be the late timestamp")
	require.NotNil(t, dt.LastSuccessTimestamp, "last success timestamp should be set")
	assert.True(t, dt.LastSuccessTimestamp.Equal(lateTS), "last success should also be late (only one success)")

	// Second batch: add an earlier success and a later failure
	earlierSuccessTS := time.Date(2026, 7, 15, 6, 0, 0, 0, time.UTC)
	laterFailureTS := time.Date(2026, 7, 15, 20, 0, 0, 0, time.UTC)

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{
		{
			Run: pgwriter.RunRow{ID: 30003, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: earlierSuccessTS},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: 30003, ProwJobID: jobID, ProwJobRunTimestamp: earlierSuccessTS, ProwJobRunRelease: "4.18", TestName: "ts-test", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 1.0},
			},
		},
		{
			Run: pgwriter.RunRow{ID: 30004, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: laterFailureTS},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: 30004, ProwJobID: jobID, ProwJobRunTimestamp: laterFailureTS, ProwJobRunRelease: "4.18", TestName: "ts-test", SuiteName: "junit_e2e", Status: statusFailure, Duration: 1.0},
			},
		},
	})

	require.NoError(t, dbc.DB.Where("test_id = ? AND prow_job_id = ? AND release = ?", test.ID, jobID, "4.18").First(&dt).Error)

	require.NotNil(t, dt.FirstFailureTimestamp)
	assert.True(t, dt.FirstFailureTimestamp.Equal(earlyTS), "first failure should remain the earlier one")
	require.NotNil(t, dt.LastFailureTimestamp)
	assert.True(t, dt.LastFailureTimestamp.Equal(laterFailureTS), "last failure should be updated to the later one")

	require.NotNil(t, dt.FirstSuccessTimestamp)
	assert.True(t, dt.FirstSuccessTimestamp.Equal(earlierSuccessTS), "first success should be updated to the earlier one")
	require.NotNil(t, dt.LastSuccessTimestamp)
	assert.True(t, dt.LastSuccessTimestamp.Equal(lateTS), "last success should remain the later one")
}

func TestAllPassingBatchPreservesExistingFailureTimestamps(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")

	failTS := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{ID: 31001, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: failTS},
		Tests: []pgwriter.TestRow{
			{ProwJobRunID: 31001, ProwJobID: jobID, ProwJobRunTimestamp: failTS, ProwJobRunRelease: "4.18", TestName: "pass-preserve-test", SuiteName: "junit_e2e", Status: statusFailure, Duration: 1.0},
		},
	}})

	passTS := time.Date(2026, 7, 15, 14, 0, 0, 0, time.UTC)
	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{ID: 31002, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: passTS},
		Tests: []pgwriter.TestRow{
			{ProwJobRunID: 31002, ProwJobID: jobID, ProwJobRunTimestamp: passTS, ProwJobRunRelease: "4.18", TestName: "pass-preserve-test", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 1.0},
		},
	}})

	var test models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "pass-preserve-test").First(&test).Error)

	var dt models.TestDailyTotal
	require.NoError(t, dbc.DB.Where("test_id = ? AND prow_job_id = ? AND release = ?", test.ID, jobID, "4.18").First(&dt).Error)

	require.NotNil(t, dt.FirstFailureTimestamp, "failure timestamp should be preserved after all-passing batch")
	assert.True(t, dt.FirstFailureTimestamp.Equal(failTS))
	require.NotNil(t, dt.LastFailureTimestamp, "failure timestamp should be preserved after all-passing batch")
	assert.True(t, dt.LastFailureTimestamp.Equal(failTS))

	require.NotNil(t, dt.FirstSuccessTimestamp, "success timestamp should be set by passing batch")
	assert.True(t, dt.FirstSuccessTimestamp.Equal(passTS))
}

func TestCumulativeSummariesTrackLatestTimestamps(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")

	today := testDate
	tomorrow := today.AddDays(1)
	failTS := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	successTS := time.Date(2026, 7, 15, 14, 0, 0, 0, time.UTC)

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{
		{
			Run: pgwriter.RunRow{ID: 32001, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: failTS},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: 32001, ProwJobID: jobID, ProwJobRunTimestamp: failTS, ProwJobRunRelease: "4.18", TestName: "cum-ts-test", SuiteName: "junit_e2e", Status: statusFailure, Duration: 1.0},
			},
		},
		{
			Run: pgwriter.RunRow{ID: 32002, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: successTS},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: 32002, ProwJobID: jobID, ProwJobRunTimestamp: successTS, ProwJobRunRelease: "4.18", TestName: "cum-ts-test", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 1.0},
			},
		},
	})

	var test models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "cum-ts-test").First(&test).Error)

	var summary models.TestCumulativeSummary
	require.NoError(t, dbc.DB.Where("test_id = ? AND date = ? AND release = ?", test.ID, tomorrow, "4.18").First(&summary).Error)

	require.NotNil(t, summary.PrefixMaxLastFailure, "prefix_max_last_failure should be set")
	assert.True(t, summary.PrefixMaxLastFailure.Equal(failTS))
	require.NotNil(t, summary.PrefixMaxLastSuccess, "prefix_max_last_success should be set")
	assert.True(t, summary.PrefixMaxLastSuccess.Equal(successTS))
}

func TestCarryForwardPreservesTimestamps(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")

	seedDate := civil.Date{Year: 2026, Month: 7, Day: 13}
	failTS := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)
	successTS := time.Date(2026, 7, 13, 14, 0, 0, 0, time.UTC)

	writeBatch(t, dbc, seedDate, []pgwriter.JobRunResult{
		{
			Run: pgwriter.RunRow{ID: 33001, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: failTS},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: 33001, ProwJobID: jobID, ProwJobRunTimestamp: failTS, ProwJobRunRelease: "4.18", TestName: "carry-ts-test", SuiteName: "junit_e2e", Status: statusFailure, Duration: 1.0},
			},
		},
		{
			Run: pgwriter.RunRow{ID: 33002, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: successTS},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: 33002, ProwJobID: jobID, ProwJobRunTimestamp: successTS, ProwJobRunRelease: "4.18", TestName: "carry-ts-test", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 1.0},
			},
		},
	})

	var test models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "carry-ts-test").First(&test).Error)

	laterDate := civil.Date{Year: 2026, Month: 7, Day: 16}
	carryForward(t, dbc, laterDate, []string{"4.18"})

	laterTomorrow := laterDate.AddDays(1)
	var summary models.TestCumulativeSummary
	require.NoError(t, dbc.DB.Where("test_id = ? AND date = ? AND release = ?", test.ID, laterTomorrow, "4.18").First(&summary).Error)

	require.NotNil(t, summary.PrefixMaxLastFailure, "failure timestamp should survive carry-forward")
	assert.True(t, summary.PrefixMaxLastFailure.Equal(failTS))
	require.NotNil(t, summary.PrefixMaxLastSuccess, "success timestamp should survive carry-forward")
	assert.True(t, summary.PrefixMaxLastSuccess.Equal(successTS))
}

func TestMultipleRunsForSameTestInOneBatchAggregateCounts(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")

	ts1 := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)
	ts2 := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	ts3 := time.Date(2026, 7, 15, 16, 0, 0, 0, time.UTC)

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{
		{
			Run: pgwriter.RunRow{ID: 34001, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts1},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: 34001, ProwJobID: jobID, ProwJobRunTimestamp: ts1, ProwJobRunRelease: "4.18", TestName: "multi-run-test", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 1.0},
			},
		},
		{
			Run: pgwriter.RunRow{ID: 34002, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts2},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: 34002, ProwJobID: jobID, ProwJobRunTimestamp: ts2, ProwJobRunRelease: "4.18", TestName: "multi-run-test", SuiteName: "junit_e2e", Status: statusFailure, Duration: 2.0},
			},
		},
		{
			Run: pgwriter.RunRow{ID: 34003, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts3},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: 34003, ProwJobID: jobID, ProwJobRunTimestamp: ts3, ProwJobRunRelease: "4.18", TestName: "multi-run-test", SuiteName: "junit_e2e", Status: statusFlake, Duration: 3.0},
			},
		},
	})

	var test models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "multi-run-test").First(&test).Error)

	var dt models.TestDailyTotal
	require.NoError(t, dbc.DB.Where("test_id = ? AND prow_job_id = ? AND release = ?", test.ID, jobID, "4.18").First(&dt).Error)
	assert.Equal(t, int32(1), dt.Successes, "one success across three runs")
	assert.Equal(t, int32(1), dt.Failures, "one failure across three runs")
	assert.Equal(t, int32(1), dt.Flakes, "one flake across three runs")
	assert.Equal(t, int32(3), dt.Runs, "three total runs")
}

func TestPullRequestMetadataPopulatedOnSecondLoad(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "pull-ci-e2e", "4.18")
	ts := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{ID: 35001, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
		PullRequests: []pgwriter.PullRequestRow{
			{Org: "openshift", Repo: "origin", Link: "https://github.com/openshift/origin/pull/300", SHA: "aaa111", Author: "", Title: "", Number: 300},
		},
		PullRequestAssoc: []pgwriter.PullRequestAssocRow{
			{ProwJobRunID: 35001, Link: "https://github.com/openshift/origin/pull/300", SHA: "aaa111", ProwJobRunRelease: "4.18", ProwJobRunTimestamp: ts},
		},
	}})

	var prBefore models.ProwPullRequest
	require.NoError(t, dbc.DB.Where("link = ? AND sha = ?", "https://github.com/openshift/origin/pull/300", "aaa111").First(&prBefore).Error)
	assert.Empty(t, prBefore.Author)
	assert.Empty(t, prBefore.Title)

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{ID: 35002, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
		PullRequests: []pgwriter.PullRequestRow{
			{Org: "openshift", Repo: "origin", Link: "https://github.com/openshift/origin/pull/300", SHA: "aaa111", Author: "contributor", Title: "Add feature X", Number: 300},
		},
		PullRequestAssoc: []pgwriter.PullRequestAssocRow{
			{ProwJobRunID: 35002, Link: "https://github.com/openshift/origin/pull/300", SHA: "aaa111", ProwJobRunRelease: "4.18", ProwJobRunTimestamp: ts},
		},
	}})

	var prAfter models.ProwPullRequest
	require.NoError(t, dbc.DB.Where("link = ? AND sha = ?", "https://github.com/openshift/origin/pull/300", "aaa111").First(&prAfter).Error)
	assert.Equal(t, "contributor", prAfter.Author, "empty author should be populated on second load")
	assert.Equal(t, "Add feature X", prAfter.Title, "empty title should be populated on second load")
}

func TestSoftDeletedSuiteRestoredWhenTestReappears(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")
	ts := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)

	deletedAt := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, dbc.DB.Exec(
		"INSERT INTO suites (name, created_at, updated_at, deleted_at) VALUES (?, NOW(), NOW(), ?)",
		"deleted-suite", deletedAt,
	).Error)

	var softDeleted models.Suite
	require.NoError(t, dbc.DB.Unscoped().Where("name = ?", "deleted-suite").First(&softDeleted).Error)
	require.True(t, softDeleted.DeletedAt.Valid, "precondition: suite should be soft-deleted")

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{ID: 36001, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
		Tests: []pgwriter.TestRow{
			{ProwJobRunID: 36001, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "suite-restore-test", SuiteName: "deleted-suite", Status: statusSuccess, Duration: 1.0},
		},
	}})

	var restored models.Suite
	require.NoError(t, dbc.DB.Where("name = ?", "deleted-suite").First(&restored).Error)
	assert.False(t, restored.DeletedAt.Valid, "soft-deleted suite should be restored")
}

func TestDuplicatePullRequestsInSameBatchDeduplicated(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "pull-ci-e2e", "4.18")
	ts := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)

	mergedEarly := time.Date(2026, 7, 14, 8, 0, 0, 0, time.UTC)
	mergedLate := time.Date(2026, 7, 14, 16, 0, 0, 0, time.UTC)

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{ID: 37001, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
		PullRequests: []pgwriter.PullRequestRow{
			{Org: "openshift", Repo: "origin", Link: "https://github.com/openshift/origin/pull/400", SHA: "bbb222", Author: "dev-early", Title: "Early title", Number: 400, MergedAt: &mergedEarly},
			{Org: "openshift", Repo: "origin", Link: "https://github.com/openshift/origin/pull/400", SHA: "bbb222", Author: "dev-late", Title: "Late title", Number: 400, MergedAt: &mergedLate},
		},
		PullRequestAssoc: []pgwriter.PullRequestAssocRow{
			{ProwJobRunID: 37001, Link: "https://github.com/openshift/origin/pull/400", SHA: "bbb222", ProwJobRunRelease: "4.18", ProwJobRunTimestamp: ts},
		},
	}})

	var prs []models.ProwPullRequest
	require.NoError(t, dbc.DB.Where("link = ? AND sha = ?", "https://github.com/openshift/origin/pull/400", "bbb222").Find(&prs).Error)
	assert.Len(t, prs, 1, "duplicate PRs should be deduplicated to one row")

	pr := prs[0]
	require.NotNil(t, pr.MergedAt, "merged_at should be set")
	assert.True(t, pr.MergedAt.Equal(mergedLate), "DISTINCT ON should pick the row with latest merged_at")
}

func TestDailyTotalsScopedBySuite(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")
	ts := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{ID: 39001, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
		Tests: []pgwriter.TestRow{
			{ProwJobRunID: 39001, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "suite-scoped-test", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 1.0},
			{ProwJobRunID: 39001, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "suite-scoped-test", SuiteName: "junit_serial", Status: statusFailure, Duration: 2.0},
		},
	}})

	var test models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "suite-scoped-test").First(&test).Error)

	var suiteE2E models.Suite
	require.NoError(t, dbc.DB.Where("name = ?", "junit_e2e").First(&suiteE2E).Error)
	var suiteSerial models.Suite
	require.NoError(t, dbc.DB.Where("name = ?", "junit_serial").First(&suiteSerial).Error)

	var totals []models.TestDailyTotal
	require.NoError(t, dbc.DB.Where("test_id = ? AND release = ?", test.ID, "4.18").Find(&totals).Error)
	require.Len(t, totals, 2, "same test in two suites should produce separate daily total rows")

	bySuite := make(map[uint]models.TestDailyTotal)
	for _, dt := range totals {
		bySuite[dt.SuiteID] = dt
	}
	assert.Equal(t, int32(1), bySuite[suiteE2E.ID].Successes)
	assert.Equal(t, int32(0), bySuite[suiteE2E.ID].Failures)
	assert.Equal(t, int32(0), bySuite[suiteSerial.ID].Successes)
	assert.Equal(t, int32(1), bySuite[suiteSerial.ID].Failures)
}

func TestCumulativeSummariesScopedBySuite(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")
	ts := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	tomorrow := testDate.AddDays(1)

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{ID: 39101, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
		Tests: []pgwriter.TestRow{
			{ProwJobRunID: 39101, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "suite-cum-test", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 1.0},
			{ProwJobRunID: 39101, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "suite-cum-test", SuiteName: "junit_serial", Status: statusFailure, Duration: 2.0},
		},
	}})

	var test models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "suite-cum-test").First(&test).Error)

	var suiteE2E models.Suite
	require.NoError(t, dbc.DB.Where("name = ?", "junit_e2e").First(&suiteE2E).Error)
	var suiteSerial models.Suite
	require.NoError(t, dbc.DB.Where("name = ?", "junit_serial").First(&suiteSerial).Error)

	var summaries []models.TestCumulativeSummary
	require.NoError(t, dbc.DB.Where("test_id = ? AND date = ? AND release = ?", test.ID, tomorrow, "4.18").Find(&summaries).Error)
	require.Len(t, summaries, 2, "same test in two suites should produce separate cumulative summary rows")

	bySuite := make(map[uint]models.TestCumulativeSummary)
	for _, cs := range summaries {
		bySuite[cs.SuiteID] = cs
	}
	assert.Equal(t, int64(1), bySuite[suiteE2E.ID].PrefixSumSuccesses)
	assert.Equal(t, int64(0), bySuite[suiteE2E.ID].PrefixSumFailures)
	assert.Equal(t, int64(0), bySuite[suiteSerial.ID].PrefixSumSuccesses)
	assert.Equal(t, int64(1), bySuite[suiteSerial.ID].PrefixSumFailures)
}

func TestEmptySuiteNameGetsSuiteIDZeroInCumulativeSummaries(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")
	ts := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	tomorrow := testDate.AddDays(1)

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{ID: 39201, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: ts},
		Tests: []pgwriter.TestRow{
			{ProwJobRunID: 39201, ProwJobID: jobID, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "no-suite-cum-test", SuiteName: "", Status: statusSuccess, Duration: 1.0},
		},
	}})

	var test models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "no-suite-cum-test").First(&test).Error)

	var summary models.TestCumulativeSummary
	require.NoError(t, dbc.DB.Where("test_id = ? AND date = ? AND release = ?", test.ID, tomorrow, "4.18").First(&summary).Error)
	assert.Equal(t, uint(0), summary.SuiteID, "empty suite name should map to suite_id=0 in cumulative summaries")
}

func TestDailyTotalsScopedByProwJob(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobA := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")
	jobB := seedProwJob(t, dbc, "periodic-e2e-gcp", "4.18")
	ts := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{
		{
			Run: pgwriter.RunRow{ID: 39301, ProwJobID: jobA, ProwJobRelease: "4.18", Timestamp: ts},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: 39301, ProwJobID: jobA, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "job-scoped-test", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 1.0},
			},
		},
		{
			Run: pgwriter.RunRow{ID: 39302, ProwJobID: jobB, ProwJobRelease: "4.18", Timestamp: ts},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: 39302, ProwJobID: jobB, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "job-scoped-test", SuiteName: "junit_e2e", Status: statusFailure, Duration: 2.0},
			},
		},
	})

	var test models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "job-scoped-test").First(&test).Error)

	var totals []models.TestDailyTotal
	require.NoError(t, dbc.DB.Where("test_id = ? AND release = ?", test.ID, "4.18").Find(&totals).Error)
	require.Len(t, totals, 2, "same test under two prow jobs should produce separate daily total rows")

	byJob := make(map[uint]models.TestDailyTotal)
	for _, dt := range totals {
		byJob[dt.ProwJobID] = dt
	}
	assert.Equal(t, int32(1), byJob[jobA].Successes)
	assert.Equal(t, int32(0), byJob[jobA].Failures)
	assert.Equal(t, int32(0), byJob[jobB].Successes)
	assert.Equal(t, int32(1), byJob[jobB].Failures)
}

func TestCumulativeSummariesScopedByProwJob(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobA := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")
	jobB := seedProwJob(t, dbc, "periodic-e2e-gcp", "4.18")
	ts := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	tomorrow := testDate.AddDays(1)

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{
		{
			Run: pgwriter.RunRow{ID: 39401, ProwJobID: jobA, ProwJobRelease: "4.18", Timestamp: ts},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: 39401, ProwJobID: jobA, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "job-cum-test", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 1.0},
			},
		},
		{
			Run: pgwriter.RunRow{ID: 39402, ProwJobID: jobB, ProwJobRelease: "4.18", Timestamp: ts},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: 39402, ProwJobID: jobB, ProwJobRunTimestamp: ts, ProwJobRunRelease: "4.18", TestName: "job-cum-test", SuiteName: "junit_e2e", Status: statusFailure, Duration: 2.0},
			},
		},
	})

	var test models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "job-cum-test").First(&test).Error)

	var summaries []models.TestCumulativeSummary
	require.NoError(t, dbc.DB.Where("test_id = ? AND date = ? AND release = ?", test.ID, tomorrow, "4.18").Find(&summaries).Error)
	require.Len(t, summaries, 2, "same test under two prow jobs should produce separate cumulative summary rows")

	byJob := make(map[uint]models.TestCumulativeSummary)
	for _, cs := range summaries {
		byJob[cs.ProwJobID] = cs
	}
	assert.Equal(t, int64(1), byJob[jobA].PrefixSumSuccesses)
	assert.Equal(t, int64(0), byJob[jobA].PrefixSumFailures)
	assert.Equal(t, int64(0), byJob[jobB].PrefixSumSuccesses)
	assert.Equal(t, int64(1), byJob[jobB].PrefixSumFailures)
}

func TestCumulativeSummariesPropagateFlakes(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")

	today := testDate
	yesterday := today.AddDays(-1)
	tomorrow := today.AddDays(1)

	tsYesterday := time.Date(yesterday.Year, yesterday.Month, yesterday.Day, 10, 0, 0, 0, time.UTC)
	tsToday := time.Date(today.Year, today.Month, today.Day, 10, 0, 0, 0, time.UTC)

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{
		{
			Run: pgwriter.RunRow{ID: 39501, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: tsYesterday},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: 39501, ProwJobID: jobID, ProwJobRunTimestamp: tsYesterday, ProwJobRunRelease: "4.18", TestName: "flake-cum-test", SuiteName: "junit_e2e", Status: statusFlake, Duration: 1.0},
			},
		},
		{
			Run: pgwriter.RunRow{ID: 39502, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: tsToday},
			Tests: []pgwriter.TestRow{
				{ProwJobRunID: 39502, ProwJobID: jobID, ProwJobRunTimestamp: tsToday, ProwJobRunRelease: "4.18", TestName: "flake-cum-test", SuiteName: "junit_e2e", Status: statusFlake, Duration: 2.0},
			},
		},
	})

	var test models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "flake-cum-test").First(&test).Error)

	dateMap := make(map[civil.Date]models.TestCumulativeSummary)
	var summaries []models.TestCumulativeSummary
	require.NoError(t, dbc.DB.Where("test_id = ? AND release = ?", test.ID, "4.18").Find(&summaries).Error)
	for _, s := range summaries {
		dateMap[s.Date] = s
	}

	assert.Equal(t, int64(1), dateMap[yesterday].PrefixSumFlakes, "yesterday: one flake")
	assert.Equal(t, int64(1), dateMap[yesterday].PrefixSumRuns, "yesterday: one run")
	assert.Equal(t, int64(2), dateMap[today].PrefixSumFlakes, "today: two flakes (prefix sum)")
	assert.Equal(t, int64(2), dateMap[today].PrefixSumRuns, "today: two runs")
	assert.Equal(t, int64(2), dateMap[tomorrow].PrefixSumFlakes, "tomorrow: same as today")
	assert.Equal(t, int64(2), dateMap[tomorrow].PrefixSumRuns, "tomorrow: same as today")
}

func TestLaterBatchDoesNotCorruptEarlierCumulativeRow(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")

	dayD := testDate.AddDays(-2)
	dayD2 := testDate

	tsD := time.Date(dayD.Year, dayD.Month, dayD.Day, 10, 0, 0, 0, time.UTC)
	tsD2 := time.Date(dayD2.Year, dayD2.Month, dayD2.Day, 10, 0, 0, 0, time.UTC)

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{ID: 39601, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: tsD},
		Tests: []pgwriter.TestRow{
			{ProwJobRunID: 39601, ProwJobID: jobID, ProwJobRunTimestamp: tsD, ProwJobRunRelease: "4.18", TestName: "no-corrupt-test", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 1.0},
		},
	}})

	var test models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "no-corrupt-test").First(&test).Error)

	var beforeSummary models.TestCumulativeSummary
	require.NoError(t, dbc.DB.Where("test_id = ? AND date = ? AND release = ?", test.ID, dayD, "4.18").First(&beforeSummary).Error)
	assert.Equal(t, int64(1), beforeSummary.PrefixSumSuccesses)
	assert.Equal(t, int64(0), beforeSummary.PrefixSumFailures)
	assert.Equal(t, int64(1), beforeSummary.PrefixSumRuns)

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{ID: 39602, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: tsD2},
		Tests: []pgwriter.TestRow{
			{ProwJobRunID: 39602, ProwJobID: jobID, ProwJobRunTimestamp: tsD2, ProwJobRunRelease: "4.18", TestName: "no-corrupt-test", SuiteName: "junit_e2e", Status: statusFailure, Duration: 2.0},
		},
	}})

	var afterSummary models.TestCumulativeSummary
	require.NoError(t, dbc.DB.Where("test_id = ? AND date = ? AND release = ?", test.ID, dayD, "4.18").First(&afterSummary).Error)
	assert.Equal(t, int64(1), afterSummary.PrefixSumSuccesses, "day D success count should be unchanged after later batch")
	assert.Equal(t, int64(0), afterSummary.PrefixSumFailures, "day D failure count should be unchanged after later batch")
	assert.Equal(t, int64(1), afterSummary.PrefixSumRuns, "day D run count should be unchanged after later batch")

	tomorrow := testDate.AddDays(1)
	var tomorrowSummary models.TestCumulativeSummary
	require.NoError(t, dbc.DB.Where("test_id = ? AND date = ? AND release = ?", test.ID, tomorrow, "4.18").First(&tomorrowSummary).Error)
	assert.Equal(t, int64(1), tomorrowSummary.PrefixSumSuccesses, "tomorrow should include day D's success")
	assert.Equal(t, int64(1), tomorrowSummary.PrefixSumFailures, "tomorrow should include day D+2's failure")
	assert.Equal(t, int64(2), tomorrowSummary.PrefixSumRuns, "tomorrow should include both runs")
}

func TestCarryForwardThenWriteNewDataAccumulates(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")

	seedDate := civil.Date{Year: 2026, Month: 7, Day: 13}
	gapDate := civil.Date{Year: 2026, Month: 7, Day: 15}
	tsSeed := time.Date(seedDate.Year, seedDate.Month, seedDate.Day, 10, 0, 0, 0, time.UTC)

	writeBatch(t, dbc, seedDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{ID: 39701, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: tsSeed},
		Tests: []pgwriter.TestRow{
			{ProwJobRunID: 39701, ProwJobID: jobID, ProwJobRunTimestamp: tsSeed, ProwJobRunRelease: "4.18", TestName: "carry-then-write-test", SuiteName: "junit_e2e", Status: statusSuccess, Duration: 1.0},
		},
	}})

	var test models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "carry-then-write-test").First(&test).Error)

	carryForward(t, dbc, gapDate, []string{"4.18"})

	var carriedSummary models.TestCumulativeSummary
	require.NoError(t, dbc.DB.Where("test_id = ? AND date = ? AND release = ?", test.ID, gapDate, "4.18").First(&carriedSummary).Error)
	assert.Equal(t, int64(1), carriedSummary.PrefixSumSuccesses, "carried-forward row should have seed data")
	assert.Equal(t, int64(1), carriedSummary.PrefixSumRuns)

	tsGap := time.Date(gapDate.Year, gapDate.Month, gapDate.Day, 10, 0, 0, 0, time.UTC)
	writeBatch(t, dbc, gapDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{ID: 39702, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: tsGap},
		Tests: []pgwriter.TestRow{
			{ProwJobRunID: 39702, ProwJobID: jobID, ProwJobRunTimestamp: tsGap, ProwJobRunRelease: "4.18", TestName: "carry-then-write-test", SuiteName: "junit_e2e", Status: statusFailure, Duration: 2.0},
		},
	}})

	var updatedSummary models.TestCumulativeSummary
	require.NoError(t, dbc.DB.Where("test_id = ? AND date = ? AND release = ?", test.ID, gapDate, "4.18").First(&updatedSummary).Error)
	assert.Equal(t, int64(1), updatedSummary.PrefixSumSuccesses, "carried-forward success should be preserved")
	assert.Equal(t, int64(1), updatedSummary.PrefixSumFailures, "new failure should be added on top of carried-forward data")
	assert.Equal(t, int64(2), updatedSummary.PrefixSumRuns, "runs should accumulate on top of carried-forward data")
}
