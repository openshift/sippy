package integration

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/civil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/openshift/sippy/pkg/api/componentreadiness"
	"github.com/openshift/sippy/pkg/api/componentreadiness/dataprovider/postgres"
	"github.com/openshift/sippy/pkg/api/componentreadiness/utils"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crview"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	"github.com/openshift/sippy/pkg/dataloader/gateststatus"
	"github.com/openshift/sippy/pkg/dataloader/regressioncacheloader"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	intutil "github.com/openshift/sippy/test/integration/util"
)

// TestGAMissingSnapshotUsesOrdinaryData exercises the report and details paths
// before a release's first snapshot and after an aggregate becomes available.
func TestGAMissingSnapshotUsesOrdinaryData(t *testing.T) {
	ctx := context.Background()
	dbc := crTestDB(t)
	release := "4.15"
	gaDate := civil.DateOf(time.Now().UTC()).AddDays(-1)
	createReleaseDefinition(t, dbc, release, &gaDate)
	createReleaseDefinition(t, dbc, "4.16", &gaDate)
	vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
	job := createProwJobWithVC(t, dbc, "periodic-ga-before-snapshot", release, vc)
	test := intutil.CreateTest(t, dbc, "openshift-tests:ga-before-snapshot")
	suite := intutil.CreateSuite(t, dbc, "openshift-tests-ga-before-snapshot")
	ownership := createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:ga-before-snapshot", "Storage", []string{"PVC"})
	for _, runData := range []struct {
		at     time.Time
		status int
	}{
		{gaDate.AddDays(-30).In(time.UTC).Add(time.Hour), 1},
		{gaDate.AddDays(-2).In(time.UTC).Add(time.Hour), 1},
		{gaDate.AddDays(-1).In(time.UTC).Add(12 * time.Hour), 12},
		{gaDate.In(time.UTC).Add(24*time.Hour - time.Second), 1},
	} {
		run := createProwJobRunForCR(t, dbc, job.ID, release, runData.at)
		createProwJobRunTest(t, dbc, run.ID, job.ID, test.ID, &suite.ID, runData.status, release, runData.at)
	}
	createCumulativeSummary(t, dbc, gaDate.AddDays(-31), release, test.ID, job.ID, suite.ID, 0, 0, 0)
	createCumulativeSummary(t, dbc, gaDate, release, test.ID, job.ID, suite.ID, 4, 3, 0, withFailures(1))
	opts := defaultReqOptions(release)
	opts.BaseRelease = reqopts.Release{Name: release, Start: utils.GAWindowStart(gaDate, 30).In(time.UTC).Add(12 * time.Hour), End: gaDate.In(time.UTC)}
	opts.SampleRelease = opts.BaseRelease
	opts.IncludeAllTests = true
	opts.SkipGABaseJobRuns = true
	opts.VariantOption.IncludeVariants = map[string][]string{"Platform": {"aws"}, "Network": {"ovn"}}
	opts.TestIDOptions = []reqopts.TestIdentification{{TestID: ownership.UniqueID, RequestedVariants: map[string]string{"Platform": "aws", "Network": "ovn"}}}
	key := crtest.KeyWithVariants{TestID: ownership.UniqueID, Variants: map[string]string{"Platform": "aws", "Network": "ovn"}}
	provider := postgres.NewPostgresProvider(dbc, nil)
	loader := gateststatus.New(ctx, dbc, true, []string{release})
	loader.Load()
	require.Empty(t, loader.Errors())
	var snapshotRows int64
	require.NoError(t, dbc.DB.Model(&models.ProwGARawTestDatum{}).Where("release = ?", release).Count(&snapshotRows).Error)
	require.Zero(t, snapshotRows, "the loader must wait for the two-day delay")
	assertOrdinaryData := func() {
		t.Helper()
		for _, tc := range []struct {
			name string
			end  time.Time
		}{
			{"midnight", gaDate.In(time.UTC)},
			{"end of day", gaDate.In(time.UTC).Add(24*time.Hour - time.Second)},
		} {
			t.Run(tc.name, func(t *testing.T) {
				request := opts
				request.BaseRelease.End = tc.end
				status, errs := provider.QueryBaseTestStatus(ctx, request)
				require.Empty(t, errs)
				require.Equal(t, 4, status[key.Encode()].TotalCount)
				combined, _, errs := provider.QueryTestStatus(ctx, request)
				require.Empty(t, errs)
				require.Equal(t, 4, combined[key.Encode()].TotalCount)
				details, errs := provider.QueryBaseJobRunTestStatus(ctx, request)
				require.Empty(t, errs)
				var total, runs int
				for _, summaries := range details {
					for _, summary := range summaries {
						total += summary.Stats.Total()
						runs += len(summary.JobRuns)
					}
				}
				require.Equal(t, 4, total)
				require.Equal(t, 4, runs, "the GA-only optimization must not skip ordinary raw details")
			})
		}
	}
	assertOrdinaryData()
	// A non-GA window must still honor its exact partial-day bounds.
	nonGA := opts
	nonGA.BaseRelease.Start = gaDate.AddDays(-2).In(time.UTC).Add(11 * time.Hour)
	nonGA.BaseRelease.End = gaDate.In(time.UTC)
	details, errs := provider.QueryBaseJobRunTestStatus(ctx, nonGA)
	require.Empty(t, errs)
	var nonGATotal, nonGARuns int
	for _, summaries := range details {
		for _, summary := range summaries {
			nonGATotal += summary.Stats.Total()
			nonGARuns += len(summary.JobRuns)
		}
	}
	require.Equal(t, 1, nonGATotal)
	require.Equal(t, 1, nonGARuns)
	// Neither a loaded-date marker, another window, nor another release's
	// snapshot establishes that this release's 30-day snapshot exists.
	createGARawData(t, dbc, release, 90, test.ID, job.ID, suite.ID, 7, 7, 0)
	createGARawData(t, dbc, "4.16", 30, test.ID, job.ID, suite.ID, 8, 8, 0)
	assertOrdinaryData()

	createGARawData(t, dbc, release, 30, test.ID, job.ID, suite.ID, 9, 8, 0)
	require.NoError(t, dbc.DB.Exec("UPDATE release_definitions SET ga_data_loaded_date = NULL WHERE release = ?", release).Error)
	status, errs := provider.QueryBaseTestStatus(ctx, opts)
	require.Empty(t, errs)
	require.Equal(t, 9, status[key.Encode()].TotalCount, "an existing snapshot remains usable without a loaded-date marker")
	combined, _, errs := provider.QueryTestStatus(ctx, opts)
	require.Empty(t, errs)
	require.Equal(t, 9, combined[key.Encode()].TotalCount)
	details, errs = provider.QueryBaseJobRunTestStatus(ctx, opts)
	require.Empty(t, errs)
	var total, runs int
	for _, summaries := range details {
		for _, summary := range summaries {
			total += summary.Stats.Total()
			runs += len(summary.JobRuns)
		}
	}
	require.Equal(t, 9, total)
	require.Zero(t, runs, "the GA-only optimization applies when the snapshot exists")
	require.NoError(t, dbc.DB.Exec("UPDATE release_definitions SET ga_data_loaded_date = ? WHERE release = ?", gaDate.AddDays(-10), release).Error)
	status, errs = provider.QueryBaseTestStatus(ctx, opts)
	require.Empty(t, errs)
	require.Equal(t, 9, status[key.Encode()].TotalCount, "a stale loaded-date marker does not hide an existing snapshot")

	require.NoError(t, dbc.DB.Exec("ALTER TABLE prow_ga_raw_test_data RENAME TO unavailable_ga_snapshot").Error)
	_, errs = provider.QueryBaseTestStatus(ctx, opts)
	require.NotEmpty(t, errs, "an existence query error is not evidence that a snapshot is absent")
	_, _, errs = provider.QueryTestStatus(ctx, opts)
	require.NotEmpty(t, errs)
	_, errs = provider.QueryBaseJobRunTestStatus(ctx, opts)
	require.NotEmpty(t, errs)
}

