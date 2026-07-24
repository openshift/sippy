package flags

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/openshift/sippy/pkg/api"
	apitype "github.com/openshift/sippy/pkg/apis/api"
	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/db/query"
	"github.com/openshift/sippy/pkg/filter"
	"github.com/openshift/sippy/pkg/sippyserver"
	"github.com/openshift/sippy/pkg/util"
	log "github.com/sirupsen/logrus"
)

const benchmarkRelease = "4.22"
const benchmarkTestName = "[Monitor:legacy-test-framework-invariants-pathological][sig-arch] events should not repeat pathologically for ns/kube-system"
const benchmarkJobName = "periodic-ci-openshift-release-main-ci-4.22-e2e-aws-ovn"

type benchmarkCase struct {
	name string
	fn   func(dbc *db.DB) error
}

type benchmarkResult struct {
	name       string
	iterations int
	total      time.Duration
	avg        time.Duration
	min        time.Duration
	max        time.Duration
}

func extractConnectionName(dsn string) string {
	atIdx := strings.Index(dsn, "@")
	if atIdx < 0 {
		return ""
	}
	host := dsn[atIdx+1:]
	if dotIdx := strings.Index(host, "."); dotIdx > 0 {
		return host[:dotIdx]
	}
	return ""
}

func printSummaryTable(t *testing.T, results []benchmarkResult, connName string) {
	nameWidth := 4
	for _, r := range results {
		if len(r.name) > nameWidth {
			nameWidth = len(r.name)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].avg > results[j].avg
	})

	var sb strings.Builder
	header := fmt.Sprintf("  %-*s  %5s  %12s  %12s  %12s  %12s",
		nameWidth, "Name", "Iters", "Total", "Avg", "Min", "Max")
	sb.WriteString("\n")
	sb.WriteString(header + "\n")
	sb.WriteString("  " + strings.Repeat("-", len(header)-2) + "\n")
	for _, r := range results {
		fmt.Fprintf(&sb, "  %-*s  %5d  %12s  %12s  %12s  %12s\n",
			nameWidth, r.name, r.iterations, r.total, r.avg, r.min, r.max)
	}
	sb.WriteString("\n")
	fmt.Print(sb.String())

	// optional helper to track results
	benchmarkFilePath := os.Getenv("benchmarking_file_path")
	if connName != "" && len(benchmarkFilePath) > 0 {
		ts := time.Now().UTC().Format("2006-01-02T15-04-05")
		filename := fmt.Sprintf("benchmark-%s-%s.txt", connName, ts)
		fullPath := filepath.Join(benchmarkFilePath, filename)
		fullPath = filepath.Clean(fullPath)
		if err := os.WriteFile(fullPath, []byte(sb.String()), 0o600); err != nil { // #nosec G703
			t.Logf("failed to write benchmark report to %s: %v", fullPath, err)
		} else {
			t.Logf("benchmark report written to %s", fullPath)
		}
	}
}

func runBenchmarkCase(t *testing.T, dbc *db.DB, bc benchmarkCase, iterations int) benchmarkResult {
	t.Helper()
	result := benchmarkResult{
		name:       bc.name,
		iterations: iterations,
	}
	for i := 0; i < iterations; i++ {
		start := time.Now()
		err := bc.fn(dbc)
		elapsed := time.Since(start)
		if err != nil {
			t.Fatalf("%s iteration %d failed: %v", bc.name, i+1, err)
		}
		result.total += elapsed
		if i == 0 || elapsed < result.min {
			result.min = elapsed
		}
		if elapsed > result.max {
			result.max = elapsed
		}
		fmt.Printf("  %s iteration %d: %s\n", bc.name, i+1, elapsed)
	}
	result.avg = result.total / time.Duration(iterations)
	return result
}

func getIndividualBenchmarkCases() map[string]benchmarkCase {
	return map[string]benchmarkCase{
		"FindTestsByRelease": {
			name: "FindTestsByRelease",
			fn: func(dbc *db.DB) error {
				type testResult struct {
					ID   uint
					Name string
				}
				var results []testResult
				res := dbc.DB.Raw(`
					SELECT DISTINCT t.id, t.name
					FROM tests t
					JOIN prow_job_run_tests pjrt ON pjrt.test_id = t.id
					WHERE pjrt.prow_job_run_release = ?
					  AND t.name LIKE ?
					  AND pjrt.prow_job_run_timestamp > NOW() - INTERVAL '14 days'
					ORDER BY t.name
					LIMIT 20`, benchmarkRelease, "%events should not repeat%").Scan(&results)
				if res.Error != nil {
					return res.Error
				}
				log.Printf("Found %d tests matching pattern for release %s", len(results), benchmarkRelease)
				for _, r := range results {
					log.Printf("  [%d] %s", r.ID, r.Name)
				}
				return nil
			},
		},
	}
}

