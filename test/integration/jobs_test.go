package integration

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/db/query"
	"github.com/openshift/sippy/pkg/filter"
	intutil "github.com/openshift/sippy/test/integration/util"
)

var pgContainer *intutil.PostgresContainer

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	ctx := context.Background()

	var err error
	pgContainer, err = intutil.StartPostgresContainer(ctx)
	if err != nil {
		panic("failed to start postgres container: " + err.Error())
	}
	defer func() {
		if err := pgContainer.Terminate(ctx); err != nil {
			log.Printf("warning: failed to terminate postgres container: %v", err)
		}
	}()

	return m.Run()
}

func TestProwJobSimilarName(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	jobs := []models.ProwJob{
		{Name: "periodic-ci-openshift-release-master-nightly-4.16-e2e-aws-ovn", Release: "4.16", Variants: pq.StringArray{"aws", "ovn"}},
		{Name: "pull-ci-openshift-origin-master-e2e-aws-ovn", Release: "4.16", Variants: pq.StringArray{"aws", "ovn"}},
		{Name: "periodic-ci-openshift-release-master-nightly-4.16-e2e-gcp-ovn", Release: "4.16", Variants: pq.StringArray{"gcp", "ovn"}},
		{Name: "periodic-ci-openshift-release-master-nightly-4.15-e2e-aws-ovn", Release: "4.15", Variants: pq.StringArray{"aws", "ovn"}},
		{Name: "periodic-ci-openshift-release-master-nightly-4.16-e2e-gcp_ovn", Release: "4.16", Variants: pq.StringArray{"gcp", "ovn"}},
	}
	for i := range jobs {
		require.NoError(t, dbc.DB.Create(&jobs[i]).Error)
	}

	tests := []struct {
		name      string
		rootName  string
		release   string
		wantNames []string
	}{
		{
			name:      "matches suffix across job types",
			rootName:  "e2e-aws-ovn",
			release:   "4.16",
			wantNames: []string{"periodic-ci-openshift-release-master-nightly-4.16-e2e-aws-ovn", "pull-ci-openshift-origin-master-e2e-aws-ovn"},
		},
		{
			name:      "filters by release",
			rootName:  "e2e-aws-ovn",
			release:   "4.15",
			wantNames: []string{"periodic-ci-openshift-release-master-nightly-4.15-e2e-aws-ovn"},
		},
		{
			name:      "no matches",
			rootName:  "e2e-azure-ovn",
			release:   "4.16",
			wantNames: []string{},
		},
		{
			name:     "LIKE underscore in pattern matches any character",
			rootName: "e2e-gcp_ovn",
			release:  "4.16",
			wantNames: []string{
				"periodic-ci-openshift-release-master-nightly-4.16-e2e-gcp-ovn",
				"periodic-ci-openshift-release-master-nightly-4.16-e2e-gcp_ovn",
			},
		},
		{
			name:      "case sensitive",
			rootName:  "E2E-AWS-OVN",
			release:   "4.16",
			wantNames: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := query.ProwJobSimilarName(dbc, tt.rootName, tt.release)
			require.NoError(t, err)

			gotNames := make([]string, len(result))
			for i, j := range result {
				gotNames[i] = j.Name
			}
			assert.ElementsMatch(t, tt.wantNames, gotNames)

			for _, j := range result {
				assert.Equal(t, tt.release, j.Release, "returned job should match queried release")
				assert.NotZero(t, j.ID, "returned job should have a non-zero ID")
				assert.NotEmpty(t, j.Variants, "returned job should have variants")
			}
		})
	}
}

