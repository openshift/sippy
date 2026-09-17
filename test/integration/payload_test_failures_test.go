package integration

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/sippy/pkg/api"
	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/filter"
	intutil "github.com/openshift/sippy/test/integration/util"
)

// payloadFixture holds the common test data for payload test failure tests.
type payloadFixture struct {
	release     string
	arch        string
	stream      string
	reportEnd   time.Time
	recentTag   models.ReleaseTag
	oldTag      models.ReleaseTag
	acceptedTag models.ReleaseTag
	otherTag    models.ReleaseTag
	testA       models.Test
	testB       models.Test
}

func setupPayloadFixture(t *testing.T, dbc *db.DB) payloadFixture {
	t.Helper()

	f := payloadFixture{
		release:   "4.16",
		arch:      "amd64",
		stream:    "nightly",
		reportEnd: time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC),
	}

	// Create prow jobs
	blockingJob := models.ProwJob{Name: "periodic-ci-e2e-aws-ovn-4.16", Release: f.release}
	require.NoError(t, dbc.DB.Create(&blockingJob).Error)

	infoJob := models.ProwJob{Name: "periodic-ci-e2e-gcp-informing-4.16", Release: f.release}
	require.NoError(t, dbc.DB.Create(&infoJob).Error)

	// Create tests
	f.testA = models.Test{Name: "test-a-network-check"}
	require.NoError(t, dbc.DB.Create(&f.testA).Error)
	f.testB = models.Test{Name: "test-b-install-check"}
	require.NoError(t, dbc.DB.Create(&f.testB).Error)

	// Recent rejected payload (within 14 days of reportEnd)
	f.recentTag = models.ReleaseTag{
		ReleaseTag:   "4.16.0-0.nightly-2024-06-14-010000",
		Release:      f.release,
		Stream:       f.stream,
		Architecture: f.arch,
		Phase:        apitype.PayloadRejected,
		ReleaseTime:  time.Date(2024, 6, 14, 1, 0, 0, 0, time.UTC),
	}
	require.NoError(t, dbc.DB.Create(&f.recentTag).Error)

	// Old rejected payload (outside 14-day window)
	f.oldTag = models.ReleaseTag{
		ReleaseTag:   "4.16.0-0.nightly-2024-05-20-010000",
		Release:      f.release,
		Stream:       f.stream,
		Architecture: f.arch,
		Phase:        apitype.PayloadRejected,
		ReleaseTime:  time.Date(2024, 5, 20, 1, 0, 0, 0, time.UTC),
	}
	require.NoError(t, dbc.DB.Create(&f.oldTag).Error)

	// Accepted payload (within window but not rejected)
	f.acceptedTag = models.ReleaseTag{
		ReleaseTag:   "4.16.0-0.nightly-2024-06-13-010000",
		Release:      f.release,
		Stream:       f.stream,
		Architecture: f.arch,
		Phase:        apitype.PayloadAccepted,
		ReleaseTime:  time.Date(2024, 6, 13, 1, 0, 0, 0, time.UTC),
	}
	require.NoError(t, dbc.DB.Create(&f.acceptedTag).Error)

	// Different stream payload (within window, rejected)
	f.otherTag = models.ReleaseTag{
		ReleaseTag:   "4.16.0-0.ci-2024-06-14-010000",
		Release:      f.release,
		Stream:       "ci",
		Architecture: f.arch,
		Phase:        apitype.PayloadRejected,
		ReleaseTime:  time.Date(2024, 6, 14, 1, 0, 0, 0, time.UTC),
	}
	require.NoError(t, dbc.DB.Create(&f.otherTag).Error)

	recentRunTime := time.Date(2024, 6, 14, 2, 0, 0, 0, time.UTC)
	oldRunTime := time.Date(2024, 5, 20, 2, 0, 0, 0, time.UTC)
	acceptedRunTime := time.Date(2024, 6, 13, 2, 0, 0, 0, time.UTC)
	otherRunTime := time.Date(2024, 6, 14, 3, 0, 0, 0, time.UTC)

	// Prow job runs for each payload
	recentRun := models.ProwJobRun{
		ProwJobID: blockingJob.ID, ProwJobRelease: f.release,
		Timestamp: recentRunTime, URL: "https://prow/run/1",
	}
	require.NoError(t, dbc.DB.Create(&recentRun).Error)

	oldRun := models.ProwJobRun{
		ProwJobID: blockingJob.ID, ProwJobRelease: f.release,
		Timestamp: oldRunTime, URL: "https://prow/run/2",
	}
	require.NoError(t, dbc.DB.Create(&oldRun).Error)

	acceptedRun := models.ProwJobRun{
		ProwJobID: blockingJob.ID, ProwJobRelease: f.release,
		Timestamp: acceptedRunTime, URL: "https://prow/run/3",
	}
	require.NoError(t, dbc.DB.Create(&acceptedRun).Error)

	otherStreamRun := models.ProwJobRun{
		ProwJobID: blockingJob.ID, ProwJobRelease: f.release,
		Timestamp: otherRunTime, URL: "https://prow/run/4",
	}
	require.NoError(t, dbc.DB.Create(&otherStreamRun).Error)

	informingRun := models.ProwJobRun{
		ProwJobID: infoJob.ID, ProwJobRelease: f.release,
		Timestamp: recentRunTime, URL: "https://prow/run/5",
	}
	require.NoError(t, dbc.DB.Create(&informingRun).Error)

	// Release job runs linking payloads to prow job runs
	// Recent blocking failed
	require.NoError(t, dbc.DB.Create(&models.ReleaseJobRun{
		ReleaseTagID: itoa(f.recentTag.ID), Name: recentRun.ID,
		Kind: "Blocking", State: "Failed", JobName: blockingJob.Name,
		URL: recentRun.URL,
	}).Error)

	// Old blocking failed (outside window)
	require.NoError(t, dbc.DB.Create(&models.ReleaseJobRun{
		ReleaseTagID: itoa(f.oldTag.ID), Name: oldRun.ID,
		Kind: "Blocking", State: "Failed", JobName: blockingJob.Name,
		URL: oldRun.URL,
	}).Error)

	// Accepted blocking succeeded (should not appear: State != Failed)
	require.NoError(t, dbc.DB.Create(&models.ReleaseJobRun{
		ReleaseTagID: itoa(f.acceptedTag.ID), Name: acceptedRun.ID,
		Kind: "Blocking", State: "Succeeded", JobName: blockingJob.Name,
		URL: acceptedRun.URL,
	}).Error)

	// Different stream blocking failed
	require.NoError(t, dbc.DB.Create(&models.ReleaseJobRun{
		ReleaseTagID: itoa(f.otherTag.ID), Name: otherStreamRun.ID,
		Kind: "Blocking", State: "Failed", JobName: blockingJob.Name,
		URL: otherStreamRun.URL,
	}).Error)

	// Informing failed (should not appear: Kind != Blocking)
	require.NoError(t, dbc.DB.Create(&models.ReleaseJobRun{
		ReleaseTagID: itoa(f.recentTag.ID), Name: informingRun.ID,
		Kind: "Informing", State: "Failed", JobName: infoJob.Name,
		URL: informingRun.URL,
	}).Error)

	// Test results (status 12 = failure)
	// testA failed in recent blocking run
	require.NoError(t, dbc.DB.Create(&models.ProwJobRunTest{
		ProwJobRunID: recentRun.ID, ProwJobID: blockingJob.ID,
		ProwJobRunTimestamp: recentRunTime, ProwJobRunRelease: f.release,
		TestID: f.testA.ID, Status: 12,
	}).Error)
	// testB failed in recent blocking run
	require.NoError(t, dbc.DB.Create(&models.ProwJobRunTest{
		ProwJobRunID: recentRun.ID, ProwJobID: blockingJob.ID,
		ProwJobRunTimestamp: recentRunTime, ProwJobRunRelease: f.release,
		TestID: f.testB.ID, Status: 12,
	}).Error)

	// testA failed in old blocking run (outside window)
	require.NoError(t, dbc.DB.Create(&models.ProwJobRunTest{
		ProwJobRunID: oldRun.ID, ProwJobID: blockingJob.ID,
		ProwJobRunTimestamp: oldRunTime, ProwJobRunRelease: f.release,
		TestID: f.testA.ID, Status: 12,
	}).Error)

	// testA failed in other stream run
	require.NoError(t, dbc.DB.Create(&models.ProwJobRunTest{
		ProwJobRunID: otherStreamRun.ID, ProwJobID: blockingJob.ID,
		ProwJobRunTimestamp: otherRunTime, ProwJobRunRelease: f.release,
		TestID: f.testA.ID, Status: 12,
	}).Error)

	// testA failed in informing run (should not appear: Kind != Blocking)
	require.NoError(t, dbc.DB.Create(&models.ProwJobRunTest{
		ProwJobRunID: informingRun.ID, ProwJobID: infoJob.ID,
		ProwJobRunTimestamp: recentRunTime, ProwJobRunRelease: f.release,
		TestID: f.testA.ID, Status: 12,
	}).Error)

	// testA passed in recent blocking run (status 1 = pass, should not appear)
	require.NoError(t, dbc.DB.Create(&models.ProwJobRunTest{
		ProwJobRunID: recentRun.ID, ProwJobID: blockingJob.ID,
		ProwJobRunTimestamp: recentRunTime, ProwJobRunRelease: f.release,
		TestID: f.testA.ID, Status: 1,
	}).Error)

	return f
}

