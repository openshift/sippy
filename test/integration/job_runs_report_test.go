package integration

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/sippy/pkg/api"
	apitype "github.com/openshift/sippy/pkg/apis/api"
	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/filter"
	intutil "github.com/openshift/sippy/test/integration/util"
)

var (
	jrReportEnd = time.Date(2024, 7, 15, 12, 0, 0, 0, time.UTC)
	jrLookback  = jrReportEnd.Add(-90 * 24 * time.Hour)
)

type jobRunsTestData struct {
	jobAWS   models.ProwJob
	jobGCP   models.ProwJob
	jobOther models.ProwJob

	runA1              models.ProwJobRun
	runA2              models.ProwJobRun
	runA3              models.ProwJobRun
	runG1              models.ProwJobRun
	runOther           models.ProwJobRun
	runOutsideLookback models.ProwJobRun
	runAtReportEnd     models.ProwJobRun

	testEtcd    models.Test
	testNetwork models.Test
	testUpgrade models.Test

	pr1 models.ProwPullRequest
}

func setupJobRunsTestData(t *testing.T, dbc *db.DB) jobRunsTestData {
	t.Helper()
	var td jobRunsTestData

	td.jobAWS = models.ProwJob{
		Name:     "periodic-ci-openshift-release-master-nightly-4.16-e2e-aws-ovn",
		Release:  "4.16",
		Variants: pq.StringArray{"aws", "ovn"},
	}
	td.jobGCP = models.ProwJob{
		Name:     "periodic-ci-openshift-release-master-nightly-4.16-e2e-gcp-sdn",
		Release:  "4.16",
		Variants: pq.StringArray{"gcp", "sdn"},
	}
	td.jobOther = models.ProwJob{
		Name:     "periodic-ci-openshift-release-master-nightly-4.15-e2e-aws-ovn",
		Release:  "4.15",
		Variants: pq.StringArray{"aws", "ovn"},
	}
	for _, job := range []*models.ProwJob{&td.jobAWS, &td.jobGCP, &td.jobOther} {
		require.NoError(t, dbc.DB.Create(job).Error)
	}

	td.runA1 = createSingleRun(t, dbc, td.jobAWS.ID, "4.16", runSpec{
		timestamp:    time.Date(2024, 7, 10, 12, 0, 0, 0, time.UTC),
		succeeded:    true,
		testFailures: 2,
		testFlakes:   1,
		cluster:      "build01",
		url:          "https://prow.ci/runA1",
	})
	td.runA2 = createSingleRun(t, dbc, td.jobAWS.ID, "4.16", runSpec{
		timestamp:    time.Date(2024, 7, 12, 12, 0, 0, 0, time.UTC),
		testFailures: 5,
		testFlakes:   3,
		cluster:      "build02",
		url:          "https://prow.ci/runA2",
	})
	td.runA3 = createSingleRun(t, dbc, td.jobAWS.ID, "4.16", runSpec{
		timestamp: time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC),
		succeeded: true,
		cluster:   "build01",
		url:       "https://prow.ci/runA3",
	})
	td.runG1 = createSingleRun(t, dbc, td.jobGCP.ID, "4.16", runSpec{
		timestamp:    time.Date(2024, 7, 11, 12, 0, 0, 0, time.UTC),
		succeeded:    true,
		testFailures: 1,
		cluster:      "build03",
		url:          "https://prow.ci/runG1",
	})
	td.runOther = createSingleRun(t, dbc, td.jobOther.ID, "4.15", runSpec{
		timestamp: time.Date(2024, 7, 10, 12, 0, 0, 0, time.UTC),
		succeeded: true,
		url:       "https://prow.ci/runOther",
	})
	td.runOutsideLookback = createSingleRun(t, dbc, td.jobAWS.ID, "4.16", runSpec{
		timestamp: jrLookback.Add(-24 * time.Hour),
		succeeded: true,
		url:       "https://prow.ci/runOld",
	})
	td.runAtReportEnd = createSingleRun(t, dbc, td.jobAWS.ID, "4.16", runSpec{
		timestamp: jrReportEnd,
		succeeded: true,
		url:       "https://prow.ci/runAtEnd",
	})

	td.testEtcd = intutil.CreateTest(t, dbc, "openshift-tests.etcd-leader-election")
	td.testNetwork = intutil.CreateTest(t, dbc, "openshift-tests.network-connectivity")
	td.testUpgrade = intutil.CreateTest(t, dbc, "openshift-tests.upgrade-cluster")

	// runA1: test_failures=2, test_flakes=1
	testExtra1 := intutil.CreateTest(t, dbc, "openshift-tests.extra-failure-1")
	intutil.CreateProwJobRunTest(t, dbc, td.runA1.ID, td.runA1.ProwJobID, td.testEtcd.ID, "4.16", td.runA1.Timestamp, int(v1.TestStatusFailure))
	intutil.CreateProwJobRunTest(t, dbc, td.runA1.ID, td.runA1.ProwJobID, testExtra1.ID, "4.16", td.runA1.Timestamp, int(v1.TestStatusFailure))
	intutil.CreateProwJobRunTest(t, dbc, td.runA1.ID, td.runA1.ProwJobID, td.testNetwork.ID, "4.16", td.runA1.Timestamp, int(v1.TestStatusFlake))

	// runA2: 5 failed, 3 flaked
	testExtra2 := intutil.CreateTest(t, dbc, "openshift-tests.extra-failure-2")
	testExtra3 := intutil.CreateTest(t, dbc, "openshift-tests.extra-failure-3")
	testExtra4 := intutil.CreateTest(t, dbc, "openshift-tests.extra-failure-4")
	testFlakeExtra1 := intutil.CreateTest(t, dbc, "openshift-tests.flake-extra-1")
	testFlakeExtra2 := intutil.CreateTest(t, dbc, "openshift-tests.flake-extra-2")
	intutil.CreateProwJobRunTest(t, dbc, td.runA2.ID, td.runA2.ProwJobID, td.testEtcd.ID, "4.16", td.runA2.Timestamp, int(v1.TestStatusFailure))
	intutil.CreateProwJobRunTest(t, dbc, td.runA2.ID, td.runA2.ProwJobID, td.testNetwork.ID, "4.16", td.runA2.Timestamp, int(v1.TestStatusFailure))
	intutil.CreateProwJobRunTest(t, dbc, td.runA2.ID, td.runA2.ProwJobID, testExtra2.ID, "4.16", td.runA2.Timestamp, int(v1.TestStatusFailure))
	intutil.CreateProwJobRunTest(t, dbc, td.runA2.ID, td.runA2.ProwJobID, testExtra3.ID, "4.16", td.runA2.Timestamp, int(v1.TestStatusFailure))
	intutil.CreateProwJobRunTest(t, dbc, td.runA2.ID, td.runA2.ProwJobID, testExtra4.ID, "4.16", td.runA2.Timestamp, int(v1.TestStatusFailure))
	intutil.CreateProwJobRunTest(t, dbc, td.runA2.ID, td.runA2.ProwJobID, td.testUpgrade.ID, "4.16", td.runA2.Timestamp, int(v1.TestStatusFlake))
	intutil.CreateProwJobRunTest(t, dbc, td.runA2.ID, td.runA2.ProwJobID, testFlakeExtra1.ID, "4.16", td.runA2.Timestamp, int(v1.TestStatusFlake))
	intutil.CreateProwJobRunTest(t, dbc, td.runA2.ID, td.runA2.ProwJobID, testFlakeExtra2.ID, "4.16", td.runA2.Timestamp, int(v1.TestStatusFlake))

	// runG1: 1 failed
	intutil.CreateProwJobRunTest(t, dbc, td.runG1.ID, td.runG1.ProwJobID, td.testUpgrade.ID, "4.16", td.runG1.Timestamp, int(v1.TestStatusFailure))

	// runA3: upgrade passed
	intutil.CreateProwJobRunTest(t, dbc, td.runA3.ID, td.runA3.ProwJobID, td.testUpgrade.ID, "4.16", td.runA3.Timestamp, int(v1.TestStatusSuccess))

	// Pull request linked to runA1
	td.pr1 = jrCreatePullRequest(t, dbc, "openshift", "origin", 100, "dev1", "abc123", "https://github.com/openshift/origin/pull/100")
	jrLinkRunToPR(t, dbc, td.runA1, td.pr1.ID)

	// Annotation on runA2
	jrCreateAnnotation(t, dbc, td.runA2.ID, "4.16", td.runA2.Timestamp, "jira/trt", "TRT-1234")

	return td
}