func TestVariantReports(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	start := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	boundary := time.Date(2024, 6, 8, 12, 0, 0, 0, time.UTC)
	end := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	job := models.ProwJob{
		Name:     "periodic-ci-openshift-release-master-nightly-4.16-e2e-aws-ovn",
		Release:  "4.16",
		Variants: pq.StringArray{"aws", "ovn"},
	}
	require.NoError(t, dbc.DB.Create(&job).Error)

	createRuns(t, dbc, job.ID, "4.16", []runSpec{
		// Previous period [start, boundary): 8 runs, 6 pass, 2 fail = 75%
		{timestamp: start, succeeded: true},
		{timestamp: start.Add(24 * time.Hour), succeeded: true},
		{timestamp: start.Add(48 * time.Hour), succeeded: true},
		{timestamp: start.Add(72 * time.Hour), succeeded: true},
		{timestamp: start.Add(96 * time.Hour), succeeded: true},
		{timestamp: start.Add(120 * time.Hour), succeeded: true},
		{timestamp: start.Add(132 * time.Hour)},
		{timestamp: start.Add(144 * time.Hour)},
		// Current period [boundary, end]: 10 runs, 9 pass, 1 fail = 90%
		{timestamp: boundary, succeeded: true},
		{timestamp: boundary.Add(24 * time.Hour), succeeded: true},
		{timestamp: boundary.Add(48 * time.Hour), succeeded: true},
		{timestamp: boundary.Add(72 * time.Hour), succeeded: true},
		{timestamp: boundary.Add(96 * time.Hour), succeeded: true},
		{timestamp: boundary.Add(108 * time.Hour), succeeded: true},
		{timestamp: boundary.Add(120 * time.Hour), succeeded: true},
		{timestamp: boundary.Add(132 * time.Hour), succeeded: true},
		{timestamp: boundary.Add(144 * time.Hour), succeeded: true},
		{timestamp: end},
	})

	variants, err := query.VariantReports(dbc, "4.16", start, boundary, end)
	require.NoError(t, err)

	require.Len(t, variants, 2)

	// Verify ordering: query sorts by current_pass_percentage ASC
	assert.LessOrEqual(t, variants[0].CurrentPassPercentage, variants[1].CurrentPassPercentage,
		"results should be ordered by current_pass_percentage ASC")

	for _, v := range variants {
		assert.Contains(t, []string{"aws", "ovn"}, v.Name)

		assert.Equal(t, 9, v.CurrentPasses, "variant %s current passes", v.Name)
		assert.Equal(t, 1, v.CurrentFails, "variant %s current fails", v.Name)
		assert.Equal(t, 10, v.CurrentRuns, "variant %s current runs", v.Name)
		assert.InDelta(t, 90.0, v.CurrentPassPercentage, 0.1, "variant %s current pass percentage", v.Name)

		assert.Equal(t, 6, v.PreviousPasses, "variant %s previous passes", v.Name)
		assert.Equal(t, 2, v.PreviousFails, "variant %s previous fails", v.Name)
		assert.Equal(t, 8, v.PreviousRuns, "variant %s previous runs", v.Name)
		assert.InDelta(t, 75.0, v.PreviousPassPercentage, 0.1, "variant %s previous pass percentage", v.Name)

		assert.InDelta(t, 15.0, v.NetImprovement, 0.1, "variant %s net improvement", v.Name)
	}
}