func itoa(id uint) string {
	return fmt.Sprintf("%d", id)
}

func emptyFilterOpts() *filter.FilterOptions {
	return &filter.FilterOptions{Filter: &filter.Filter{}}
}

func TestGetPayloadStreamTestFailures(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	f := setupPayloadFixture(t, dbc)

	results, err := api.GetPayloadStreamTestFailures(dbc, f.release, f.stream, f.arch,
		emptyFilterOpts(), f.reportEnd)
	require.NoError(t, err)

	// Should only include test failures from:
	// - the recent rejected nightly/amd64 payload (within 14-day window)
	// - blocking jobs that failed
	// - status = 12 (failure)
	// Excludes: old payload, other stream, informing jobs, passed tests
	require.Len(t, results, 2, "should return analysis for 2 distinct tests")

	analysisByName := make(map[string]*apitype.TestFailureAnalysis)
	for _, r := range results {
		analysisByName[r.Name] = r
	}

	testA := analysisByName["test-a-network-check"]
	require.NotNil(t, testA)
	assert.Equal(t, 1, testA.FailureCount)
	require.Contains(t, testA.FailedPayloads, f.recentTag.ReleaseTag)
	assert.Len(t, testA.FailedPayloads[f.recentTag.ReleaseTag].FailedJobs, 1)
	assert.NotContains(t, testA.FailedPayloads, f.oldTag.ReleaseTag,
		"old tag outside 14-day window should not appear")

	testB := analysisByName["test-b-install-check"]
	require.NotNil(t, testB)
	assert.Equal(t, 1, testB.FailureCount)
	require.Contains(t, testB.FailedPayloads, f.recentTag.ReleaseTag)
}