// Helpers (prefixed with jr to avoid conflicts with component_readiness_test.go)

func jrCreatePullRequest(t *testing.T, dbc *db.DB, org, repo string, number int, author, sha, link string) models.ProwPullRequest {
	t.Helper()
	pr := models.ProwPullRequest{
		Org:    org,
		Repo:   repo,
		Number: number,
		Author: author,
		SHA:    sha,
		Link:   link,
	}
	require.NoError(t, dbc.DB.Create(&pr).Error)
	return pr
}

func jrLinkRunToPR(t *testing.T, dbc *db.DB, run models.ProwJobRun, prID uint) {
	t.Helper()
	link := models.ProwJobRunProwPullRequest{
		ProwJobRunID:        run.ID,
		ProwPullRequestID:   prID,
		ProwJobRunRelease:   run.ProwJobRelease,
		ProwJobRunTimestamp: run.Timestamp,
	}
	require.NoError(t, dbc.DB.Create(&link).Error)
}

func jrCreateAnnotation(t *testing.T, dbc *db.DB, runID uint, release string, timestamp time.Time, key, value string) {
	t.Helper()
	ann := models.ProwJobRunAnnotation{
		ProwJobRunID:        runID,
		Key:                 key,
		Value:               value,
		ProwJobRunRelease:   release,
		ProwJobRunTimestamp: timestamp,
	}
	require.NoError(t, dbc.DB.Create(&ann).Error)
}

func callJobRunsReport(t *testing.T, dbc *db.DB, release string, filterOpts *filter.FilterOptions, pagination *apitype.Pagination, end time.Time) *apitype.PaginationResult {
	t.Helper()
	result, err := api.JobsRunsReportFromDB(dbc, filterOpts, release, pagination, end)
	require.NoError(t, err)
	return result
}

func jobRunsFromResult(t *testing.T, result *apitype.PaginationResult) []apitype.JobRun {
	t.Helper()
	runs, ok := result.Rows.([]apitype.JobRun)
	require.True(t, ok, "result.Rows should be []apitype.JobRun")
	return runs
}

func defaultFilterOpts() *filter.FilterOptions {
	return &filter.FilterOptions{Filter: &filter.Filter{}}
}

func defaultPagination() *apitype.Pagination {
	return &apitype.Pagination{PerPage: 25, Page: 0}
}

// idInt converts a GORM model uint ID to the int type used in the API response.
func idInt(id uint) int {
	return int(id) //nolint:gosec // DB auto-increment IDs are well within int range
}

func findRunByID(runs []apitype.JobRun, id uint) *apitype.JobRun {
	for i := range runs {
		if runs[i].ID == idInt(id) {
			return &runs[i]
		}
	}
	return nil
}

func runIDs(runs []apitype.JobRun) []int {
	ids := make([]int, len(runs))
	for i, r := range runs {
		ids[i] = r.ID
	}
	return ids
}

// Tests

func TestJobRunsReport_BasicQuery(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	td := setupJobRunsTestData(t, dbc)

	result := callJobRunsReport(t, dbc, "4.16", defaultFilterOpts(), defaultPagination(), jrReportEnd)

	assert.Equal(t, int64(4), result.TotalRows, "should have 4 runs for release 4.16 within time window")

	runs := jobRunsFromResult(t, result)
	require.Len(t, runs, 4)

	r := findRunByID(runs, td.runA1.ID)
	require.NotNil(t, r, "runA1 should be in results")
	assert.Equal(t, idInt(td.runA1.ID), r.ID) //nolint:gosec // DB IDs are well within int range
	assert.Equal(t, td.jobAWS.Name, r.Job)
	assert.Equal(t, "e2e-aws-ovn", r.BriefName, "regexp_replace should strip the periodic prefix")
	assert.ElementsMatch(t, pq.StringArray{"aws", "ovn"}, r.Variants)
	assert.Equal(t, string(v1.JobSucceeded), string(r.OverallResult))
	assert.Equal(t, "https://prow.ci/runA1", r.URL)
	assert.Equal(t, "https://prow.ci/runA1", r.TestGridURL)
	assert.True(t, r.Succeeded)
	assert.False(t, r.InfrastructureFailure)
	assert.False(t, r.KnownFailure)
	assert.Equal(t, "build01", r.Cluster)
	assert.Equal(t, 2, r.TestFailures)
	assert.Equal(t, 1, r.TestFlakes)

	expectedTimestampMs := td.runA1.Timestamp.UnixMilli()
	assert.Equal(t, int(expectedTimestampMs), r.Timestamp, "timestamp should be epoch milliseconds")
}

func TestJobRunsReport_BriefNameWithMainBranch(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	job := models.ProwJob{
		Name:     "periodic-ci-openshift-release-main-nightly-4.16-e2e-azure-ovn",
		Release:  "4.16",
		Variants: pq.StringArray{"azure", "ovn"},
	}
	require.NoError(t, dbc.DB.Create(&job).Error)

	run := createSingleRun(t, dbc, job.ID, "4.16", runSpec{
		timestamp: time.Date(2024, 7, 10, 12, 0, 0, 0, time.UTC),
		succeeded: true,
		url:       "https://prow.ci/runMain",
	})

	result := callJobRunsReport(t, dbc, "4.16", defaultFilterOpts(), defaultPagination(), jrReportEnd)
	runs := jobRunsFromResult(t, result)

	r := findRunByID(runs, run.ID)
	require.NotNil(t, r, "main-branch run should be in results")
	assert.Equal(t, "e2e-azure-ovn", r.BriefName, "regexp_replace should strip the periodic prefix for main-branch jobs")
}