// TestGARawOnlyDetailsKeepZeroSnapshotStats covers raw keys and jobs that
// were not present when the release's GA snapshot was taken.
func TestGARawOnlyDetailsKeepZeroSnapshotStats(t *testing.T) {
	ctx := context.Background()
	dbc := crTestDB(t)
	release := "4.15"
	gaDate := civil.Date{Year: 2024, Month: 3, Day: 1}
	createReleaseDefinition(t, dbc, release, &gaDate)
	vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
	snapshotJob := createProwJobWithVC(t, dbc, "periodic-ga-snapshot-job", release, vc)
	rawJob := createProwJobWithVC(t, dbc, "periodic-ga-new-job", release, vc)
	suite := intutil.CreateSuite(t, dbc, "openshift-tests-ga-raw-only")
	snapshotTest := intutil.CreateTest(t, dbc, "snapshot test")
	rawTest := intutil.CreateTest(t, dbc, "raw-only test")
	snapshotOwnership := createTestOwnership(t, dbc, snapshotTest.ID, &suite.ID, "snapshot-key", "Storage", []string{"PVC"})
	rawOwnership := createTestOwnership(t, dbc, rawTest.ID, &suite.ID, "raw-only-key", "Storage", []string{"PVC"})
	createGARawData(t, dbc, release, 30, snapshotTest.ID, snapshotJob.ID, suite.ID, 5, 4, 0)
	var runIDs []string
	for i, job := range []models.ProwJob{snapshotJob, rawJob} {
		at := gaDate.AddDays(-2 + i).In(time.UTC)
		run := createProwJobRunForCR(t, dbc, job.ID, release, at)
		createProwJobRunTest(t, dbc, run.ID, job.ID, rawTest.ID, &suite.ID, 1, release, at)
		if i == 0 {
			createProwJobRunTest(t, dbc, run.ID, job.ID, snapshotTest.ID, &suite.ID, 1, release, at)
		}
		runIDs = append(runIDs, strconv.FormatUint(uint64(run.ID), 10))
	}
	variants := map[string]string{"Platform": "aws", "Network": "ovn"}
	rawID := reqopts.TestIdentification{TestID: rawOwnership.UniqueID, RequestedVariants: variants}
	snapshotID := reqopts.TestIdentification{TestID: snapshotOwnership.UniqueID, RequestedVariants: variants}
	opts := defaultReqOptions(release)
	opts.BaseRelease = reqopts.Release{Name: release, Start: gaDate.AddDays(-30).In(time.UTC), End: gaDate.In(time.UTC)}
	opts.VariantOption.IncludeVariants = map[string][]string{"Platform": {"aws"}, "Network": {"ovn"}}
	provider := postgres.NewPostgresProvider(dbc, nil)
	for _, tc := range []struct {
		name           string
		identification []reqopts.TestIdentification
		wantSnapshot   bool
	}{
		{"mixed snapshot and raw-only keys", []reqopts.TestIdentification{snapshotID, rawID}, true},
		{"empty requested aggregate", []reqopts.TestIdentification{rawID}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := opts
			request.TestIDOptions = tc.identification
			result, errs := provider.QueryBaseJobRunTestStatus(ctx, request)
			require.Empty(t, errs)
			var rawRuns, snapshotRuns []string
			var snapshotTotal int
			for _, summaries := range result {
				for _, summary := range summaries {
					switch summary.TestKey.TestID {
					case rawOwnership.UniqueID:
						require.Equal(t, crtest.Stats{}, summary.Stats)
						require.Equal(t, "raw-only test", summary.TestName)
						require.Equal(t, "Storage", summary.JiraComponent)
						require.Contains(t, []string{snapshotJob.Name, rawJob.Name}, summary.ProwJob)
						for _, run := range summary.JobRuns {
							rawRuns = append(rawRuns, run.ProwJobRunID)
						}
					case snapshotOwnership.UniqueID:
						snapshotTotal += summary.Stats.Total()
						for _, run := range summary.JobRuns {
							snapshotRuns = append(snapshotRuns, run.ProwJobRunID)
						}
					default:
						t.Fatalf("unexpected test key: %s", summary.TestKey.TestID)
					}
				}
			}
			require.ElementsMatch(t, runIDs, rawRuns)
			if tc.wantSnapshot {
				require.Equal(t, 5, snapshotTotal)
				require.Equal(t, []string{runIDs[0]}, snapshotRuns)
			} else {
				require.Zero(t, snapshotTotal)
				require.Empty(t, snapshotRuns)
			}

			request.SkipGABaseJobRuns = true
			statsOnly, errs := provider.QueryBaseJobRunTestStatus(ctx, request)
			require.Empty(t, errs)
			var statsOnlyTotal int
			for _, summaries := range statsOnly {
				for _, summary := range summaries {
					require.Equal(t, snapshotOwnership.UniqueID, summary.TestKey.TestID)
					require.Empty(t, summary.JobRuns)
					statsOnlyTotal += summary.Stats.Total()
				}
			}
			require.Equal(t, snapshotTotal, statsOnlyTotal)
		})
	}
	// Exercise the report assembly as well as the provider's raw-only output.
	sampleAt := time.Date(2024, 6, 5, 12, 0, 0, 0, time.UTC)
	sampleRun := createProwJobRunForCR(t, dbc, rawJob.ID, release, sampleAt)
	createProwJobRunTest(t, dbc, sampleRun.ID, rawJob.ID, rawTest.ID, &suite.ID, 1, release, sampleAt)
	opts.TestIDOptions = []reqopts.TestIdentification{rawID}
	generator := componentreadiness.NewComponentReportGenerator(provider, opts, dbc, nil, "")
	report, errs := generator.GenerateTestDetailsReport(ctx)
	require.Empty(t, errs)
	require.Len(t, report.Analyses, 1)
	require.Equal(t, 0, report.Analyses[0].BaseStats.Total())
	var reportRunIDs []string
	for _, job := range report.Analyses[0].JobStats {
		for _, run := range job.BaseJobRunStats {
			reportRunIDs = append(reportRunIDs, run.JobRunID)
		}
	}
	require.ElementsMatch(t, runIDs, reportRunIDs)

	// A stats-only request must not query the raw tables, even for an empty aggregate.
	require.NoError(t, dbc.DB.Exec("ALTER TABLE prow_job_run_tests RENAME TO unavailable_run_tests").Error)
	opts.SkipGABaseJobRuns = true
	result, errs := provider.QueryBaseJobRunTestStatus(ctx, opts)
	require.Empty(t, errs)
	require.Empty(t, result)
}

