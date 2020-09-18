package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"

	"github.com/openshift/sippy/pkg/sippyserver"

	"github.com/openshift/sippy/pkg/testgridanalysis/testgridhelpers"

	"github.com/openshift/sippy/pkg/buganalysis"
	"github.com/spf13/cobra"
	"k8s.io/klog"
)

type Options struct {
	LocalData               string
	Releases                []string
	StartDay                int
	EndDay                  int
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
		EndDay:                  7,
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
			if err := opt.Run(); err != nil {
				klog.Exitf("error: %v", err)
			}
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&opt.LocalData, "local-data", opt.LocalData, "Path to testgrid data from local disk")
	flags.StringArrayVar(&opt.Releases, "release", opt.Releases, "Which releases to analyze (one per arg instance)")
	flags.IntVar(&opt.StartDay, "start-day", opt.StartDay, "Analyze data starting from this day")
	flags.IntVar(&opt.EndDay, "end-day", opt.EndDay, "Look at job runs going back to this day")
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

func (o *Options) Run() error {
	switch o.Output {
	case "json", "text", "dashboard":
	default:
		return fmt.Errorf("invalid output type: %s\n", o.Output)
	}

	if len(o.FetchData) != 0 {
		testgridhelpers.DownloadData(o.Releases, o.JobFilter, o.FetchData)
		return nil
	}
	if !o.Server {
		analyzer := sippyserver.Analyzer{
			Options:  o.toServerOptions(),
			BugCache: buganalysis.NewBugCache(),
		}

		testReport := analyzer.PrepareTestReport()
		o.printJSONReport(testReport)
	}

	if o.Server {
		server := sippyserver.NewServer(o.toServerOptions())
		server.RefreshData()
		server.Serve()
	}

	return nil
}

func (o *Options) toServerOptions() sippyserver.Options {
	return sippyserver.Options{
		LocalData:               o.LocalData,
		Releases:                o.Releases,
		StartDay:                o.StartDay,
		EndDay:                  o.EndDay,
		TestSuccessThreshold:    o.TestSuccessThreshold,
		JobFilter:               o.JobFilter,
		MinTestRuns:             o.MinTestRuns,
		Output:                  o.Output,
		FailureClusterThreshold: o.FailureClusterThreshold,
		FetchData:               o.FetchData,
		ListenAddr:              o.ListenAddr,
		Server:                  o.Server,
	}
}

func (o *Options) printJSONReport(testReport sippyprocessingv1.TestReport) {
	switch o.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.Encode(testReport)
	}
}
