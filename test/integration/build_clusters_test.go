package integration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/sippy/pkg/db/query"
	intutil "github.com/openshift/sippy/test/integration/util"
)

func TestHasBuildClusterData(t *testing.T) {
	tests := []struct {
		name    string
		cluster string
		want    bool
	}{
		{
			name:    "returns true when cluster is set",
			cluster: "build01",
			want:    true,
		},
		{
			name:    "returns false when cluster is empty",
			cluster: "",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dbc := intutil.NewTestDB(t, pgContainer)

			job := intutil.CreateProwJob(t, dbc, "periodic-ci-e2e-aws-4.16", "4.16", []string{"aws"})
			createSingleRun(t, dbc, job.ID, "4.16", runSpec{
				timestamp: time.Date(2024, 6, 10, 12, 0, 0, 0, time.UTC),
				succeeded: true,
				cluster:   tt.cluster,
			})

			has, err := query.HasBuildClusterData(dbc)
			require.NoError(t, err)
			assert.Equal(t, tt.want, has)
		})
	}
}

func TestHasBuildClusterDataEmpty(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	has, err := query.HasBuildClusterData(dbc)
	require.NoError(t, err)
	assert.False(t, has)
}

func TestBuildClusterHealth(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	start := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	boundary := time.Date(2024, 6, 8, 12, 0, 0, 0, time.UTC)
	end := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	job := intutil.CreateProwJobWithOptions(t, dbc,
		"periodic-ci-e2e-aws-4.16", "4.16", []string{"aws"},
		intutil.WithKind("periodic"),
	)

	createRuns(t, dbc, job.ID, "4.16", []runSpec{
		// Previous period: 3 pass, 1 fail
		{timestamp: start, succeeded: true, cluster: "build01"},
		{timestamp: start.Add(24 * time.Hour), succeeded: true, cluster: "build01"},
		{timestamp: start.Add(48 * time.Hour), succeeded: true, cluster: "build01"},
		{timestamp: start.Add(72 * time.Hour), cluster: "build01"},
		// Current period: 4 pass, 1 fail
		{timestamp: boundary, succeeded: true, cluster: "build01"},
		{timestamp: boundary.Add(24 * time.Hour), succeeded: true, cluster: "build01"},
		{timestamp: boundary.Add(48 * time.Hour), succeeded: true, cluster: "build01"},
		{timestamp: boundary.Add(72 * time.Hour), succeeded: true, cluster: "build01"},
		{timestamp: end, cluster: "build01"},
	})

	results, err := query.BuildClusterHealth(dbc, start, boundary, end)
	require.NoError(t, err)
	require.Len(t, results, 1)

	report := results[0]
	assert.Equal(t, "build01", report.Cluster)
	assert.Equal(t, 3, report.PreviousPasses)
	assert.Equal(t, 1, report.PreviousFails)
	assert.Equal(t, 4, report.PreviousRuns)
	assert.InDelta(t, 75.0, report.PreviousPassPercentage, 0.1)
	assert.Equal(t, 4, report.CurrentPasses)
	assert.Equal(t, 1, report.CurrentFails)
	assert.Equal(t, 5, report.CurrentRuns)
	assert.InDelta(t, 80.0, report.CurrentPassPercentage, 0.1)
	assert.InDelta(t, 5.0, report.NetImprovement, 0.1)
}

func TestBuildClusterHealthMultipleClusters(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	start := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	boundary := time.Date(2024, 6, 8, 12, 0, 0, 0, time.UTC)
	end := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	job := intutil.CreateProwJobWithOptions(t, dbc,
		"periodic-ci-e2e-aws-4.16", "4.16", []string{"aws"},
		intutil.WithKind("periodic"),
	)

	for _, cluster := range []string{"build01", "build02"} {
		createSingleRun(t, dbc, job.ID, "4.16", runSpec{
			timestamp: boundary,
			succeeded: true,
			cluster:   cluster,
		})
	}

	results, err := query.BuildClusterHealth(dbc, start, boundary, end)
	require.NoError(t, err)
	assert.Len(t, results, 2)

	clusters := make([]string, len(results))
	for i, r := range results {
		clusters[i] = r.Cluster
	}
	assert.ElementsMatch(t, []string{"build01", "build02"}, clusters)
}

func TestBuildClusterHealthZeroRunsInPreviousPeriod(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	start := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	boundary := time.Date(2024, 6, 8, 12, 0, 0, 0, time.UTC)
	end := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	job := intutil.CreateProwJobWithOptions(t, dbc,
		"periodic-ci-e2e-aws-4.16", "4.16", []string{"aws"},
		intutil.WithKind("periodic"),
	)

	createRuns(t, dbc, job.ID, "4.16", []runSpec{
		{timestamp: boundary, succeeded: true, cluster: "build01"},
		{timestamp: boundary.Add(24 * time.Hour), cluster: "build01"},
	})

	results, err := query.BuildClusterHealth(dbc, start, boundary, end)
	require.NoError(t, err)
	require.Len(t, results, 1)

	report := results[0]
	assert.Equal(t, "build01", report.Cluster)
	assert.Equal(t, 0, report.PreviousRuns)
	assert.Equal(t, 0, report.PreviousPasses)
	// NULLIF(0, 0) returns NULL, which scans to 0.0 for float64
	assert.Equal(t, 0.0, report.PreviousPassPercentage)
	assert.Equal(t, 2, report.CurrentRuns)
	assert.Equal(t, 1, report.CurrentPasses)
	assert.InDelta(t, 50.0, report.CurrentPassPercentage, 0.1)
}