// TestGAPostgresSnapshotParity checks that GA aggregation and raw details
// count the same eligible rows, including flakes and the final GA day.
func TestGAPostgresSnapshotParity(t *testing.T) {
	dbc := crTestDB(t)
	release := "4.15"
	gaDate := civil.DateOf(time.Now().UTC()).AddDays(-3)
	prepareGAPartitionedTables(t, dbc, release, gaDate)
	createReleaseDefinition(t, dbc, release, &gaDate)
	vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
	otherVC := createVariantCombination(t, dbc, []string{"Platform:gcp", "Network:ovn"})
	job := createProwJobWithVC(t, dbc, "periodic-ga-parity-aws", release, vc)
	otherJob := createProwJobWithVC(t, dbc, "periodic-ga-parity-gcp", release, otherVC)
	test := intutil.CreateTest(t, dbc, "openshift-tests:ga-parity")
	suite := intutil.CreateSuite(t, dbc, "openshift-tests-ga-parity")
	ownership := createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:ga-parity", "Storage", []string{"PVC"})
	gaDay := gaDate.In(time.UTC)
	cases := []struct {
		jobID          uint
		at             time.Time
		status         int
		infra, deleted bool
	}{
		{job.ID, gaDay.Add(-31 * 24 * time.Hour), 12, false, false},
		{job.ID, gaDay.Add(-2 * 24 * time.Hour), 1, false, false},
		{job.ID, gaDay.Add(-24 * time.Hour), 1, false, false},
		{job.ID, gaDay.Add(12 * time.Hour), 12, false, false},
		{job.ID, gaDay.Add(23 * time.Hour), 13, false, false},
		{job.ID, gaDay.Add(13 * time.Hour), 12, true, false},
		{job.ID, gaDay.Add(14 * time.Hour), 12, false, true},
		{job.ID, gaDay.Add(24 * time.Hour), 12, false, false},
		{otherJob.ID, gaDay.Add(15 * time.Hour), 1, false, false},
	}
	for _, tc := range cases {
		run := createProwJobRunForCR(t, dbc, tc.jobID, release, tc.at)
		createProwJobRunTest(t, dbc, run.ID, tc.jobID, test.ID, &suite.ID, tc.status, release, tc.at)
		if tc.infra {
			setJobRunLabels(t, dbc, run.ID, []string{"InfraFailure"})
		}
		if tc.deleted {
			require.NoError(t, dbc.DB.Model(&models.ProwJobRunTest{}).Where("prow_job_run_id = ?", run.ID).Update("deleted_at", time.Now()).Error)
		}
	}
	loader := gateststatus.New(context.Background(), dbc, false, []string{release})
	require.NotPanics(t, loader.Load, "GA loading must use PostgreSQL without a BigQuery client")
	require.Empty(t, loader.Errors())
	assertGASnapshotMatchesRaw(t, dbc, release, gaDate)
	for windowDays, wantRuns := range map[int]int64{1: 3, 30: 4, 90: 5} {
		var windowSnapshot models.ProwGARawTestDatum
		require.NoError(t, dbc.DB.Where("release = ? AND window_days = ? AND prow_job_id = ?", release, windowDays, job.ID).Take(&windowSnapshot).Error)
		assert.EqualValues(t, wantRuns, windowSnapshot.Runs)
	}

	var snapshot models.ProwGARawTestDatum
	require.NoError(t, dbc.DB.Where("release = ? AND window_days = ? AND prow_job_id = ?", release, 30, job.ID).Take(&snapshot).Error)
	assert.EqualValues(t, 4, snapshot.Runs)
	assert.EqualValues(t, 2, snapshot.Passes)
	assert.EqualValues(t, 1, snapshot.Failures)
	assert.EqualValues(t, 1, snapshot.Flakes)

	opts := defaultReqOptions(release)
	opts.BaseRelease = reqopts.Release{Name: release, Start: utils.GAWindowStart(gaDate, 30).In(time.UTC), End: gaDay}
	opts.VariantOption.IncludeVariants = map[string][]string{"Platform": {"aws"}, "Network": {"ovn"}}
	opts.TestIDOptions = []reqopts.TestIdentification{{TestID: ownership.UniqueID, RequestedVariants: map[string]string{"Platform": "aws", "Network": "ovn"}}}
	provider := postgres.NewPostgresProvider(dbc, nil)
	statuses, errs := provider.QueryBaseTestStatus(context.Background(), opts)
	require.Empty(t, errs)
	key := crtest.KeyWithVariants{TestID: ownership.UniqueID, Variants: map[string]string{"Platform": "aws", "Network": "ovn"}}
	status, ok := statuses[key.Encode()]
	require.True(t, ok)
	assert.Equal(t, 4, status.TotalCount)
	assert.Equal(t, 2, status.SuccessCount)
	assert.Equal(t, 1, status.FlakeCount)
	details, errs := provider.QueryBaseJobRunTestStatus(context.Background(), opts)
	require.Empty(t, errs)
	var total, passed, flakes int
	for _, summaries := range details {
		for _, summary := range summaries {
			for _, run := range summary.JobRuns {
				total += run.TotalCount
				passed += run.SuccessCount
				flakes += run.FlakeCount
			}
		}
	}
	assert.Equal(t, status.TotalCount, total)
	assert.Equal(t, 3, passed, "raw details count the flake as both a success and a flake")
	assert.Equal(t, status.SuccessCount, passed-flakes, "GA passes exclude flakes")
	assert.Equal(t, status.FlakeCount, flakes)
	var rawDetailStats crtest.Stats
	for _, summaries := range details {
		for _, summary := range summaries {
			if summary.TestKeyStr == key.Encode() {
				rawDetailStats = summary.Stats
			}
		}
	}
	require.Equal(t, crtest.NewTestStats(2, 1, 1, false), rawDetailStats)

	// With raw test rows unavailable, test details retain the exclusive snapshot counts.
	aggregateDetailMarker := time.Now().UTC()
	deleteResult := dbc.DB.Model(&models.ProwJobRunTest{}).
		Where("prow_job_run_release = ? AND test_id = ? AND deleted_at IS NULL", release, test.ID).
		Update("deleted_at", aggregateDetailMarker)
	require.NoError(t, deleteResult.Error)
	require.Positive(t, deleteResult.RowsAffected)
	aggregateDetails, errs := provider.QueryBaseJobRunTestStatus(context.Background(), opts)
	require.Empty(t, errs)
	var aggregateDetailStats crtest.Stats
	for _, summaries := range aggregateDetails {
		for _, summary := range summaries {
			if summary.TestKeyStr == key.Encode() {
				aggregateDetailStats = summary.Stats
			}
		}
	}
	assert.Equal(t, rawDetailStats, aggregateDetailStats)
	assert.Equal(t, crtest.NewTestStats(2, 1, 1, false), aggregateDetailStats)
	restoreResult := dbc.DB.Unscoped().Model(&models.ProwJobRunTest{}).
		Where("prow_job_run_release = ? AND test_id = ? AND deleted_at = ?", release, test.ID, aggregateDetailMarker).
		Update("deleted_at", nil)
	require.NoError(t, restoreResult.Error)
	require.Equal(t, deleteResult.RowsAffected, restoreResult.RowsAffected)

	// A corrected raw status does not interrupt GA reads before an explicit rebuild.
	require.NoError(t, dbc.DB.Model(&models.ProwJobRunTest{}).
		Where("prow_job_run_release = ? AND prow_job_run_timestamp = ?", release, gaDay.Add(12*time.Hour)).
		Update("status", 13).Error)
	statuses, errs = provider.QueryBaseTestStatus(context.Background(), opts)
	require.Empty(t, errs)
	require.Equal(t, 1, statuses[key.Encode()].FlakeCount)
	_, errs = provider.QueryBaseJobRunTestStatus(context.Background(), opts)
	require.Empty(t, errs)
	base, _, errs := provider.QueryTestStatus(context.Background(), opts)
	require.Empty(t, errs)
	require.Equal(t, 1, base[key.Encode()].FlakeCount)
	loader = gateststatus.New(context.Background(), dbc, false, []string{release})
	loader.Load()
	require.Empty(t, loader.Errors())
	require.NoError(t, dbc.DB.Where("release = ? AND window_days = ? AND prow_job_id = ?", release, 30, job.ID).Take(&snapshot).Error)
	assert.EqualValues(t, 1, snapshot.Flakes, "ordinary load must preserve the snapshot")
	loader = gateststatus.New(context.Background(), dbc, true, []string{release})
	loader.Load()
	require.Empty(t, loader.Errors())
	require.NoError(t, dbc.DB.Where("release = ? AND window_days = ? AND prow_job_id = ?", release, 30, job.ID).Take(&snapshot).Error)
	assert.EqualValues(t, 4, snapshot.Runs)
	assert.EqualValues(t, 2, snapshot.Passes)
	assert.EqualValues(t, 0, snapshot.Failures)
	assert.EqualValues(t, 2, snapshot.Flakes)
	assertGASnapshotMatchesRaw(t, dbc, release, gaDate)

	// If all raw rows become ineligible, preserve a previously populated snapshot.
	require.NoError(t, dbc.DB.Exec("UPDATE prow_job_runs SET labels = ARRAY['InfraFailure'] WHERE prow_job_release = ?", release).Error)
	loader = gateststatus.New(context.Background(), dbc, true, []string{release})
	loader.Load()
	require.NotEmpty(t, loader.Errors())
	require.ErrorContains(t, loader.Errors()[0], "no eligible")
	var afterEmpty models.ProwGARawTestDatum
	require.NoError(t, dbc.DB.Where("release = ? AND window_days = ? AND prow_job_id = ?", release, 30, job.ID).Take(&afterEmpty).Error)
	assert.Equal(t, snapshot, afterEmpty)
	require.NoError(t, dbc.DB.Exec("UPDATE prow_job_runs SET labels = NULL WHERE prow_job_release = ? AND timestamp <> ?", release, gaDay.Add(13*time.Hour)).Error)
	loader = gateststatus.New(context.Background(), dbc, true, []string{release})
	loader.Load()
	require.Empty(t, loader.Errors())
	require.NoError(t, dbc.DB.Where("release = ? AND window_days = ? AND prow_job_id = ?", release, 30, job.ID).Take(&snapshot).Error)

	// An incomplete restored window must leave the last snapshot untouched.
	missing := utils.GAWindowStart(gaDate, 90).In(time.UTC).Format("2006_01_02")
	require.NoError(t, dbc.DB.Exec(fmt.Sprintf("ALTER TABLE prow_job_run_tests_p4_15 DETACH PARTITION prow_job_run_tests_p4_15_p%s", missing)).Error)
	loader = gateststatus.New(context.Background(), dbc, true, []string{release})
	loader.Load()
	require.Empty(t, loader.Errors(), "missing historical partitions must not fail a forced refresh when a snapshot exists")
	var preserved models.ProwGARawTestDatum
	require.NoError(t, dbc.DB.Where("release = ? AND window_days = ? AND prow_job_id = ?", release, 30, job.ID).Take(&preserved).Error)
	assert.Equal(t, snapshot, preserved)
	expiredDate := civil.DateOf(time.Now().UTC()).AddDays(-400)
	require.NoError(t, dbc.DB.Model(&models.ReleaseDefinition{}).Where("release = ?", release).Update("ga_date", expiredDate).Error)
	expiredLoader := gateststatus.New(context.Background(), dbc, false, []string{release})
	expiredLoader.Load()
	require.Empty(t, expiredLoader.Errors())
	require.NoError(t, dbc.DB.Where("release = ? AND window_days = ? AND prow_job_id = ?", release, 30, job.ID).Take(&preserved).Error)
	assert.Equal(t, snapshot, preserved)
	require.NoError(t, dbc.DB.Exec("ALTER TABLE release_definitions RENAME TO unavailable_release_definitions").Error)
	_, errs = provider.QueryBaseTestStatus(context.Background(), opts)
	require.Empty(t, errs, "missing GA-date metadata must fall back to the prefix-sum query")
}

