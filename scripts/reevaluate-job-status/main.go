package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"gorm.io/gorm"

	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/testconversion"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/flags"
	"github.com/openshift/sippy/pkg/synthetictests"

	"github.com/openshift/sippy/pkg/apis/prow"
)

func main() {
	dbFlags := flags.NewPostgresDatabaseFlags()
	var release, job string
	var days, samples int
	pflag.StringVar(&release, "release", "", "Filter by release (e.g. 4.22)")
	pflag.StringVar(&job, "job", "", "Filter by job name (exact match)")
	pflag.IntVar(&days, "days", 0, "Only evaluate job runs from the last N days")
	pflag.IntVar(&samples, "samples", 10, "Number of sample URLs to show per transition")
	dbFlags.BindFlags(pflag.CommandLine)
	pflag.Parse()

	if release == "" && job == "" {
		fmt.Fprintln(os.Stderr, "At least one of --release or --job is required")
		os.Exit(1)
	}

	dbc, err := dbFlags.GetDBClient()
	if err != nil {
		log.WithError(err).Fatal("could not connect to db")
	}

	query := dbc.DB.
		Table("prow_job_runs").
		Select("prow_job_runs.id, prow_job_runs.url, prow_job_runs.overall_result, prow_job_runs.succeeded, prow_job_runs.failed, prow_jobs.name AS job_name").
		Joins("JOIN prow_jobs ON prow_jobs.id = prow_job_runs.prow_job_id").
		Where("prow_job_runs.deleted_at IS NULL")

	if job != "" {
		query = query.Where("prow_jobs.name = ?", job)
	}
	if release != "" {
		query = query.Where("prow_jobs.release = ?", release)
	}
	if days > 0 {
		query = query.Where("prow_job_runs.timestamp >= NOW() - INTERVAL '1 day' * ?", days)
	}

	type jobRunRow struct {
		ID            uint
		URL           string
		OverallResult v1.JobOverallResult
		Succeeded     bool
		Failed        bool
		JobName       string
	}

	var runs []jobRunRow
	if err := query.Find(&runs).Error; err != nil {
		log.WithError(err).Fatal("could not query job runs")
	}

	manager := synthetictests.NewOpenshiftSyntheticTestManager()

	// Track transitions: "old -> new" counts and sample URLs
	transitions := make(map[string]int)
	transitionSamples := make(map[string][]string) // up to 10 sample URLs per transition
	oldCounts := make(map[v1.JobOverallResult]int)
	newCounts := make(map[v1.JobOverallResult]int)
	var totalChanged int

	for _, run := range runs {
		var newResult v1.JobOverallResult

		if run.Succeeded {
			newResult = v1.JobSucceeded
		} else {
			tests, err := loadTests(dbc.DB, run.ID)
			if err != nil {
				log.WithError(err).Warnf("could not load tests for run %d", run.ID)
				continue
			}

			var prowState prow.ProwJobState
			switch run.OverallResult {
			case v1.JobAborted:
				prowState = prow.AbortedState
			case v1.JobInternalInfrastructureFailure:
				prowState = prow.ErrorState
			default:
				prowState = prow.FailureState
			}

			pj := prow.ProwJob{
				Spec:   prow.ProwJobSpec{Job: run.JobName},
				Status: prow.ProwJobStatus{State: prowState},
			}

			_, newResult = testconversion.ConvertProwJobRunToSyntheticTests(pj, tests, manager)
		}

		oldCounts[run.OverallResult]++
		newCounts[newResult]++

		if run.OverallResult != newResult {
			totalChanged++
			key := fmt.Sprintf("%s -> %s", run.OverallResult, newResult)
			transitions[key]++
			if samples > 0 && len(transitionSamples[key]) < samples {
				transitionSamples[key] = append(transitionSamples[key], run.URL)
			}
		}
	}

	total := len(runs)

	// Print summary table
	allStatuses := []v1.JobOverallResult{
		v1.JobSucceeded,
		v1.JobTestFailure,
		v1.JobInstallFailure,
		v1.JobUpgradeFailure,
		v1.JobExternalInfrastructureFailure,
		v1.JobInternalInfrastructureFailure,
		v1.JobAborted,
		v1.JobUnknown,
	}

	fmt.Printf("\nJob runs evaluated: %d\n", total)
	fmt.Printf("Would change: %d\n\n", totalChanged)

	fmt.Printf("%-5s %-40s %10s %8s %10s %8s\n", "Code", "Status", "Current", "%", "After", "%")
	fmt.Println(strings.Repeat("-", 85))
	for _, s := range allStatuses {
		old := oldCounts[s]
		new_ := newCounts[s]
		if old == 0 && new_ == 0 {
			continue
		}
		oldPct := float64(old) / float64(total) * 100
		newPct := float64(new_) / float64(total) * 100
		fmt.Printf("%-5s %-40s %10d %7.1f%% %10d %7.1f%%\n",
			string(s), s.String(), old, oldPct, new_, newPct)
	}
	fmt.Println(strings.Repeat("-", 85))
	fmt.Printf("%-5s %-40s %10d %7.1f%% %10d %7.1f%%\n",
		"", "Total", total, 100.0, total, 100.0)

	// Print transitions
	if totalChanged > 0 {
		fmt.Printf("\nTransitions:\n")
		fmt.Printf("%-10s %10s %8s\n", "Change", "Count", "%")
		fmt.Println(strings.Repeat("-", 30))

		sorted := make([]string, 0, len(transitions))
		for k := range transitions {
			sorted = append(sorted, k)
		}
		sort.Slice(sorted, func(i, j int) bool {
			return transitions[sorted[i]] > transitions[sorted[j]]
		})
		for _, k := range sorted {
			count := transitions[k]
			pct := float64(count) / float64(total) * 100
			fmt.Printf("%-10s %10d %7.1f%%\n", k, count, pct)
		}

		if samples == 0 {
			return
		}
		fmt.Printf("\nSamples (up to %d per transition):\n", samples)
		for _, k := range sorted {
			fmt.Printf("\n  %s:\n", k)
			for _, url := range transitionSamples[k] {
				fmt.Printf("    %s\n", url)
			}
		}
	}
}

func loadTests(db *gorm.DB, prowJobRunID uint) (map[string]*models.ProwJobRunTest, error) {
	var tests []struct {
		models.ProwJobRunTest
		SuiteName string
		TestName  string
	}

	err := db.
		Table("prow_job_run_tests").
		Select("prow_job_run_tests.*, suites.name AS suite_name, tests.name AS test_name").
		Joins("LEFT JOIN suites ON suites.id = prow_job_run_tests.suite_id").
		Joins("JOIN tests ON tests.id = prow_job_run_tests.test_id").
		Where("prow_job_run_tests.prow_job_run_id = ?", prowJobRunID).
		Find(&tests).Error
	if err != nil {
		return nil, err
	}

	result := make(map[string]*models.ProwJobRunTest, len(tests))
	for i := range tests {
		// Skip synthetic tests that sippy itself created — they weren't
		// part of the original test results and would pollute reclassification.
		if tests[i].SuiteName == "sippy" {
			continue
		}
		key := fmt.Sprintf("%s.%s", tests[i].SuiteName, tests[i].TestName)
		t := tests[i].ProwJobRunTest
		result[key] = &t
	}
	return result, nil
}
