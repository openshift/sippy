package integration

import (
	"slices"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/civil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/sets"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/db/query"
	"github.com/openshift/sippy/pkg/filter"
	intutil "github.com/openshift/sippy/test/integration/util"
)

// testsReportDB returns a fresh test database for a Tests-report integration test.
func testsReportDB(t *testing.T) *db.DB {
	t.Helper()
	return intutil.NewTestDB(t, pgContainer)
}

// testsReportPeriods returns fixed sample/base DateRanges for Tests-report tests, along
// with the three prefix-sum lookup dates (start, boundary, end) a test_cumulative_summaries
// row must be seeded at to be visible to the query at that point in the period:
//
//	previous period = (start, boundary], current period = (boundary, end]
//	(resolvePrefixSumDates: end = sample.End-1, boundary = sample.Start-1 = base.End-1, start = base.Start-1)
//
// A row with no prior row at an earlier lookup date is treated as having a baseline of
// zero as of that date (see TestTestReportQueryCollapsed_NewTestWithNoPriorHistoryReportsZeroPrevious),
// so tests that don't care about the previous period may omit the start/boundary rows entirely.
func testsReportPeriods() (sample, base query.DateRange, start, boundary, end civil.Date) {
	sample = query.DateRange{Start: civil.Date{Year: 2024, Month: 6, Day: 8}, End: civil.Date{Year: 2024, Month: 6, Day: 15}}
	base = query.DateRange{Start: civil.Date{Year: 2024, Month: 6, Day: 1}, End: civil.Date{Year: 2024, Month: 6, Day: 8}}
	return sample, base,
		civil.Date{Year: 2024, Month: 5, Day: 31},
		civil.Date{Year: 2024, Month: 6, Day: 7},
		civil.Date{Year: 2024, Month: 6, Day: 14}
}

// runCollapsedReport reproduces the wrapping buildTestsResultsPGGenerator (pkg/api/tests.go,
// unexported) applies around TestReportQueryCollapsed in production: computing percentages
// via QueryTestSummarizer and dropping zero-run rows, neither of which TestReportQueryCollapsed
// does on its own.
func runCollapsedReport(t *testing.T, dbc *db.DB, release string, sample, base query.DateRange, variantFilter, nameFilter, lifecycleFilter *filter.Filter) ([]apitype.Test, error) {
	t.Helper()
	collapsedQuery, err := query.TestReportQueryCollapsed(dbc, release, sample, base, variantFilter, nameFilter, lifecycleFilter)
	if err != nil {
		return nil, err
	}
	testMetadataColumns := []string{"suite_name", "name", "jira_component", "jira_component_id"}
	collapsedColumns := slices.Concat(testMetadataColumns, []string{query.QueryTestFields})
	rawQuery := dbc.DB.Table("(?) AS r", collapsedQuery).Select(strings.Join(collapsedColumns, ","))
	selectColumns := slices.Concat(testMetadataColumns, []string{query.QueryTestSummarizer})
	processedResults := dbc.DB.Table("(?) as results", rawQuery).
		Select(strings.Join(selectColumns, ",")).
		Where("current_runs > 0 or previous_runs > 0")

	var results []apitype.Test
	r := dbc.DB.Table("(?) as final_results", processedResults).Scan(&results)
	return results, r.Error
}

// runUncollapsedReport calls UncollapsedTestReportWithStats and scans the result; unlike
// the collapsed path, percentages and cross-variant stats are computed inside the query
// itself, so no further wrapping is needed to match production output.
func runUncollapsedReport(t *testing.T, dbc *db.DB, release string, sample, base query.DateRange, nameFilter, variantFilter, processedFilter, lifecycleFilter *filter.Filter) ([]apitype.Test, *filter.Filter, error) {
	t.Helper()
	rawQuery, remainingFilter, err := query.UncollapsedTestReportWithStats(dbc, release, sample, base, nameFilter, variantFilter, processedFilter, lifecycleFilter)
	if err != nil {
		return nil, nil, err
	}
	var results []apitype.Test
	r := dbc.DB.Table("(?) as final_results", rawQuery).Scan(&results)
	return results, remainingFilter, r.Error
}

// findTestRow returns the row matching name (and, if suiteName is non-empty, suite) from
// results, failing the test if there's no such row.
func findTestRow(t *testing.T, results []apitype.Test, name, suiteName string) apitype.Test {
	t.Helper()
	for _, r := range results {
		if r.Name == name && (suiteName == "" || r.SuiteName == suiteName) {
			return r
		}
	}
	t.Fatalf("no row found for test %q suite %q among %d results", name, suiteName, len(results))
	return apitype.Test{}
}

// findVariantRow returns the row whose Variants slice matches exactly the given variants
// (order-independent), failing the test if there's no such row. Used to distinguish
// per-variant-combination rows from the uncollapsed report.
func findVariantRow(t *testing.T, results []apitype.Test, name string, variants []string) apitype.Test {
	t.Helper()
	want := sets.New[string](variants...)
	for _, r := range results {
		if r.Name != name {
			continue
		}
		if sets.New[string](r.Variants...).Equal(want) {
			return r
		}
	}
	t.Fatalf("no row found for test %q with variants %v among %d results", name, variants, len(results))
	return apitype.Test{}
}

// --- TestReportQueryCollapsed ---