func TestVariantReports_MultipleJobs(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	start := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	boundary := time.Date(2024, 6, 8, 12, 0, 0, 0, time.UTC)
	end := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	jobA := models.ProwJob{
		Name:     "periodic-ci-openshift-release-master-nightly-4.16-e2e-aws-ovn",
		Release:  "4.16",
		Variants: pq.StringArray{"aws", "ovn"},
	}
	jobB := models.ProwJob{
		Name:     "periodic-ci-openshift-release-master-nightly-4.16-e2e-aws-gcp",
		Release:  "4.16",
		Variants: pq.StringArray{"aws", "gcp"},
	}
	require.NoError(t, dbc.DB.Create(&jobA).Error)
	require.NoError(t, dbc.DB.Create(&jobB).Error)

	// Job A: 4 current runs (3 pass, 1 fail), 2 previous runs (1 pass, 1 fail)
	createRuns(t, dbc, jobA.ID, "4.16", []runSpec{
		{timestamp: start.Add(24 * time.Hour), succeeded: true},
		{timestamp: start.Add(48 * time.Hour)},
		{timestamp: boundary.Add(24 * time.Hour), succeeded: true},
		{timestamp: boundary.Add(48 * time.Hour), succeeded: true},
		{timestamp: boundary.Add(72 * time.Hour), succeeded: true},
		{timestamp: boundary.Add(96 * time.Hour)},
	})

	// Job B: 2 current runs (2 pass, 0 fail), 1 previous run (1 pass, 0 fail)
	createRuns(t, dbc, jobB.ID, "4.16", []runSpec{
		{timestamp: start.Add(24 * time.Hour), succeeded: true},
		{timestamp: boundary.Add(24 * time.Hour), succeeded: true},
		{timestamp: boundary.Add(48 * time.Hour), succeeded: true},
	})

	variants, err := query.VariantReports(dbc, "4.16", start, boundary, end)
	require.NoError(t, err)

	variantByName := make(map[string]apitype.Variant)
	for _, v := range variants {
		variantByName[v.Name] = v
	}

	require.Len(t, variants, 3, "should have aws, ovn, gcp variants")

	// aws: aggregates job A (3p/1f) + job B (2p/0f) = 5 pass, 1 fail, 6 runs = 83.3%
	aws := variantByName["aws"]
	assert.Equal(t, 5, aws.CurrentPasses)
	assert.Equal(t, 1, aws.CurrentFails)
	assert.Equal(t, 6, aws.CurrentRuns)
	assert.InDelta(t, 83.3, aws.CurrentPassPercentage, 0.1)
	assert.Equal(t, 2, aws.PreviousPasses)
	assert.Equal(t, 1, aws.PreviousFails)
	assert.Equal(t, 3, aws.PreviousRuns)
	assert.InDelta(t, 66.7, aws.PreviousPassPercentage, 0.1)

	// ovn: only job A = 3 pass, 1 fail, 4 runs = 75%
	ovn := variantByName["ovn"]
	assert.Equal(t, 3, ovn.CurrentPasses)
	assert.Equal(t, 1, ovn.CurrentFails)
	assert.Equal(t, 4, ovn.CurrentRuns)
	assert.InDelta(t, 75.0, ovn.CurrentPassPercentage, 0.1)

	// gcp: only job B = 2 pass, 0 fail, 2 runs = 100%
	gcp := variantByName["gcp"]
	assert.Equal(t, 2, gcp.CurrentPasses)
	assert.Equal(t, 0, gcp.CurrentFails)
	assert.Equal(t, 2, gcp.CurrentRuns)
	assert.InDelta(t, 100.0, gcp.CurrentPassPercentage, 0.1)
}

func TestVariantReports_ZeroPreviousRuns(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	start := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	boundary := time.Date(2024, 6, 8, 12, 0, 0, 0, time.UTC)
	end := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	job := models.ProwJob{
		Name:     "periodic-ci-openshift-release-master-nightly-4.16-e2e-aws-ovn",
		Release:  "4.16",
		Variants: pq.StringArray{"aws"},
	}
	require.NoError(t, dbc.DB.Create(&job).Error)

	// Only current period runs
	createRuns(t, dbc, job.ID, "4.16", []runSpec{
		{timestamp: boundary.Add(24 * time.Hour), succeeded: true},
		{timestamp: boundary.Add(48 * time.Hour), succeeded: true},
		{timestamp: boundary.Add(72 * time.Hour)},
	})

	variants, err := query.VariantReports(dbc, "4.16", start, boundary, end)
	require.NoError(t, err)
	require.Len(t, variants, 1)

	v := variants[0]
	assert.Equal(t, "aws", v.Name)
	assert.Equal(t, 2, v.CurrentPasses)
	assert.Equal(t, 1, v.CurrentFails)
	assert.Equal(t, 3, v.CurrentRuns)
	assert.InDelta(t, 66.7, v.CurrentPassPercentage, 0.1)

	assert.Equal(t, 0, v.PreviousPasses)
	assert.Equal(t, 0, v.PreviousFails)
	assert.Equal(t, 0, v.PreviousRuns)
	assert.Equal(t, 0.0, v.PreviousPassPercentage, "zero previous runs should yield 0 percentage")
}

