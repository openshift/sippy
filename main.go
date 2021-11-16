package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"

	rice "github.com/GeertJohan/go.rice"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"k8s.io/klog"

	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/bigqueryexporter"
	"github.com/openshift/sippy/pkg/buganalysis"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/perfscaleanalysis"
	"github.com/openshift/sippy/pkg/sippyserver"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridconversion"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridhelpers"
	"github.com/openshift/sippy/pkg/testgridanalysis/testidentification"
	"github.com/openshift/sippy/pkg/util/sets"
)

type Options struct {
	LocalData         string
	OpenshiftReleases []string
	Dashboards        []string
	// TODO perhaps this could drive the synthetic tests too
	Variants                []string
	StartDay                int
	endDay                  int
	NumDays                 int
	TestSuccessThreshold    float64
	JobFilter               string
	MinTestRuns             int
	Output                  string
	FailureClusterThreshold int
	FetchData               string
	FetchPerfScaleData      bool
	ListenAddr              string
	Server                  bool
	SkipBugLookup           bool
	DSN                     string
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
	}

	klog.InitFlags(nil)
	if err := flag.CommandLine.Set("skip_headers", "true"); err != nil {
		klog.Exitf("could not set commandline flag: %s", err)
	}

	cmd := &cobra.Command{
		Run: func(cmd *cobra.Command, arguments []string) {
			opt.Complete()

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
	flags.StringVar(&opt.DSN, "database-dsn", os.Getenv("SIPPY_DATABASE_DSN"), "Database DSN for storage of some types of data")
	flags.StringArrayVar(&opt.OpenshiftReleases, "release", opt.OpenshiftReleases, "Which releases to analyze (one per arg instance)")
	flags.StringArrayVar(&opt.Dashboards, "dashboard", opt.Dashboards, "<display-name>=<comma-separated-list-of-dashboards>=<openshift-version>")
	flags.StringArrayVar(&opt.Variants, "variant", opt.Variants, "{ocp,kube,none}")
	flags.IntVar(&opt.StartDay, "start-day", opt.StartDay, "Analyze data starting from this day")
	// TODO convert this to be an offset so that we can go backwards from "data we have"
	flags.IntVar(&opt.endDay, "end-day", opt.endDay, "Look at job runs going back to this day")
	flags.IntVar(&opt.NumDays, "num-days", opt.NumDays, "Look at job runs going back to this many days from the start day")
	flags.Float64Var(&opt.TestSuccessThreshold, "test-success-threshold", opt.TestSuccessThreshold, "Filter results for tests that are more than this percent successful")
	flags.StringVar(&opt.JobFilter, "job-filter", opt.JobFilter, "Only analyze jobs that match this regex")
	flags.StringVar(&opt.FetchData, "fetch-data", opt.FetchData, "Download testgrid data to directory specified for future use with --local-data")
	flags.BoolVar(&opt.FetchPerfScaleData, "fetch-openshift-perfscale-data", opt.FetchPerfScaleData, "Download ElasticSearch data for workload CPU/memory use from jobs run by the OpenShift perfscale team. Will be stored in 'perfscale-metrics/' subdirectory beneath the --fetch-data dir.")
	flags.IntVar(&opt.MinTestRuns, "min-test-runs", opt.MinTestRuns, "Ignore tests with less than this number of runs")
	flags.IntVar(&opt.FailureClusterThreshold, "failure-cluster-threshold", opt.FailureClusterThreshold, "Include separate report on job runs with more than N test failures, -1 to disable")
	flags.StringVarP(&opt.Output, "output", "o", opt.Output, "Output format for report: json, text")
	flag.StringVar(&opt.ListenAddr, "listen", opt.ListenAddr, "The address to serve analysis reports on")
	flags.BoolVar(&opt.Server, "server", opt.Server, "Run in web server mode (serve reports over http)")
	flags.BoolVar(&opt.SkipBugLookup, "skip-bug-lookup", opt.SkipBugLookup, "Do not attempt to find bugs that match test/job failures")

	flags.AddGoFlag(flag.CommandLine.Lookup("v"))
	flags.AddGoFlag(flag.CommandLine.Lookup("skip_headers"))

	if err := cmd.Execute(); err != nil {
		klog.Exitf("error: %v", err)
	}
}

func (o *Options) Complete() {
	// if the end day was explicitly specified, honor that
	if o.endDay != 0 {
		o.NumDays = o.endDay - o.StartDay
	}

	for _, openshiftRelease := range o.OpenshiftReleases {
		o.Dashboards = append(o.Dashboards, dashboardArgFromOpenshiftRelease(openshiftRelease))
	}
}

func (o *Options) ToTestGridDashboardCoordinates() []sippyserver.TestGridDashboardCoordinates {
	dashboards := []sippyserver.TestGridDashboardCoordinates{}
	for _, dashboard := range o.Dashboards {
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

// dashboardArgFromOpenshiftRelease converts a --release string into the generic --dashboard arg
func dashboardArgFromOpenshiftRelease(release string) string {
	const openshiftDashboardTemplate = "redhat-openshift-ocp-release-%s-%s"

	dashboards := []string{
		fmt.Sprintf(openshiftDashboardTemplate, release, "blocking"),
		fmt.Sprintf(openshiftDashboardTemplate, release, "informing"),
	}

	argString := release + "=" + strings.Join(dashboards, ",") + "=" + release
	return argString
}

func (o *Options) Validate() error {
	switch o.Output {
	case "json":
	default:
		return fmt.Errorf("invalid output type: %s", o.Output)
	}

	for _, dashboard := range o.Dashboards {
		tokens := strings.Split(dashboard, "=")
		if len(tokens) != 3 {
			return fmt.Errorf("must have three tokens: %q", dashboard)
		}
	}

	if len(o.Variants) > 1 {
		return fmt.Errorf("only one --variant allowed for now")
	} else if len(o.Variants) == 1 {
		if !sets.NewString("ocp", "kube", "none").Has(o.Variants[0]) {
			return fmt.Errorf("only ocp, kube, or none is allowed")
		}
	}

	if o.FetchPerfScaleData && o.FetchData == "" {
		return fmt.Errorf("must specify --fetch-data with --fetch-openshift-perfscale-data")
	}

	return nil
}

func (o *Options) Run() error {
	if o.FetchData != "" {
		err := os.MkdirAll(o.FetchData, os.ModePerm)
		if err != nil {
			return err
		}

		dashboards := []string{}

		for _, dashboardCoordinate := range o.ToTestGridDashboardCoordinates() {
			dashboards = append(dashboards, dashboardCoordinate.TestGridDashboardNames...)
		}
		testgridhelpers.DownloadData(dashboards, o.JobFilter, o.FetchData)

		if o.DSN != "" && os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") != "" {
			bge, err := bigqueryexporter.New(context.Background())
			if err != nil {
				return err
			}

			dbc, err := db.New(o.DSN)
			if err != nil {
				return err
			}

			if err := bge.ExportData(context.Background(), dbc); err != nil {
				return err
			}
		} else {
			klog.V(1).Infof("")
		}

		// Fetch OpenShift PerfScale Data from ElasticSearch:
		if o.FetchPerfScaleData {
			scaleJobsDir := path.Join(o.FetchData, perfscaleanalysis.ScaleJobsSubDir)
			err := os.MkdirAll(scaleJobsDir, os.ModePerm)
			if err != nil {
				return err
			}
			err = perfscaleanalysis.DownloadPerfScaleData(scaleJobsDir)
			if err != nil {
				return err
			}
		}

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

	go func() {
		http.Handle("/metrics", promhttp.Handler())
		err := http.ListenAndServe(":2112", nil)
		if err != nil {
			panic(err)
		}
	}()

	// This embeds the contents of the two static directories directly into the binary. It
	// needs to be in main.go, so rice can find it when injecting the contents.
	sippyNG, err := rice.FindBox("./sippy-ng/build")
	if err != nil {
		panic(err)
	}

	static, err := rice.FindBox("./static")
	if err != nil {
		panic(err)
	}

	var dbc *db.DB
	if o.DSN != "" {
		dbc, err = db.New(o.DSN)
		if err != nil {
			return err
		}
	}

	server := sippyserver.NewServer(
		o.toTestGridLoadingConfig(),
		o.toRawJobResultsAnalysisConfig(),
		o.toDisplayDataConfig(),
		o.ToTestGridDashboardCoordinates(),
		o.ListenAddr,
		o.getSyntheticTestManager(),
		o.getVariantManager(),
		o.getBugCache(),
		sippyNG,
		static,
		dbc,
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

	testReport := analyzer.PrepareTestReport(o.ToTestGridDashboardCoordinates()[0], v1.CurrentReport, o.getSyntheticTestManager(), o.getVariantManager(), o.getBugCache())

	enc := json.NewEncoder(os.Stdout)
	return enc.Encode(testReport.ByTest)
}

func (o *Options) hasOCPDashboard() bool {
	for _, dashboardCoordinate := range o.ToTestGridDashboardCoordinates() {
		for _, dashboardName := range dashboardCoordinate.TestGridDashboardNames {
			if strings.Contains(dashboardName, "redhat-openshift-ocp-release-") {
				return true
			}
		}
	}
	return false
}

func (o *Options) getBugCache() buganalysis.BugCache {
	if o.SkipBugLookup || len(o.OpenshiftReleases) == 0 {
		return buganalysis.NewNoOpBugCache()
	}

	return buganalysis.NewBugCache()
}

func (o *Options) getVariantManager() testidentification.VariantManager {
	if len(o.Variants) == 0 {
		if o.hasOCPDashboard() {
			return testidentification.NewOpenshiftVariantManager()
		}
		return testidentification.NewEmptyVariantManager()
	}

	// TODO allow more than one with a union
	switch o.Variants[0] {
	case "ocp":
		return testidentification.NewOpenshiftVariantManager()
	case "kube":
		return testidentification.NewKubeVariantManager()
	case "none":
		return testidentification.NewEmptyVariantManager()
	default:
		panic("only ocp, kube, or none is allowed")
	}
}

func (o *Options) getSyntheticTestManager() testgridconversion.SyntheticTestManager {
	if o.hasOCPDashboard() {
		return testgridconversion.NewOpenshiftSyntheticTestManager()
	}

	return testgridconversion.NewEmptySyntheticTestManager()
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