func TestJobRunsReport_Pagination(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	setupJobRunsTestData(t, dbc)

	opts := defaultFilterOpts()
	opts.SortField = "timestamp"
	opts.Sort = apitype.SortDescending

	page0 := callJobRunsReport(t, dbc, "4.16", opts, &apitype.Pagination{PerPage: 2, Page: 0}, jrReportEnd)
	page1 := callJobRunsReport(t, dbc, "4.16", opts, &apitype.Pagination{PerPage: 2, Page: 1}, jrReportEnd)

	assert.Equal(t, page0.TotalRows, page1.TotalRows, "total rows should be consistent across pages")
	assert.Equal(t, int64(4), page0.TotalRows)

	runs0 := jobRunsFromResult(t, page0)
	runs1 := jobRunsFromResult(t, page1)
	assert.Len(t, runs0, 2)
	assert.Len(t, runs1, 2)

	ids0 := runIDs(runs0)
	ids1 := runIDs(runs1)
	for _, id := range ids0 {
		assert.NotContains(t, ids1, id, "pages should not overlap")
	}
}

func TestJobRunsReport_PaginationStableWithTiedSortKey(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	td := setupJobRunsTestData(t, dbc)

	opts := defaultFilterOpts()
	opts.SortField = "test_flakes"
	opts.Sort = apitype.SortAscending

	var allIDs []int
	for page := 0; page < 4; page++ {
		result := callJobRunsReport(t, dbc, "4.16", opts, &apitype.Pagination{PerPage: 1, Page: page}, jrReportEnd)
		runs := jobRunsFromResult(t, result)
		require.Len(t, runs, 1, "page %d should have exactly 1 run", page)
		allIDs = append(allIDs, runs[0].ID)
	}

	assert.ElementsMatch(t,
		[]int{idInt(td.runA1.ID), idInt(td.runA2.ID), idInt(td.runA3.ID), idInt(td.runG1.ID)},
		allIDs,
		"paginating one-by-one through a tied sort key should return every run exactly once")
}

func TestJobRunsReport_PaginationDeterministicWithoutSort(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	td := setupJobRunsTestData(t, dbc)

	var allIDs []int
	for page := 0; page < 4; page++ {
		result := callJobRunsReport(t, dbc, "4.16", defaultFilterOpts(), &apitype.Pagination{PerPage: 1, Page: page}, jrReportEnd)
		runs := jobRunsFromResult(t, result)
		require.Len(t, runs, 1, "page %d should have exactly 1 run", page)
		allIDs = append(allIDs, runs[0].ID)
	}

	assert.ElementsMatch(t,
		[]int{idInt(td.runA1.ID), idInt(td.runA2.ID), idInt(td.runA3.ID), idInt(td.runG1.ID)},
		allIDs,
		"paginating without an explicit sort field should still return every run exactly once")
}

func TestJobRunsReport_NoPagination(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	setupJobRunsTestData(t, dbc)

	result := callJobRunsReport(t, dbc, "4.16", defaultFilterOpts(), nil, jrReportEnd)
	runs := jobRunsFromResult(t, result)

	assert.Equal(t, int64(4), result.TotalRows)
	assert.Len(t, runs, 4, "nil pagination should return all rows")
	assert.Equal(t, 4, result.PageSize)
}

func TestJobRunsReport_ReleaseFilter(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	intutil.CreateReleaseDefinition(t, dbc, "4.16", 4, 16)
	td := setupJobRunsTestData(t, dbc)

	t.Run("release 4.16", func(t *testing.T) {
		result := callJobRunsReport(t, dbc, "4.16", defaultFilterOpts(), defaultPagination(), jrReportEnd)
		runs := jobRunsFromResult(t, result)
		ids := runIDs(runs)
		assert.NotContains(t, ids, idInt(td.runOther.ID), "4.15 runs should be excluded") //nolint:gosec // DB IDs are well within int range
	})

	t.Run("release 4.15", func(t *testing.T) {
		result := callJobRunsReport(t, dbc, "4.15", defaultFilterOpts(), defaultPagination(), jrReportEnd)
		runs := jobRunsFromResult(t, result)
		require.Len(t, runs, 1)
		assert.Equal(t, idInt(td.runOther.ID), runs[0].ID) //nolint:gosec // DB IDs are well within int range
	})

	t.Run("empty release returns error", func(t *testing.T) {
		_, err := api.JobsRunsReportFromDB(dbc, defaultFilterOpts(), "", defaultPagination(), jrReportEnd)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "release is required")
	})
}

func TestJobRunsReport_TimestampWindow(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	td := setupJobRunsTestData(t, dbc)

	result := callJobRunsReport(t, dbc, "4.16", defaultFilterOpts(), defaultPagination(), jrReportEnd)
	runs := jobRunsFromResult(t, result)
	ids := runIDs(runs)

	assert.NotContains(t, ids, idInt(td.runOutsideLookback.ID), "run before jrLookback should be excluded")          //nolint:gosec // DB IDs are well within int range
	assert.NotContains(t, ids, idInt(td.runAtReportEnd.ID), "run at jrReportEnd should be excluded (uses < not <=)") //nolint:gosec // DB IDs are well within int range
	assert.Contains(t, ids, idInt(td.runA1.ID), "run within window should be included")                              //nolint:gosec // DB IDs are well within int range
	assert.Contains(t, ids, idInt(td.runA3.ID), "run near jrLookback boundary but within window should be included") //nolint:gosec // DB IDs are well within int range
}

func TestJobRunsReport_SortByTimestamp(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	setupJobRunsTestData(t, dbc)

	t.Run("descending", func(t *testing.T) {
		opts := &filter.FilterOptions{
			Filter:    &filter.Filter{},
			SortField: "timestamp",
			Sort:      apitype.SortDescending,
		}
		result := callJobRunsReport(t, dbc, "4.16", opts, defaultPagination(), jrReportEnd)
		runs := jobRunsFromResult(t, result)
		for i := 1; i < len(runs); i++ {
			assert.GreaterOrEqual(t, runs[i-1].Timestamp, runs[i].Timestamp,
				"runs should be ordered by timestamp descending")
		}
	})

	t.Run("ascending", func(t *testing.T) {
		opts := &filter.FilterOptions{
			Filter:    &filter.Filter{},
			SortField: "timestamp",
			Sort:      apitype.SortAscending,
		}
		result := callJobRunsReport(t, dbc, "4.16", opts, defaultPagination(), jrReportEnd)
		runs := jobRunsFromResult(t, result)
		for i := 1; i < len(runs); i++ {
			assert.LessOrEqual(t, runs[i-1].Timestamp, runs[i].Timestamp,
				"runs should be ordered by timestamp ascending")
		}
	})
}

func TestJobRunsReport_SortByTestFailures(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	td := setupJobRunsTestData(t, dbc)

	opts := &filter.FilterOptions{
		Filter:    &filter.Filter{},
		SortField: "test_failures",
		Sort:      apitype.SortDescending,
	}
	result := callJobRunsReport(t, dbc, "4.16", opts, defaultPagination(), jrReportEnd)
	runs := jobRunsFromResult(t, result)
	require.NotEmpty(t, runs)
	assert.Equal(t, idInt(td.runA2.ID), runs[0].ID, "run with most failures should be first")
	for i := 1; i < len(runs); i++ {
		assert.GreaterOrEqual(t, runs[i-1].TestFailures, runs[i].TestFailures)
	}
}