func TestVariantReports_EmptyVariants(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	start := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	boundary := time.Date(2024, 6, 8, 12, 0, 0, 0, time.UTC)
	end := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	job := models.ProwJob{
		Name:     "periodic-ci-openshift-release-master-nightly-4.16-e2e-bare",
		Release:  "4.16",
		Variants: pq.StringArray{},
	}
	require.NoError(t, dbc.DB.Create(&job).Error)

	createRuns(t, dbc, job.ID, "4.16", []runSpec{
		{timestamp: boundary.Add(24 * time.Hour), succeeded: true},
	})

	variants, err := query.VariantReports(dbc, "4.16", start, boundary, end)
	require.NoError(t, err)
	assert.Empty(t, variants, "empty variants array should produce no variant rows via unnest")
}

func TestVariantReports_BoundaryTimestamp(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	start := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	boundary := time.Date(2024, 6, 8, 12, 0, 0, 0, time.UTC)
	end := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	job := models.ProwJob{
		Name:     "periodic-ci-openshift-release-master-nightly-4.16-e2e-aws-ovn",
		Release:  "4.16",
		Variants: pq.StringArray{"aws"},
	}
	require.NoError(t, dbc.DB.Create(&job).Error)

	// 1 previous pass, 1 failure at exactly boundary, 1 current pass
	createRuns(t, dbc, job.ID, "4.16", []runSpec{
		{timestamp: start.Add(24 * time.Hour), succeeded: true},
		{timestamp: boundary},
		{timestamp: boundary.Add(48 * time.Hour), succeeded: true},
	})

	variants, err := query.VariantReports(dbc, "4.16", start, boundary, end)
	require.NoError(t, err)
	require.Len(t, variants, 1)

	v := variants[0]
	// Previous [start, boundary): 1 pass, 0 fail = 100%
	assert.Equal(t, 1, v.PreviousPasses)
	assert.Equal(t, 0, v.PreviousFails)
	assert.Equal(t, 1, v.PreviousRuns)
	assert.InDelta(t, 100.0, v.PreviousPassPercentage, 0.1)

	// Current [boundary, end]: boundary failure + 1 pass = 50%
	assert.Equal(t, 1, v.CurrentPasses)
	assert.Equal(t, 1, v.CurrentFails)
	assert.Equal(t, 2, v.CurrentRuns)
	assert.InDelta(t, 50.0, v.CurrentPassPercentage, 0.1)
}

func TestVariantReports_MultipleReleases(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	start := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	boundary := time.Date(2024, 6, 8, 12, 0, 0, 0, time.UTC)
	end := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	job416 := models.ProwJob{
		Name:     "periodic-ci-openshift-release-master-nightly-4.16-e2e-aws-ovn",
		Release:  "4.16",
		Variants: pq.StringArray{"aws"},
	}
	job415 := models.ProwJob{
		Name:     "periodic-ci-openshift-release-master-nightly-4.15-e2e-aws-ovn",
		Release:  "4.15",
		Variants: pq.StringArray{"aws"},
	}
	require.NoError(t, dbc.DB.Create(&job416).Error)
	require.NoError(t, dbc.DB.Create(&job415).Error)

	// 4.16: 2 current runs (2 pass) = 100%
	createRuns(t, dbc, job416.ID, "4.16", []runSpec{
		{timestamp: boundary.Add(24 * time.Hour), succeeded: true},
		{timestamp: boundary.Add(48 * time.Hour), succeeded: true},
	})

	// 4.15: 2 current runs (1 pass, 1 fail) = 50%
	createRuns(t, dbc, job415.ID, "4.15", []runSpec{
		{timestamp: boundary.Add(24 * time.Hour), succeeded: true},
		{timestamp: boundary.Add(48 * time.Hour)},
	})

	variants, err := query.VariantReports(dbc, "4.16", start, boundary, end)
	require.NoError(t, err)
	require.Len(t, variants, 1)
	assert.InDelta(t, 100.0, variants[0].CurrentPassPercentage, 0.1,
		"should only include 4.16 runs")
}