func TestTestReportQueryCollapsed_AggregatesCurrentAndPreviousPeriods(t *testing.T) {
	dbc := testsReportDB(t)
	release := "4.16"
	sample, base, start, boundary, end := testsReportPeriods()

	vc := intutil.CreateVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
	job := intutil.CreateProwJobWithOptions(t, dbc, "periodic-e2e-aws-ovn", release, nil, intutil.WithVariantCombination(vc))
	test := intutil.CreateTest(t, dbc, "sig-network test-a")
	suite := intutil.CreateSuite(t, dbc, "openshift-tests")

	// previous = boundary - start: runs=8, successes=6, failures=1, flakes=1
	intutil.CreateCumulativeSummary(t, dbc, start, release, test.ID, job.ID, suite.ID, 0, 0, 0)
	intutil.CreateCumulativeSummary(t, dbc, boundary, release, test.ID, job.ID, suite.ID, 8, 6, 1, intutil.WithCumulativeSummaryFailures(1))
	// current = end - boundary: runs=12, successes=8, failures=3, flakes=1
	intutil.CreateCumulativeSummary(t, dbc, end, release, test.ID, job.ID, suite.ID, 20, 14, 2, intutil.WithCumulativeSummaryFailures(4))

	results, err := runCollapsedReport(t, dbc, release, sample, base, nil, nil, nil)
	require.NoError(t, err)
	require.Len(t, results, 1)

	row := results[0]
	assert.Equal(t, "sig-network test-a", row.Name)
	assert.Equal(t, "openshift-tests", row.SuiteName)

	assert.Equal(t, 12, row.CurrentRuns, "current runs")
	assert.Equal(t, 8, row.CurrentSuccesses, "current successes")
	assert.Equal(t, 3, row.CurrentFailures, "current failures")
	assert.Equal(t, 1, row.CurrentFlakes, "current flakes")
	assert.InDelta(t, 66.667, row.CurrentPassPercentage, 0.01, "current pass percentage")
	assert.InDelta(t, 8.333, row.CurrentFlakePercentage, 0.01, "current flake percentage")
	assert.InDelta(t, 25.0, row.CurrentFailurePercentage, 0.01, "current failure percentage")
	assert.InDelta(t, 75.0, row.CurrentWorkingPercentage, 0.01, "current working percentage")

	assert.Equal(t, 8, row.PreviousRuns, "previous runs")
	assert.Equal(t, 6, row.PreviousSuccesses, "previous successes")
	assert.Equal(t, 1, row.PreviousFailures, "previous failures")
	assert.Equal(t, 1, row.PreviousFlakes, "previous flakes")
	assert.InDelta(t, 75.0, row.PreviousPassPercentage, 0.01, "previous pass percentage")
	assert.InDelta(t, 12.5, row.PreviousFlakePercentage, 0.01, "previous flake percentage")
	assert.InDelta(t, 12.5, row.PreviousFailurePercentage, 0.01, "previous failure percentage")
	assert.InDelta(t, 87.5, row.PreviousWorkingPercentage, 0.01, "previous working percentage")

	assert.InDelta(t, -8.333, row.NetImprovement, 0.01, "net pass-rate improvement")
}

func TestTestReportQueryCollapsed_CollapsesAcrossVariantCombinations(t *testing.T) {
	dbc := testsReportDB(t)
	release := "4.16"
	sample, base, _, boundary, end := testsReportPeriods()

	test := intutil.CreateTest(t, dbc, "sig-storage test-a")
	suite := intutil.CreateSuite(t, dbc, "openshift-tests")

	vcAWS := intutil.CreateVariantCombination(t, dbc, []string{"Platform:aws"})
	jobAWS := intutil.CreateProwJobWithOptions(t, dbc, "periodic-e2e-aws", release, nil, intutil.WithVariantCombination(vcAWS))
	// current = 9-5 = 4
	intutil.CreateCumulativeSummary(t, dbc, boundary, release, test.ID, jobAWS.ID, suite.ID, 5, 5, 0)
	intutil.CreateCumulativeSummary(t, dbc, end, release, test.ID, jobAWS.ID, suite.ID, 9, 9, 0)

	vcGCP := intutil.CreateVariantCombination(t, dbc, []string{"Platform:gcp"})
	jobGCP := intutil.CreateProwJobWithOptions(t, dbc, "periodic-e2e-gcp", release, nil, intutil.WithVariantCombination(vcGCP))
	// current = 8-3 = 5
	intutil.CreateCumulativeSummary(t, dbc, boundary, release, test.ID, jobGCP.ID, suite.ID, 3, 3, 0)
	intutil.CreateCumulativeSummary(t, dbc, end, release, test.ID, jobGCP.ID, suite.ID, 8, 8, 0)

	results, err := runCollapsedReport(t, dbc, release, sample, base, nil, nil, nil)
	require.NoError(t, err)
	require.Len(t, results, 1, "collapsed report should merge both variant combinations into one row")

	row := results[0]
	assert.Equal(t, 9, row.CurrentRuns, "current runs should sum across both variant combinations (4+5)")
	assert.Equal(t, 9, row.CurrentSuccesses)
}

func TestTestReportQueryCollapsed_KeepsDifferentSuitesSeparate(t *testing.T) {
	dbc := testsReportDB(t)
	release := "4.16"
	sample, base, _, boundary, end := testsReportPeriods()

	vc := intutil.CreateVariantCombination(t, dbc, []string{"Platform:aws"})
	job := intutil.CreateProwJobWithOptions(t, dbc, "periodic-e2e-aws", release, nil, intutil.WithVariantCombination(vc))
	test := intutil.CreateTest(t, dbc, "sig-network shared-test-name")

	suiteX := intutil.CreateSuite(t, dbc, "suite-x")
	intutil.CreateCumulativeSummary(t, dbc, boundary, release, test.ID, job.ID, suiteX.ID, 0, 0, 0)
	intutil.CreateCumulativeSummary(t, dbc, end, release, test.ID, job.ID, suiteX.ID, 5, 5, 0)

	suiteY := intutil.CreateSuite(t, dbc, "suite-y")
	intutil.CreateCumulativeSummary(t, dbc, boundary, release, test.ID, job.ID, suiteY.ID, 0, 0, 0)
	intutil.CreateCumulativeSummary(t, dbc, end, release, test.ID, job.ID, suiteY.ID, 7, 7, 0)

	results, err := runCollapsedReport(t, dbc, release, sample, base, nil, nil, nil)
	require.NoError(t, err)
	require.Len(t, results, 2, "same test name in two suites should produce two separate rows")

	rowX := findTestRow(t, results, "sig-network shared-test-name", "suite-x")
	assert.Equal(t, 5, rowX.CurrentRuns)
	rowY := findTestRow(t, results, "sig-network shared-test-name", "suite-y")
	assert.Equal(t, 7, rowY.CurrentRuns)
}