func getBenchmarkCases(asOf time.Time) []benchmarkCase {
	return []benchmarkCase{
		{
			name: "TestDurations",
			fn: func(dbc *db.DB) error {
				durations, err := query.TestDurations(dbc, benchmarkRelease,
					benchmarkTestName, nil, nil)

				if err == nil {
					log.Printf("Found %d test durations", len(durations))
				}

				return err
			},
		},
		{
			name: "TestOutputs",
			fn: func(dbc *db.DB) error {
				testOutputs, err := query.TestOutputs(dbc, benchmarkRelease,
					benchmarkTestName, nil, nil, 10)

				if err == nil {
					log.Printf("Found %d test outputs", len(testOutputs))
				}

				return err
			},
		},
		{
			name: "JobDetails",
			fn: func(dbc *db.DB) error {
				jobRuns, err := api.JobDetailsReport(dbc, benchmarkRelease,
					benchmarkJobName, asOf)

				if err == nil {
					log.Printf("Found %d job runs", len(jobRuns))
				}

				return err
			},
		},
		{
			name: "TestAnalysisOverall",
			fn: func(dbc *db.DB) error {
				results, err := api.GetTestAnalysisOverallFromDB(dbc, nil,
					benchmarkRelease, benchmarkTestName, asOf)

				if err == nil {
					for group, rows := range results {
						log.Printf("TestAnalysisOverall group %s: %d rows", group, len(rows))
					}
				}

				return err
			},
		},
		{
			name: "TestAnalysisByJob",
			fn: func(dbc *db.DB) error {
				results, err := api.GetTestAnalysisByJobFromDB(dbc, nil,
					benchmarkRelease, benchmarkTestName, asOf)

				if err == nil {
					log.Printf("TestAnalysisByJob: %d groups", len(results))
				}

				return err
			},
		},
		{
			name: "TestAnalysisByJobWithVariantFilter",
			fn: func(dbc *db.DB) error {
				f := &filter.Filter{
					Items: []filter.FilterItem{
						{Field: "variants", Value: "aws", Not: false},
					},
				}
				results, err := api.GetTestAnalysisByJobFromDB(dbc, f,
					benchmarkRelease, benchmarkTestName, asOf)

				if err == nil {
					log.Printf("TestAnalysisByJobWithVariantFilter: %d groups", len(results))
				}

				return err
			},
		},
		{
			name: "QueryTestAnalysis",
			fn: func(dbc *db.DB) error {
				analyzeSince := asOf.Add(-14 * 24 * time.Hour)
				type testResult struct {
					CurrentSuccesses   int
					CurrentRuns        int
					CurrentPassPercent float64
				}
				var result testResult
				res := dbc.DB.Raw(query.QueryTestAnalysis, analyzeSince, benchmarkTestName, []string{benchmarkJobName}, benchmarkRelease)
				if res.Error != nil {
					return res.Error
				}
				res.Scan(&result)
				log.Printf("QueryTestAnalysis: runs=%d successes=%d", result.CurrentRuns, result.CurrentSuccesses)
				return res.Error
			},
		},
		{
			name: "TestCountsByLookback14",
			fn: func(dbc *db.DB) error {
				jobRuns, testIDs, err := api.GetJobRunTestsCountByLookback(dbc, 14)
				if err == nil {
					log.Printf("TestCountsByLookback14: %d job runs, %d test IDs", jobRuns, testIDs)
				}
				return err
			},
		},
		{
			name: "TestCountsByLookback9",
			fn: func(dbc *db.DB) error {
				jobRuns, testIDs, err := api.GetJobRunTestsCountByLookback(dbc, 9)
				if err == nil {
					log.Printf("TestCountsByLookback9: %d job runs, %d test IDs", jobRuns, testIDs)
				}
				return err
			},
		},
		{
			name: "TestCountsByLookback14ForRelease",
			fn: func(dbc *db.DB) error {
				type counts struct {
					JobRunsCount int64
					TestIDsCount int64
				}
				var result counts
				truncatedTime := time.Now().UTC().AddDate(0, 0, -14).Truncate(24 * time.Hour)
				res := dbc.DB.Raw(`
					SELECT count(distinct pjrt.prow_job_run_id) as job_runs_count,
					       count(distinct pjrt.test_id) as test_ids_count
					FROM prow_job_run_tests pjrt
					WHERE pjrt.prow_job_run_timestamp > ?
					  AND pjrt.prow_job_run_release = ?`, truncatedTime, benchmarkRelease).Scan(&result)
				if res.Error != nil {
					return res.Error
				}
				log.Printf("TestCountsByLookback14ForRelease %s: %d job runs, %d test IDs",
					benchmarkRelease, result.JobRunsCount, result.TestIDsCount)
				return nil
			},
		},
		{
			name: "TestCountsByLookback9ForRelease",
			fn: func(dbc *db.DB) error {
				type counts struct {
					JobRunsCount int64
					TestIDsCount int64
				}
				var result counts
				truncatedTime := time.Now().UTC().AddDate(0, 0, -9).Truncate(24 * time.Hour)
				res := dbc.DB.Raw(`
					SELECT count(distinct pjrt.prow_job_run_id) as job_runs_count,
					       count(distinct pjrt.test_id) as test_ids_count
					FROM prow_job_run_tests pjrt
					WHERE pjrt.prow_job_run_timestamp > ?
					  AND pjrt.prow_job_run_release = ?`, truncatedTime, benchmarkRelease).Scan(&result)
				if res.Error != nil {
					return res.Error
				}
				log.Printf("TestCountsByLookback9ForRelease %s: %d job runs, %d test IDs",
					benchmarkRelease, result.JobRunsCount, result.TestIDsCount)
				return nil
			},
		},
		{
			name: "VariantReports",
			fn: func(dbc *db.DB) error {
				start, boundary, end := util.PeriodToDates("default", asOf)
				results, err := query.VariantReports(dbc, benchmarkRelease, start, boundary, end)
				if err == nil {
					log.Printf("VariantReports: %d variants", len(results))
				}
				return err
			},
		},
		{
			name: "JobReports",
			fn: func(dbc *db.DB) error {
				start, boundary, end := util.PeriodToDates("default", asOf)
				results, err := query.JobReports(dbc, &filter.FilterOptions{Filter: &filter.Filter{}}, benchmarkRelease, start, boundary, end)
				if err == nil {
					log.Printf("JobReports: %d jobs", len(results))
				}
				return err
			},
		},
		{
			name: "BuildClusterHealth",
			fn: func(dbc *db.DB) error {
				start, boundary, end := util.PeriodToDates("default", asOf)
				results, err := query.BuildClusterHealth(dbc, start, boundary, end)
				if err == nil {
					log.Printf("BuildClusterHealth: %d clusters", len(results))
				}
				return err
			},
		},
		{
			name: "RecentTestFailures",
			fn: func(dbc *db.DB) error {
				period := 7 * 24 * time.Hour
				previousPeriod := 7 * 24 * time.Hour
				pagination := &apitype.Pagination{PerPage: 20, Page: 0}
				result, err := api.GetRecentTestFailures(dbc, benchmarkRelease, period, &previousPeriod, false, &filter.FilterOptions{Filter: &filter.Filter{}}, pagination, asOf)
				if err == nil {
					log.Printf("RecentTestFailures: %d rows", result.TotalRows)
				}
				return err
			},
		},
		{
			name: "PullRequestReport",
			fn: func(dbc *db.DB) error {
				results, err := query.PullRequestReport(dbc, &filter.FilterOptions{Filter: &filter.Filter{}}, benchmarkRelease)
				if err == nil {
					log.Printf("PullRequestReport: %d PRs", len(results))
				}
				return err
			},
		},
		{
			name: "RepositoryReport",
			fn: func(dbc *db.DB) error {
				results, err := query.RepositoryReport(dbc, &filter.FilterOptions{Filter: &filter.Filter{}}, benchmarkRelease, asOf)
				if err == nil {
					log.Printf("RepositoryReport: %d repos", len(results))
				}
				return err
			},
		},
		{
			name: "JobsRunsReport",
			fn: func(dbc *db.DB) error {
				pagination := &apitype.Pagination{PerPage: 20, Page: 0}
				result, err := api.JobsRunsReportFromDB(dbc, &filter.FilterOptions{Filter: &filter.Filter{}}, benchmarkRelease, pagination, asOf)
				if err == nil {
					log.Printf("JobsRunsReport: %d rows", result.TotalRows)
				}
				return err
			},
		},
		{
			name: "ProwJobHistoricalTestCounts",
			fn: func(dbc *db.DB) error {
				var prowJob models.ProwJob
				if err := dbc.DB.Where("name = ? AND release = ?", benchmarkJobName, benchmarkRelease).First(&prowJob).Error; err != nil {
					return err
				}
				count, err := query.ProwJobHistoricalTestCounts(dbc, prowJob.ID, benchmarkRelease)
				if err == nil {
					log.Printf("ProwJobHistoricalTestCounts for %s: %d", benchmarkJobName, count)
				}
				return err
			},
		},
		{
			name: "JobRunTestCount",
			fn: func(dbc *db.DB) error {
				var result struct {
					ID        int64
					Timestamp time.Time
				}
				res := dbc.DB.Table("prow_job_runs").
					Joins("JOIN prow_jobs ON prow_jobs.id = prow_job_runs.prow_job_id").
					Where("prow_jobs.name = ? AND prow_jobs.release = ?", benchmarkJobName, benchmarkRelease).
					Order("prow_job_runs.timestamp DESC").
					Limit(1).
					Select("prow_job_runs.id, prow_job_runs.timestamp").
					Scan(&result)
				if res.Error != nil {
					return res.Error
				}
				count, err := query.JobRunTestCount(dbc, result.ID, benchmarkRelease, result.Timestamp)
				if err == nil {
					log.Printf("JobRunTestCount for run %d: %d tests", result.ID, count)
				}
				return err
			},
		},
		{
			name: "IsNewTestQuery",
			fn: func(dbc *db.DB) error {
				var testID uint
				res := dbc.DB.Table("tests").
					Where("name = ?", benchmarkTestName).
					Select("id").
					Scan(&testID)
				if res.Error != nil {
					return res.Error
				}
				var result struct {
					Org      string
					Repo     string
					Number   int
					SHA      string
					MergedAt *time.Time
				}
				res = dbc.DB.
					Table("prow_job_run_tests as t").
					Joins("INNER JOIN prow_job_run_prow_pull_requests as prmap on prmap.prow_job_run_id = t.prow_job_run_id").
					Joins("INNER JOIN prow_pull_requests as prs on prs.id = prmap.prow_pull_request_id").
					Where("t.test_id = ?", testID).
					Where("t.prow_job_run_release = ?", benchmarkRelease).
					Where("merged_at is not null").
					Select("org, repo, number, sha, merged_at").
					Limit(1).Scan(&result)
				if res.Error != nil {
					return res.Error
				}
				log.Printf("IsNewTestQuery for test %d: found=%v", testID, result.MergedAt != nil)
				return nil
			},
		},
		{
			name: "TestAnalysisPassRate",
			fn: func(dbc *db.DB) error {
				type passRate struct {
					CurrentSuccesses   int
					CurrentRuns        int
					CurrentPassPercent float64
				}
				var result passRate
				res := dbc.DB.Raw(query.QueryTestAnalysis,
					time.Now().Add(-24*14*time.Hour),
					benchmarkTestName,
					[]string{benchmarkJobName},
					benchmarkRelease).Scan(&result)
				if res.Error != nil {
					return res.Error
				}
				log.Printf("TestAnalysisPassRate: %d/%d runs (%.1f%%)",
					result.CurrentSuccesses, result.CurrentRuns, result.CurrentPassPercent)
				return nil
			},
		},
	}
}