func TestJobRunsReport_SortByTestFlakes(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	td := setupJobRunsTestData(t, dbc)

	opts := &filter.FilterOptions{
		Filter:    &filter.Filter{},
		SortField: "test_flakes",
		Sort:      apitype.SortDescending,
	}
	result := callJobRunsReport(t, dbc, "4.16", opts, defaultPagination(), jrReportEnd)
	runs := jobRunsFromResult(t, result)
	require.NotEmpty(t, runs)
	assert.Equal(t, idInt(td.runA2.ID), runs[0].ID, "run with most flakes should be first")
	for i := 1; i < len(runs); i++ {
		assert.GreaterOrEqual(t, runs[i-1].TestFlakes, runs[i].TestFlakes)
	}
}

func TestJobRunsReport_SortByJob(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	setupJobRunsTestData(t, dbc)

	opts := &filter.FilterOptions{
		Filter:    &filter.Filter{},
		SortField: "job",
		Sort:      apitype.SortAscending,
	}
	result := callJobRunsReport(t, dbc, "4.16", opts, defaultPagination(), jrReportEnd)
	runs := jobRunsFromResult(t, result)
	for i := 1; i < len(runs); i++ {
		assert.LessOrEqual(t, runs[i-1].Job, runs[i].Job,
			"runs should be ordered by job name ascending")
	}
}

func TestJobRunsReport_FilterByJob(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	td := setupJobRunsTestData(t, dbc)

	tests := []struct {
		name     string
		operator filter.Operator
		value    string
		wantIDs  []int
	}{
		{
			name:     "contains aws",
			operator: filter.OperatorContains,
			value:    "aws",
			wantIDs:  []int{idInt(td.runA1.ID), idInt(td.runA2.ID), idInt(td.runA3.ID)},
		},
		{
			name:     "contains gcp",
			operator: filter.OperatorContains,
			value:    "gcp",
			wantIDs:  []int{idInt(td.runG1.ID)},
		},
		{
			name:     "equals exact name",
			operator: filter.OperatorEquals,
			value:    td.jobGCP.Name,
			wantIDs:  []int{idInt(td.runG1.ID)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &filter.FilterOptions{
				Filter: &filter.Filter{
					Items: []filter.FilterItem{
						{Field: "job", Operator: tt.operator, Value: tt.value},
					},
				},
			}
			result := callJobRunsReport(t, dbc, "4.16", opts, defaultPagination(), jrReportEnd)
			runs := jobRunsFromResult(t, result)
			assert.ElementsMatch(t, tt.wantIDs, runIDs(runs))
		})
	}
}

func TestJobRunsReport_FilterByTestFailures(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	td := setupJobRunsTestData(t, dbc)

	tests := []struct {
		name     string
		operator filter.Operator
		value    string
		wantIDs  []int
	}{
		{
			name:     "greater than 0",
			operator: filter.OperatorArithmeticGreaterThan,
			value:    "0",
			wantIDs:  []int{idInt(td.runA1.ID), idInt(td.runA2.ID), idInt(td.runG1.ID)},
		},
		{
			name:     "greater than or equal to 5",
			operator: filter.OperatorArithmeticGreaterThanOrEquals,
			value:    "5",
			wantIDs:  []int{idInt(td.runA2.ID)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &filter.FilterOptions{
				Filter: &filter.Filter{
					Items: []filter.FilterItem{
						{Field: "test_failures", Operator: tt.operator, Value: tt.value},
					},
				},
			}
			result := callJobRunsReport(t, dbc, "4.16", opts, defaultPagination(), jrReportEnd)
			runs := jobRunsFromResult(t, result)
			assert.ElementsMatch(t, tt.wantIDs, runIDs(runs))
		})
	}
}

func TestJobRunsReport_FilterByTestFlakes(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	td := setupJobRunsTestData(t, dbc)

	tests := []struct {
		name     string
		operator filter.Operator
		value    string
		wantIDs  []int
	}{
		{
			name:     "greater than 0",
			operator: filter.OperatorArithmeticGreaterThan,
			value:    "0",
			wantIDs:  []int{idInt(td.runA1.ID), idInt(td.runA2.ID)},
		},
		{
			name:     "equals 0",
			operator: filter.OperatorArithmeticEquals,
			value:    "0",
			wantIDs:  []int{idInt(td.runA3.ID), idInt(td.runG1.ID)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &filter.FilterOptions{
				Filter: &filter.Filter{
					Items: []filter.FilterItem{
						{Field: "test_flakes", Operator: tt.operator, Value: tt.value},
					},
				},
			}
			result := callJobRunsReport(t, dbc, "4.16", opts, defaultPagination(), jrReportEnd)
			runs := jobRunsFromResult(t, result)
			assert.ElementsMatch(t, tt.wantIDs, runIDs(runs))
		})
	}
}

func TestJobRunsReport_FilterByFailedTestNames(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	td := setupJobRunsTestData(t, dbc)

	tests := []struct {
		name     string
		operator filter.Operator
		value    string
		wantIDs  []int
	}{
		{
			name:     "contains etcd",
			operator: filter.OperatorContains,
			value:    "etcd",
			wantIDs:  []int{idInt(td.runA1.ID), idInt(td.runA2.ID)},
		},
		{
			name:     "isEmpty",
			operator: filter.OperatorIsEmpty,
			wantIDs:  []int{idInt(td.runA3.ID)},
		},
		{
			name:     "isNotEmpty",
			operator: filter.OperatorIsNotEmpty,
			wantIDs:  []int{idInt(td.runA1.ID), idInt(td.runA2.ID), idInt(td.runG1.ID)},
		},
		{
			name:     "hasEntry exact name",
			operator: filter.OperatorHasEntry,
			value:    "openshift-tests.etcd-leader-election",
			wantIDs:  []int{idInt(td.runA1.ID), idInt(td.runA2.ID)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &filter.FilterOptions{
				Filter: &filter.Filter{
					Items: []filter.FilterItem{
						{Field: "failed_test_names", Operator: tt.operator, Value: tt.value},
					},
				},
			}
			result := callJobRunsReport(t, dbc, "4.16", opts, defaultPagination(), jrReportEnd)
			runs := jobRunsFromResult(t, result)
			assert.ElementsMatch(t, tt.wantIDs, runIDs(runs))
		})
	}
}

func TestJobRunsReport_FilterByFlakedTestNames(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	td := setupJobRunsTestData(t, dbc)

	opts := &filter.FilterOptions{
		Filter: &filter.Filter{
			Items: []filter.FilterItem{
				{Field: "flaked_test_names", Operator: filter.OperatorContains, Value: "network"},
			},
		},
	}
	result := callJobRunsReport(t, dbc, "4.16", opts, defaultPagination(), jrReportEnd)
	runs := jobRunsFromResult(t, result)
	assert.ElementsMatch(t, []int{idInt(td.runA1.ID)}, runIDs(runs),
		"only runA1 has a flaked test matching 'network'")
}

