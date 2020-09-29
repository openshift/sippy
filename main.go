package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"

	"github.com/openshift/sippy/pkg/buganalysis"

	"github.com/openshift/sippy/pkg/sippyserver"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridhelpers"
	"github.com/spf13/cobra"
	"k8s.io/klog"
)

type Options struct {
	LocalData               string
	Releases                []string
	StartDay                int
	endDay                  int
	NumDays                 int
	TestSuccessThreshold    float64
	JobFilter               string
	MinTestRuns             int
	Output                  string
	FailureClusterThreshold int
	FetchData               string
	ListenAddr              string
	Server                  bool
}

func main() {
	opt := &Options{
		endDay:                  0,
		NumDays:                 7,
		TestSuccessThreshold:    99.99,
		MinTestRuns:             10,
		Output:                  "json",
		FailureClusterThreshold: 10,
		StartDay:                0,
		ListenAddr:              ":8080",
		Releases:                []string{"4.4"},
	}

	klog.InitFlags(nil)
	flag.CommandLine.Set("skip_headers", "true")

	cmd := &cobra.Command{
		Run: func(cmd *cobra.Command, arguments []string) {
			if err := opt.Complete(); err != nil {
				klog.Exitf("error: %v", err)
			}
			if err := opt.Validate(); err != nil {
				klog.Exitf("error: %v", err)
			}
			if err := opt.Run(); err != nil {
				klog.Exitf("error: %v", err)
			}
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&opt.LocalData, "local-data", opt.LocalData, "Path to testgrid data from local disk")
	flags.StringArrayVar(&opt.Releases, "release", opt.Releases, "Which releases to analyze (one per arg instance)")
	flags.IntVar(&opt.StartDay, "start-day", opt.StartDay, "Analyze data starting from this day")
	// TODO convert this to be an offset so that we can go backwards from "data we have"
	flags.IntVar(&opt.endDay, "end-day", opt.endDay, "Look at job runs going back to this day")
	flags.IntVar(&opt.NumDays, "num-days", opt.NumDays, "Look at job runs going back to this many days from the start day")
	flags.Float64Var(&opt.TestSuccessThreshold, "test-success-threshold", opt.TestSuccessThreshold, "Filter results for tests that are more than this percent successful")
	flags.StringVar(&opt.JobFilter, "job-filter", opt.JobFilter, "Only analyze jobs that match this regex")
	flags.StringVar(&opt.FetchData, "fetch-data", opt.FetchData, "Download testgrid data to directory specified for future use with --local-data")
	flags.IntVar(&opt.MinTestRuns, "min-test-runs", opt.MinTestRuns, "Ignore tests with less than this number of runs")
	flags.IntVar(&opt.FailureClusterThreshold, "failure-cluster-threshold", opt.FailureClusterThreshold, "Include separate report on job runs with more than N test failures, -1 to disable")
	flags.StringVarP(&opt.Output, "output", "o", opt.Output, "Output format for report: json, text")
	flag.StringVar(&opt.ListenAddr, "listen", opt.ListenAddr, "The address to serve analysis reports on")
	flags.BoolVar(&opt.Server, "server", opt.Server, "Run in web server mode (serve reports over http)")

	flags.AddGoFlag(flag.CommandLine.Lookup("v"))
	flags.AddGoFlag(flag.CommandLine.Lookup("skip_headers"))

	if err := cmd.Execute(); err != nil {
		klog.Exitf("error: %v", err)
	}
}

func (o *Options) Complete() error {
	// if the end day was explicitly specified, honor that
	if o.endDay != 0 {
		o.NumDays = o.endDay - o.StartDay
	}

	return nil
}

func (o *Options) Validate() error {
	switch o.Output {
	case "json":
	default:
		return fmt.Errorf("invalid output type: %s\n", o.Output)
	}

	return nil
}

func (o *Options) Run() error {
	if len(o.FetchData) != 0 {
		testgridhelpers.DownloadData(o.Releases, o.JobFilter, o.FetchData)
		return nil
	}

	if !o.Server {
		return o.runCLIReportMode()
	}

	if o.Server {
		return o.runServerMode()
	}

	return nil
}

func (o *Options) runServerMode() error {
	server := sippyserver.NewServer(
		o.toTestGridLoadingConfig(),
		o.toRawJobResultsAnalysisConfig(),
		o.toDisplayDataConfig(),
		o.Releases,
		o.ListenAddr,
	)
	server.RefreshData() // force a data refresh once before serving.
	server.Serve()
	return nil
}

func (o *Options) runCLIReportMode() error {
	analyzer := sippyserver.TestReportGeneratorConfig{
		TestGridLoadingConfig:       o.toTestGridLoadingConfig(),
		RawJobResultsAnalysisConfig: o.toRawJobResultsAnalysisConfig(),
		DisplayDataConfig:           o.toDisplayDataConfig(),
	}

	testReport := analyzer.PrepareTestReport(o.Releases[0], buganalysis.NewBugCache())
	enc := json.NewEncoder(os.Stdout)
	enc.Encode(testReport)
	return nil
}

func (o *Options) toTestGridLoadingConfig() sippyserver.TestGridLoadingConfig {
	var jobFilter *regexp.Regexp
	if len(o.JobFilter) > 0 {
		jobFilter = regexp.MustCompile(o.JobFilter)
	}

	return sippyserver.TestGridLoadingConfig{
		LocalData: o.LocalData,
		JobFilter: jobFilter,
	}
}

func (o *Options) toRawJobResultsAnalysisConfig() sippyserver.RawJobResultsAnalysisConfig {
	return sippyserver.RawJobResultsAnalysisConfig{
		StartDay: o.StartDay,
		NumDays:  o.NumDays,
	}
}
func (o *Options) toDisplayDataConfig() sippyserver.DisplayDataConfig {
	return sippyserver.DisplayDataConfig{
		MinTestRuns:             o.MinTestRuns,
		TestSuccessThreshold:    o.TestSuccessThreshold,
		FailureClusterThreshold: o.FailureClusterThreshold,
	}
}