// TestGAExpiredReleasesDoNotBlockOtherSnapshots verifies that expired raw
// windows without loaded-date markers are skipped without losing snapshots.
func TestGAExpiredReleasesDoNotBlockOtherSnapshots(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(fmt.Sprintf("force=%t", force), func(t *testing.T) {
			dbc := crTestDB(t)
			oldDate := civil.DateOf(time.Now().UTC()).AddDays(-400)
			recentDate := civil.DateOf(time.Now().UTC()).AddDays(-3)
			prepareGAPartitionedTables(t, dbc, "4.15", recentDate)
			createReleaseDefinition(t, dbc, "4.13", &oldDate)
			createReleaseDefinition(t, dbc, "4.14", &oldDate)
			createReleaseDefinition(t, dbc, "4.15", &recentDate)
			oldJob := intutil.CreateProwJob(t, dbc, "old-ga-job", "4.14", nil)
			recentJob := intutil.CreateProwJob(t, dbc, "recent-ga-job", "4.15", nil)
			test := intutil.CreateTest(t, dbc, "ga-expired")
			suite := intutil.CreateSuite(t, dbc, "ga-expired")
			createGARawData(t, dbc, "4.14", 30, test.ID, oldJob.ID, suite.ID, 7, 5, 1)
			require.NoError(t, dbc.DB.Exec("UPDATE release_definitions SET ga_data_loaded_date = NULL WHERE release = ?", "4.14").Error)
			var original models.ProwGARawTestDatum
			require.NoError(t, dbc.DB.Where("release = ?", "4.14").Take(&original).Error)
			at := recentDate.In(time.UTC).Add(time.Hour)
			run := createProwJobRunForCR(t, dbc, recentJob.ID, "4.15", at)
			createProwJobRunTest(t, dbc, run.ID, recentJob.ID, test.ID, &suite.ID, 1, "4.15", at)

			loader := gateststatus.New(context.Background(), dbc, force, nil)
			loader.Load()
			require.Empty(t, loader.Errors())
			var count int64
			require.NoError(t, dbc.DB.Model(&models.ProwGARawTestDatum{}).Where("release = ?", "4.13").Count(&count).Error)
			require.Zero(t, count)
			var preserved models.ProwGARawTestDatum
			require.NoError(t, dbc.DB.Where("release = ?", "4.14").Take(&preserved).Error)
			require.Equal(t, original, preserved)
			var recent models.ProwGARawTestDatum
			require.NoError(t, dbc.DB.Where("release = ? AND window_days = ?", "4.15", 30).Take(&recent).Error)
			require.EqualValues(t, 1, recent.Runs)
			for _, release := range []string{"4.13", "4.14"} {
				var definition models.ReleaseDefinition
				require.NoError(t, dbc.DB.Where("release = ?", release).Take(&definition).Error)
				require.Nil(t, definition.LoadedGADate)
			}
			var recentDefinition models.ReleaseDefinition
			require.NoError(t, dbc.DB.Where("release = ?", "4.15").Take(&recentDefinition).Error)
			require.Equal(t, &recentDate, recentDefinition.LoadedGADate)
		})
	}
}