func TestJobRunsReport_FilterByRanTestNames(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	td := setupJobRunsTestData(t, dbc)

	opts := &filter.FilterOptions{
		Filter: &filter.Filter{
			Items: []filter.FilterItem{
				{Field: "ran_test_names", Operator: filter.OperatorContains, Value: "upgrade"},
			},
		},
	}
	result := callJobRunsReport(t, dbc, "4.16", opts, defaultPagination(), jrReportEnd)
	runs := jobRunsFromResult(t, result)
	ids := runIDs(runs)

	assert.Contains(t, ids, idInt(td.runA2.ID), "runA2 has upgrade test flaked (status 13)")
	assert.Contains(t, ids, idInt(td.runG1.ID), "runG1 has upgrade test failed (status 12)")
	assert.Contains(t, ids, idInt(td.runA3.ID), "runA3 has upgrade test passed (status 1)")
	assert.NotContains(t, ids, idInt(td.runA1.ID), "runA1 did not run upgrade test")
}

func TestJobRunsReport_FilterByPRFields(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	td := setupJobRunsTestData(t, dbc)

	opts := &filter.FilterOptions{
		Filter: &filter.Filter{
			Items: []filter.FilterItem{
				{Field: "pull_request_author", Operator: filter.OperatorEquals, Value: "dev1"},
			},
		},
	}
	result := callJobRunsReport(t, dbc, "4.16", opts, defaultPagination(), jrReportEnd)
	runs := jobRunsFromResult(t, result)
	require.Len(t, runs, 1)
	assert.Equal(t, idInt(td.runA1.ID), runs[0].ID)
	assert.Equal(t, "dev1", runs[0].PullRequestAuthor)
}

func TestJobRunsReport_ORLinkOperator(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	td := setupJobRunsTestData(t, dbc)

	opts := &filter.FilterOptions{
		Filter: &filter.Filter{
			Items: []filter.FilterItem{
				{Field: "job", Operator: filter.OperatorContains, Value: "gcp"},
				{Field: "test_failures", Operator: filter.OperatorArithmeticGreaterThanOrEquals, Value: "5"},
			},
			LinkOperator: filter.LinkOperatorOr,
		},
	}
	result := callJobRunsReport(t, dbc, "4.16", opts, defaultPagination(), jrReportEnd)
	runs := jobRunsFromResult(t, result)
	ids := runIDs(runs)

	assert.Contains(t, ids, idInt(td.runG1.ID), "GCP run should match the job filter")
	assert.Contains(t, ids, idInt(td.runA2.ID), "runA2 with 5 failures should match the test_failures filter")
	assert.Len(t, runs, 2)
}

func TestJobRunsReport_SortByTestNameFieldRejected(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	setupJobRunsTestData(t, dbc)

	for _, field := range []string{"failed_test_names", "flaked_test_names", "ran_test_names"} {
		t.Run(field, func(t *testing.T) {
			opts := &filter.FilterOptions{
				Filter:    &filter.Filter{},
				SortField: field,
				Sort:      apitype.SortDescending,
			}
			_, err := api.JobsRunsReportFromDB(dbc, opts, "4.16", defaultPagination(), jrReportEnd)
			require.Error(t, err)
			var validationErr *api.ValidationError
			assert.ErrorAs(t, err, &validationErr, "should return ValidationError")
		})
	}
}

func TestJobRunsReport_UnsupportedTestNameOperator(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	setupJobRunsTestData(t, dbc)

	opts := &filter.FilterOptions{
		Filter: &filter.Filter{
			Items: []filter.FilterItem{
				{Field: "failed_test_names", Operator: filter.OperatorArithmeticGreaterThan, Value: "5"},
			},
		},
	}
	_, err := api.JobsRunsReportFromDB(dbc, opts, "4.16", defaultPagination(), jrReportEnd)
	require.Error(t, err)
	var validationErr *api.ValidationError
	assert.ErrorAs(t, err, &validationErr, "unsupported operator should return ValidationError")
}

func TestJobRunsReport_Enrichment(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	td := setupJobRunsTestData(t, dbc)

	result := callJobRunsReport(t, dbc, "4.16", defaultFilterOpts(), defaultPagination(), jrReportEnd)
	runs := jobRunsFromResult(t, result)

	t.Run("failed and flaked test names", func(t *testing.T) {
		r := findRunByID(runs, td.runA1.ID)
		require.NotNil(t, r)

		sortedFailed := make([]string, len(r.FailedTestNames))
		copy(sortedFailed, r.FailedTestNames)
		sort.Strings(sortedFailed)

		assert.Contains(t, sortedFailed, "openshift-tests.etcd-leader-election",
			"failed_test_names should include the failed etcd test")
		assert.Contains(t, sortedFailed, "openshift-tests.extra-failure-1",
			"failed_test_names should include the extra failure")

		assert.Len(t, r.FlakedTestNames, 1)
		assert.Contains(t, r.FlakedTestNames, "openshift-tests.network-connectivity",
			"flaked_test_names should include the flaked network test")
	})

	t.Run("pull request data", func(t *testing.T) {
		r := findRunByID(runs, td.runA1.ID)
		require.NotNil(t, r)
		assert.Equal(t, "https://github.com/openshift/origin/pull/100", r.PullRequestLink)
		assert.Equal(t, "abc123", r.PullRequestSHA)
		assert.Equal(t, "openshift", r.PullRequestOrg)
		assert.Equal(t, "origin", r.PullRequestRepo)
		assert.Equal(t, "dev1", r.PullRequestAuthor)
	})

	t.Run("no pull request data for unlinked run", func(t *testing.T) {
		r := findRunByID(runs, td.runG1.ID)
		require.NotNil(t, r)
		assert.Empty(t, r.PullRequestLink)
		assert.Empty(t, r.PullRequestAuthor)
	})

	t.Run("annotations", func(t *testing.T) {
		r := findRunByID(runs, td.runA2.ID)
		require.NotNil(t, r)
		require.NotNil(t, r.Annotations)
		assert.Equal(t, "TRT-1234", r.Annotations["jira/trt"])
	})

	t.Run("no annotations for unannotated run", func(t *testing.T) {
		r := findRunByID(runs, td.runA3.ID)
		require.NotNil(t, r)
		assert.Empty(t, r.Annotations)
	})
}