func getBenchmarkDBClient(t *testing.T) (*db.DB, string) {
	t.Helper()
	dsn := os.Getenv("db_benchmarking_dsn")
	if dsn == "" {
		t.Skip("skipping: set db_benchmarking_dsn to run")
	}

	dbFlags := &PostgresFlags{
		LogLevel:            4,
		DSN:                 dsn,
		EnablePartitionwise: os.Getenv("enable_partitionwise") != "",
	}

	dbc, err := dbFlags.GetDBClient()
	if err != nil {
		t.Fatal("couldn't get DB client")
	}
	sqlDB, err := dbc.DB.DB()
	if err != nil {
		t.Fatal("couldn't get sql.DB handle")
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Logf("failed to close DB client: %v", err)
		}
	})

	if dbFlags.EnablePartitionwise {
		t.Log("enabled partitionwise aggregate and join")
	}

	return dbc, extractConnectionName(dsn)
}

func Test_BenchmarkIndividual(t *testing.T) {
	dbc, connName := getBenchmarkDBClient(t)
	asOf := time.Now().UTC()
	iterations := 3
	cases := getBenchmarkCases(asOf)

	var results []benchmarkResult
	for _, bc := range cases {
		t.Run(bc.name, func(t *testing.T) {
			r := runBenchmarkCase(t, dbc, bc, iterations)
			results = append(results, r)
		})
	}
	printSummaryTable(t, results, connName)
}