// TestGARetentionCutoffPreservesSnapshot covers a missing partition on the
// cutoff day that partition cleanup can remove.
func TestGARetentionCutoffPreservesSnapshot(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(fmt.Sprintf("force=%t", force), func(t *testing.T) {
			dbc := crTestDB(t)
			release := "4.15"
			gaDate := civil.DateOf(time.Now().UTC()).AddDays(-db.DefaultPartitionRetentionDays + 90)
			prepareGAPartitionedTables(t, dbc, release, gaDate)
			createReleaseDefinition(t, dbc, release, &gaDate)
			job := intutil.CreateProwJob(t, dbc, "cutoff-ga-job", release, nil)
			test := intutil.CreateTest(t, dbc, "ga-cutoff")
			suite := intutil.CreateSuite(t, dbc, "ga-cutoff")
			createGARawData(t, dbc, release, 30, test.ID, job.ID, suite.ID, 7, 5, 1)
			require.NoError(t, dbc.DB.Exec("UPDATE release_definitions SET ga_data_loaded_date = NULL WHERE release = ?", release).Error)
			var original models.ProwGARawTestDatum
			require.NoError(t, dbc.DB.Where("release = ?", release).Take(&original).Error)

			missing := gaDate.AddDays(-90).In(time.UTC).Format("2006_01_02")
			require.NoError(t, dbc.DB.Exec(fmt.Sprintf("ALTER TABLE prow_job_run_tests_p4_15 DETACH PARTITION prow_job_run_tests_p4_15_p%s", missing)).Error)
			loader := gateststatus.New(context.Background(), dbc, force, []string{release})
			loader.Load()
			require.Empty(t, loader.Errors())
			var preserved models.ProwGARawTestDatum
			require.NoError(t, dbc.DB.Where("release = ?", release).Take(&preserved).Error)
			require.Equal(t, original, preserved)
			var definition models.ReleaseDefinition
			require.NoError(t, dbc.DB.Where("release = ?", release).Take(&definition).Error)
			require.Nil(t, definition.LoadedGADate)
		})
	}
}

// TestGAForceRebuildsRestoredExpiredWindow verifies that a forced rebuild can
// replace an old snapshot after the complete historical partitions are restored.
func TestGAForceRebuildsRestoredExpiredWindow(t *testing.T) {
	for _, tc := range []struct {
		name   string
		offset int
	}{
		{"expired", -400},
		{"retention cutoff", -db.DefaultPartitionRetentionDays + 90},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dbc := crTestDB(t)
			release := "4.14"
			gaDate := civil.DateOf(time.Now().UTC()).AddDays(tc.offset)
			prepareGAPartitionedTables(t, dbc, release, gaDate)
			createReleaseDefinition(t, dbc, release, &gaDate)
			job := intutil.CreateProwJob(t, dbc, "restored-ga-job", release, nil)
			test := intutil.CreateTest(t, dbc, "ga-restored")
			suite := intutil.CreateSuite(t, dbc, "ga-restored")
			createGARawData(t, dbc, release, 30, test.ID, job.ID, suite.ID, 7, 5, 1)
			require.NoError(t, dbc.DB.Exec("UPDATE release_definitions SET ga_data_loaded_date = NULL WHERE release = ?", release).Error)
			at := gaDate.In(time.UTC).Add(time.Hour)
			run := createProwJobRunForCR(t, dbc, job.ID, release, at)
			createProwJobRunTest(t, dbc, run.ID, job.ID, test.ID, &suite.ID, 1, release, at)
			loader := gateststatus.New(context.Background(), dbc, true, nil)
			loader.Load()
			require.Empty(t, loader.Errors())
			var snapshot models.ProwGARawTestDatum
			require.NoError(t, dbc.DB.Where("release = ? AND window_days = ?", release, 30).Take(&snapshot).Error)
			require.EqualValues(t, 1, snapshot.Runs)
			require.EqualValues(t, 1, snapshot.Passes)
			var definition models.ReleaseDefinition
			require.NoError(t, dbc.DB.Where("release = ?", release).Take(&definition).Error)
			require.Equal(t, &gaDate, definition.LoadedGADate)
		})
	}
}