func TestJobRunsReport_NOTNegationExcludesMatchingRuns(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	td := setupJobRunsTestData(t, dbc)

	t.Run("NOT job contains aws returns only GCP run", func(t *testing.T) {
		opts := &filter.FilterOptions{
			Filter: &filter.Filter{
				Items: []filter.FilterItem{
					{Field: "job", Operator: filter.OperatorContains, Value: "aws", Not: true},
				},
			},
		}
		result := callJobRunsReport(t, dbc, "4.16", opts, defaultPagination(), jrReportEnd)
		runs := jobRunsFromResult(t, result)
		assert.ElementsMatch(t, []int{idInt(td.runG1.ID)}, runIDs(runs))
	})

	t.Run("NOT failed_test_names contains etcd excludes runs with etcd failures", func(t *testing.T) {
		opts := &filter.FilterOptions{
			Filter: &filter.Filter{
				Items: []filter.FilterItem{
					{Field: "failed_test_names", Operator: filter.OperatorContains, Value: "etcd", Not: true},
				},
			},
		}
		result := callJobRunsReport(t, dbc, "4.16", opts, defaultPagination(), jrReportEnd)
		runs := jobRunsFromResult(t, result)
		ids := runIDs(runs)
		assert.NotContains(t, ids, idInt(td.runA1.ID), "runA1 has etcd failure, should be excluded")
		assert.NotContains(t, ids, idInt(td.runA2.ID), "runA2 has etcd failure, should be excluded")
		assert.Contains(t, ids, idInt(td.runG1.ID))
		assert.Contains(t, ids, idInt(td.runA3.ID))
	})
}

func TestJobRunsReport_MultipleANDFiltersNarrowResults(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	td := setupJobRunsTestData(t, dbc)

	opts := &filter.FilterOptions{
		Filter: &filter.Filter{
			Items: []filter.FilterItem{
				{Field: "job", Operator: filter.OperatorContains, Value: "aws"},
				{Field: "test_failures", Operator: filter.OperatorArithmeticGreaterThan, Value: "2"},
			},
		},
	}
	result := callJobRunsReport(t, dbc, "4.16", opts, defaultPagination(), jrReportEnd)
	runs := jobRunsFromResult(t, result)
	assert.ElementsMatch(t, []int{idInt(td.runA2.ID)}, runIDs(runs),
		"only runA2 is an aws job with >2 test failures")
}

func TestJobRunsReport_SpecialCharactersInFilterValueAreLiteral(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	td := setupJobRunsTestData(t, dbc)

	specialTest := intutil.CreateTest(t, dbc, "openshift-tests.100%_coverage")
	intutil.CreateProwJobRunTest(t, dbc, td.runA3.ID, td.runA3.ProwJobID, specialTest.ID, "4.16", td.runA3.Timestamp, int(v1.TestStatusFailure))

	t.Run("percent in filter is literal", func(t *testing.T) {
		opts := &filter.FilterOptions{
			Filter: &filter.Filter{
				Items: []filter.FilterItem{
					{Field: "failed_test_names", Operator: filter.OperatorContains, Value: "100%"},
				},
			},
		}
		result := callJobRunsReport(t, dbc, "4.16", opts, defaultPagination(), jrReportEnd)
		runs := jobRunsFromResult(t, result)
		assert.ElementsMatch(t, []int{idInt(td.runA3.ID)}, runIDs(runs),
			"only runA3 has the test with literal percent in its name")
	})

	t.Run("underscore in filter is literal", func(t *testing.T) {
		opts := &filter.FilterOptions{
			Filter: &filter.Filter{
				Items: []filter.FilterItem{
					{Field: "failed_test_names", Operator: filter.OperatorContains, Value: "100_"},
				},
			},
		}
		result := callJobRunsReport(t, dbc, "4.16", opts, defaultPagination(), jrReportEnd)
		runs := jobRunsFromResult(t, result)
		assert.Empty(t, runs,
			"underscore should be literal; no test name contains the exact substring '100_'")
	})

	t.Run("percent in job filter via columnAliases path", func(t *testing.T) {
		opts := &filter.FilterOptions{
			Filter: &filter.Filter{
				Items: []filter.FilterItem{
					{Field: "job", Operator: filter.OperatorContains, Value: "4.16-e2e_"},
				},
			},
		}
		result := callJobRunsReport(t, dbc, "4.16", opts, defaultPagination(), jrReportEnd)
		runs := jobRunsFromResult(t, result)
		assert.Empty(t, runs, "underscore should be literal; no job name contains '4.16-e2e_'")
	})
}

func TestJobRunsReport_SortByPRFieldDoesNotInflateTotalRows(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	td := setupJobRunsTestData(t, dbc)

	opts := &filter.FilterOptions{
		Filter:    &filter.Filter{},
		SortField: "pull_request_author",
		Sort:      apitype.SortAscending,
	}
	result := callJobRunsReport(t, dbc, "4.16", opts, defaultPagination(), jrReportEnd)
	assert.Equal(t, int64(4), result.TotalRows, "LEFT JOIN for PR sort should not inflate TotalRows")

	runs := jobRunsFromResult(t, result)
	r := findRunByID(runs, td.runA1.ID)
	require.NotNil(t, r)
	assert.Equal(t, "dev1", r.PullRequestAuthor, "PR data should be populated via JOIN path")
}

func TestJobRunsReport_PRDataPopulatedWhenFilteredByPRField(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	td := setupJobRunsTestData(t, dbc)

	opts := &filter.FilterOptions{
		Filter: &filter.Filter{
			Items: []filter.FilterItem{
				{Field: "pull_request_author", Operator: filter.OperatorEquals, Value: "dev1"},
			},
		},
	}
	result := callJobRunsReport(t, dbc, "4.16", opts, defaultPagination(), jrReportEnd)
	runs := jobRunsFromResult(t, result)
	require.Len(t, runs, 1)
	assert.Equal(t, idInt(td.runA1.ID), runs[0].ID)
	assert.Equal(t, "dev1", runs[0].PullRequestAuthor)
	assert.Equal(t, "https://github.com/openshift/origin/pull/100", runs[0].PullRequestLink)
	assert.Equal(t, "abc123", runs[0].PullRequestSHA)
	assert.Equal(t, "openshift", runs[0].PullRequestOrg)
	assert.Equal(t, "origin", runs[0].PullRequestRepo)
}

func TestJobRunsReport_ZeroMatchingRowsReturnsEmptyResult(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	setupJobRunsTestData(t, dbc)

	opts := &filter.FilterOptions{
		Filter: &filter.Filter{
			Items: []filter.FilterItem{
				{Field: "job", Operator: filter.OperatorEquals, Value: "nonexistent-job"},
			},
		},
	}
	result := callJobRunsReport(t, dbc, "4.16", opts, defaultPagination(), jrReportEnd)
	assert.Equal(t, int64(0), result.TotalRows)
	runs := jobRunsFromResult(t, result)
	assert.Empty(t, runs)
}

func TestJobRunsReport_RunExactlyAtLookbackBoundaryIsIncluded(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	td := setupJobRunsTestData(t, dbc)

	runAtBoundary := createSingleRun(t, dbc, td.jobAWS.ID, "4.16", runSpec{
		timestamp: jrLookback,
		succeeded: true,
		url:       "https://prow.ci/runAtBoundary",
	})

	result := callJobRunsReport(t, dbc, "4.16", defaultFilterOpts(), defaultPagination(), jrReportEnd)
	runs := jobRunsFromResult(t, result)
	ids := runIDs(runs)
	assert.Contains(t, ids, idInt(runAtBoundary.ID),
		"run at exactly the jrLookback boundary should be included (>= semantics)")
}