func Test_BenchmarkFindTestsByRelease(t *testing.T) {
	dbc, connName := getBenchmarkDBClient(t)
	iterations := 1
	bc, ok := getIndividualBenchmarkCases()["FindTestsByRelease"]
	if !ok {
		t.Fatal("benchmark case \"FindTestsByRelease\" not found")
	}

	r := runBenchmarkCase(t, dbc, bc, iterations)
	printSummaryTable(t, []benchmarkResult{r}, connName)
}

func Test_BenchmarkCombined(t *testing.T) {
	dbc, connName := getBenchmarkDBClient(t)
	asOf := time.Now().UTC()
	iterations := 3

	var results []benchmarkResult
	for _, bc := range getBenchmarkCases(asOf) {
		t.Run(bc.name, func(t *testing.T) {
			r := runBenchmarkCase(t, dbc, bc, iterations)
			results = append(results, r)
		})
	}
	for name, bc := range getIndividualBenchmarkCases() {
		t.Run(name, func(t *testing.T) {
			r := runBenchmarkCase(t, dbc, bc, iterations)
			results = append(results, r)
		})
	}
	printSummaryTable(t, results, connName)
}

func Test_BenchmarkGroup(t *testing.T) {
	dbc, connName := getBenchmarkDBClient(t)
	asOf := time.Now().UTC()
	iterations := 1
	cases := getBenchmarkCases(asOf)

	group := benchmarkResult{name: "group"}
	for i := 0; i < iterations; i++ {
		start := time.Now()
		for _, bc := range cases {
			err := bc.fn(dbc)
			if err != nil {
				t.Fatalf("group iteration %d, case %s failed: %v", i+1, bc.name, err)
			}
		}
		elapsed := time.Since(start)
		group.total += elapsed
		group.iterations++
		if i == 0 || elapsed < group.min {
			group.min = elapsed
		}
		if elapsed > group.max {
			group.max = elapsed
		}
		fmt.Printf("  group iteration %d: %s\n", i+1, elapsed)
	}
	group.avg = group.total / time.Duration(iterations)
	printSummaryTable(t, []benchmarkResult{group}, connName)
}

