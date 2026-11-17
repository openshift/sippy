package main

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/openshift/sippy/pkg/apis/prow"
	sippyv1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	"github.com/openshift/sippy/pkg/dataloader/prowloader"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/pgwriter"
)

func TestValidateTestSource(t *testing.T) {
	for _, source := range []string{"auto", "gcs", "bigquery"} {
		require.NoError(t, validateTestSource(source))
	}
	require.EqualError(t, validateTestSource("unknown"), `invalid --test-source "unknown"`)
}

func TestProwBackfillParseRangeUsesInclusiveUTCDates(t *testing.T) {
	for _, tc := range []struct {
		name, startDate, endDate string
		wantStart, wantEnd       time.Time
		wantErr                  string
	}{
		{name: "single day", startDate: "2026-05-22", endDate: "2026-05-22", wantStart: time.Date(2026, time.May, 22, 0, 0, 0, 0, time.UTC), wantEnd: time.Date(2026, time.May, 23, 0, 0, 0, 0, time.UTC)},
		{name: "month boundary", startDate: "2026-05-31", endDate: "2026-06-01", wantStart: time.Date(2026, time.May, 31, 0, 0, 0, 0, time.UTC), wantEnd: time.Date(2026, time.June, 2, 0, 0, 0, 0, time.UTC)},
		{name: "reversed dates", startDate: "2026-05-23", endDate: "2026-05-22", wantErr: "--start-date must be before --end-date"},
		{name: "invalid start", startDate: "May 22", endDate: "2026-05-22", wantErr: "invalid --start-date"},
		{name: "invalid end", startDate: "2026-05-22", endDate: "May 23", wantErr: "invalid --end-date"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			flags := &prowBackfillFlags{Run: historicalRunFlags{StartDate: tc.startDate, EndDate: tc.endDate}}
			start, end, err := flags.parseRange()
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantStart, start)
			require.Equal(t, tc.wantEnd, end)
		})
	}
}

func TestValidateHistoricalDatasets(t *testing.T) {
	require.NoError(t, validateHistoricalDatasets("ci_analysis_us", ""))
	require.NoError(t, validateHistoricalDatasets("ci_analysis_us", "openshift-gce-devel.ci_data"))
	require.ErrorContains(t, validateHistoricalDatasets("ci_analysis_us`", ""), "--bigquery-dataset")
	require.ErrorContains(t, validateHistoricalDatasets("ci_analysis_us", "ci_data;SELECT 1"), "JOB_LABELS_DATASET")
}

func TestShouldInitializeGCS(t *testing.T) {
	require.True(t, shouldInitializeGCS("gcs", "", ""))
	require.True(t, shouldInitializeGCS("auto", "/credentials.json", ""))
	require.True(t, shouldInitializeGCS("auto", "", "/oauth.json"))
	require.False(t, shouldInitializeGCS("auto", "", ""))
	require.False(t, shouldInitializeGCS("bigquery", "/credentials.json", "/oauth.json"))
}

func TestShouldFallbackToBigQueryOnlyWhenGCSBuildPrefixIsAbsent(t *testing.T) {
	require.False(t, shouldFallbackToBigQuery("auto", true, nil))
	require.True(t, shouldFallbackToBigQuery("auto", false, nil))
	require.False(t, shouldFallbackToBigQuery("auto", false, errors.New("GCS listing failed")))
	require.False(t, shouldFallbackToBigQuery("gcs", false, nil))
	require.False(t, shouldFallbackToBigQuery("bigquery", false, nil))
}

func TestReleaseNamesPreservesNormalLoaderOrder(t *testing.T) {
	got, err := releaseNames([]sippyv1.Release{{Release: "4.22"}, {Release: "4.21"}})
	require.NoError(t, err)
	require.Equal(t, []string{"4.22", "4.21"}, got)
}

func TestReleaseNamesRequiresPostgresDefinitions(t *testing.T) {
	_, err := releaseNames(nil)
	require.EqualError(t, err, "no release definitions found in PostgreSQL; populate release_definitions before running prow-backfill")
}

func TestHistoricalJUnitBatchesUseCandidateTimeIntervals(t *testing.T) {
	start := time.Date(2026, time.July, 29, 23, 55, 0, 0, time.UTC)
	completion := start.Add(15 * time.Minute)
	batches, err := historicalJUnitBatches([]prowloader.HistoricalJob{
		{ProwJob: prow.ProwJob{
			Spec:   prow.ProwJobSpec{Job: "periodic-job"},
			Status: prow.ProwJobStatus{BuildID: "1", StartTime: start, CompletionTime: &completion, URL: "https://prow/view/gs/bucket/logs/job/1"},
		}},
	})
	require.NoError(t, err)
	require.Len(t, batches, 1)
	require.Equal(t, start, batches[0].Start)
	require.Equal(t, completion, batches[0].End)
	require.Equal(t, []string{"1"}, batches[0].BuildIDs)
}

func TestHistoricalRawBatchWriterFlushesFullAndFinalBatches(t *testing.T) {
	var batches [][]pgwriter.JobRunResult
	writer := historicalRawBatchWriter{
		batchSize: 3,
		write: func(batch []pgwriter.JobRunResult) error {
			copyOfBatch := append([]pgwriter.JobRunResult(nil), batch...)
			batches = append(batches, copyOfBatch)
			return nil
		},
	}

	for i := 0; i < 7; i++ {
		err := writer.add(&pgwriter.JobRunResult{Run: pgwriter.RunRow{ID: uint(i + 1)}})
		require.NoError(t, err)
	}
	require.NoError(t, writer.flush())

	require.Equal(t, []int{3, 3, 1}, []int{len(batches[0]), len(batches[1]), len(batches[2])})
	require.Equal(t, 7, writer.persisted)
	require.Equal(t, uint(1), batches[0][0].Run.ID)
	require.Equal(t, uint(7), batches[2][0].Run.ID)
}