func TestJobReports(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	start := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	boundary := time.Date(2024, 6, 8, 12, 0, 0, 0, time.UTC)
	end := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	job := models.ProwJob{
		Name:     "periodic-ci-openshift-release-master-nightly-4.16-e2e-aws-ovn",
		Release:  "4.16",
		Variants: pq.StringArray{"aws", "ovn"},
	}
	require.NoError(t, dbc.DB.Create(&job).Error)

	createRuns(t, dbc, job.ID, "4.16", []runSpec{
		// Previous period [start, boundary): 4 runs, 3 pass, 1 fail = 75%
		{timestamp: start, succeeded: true, duration: 30 * time.Minute},
		{timestamp: start.Add(48 * time.Hour), succeeded: true, duration: 30 * time.Minute},
		{timestamp: start.Add(96 * time.Hour), succeeded: true, duration: 30 * time.Minute},
		{timestamp: start.Add(144 * time.Hour), duration: 30 * time.Minute},
		// Current period [boundary, end]: 5 runs, 4 pass, 1 infra fail = 80%
		{timestamp: boundary, succeeded: true, duration: 45 * time.Minute},
		{timestamp: boundary.Add(48 * time.Hour), succeeded: true, duration: 45 * time.Minute},
		{timestamp: boundary.Add(96 * time.Hour), succeeded: true, duration: 45 * time.Minute},
		{timestamp: boundary.Add(120 * time.Hour), succeeded: true, duration: 45 * time.Minute},
		{timestamp: end, infraFailure: true, duration: 45 * time.Minute},
	})

	lastSucceededTimestamp := boundary.Add(120 * time.Hour)

	reports, err := query.JobReports(dbc, &filter.FilterOptions{Filter: &filter.Filter{}}, "4.16", start, boundary, end)
	require.NoError(t, err)
	require.Len(t, reports, 1)

	report := reports[0]
	assert.Equal(t, job.Name, report.Name)
	assert.Equal(t, "e2e-aws-ovn", report.BriefName)
	assert.ElementsMatch(t, pq.StringArray{"aws", "ovn"}, report.Variants)

	assert.Equal(t, 4, report.CurrentPasses)
	assert.Equal(t, 1, report.CurrentFails)
	assert.Equal(t, 5, report.CurrentRuns)
	assert.Equal(t, 1, report.CurrentInfraFails)
	assert.InDelta(t, 80.0, report.CurrentPassPercentage, 0.1)
	assert.InDelta(t, 100.0, report.CurrentProjectedPassPercentage, 0.1)

	assert.Equal(t, 3, report.PreviousPasses)
	assert.Equal(t, 1, report.PreviousFails)
	assert.Equal(t, 4, report.PreviousRuns)
	assert.Equal(t, 0, report.PreviousInfraFails)
	assert.InDelta(t, 75.0, report.PreviousPassPercentage, 0.1)
	assert.InDelta(t, 75.0, report.PreviousProjectedPassPercentage, 0.1)

	assert.InDelta(t, 5.0, report.NetImprovement, 0.1)
	assert.Equal(t, 0, report.OpenBugs)
	assert.Equal(t, 45, report.CurrentAverageDurationMinutes)
	assert.Equal(t, 30, report.PreviousAverageDurationMinutes)

	require.NotNil(t, report.LastPass)
	assert.True(t, lastSucceededTimestamp.Equal(*report.LastPass),
		"last pass should be %v, got %v", lastSucceededTimestamp, *report.LastPass)

	assert.Empty(t, report.Org)
	assert.Empty(t, report.Repo)
	assert.Equal(t, 0.0, report.AverageRetestsToMerge)
}

func TestJobReports_WithBugs(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	start := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	boundary := time.Date(2024, 6, 8, 12, 0, 0, 0, time.UTC)
	end := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	job := models.ProwJob{
		Name:     "periodic-ci-openshift-release-master-nightly-4.16-e2e-aws-ovn",
		Release:  "4.16",
		Variants: pq.StringArray{"aws"},
	}
	require.NoError(t, dbc.DB.Create(&job).Error)

	createRuns(t, dbc, job.ID, "4.16", []runSpec{
		{timestamp: boundary.Add(24 * time.Hour), succeeded: true},
		{timestamp: boundary.Add(48 * time.Hour), succeeded: true},
	})

	bugs := []models.Bug{
		{Key: "BUG-1", Status: "NEW", Summary: "active bug 1"},
		{Key: "BUG-2", Status: "ASSIGNED", Summary: "active bug 2"},
		{Key: "BUG-3", Status: "CLOSED", Summary: "closed bug"},
		{Key: "BUG-4", Status: "VERIFIED", Summary: "verified bug"},
		{Key: "BUG-5", Status: "ON_QA", Summary: "on_qa bug"},
	}
	for i := range bugs {
		require.NoError(t, dbc.DB.Create(&bugs[i]).Error)
		require.NoError(t, dbc.DB.Model(&job).Association("Bugs").Append(&bugs[i]))
	}

	reports, err := query.JobReports(dbc, &filter.FilterOptions{Filter: &filter.Filter{}}, "4.16", start, boundary, end)
	require.NoError(t, err)
	require.Len(t, reports, 1)

	assert.Equal(t, 2, reports[0].OpenBugs,
		"should count only bugs with active statuses (not closed/verified/modified/on_qa)")
	assert.Equal(t, 2, reports[0].CurrentRuns,
		"bug associations must not inflate run counts")
	assert.Equal(t, 2, reports[0].CurrentPasses,
		"bug associations must not inflate pass counts")
}