func Test_CompareTestOutputsQueries(t *testing.T) {
	dbc, _ := getBenchmarkDBClient(t)
	quantity := 50

	testQuery := dbc.DB.Table("tests").Where("name = ?", benchmarkTestName).Select("id")

	var baseline []apitype.TestOutput
	baseStart := time.Now()
	res := dbc.DB.Table("prow_job_run_test_outputs").
		Joins("JOIN prow_job_run_tests ON prow_job_run_test_outputs.prow_job_run_test_id = prow_job_run_tests.id").
		Joins("JOIN prow_job_runs ON prow_job_run_tests.prow_job_run_id = prow_job_runs.id").
		Joins("JOIN prow_jobs ON prow_job_runs.prow_job_id = prow_jobs.id").
		Where("prow_job_runs.timestamp > current_date - interval '14' day").
		Where("prow_job_run_tests.test_id = (?)", testQuery).
		Where("prow_jobs.release = ?", benchmarkRelease).
		Select("prow_job_runs.url as prow_job_url, output").
		Order("prow_job_run_test_outputs.id DESC").
		Limit(quantity).
		Scan(&baseline)
	if res.Error != nil {
		t.Fatalf("baseline query failed: %v", res.Error)
	}
	baseElapsed := time.Since(baseStart)

	var current []apitype.TestOutput
	curStart := time.Now()
	currentOutputs, err := query.TestOutputs(dbc, benchmarkRelease, benchmarkTestName, nil, nil, quantity)
	if err != nil {
		t.Fatalf("current query failed: %v", err)
	}
	current = currentOutputs
	curElapsed := time.Since(curStart)

	t.Logf("baseline: %d results in %s", len(baseline), baseElapsed)
	t.Logf("current:  %d results in %s", len(current), curElapsed)

	baselineByURL := make(map[string]string, len(baseline))
	for _, r := range baseline {
		baselineByURL[r.ProwJobURL] = r.Output
	}
	currentByURL := make(map[string]string, len(current))
	for _, r := range current {
		currentByURL[r.ProwJobURL] = r.Output
	}

	missingFromCurrent := 0
	missingFromBaseline := 0
	outputMismatch := 0

	for url, output := range baselineByURL {
		curOutput, ok := currentByURL[url]
		if !ok {
			missingFromCurrent++
			t.Logf("MISSING from current: %s", url)
			continue
		}
		if curOutput != output {
			outputMismatch++
			t.Logf("OUTPUT MISMATCH for %s", url)
		}
	}
	for url := range currentByURL {
		if _, ok := baselineByURL[url]; !ok {
			missingFromBaseline++
			t.Logf("EXTRA in current (not in baseline): %s", url)
		}
	}

	t.Logf("comparison: baseline=%d current=%d missing_from_current=%d extra_in_current=%d output_mismatch=%d",
		len(baseline), len(current), missingFromCurrent, missingFromBaseline, outputMismatch)

	if missingFromCurrent > 0 || outputMismatch > 0 {
		t.Errorf("query results differ: %d missing, %d mismatched", missingFromCurrent, outputMismatch)
	}
}

func getMatviewBenchmarkCases(asOf time.Time) []benchmarkCase {
	return []benchmarkCase{
		{
			name: "MatviewTestReport7d",
			fn: func(dbc *db.DB) error {
				results, err := query.TestReportsByVariant(dbc, benchmarkRelease,
					v1.CurrentReport, query.TestNameMatches{Substrings: []string{benchmarkTestName}}, nil, false)
				if err == nil {
					log.Printf("MatviewTestReport7d: %d results", len(results))
				}
				return err
			},
		},
		{
			name: "MatviewTestReport2d",
			fn: func(dbc *db.DB) error {
				results, err := query.TestReportsByVariant(dbc, benchmarkRelease,
					v1.TwoDayReport, query.TestNameMatches{Substrings: []string{benchmarkTestName}}, nil, false)
				if err == nil {
					log.Printf("MatviewTestReport2d: %d results", len(results))
				}
				return err
			},
		},
		{
			name: "MatviewTestReportExcludeVariants",
			fn: func(dbc *db.DB) error {
				_, found := query.TestReportExcludeVariants(dbc, benchmarkRelease,
					benchmarkTestName, []string{"never-stable"})
				log.Printf("MatviewTestReportExcludeVariants: found=%v", found)
				return nil
			},
		},
		{
			name: "MatviewJobRunsReport",
			fn: func(dbc *db.DB) error {
				pagination := &apitype.Pagination{PerPage: 20, Page: 0}
				result, err := api.JobsRunsReportFromDB(dbc,
					&filter.FilterOptions{Filter: &filter.Filter{}},
					benchmarkRelease, pagination, asOf)
				if err == nil {
					log.Printf("MatviewJobRunsReport: %d rows", result.TotalRows)
				}
				return err
			},
		},
		{
			name: "MatviewPayloadTestFailures",
			fn: func(dbc *db.DB) error {
				type payloadFailure struct {
					Release       string
					Architecture  string
					Stream        string
					ProwJobRunID  uint
					TestID        uint
					Name          string
					ProwJobName   string
					ProwJobRunURL string
				}
				var results []payloadFailure
				res := dbc.DB.Table("payload_test_failures_14d_matview").
					Where("release = ?", benchmarkRelease).
					Limit(50).
					Scan(&results)
				if res.Error != nil {
					return res.Error
				}
				log.Printf("MatviewPayloadTestFailures: %d results for release %s", len(results), benchmarkRelease)
				return nil
			},
		},
		{
			name: "ViewTestAnalysisByVariant",
			fn: func(dbc *db.DB) error {
				results, err := api.GetTestAnalysisByVariantFromDB(dbc, nil,
					benchmarkRelease, benchmarkTestName, asOf)
				if err == nil {
					log.Printf("ViewTestAnalysisByVariant: %d groups", len(results))
				}
				return err
			},
		},
	}
}