func TestJobRunsReport_RunWithNoTestFailuresHasEmptyTestNames(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	td := setupJobRunsTestData(t, dbc)

	result := callJobRunsReport(t, dbc, "4.16", defaultFilterOpts(), defaultPagination(), jrReportEnd)
	runs := jobRunsFromResult(t, result)

	r := findRunByID(runs, td.runA3.ID)
	require.NotNil(t, r, "runA3 should be in results")
	assert.Equal(t, 0, r.TestFailures)
	assert.Equal(t, 0, r.TestFlakes)
	assert.Empty(t, r.FailedTestNames, "run with no failures should have empty failed test names")
	assert.Empty(t, r.FlakedTestNames, "run with no flakes should have empty flaked test names")
}

func TestJobRunsReport_StartsWithAndEndsWithFilterTestNames(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	td := setupJobRunsTestData(t, dbc)

	t.Run("startsWith matches prefix", func(t *testing.T) {
		opts := &filter.FilterOptions{
			Filter: &filter.Filter{
				Items: []filter.FilterItem{
					{Field: "failed_test_names", Operator: filter.OperatorStartsWith, Value: "openshift-tests.etcd"},
				},
			},
		}
		result := callJobRunsReport(t, dbc, "4.16", opts, defaultPagination(), jrReportEnd)
		runs := jobRunsFromResult(t, result)
		assert.ElementsMatch(t, []int{idInt(td.runA1.ID), idInt(td.runA2.ID)}, runIDs(runs),
			"only runA1 and runA2 have failed tests starting with 'openshift-tests.etcd'")
	})

	t.Run("endsWith matches suffix", func(t *testing.T) {
		opts := &filter.FilterOptions{
			Filter: &filter.Filter{
				Items: []filter.FilterItem{
					{Field: "failed_test_names", Operator: filter.OperatorEndsWith, Value: "leader-election"},
				},
			},
		}
		result := callJobRunsReport(t, dbc, "4.16", opts, defaultPagination(), jrReportEnd)
		runs := jobRunsFromResult(t, result)
		assert.ElementsMatch(t, []int{idInt(td.runA1.ID), idInt(td.runA2.ID)}, runIDs(runs),
			"only runA1 and runA2 have failed tests ending with 'leader-election'")
	})
}

func TestJobRunsReport_MultipleAnnotationsOnOneRun(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	td := setupJobRunsTestData(t, dbc)

	jrCreateAnnotation(t, dbc, td.runA2.ID, "4.16", td.runA2.Timestamp, "group/team", "team-alpha")

	result := callJobRunsReport(t, dbc, "4.16", defaultFilterOpts(), defaultPagination(), jrReportEnd)
	runs := jobRunsFromResult(t, result)

	r := findRunByID(runs, td.runA2.ID)
	require.NotNil(t, r)
	require.NotNil(t, r.Annotations)
	assert.Equal(t, "TRT-1234", r.Annotations["jira/trt"])
	assert.Equal(t, "team-alpha", r.Annotations["group/team"])
}

func TestJobRunsReport_MultiplePRsOnOneRunReturnsSingleRun(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	td := setupJobRunsTestData(t, dbc)

	pr2 := jrCreatePullRequest(t, dbc, "openshift", "installer", 200, "dev2", "def456", "https://github.com/openshift/installer/pull/200")
	jrLinkRunToPR(t, dbc, td.runA1, pr2.ID)

	result := callJobRunsReport(t, dbc, "4.16", defaultFilterOpts(), defaultPagination(), jrReportEnd)
	assert.Equal(t, int64(4), result.TotalRows, "multiple PRs should not duplicate the run in results")

	runs := jobRunsFromResult(t, result)
	count := 0
	for _, r := range runs {
		if r.ID == idInt(td.runA1.ID) {
			count++
		}
	}
	assert.Equal(t, 1, count, "runA1 should appear exactly once despite having two linked PRs")
}

func TestJobRunsReport_CrossReleaseAnnotationsExcluded(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	td := setupJobRunsTestData(t, dbc)

	jrCreateAnnotation(t, dbc, td.runA1.ID, "4.15", td.runA1.Timestamp, "jira/wrong-release", "TRT-9999")

	result := callJobRunsReport(t, dbc, "4.16", defaultFilterOpts(), defaultPagination(), jrReportEnd)
	runs := jobRunsFromResult(t, result)

	r := findRunByID(runs, td.runA1.ID)
	require.NotNil(t, r)
	if r.Annotations != nil {
		assert.Empty(t, r.Annotations["jira/wrong-release"],
			"annotation for release 4.15 should not appear in 4.16 query results")
	}
}

// Priority 1: Missing arithmetic operators

func TestJobRunsReport_FilterByTestFailuresLessThan(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	td := setupJobRunsTestData(t, dbc)

	tests := []struct {
		name     string
		operator filter.Operator
		value    string
		expected []uint
	}{
		{
			name:     "less than 2 returns runs with 0 and 1 failures",
			operator: filter.OperatorArithmeticLessThan,
			value:    "2",
			expected: []uint{td.runA3.ID, td.runG1.ID},
		},
		{
			name:     "less than or equal 1 returns runs with 0 and 1 failures",
			operator: filter.OperatorArithmeticLessThanOrEquals,
			value:    "1",
			expected: []uint{td.runA3.ID, td.runG1.ID},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opts := &filter.FilterOptions{
				Filter: &filter.Filter{
					Items: []filter.FilterItem{
						{Field: "test_failures", Operator: tc.operator, Value: tc.value},
					},
				},
			}
			result := callJobRunsReport(t, dbc, "4.16", opts, defaultPagination(), jrReportEnd)
			runs := jobRunsFromResult(t, result)
			var expectedIDs []int
			for _, id := range tc.expected {
				expectedIDs = append(expectedIDs, idInt(id))
			}
			assert.ElementsMatch(t, expectedIDs, runIDs(runs))
		})
	}
}

func TestJobRunsReport_FilterByTestFailuresNotEquals(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	td := setupJobRunsTestData(t, dbc)

	opts := &filter.FilterOptions{
		Filter: &filter.Filter{
			Items: []filter.FilterItem{
				{Field: "test_failures", Operator: filter.OperatorArithmeticNotEquals, Value: "0"},
			},
		},
	}
	result := callJobRunsReport(t, dbc, "4.16", opts, defaultPagination(), jrReportEnd)
	runs := jobRunsFromResult(t, result)
	assert.ElementsMatch(t,
		[]int{idInt(td.runA1.ID), idInt(td.runA2.ID), idInt(td.runG1.ID)},
		runIDs(runs),
		"should exclude runA3 which has 0 test failures")
}

// Priority 2: Untested filterable fields