func TestBuildClusterHealthExcludesNonPeriodic(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	start := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	boundary := time.Date(2024, 6, 8, 12, 0, 0, 0, time.UTC)
	end := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	presubmitJob := intutil.CreateProwJobWithOptions(t, dbc,
		"pull-ci-e2e-aws-4.16", "4.16", []string{"aws"},
		intutil.WithKind("presubmit"),
	)
	createSingleRun(t, dbc, presubmitJob.ID, "4.16", runSpec{
		timestamp: boundary,
		succeeded: true,
		cluster:   "build01",
	})

	results, err := query.BuildClusterHealth(dbc, start, boundary, end)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestBuildClusterAnalysis(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	job := intutil.CreateProwJobWithOptions(t, dbc,
		"periodic-ci-e2e-aws-4.16", "4.16", []string{"aws"},
		intutil.WithKind("periodic"),
	)

	now := time.Now().UTC()
	yesterday := time.Date(now.Year(), now.Month(), now.Day()-1, 10, 0, 0, 0, time.UTC)

	// Create runs yesterday with different results, pinned to 10:00-13:00 to avoid midnight crossing
	specs := make([]runSpec, 4)
	for i := 0; i < 4; i++ {
		specs[i] = runSpec{
			timestamp: yesterday.Add(time.Duration(i) * time.Hour),
			succeeded: i < 3,
			cluster:   "build01",
		}
	}
	createRuns(t, dbc, job.ID, "4.16", specs)

	tests := []struct {
		name     string
		period   string
		wantRows int
	}{
		{name: "group by day", period: "day", wantRows: 1},
		{name: "group by hour", period: "hour", wantRows: 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := query.BuildClusterAnalysis(dbc, tt.period)
			require.NoError(t, err)
			assert.Len(t, results, tt.wantRows)

			totalRuns := 0
			totalPasses := 0
			for _, r := range results {
				assert.Equal(t, "build01", r.Cluster)
				totalRuns += r.TotalRuns
				totalPasses += r.Passes
			}
			assert.Equal(t, 4, totalRuns)
			assert.Equal(t, 3, totalPasses)
		})
	}
}

func TestBuildClusterAnalysisByDay(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	job := intutil.CreateProwJobWithOptions(t, dbc,
		"periodic-ci-e2e-aws-4.16", "4.16", []string{"aws"},
		intutil.WithKind("periodic"),
	)

	now := time.Now().UTC()
	yesterday := time.Date(now.Year(), now.Month(), now.Day()-1, 12, 0, 0, 0, time.UTC)

	createRuns(t, dbc, job.ID, "4.16", []runSpec{
		{timestamp: yesterday, succeeded: true, cluster: "build01"},
		{timestamp: yesterday.Add(2 * time.Hour), cluster: "build01"},
	})

	results, err := query.BuildClusterAnalysis(dbc, "day")
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.Equal(t, "build01", results[0].Cluster)
	assert.Equal(t, 2, results[0].TotalRuns)
	assert.Equal(t, 1, results[0].Passes)
	assert.Equal(t, 1, results[0].Failures)
	assert.InDelta(t, 50.0, results[0].PassPercentage, 0.1)
}

func TestBuildClusterAnalysisExcludesOldData(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	job := intutil.CreateProwJobWithOptions(t, dbc,
		"periodic-ci-e2e-aws-4.16", "4.16", []string{"aws"},
		intutil.WithKind("periodic"),
	)

	oldTimestamp := time.Now().UTC().Add(-30 * 24 * time.Hour)
	createSingleRun(t, dbc, job.ID, "4.16", runSpec{
		timestamp: oldTimestamp,
		succeeded: true,
		cluster:   "build01",
	})

	results, err := query.BuildClusterAnalysis(dbc, "day")
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestBuildClusterAnalysisExcludesNonPeriodic(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	presubmitJob := intutil.CreateProwJobWithOptions(t, dbc,
		"pull-ci-e2e-aws-4.16", "4.16", []string{"aws"},
		intutil.WithKind("presubmit"),
	)

	yesterday := time.Now().UTC().Add(-24 * time.Hour)
	createSingleRun(t, dbc, presubmitJob.ID, "4.16", runSpec{
		timestamp: yesterday,
		succeeded: true,
		cluster:   "build01",
	})

	results, err := query.BuildClusterAnalysis(dbc, "day")
	require.NoError(t, err)
	assert.Empty(t, results)
}
