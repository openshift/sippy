package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/openshift/sippy/pkg/buganalysis"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/perfscaleanalysis"
	"github.com/openshift/sippy/pkg/prowloader"
	"github.com/openshift/sippy/pkg/prowloader/gcs"
	"github.com/openshift/sippy/pkg/releasesync"
	"github.com/openshift/sippy/pkg/sippyserver"
	"github.com/openshift/sippy/pkg/synthetictests"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridhelpers"
	"github.com/openshift/sippy/pkg/testidentification"
	"github.com/openshift/sippy/pkg/util/sets"
)

//go:embed sippy-ng/build
var sippyNG embed.FS

//go:embed static
var static embed.FS

const (
	defaultLogLevel = "info"
)

type Options struct {
	LocalData              string
	OpenshiftReleases      []string
	OpenshiftArchitectures []string
	Dashboards             []string
	// TODO perhaps this could drive the synthetic tests too
	Variants                           []string
	StartDay                           int
	NumDays                            int
	TestSuccessThreshold               float64
	JobFilter                          string
	MinTestRuns                        int
	Output                             string
	FailureClusterThreshold            int
	FetchData                          string
	FetchPerfScaleData                 bool
	InitDatabase                       bool
	LoadDatabase                       bool
	ListenAddr                         string
	Server                             bool
	DBOnlyMode                         bool
	SkipBugLookup                      bool
	DSN                                string
	LogLevel                           string
	LoadTestgrid                       bool
	LoadProw                           bool
	GoogleServiceAccountCredentialFile string
	GoogleOAuthClientCredentialFile    string
}