func Test_BenchmarkMatviews(t *testing.T) {
	dbc, connName := getBenchmarkDBClient(t)
	asOf := time.Now().UTC()
	iterations := 3
	cases := getMatviewBenchmarkCases(asOf)

	var results []benchmarkResult
	for _, bc := range cases {
		t.Run(bc.name, func(t *testing.T) {
			r := runBenchmarkCase(t, dbc, bc, iterations)
			results = append(results, r)
		})
	}
	printSummaryTable(t, results, connName)
}

func testAnalysisPageFilter() *filter.Filter {
	return &filter.Filter{
		Items: []filter.FilterItem{
			{Field: "variants", Value: "never-stable", Not: true},
			{Field: "variants", Value: "aggregated", Not: true},
		},
	}
}

func getAPIBenchmarkCases(asOf time.Time) []benchmarkCase {
	f := testAnalysisPageFilter()
	return []benchmarkCase{
		{
			name: "APITestAnalysisOverall",
			fn: func(dbc *db.DB) error {
				results, err := api.GetTestAnalysisOverallFromDB(dbc, f,
					benchmarkRelease, benchmarkTestName, asOf)
				if err == nil {
					log.Printf("APITestAnalysisOverall: %d dates", len(results["overall"]))
				}
				return err
			},
		},
		{
			name: "APITestAnalysisByJob",
			fn: func(dbc *db.DB) error {
				results, err := api.GetTestAnalysisByJobFromDB(dbc, f,
					benchmarkRelease, benchmarkTestName, asOf)
				if err == nil {
					log.Printf("APITestAnalysisByJob: %d groups", len(results))
				}
				return err
			},
		},
		{
			name: "APITestAnalysisByVariant",
			fn: func(dbc *db.DB) error {
				results, err := api.GetTestAnalysisByVariantFromDB(dbc, f,
					benchmarkRelease, benchmarkTestName, asOf)
				if err == nil {
					log.Printf("APITestAnalysisByVariant: %d groups", len(results))
				}
				return err
			},
		},
		{
			name: "APITestAnalysisPageLoad",
			fn: func(dbc *db.DB) error {
				if _, err := api.GetTestAnalysisOverallFromDB(dbc, f,
					benchmarkRelease, benchmarkTestName, asOf); err != nil {
					return err
				}
				if _, err := api.GetTestAnalysisByJobFromDB(dbc, f,
					benchmarkRelease, benchmarkTestName, asOf); err != nil {
					return err
				}
				if _, err := api.GetTestAnalysisByVariantFromDB(dbc, f,
					benchmarkRelease, benchmarkTestName, asOf); err != nil {
					return err
				}
				log.Printf("APITestAnalysisPageLoad: all 3 endpoints completed")
				return nil
			},
		},
		{
			name: "APITestsReport",
			fn: func(dbc *db.DB) error {
				rawFilter := &filter.Filter{
					Items: []filter.FilterItem{
						{Field: "name", Operator: filter.OperatorContains, Value: "test"},
						{Field: "variants", Operator: filter.OperatorHasEntry, Value: "never-stable", Not: true},
						{Field: "variants", Operator: filter.OperatorHasEntry, Value: "aggregated", Not: true},
					},
				}
				processedFilter := &filter.Filter{
					Items: []filter.FilterItem{
						{Field: "current_runs", Operator: filter.OperatorArithmeticGreaterThanOrEquals, Value: "7"},
						{Field: "current_flake_percentage", Operator: filter.OperatorArithmeticEquals, Value: "100", Not: true},
					},
				}
				sample, base := query.PeriodsForReportType(v1.CurrentReport)
				inner, err := query.TestReportQuery(dbc, benchmarkRelease, sample, base, query.TestNameMatches{})
				if err != nil {
					return err
				}
				rawQuery := dbc.DB.
					Table("(?) AS r", inner).
					Select("suite_name, name, jira_component, jira_component_id, " + query.QueryTestSummer).
					Group("suite_name, name, jira_component, jira_component_id")
				rawQuery = rawFilter.ToSQL(rawQuery, apitype.Test{})

				processedResults := dbc.DB.Table("(?) as results", rawQuery).
					Select("suite_name, name, jira_component, jira_component_id, " + query.QueryTestSummarizer).
					Where("current_runs > 0 or previous_runs > 0")

				finalResults := dbc.DB.Table("(?) as final_results", processedResults)
				finalResults = processedFilter.ToSQL(finalResults, apitype.Test{})

				var testReports []apitype.Test
				res := finalResults.Order("net_improvement asc").Scan(&testReports)
				if res.Error != nil {
					return res.Error
				}
				log.Printf("APITestsReport: %d tests", len(testReports))
				return nil
			},
		},
		{
			name: "APIJobRunsReport",
			fn: func(dbc *db.DB) error {
				pagination := &apitype.Pagination{PerPage: 20, Page: 0}
				filterOpts := &filter.FilterOptions{
					Filter: &filter.Filter{
						Items: []filter.FilterItem{
							{Field: "ran_test_names", Operator: filter.OperatorHasEntry, Value: benchmarkTestName},
							{Field: "timestamp", Operator: filter.OperatorArithmeticGreaterThan, Value: fmt.Sprintf("%d", asOf.Add(-14*24*time.Hour).UnixMilli())},
							{Field: "variants", Operator: filter.OperatorHasEntry, Value: "never-stable", Not: true},
							{Field: "variants", Operator: filter.OperatorHasEntry, Value: "aggregated", Not: true},
						},
					},
					SortField: "timestamp",
					Sort:      "desc",
				}
				result, err := api.JobsRunsReportFromDB(dbc, filterOpts, benchmarkRelease, pagination, asOf)
				if err == nil {
					log.Printf("APIJobRunsReport: %d total rows", result.TotalRows)
				}
				return err
			},
		},
		{
			name: "APIJobRunsReportNoTestFilter",
			fn: func(dbc *db.DB) error {
				pagination := &apitype.Pagination{PerPage: 20, Page: 0}
				filterOpts := &filter.FilterOptions{
					Filter: &filter.Filter{
						Items: []filter.FilterItem{
							{Field: "timestamp", Operator: filter.OperatorArithmeticGreaterThan, Value: fmt.Sprintf("%d", asOf.Add(-14*24*time.Hour).UnixMilli())},
							{Field: "variants", Operator: filter.OperatorHasEntry, Value: "never-stable", Not: true},
							{Field: "variants", Operator: filter.OperatorHasEntry, Value: "aggregated", Not: true},
						},
					},
					SortField: "timestamp",
					Sort:      "desc",
				}
				result, err := api.JobsRunsReportFromDB(dbc, filterOpts, benchmarkRelease, pagination, asOf)
				if err == nil {
					log.Printf("APIJobRunsReportNoTestFilter: %d total rows", result.TotalRows)
				}
				return err
			},
		},
	}
}