// TestGARegressionTrackingSkipsFallbackRawRuns checks the loader's batched
// fallback path while confirming sample failures are still tracked.
func TestGARegressionTrackingSkipsFallbackRawRuns(t *testing.T) {
	dbc := crTestDB(t)
	fallbackGA := civil.Date{Year: 2024, Month: 3, Day: 1}
	baseGA := civil.Date{Year: 2024, Month: 6, Day: 1}
	createReleaseDefinition(t, dbc, "4.15", &fallbackGA)
	createReleaseDefinition(t, dbc, "4.16", &baseGA)
	createReleaseDefinition(t, dbc, "4.17", nil)
	vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
	fallbackJob := createProwJobWithVC(t, dbc, "periodic-fallback-4.15-aws", "4.15", vc)
	baseJob := createProwJobWithVC(t, dbc, "periodic-fallback-4.16-aws", "4.16", vc)
	sampleJob := createProwJobWithVC(t, dbc, "periodic-fallback-4.17-aws", "4.17", vc)
	test := intutil.CreateTest(t, dbc, "openshift-tests:fallback-tracking")
	suite := intutil.CreateSuite(t, dbc, "openshift-tests-fallback-tracking")
	ownership := createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:fallback-tracking", "Storage", []string{"PVC"})
	createGARawData(t, dbc, "4.15", 30, test.ID, fallbackJob.ID, suite.ID, 100, 100, 0)
	createGARawData(t, dbc, "4.16", 30, test.ID, baseJob.ID, suite.ID, 100, 80, 0)
	fallbackAt := fallbackGA.AddDays(-1).In(time.UTC)
	run := createProwJobRunForCR(t, dbc, fallbackJob.ID, "4.15", fallbackAt)
	createProwJobRunTest(t, dbc, run.ID, fallbackJob.ID, test.ID, &suite.ID, 1, "4.15", fallbackAt)
	sampleStart := time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)
	for i := range 10 {
		at := sampleStart.Add(time.Duration(i+1) * time.Hour)
		run := createProwJobRunForCR(t, dbc, sampleJob.ID, "4.17", at)
		createProwJobRunTest(t, dbc, run.ID, sampleJob.ID, test.ID, &suite.ID, 12, "4.17", at)
	}
	createCumulativeSummary(t, dbc, civil.Date{Year: 2024, Month: 6, Day: 30}, "4.17", test.ID, sampleJob.ID, suite.ID, 0, 0, 0)
	createCumulativeSummary(t, dbc, civil.Date{Year: 2024, Month: 7, Day: 2}, "4.17", test.ID, sampleJob.ID, suite.ID, 10, 0, 0)

	var mu sync.Mutex
	var fallbackReads, sampleReads int
	require.NoError(t, dbc.DB.Callback().Row().Before("gorm:row").Register("count-detail-reads", func(tx *gorm.DB) {
		if !strings.Contains(tx.Statement.SQL.String(), "JOIN prow_job_run_tests pjrt") {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		for _, arg := range tx.Statement.Vars {
			release, ok := arg.(string)
			if !ok {
				continue
			}
			if release == "4.15" {
				fallbackReads++
				break
			}
			if release == "4.17" {
				sampleReads++
				break
			}
		}
	}))
	base := reqopts.Release{Name: "4.16", Start: baseGA.AddDays(-30).In(time.UTC), End: baseGA.In(time.UTC)}
	sample := reqopts.Release{Name: "4.17", Start: sampleStart, End: sampleStart.Add(48 * time.Hour)}
	opts := defaultReqOptions("4.17")
	opts.AdvancedOption.Confidence = 95
	opts.AdvancedOption.IncludeMultiReleaseAnalysis = true
	view := crview.View{Name: "fallback-tracking", BaseRelease: reqopts.RelativeRelease{Release: base, RelativeStart: base.Start.Format(time.RFC3339), RelativeEnd: base.End.Format(time.RFC3339)}, SampleRelease: reqopts.RelativeRelease{Release: sample, RelativeStart: sample.Start.Format(time.RFC3339), RelativeEnd: sample.End.Format(time.RFC3339)}, VariantOptions: opts.VariantOption, AdvancedOptions: opts.AdvancedOption, RegressionTracking: crview.RegressionTracking{Enabled: true}}
	configs := []v1.Release{{Release: "4.16", GADate: &baseGA, PreviousRelease: "4.15"}, {Release: "4.15", GADate: &fallbackGA}, {Release: "4.17"}}
	provider := postgres.NewPostgresProvider(dbc, nil)
	loader, err := regressioncacheloader.New(dbc, provider, nil, []crview.View{view}, configs, 0, 0, componentreadiness.NewPostgresRegressionStore(dbc, nil))
	require.NoError(t, err)
	loader.Load()
	require.Empty(t, loader.Errors())
	var reg models.TestRegression
	require.NoError(t, dbc.DB.Preload("JobRuns").Where("test_id = ?", ownership.UniqueID).First(&reg).Error)
	assert.Equal(t, "4.15", reg.BaseRelease, "fixture must select the fallback release")
	assert.Len(t, reg.JobRuns, 10, "sample failures must still be recorded")
	mu.Lock()
	defer mu.Unlock()
	assert.Positive(t, sampleReads, "fixture must execute the sample detail query")
	assert.Zero(t, fallbackReads, "stats-only fallback must not query raw job runs")
}

// assertGASnapshotMatchesRaw compares all keys and windows against an
// independent aggregation over retained rows.
func assertGASnapshotMatchesRaw(t *testing.T, dbc *db.DB, release string, gaDate civil.Date) {
	t.Helper()
	var raw, snapshot []models.ProwGARawTestDatum
	err := dbc.DB.Raw(`
        SELECT ? AS release, w.window_days, t.test_id, r.prow_job_id, COALESCE(t.suite_id, 0) AS suite_id,
               COUNT(*) AS runs, COUNT(*) FILTER (WHERE t.status = 1) AS passes,
               COUNT(*) FILTER (WHERE t.status NOT IN (1,13) OR t.status IS NULL) AS failures,
               COUNT(*) FILTER (WHERE t.status = 13) AS flakes
        FROM (VALUES (1),(30),(90)) w(window_days)
        JOIN prow_job_run_tests t ON t.prow_job_run_release = ?
          AND t.prow_job_run_timestamp >= ?::timestamptz - w.window_days * interval '1 day'
          AND t.prow_job_run_timestamp < ?::timestamptz
        JOIN prow_job_runs r ON r.id = t.prow_job_run_id AND r.prow_job_release = t.prow_job_run_release
          AND r.timestamp = t.prow_job_run_timestamp
        WHERE t.deleted_at IS NULL AND r.deleted_at IS NULL
          AND (r.labels IS NULL OR NOT r.labels @> ARRAY['InfraFailure'])
        GROUP BY w.window_days, t.test_id, r.prow_job_id, COALESCE(t.suite_id, 0)
        ORDER BY w.window_days, t.test_id, r.prow_job_id, COALESCE(t.suite_id, 0)`,
		release, release, gaDate.In(time.UTC), utils.GAWindowEnd(gaDate).In(time.UTC)).Scan(&raw).Error
	require.NoError(t, err)
	require.NoError(t, dbc.DB.Where("release = ?", release).Order("window_days, test_id, prow_job_id, suite_id").Find(&snapshot).Error)
	require.Equal(t, raw, snapshot)
}

// prepareGAPartitionedTables replaces the generic integration fixture's
// unpartitioned raw tables with release/date partitions and installs the real
// migration. The original tables retain the sequences used by copied defaults.
func prepareGAPartitionedTables(t *testing.T, dbc *db.DB, release string, gaDate civil.Date) {
	t.Helper()
	for _, table := range []struct{ name, releaseColumn, dateColumn string }{
		{"prow_job_runs", "prow_job_release", "timestamp"},
		{"prow_job_run_tests", "prow_job_run_release", "prow_job_run_timestamp"},
	} {
		require.NoError(t, dbc.DB.Exec("ALTER TABLE "+table.name+" RENAME TO fixture_original_"+table.name).Error)
		require.NoError(t, dbc.DB.Exec("CREATE TABLE "+table.name+" (LIKE fixture_original_"+table.name+" INCLUDING DEFAULTS INCLUDING IDENTITY) PARTITION BY LIST ("+table.releaseColumn+")").Error)
		_, err := dbc.GoparPartitions.CreateMissingPartitionsListToRange(table.name, []string{release},
			utils.GAWindowStart(gaDate, 90).In(time.UTC), utils.GAWindowEnd(gaDate).In(time.UTC), table.dateColumn, true, false)
		require.NoError(t, err)
	}
}

