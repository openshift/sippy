package flags

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/openshift/sippy/pkg/api"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/query"
	"github.com/openshift/sippy/pkg/filter"
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

func printSummaryTable(results []benchmarkResult) {
	nameWidth := 4
	for _, r := range results {
		if len(r.name) > nameWidth {
			nameWidth = len(r.name)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].avg > results[j].avg
	})

	header := fmt.Sprintf("  %-*s  %5s  %12s  %12s  %12s  %12s",
		nameWidth, "Name", "Iters", "Total", "Avg", "Min", "Max")
	fmt.Println()
	fmt.Println(header)
	fmt.Println("  " + strings.Repeat("-", len(header)-2))
	for _, r := range results {
		fmt.Printf("  %-*s  %5d  %12s  %12s  %12s  %12s\n",
			nameWidth, r.name, r.iterations, r.total, r.avg, r.min, r.max)
	}
	fmt.Println()
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
					JOIN prow_job_runs pjr ON pjr.id = pjrt.prow_job_run_id
					JOIN prow_jobs pj ON pj.id = pjr.prow_job_id
					WHERE pj.release = ?
					  AND t.name LIKE ?
					  AND pjrt.created_at > NOW() - INTERVAL '14 days'
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

func getBenchmarkCases() []benchmarkCase {
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
					benchmarkJobName, time.Now())

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
					benchmarkRelease, benchmarkTestName, time.Now())

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
					benchmarkRelease, benchmarkTestName, time.Now())

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
					benchmarkRelease, benchmarkTestName, time.Now())

				if err == nil {
					log.Printf("TestAnalysisByJobWithVariantFilter: %d groups", len(results))
				}

				return err
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
					JOIN prow_job_runs pjr ON pjr.id = pjrt.prow_job_run_id
					JOIN prow_jobs pj ON pj.id = pjr.prow_job_id
					WHERE pjrt.created_at > ?
					  AND pj.release = ?`, truncatedTime, benchmarkRelease).Scan(&result)
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
					JOIN prow_job_runs pjr ON pjr.id = pjrt.prow_job_run_id
					JOIN prow_jobs pj ON pj.id = pjr.prow_job_id
					WHERE pjrt.created_at > ?
					  AND pj.release = ?`, truncatedTime, benchmarkRelease).Scan(&result)
				if res.Error != nil {
					return res.Error
				}
				log.Printf("TestCountsByLookback9ForRelease %s: %d job runs, %d test IDs",
					benchmarkRelease, result.JobRunsCount, result.TestIDsCount)
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
					[]string{benchmarkJobName}).Scan(&result)
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

func getBenchmarkDBClient(t *testing.T) *db.DB {
	t.Helper()
	dsn := os.Getenv("db_benchmarking_dsn")
	if dsn == "" {
		t.Skip("skipping: set db_benchmarking_dsn to run")
	}

	dbFlags := &PostgresFlags{
		LogLevel: 4,
		DSN:      dsn,
	}

	dbc, err := dbFlags.GetDBClient()
	if err != nil {
		t.Fatalf("couldn't get DB client: %v", err)
	}
	return dbc
}

func Test_BenchmarkIndividual(t *testing.T) {
	dbc := getBenchmarkDBClient(t)
	iterations := 3
	cases := getBenchmarkCases()

	var results []benchmarkResult
	for _, bc := range cases {
		t.Run(bc.name, func(t *testing.T) {
			r := runBenchmarkCase(t, dbc, bc, iterations)
			results = append(results, r)
		})
	}
	printSummaryTable(results)
}

func Test_BenchmarkFindTestsByRelease(t *testing.T) {
	dbc := getBenchmarkDBClient(t)
	iterations := 1
	bc := getIndividualBenchmarkCases()["FindTestsByRelease"]

	r := runBenchmarkCase(t, dbc, bc, iterations)
	printSummaryTable([]benchmarkResult{r})
}

func Test_BenchmarkCombined(t *testing.T) {
	dbc := getBenchmarkDBClient(t)
	iterations := 3

	var results []benchmarkResult
	for _, bc := range getBenchmarkCases() {
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
	printSummaryTable(results)
}

func Test_BenchmarkGroup(t *testing.T) {
	dbc := getBenchmarkDBClient(t)
	iterations := 1
	cases := getBenchmarkCases()

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
	printSummaryTable([]benchmarkResult{group})
}