func TestGetPayloadStreamTestFailures_FiltersByStream(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	f := setupPayloadFixture(t, dbc)

	results, err := api.GetPayloadStreamTestFailures(dbc, f.release, "ci", f.arch,
		emptyFilterOpts(), f.reportEnd)
	require.NoError(t, err)

	require.Len(t, results, 1, "should find only failures in ci stream")
	assert.Equal(t, "test-a-network-check", results[0].Name)
	require.Contains(t, results[0].FailedPayloads, f.otherTag.ReleaseTag)
}

func TestGetPayloadStreamTestFailures_NoPayloads(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	results, err := api.GetPayloadStreamTestFailures(dbc, "4.99", "nightly", "amd64",
		emptyFilterOpts(), time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.Nil(t, results, "should return nil when no payloads exist")
}

func TestGetPayloadStreamTestFailures_MultiPayloadAggregation(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	f := setupPayloadFixture(t, dbc)

	// Add a second in-window rejected payload where test-a also fails.
	secondTag := models.ReleaseTag{
		ReleaseTag:   "4.16.0-0.nightly-2024-06-12-010000",
		Release:      f.release,
		Stream:       f.stream,
		Architecture: f.arch,
		Phase:        apitype.PayloadRejected,
		ReleaseTime:  time.Date(2024, 6, 12, 1, 0, 0, 0, time.UTC),
	}
	require.NoError(t, dbc.DB.Create(&secondTag).Error)

	secondJob := models.ProwJob{Name: "periodic-ci-e2e-gcp-ovn-4.16", Release: f.release}
	require.NoError(t, dbc.DB.Create(&secondJob).Error)

	secondRunTime := time.Date(2024, 6, 12, 2, 0, 0, 0, time.UTC)
	secondRun := models.ProwJobRun{
		ProwJobID: secondJob.ID, ProwJobRelease: f.release,
		Timestamp: secondRunTime, URL: "https://prow/run/second",
	}
	require.NoError(t, dbc.DB.Create(&secondRun).Error)

	require.NoError(t, dbc.DB.Create(&models.ReleaseJobRun{
		ReleaseTagID: itoa(secondTag.ID), Name: secondRun.ID,
		Kind: "Blocking", State: "Failed", JobName: secondJob.Name,
		URL: secondRun.URL,
	}).Error)

	require.NoError(t, dbc.DB.Create(&models.ProwJobRunTest{
		ProwJobRunID: secondRun.ID, ProwJobID: secondJob.ID,
		ProwJobRunTimestamp: secondRunTime, ProwJobRunRelease: f.release,
		TestID: f.testA.ID, Status: 12,
	}).Error)

	results, err := api.GetPayloadStreamTestFailures(dbc, f.release, f.stream, f.arch,
		emptyFilterOpts(), f.reportEnd)
	require.NoError(t, err)

	analysisByName := make(map[string]*apitype.TestFailureAnalysis)
	for _, r := range results {
		analysisByName[r.Name] = r
	}

	testA := analysisByName["test-a-network-check"]
	require.NotNil(t, testA)
	assert.Equal(t, 2, testA.FailureCount,
		"failure_count should sum across both in-window payloads")
	assert.Len(t, testA.FailedPayloads, 2,
		"failed_payloads should have entries for both payload tags")
	assert.Contains(t, testA.FailedPayloads, f.recentTag.ReleaseTag)
	assert.Contains(t, testA.FailedPayloads, secondTag.ReleaseTag)
}

func TestGetPayloadStreamTestFailures_MultipleJobsPerPayload(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	f := setupPayloadFixture(t, dbc)

	// Add a second blocking job that also fails for the recent payload,
	// with test-a failing in it.
	secondJob := models.ProwJob{Name: "periodic-ci-e2e-metal-ovn-4.16", Release: f.release}
	require.NoError(t, dbc.DB.Create(&secondJob).Error)

	secondRunTime := time.Date(2024, 6, 14, 3, 30, 0, 0, time.UTC)
	secondRun := models.ProwJobRun{
		ProwJobID: secondJob.ID, ProwJobRelease: f.release,
		Timestamp: secondRunTime, URL: "https://prow/run/second-job",
	}
	require.NoError(t, dbc.DB.Create(&secondRun).Error)

	require.NoError(t, dbc.DB.Create(&models.ReleaseJobRun{
		ReleaseTagID: itoa(f.recentTag.ID), Name: secondRun.ID,
		Kind: "Blocking", State: "Failed", JobName: secondJob.Name,
		URL: secondRun.URL,
	}).Error)

	require.NoError(t, dbc.DB.Create(&models.ProwJobRunTest{
		ProwJobRunID: secondRun.ID, ProwJobID: secondJob.ID,
		ProwJobRunTimestamp: secondRunTime, ProwJobRunRelease: f.release,
		TestID: f.testA.ID, Status: 12,
	}).Error)

	results, err := api.GetPayloadStreamTestFailures(dbc, f.release, f.stream, f.arch,
		emptyFilterOpts(), f.reportEnd)
	require.NoError(t, err)

	analysisByName := make(map[string]*apitype.TestFailureAnalysis)
	for _, r := range results {
		analysisByName[r.Name] = r
	}

	testA := analysisByName["test-a-network-check"]
	require.NotNil(t, testA)
	payload := testA.FailedPayloads[f.recentTag.ReleaseTag]
	require.NotNil(t, payload)
	require.Len(t, payload.FailedJobs, 2,
		"failed_jobs should contain both blocking job names")
	require.Len(t, payload.FailedJobRuns, 2,
		"failed_job_runs should contain both run URLs")

	assert.Equal(t, []string{
		"periodic-ci-e2e-aws-ovn-4.16",
		"periodic-ci-e2e-metal-ovn-4.16",
	}, payload.FailedJobs, "failed_jobs should be sorted alphabetically by job name")
	assert.Equal(t, []string{
		"https://prow/run/1",
		"https://prow/run/second-job",
	}, payload.FailedJobRuns, "failed_job_runs should be paired with their corresponding job names")
}

func TestGetPayloadStreamTestFailures_ExcludesOpenShiftTestsName(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	f := setupPayloadFixture(t, dbc)

	// Create a test named "[sig-sippy] openshift-tests should work" (the synthetic
	// overall test that GetPayloadStreamTestFailures excludes) and add a
	// qualifying failure row for it.
	excludedTest := models.Test{Name: "[sig-sippy] openshift-tests should work"}
	require.NoError(t, dbc.DB.Create(&excludedTest).Error)

	var recentRun models.ProwJobRun
	require.NoError(t, dbc.DB.Where("url = ?", "https://prow/run/1").First(&recentRun).Error)

	require.NoError(t, dbc.DB.Create(&models.ProwJobRunTest{
		ProwJobRunID:        recentRun.ID,
		ProwJobID:           recentRun.ProwJobID,
		ProwJobRunTimestamp: recentRun.Timestamp,
		ProwJobRunRelease:   f.release,
		TestID:              excludedTest.ID,
		Status:              12,
	}).Error)

	results, err := api.GetPayloadStreamTestFailures(dbc, f.release, f.stream, f.arch,
		emptyFilterOpts(), f.reportEnd)
	require.NoError(t, err)

	for _, r := range results {
		assert.NotEqual(t, "[sig-sippy] openshift-tests should work", r.Name,
			"synthetic openshift-tests test should be excluded from results")
	}
	assert.Len(t, results, 2, "only the two real tests should remain")
}

func TestGetPayloadStreamTestFailures_BlockerScoreAndSortOrder(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	release := "4.18"
	arch := "amd64"
	stream := "nightly"
	reportEnd := time.Date(2024, 7, 10, 12, 0, 0, 0, time.UTC)

	job := models.ProwJob{Name: "periodic-ci-e2e-aws-4.18", Release: release}
	require.NoError(t, dbc.DB.Create(&job).Error)

	blockerTest := models.Test{Name: "test-always-fails"}
	intermittentTest := models.Test{Name: "test-sometimes-fails"}
	require.NoError(t, dbc.DB.Create(&blockerTest).Error)
	require.NoError(t, dbc.DB.Create(&intermittentTest).Error)

	// Create 4 consecutive rejected payloads (most recent first).
	// blockerTest fails in all 4; intermittentTest fails in only the oldest.
	for i := 0; i < 4; i++ {
		dayOffset := i
		tagTime := time.Date(2024, 7, 9-dayOffset, 1, 0, 0, 0, time.UTC)
		tag := models.ReleaseTag{
			ReleaseTag:   fmt.Sprintf("4.18.0-0.nightly-2024-07-%02d-010000", 9-dayOffset),
			Release:      release,
			Stream:       stream,
			Architecture: arch,
			Phase:        apitype.PayloadRejected,
			ReleaseTime:  tagTime,
		}
		require.NoError(t, dbc.DB.Create(&tag).Error)

		runTime := tagTime.Add(time.Hour)
		run := models.ProwJobRun{
			ProwJobID: job.ID, ProwJobRelease: release,
			Timestamp: runTime, URL: fmt.Sprintf("https://prow/run/blocker-%d", i),
		}
		require.NoError(t, dbc.DB.Create(&run).Error)

		require.NoError(t, dbc.DB.Create(&models.ReleaseJobRun{
			ReleaseTagID: itoa(tag.ID), Name: run.ID,
			Kind: "Blocking", State: "Failed", JobName: job.Name,
			URL: run.URL,
		}).Error)

		// blockerTest fails in every payload
		require.NoError(t, dbc.DB.Create(&models.ProwJobRunTest{
			ProwJobRunID: run.ID, ProwJobID: job.ID,
			ProwJobRunTimestamp: runTime, ProwJobRunRelease: release,
			TestID: blockerTest.ID, Status: 12,
		}).Error)

		// intermittentTest fails only in the oldest payload (i=3)
		if i == 3 {
			require.NoError(t, dbc.DB.Create(&models.ProwJobRunTest{
				ProwJobRunID: run.ID, ProwJobID: job.ID,
				ProwJobRunTimestamp: runTime, ProwJobRunRelease: release,
				TestID: intermittentTest.ID, Status: 12,
			}).Error)
		}
	}

	results, err := api.GetPayloadStreamTestFailures(dbc, release, stream, arch,
		emptyFilterOpts(), reportEnd)
	require.NoError(t, err)
	require.Len(t, results, 2)

	// Results should be sorted by BlockerScore descending.
	assert.GreaterOrEqual(t, results[0].BlockerScore, results[1].BlockerScore,
		"results should be sorted by BlockerScore descending")

	// blockerTest fails in 4 consecutive rejected payloads: score = 100
	assert.Equal(t, "test-always-fails", results[0].Name)
	assert.Equal(t, 100, results[0].BlockerScore)
	assert.Equal(t, 4, results[0].FailureCount)

	// intermittentTest fails only in the oldest payload, not in the most
	// recent consecutive streak, so its consecutive count is 0 and score = 0.
	assert.Equal(t, "test-sometimes-fails", results[1].Name)
	assert.Equal(t, 0, results[1].BlockerScore)
	assert.Equal(t, 1, results[1].FailureCount)
}

func TestGetPayloadStreamTestFailures_FiltersByArchitecture(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	f := setupPayloadFixture(t, dbc)

	// The fixture's otherTag is ci stream. Add an arm64 nightly payload
	// with test failures to verify architecture filtering.
	arm64Tag := models.ReleaseTag{
		ReleaseTag:   "4.16.0-0.nightly-arm64-2024-06-14-010000",
		Release:      f.release,
		Stream:       f.stream,
		Architecture: "arm64",
		Phase:        apitype.PayloadRejected,
		ReleaseTime:  time.Date(2024, 6, 14, 1, 0, 0, 0, time.UTC),
	}
	require.NoError(t, dbc.DB.Create(&arm64Tag).Error)

	arm64Job := models.ProwJob{Name: "periodic-ci-e2e-aws-arm64-4.16", Release: f.release}
	require.NoError(t, dbc.DB.Create(&arm64Job).Error)

	arm64RunTime := time.Date(2024, 6, 14, 4, 0, 0, 0, time.UTC)
	arm64Run := models.ProwJobRun{
		ProwJobID: arm64Job.ID, ProwJobRelease: f.release,
		Timestamp: arm64RunTime, URL: "https://prow/run/arm64",
	}
	require.NoError(t, dbc.DB.Create(&arm64Run).Error)

	require.NoError(t, dbc.DB.Create(&models.ReleaseJobRun{
		ReleaseTagID: itoa(arm64Tag.ID), Name: arm64Run.ID,
		Kind: "Blocking", State: "Failed", JobName: arm64Job.Name,
		URL: arm64Run.URL,
	}).Error)

	arm64Test := models.Test{Name: "test-arm64-only"}
	require.NoError(t, dbc.DB.Create(&arm64Test).Error)

	require.NoError(t, dbc.DB.Create(&models.ProwJobRunTest{
		ProwJobRunID: arm64Run.ID, ProwJobID: arm64Job.ID,
		ProwJobRunTimestamp: arm64RunTime, ProwJobRunRelease: f.release,
		TestID: arm64Test.ID, Status: 12,
	}).Error)

	// Query for amd64: should not include arm64 failures
	results, err := api.GetPayloadStreamTestFailures(dbc, f.release, f.stream, f.arch,
		emptyFilterOpts(), f.reportEnd)
	require.NoError(t, err)
	for _, r := range results {
		assert.NotEqual(t, "test-arm64-only", r.Name,
			"arm64 test failures should not appear in amd64 results")
	}
	assert.Len(t, results, 2, "only amd64 test failures should be returned")
}

func TestGetPayloadStreamTestFailures_FilterByName(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	f := setupPayloadFixture(t, dbc)

	results, err := api.GetPayloadStreamTestFailures(dbc, f.release, f.stream, f.arch,
		&filter.FilterOptions{
			Filter: &filter.Filter{
				Items: []filter.FilterItem{
					{
						Field:    "name",
						Operator: filter.OperatorEquals,
						Value:    "test-a-network-check",
					},
				},
			},
		}, f.reportEnd)
	require.NoError(t, err)
	require.Len(t, results, 1, "filter should return only the matching test")
	assert.Equal(t, "test-a-network-check", results[0].Name)
}

func TestGetPayloadStreamTestFailures_BlockerScoreStreakPercentageOverride(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	release := "4.19"
	arch := "amd64"
	stream := "nightly"
	reportEnd := time.Date(2024, 8, 10, 12, 0, 0, 0, time.UTC)

	job := models.ProwJob{Name: "periodic-ci-e2e-aws-4.19", Release: release}
	require.NoError(t, dbc.DB.Create(&job).Error)

	overrideTest := models.Test{Name: "test-streak-override"}
	require.NoError(t, dbc.DB.Create(&overrideTest).Error)

	// Create 5 consecutive rejected payloads.
	// overrideTest fails in payloads 0, 1, and 3 (skipping 2).
	// failedInConsecPayloads = 2 (payloads 0 and 1, then break at 2)
	// base score = min(2*25, 100) = 50
	// failedInStreak = 3 out of 5 = 60%
	// Since score(50) >= 50 AND streakPct(60) >= score(50), override to 60.
	failsInPayload := map[int]bool{0: true, 1: true, 3: true}
	for i := 0; i < 5; i++ {
		tagTime := time.Date(2024, 8, 9-i, 1, 0, 0, 0, time.UTC)
		tag := models.ReleaseTag{
			ReleaseTag:   fmt.Sprintf("4.19.0-0.nightly-2024-08-%02d-010000", 9-i),
			Release:      release,
			Stream:       stream,
			Architecture: arch,
			Phase:        apitype.PayloadRejected,
			ReleaseTime:  tagTime,
		}
		require.NoError(t, dbc.DB.Create(&tag).Error)

		runTime := tagTime.Add(time.Hour)
		run := models.ProwJobRun{
			ProwJobID: job.ID, ProwJobRelease: release,
			Timestamp: runTime, URL: fmt.Sprintf("https://prow/run/override-%d", i),
		}
		require.NoError(t, dbc.DB.Create(&run).Error)

		require.NoError(t, dbc.DB.Create(&models.ReleaseJobRun{
			ReleaseTagID: itoa(tag.ID), Name: run.ID,
			Kind: "Blocking", State: "Failed", JobName: job.Name,
			URL: run.URL,
		}).Error)

		if failsInPayload[i] {
			require.NoError(t, dbc.DB.Create(&models.ProwJobRunTest{
				ProwJobRunID: run.ID, ProwJobID: job.ID,
				ProwJobRunTimestamp: runTime, ProwJobRunRelease: release,
				TestID: overrideTest.ID, Status: 12,
			}).Error)
		}
	}

	results, err := api.GetPayloadStreamTestFailures(dbc, release, stream, arch,
		emptyFilterOpts(), reportEnd)
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.Equal(t, "test-streak-override", results[0].Name)
	assert.Equal(t, 60, results[0].BlockerScore,
		"score should be overridden to streak percentage (3/5 = 60%) when base score >= 50")
	assert.Equal(t, 3, results[0].FailureCount)
}

func TestGetPayloadStreamTestFailures_MostRecentPayloadAccepted(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	release := "4.20"
	arch := "amd64"
	stream := "nightly"
	reportEnd := time.Date(2024, 9, 10, 12, 0, 0, 0, time.UTC)

	job := models.ProwJob{Name: "periodic-ci-e2e-aws-4.20", Release: release}
	require.NoError(t, dbc.DB.Create(&job).Error)

	testX := models.Test{Name: "test-x-but-accepted"}
	require.NoError(t, dbc.DB.Create(&testX).Error)

	// Most recent payload is Accepted.
	acceptedTag := models.ReleaseTag{
		ReleaseTag:   "4.20.0-0.nightly-2024-09-09-010000",
		Release:      release,
		Stream:       stream,
		Architecture: arch,
		Phase:        apitype.PayloadAccepted,
		ReleaseTime:  time.Date(2024, 9, 9, 1, 0, 0, 0, time.UTC),
	}
	require.NoError(t, dbc.DB.Create(&acceptedTag).Error)

	// Older rejected payload with a test failure.
	rejectedTag := models.ReleaseTag{
		ReleaseTag:   "4.20.0-0.nightly-2024-09-08-010000",
		Release:      release,
		Stream:       stream,
		Architecture: arch,
		Phase:        apitype.PayloadRejected,
		ReleaseTime:  time.Date(2024, 9, 8, 1, 0, 0, 0, time.UTC),
	}
	require.NoError(t, dbc.DB.Create(&rejectedTag).Error)

	rejRunTime := time.Date(2024, 9, 8, 2, 0, 0, 0, time.UTC)
	rejRun := models.ProwJobRun{
		ProwJobID: job.ID, ProwJobRelease: release,
		Timestamp: rejRunTime, URL: "https://prow/run/accepted-latest",
	}
	require.NoError(t, dbc.DB.Create(&rejRun).Error)

	require.NoError(t, dbc.DB.Create(&models.ReleaseJobRun{
		ReleaseTagID: itoa(rejectedTag.ID), Name: rejRun.ID,
		Kind: "Blocking", State: "Failed", JobName: job.Name,
		URL: rejRun.URL,
	}).Error)

	require.NoError(t, dbc.DB.Create(&models.ProwJobRunTest{
		ProwJobRunID: rejRun.ID, ProwJobID: job.ID,
		ProwJobRunTimestamp: rejRunTime, ProwJobRunRelease: release,
		TestID: testX.ID, Status: 12,
	}).Error)

	results, err := api.GetPayloadStreamTestFailures(dbc, release, stream, arch,
		emptyFilterOpts(), reportEnd)
	require.NoError(t, err)
	require.Len(t, results, 1, "test failure from older rejected payload should still appear")

	assert.Equal(t, "test-x-but-accepted", results[0].Name)
	assert.Equal(t, 0, results[0].BlockerScore,
		"blocker score should be 0 when most recent payload is Accepted")
	assert.Contains(t, results[0].BlockerScoreReasons[0], "most recent payload was not Rejected",
		"reason should explain why score is 0")
}

func TestGetPayloadStreamTestFailures_SortTiebreakers(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	release := "4.21"
	arch := "amd64"
	stream := "nightly"
	reportEnd := time.Date(2024, 10, 10, 12, 0, 0, 0, time.UTC)

	job1 := models.ProwJob{Name: "periodic-ci-e2e-aws-4.21", Release: release}
	job2 := models.ProwJob{Name: "periodic-ci-e2e-gcp-4.21", Release: release}
	require.NoError(t, dbc.DB.Create(&job1).Error)
	require.NoError(t, dbc.DB.Create(&job2).Error)

	// Three tests that will have the same blocker score (all fail in 1 consecutive payload = 25).
	// testAlpha fails in 1 job (FailureCount=1)
	// testBravo fails in 2 jobs (FailureCount=2)
	// testCharlie fails in 1 job (FailureCount=1), same as testAlpha
	// Expected sort: testBravo (25, 2), testAlpha (25, 1, "alpha" < "charlie"), testCharlie (25, 1)
	testAlpha := models.Test{Name: "test-alpha"}
	testBravo := models.Test{Name: "test-bravo"}
	testCharlie := models.Test{Name: "test-charlie"}
	require.NoError(t, dbc.DB.Create(&testAlpha).Error)
	require.NoError(t, dbc.DB.Create(&testBravo).Error)
	require.NoError(t, dbc.DB.Create(&testCharlie).Error)

	tag := models.ReleaseTag{
		ReleaseTag:   "4.21.0-0.nightly-2024-10-09-010000",
		Release:      release,
		Stream:       stream,
		Architecture: arch,
		Phase:        apitype.PayloadRejected,
		ReleaseTime:  time.Date(2024, 10, 9, 1, 0, 0, 0, time.UTC),
	}
	require.NoError(t, dbc.DB.Create(&tag).Error)

	run1Time := time.Date(2024, 10, 9, 2, 0, 0, 0, time.UTC)
	run1 := models.ProwJobRun{
		ProwJobID: job1.ID, ProwJobRelease: release,
		Timestamp: run1Time, URL: "https://prow/run/tiebreak-1",
	}
	require.NoError(t, dbc.DB.Create(&run1).Error)

	run2Time := time.Date(2024, 10, 9, 3, 0, 0, 0, time.UTC)
	run2 := models.ProwJobRun{
		ProwJobID: job2.ID, ProwJobRelease: release,
		Timestamp: run2Time, URL: "https://prow/run/tiebreak-2",
	}
	require.NoError(t, dbc.DB.Create(&run2).Error)

	require.NoError(t, dbc.DB.Create(&models.ReleaseJobRun{
		ReleaseTagID: itoa(tag.ID), Name: run1.ID,
		Kind: "Blocking", State: "Failed", JobName: job1.Name, URL: run1.URL,
	}).Error)
	require.NoError(t, dbc.DB.Create(&models.ReleaseJobRun{
		ReleaseTagID: itoa(tag.ID), Name: run2.ID,
		Kind: "Blocking", State: "Failed", JobName: job2.Name, URL: run2.URL,
	}).Error)

	// testAlpha fails in job1 only (FailureCount=1)
	require.NoError(t, dbc.DB.Create(&models.ProwJobRunTest{
		ProwJobRunID: run1.ID, ProwJobID: job1.ID,
		ProwJobRunTimestamp: run1Time, ProwJobRunRelease: release,
		TestID: testAlpha.ID, Status: 12,
	}).Error)

	// testBravo fails in both job1 and job2 (FailureCount=2)
	require.NoError(t, dbc.DB.Create(&models.ProwJobRunTest{
		ProwJobRunID: run1.ID, ProwJobID: job1.ID,
		ProwJobRunTimestamp: run1Time, ProwJobRunRelease: release,
		TestID: testBravo.ID, Status: 12,
	}).Error)
	require.NoError(t, dbc.DB.Create(&models.ProwJobRunTest{
		ProwJobRunID: run2.ID, ProwJobID: job2.ID,
		ProwJobRunTimestamp: run2Time, ProwJobRunRelease: release,
		TestID: testBravo.ID, Status: 12,
	}).Error)

	// testCharlie fails in job1 only (FailureCount=1, same as alpha)
	require.NoError(t, dbc.DB.Create(&models.ProwJobRunTest{
		ProwJobRunID: run1.ID, ProwJobID: job1.ID,
		ProwJobRunTimestamp: run1Time, ProwJobRunRelease: release,
		TestID: testCharlie.ID, Status: 12,
	}).Error)

	results, err := api.GetPayloadStreamTestFailures(dbc, release, stream, arch,
		emptyFilterOpts(), reportEnd)
	require.NoError(t, err)
	require.Len(t, results, 3)

	// All have BlockerScore=25 (1 consecutive rejected payload).
	// testBravo has FailureCount=2, so it comes first.
	// testAlpha and testCharlie both have FailureCount=1, tiebroken by name ascending.
	assert.Equal(t, "test-bravo", results[0].Name,
		"highest FailureCount should sort first when BlockerScores tie")
	assert.Equal(t, 2, results[0].FailureCount)

	assert.Equal(t, "test-alpha", results[1].Name,
		"alphabetically earlier name should sort first when BlockerScore and FailureCount tie")
	assert.Equal(t, 1, results[1].FailureCount)

	assert.Equal(t, "test-charlie", results[2].Name)
	assert.Equal(t, 1, results[2].FailureCount)

	for _, r := range results {
		assert.Equal(t, 25, r.BlockerScore, "all tests should have BlockerScore=25")
	}
}

func TestGetPayloadStreamTestFailures_FailureCountReflectsPerJobFailures(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	f := setupPayloadFixture(t, dbc)

	// Add a second blocking job that also fails for the recent payload,
	// with test-a failing in it. FailureCount should be 2 (one per job run).
	secondJob := models.ProwJob{Name: "periodic-ci-e2e-metal-ovn-4.16", Release: f.release}
	require.NoError(t, dbc.DB.Create(&secondJob).Error)

	secondRunTime := time.Date(2024, 6, 14, 3, 30, 0, 0, time.UTC)
	secondRun := models.ProwJobRun{
		ProwJobID: secondJob.ID, ProwJobRelease: f.release,
		Timestamp: secondRunTime, URL: "https://prow/run/fc-second",
	}
	require.NoError(t, dbc.DB.Create(&secondRun).Error)

	require.NoError(t, dbc.DB.Create(&models.ReleaseJobRun{
		ReleaseTagID: itoa(f.recentTag.ID), Name: secondRun.ID,
		Kind: "Blocking", State: "Failed", JobName: secondJob.Name,
		URL: secondRun.URL,
	}).Error)

	require.NoError(t, dbc.DB.Create(&models.ProwJobRunTest{
		ProwJobRunID: secondRun.ID, ProwJobID: secondJob.ID,
		ProwJobRunTimestamp: secondRunTime, ProwJobRunRelease: f.release,
		TestID: f.testA.ID, Status: 12,
	}).Error)

	results, err := api.GetPayloadStreamTestFailures(dbc, f.release, f.stream, f.arch,
		emptyFilterOpts(), f.reportEnd)
	require.NoError(t, err)

	analysisByName := make(map[string]*apitype.TestFailureAnalysis)
	for _, r := range results {
		analysisByName[r.Name] = r
	}

	testA := analysisByName["test-a-network-check"]
	require.NotNil(t, testA)
	assert.Equal(t, 2, testA.FailureCount,
		"failure_count should reflect per-job-run failures, not per-payload count")
	assert.Len(t, testA.FailedPayloads, 1,
		"all failures are in the same payload")
}

func TestGetPayloadStreamTestFailures_BlockerScoreReasons(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	release := "4.22"
	arch := "amd64"
	stream := "nightly"
	reportEnd := time.Date(2024, 11, 10, 12, 0, 0, 0, time.UTC)

	job := models.ProwJob{Name: "periodic-ci-e2e-aws-4.22", Release: release}
	require.NoError(t, dbc.DB.Create(&job).Error)

	reasonTest := models.Test{Name: "test-reasons"}
	require.NoError(t, dbc.DB.Create(&reasonTest).Error)

	// Create 3 consecutive rejected payloads where the test fails in all 3.
	// Score = min(3*25, 100) = 75
	// failedInStreak = 3/3 = 100%, and since 75 >= 50 and 100 >= 75, override to 100.
	for i := 0; i < 3; i++ {
		tagTime := time.Date(2024, 11, 9-i, 1, 0, 0, 0, time.UTC)
		tag := models.ReleaseTag{
			ReleaseTag:   fmt.Sprintf("4.22.0-0.nightly-2024-11-%02d-010000", 9-i),
			Release:      release,
			Stream:       stream,
			Architecture: arch,
			Phase:        apitype.PayloadRejected,
			ReleaseTime:  tagTime,
		}
		require.NoError(t, dbc.DB.Create(&tag).Error)

		runTime := tagTime.Add(time.Hour)
		run := models.ProwJobRun{
			ProwJobID: job.ID, ProwJobRelease: release,
			Timestamp: runTime, URL: fmt.Sprintf("https://prow/run/reason-%d", i),
		}
		require.NoError(t, dbc.DB.Create(&run).Error)

		require.NoError(t, dbc.DB.Create(&models.ReleaseJobRun{
			ReleaseTagID: itoa(tag.ID), Name: run.ID,
			Kind: "Blocking", State: "Failed", JobName: job.Name,
			URL: run.URL,
		}).Error)

		require.NoError(t, dbc.DB.Create(&models.ProwJobRunTest{
			ProwJobRunID: run.ID, ProwJobID: job.ID,
			ProwJobRunTimestamp: runTime, ProwJobRunRelease: release,
			TestID: reasonTest.ID, Status: 12,
		}).Error)
	}

	results, err := api.GetPayloadStreamTestFailures(dbc, release, stream, arch,
		emptyFilterOpts(), reportEnd)
	require.NoError(t, err)
	require.Len(t, results, 1)

	reasons := results[0].BlockerScoreReasons
	require.Len(t, reasons, 2, "should have both consecutive and streak reasons")
	assert.Contains(t, reasons[0], "failed in 3 most recent rejected payloads")
	assert.Contains(t, reasons[1], "failed in 3/3 of current rejected payload streak")
	assert.Equal(t, 100, results[0].BlockerScore,
		"streak percentage override (3/3 = 100%) should apply since base score (75) >= 50")
}

func TestGetPayloadStreamTestFailures_AcceptedPayloadWithFailedBlockingJob(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	release := "4.23"
	arch := "amd64"
	stream := "nightly"
	reportEnd := time.Date(2024, 12, 10, 12, 0, 0, 0, time.UTC)

	job := models.ProwJob{Name: "periodic-ci-e2e-aws-4.23", Release: release}
	require.NoError(t, dbc.DB.Create(&job).Error)

	forceTest := models.Test{Name: "test-force-accepted"}
	require.NoError(t, dbc.DB.Create(&forceTest).Error)

	// A force-accepted payload: phase is Accepted, but it has a blocking job
	// that failed. The test failure should still appear in results because
	// the SQL query does not filter on rt.phase.
	forceAcceptedTag := models.ReleaseTag{
		ReleaseTag:   "4.23.0-0.nightly-2024-12-09-010000",
		Release:      release,
		Stream:       stream,
		Architecture: arch,
		Phase:        apitype.PayloadAccepted,
		ReleaseTime:  time.Date(2024, 12, 9, 1, 0, 0, 0, time.UTC),
	}
	require.NoError(t, dbc.DB.Create(&forceAcceptedTag).Error)

	runTime := time.Date(2024, 12, 9, 2, 0, 0, 0, time.UTC)
	run := models.ProwJobRun{
		ProwJobID: job.ID, ProwJobRelease: release,
		Timestamp: runTime, URL: "https://prow/run/force-accepted",
	}
	require.NoError(t, dbc.DB.Create(&run).Error)

	require.NoError(t, dbc.DB.Create(&models.ReleaseJobRun{
		ReleaseTagID: itoa(forceAcceptedTag.ID), Name: run.ID,
		Kind: "Blocking", State: "Failed", JobName: job.Name,
		URL: run.URL,
	}).Error)

	require.NoError(t, dbc.DB.Create(&models.ProwJobRunTest{
		ProwJobRunID: run.ID, ProwJobID: job.ID,
		ProwJobRunTimestamp: runTime, ProwJobRunRelease: release,
		TestID: forceTest.ID, Status: 12,
	}).Error)

	results, err := api.GetPayloadStreamTestFailures(dbc, release, stream, arch,
		emptyFilterOpts(), reportEnd)
	require.NoError(t, err)
	require.Len(t, results, 1,
		"test failures from a force-accepted payload should still appear")

	assert.Equal(t, "test-force-accepted", results[0].Name)
	assert.Equal(t, 1, results[0].FailureCount)
	assert.Contains(t, results[0].FailedPayloads, forceAcceptedTag.ReleaseTag)
	assert.Equal(t, 0, results[0].BlockerScore,
		"score should be 0 since the most recent payload is Accepted")
}