func TestTestReportQueryCollapsed_FiltersByName(t *testing.T) {
	dbc := testsReportDB(t)
	release := "4.16"
	sample, base, _, boundary, end := testsReportPeriods()

	vc := intutil.CreateVariantCombination(t, dbc, []string{"Platform:aws"})
	job := intutil.CreateProwJobWithOptions(t, dbc, "periodic-e2e-aws", release, nil, intutil.WithVariantCombination(vc))
	suite := intutil.CreateSuite(t, dbc, "openshift-tests")

	testNetwork := intutil.CreateTest(t, dbc, "sig-network aws connectivity test")
	intutil.CreateCumulativeSummary(t, dbc, boundary, release, testNetwork.ID, job.ID, suite.ID, 0, 0, 0)
	intutil.CreateCumulativeSummary(t, dbc, end, release, testNetwork.ID, job.ID, suite.ID, 5, 5, 0)

	testStorage := intutil.CreateTest(t, dbc, "sig-storage gcp volume test")
	intutil.CreateCumulativeSummary(t, dbc, boundary, release, testStorage.ID, job.ID, suite.ID, 0, 0, 0)
	intutil.CreateCumulativeSummary(t, dbc, end, release, testStorage.ID, job.ID, suite.ID, 6, 6, 0)

	t.Run("equals matches exact name only", func(t *testing.T) {
		nameFilter := &filter.Filter{Items: []filter.FilterItem{
			{Field: "name", Operator: filter.OperatorEquals, Value: "sig-network aws connectivity test"},
		}}
		results, err := runCollapsedReport(t, dbc, release, sample, base, nil, nameFilter, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "sig-network aws connectivity test", results[0].Name)
	})

	t.Run("starts with matches prefix", func(t *testing.T) {
		nameFilter := &filter.Filter{Items: []filter.FilterItem{
			{Field: "name", Operator: filter.OperatorStartsWith, Value: "sig-network"},
		}}
		results, err := runCollapsedReport(t, dbc, release, sample, base, nil, nameFilter, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "sig-network aws connectivity test", results[0].Name)
	})

	t.Run("contains matches substring case-insensitively", func(t *testing.T) {
		nameFilter := &filter.Filter{Items: []filter.FilterItem{
			{Field: "name", Operator: filter.OperatorContains, Value: "VOLUME"},
		}}
		results, err := runCollapsedReport(t, dbc, release, sample, base, nil, nameFilter, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "sig-storage gcp volume test", results[0].Name)
	})

	t.Run("negated contains excludes matches", func(t *testing.T) {
		nameFilter := &filter.Filter{Items: []filter.FilterItem{
			{Field: "name", Operator: filter.OperatorContains, Value: "storage", Not: true},
		}}
		results, err := runCollapsedReport(t, dbc, release, sample, base, nil, nameFilter, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "sig-network aws connectivity test", results[0].Name)
	})
}

func TestTestReportQueryCollapsed_FiltersByVariant(t *testing.T) {
	dbc := testsReportDB(t)
	release := "4.16"
	sample, base, _, boundary, end := testsReportPeriods()

	test := intutil.CreateTest(t, dbc, "sig-network variant-filter-test")
	suite := intutil.CreateSuite(t, dbc, "openshift-tests")

	vcAWS := intutil.CreateVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
	jobAWS := intutil.CreateProwJobWithOptions(t, dbc, "periodic-e2e-aws-ovn", release, nil, intutil.WithVariantCombination(vcAWS))
	intutil.CreateCumulativeSummary(t, dbc, boundary, release, test.ID, jobAWS.ID, suite.ID, 0, 0, 0)
	intutil.CreateCumulativeSummary(t, dbc, end, release, test.ID, jobAWS.ID, suite.ID, 4, 4, 0)

	vcGCP := intutil.CreateVariantCombination(t, dbc, []string{"Platform:gcp", "Network:ovn"})
	jobGCP := intutil.CreateProwJobWithOptions(t, dbc, "periodic-e2e-gcp-ovn", release, nil, intutil.WithVariantCombination(vcGCP))
	intutil.CreateCumulativeSummary(t, dbc, boundary, release, test.ID, jobGCP.ID, suite.ID, 0, 0, 0)
	intutil.CreateCumulativeSummary(t, dbc, end, release, test.ID, jobGCP.ID, suite.ID, 6, 6, 0)

	t.Run("has entry narrows to matching variant combination", func(t *testing.T) {
		variantFilter := &filter.Filter{Items: []filter.FilterItem{
			{Field: "variants", Operator: filter.OperatorHasEntry, Value: "Platform:aws"},
		}}
		results, err := runCollapsedReport(t, dbc, release, sample, base, variantFilter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, 4, results[0].CurrentRuns)
	})

	t.Run("has entry containing matches substring within a variant", func(t *testing.T) {
		variantFilter := &filter.Filter{Items: []filter.FilterItem{
			{Field: "variants", Operator: filter.OperatorHasEntryContaining, Value: "gcp"},
		}}
		results, err := runCollapsedReport(t, dbc, release, sample, base, variantFilter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, 6, results[0].CurrentRuns)
	})

	t.Run("negated has entry excludes matching variant combination", func(t *testing.T) {
		variantFilter := &filter.Filter{Items: []filter.FilterItem{
			{Field: "variants", Operator: filter.OperatorHasEntry, Value: "Platform:aws", Not: true},
		}}
		results, err := runCollapsedReport(t, dbc, release, sample, base, variantFilter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, 6, results[0].CurrentRuns, "should only include the gcp variant combination")
	})
}

func TestTestReportQueryCollapsed_LifecycleFilter(t *testing.T) {
	dbc := testsReportDB(t)
	release := "4.16"
	sample, base, _, boundary, end := testsReportPeriods()

	vc := intutil.CreateVariantCombination(t, dbc, []string{"Platform:aws"})
	job := intutil.CreateProwJobWithOptions(t, dbc, "periodic-e2e-aws", release, nil, intutil.WithVariantCombination(vc))
	test := intutil.CreateTest(t, dbc, "sig-network lifecycle-test")
	suite := intutil.CreateSuite(t, dbc, "openshift-tests")

	// blocking: current = 15-5 = 10
	intutil.CreateCumulativeSummary(t, dbc, boundary, release, test.ID, job.ID, suite.ID, 5, 5, 0, intutil.WithCumulativeSummaryLifecycle("blocking"))
	intutil.CreateCumulativeSummary(t, dbc, end, release, test.ID, job.ID, suite.ID, 15, 15, 0, intutil.WithCumulativeSummaryLifecycle("blocking"))
	// informing: current = 8-2 = 6
	intutil.CreateCumulativeSummary(t, dbc, boundary, release, test.ID, job.ID, suite.ID, 2, 2, 0, intutil.WithCumulativeSummaryLifecycle("informing"))
	intutil.CreateCumulativeSummary(t, dbc, end, release, test.ID, job.ID, suite.ID, 8, 8, 0, intutil.WithCumulativeSummaryLifecycle("informing"))

	t.Run("no filter combines blocking and informing", func(t *testing.T) {
		results, err := runCollapsedReport(t, dbc, release, sample, base, nil, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, 16, results[0].CurrentRuns, "10 blocking + 6 informing")
	})

	t.Run("equals blocking narrows to blocking only", func(t *testing.T) {
		lifecycleFilter := &filter.Filter{Items: []filter.FilterItem{
			{Field: "lifecycle", Operator: filter.OperatorEquals, Value: "blocking"},
		}}
		results, err := runCollapsedReport(t, dbc, release, sample, base, nil, nil, lifecycleFilter)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, 10, results[0].CurrentRuns)
	})

	t.Run("equals informing narrows to informing only", func(t *testing.T) {
		lifecycleFilter := &filter.Filter{Items: []filter.FilterItem{
			{Field: "lifecycle", Operator: filter.OperatorEquals, Value: "informing"},
		}}
		results, err := runCollapsedReport(t, dbc, release, sample, base, nil, nil, lifecycleFilter)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, 6, results[0].CurrentRuns)
	})

	t.Run("not-equals blocking returns only informing", func(t *testing.T) {
		lifecycleFilter := &filter.Filter{Items: []filter.FilterItem{
			{Field: "lifecycle", Operator: filter.OperatorArithmeticNotEquals, Value: "blocking"},
		}}
		results, err := runCollapsedReport(t, dbc, release, sample, base, nil, nil, lifecycleFilter)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, 6, results[0].CurrentRuns)
	})

	t.Run("negated equals blocking is equivalent to not-equals", func(t *testing.T) {
		lifecycleFilter := &filter.Filter{Items: []filter.FilterItem{
			{Field: "lifecycle", Operator: filter.OperatorEquals, Value: "blocking", Not: true},
		}}
		results, err := runCollapsedReport(t, dbc, release, sample, base, nil, nil, lifecycleFilter)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, 6, results[0].CurrentRuns)
	})

	// Regression test: selecting both lifecycles via two OR-linked "equals" items must
	// produce their union (16), not an unsatisfiable "lifecycle = 'blocking' AND
	// lifecycle = 'informing'" that silently returns zero rows.
	t.Run("OR-linked equals blocking or informing returns the union", func(t *testing.T) {
		lifecycleFilter := &filter.Filter{
			LinkOperator: filter.LinkOperatorOr,
			Items: []filter.FilterItem{
				{Field: "lifecycle", Operator: filter.OperatorEquals, Value: "blocking"},
				{Field: "lifecycle", Operator: filter.OperatorEquals, Value: "informing"},
			},
		}
		results, err := runCollapsedReport(t, dbc, release, sample, base, nil, nil, lifecycleFilter)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, 16, results[0].CurrentRuns, "10 blocking + 6 informing")
	})
}

