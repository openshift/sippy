package componentreadiness

import (
	"testing"
	"time"

	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/testdetails"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFailedJobRunsFromTestDetails(t *testing.T) {
	startTime1 := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
	startTime2 := time.Date(2026, 4, 11, 8, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		report         testdetails.Report
		expectedCount  int
		expectedRunIDs []string
		checkFunc      func(t *testing.T, runs []models.RegressionJobRun)
	}{
		{
			name: "only extracts failed job runs, skips passing",
			report: testdetails.Report{
				Identification: crtest.Identification{
					RowIdentification: crtest.RowIdentification{TestID: "test1"},
				},
				Analyses: []testdetails.Analysis{
					{
						JobStats: []testdetails.JobStats{
							{
								SampleJobName: "periodic-ci-job-1",
								SampleJobRunStats: []testdetails.JobRunStats{
									{
										JobRunID:     "run-1",
										JobURL:       "https://prow.ci/run-1",
										StartTime:    startTime1,
										TestStats:    crtest.Stats{FailureCount: 1},
										TestFailures: 15,
										JobLabels:    []string{"InfraFailure"},
									},
									{
										JobRunID:     "run-2",
										JobURL:       "https://prow.ci/run-2",
										StartTime:    startTime2,
										TestStats:    crtest.Stats{SuccessCount: 1},
										TestFailures: 0,
									},
								},
							},
						},
					},
				},
			},
			expectedCount:  1,
			expectedRunIDs: []string{"run-1"},
			checkFunc: func(t *testing.T, runs []models.RegressionJobRun) {
				assert.Equal(t, "run-1", runs[0].ProwJobRunID)
				assert.Equal(t, "periodic-ci-job-1", runs[0].ProwJobName)
				assert.Equal(t, "https://prow.ci/run-1", runs[0].ProwJobURL)
				assert.Equal(t, 15, runs[0].TestFailures)
				assert.Equal(t, []string{"InfraFailure"}, []string(runs[0].JobLabels))
			},
		},
		{
			name: "extracts failed runs from multiple jobs",
			report: testdetails.Report{
				Analyses: []testdetails.Analysis{
					{
						JobStats: []testdetails.JobStats{
							{
								SampleJobName: "job-a",
								SampleJobRunStats: []testdetails.JobRunStats{
									{JobRunID: "a-1", StartTime: startTime1, TestStats: crtest.Stats{SuccessCount: 1}},
								},
							},
							{
								SampleJobName: "job-b",
								SampleJobRunStats: []testdetails.JobRunStats{
									{JobRunID: "b-1", StartTime: startTime1, TestStats: crtest.Stats{FailureCount: 1}},
									{JobRunID: "b-2", StartTime: startTime2, TestStats: crtest.Stats{SuccessCount: 1}},
								},
							},
						},
					},
				},
			},
			expectedCount:  1,
			expectedRunIDs: []string{"b-1"},
		},
		{
			name: "empty when no sample job runs",
			report: testdetails.Report{
				Analyses: []testdetails.Analysis{
					{
						JobStats: []testdetails.JobStats{
							{
								BaseJobName: "base-job",
								BaseJobRunStats: []testdetails.JobRunStats{
									{JobRunID: "base-1", StartTime: startTime1},
								},
							},
						},
					},
				},
			},
			expectedCount: 0,
		},
		{
			name: "empty report",
			report: testdetails.Report{
				Analyses: []testdetails.Analysis{},
			},
			expectedCount: 0,
		},
		{
			name: "empty when all runs pass",
			report: testdetails.Report{
				Analyses: []testdetails.Analysis{
					{
						JobStats: []testdetails.JobStats{
							{
								SampleJobName: "job-a",
								SampleJobRunStats: []testdetails.JobRunStats{
									{JobRunID: "a-1", StartTime: startTime1, TestStats: crtest.Stats{SuccessCount: 1}},
									{JobRunID: "a-2", StartTime: startTime2, TestStats: crtest.Stats{SuccessCount: 1}},
								},
							},
						},
					},
				},
			},
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runs := FailedJobRunsFromTestDetails(tt.report)
			assert.Len(t, runs, tt.expectedCount)

			if tt.expectedRunIDs != nil {
				var gotIDs []string
				for _, r := range runs {
					gotIDs = append(gotIDs, r.ProwJobRunID)
				}
				assert.Equal(t, tt.expectedRunIDs, gotIDs)
			}

			if tt.checkFunc != nil {
				require.Len(t, runs, tt.expectedCount)
				tt.checkFunc(t, runs)
			}

			// All runs should have zero RegressionID (set later during merge)
			for _, r := range runs {
				assert.Equal(t, uint(0), r.RegressionID)
			}
		})
	}
}

func TestFailedJobRunsFromTestDetails_StartTimePreserved(t *testing.T) {
	startTime := time.Date(2026, 4, 10, 14, 30, 0, 0, time.UTC)
	report := testdetails.Report{
		Analyses: []testdetails.Analysis{
			{
				JobStats: []testdetails.JobStats{
					{
						SampleJobName: "job",
						SampleJobRunStats: []testdetails.JobRunStats{
							{
								JobRunID:  "run-1",
								StartTime: startTime,
								TestStats: crtest.Stats{FailureCount: 1},
							},
						},
					},
				},
			},
		},
	}
	runs := FailedJobRunsFromTestDetails(report)
	require.Len(t, runs, 1)
	assert.Equal(t, startTime, runs[0].StartTime)
}