func TestJobReports_WithPullRequests(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	start := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	boundary := time.Date(2024, 6, 8, 12, 0, 0, 0, time.UTC)
	end := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	job := models.ProwJob{
		Name:     "pull-ci-openshift-origin-master-e2e-aws-ovn",
		Release:  "4.16",
		Variants: pq.StringArray{"aws", "ovn"},
	}
	require.NoError(t, dbc.DB.Create(&job).Error)

	mergedAt1 := boundary.Add(24 * time.Hour)
	mergedAt2 := boundary.Add(72 * time.Hour)

	pr1 := models.ProwPullRequest{
		Org: "openshift", Repo: "origin", Number: 100, Author: "dev1",
		SHA: "sha-pr1", Link: "https://github.com/openshift/origin/pull/100",
		MergedAt: &mergedAt1,
	}
	pr2 := models.ProwPullRequest{
		Org: "openshift", Repo: "origin", Number: 200, Author: "dev2",
		SHA: "sha-pr2", Link: "https://github.com/openshift/origin/pull/200",
		MergedAt: &mergedAt2,
	}
	require.NoError(t, dbc.DB.Create(&pr1).Error)
	require.NoError(t, dbc.DB.Create(&pr2).Error)

	// PR 1: 3 runs total (2 failures, 1 success). merged_prs CTE excludes S/A, so counts 2.
	run1a := createSingleRun(t, dbc, job.ID, "4.16", runSpec{timestamp: boundary.Add(12 * time.Hour), duration: 20 * time.Minute})
	run1b := createSingleRun(t, dbc, job.ID, "4.16", runSpec{timestamp: boundary.Add(18 * time.Hour), duration: 20 * time.Minute})
	run1c := createSingleRun(t, dbc, job.ID, "4.16", runSpec{timestamp: boundary.Add(22 * time.Hour), succeeded: true, duration: 20 * time.Minute})

	// PR 2: 2 runs total (1 failure, 1 success). merged_prs CTE excludes S/A, so counts 1.
	run2a := createSingleRun(t, dbc, job.ID, "4.16", runSpec{timestamp: boundary.Add(60 * time.Hour), duration: 20 * time.Minute})
	run2b := createSingleRun(t, dbc, job.ID, "4.16", runSpec{timestamp: boundary.Add(70 * time.Hour), succeeded: true, duration: 20 * time.Minute})

	// Link runs to PRs via the join table with denormalized fields
	for _, run := range []models.ProwJobRun{run1a, run1b, run1c} {
		require.NoError(t, dbc.DB.Create(&models.ProwJobRunProwPullRequest{
			ProwJobRunID:        run.ID,
			ProwPullRequestID:   pr1.ID,
			ProwJobRunRelease:   run.ProwJobRelease,
			ProwJobRunTimestamp: run.Timestamp,
		}).Error)
	}
	for _, run := range []models.ProwJobRun{run2a, run2b} {
		require.NoError(t, dbc.DB.Create(&models.ProwJobRunProwPullRequest{
			ProwJobRunID:        run.ID,
			ProwPullRequestID:   pr2.ID,
			ProwJobRunRelease:   run.ProwJobRelease,
			ProwJobRunTimestamp: run.Timestamp,
		}).Error)
	}

	reports, err := query.JobReports(dbc, &filter.FilterOptions{Filter: &filter.Filter{}}, "4.16", start, boundary, end)
	require.NoError(t, err)
	require.Len(t, reports, 1)

	report := reports[0]
	assert.Equal(t, "openshift", report.Org)
	assert.Equal(t, "origin", report.Repo)
	// PR 1: 2 non-S/A runs, PR 2: 1 non-S/A run. AVG(2, 1) = 1.5
	assert.InDelta(t, 1.5, report.AverageRetestsToMerge, 0.1)
	assert.Equal(t, 5, report.CurrentRuns,
		"PR associations must not inflate run counts")
	assert.Equal(t, 2, report.CurrentPasses,
		"PR associations must not inflate pass counts")
}