func TestTestReportQueryCollapsed_LifecycleFilterUnsupportedOperatorReturnsError(t *testing.T) {
	dbc := testsReportDB(t)
	release := "4.16"
	sample, base, _, _, _ := testsReportPeriods()

	lifecycleFilter := &filter.Filter{Items: []filter.FilterItem{
		{Field: "lifecycle", Operator: filter.OperatorContains, Value: "block"},
	}}
	_, err := runCollapsedReport(t, dbc, release, sample, base, nil, nil, lifecycleFilter)
	require.Error(t, err)
	assert.ErrorIs(t, err, filter.ErrUnsupportedOperator)
}

// TestTestReportQueryCollapsed_NeverStableVariantRequiresExplicitFilter documents an
// intentional asymmetry with UncollapsedTestReportWithStats: unlike the uncollapsed path
// (which always excludes never-stable variant combinations, hardcoded into the query),
// the collapsed path applies no such exclusion on its own -- it relies entirely on the
// caller supplying a variant filter for it, exactly as the frontend does by default via
// DEFAULT_TEST_FILTERS (sippy-ng/src/constants.jsx) applied from the sidebar "Tests" link
// (sippy-ng/src/components/Sidebar.jsx).
func TestTestReportQueryCollapsed_NeverStableVariantRequiresExplicitFilter(t *testing.T) {
	dbc := testsReportDB(t)
	release := "4.16"
	sample, base, _, boundary, end := testsReportPeriods()

	vc := intutil.CreateVariantCombination(t, dbc, []string{"Platform:aws", "never-stable"})
	job := intutil.CreateProwJobWithOptions(t, dbc, "periodic-e2e-aws-never-stable", release, nil, intutil.WithVariantCombination(vc))
	test := intutil.CreateTest(t, dbc, "sig-network never-stable-test")
	suite := intutil.CreateSuite(t, dbc, "openshift-tests")

	intutil.CreateCumulativeSummary(t, dbc, boundary, release, test.ID, job.ID, suite.ID, 0, 0, 0)
	intutil.CreateCumulativeSummary(t, dbc, end, release, test.ID, job.ID, suite.ID, 10, 10, 0)

	t.Run("without a variant filter, never-stable data is included", func(t *testing.T) {
		results, err := runCollapsedReport(t, dbc, release, sample, base, nil, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, 10, results[0].CurrentRuns)
	})

	t.Run("excluding never-stable via a variant filter narrows it out, as the frontend does by default", func(t *testing.T) {
		variantFilter := &filter.Filter{Items: []filter.FilterItem{
			{Field: "variants", Operator: filter.OperatorHasEntry, Value: "never-stable", Not: true},
		}}
		results, err := runCollapsedReport(t, dbc, release, sample, base, variantFilter, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, results)
	})
}

func TestTestReportQueryCollapsed_ExcludesJobsWithNilVariantCombinationID(t *testing.T) {
	dbc := testsReportDB(t)
	release := "4.16"
	sample, base, _, boundary, end := testsReportPeriods()

	// A plain ProwJob has no VariantCombinationID set.
	job := intutil.CreateProwJob(t, dbc, "periodic-e2e-unclassified", release, nil)
	test := intutil.CreateTest(t, dbc, "sig-network unclassified-job-test")
	suite := intutil.CreateSuite(t, dbc, "openshift-tests")

	intutil.CreateCumulativeSummary(t, dbc, boundary, release, test.ID, job.ID, suite.ID, 0, 0, 0)
	intutil.CreateCumulativeSummary(t, dbc, end, release, test.ID, job.ID, suite.ID, 10, 10, 0)

	results, err := runCollapsedReport(t, dbc, release, sample, base, nil, nil, nil)
	require.NoError(t, err)
	assert.Empty(t, results, "a job with no variant_combination_id should not contribute to the report")
}