// TestGAHistoricalRunReleaseSurvivesJobReattribution checks two jobs sharing
// one test and variant key when current job attribution changes after snapshot.
func TestGAHistoricalRunReleaseSurvivesJobReattribution(t *testing.T) {
	dbc := crTestDB(t)
	release := "4.15"
	gaDate := civil.DateOf(time.Now().UTC()).AddDays(-3)
	prepareGAPartitionedTables(t, dbc, release, gaDate)
	createReleaseDefinition(t, dbc, release, &gaDate)
	vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
	job1 := createProwJobWithVC(t, dbc, "periodic-ga-reattributed", release, vc)
	job2 := createProwJobWithVC(t, dbc, "periodic-ga-stable", release, vc)
	test := intutil.CreateTest(t, dbc, "openshift-tests:ga-reattribution")
	suite := intutil.CreateSuite(t, dbc, "openshift-tests-ga-reattribution")
	ownership := createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:ga-reattribution", "Storage", []string{"PVC"})
	for _, job := range []models.ProwJob{job1, job2} {
		at := gaDate.In(time.UTC).Add(time.Hour)
		run := createProwJobRunForCR(t, dbc, job.ID, release, at)
		createProwJobRunTest(t, dbc, run.ID, job.ID, test.ID, &suite.ID, 1, release, at)
	}
	loader := gateststatus.New(context.Background(), dbc, false, []string{release})
	loader.Load()
	require.Empty(t, loader.Errors())
	require.NoError(t, dbc.DB.Model(&models.ProwJob{}).Where("id = ?", job1.ID).Update("release", "4.16").Error)
	opts := defaultReqOptions(release)
	opts.BaseRelease = reqopts.Release{Name: release, Start: utils.GAWindowStart(gaDate, 30).In(time.UTC), End: gaDate.In(time.UTC)}
	opts.VariantOption.IncludeVariants = map[string][]string{"Platform": {"aws"}, "Network": {"ovn"}}
	opts.TestIDOptions = []reqopts.TestIdentification{{TestID: ownership.UniqueID, RequestedVariants: map[string]string{"Platform": "aws", "Network": "ovn"}}}
	provider := postgres.NewPostgresProvider(dbc, nil)
	statuses, errs := provider.QueryBaseTestStatus(context.Background(), opts)
	require.Empty(t, errs)
	key := crtest.KeyWithVariants{TestID: ownership.UniqueID, Variants: map[string]string{"Platform": "aws", "Network": "ovn"}}
	require.Equal(t, 2, statuses[key.Encode()].TotalCount)
	details, errs := provider.QueryBaseJobRunTestStatus(context.Background(), opts)
	require.Empty(t, errs)
	total := 0
	for _, summaries := range details {
		for _, summary := range summaries {
			for _, run := range summary.JobRuns {
				total += run.TotalCount
			}
		}
	}
	require.Equal(t, 2, total)
}

// TestGASnapshotWhenRawWindowMissing preserves counts until an explicit
// rebuild can use a complete raw window.
func TestGASnapshotWhenRawWindowMissing(t *testing.T) {
	dbc := crTestDB(t)
	release := "4.15"
	gaDate := civil.DateOf(time.Now().UTC()).AddDays(-10)
	prepareGAPartitionedTables(t, dbc, release, gaDate)
	createReleaseDefinition(t, dbc, release, &gaDate)
	vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
	job := createProwJobWithVC(t, dbc, "periodic-ga-preserved", release, vc)
	test := intutil.CreateTest(t, dbc, "openshift-tests:ga-preserved")
	suite := intutil.CreateSuite(t, dbc, "openshift-tests-ga-preserved")
	ownership := createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:ga-preserved", "Storage", []string{"PVC"})
	at := gaDate.In(time.UTC).Add(time.Hour)
	run := createProwJobRunForCR(t, dbc, job.ID, release, at)
	createProwJobRunTest(t, dbc, run.ID, job.ID, test.ID, &suite.ID, 1, release, at)
	createGARawData(t, dbc, release, 30, test.ID, job.ID, suite.ID, 99, 90, 2)
	opts := defaultReqOptions(release)
	opts.BaseRelease = reqopts.Release{Name: release, Start: utils.GAWindowStart(gaDate, 30).In(time.UTC), End: gaDate.In(time.UTC)}
	opts.VariantOption.IncludeVariants = map[string][]string{"Platform": {"aws"}, "Network": {"ovn"}}
	provider := postgres.NewPostgresProvider(dbc, nil)
	result, errs := provider.QueryBaseTestStatus(context.Background(), opts)
	require.Empty(t, errs)
	key := crtest.KeyWithVariants{TestID: ownership.UniqueID, Variants: map[string]string{"Platform": "aws", "Network": "ovn"}}
	require.Equal(t, 99, result[key.Encode()].TotalCount)

	start := utils.GAWindowStart(gaDate, 90).In(time.UTC)
	name := "prow_job_run_tests_p4_15_p" + start.Format("2006_01_02")
	require.NoError(t, dbc.DB.Exec("ALTER TABLE prow_job_run_tests_p4_15 DETACH PARTITION "+name).Error)
	result, errs = provider.QueryBaseTestStatus(context.Background(), opts)
	require.Empty(t, errs)
	require.Equal(t, 99, result[key.Encode()].TotalCount)
	opts.TestIDOptions = []reqopts.TestIdentification{{TestID: ownership.UniqueID, RequestedVariants: map[string]string{"Platform": "aws", "Network": "ovn"}}}
	details, errs := provider.QueryBaseJobRunTestStatus(context.Background(), opts)
	require.Empty(t, errs)
	var total, jobRuns int
	for _, summaries := range details {
		for _, summary := range summaries {
			total += summary.Stats.Total()
			jobRuns += len(summary.JobRuns)
		}
	}
	require.Equal(t, 99, total, "details use the preserved aggregate's exclusive result counts")
	require.Positive(t, jobRuns)
	loader := gateststatus.New(context.Background(), dbc, false, []string{release})
	loader.Load()
	require.Empty(t, loader.Errors())
	result, errs = provider.QueryBaseTestStatus(context.Background(), opts)
	require.Empty(t, errs)
	require.Equal(t, 99, result[key.Encode()].TotalCount)
	attach := fmt.Sprintf("ALTER TABLE prow_job_run_tests_p4_15 ATTACH PARTITION %s FOR VALUES FROM ('%s') TO ('%s')", name, start.Format("2006-01-02"), start.AddDate(0, 0, 1).Format("2006-01-02"))
	require.NoError(t, dbc.DB.Exec(attach).Error)
	loader = gateststatus.New(context.Background(), dbc, false, []string{release})
	loader.Load()
	require.Empty(t, loader.Errors())
	result, errs = provider.QueryBaseTestStatus(context.Background(), opts)
	require.Empty(t, errs)
	require.Equal(t, 99, result[key.Encode()].TotalCount, "ordinary loading must preserve a snapshot for the current GA date")
	loader = gateststatus.New(context.Background(), dbc, true, []string{release})
	loader.Load()
	require.Empty(t, loader.Errors())
	result, errs = provider.QueryBaseTestStatus(context.Background(), opts)
	require.Empty(t, errs)
	require.Equal(t, 1, result[key.Encode()].TotalCount)
}