func TestJobReports_ZeroPreviousRuns(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	start := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	boundary := time.Date(2024, 6, 8, 12, 0, 0, 0, time.UTC)
	end := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	job := models.ProwJob{
		Name:     "periodic-ci-openshift-release-master-nightly-4.16-e2e-aws-ovn",
		Release:  "4.16",
		Variants: pq.StringArray{"aws"},
	}
	require.NoError(t, dbc.DB.Create(&job).Error)

	createRuns(t, dbc, job.ID, "4.16", []runSpec{
		{timestamp: boundary.Add(24 * time.Hour), succeeded: true},
		{timestamp: boundary.Add(48 * time.Hour), succeeded: true},
		{timestamp: boundary.Add(72 * time.Hour)},
	})

	reports, err := query.JobReports(dbc, &filter.FilterOptions{Filter: &filter.Filter{}}, "4.16", start, boundary, end)
	require.NoError(t, err)
	require.Len(t, reports, 1)

	report := reports[0]
	assert.Equal(t, 0, report.PreviousRuns)
	assert.Equal(t, 0, report.PreviousPasses)
	assert.Equal(t, 0, report.PreviousFails)
	assert.Equal(t, 0.0, report.PreviousPassPercentage, "zero previous runs should yield 0 percentage")
	assert.Equal(t, 0.0, report.PreviousProjectedPassPercentage)
	assert.Equal(t, 2, report.CurrentPasses)
	assert.Equal(t, 1, report.CurrentFails)
	assert.Equal(t, 3, report.CurrentRuns)
}

func TestJobReports_WithFilters(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	start := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	boundary := time.Date(2024, 6, 8, 12, 0, 0, 0, time.UTC)
	end := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	jobAWS := models.ProwJob{
		Name:     "periodic-ci-openshift-release-master-nightly-4.16-e2e-aws-ovn",
		Release:  "4.16",
		Variants: pq.StringArray{"aws", "ovn"},
	}
	jobGCP := models.ProwJob{
		Name:     "periodic-ci-openshift-release-master-nightly-4.16-e2e-gcp-ovn",
		Release:  "4.16",
		Variants: pq.StringArray{"gcp", "ovn"},
	}
	require.NoError(t, dbc.DB.Create(&jobAWS).Error)
	require.NoError(t, dbc.DB.Create(&jobGCP).Error)

	// AWS: 4 current runs, 3 pass, 1 fail = 75%
	createRuns(t, dbc, jobAWS.ID, "4.16", []runSpec{
		{timestamp: boundary.Add(24 * time.Hour), succeeded: true},
		{timestamp: boundary.Add(48 * time.Hour), succeeded: true},
		{timestamp: boundary.Add(72 * time.Hour), succeeded: true},
		{timestamp: boundary.Add(96 * time.Hour)},
	})

	// GCP: 2 current runs, 2 pass = 100%
	createRuns(t, dbc, jobGCP.ID, "4.16", []runSpec{
		{timestamp: boundary.Add(24 * time.Hour), succeeded: true},
		{timestamp: boundary.Add(48 * time.Hour), succeeded: true},
	})

	t.Run("filter by name contains", func(t *testing.T) {
		opts := &filter.FilterOptions{
			Filter: &filter.Filter{
				Items: []filter.FilterItem{
					{Field: "name", Operator: filter.OperatorContains, Value: "aws"},
				},
			},
		}
		reports, err := query.JobReports(dbc, opts, "4.16", start, boundary, end)
		require.NoError(t, err)
		require.Len(t, reports, 1)
		assert.Contains(t, reports[0].Name, "aws")
	})

	t.Run("filter by current_pass_percentage", func(t *testing.T) {
		opts := &filter.FilterOptions{
			Filter: &filter.Filter{
				Items: []filter.FilterItem{
					{Field: "current_pass_percentage", Operator: filter.OperatorArithmeticGreaterThan, Value: "80"},
				},
			},
		}
		reports, err := query.JobReports(dbc, opts, "4.16", start, boundary, end)
		require.NoError(t, err)
		require.Len(t, reports, 1)
		assert.Contains(t, reports[0].Name, "gcp")
	})

	t.Run("sort and limit", func(t *testing.T) {
		opts := &filter.FilterOptions{
			Filter:    &filter.Filter{},
			SortField: "current_pass_percentage",
			Sort:      apitype.SortAscending,
			Limit:     1,
		}
		reports, err := query.JobReports(dbc, opts, "4.16", start, boundary, end)
		require.NoError(t, err)
		require.Len(t, reports, 1)
		assert.Contains(t, reports[0].Name, "aws", "lowest pass percentage should be aws at 75%%")
	})
}