func TestTestReportQueryCollapsed_OpenBugsCountsDistinctOpenExcludesClosedCaseInsensitive(t *testing.T) {
	dbc := testsReportDB(t)
	release := "4.16"
	sample, base, _, boundary, end := testsReportPeriods()

	vc := intutil.CreateVariantCombination(t, dbc, []string{"Platform:aws"})
	job := intutil.CreateProwJobWithOptions(t, dbc, "periodic-e2e-aws", release, nil, intutil.WithVariantCombination(vc))
	test := intutil.CreateTest(t, dbc, "sig-network bugged-test")
	suite := intutil.CreateSuite(t, dbc, "openshift-tests")
	intutil.CreateCumulativeSummary(t, dbc, boundary, release, test.ID, job.ID, suite.ID, 0, 0, 0)
	intutil.CreateCumulativeSummary(t, dbc, end, release, test.ID, job.ID, suite.ID, 5, 5, 0)

	lastChangeTime := time.Date(2024, 6, 10, 12, 0, 0, 0, time.UTC)
	intutil.CreateBugForTests(t, dbc, "BUG-1", "NEW", "still open", lastChangeTime, []models.Test{test})
	intutil.CreateBugForTests(t, dbc, "BUG-2", "POST", "also open", lastChangeTime, []models.Test{test})
	intutil.CreateBugForTests(t, dbc, "BUG-3", "Closed", "should be excluded", lastChangeTime, []models.Test{test})

	results, err := runCollapsedReport(t, dbc, release, sample, base, nil, nil, nil)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, 2, results[0].OpenBugs, "only the two non-closed bugs should count, case-insensitively")
}

func TestTestReportQueryCollapsed_ResolvesJiraComponentViaTestOwnershipAndSuite(t *testing.T) {
	dbc := testsReportDB(t)
	release := "4.16"
	sample, base, _, boundary, end := testsReportPeriods()

	vc := intutil.CreateVariantCombination(t, dbc, []string{"Platform:aws"})
	job := intutil.CreateProwJobWithOptions(t, dbc, "periodic-e2e-aws", release, nil, intutil.WithVariantCombination(vc))
	test := intutil.CreateTest(t, dbc, "sig-network owned-test")
	suite := intutil.CreateSuite(t, dbc, "openshift-tests")
	intutil.CreateCumulativeSummary(t, dbc, boundary, release, test.ID, job.ID, suite.ID, 0, 0, 0)
	intutil.CreateCumulativeSummary(t, dbc, end, release, test.ID, job.ID, suite.ID, 5, 5, 0)

	jc := intutil.CreateJiraComponent(t, dbc, "Networking")
	intutil.CreateTestOwnership(t, dbc, test.ID, &suite.ID, "sig-network owned-test", "Networking-team", intutil.WithTestOwnershipJiraComponent(jc.Name))

	results, err := runCollapsedReport(t, dbc, release, sample, base, nil, nil, nil)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "openshift-tests", results[0].SuiteName)
	assert.Equal(t, "Networking", results[0].JiraComponent)
	assert.Equal(t, int(jc.ID), results[0].JiraComponentID) //nolint:gosec
}

func TestTestReportQueryCollapsed_NewTestWithNoPriorHistoryReportsZeroPrevious(t *testing.T) {
	dbc := testsReportDB(t)
	release := "4.16"
	sample, base, _, _, end := testsReportPeriods()

	vc := intutil.CreateVariantCombination(t, dbc, []string{"Platform:aws"})
	job := intutil.CreateProwJobWithOptions(t, dbc, "periodic-e2e-aws", release, nil, intutil.WithVariantCombination(vc))
	test := intutil.CreateTest(t, dbc, "sig-network brand-new-test")
	suite := intutil.CreateSuite(t, dbc, "openshift-tests")

	// Only a row at "end" -- no row at "start" or "boundary" at all, simulating a test
	// that started running partway through the current period.
	intutil.CreateCumulativeSummary(t, dbc, end, release, test.ID, job.ID, suite.ID, 6, 5, 0, intutil.WithCumulativeSummaryFailures(1))

	results, err := runCollapsedReport(t, dbc, release, sample, base, nil, nil, nil)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, 6, results[0].CurrentRuns, "current runs should be the full cumulative total since the missing boundary row is treated as a zero baseline")
	assert.Equal(t, 0, results[0].PreviousRuns, "previous runs should be zero when there's no earlier data at all")
}

// --- UncollapsedTestReportWithStats ---

func TestUncollapsedTestReportWithStats_OneRowPerVariantCombination(t *testing.T) {
	dbc := testsReportDB(t)
	release := "4.16"
	sample, base, _, boundary, end := testsReportPeriods()

	test := intutil.CreateTest(t, dbc, "sig-storage per-variant-test")
	suite := intutil.CreateSuite(t, dbc, "openshift-tests")

	vcAWS := intutil.CreateVariantCombination(t, dbc, []string{"Platform:aws"})
	jobAWS := intutil.CreateProwJobWithOptions(t, dbc, "periodic-e2e-aws", release, nil, intutil.WithVariantCombination(vcAWS))
	intutil.CreateCumulativeSummary(t, dbc, boundary, release, test.ID, jobAWS.ID, suite.ID, 0, 0, 0)
	intutil.CreateCumulativeSummary(t, dbc, end, release, test.ID, jobAWS.ID, suite.ID, 4, 4, 0)

	vcGCP := intutil.CreateVariantCombination(t, dbc, []string{"Platform:gcp"})
	jobGCP := intutil.CreateProwJobWithOptions(t, dbc, "periodic-e2e-gcp", release, nil, intutil.WithVariantCombination(vcGCP))
	intutil.CreateCumulativeSummary(t, dbc, boundary, release, test.ID, jobGCP.ID, suite.ID, 0, 0, 0)
	intutil.CreateCumulativeSummary(t, dbc, end, release, test.ID, jobGCP.ID, suite.ID, 6, 6, 0)

	results, _, err := runUncollapsedReport(t, dbc, release, sample, base, nil, nil, nil, nil)
	require.NoError(t, err)
	require.Len(t, results, 2, "uncollapsed report should keep each variant combination as its own row")

	rowAWS := findVariantRow(t, results, "sig-storage per-variant-test", []string{"Platform:aws"})
	assert.Equal(t, 4, rowAWS.CurrentRuns)
	rowGCP := findVariantRow(t, results, "sig-storage per-variant-test", []string{"Platform:gcp"})
	assert.Equal(t, 6, rowGCP.CurrentRuns)
}