func TestJobRunsReport_FilterByCluster(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	td := setupJobRunsTestData(t, dbc)

	tests := []struct {
		name     string
		cluster  string
		expected []uint
	}{
		{
			name:     "build01 returns two AWS runs",
			cluster:  "build01",
			expected: []uint{td.runA1.ID, td.runA3.ID},
		},
		{
			name:     "build03 returns only GCP run",
			cluster:  "build03",
			expected: []uint{td.runG1.ID},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opts := &filter.FilterOptions{
				Filter: &filter.Filter{
					Items: []filter.FilterItem{
						{Field: "cluster", Operator: filter.OperatorEquals, Value: tc.cluster},
					},
				},
			}
			result := callJobRunsReport(t, dbc, "4.16", opts, defaultPagination(), jrReportEnd)
			runs := jobRunsFromResult(t, result)
			var expectedIDs []int
			for _, id := range tc.expected {
				expectedIDs = append(expectedIDs, idInt(id))
			}
			assert.ElementsMatch(t, expectedIDs, runIDs(runs))
		})
	}
}

func TestJobRunsReport_FilterByBriefName(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	td := setupJobRunsTestData(t, dbc)

	opts := &filter.FilterOptions{
		Filter: &filter.Filter{
			Items: []filter.FilterItem{
				{Field: "brief_name", Operator: filter.OperatorContains, Value: "e2e-aws"},
			},
		},
	}
	result := callJobRunsReport(t, dbc, "4.16", opts, defaultPagination(), jrReportEnd)
	runs := jobRunsFromResult(t, result)
	assert.ElementsMatch(t,
		[]int{idInt(td.runA1.ID), idInt(td.runA2.ID), idInt(td.runA3.ID)},
		runIDs(runs),
		"brief_name 'e2e-aws-ovn' matches all AWS job runs")
}

func TestJobRunsReport_FilterByOverallResult(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	td := setupJobRunsTestData(t, dbc)

	tests := []struct {
		name     string
		result   string
		expected []uint
	}{
		{
			name:     "succeeded runs",
			result:   string(v1.JobSucceeded),
			expected: []uint{td.runA1.ID, td.runA3.ID, td.runG1.ID},
		},
		{
			name:     "test failure runs",
			result:   string(v1.JobTestFailure),
			expected: []uint{td.runA2.ID},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opts := &filter.FilterOptions{
				Filter: &filter.Filter{
					Items: []filter.FilterItem{
						{Field: "overall_result", Operator: filter.OperatorEquals, Value: tc.result},
					},
				},
			}
			result := callJobRunsReport(t, dbc, "4.16", opts, defaultPagination(), jrReportEnd)
			runs := jobRunsFromResult(t, result)
			var expectedIDs []int
			for _, id := range tc.expected {
				expectedIDs = append(expectedIDs, idInt(id))
			}
			assert.ElementsMatch(t, expectedIDs, runIDs(runs))
		})
	}
}

func TestJobRunsReport_FilterByVariants(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	td := setupJobRunsTestData(t, dbc)

	tests := []struct {
		name     string
		variant  string
		expected []uint
	}{
		{
			name:     "aws variant returns all AWS job runs",
			variant:  "aws",
			expected: []uint{td.runA1.ID, td.runA2.ID, td.runA3.ID},
		},
		{
			name:     "sdn variant returns only GCP run",
			variant:  "sdn",
			expected: []uint{td.runG1.ID},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opts := &filter.FilterOptions{
				Filter: &filter.Filter{
					Items: []filter.FilterItem{
						{Field: "variants", Operator: filter.OperatorHasEntry, Value: tc.variant},
					},
				},
			}
			result := callJobRunsReport(t, dbc, "4.16", opts, defaultPagination(), jrReportEnd)
			runs := jobRunsFromResult(t, result)
			var expectedIDs []int
			for _, id := range tc.expected {
				expectedIDs = append(expectedIDs, idInt(id))
			}
			assert.ElementsMatch(t, expectedIDs, runIDs(runs))
		})
	}
}

// Priority 3: Edge cases

func TestJobRunsReport_FilterByTimestampEpoch(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	td := setupJobRunsTestData(t, dbc)

	cutoff := time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)
	cutoffEpochMs := fmt.Sprintf("%d", cutoff.UnixMilli())

	opts := &filter.FilterOptions{
		Filter: &filter.Filter{
			Items: []filter.FilterItem{
				{Field: "timestamp", Operator: filter.OperatorArithmeticGreaterThan, Value: cutoffEpochMs},
			},
		},
	}
	result := callJobRunsReport(t, dbc, "4.16", opts, defaultPagination(), jrReportEnd)
	runs := jobRunsFromResult(t, result)
	assert.ElementsMatch(t,
		[]int{idInt(td.runA1.ID), idInt(td.runA2.ID), idInt(td.runG1.ID)},
		runIDs(runs),
		"should return July runs, excluding May runA3")
}

func TestJobRunsReport_ORCombiningTestNameAndColumnAlias(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	td := setupJobRunsTestData(t, dbc)

	opts := &filter.FilterOptions{
		Filter: &filter.Filter{
			Items: []filter.FilterItem{
				{Field: "flaked_test_names", Operator: filter.OperatorContains, Value: "upgrade"},
				{Field: "job", Operator: filter.OperatorContains, Value: "gcp"},
			},
			LinkOperator: filter.LinkOperatorOr,
		},
	}
	result := callJobRunsReport(t, dbc, "4.16", opts, defaultPagination(), jrReportEnd)
	runs := jobRunsFromResult(t, result)
	assert.ElementsMatch(t,
		[]int{idInt(td.runA2.ID), idInt(td.runG1.ID)},
		runIDs(runs),
		"runA2 has upgrade as flake, runG1 matches GCP job name")
}

func TestJobRunsReport_PaginationBeyondResults(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	setupJobRunsTestData(t, dbc)

	result := callJobRunsReport(t, dbc, "4.16", defaultFilterOpts(), &apitype.Pagination{PerPage: 25, Page: 100}, jrReportEnd)
	assert.Equal(t, int64(4), result.TotalRows, "total should still reflect all matching rows")
	runs := jobRunsFromResult(t, result)
	assert.Empty(t, runs, "page beyond results should return no rows")
}

func TestJobRunsReport_PaginationPartialLastPage(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	setupJobRunsTestData(t, dbc)

	result := callJobRunsReport(t, dbc, "4.16", defaultFilterOpts(), &apitype.Pagination{PerPage: 3, Page: 1}, jrReportEnd)
	assert.Equal(t, int64(4), result.TotalRows, "total rows should be 4")
	runs := jobRunsFromResult(t, result)
	assert.Len(t, runs, 1, "partial last page should have exactly 1 run")
}

func TestJobRunsReport_HasEntryContainingOnTestNames(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	td := setupJobRunsTestData(t, dbc)

	opts := &filter.FilterOptions{
		Filter: &filter.Filter{
			Items: []filter.FilterItem{
				{Field: "failed_test_names", Operator: filter.OperatorHasEntryContaining, Value: "etcd"},
			},
		},
	}
	result := callJobRunsReport(t, dbc, "4.16", opts, defaultPagination(), jrReportEnd)
	runs := jobRunsFromResult(t, result)
	assert.ElementsMatch(t,
		[]int{idInt(td.runA1.ID), idInt(td.runA2.ID)},
		runIDs(runs),
		"hasEntryContaining should match runs with failed tests containing 'etcd'")
}