func Test_BenchmarkCumulativeQueryTestsReport(t *testing.T) {
	dbc, connName := getBenchmarkDBClient(t)

	var results []benchmarkResult

	results = append(results, runBenchmarkCase(t, dbc, benchmarkCase{
		name: "QueryAPITestsReport",
		fn: func(dbc *db.DB) error {
			rawFilter := &filter.Filter{
				Items: []filter.FilterItem{
					{Field: "name", Operator: filter.OperatorContains, Value: "test"},
					{Field: "variants", Operator: filter.OperatorHasEntry, Value: "never-stable", Not: true},
					{Field: "variants", Operator: filter.OperatorHasEntry, Value: "aggregated", Not: true},
				},
			}
			processedFilter := &filter.Filter{
				Items: []filter.FilterItem{
					{Field: "current_runs", Operator: filter.OperatorArithmeticGreaterThanOrEquals, Value: "7"},
					{Field: "current_flake_percentage", Operator: filter.OperatorArithmeticEquals, Value: "100", Not: true},
				},
			}
			sample, base := query.PeriodsForReportType(v1.CurrentReport)
			inner, err := query.TestReportQuery(dbc, benchmarkRelease, sample, base, query.TestNameMatches{})
			if err != nil {
				return err
			}
			rawQuery := dbc.DB.
				Table("(?) AS r", inner).
				Select("suite_name, name, jira_component, jira_component_id, " + query.QueryTestSummer).
				Group("suite_name, name, jira_component, jira_component_id")
			rawQuery = rawFilter.ToSQL(rawQuery, apitype.Test{})

			processedResults := dbc.DB.Table("(?) as results", rawQuery).
				Select("suite_name, name, jira_component, jira_component_id, " + query.QueryTestSummarizer).
				Where("current_runs > 0 or previous_runs > 0")

			finalResults := dbc.DB.Table("(?) as final_results", processedResults)
			finalResults = processedFilter.ToSQL(finalResults, apitype.Test{})

			var testReports []apitype.Test
			res := finalResults.Order("net_improvement asc").Scan(&testReports)
			if res.Error != nil {
				return res.Error
			}
			log.Printf("QueryAPITestsReport: %d tests from cumulative summaries", len(testReports))
			return nil
		},
	}, 3))

	printSummaryTable(t, results, connName)
}