func TestUncollapsedTestReportWithStats_ComputesCrossVariantStatsAndDeltas(t *testing.T) {
	dbc := testsReportDB(t)
	release := "4.16"
	sample, base, _, boundary, end := testsReportPeriods()

	test := intutil.CreateTest(t, dbc, "sig-network cross-variant-stats-test")
	suite := intutil.CreateSuite(t, dbc, "openshift-tests")

	// Three variant combinations at 100%, 80%, and 60% current pass rates.
	// working average = (100+80+60)/3 = 80.
	type combo struct {
		platform         string
		runs, successes  int64
		wantWorkingDelta float64
		wantPassingAvg   float64
	}
	combos := []combo{
		{"aws", 10, 10, 20, 80},
		{"gcp", 10, 8, 0, 80},
		{"azure", 10, 6, -20, 80},
	}
	for _, c := range combos {
		vc := intutil.CreateVariantCombination(t, dbc, []string{"Platform:" + c.platform})
		job := intutil.CreateProwJobWithOptions(t, dbc, "periodic-e2e-"+c.platform, release, nil, intutil.WithVariantCombination(vc))
		intutil.CreateCumulativeSummary(t, dbc, boundary, release, test.ID, job.ID, suite.ID, 0, 0, 0)
		intutil.CreateCumulativeSummary(t, dbc, end, release, test.ID, job.ID, suite.ID, c.runs, c.successes, 0)
	}

	results, _, err := runUncollapsedReport(t, dbc, release, sample, base, nil, nil, nil, nil)
	require.NoError(t, err)
	require.Len(t, results, 3)

	for _, c := range combos {
		row := findVariantRow(t, results, "sig-network cross-variant-stats-test", []string{"Platform:" + c.platform})
		assert.InDelta(t, 80.0, row.WorkingAverage, 0.5, "platform %s working average", c.platform)
		assert.InDelta(t, c.wantWorkingDelta, row.DeltaFromWorkingAverage, 0.5, "platform %s delta from working average", c.platform)
		assert.InDelta(t, c.wantPassingAvg, row.PassingAverage, 0.5, "platform %s passing average", c.platform)
	}
}

// TestUncollapsedTestReportWithStats_LifecycleFilterAppliesToStatsToo is a regression
// test: the stats CTE scopes its input by test_id membership in the filtered/post_filtered
// CTE, not by lifecycle directly, so it must re-apply the lifecycle filter itself or it
// will silently blend in data from lifecycles the caller asked to exclude.
func TestUncollapsedTestReportWithStats_LifecycleFilterAppliesToStatsToo(t *testing.T) {
	dbc := testsReportDB(t)
	release := "4.16"
	sample, base, _, _, end := testsReportPeriods()

	test := intutil.CreateTest(t, dbc, "sig-network lifecycle-stats-test")
	suite := intutil.CreateSuite(t, dbc, "openshift-tests")

	// VC-A: blocking 90%, informing 10%. VC-B: blocking 50%, informing 10%.
	// If lifecycle correctly scopes the stats calculation, filtering to blocking alone
	// should yield a cross-variant average of (90+50)/2 = 70, giving VC-A a delta of
	// +20 and VC-B a delta of -20. If the informing rows leak into the stats average
	// (the bug this test guards against), the blended rows would each show a combined
	// (blocking+informing) rate of 50%/30%, averaging to 40 -- giving deltas of +50/+10
	// instead, even though the filtered rows themselves correctly show 90%/50%.
	vcA := intutil.CreateVariantCombination(t, dbc, []string{"Platform:aws"})
	jobA := intutil.CreateProwJobWithOptions(t, dbc, "periodic-e2e-aws", release, nil, intutil.WithVariantCombination(vcA))
	intutil.CreateCumulativeSummary(t, dbc, end, release, test.ID, jobA.ID, suite.ID, 10, 9, 0, intutil.WithCumulativeSummaryLifecycle("blocking"), intutil.WithCumulativeSummaryFailures(1))
	intutil.CreateCumulativeSummary(t, dbc, end, release, test.ID, jobA.ID, suite.ID, 10, 1, 0, intutil.WithCumulativeSummaryLifecycle("informing"), intutil.WithCumulativeSummaryFailures(9))

	vcB := intutil.CreateVariantCombination(t, dbc, []string{"Platform:gcp"})
	jobB := intutil.CreateProwJobWithOptions(t, dbc, "periodic-e2e-gcp", release, nil, intutil.WithVariantCombination(vcB))
	intutil.CreateCumulativeSummary(t, dbc, end, release, test.ID, jobB.ID, suite.ID, 10, 5, 0, intutil.WithCumulativeSummaryLifecycle("blocking"), intutil.WithCumulativeSummaryFailures(5))
	intutil.CreateCumulativeSummary(t, dbc, end, release, test.ID, jobB.ID, suite.ID, 10, 1, 0, intutil.WithCumulativeSummaryLifecycle("informing"), intutil.WithCumulativeSummaryFailures(9))

	lifecycleFilter := &filter.Filter{Items: []filter.FilterItem{
		{Field: "lifecycle", Operator: filter.OperatorEquals, Value: "blocking"},
	}}
	results, _, err := runUncollapsedReport(t, dbc, release, sample, base, nil, nil, nil, lifecycleFilter)
	require.NoError(t, err)
	require.Len(t, results, 2)

	rowA := findVariantRow(t, results, "sig-network lifecycle-stats-test", []string{"Platform:aws"})
	assert.Equal(t, 10, rowA.CurrentRuns, "filtered rows themselves should only reflect blocking data")
	assert.InDelta(t, 90.0, rowA.CurrentPassPercentage, 0.01)
	assert.InDelta(t, 70.0, rowA.WorkingAverage, 0.5, "stats average must be computed from blocking-only data (90+50)/2=70, not the blended (50+30)/2=40")
	assert.InDelta(t, 20.0, rowA.DeltaFromWorkingAverage, 0.5)

	rowB := findVariantRow(t, results, "sig-network lifecycle-stats-test", []string{"Platform:gcp"})
	assert.Equal(t, 10, rowB.CurrentRuns)
	assert.InDelta(t, 50.0, rowB.CurrentPassPercentage, 0.01)
	assert.InDelta(t, 70.0, rowB.WorkingAverage, 0.5)
	assert.InDelta(t, -20.0, rowB.DeltaFromWorkingAverage, 0.5)
}