func main() {
	opt := &Options{
		NumDays:                 14,
		TestSuccessThreshold:    99.99,
		MinTestRuns:             10,
		Output:                  "json",
		FailureClusterThreshold: 10,
		StartDay:                0,
		ListenAddr:              ":8080",
	}

	cmd := &cobra.Command{
		Run: func(cmd *cobra.Command, arguments []string) {
			opt.Complete()

			if err := opt.Validate(); err != nil {
				log.WithError(err).Fatalf("error validation options")
			}
			if err := opt.Run(); err != nil {
				log.WithError(err).Fatalf("error running command")
			}
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&opt.LocalData, "local-data", opt.LocalData, "Path to testgrid data from local disk")
	flags.StringVar(&opt.DSN, "database-dsn", os.Getenv("SIPPY_DATABASE_DSN"), "Database DSN for storage of some types of data")
	flags.StringArrayVar(&opt.OpenshiftReleases, "release", opt.OpenshiftReleases, "Which releases to analyze (one per arg instance)")
	flags.StringArrayVar(&opt.OpenshiftArchitectures, "arch", opt.OpenshiftArchitectures, "Which architectures to analyze (one per arg instance)")
	flags.StringArrayVar(&opt.Dashboards, "dashboard", opt.Dashboards, "<display-name>=<comma-separated-list-of-dashboards>=<openshift-version>")
	flags.StringArrayVar(&opt.Variants, "variant", opt.Variants, "Variant manager to use: {ocp,kube,none}. Only useful when using --load-database.")
	flags.IntVar(&opt.StartDay, "start-day", opt.StartDay,
		"Most recent day to start processing testgrid results for (moving backward in time). (0 will start from now (default), -1 will start from whatever the most recent test results are) i.e. --start-day 30 --num-days 14 would load test grid results from 30 days ago back to 30+14=44 days ago.")
	flags.IntVar(&opt.NumDays, "num-days", opt.NumDays,
		"Number of days prior to --start-day to analyze testgrid results back to. (default 14 days) i.e. --start-day 30 --num-days 14 would load test grid results from 30 days ago back to 30+14=44 days ago.")
	flags.Float64Var(&opt.TestSuccessThreshold, "test-success-threshold", opt.TestSuccessThreshold, "Filter results for tests that are more than this percent successful")
	flags.StringVar(&opt.JobFilter, "job-filter", opt.JobFilter, "Only analyze jobs that match this regex")
	flags.StringVar(&opt.FetchData, "fetch-data", opt.FetchData, "Download testgrid data to directory specified for future use with --local-data")
	flags.BoolVar(&opt.LoadDatabase, "load-database", opt.LoadDatabase, "Process testgrid data in --local-data and store in database")
	flags.BoolVar(&opt.InitDatabase, "init-database", opt.InitDatabase, "Initialize postgresql database tables and materialized views")
	flags.BoolVar(&opt.FetchPerfScaleData, "fetch-openshift-perfscale-data", opt.FetchPerfScaleData, "Download ElasticSearch data for workload CPU/memory use from jobs run by the OpenShift perfscale team. Will be stored in 'perfscale-metrics/' subdirectory beneath the --fetch-data dir.")
	flags.IntVar(&opt.MinTestRuns, "min-test-runs", opt.MinTestRuns, "Ignore tests with less than this number of runs")
	flags.IntVar(&opt.FailureClusterThreshold, "failure-cluster-threshold", opt.FailureClusterThreshold, "Include separate report on job runs with more than N test failures, -1 to disable")
	flags.StringVarP(&opt.Output, "output", "o", opt.Output, "Output format for report: json, text")
	flag.StringVar(&opt.ListenAddr, "listen", opt.ListenAddr, "The address to serve analysis reports on")
	flags.BoolVar(&opt.Server, "server", opt.Server, "Run in web server mode (serve reports over http)")
	flags.BoolVar(&opt.DBOnlyMode, "db-only-mode", true, "OBSOLETE, this is now the default. Will soon be removed.")
	flags.BoolVar(&opt.SkipBugLookup, "skip-bug-lookup", opt.SkipBugLookup, "Do not attempt to find bugs that match test/job failures")
	flags.StringVar(&opt.LogLevel, "log-level", defaultLogLevel, "Log level (trace,debug,info,warn,error)")
	flags.BoolVar(&opt.LoadTestgrid, "load-testgrid", true, "Fetch job and job run data from testgrid")
	flags.BoolVar(&opt.LoadProw, "load-prow", opt.LoadProw, "Fetch job and job run data from prow")

	// google cloud creds
	flags.StringVar(&opt.GoogleServiceAccountCredentialFile, "google-service-account-credential-file", os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"), "location of a credential file described by https://cloud.google.com/docs/authentication/production")
	flags.StringVar(&opt.GoogleOAuthClientCredentialFile, "google-oauth-credential-file", opt.GoogleOAuthClientCredentialFile, "location of a credential file described by https://developers.google.com/people/quickstart/go, setup from https://cloud.google.com/bigquery/docs/authentication/end-user-installed#client-credentials")

	if err := cmd.Execute(); err != nil {
		log.Fatalf("error: %v", err)
	}
}

func (o *Options) Complete() {
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

	if o.Server && o.FetchData != "" {
		return fmt.Errorf("cannot specify --server with --fetch-data")
	}

	if o.Server && o.LoadDatabase {
		return fmt.Errorf("cannot specify --server with --load-database")
	}

	if o.LoadDatabase && o.FetchData != "" {
		return fmt.Errorf("cannot specify --load-database with --fetch-data")
	}

	if o.LoadDatabase && o.LocalData == "" && o.LoadTestgrid {
		return fmt.Errorf("must specify --local-data with --load-database for loading testgrid data")
	}

	if (o.LoadDatabase || o.Server) && o.DSN == "" {
		return fmt.Errorf("must specify --database-dsn with --load-database and --server")
	}

	if !o.DBOnlyMode {
		return fmt.Errorf("--db-only-mode cannot be set to false (deprecated flag soon to be removed, feature now mandatory)")
	}

	return nil
}

func (o *Options) Run() error {
	// Set log level
	level, err := log.ParseLevel(o.LogLevel)
	if err != nil {
		log.WithError(err).Fatal("Cannot parse log level")
	}
	log.SetLevel(level)

	// Add some millisecond precision to log timestamps, useful for debugging performance.
	formatter := new(log.TextFormatter)
	formatter.TimestampFormat = "2006-01-02T15:04:05.999Z07:00"
	formatter.FullTimestamp = true
	formatter.DisableColors = false
	log.SetFormatter(formatter)

	log.Debug("debug logging enabled")
	if o.FetchData != "" {
		start := time.Now()
		err := os.MkdirAll(o.FetchData, os.ModePerm)
		if err != nil {
			return err
		}

		dashboards := []string{}

		for _, dashboardCoordinate := range o.ToTestGridDashboardCoordinates() {
			dashboards = append(dashboards, dashboardCoordinate.TestGridDashboardNames...)
		}
		testgridhelpers.DownloadData(dashboards, o.JobFilter, o.FetchData)

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

		elapsed := time.Since(start)
		log.Infof("Testgrid data fetched in: %s", elapsed)

		return nil
	}

	if o.InitDatabase {
		_, err := db.New(o.DSN)
		return err
	}

	if o.LoadDatabase {
		dbc, err := db.New(o.DSN)
		if err != nil {
			return err
		}

		start := time.Now()
		if o.LoadTestgrid {

			trgc := sippyserver.TestReportGeneratorConfig{
				TestGridLoadingConfig:       o.toTestGridLoadingConfig(),
				RawJobResultsAnalysisConfig: o.toRawJobResultsAnalysisConfig(),
				DisplayDataConfig:           o.toDisplayDataConfig(),
			}

			loadBugs := !o.SkipBugLookup && len(o.OpenshiftReleases) > 0
			for _, dashboard := range o.ToTestGridDashboardCoordinates() {
				err := trgc.LoadDatabase(dbc, dashboard, o.getVariantManager(), o.getSyntheticTestManager(),
					o.StartDay, o.NumDays)
				if err != nil {
					log.WithError(err).Error("error loading database")
					return err
				}
			}

			if loadBugs {
				testCache, err := sippyserver.LoadTestCache(dbc)
				if err != nil {
					return err
				}
				prowJobCache, err := sippyserver.LoadProwJobCache(dbc)
				if err != nil {
					return err
				}
				if err := sippyserver.LoadBugs(dbc, o.getBugCache(), testCache, prowJobCache); err != nil {
					return errors.Wrapf(err, "error syncing bugzilla bugs to db")
				}
			}
		}

		loadReleases := len(o.OpenshiftReleases) > 0
		if loadReleases {
			releaseStreams := make([]string, 0)
			for _, release := range o.OpenshiftReleases {
				for _, stream := range []string{"nightly", "ci"} {
					releaseStreams = append(releaseStreams, fmt.Sprintf("%s.0-0.%s", release, stream))
				}
			}

			if err := releasesync.Import(dbc, releaseStreams, o.OpenshiftArchitectures); err != nil {
				panic(err)
			}
		}

		if o.LoadProw {
			gcsClient, err := gcs.NewGCSClient(context.TODO(),
				o.GoogleServiceAccountCredentialFile,
				o.GoogleOAuthClientCredentialFile,
			)
			if err != nil {
				return err
			}

			prowLoader := prowloader.New(dbc, gcsClient, "origin-ci-test", o.getVariantManager())

			// For now let's just get master/main presubmits in the openshift org
			allowedJobRegex := []*regexp.Regexp{
				regexp.MustCompile(`pull-ci-openshift-.*-(master|main)-e2e-.*`),
			}

			err = prowLoader.LoadProwJobsToDB(allowedJobRegex)
			if err != nil {
				return err
			}
		}

		elapsed := time.Since(start)
		log.Infof("Database loaded in: %s", elapsed)

		return err
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

	var dbc *db.DB
	var err error
	if o.DSN != "" {
		dbc, err = db.New(o.DSN)
		if err != nil {
			return err
		}
	}

	webRoot, err := fs.Sub(sippyNG, "sippy-ng/build")
	if err != nil {
		return err
	}

	server := sippyserver.NewServer(
		o.getServerMode(),
		o.toTestGridLoadingConfig(),
		o.toRawJobResultsAnalysisConfig(),
		o.toDisplayDataConfig(),
		o.ToTestGridDashboardCoordinates(),
		o.ListenAddr,
		o.getSyntheticTestManager(),
		o.getVariantManager(),
		o.getBugCache(),
		webRoot,
		&static,
		dbc,
	)

	// Initial metrics refresh to get the endpoint scrapable ASAP and prevent prom gaps, before
	// we start the lengthy mat view refreshes.
	if err := server.RefreshMetricsDB(); err != nil {
		log.WithError(err).Error("error refreshing metrics")
	}

	// force a data refresh in the background. This is important to initially populate the db's materialized views
	// if this is the first time starting sippy.
	go server.RefreshData(true)

	server.Serve()
	return nil
}

func (o *Options) getServerMode() sippyserver.Mode {
	for _, dashboardCoordinate := range o.ToTestGridDashboardCoordinates() {
		for _, dashboardName := range dashboardCoordinate.TestGridDashboardNames {
			if strings.Contains(dashboardName, "redhat-openshift-ocp-release-") {
				return sippyserver.ModeOpenShift
			}
		}
	}
	return sippyserver.ModeKubernetes
}

func (o *Options) getBugCache() buganalysis.BugCache {
	if o.SkipBugLookup || len(o.OpenshiftReleases) == 0 {
		return buganalysis.NewNoOpBugCache()
	}

	return buganalysis.NewBugCache()
}

func (o *Options) getVariantManager() testidentification.VariantManager {
	if len(o.Variants) == 0 {
		if o.getServerMode() == sippyserver.ModeOpenShift {
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

func (o *Options) getSyntheticTestManager() synthetictests.SyntheticTestManager {
	if o.getServerMode() == sippyserver.ModeOpenShift {
		return synthetictests.NewOpenshiftSyntheticTestManager()
	}

	return synthetictests.NewEmptySyntheticTestManager()
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