// TestGAPostgresDetailsAfterPartialRetention verifies that snapshot counts and
// retained raw rows remain available after other partitions have expired.
func TestGAPostgresDetailsAfterPartialRetention(t *testing.T) {
	dbc := crTestDB(t)
	release := "4.15"
	gaDate := civil.DateOf(time.Now().UTC()).AddDays(-10)
	prepareGAPartitionedTables(t, dbc, release, gaDate)
	createReleaseDefinition(t, dbc, release, &gaDate)
	vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
	job := createProwJobWithVC(t, dbc, "periodic-ga-retained-details", release, vc)
	test := intutil.CreateTest(t, dbc, "openshift-tests:ga-retained-details")
	suite := intutil.CreateSuite(t, dbc, "openshift-tests-ga-retained-details")
	ownership := createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:ga-retained-details", "Storage", []string{"PVC"})
	for i, at := range []time.Time{gaDate.AddDays(-1).In(time.UTC).Add(time.Hour), gaDate.In(time.UTC).Add(time.Hour)} {
		run := createProwJobRunForCR(t, dbc, job.ID, release, at)
		createProwJobRunTest(t, dbc, run.ID, job.ID, test.ID, &suite.ID, []int{1, 12}[i], release, at)
	}
	loader := gateststatus.New(context.Background(), dbc, false, []string{release})
	loader.Load()
	require.Empty(t, loader.Errors())
	dayPartition := "prow_job_run_tests_p4_15_p" + gaDate.In(time.UTC).Format("2006_01_02")
	require.NoError(t, dbc.DB.Exec("ALTER TABLE prow_job_run_tests_p4_15 DETACH PARTITION "+dayPartition).Error)
	opts := defaultReqOptions(release)
	opts.BaseRelease = reqopts.Release{Name: release, Start: utils.GAWindowStart(gaDate, 30).In(time.UTC), End: gaDate.In(time.UTC)}
	opts.VariantOption.IncludeVariants = map[string][]string{"Platform": {"aws"}, "Network": {"ovn"}}
	opts.TestIDOptions = []reqopts.TestIdentification{{TestID: ownership.UniqueID, RequestedVariants: map[string]string{"Platform": "aws", "Network": "ovn"}}}
	provider := postgres.NewPostgresProvider(dbc, nil)
	details, errs := provider.QueryBaseJobRunTestStatus(context.Background(), opts)
	require.Empty(t, errs)
	var total, jobRuns int
	for _, summaries := range details {
		for _, summary := range summaries {
			total += summary.Stats.Total()
			jobRuns += len(summary.JobRuns)
		}
	}
	require.Equal(t, 2, total)
	require.Equal(t, 1, jobRuns)
	// If the release parent has expired entirely, details still use the
	// snapshot without raw job runs.
	require.NoError(t, dbc.DB.Exec("ALTER TABLE prow_job_runs DETACH PARTITION prow_job_runs_p4_15").Error)
	details, errs = provider.QueryBaseJobRunTestStatus(context.Background(), opts)
	require.Empty(t, errs)
	total, jobRuns = 0, 0
	for _, summaries := range details {
		for _, summary := range summaries {
			total += summary.Stats.Total()
			jobRuns += len(summary.JobRuns)
		}
	}
	require.Equal(t, 2, total)
	require.Zero(t, jobRuns)
}

// TestGANoncanonicalWindowParity checks that snapshot and raw details use the same UTC day boundaries.
func TestGANoncanonicalWindowParity(t *testing.T) {
	ctx := context.Background()
	dbc := crTestDB(t)
	release := "4.15"
	gaDate := civil.DateOf(time.Now().UTC()).AddDays(-3)
	prepareGAPartitionedTables(t, dbc, release, gaDate)
	createReleaseDefinition(t, dbc, release, &gaDate)
	vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
	job := createProwJobWithVC(t, dbc, "periodic-ga-noncanonical", release, vc)
	test := intutil.CreateTest(t, dbc, "openshift-tests:ga-noncanonical")
	suite := intutil.CreateSuite(t, dbc, "openshift-tests-ga-noncanonical")
	ownership := createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:ga-noncanonical", "Storage", []string{"PVC"})
	start := utils.GAWindowStart(gaDate, 30).In(time.UTC)
	for _, at := range []time.Time{start.Add(time.Hour), start.Add(13 * time.Hour)} {
		run := createProwJobRunForCR(t, dbc, job.ID, release, at)
		createProwJobRunTest(t, dbc, run.ID, job.ID, test.ID, &suite.ID, 1, release, at)
	}
	loader := gateststatus.New(ctx, dbc, false, []string{release})
	loader.Load()
	require.Empty(t, loader.Errors())
	provider := postgres.NewPostgresProvider(dbc, nil)
	opts := defaultReqOptions(release)
	opts.BaseRelease = reqopts.Release{Name: release, Start: start.Add(12 * time.Hour), End: gaDate.In(time.UTC).Add(12 * time.Hour)}
	opts.VariantOption.IncludeVariants = map[string][]string{"Platform": {"aws"}, "Network": {"ovn"}}
	opts.TestIDOptions = []reqopts.TestIdentification{{TestID: ownership.UniqueID, RequestedVariants: map[string]string{"Platform": "aws", "Network": "ovn"}}}
	key := crtest.KeyWithVariants{TestID: ownership.UniqueID, Variants: map[string]string{"Platform": "aws", "Network": "ovn"}}
	for _, location := range []*time.Location{time.UTC, time.FixedZone("crossing-offset", -13*60*60)} {
		t.Run(location.String(), func(t *testing.T) {
			request := opts
			request.BaseRelease.Start = opts.BaseRelease.Start.In(location)
			request.BaseRelease.End = opts.BaseRelease.End.In(location)
			status, errs := provider.QueryBaseTestStatus(ctx, request)
			require.Empty(t, errs)
			details, errs := provider.QueryBaseJobRunTestStatus(ctx, request)
			require.Empty(t, errs)
			var total int
			for _, summaries := range details {
				for _, summary := range summaries {
					for _, run := range summary.JobRuns {
						total += run.TotalCount
					}
				}
			}
			require.Equal(t, 2, status[key.Encode()].TotalCount)
			require.Equal(t, status[key.Encode()].TotalCount, total)
		})
	}
}