func TestUncollapsedTestReportWithStats_LifecycleFilterEqualsAndNotEquals(t *testing.T) {
	dbc := testsReportDB(t)
	release := "4.16"
	sample, base, _, boundary, end := testsReportPeriods()

	vc := intutil.CreateVariantCombination(t, dbc, []string{"Platform:aws"})
	job := intutil.CreateProwJobWithOptions(t, dbc, "periodic-e2e-aws", release, nil, intutil.WithVariantCombination(vc))
	test := intutil.CreateTest(t, dbc, "sig-network uncollapsed-lifecycle-test")
	suite := intutil.CreateSuite(t, dbc, "openshift-tests")

	intutil.CreateCumulativeSummary(t, dbc, boundary, release, test.ID, job.ID, suite.ID, 5, 5, 0, intutil.WithCumulativeSummaryLifecycle("blocking"))
	intutil.CreateCumulativeSummary(t, dbc, end, release, test.ID, job.ID, suite.ID, 15, 15, 0, intutil.WithCumulativeSummaryLifecycle("blocking"))
	intutil.CreateCumulativeSummary(t, dbc, boundary, release, test.ID, job.ID, suite.ID, 2, 2, 0, intutil.WithCumulativeSummaryLifecycle("informing"))
	intutil.CreateCumulativeSummary(t, dbc, end, release, test.ID, job.ID, suite.ID, 8, 8, 0, intutil.WithCumulativeSummaryLifecycle("informing"))

	t.Run("equals blocking", func(t *testing.T) {
		lifecycleFilter := &filter.Filter{Items: []filter.FilterItem{{Field: "lifecycle", Operator: filter.OperatorEquals, Value: "blocking"}}}
		results, _, err := runUncollapsedReport(t, dbc, release, sample, base, nil, nil, nil, lifecycleFilter)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, 10, results[0].CurrentRuns)
	})

	t.Run("not-equals blocking", func(t *testing.T) {
		lifecycleFilter := &filter.Filter{Items: []filter.FilterItem{{Field: "lifecycle", Operator: filter.OperatorArithmeticNotEquals, Value: "blocking"}}}
		results, _, err := runUncollapsedReport(t, dbc, release, sample, base, nil, nil, nil, lifecycleFilter)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, 6, results[0].CurrentRuns)
	})

	// Regression test: an OR-linked filter selecting both lifecycles must return their
	// union (16), not an unsatisfiable AND that silently returns zero rows. This also
	// exercises the stats CTE's re-application of the same clause.
	t.Run("OR-linked equals blocking or informing returns the union", func(t *testing.T) {
		lifecycleFilter := &filter.Filter{
			LinkOperator: filter.LinkOperatorOr,
			Items: []filter.FilterItem{
				{Field: "lifecycle", Operator: filter.OperatorEquals, Value: "blocking"},
				{Field: "lifecycle", Operator: filter.OperatorEquals, Value: "informing"},
			},
		}
		results, _, err := runUncollapsedReport(t, dbc, release, sample, base, nil, nil, nil, lifecycleFilter)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, 16, results[0].CurrentRuns, "10 blocking + 6 informing")
	})
}

func TestUncollapsedTestReportWithStats_LifecycleFilterUnsupportedOperatorReturnsError(t *testing.T) {
	dbc := testsReportDB(t)
	release := "4.16"
	sample, base, _, _, _ := testsReportPeriods()

	lifecycleFilter := &filter.Filter{Items: []filter.FilterItem{
		{Field: "lifecycle", Operator: filter.OperatorStartsWith, Value: "block"},
	}}
	_, _, err := runUncollapsedReport(t, dbc, release, sample, base, nil, nil, nil, lifecycleFilter)
	require.Error(t, err)
	assert.ErrorIs(t, err, filter.ErrUnsupportedOperator)
}

func TestUncollapsedTestReportWithStats_FiltersByNameAndVariant(t *testing.T) {
	dbc := testsReportDB(t)
	release := "4.16"
	sample, base, _, boundary, end := testsReportPeriods()
	suite := intutil.CreateSuite(t, dbc, "openshift-tests")

	vcAWS := intutil.CreateVariantCombination(t, dbc, []string{"Platform:aws"})
	jobAWS := intutil.CreateProwJobWithOptions(t, dbc, "periodic-e2e-aws", release, nil, intutil.WithVariantCombination(vcAWS))
	vcGCP := intutil.CreateVariantCombination(t, dbc, []string{"Platform:gcp"})
	jobGCP := intutil.CreateProwJobWithOptions(t, dbc, "periodic-e2e-gcp", release, nil, intutil.WithVariantCombination(vcGCP))

	testNetwork := intutil.CreateTest(t, dbc, "sig-network uncollapsed-name-test")
	intutil.CreateCumulativeSummary(t, dbc, boundary, release, testNetwork.ID, jobAWS.ID, suite.ID, 0, 0, 0)
	intutil.CreateCumulativeSummary(t, dbc, end, release, testNetwork.ID, jobAWS.ID, suite.ID, 3, 3, 0)
	intutil.CreateCumulativeSummary(t, dbc, boundary, release, testNetwork.ID, jobGCP.ID, suite.ID, 0, 0, 0)
	intutil.CreateCumulativeSummary(t, dbc, end, release, testNetwork.ID, jobGCP.ID, suite.ID, 4, 4, 0)

	testStorage := intutil.CreateTest(t, dbc, "sig-storage uncollapsed-name-test")
	intutil.CreateCumulativeSummary(t, dbc, boundary, release, testStorage.ID, jobAWS.ID, suite.ID, 0, 0, 0)
	intutil.CreateCumulativeSummary(t, dbc, end, release, testStorage.ID, jobAWS.ID, suite.ID, 5, 5, 0)

	t.Run("name filter narrows across all variant combinations", func(t *testing.T) {
		nameFilter := &filter.Filter{Items: []filter.FilterItem{{Field: "name", Operator: filter.OperatorContains, Value: "sig-network"}}}
		results, _, err := runUncollapsedReport(t, dbc, release, sample, base, nameFilter, nil, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 2, "sig-network test has two variant combination rows")
	})

	t.Run("variant filter narrows to one combination across tests", func(t *testing.T) {
		variantFilter := &filter.Filter{Items: []filter.FilterItem{{Field: "variants", Operator: filter.OperatorHasEntry, Value: "Platform:gcp"}}}
		results, _, err := runUncollapsedReport(t, dbc, release, sample, base, nil, variantFilter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1, "only sig-network runs under the gcp variant combination")
		assert.Equal(t, "sig-network uncollapsed-name-test", results[0].Name)
	})
}

