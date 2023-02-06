package flags

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/spf13/pflag"

	"github.com/openshift/sippy/pkg/sippyserver"
)

type TestGridFlags struct {
	JobFilter               string
	LocalData               string
	StartDay                int
	NumDays                 int
	TestSuccessThreshold    float64
	MinTestRuns             int
	FailureClusterThreshold int
	Dashboards              []string
}

func NewTestGridFlags() *TestGridFlags {
	return &TestGridFlags{
		NumDays:                 14,
		TestSuccessThreshold:    99.99,
		MinTestRuns:             10,
		FailureClusterThreshold: 10,
		StartDay:                0,
	}
}

func (f *TestGridFlags) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&f.JobFilter, "job-filter", f.JobFilter, "Only analyze jobs that match this regex")
	fs.StringVar(&f.LocalData, "local-data", f.LocalData, "Path to testgrid data from local disk")
	fs.IntVar(&f.StartDay, "start-day", f.StartDay,
		"Most recent day to start processing testgrid results for (moving backward in time). (0 will start from now (default), -1 will start from whatever the most recent test results are) i.e. --start-day 30 --num-days 14 would load test grid results from 30 days ago back to 30+14=44 days ago.")
	fs.IntVar(&f.NumDays, "num-days", f.NumDays,
		"Number of days prior to --start-day to analyze testgrid results back to. (default 14 days) i.e. --start-day 30 --num-days 14 would load test grid results from 30 days ago back to 30+14=44 days ago.")
	fs.Float64Var(&f.TestSuccessThreshold, "test-success-threshold", f.TestSuccessThreshold,
		"Filter results for tests that are more than this percent successful")
	fs.IntVar(&f.MinTestRuns, "min-test-runs", f.MinTestRuns, "Ignore tests with less than this number of runs")
	fs.IntVar(&f.FailureClusterThreshold, "failure-cluster-threshold", f.FailureClusterThreshold, "Include separate report on job runs with more than N test failures, -1 to disable")
	fs.StringArrayVar(&f.Dashboards, "dashboard", f.Dashboards, "<display-name>=<comma-separated-list-of-dashboards>=<openshift-version>")
}

func (f *TestGridFlags) TestGridLoadingConfig() sippyserver.TestGridLoadingConfig {
	var jobFilter *regexp.Regexp
	if len(f.JobFilter) > 0 {
		jobFilter = regexp.MustCompile(f.JobFilter)
	}

	return sippyserver.TestGridLoadingConfig{
		LocalData: f.LocalData,
		JobFilter: jobFilter,
	}
}

func (f *TestGridFlags) RawJobResultsAnalysisConfig() sippyserver.RawJobResultsAnalysisConfig {
	return sippyserver.RawJobResultsAnalysisConfig{
		StartDay: f.StartDay,
		NumDays:  f.NumDays,
	}
}
func (f *TestGridFlags) DisplayDataConfig() sippyserver.DisplayDataConfig {
	return sippyserver.DisplayDataConfig{
		MinTestRuns:             f.MinTestRuns,
		TestSuccessThreshold:    f.TestSuccessThreshold,
		FailureClusterThreshold: f.FailureClusterThreshold,
	}
}

func (f *TestGridFlags) TestGridDashboardCoordinates() []sippyserver.TestGridDashboardCoordinates {
	dashboards := []sippyserver.TestGridDashboardCoordinates{}
	for _, dashboard := range f.Dashboards {
		tokens := strings.Split(dashboard, "=")
		if len(tokens) != 3 {
			// launch error
			panic(fmt.Sprintf("must have three tokens: %q", dashboard))
		}

		dashboards = append(dashboards,
			sippyserver.TestGridDashboardCoordinates{
				ReportName:             tokens[0],
				TestGridDashboardNames: strings.Split(tokens[1], ","),
				BugzillaRelease:        tokens[2],
			},
		)
	}

	return dashboards
}