func Test_BenchmarkJobRunsReportMatview(t *testing.T) {
	dbc, connName := getBenchmarkDBClient(t)

	var source db.PostgresView
	for _, mv := range db.PostgresMatViews {
		if mv.Name == "prow_job_runs_report_matview" {
			source = mv
			break
		}
	}
	if source.Name == "" {
		t.Fatal("prow_job_runs_report_matview not found in PostgresMatViews")
	}

	matviewName := "bench_job_runs_report"
	viewDef := source.Definition
	for k, v := range source.ReplaceStrings {
		viewDef = strings.ReplaceAll(viewDef, k, v)
	}
	viewDef = strings.ReplaceAll(viewDef, "|||TIMENOW|||", "NOW()")

	t.Cleanup(func() {
		if err := dbc.DB.Exec(fmt.Sprintf("DROP MATERIALIZED VIEW IF EXISTS %s", matviewName)).Error; err != nil {
			t.Logf("failed to drop materialized view %s during cleanup: %v", matviewName, err)
		}
	})
	if err := dbc.DB.Exec(fmt.Sprintf("DROP MATERIALIZED VIEW IF EXISTS %s", matviewName)).Error; err != nil {
		t.Fatalf("failed to drop pre-existing materialized view %s: %v", matviewName, err)
	}

	iterations := 1
	var results []benchmarkResult

	results = append(results, runBenchmarkCase(t, dbc, benchmarkCase{
		name: "CreateMatview",
		fn: func(dbc *db.DB) error {
			if err := dbc.DB.Exec(fmt.Sprintf("DROP MATERIALIZED VIEW IF EXISTS %s", matviewName)).Error; err != nil {
				return err
			}
			res := dbc.DB.Exec(fmt.Sprintf("CREATE MATERIALIZED VIEW %s AS %s WITH DATA", matviewName, viewDef))
			if res.Error != nil {
				return res.Error
			}
			var count int64
			if err := dbc.DB.Raw(fmt.Sprintf("SELECT COUNT(*) FROM %s", matviewName)).Scan(&count).Error; err != nil {
				return err
			}
			log.Printf("CreateMatview: %s populated with %d rows", matviewName, count)
			return nil
		},
	}, iterations))

	indexName := fmt.Sprintf("idx_%s", matviewName)
	indexCols := strings.Join(source.IndexColumns, ", ")
	results = append(results, runBenchmarkCase(t, dbc, benchmarkCase{
		name: "CreateIndex",
		fn: func(dbc *db.DB) error {
			dbc.DB.Exec(fmt.Sprintf("DROP INDEX IF EXISTS %s", indexName))
			res := dbc.DB.Exec(fmt.Sprintf("CREATE UNIQUE INDEX %s ON %s(%s)", indexName, matviewName, indexCols))
			return res.Error
		},
	}, iterations))

	results = append(results, runBenchmarkCase(t, dbc, benchmarkCase{
		name: "RefreshConcurrently",
		fn: func(dbc *db.DB) error {
			res := dbc.DB.Exec(fmt.Sprintf("REFRESH MATERIALIZED VIEW CONCURRENTLY %s", matviewName))
			return res.Error
		},
	}, iterations))

	results = append(results, runBenchmarkCase(t, dbc, benchmarkCase{
		name: "QueryByRelease",
		fn: func(dbc *db.DB) error {
			var jobRuns []apitype.JobRun
			res := dbc.DB.Table(matviewName).
				Where("release = ?", benchmarkRelease).
				Order("timestamp desc").
				Limit(100).
				Scan(&jobRuns)
			if res.Error != nil {
				return res.Error
			}
			log.Printf("QueryByRelease: %d rows from %s", len(jobRuns), matviewName)
			return nil
		},
	}, iterations))

	results = append(results, runBenchmarkCase(t, dbc, benchmarkCase{
		name: "QueryByJob",
		fn: func(dbc *db.DB) error {
			var jobRuns []apitype.JobRun
			res := dbc.DB.Table(matviewName).
				Where("release = ? AND name = ?", benchmarkRelease, benchmarkJobName).
				Order("timestamp desc").
				Scan(&jobRuns)
			if res.Error != nil {
				return res.Error
			}
			log.Printf("QueryByJob: %d rows from %s", len(jobRuns), matviewName)
			return nil
		},
	}, iterations))

	results = append(results, runBenchmarkCase(t, dbc, benchmarkCase{
		name: "CountWithFailures",
		fn: func(dbc *db.DB) error {
			var count int64
			res := dbc.DB.Table(matviewName).
				Where("release = ? AND test_failures > 0", benchmarkRelease).
				Count(&count)
			if res.Error != nil {
				return res.Error
			}
			log.Printf("CountWithFailures: %d rows from %s", count, matviewName)
			return nil
		},
	}, iterations))

	printSummaryTable(t, results, connName)
}

func Test_BenchmarkRefreshData(t *testing.T) {
	dbc, connName := getBenchmarkDBClient(t)

	if err := dbc.UpdateSchema(nil); err != nil {
		t.Fatalf("could not migrate db: %v", err)
	}

	r := runBenchmarkCase(t, dbc, benchmarkCase{
		name: "RefreshData",
		fn: func(dbc *db.DB) error {
			return sippyserver.RefreshData(dbc, nil, sippyserver.RefreshOptions{})
		},
	}, 1)
	printSummaryTable(t, []benchmarkResult{r}, connName)
}

func Test_BenchmarkAPI(t *testing.T) {
	dbc, connName := getBenchmarkDBClient(t)
	asOf := time.Now().UTC()
	iterations := 3
	cases := getAPIBenchmarkCases(asOf)

	var results []benchmarkResult
	for _, bc := range cases {
		t.Run(bc.name, func(t *testing.T) {
			r := runBenchmarkCase(t, dbc, bc, iterations)
			results = append(results, r)
		})
	}
	printSummaryTable(t, results, connName)
}