func TestUncollapsedTestReportWithStats_ExcludesNeverStableVariantFromResultsAndStats(t *testing.T) {
	dbc := testsReportDB(t)
	release := "4.16"
	sample, base, _, boundary, end := testsReportPeriods()

	test := intutil.CreateTest(t, dbc, "sig-network mixed-stability-test")
	suite := intutil.CreateSuite(t, dbc, "openshift-tests")

	vcStable := intutil.CreateVariantCombination(t, dbc, []string{"Platform:aws"})
	jobStable := intutil.CreateProwJobWithOptions(t, dbc, "periodic-e2e-aws", release, nil, intutil.WithVariantCombination(vcStable))
	intutil.CreateCumulativeSummary(t, dbc, boundary, release, test.ID, jobStable.ID, suite.ID, 0, 0, 0)
	intutil.CreateCumulativeSummary(t, dbc, end, release, test.ID, jobStable.ID, suite.ID, 10, 9, 0, intutil.WithCumulativeSummaryFailures(1))

	vcNeverStable := intutil.CreateVariantCombination(t, dbc, []string{"Platform:gcp", "never-stable"})
	jobNeverStable := intutil.CreateProwJobWithOptions(t, dbc, "periodic-e2e-gcp-never-stable", release, nil, intutil.WithVariantCombination(vcNeverStable))
	// If this leaked into the stats average, working_average would be pulled toward 10%.
	intutil.CreateCumulativeSummary(t, dbc, boundary, release, test.ID, jobNeverStable.ID, suite.ID, 0, 0, 0)
	intutil.CreateCumulativeSummary(t, dbc, end, release, test.ID, jobNeverStable.ID, suite.ID, 10, 1, 0, intutil.WithCumulativeSummaryFailures(9))

	results, _, err := runUncollapsedReport(t, dbc, release, sample, base, nil, nil, nil, nil)
	require.NoError(t, err)
	require.Len(t, results, 1, "the never-stable variant combination should not appear as a row")

	row := results[0]
	assert.Equal(t, "Platform:aws", row.Variants[0])
	assert.InDelta(t, 90.0, row.WorkingAverage, 0.5, "the never-stable combination's data should not skew the stats average")
	assert.InDelta(t, 0.0, row.DeltaFromWorkingAverage, 0.5)
}

func TestUncollapsedTestReportWithStats_ProcessedFilterPushesDownAndReturnsRemainingFilter(t *testing.T) {
	dbc := testsReportDB(t)
	release := "4.16"
	sample, base, _, boundary, end := testsReportPeriods()

	test := intutil.CreateTest(t, dbc, "sig-network processed-filter-test")
	suite := intutil.CreateSuite(t, dbc, "openshift-tests")

	vcLowRuns := intutil.CreateVariantCombination(t, dbc, []string{"Platform:aws"})
	jobLowRuns := intutil.CreateProwJobWithOptions(t, dbc, "periodic-e2e-aws", release, nil, intutil.WithVariantCombination(vcLowRuns))
	intutil.CreateCumulativeSummary(t, dbc, boundary, release, test.ID, jobLowRuns.ID, suite.ID, 0, 0, 0)
	intutil.CreateCumulativeSummary(t, dbc, end, release, test.ID, jobLowRuns.ID, suite.ID, 3, 3, 0)

	vcHighRuns := intutil.CreateVariantCombination(t, dbc, []string{"Platform:gcp"})
	jobHighRuns := intutil.CreateProwJobWithOptions(t, dbc, "periodic-e2e-gcp", release, nil, intutil.WithVariantCombination(vcHighRuns))
	intutil.CreateCumulativeSummary(t, dbc, boundary, release, test.ID, jobHighRuns.ID, suite.ID, 0, 0, 0)
	intutil.CreateCumulativeSummary(t, dbc, end, release, test.ID, jobHighRuns.ID, suite.ID, 10, 10, 0)

	processedFilter := &filter.Filter{
		LinkOperator: filter.LinkOperatorAnd,
		Items: []filter.FilterItem{
			{Field: "current_runs", Operator: filter.OperatorArithmeticGreaterThanOrEquals, Value: "5"},
			{Field: "suite_name", Operator: filter.OperatorContains, Value: "openshift"},
		},
	}
	results, remaining, err := runUncollapsedReport(t, dbc, release, sample, base, nil, nil, processedFilter, nil)
	require.NoError(t, err)
	require.Len(t, results, 1, "current_runs >= 5 should be pushed down and exclude the low-runs variant")
	assert.Equal(t, "Platform:gcp", results[0].Variants[0])

	require.NotNil(t, remaining, "the suite_name filter isn't pushdown-safe and must be returned for the caller to apply")
	require.Len(t, remaining.Items, 1)
	assert.Equal(t, "suite_name", remaining.Items[0].Field)
}

func TestUncollapsedTestReportWithStats_OpenBugsAndJiraComponent(t *testing.T) {
	dbc := testsReportDB(t)
	release := "4.16"
	sample, base, _, boundary, end := testsReportPeriods()

	vc := intutil.CreateVariantCombination(t, dbc, []string{"Platform:aws"})
	job := intutil.CreateProwJobWithOptions(t, dbc, "periodic-e2e-aws", release, nil, intutil.WithVariantCombination(vc))
	test := intutil.CreateTest(t, dbc, "sig-network uncollapsed-bugged-test")
	suite := intutil.CreateSuite(t, dbc, "openshift-tests")
	intutil.CreateCumulativeSummary(t, dbc, boundary, release, test.ID, job.ID, suite.ID, 0, 0, 0)
	intutil.CreateCumulativeSummary(t, dbc, end, release, test.ID, job.ID, suite.ID, 5, 5, 0)

	jc := intutil.CreateJiraComponent(t, dbc, "Networking")
	intutil.CreateTestOwnership(t, dbc, test.ID, &suite.ID, "sig-network uncollapsed-bugged-test", "Networking-team", intutil.WithTestOwnershipJiraComponent(jc.Name))
	lastChangeTime := time.Date(2024, 6, 10, 12, 0, 0, 0, time.UTC)
	intutil.CreateBugForTests(t, dbc, "BUG-100", "NEW", "still open", lastChangeTime, []models.Test{test})
	intutil.CreateBugForTests(t, dbc, "BUG-101", "CLOSED", "should be excluded", lastChangeTime, []models.Test{test})

	results, _, err := runUncollapsedReport(t, dbc, release, sample, base, nil, nil, nil, nil)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, 1, results[0].OpenBugs)
	assert.Equal(t, "Networking", results[0].JiraComponent)
	assert.Equal(t, int(jc.ID), results[0].JiraComponentID) //nolint:gosec
}

func TestUncollapsedTestReportWithStats_NewVariantWithNoPriorHistoryReportsZeroPrevious(t *testing.T) {
	dbc := testsReportDB(t)
	release := "4.16"
	sample, base, _, _, end := testsReportPeriods()

	vc := intutil.CreateVariantCombination(t, dbc, []string{"Platform:aws"})
	job := intutil.CreateProwJobWithOptions(t, dbc, "periodic-e2e-aws", release, nil, intutil.WithVariantCombination(vc))
	test := intutil.CreateTest(t, dbc, "sig-network uncollapsed-brand-new-test")
	suite := intutil.CreateSuite(t, dbc, "openshift-tests")

	intutil.CreateCumulativeSummary(t, dbc, end, release, test.ID, job.ID, suite.ID, 6, 5, 0, intutil.WithCumulativeSummaryFailures(1))

	results, _, err := runUncollapsedReport(t, dbc, release, sample, base, nil, nil, nil, nil)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, 6, results[0].CurrentRuns)
	assert.Equal(t, 0, results[0].PreviousRuns)
}
