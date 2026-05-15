package flags

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/openshift/sippy/pkg/api"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/query"
	log "github.com/sirupsen/logrus"
)

const benchmarkRelease = "4.22"
const benchmarkTestName = "[Monitor:legacy-test-framework-invariants-pathological][sig-arch] events should not repeat pathologically for ns/kube-system"

type benchmarkCase struct {
	name string
	fn   func(dbc *db.DB) error
}

func getBenchmarkCases() []benchmarkCase {
	return []benchmarkCase{
		{
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
					"periodic-ci-openshift-release-master-ci-4.22-e2e-aws-ovn",
					time.Now())

				if err == nil {
					log.Printf("Found %d job runs", len(jobRuns))
				}

				return err
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
	iterations := 1
	cases := getBenchmarkCases()

	for _, bc := range cases {
		t.Run(bc.name, func(t *testing.T) {
			var totalDuration time.Duration
			for i := 0; i < iterations; i++ {
				start := time.Now()
				err := bc.fn(dbc)
				elapsed := time.Since(start)
				if err != nil {
					t.Fatalf("iteration %d failed: %v", i+1, err)
				}
				totalDuration += elapsed
				fmt.Printf("  %s iteration %d: %s\n", bc.name, i+1, elapsed)
			}
			avg := totalDuration / time.Duration(iterations)
			fmt.Printf("  %s total: %s, avg: %s (%d iterations)\n",
				bc.name, totalDuration, avg, iterations)
		})
	}
}

func Test_BenchmarkGroup(t *testing.T) {
	dbc := getBenchmarkDBClient(t)
	iterations := 1
	cases := getBenchmarkCases()

	var totalDuration time.Duration
	for i := 0; i < iterations; i++ {
		start := time.Now()
		for _, bc := range cases {
			err := bc.fn(dbc)
			if err != nil {
				t.Fatalf("group iteration %d, case %s failed: %v", i+1, bc.name, err)
			}
		}
		elapsed := time.Since(start)
		totalDuration += elapsed
		fmt.Printf("  group iteration %d: %s\n", i+1, elapsed)
	}
	avg := totalDuration / time.Duration(iterations)
	fmt.Printf("  group total: %s, avg: %s (%d iterations)\n",
		totalDuration, avg, iterations)
}