func TestJobReports_MultipleReleases(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	start := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	boundary := time.Date(2024, 6, 8, 12, 0, 0, 0, time.UTC)
	end := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	job416 := models.ProwJob{
		Name:     "periodic-ci-openshift-release-master-nightly-4.16-e2e-aws-ovn",
		Release:  "4.16",
		Variants: pq.StringArray{"aws"},
	}
	job415 := models.ProwJob{
		Name:     "periodic-ci-openshift-release-master-nightly-4.15-e2e-aws-ovn",
		Release:  "4.15",
		Variants: pq.StringArray{"aws"},
	}
	require.NoError(t, dbc.DB.Create(&job416).Error)
	require.NoError(t, dbc.DB.Create(&job415).Error)

	createRuns(t, dbc, job416.ID, "4.16", []runSpec{
		{timestamp: boundary.Add(24 * time.Hour), succeeded: true},
		{timestamp: boundary.Add(48 * time.Hour), succeeded: true},
	})

	createRuns(t, dbc, job415.ID, "4.15", []runSpec{
		{timestamp: boundary.Add(24 * time.Hour)},
		{timestamp: boundary.Add(48 * time.Hour)},
	})

	reports, err := query.JobReports(dbc, &filter.FilterOptions{Filter: &filter.Filter{}}, "4.16", start, boundary, end)
	require.NoError(t, err)
	require.Len(t, reports, 1)
	assert.Equal(t, job416.Name, reports[0].Name)
	assert.InDelta(t, 100.0, reports[0].CurrentPassPercentage, 0.1,
		"should only include 4.16 runs")
}

// runSpec describes a single job run for helper functions.
type runSpec struct {
	timestamp    time.Time
	succeeded    bool
	infraFailure bool
	duration     time.Duration
}

// createRuns inserts ProwJobRun records for the given job using the provided specs.
func createRuns(t *testing.T, dbc *db.DB, jobID uint, release string, specs []runSpec) {
	t.Helper()
	for _, r := range specs {
		createSingleRun(t, dbc, jobID, release, r)
	}
}

// createSingleRun inserts a single ProwJobRun and returns it.
func createSingleRun(t *testing.T, dbc *db.DB, jobID uint, release string, spec runSpec) models.ProwJobRun {
	t.Helper()
	run := models.ProwJobRun{
		ProwJobID:             jobID,
		ProwJobRelease:        release,
		Timestamp:             spec.timestamp,
		Succeeded:             spec.succeeded,
		Failed:                !spec.succeeded,
		InfrastructureFailure: spec.infraFailure,
		Duration:              spec.duration,
		OverallResult:         v1.JobSucceeded,
	}
	if spec.infraFailure {
		run.OverallResult = v1.JobExternalInfrastructureFailure
	} else if !spec.succeeded {
		run.OverallResult = v1.JobTestFailure
	}
	require.NoError(t, dbc.DB.Create(&run).Error)
	return run
}
