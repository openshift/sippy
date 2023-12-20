package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"google.golang.org/api/option"
	"gopkg.in/yaml.v3"
	gormlogger "gorm.io/gorm/logger"

	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"

	"github.com/openshift/sippy/pkg/apis/cache"
	v1 "github.com/openshift/sippy/pkg/apis/config/v1"
	"github.com/openshift/sippy/pkg/cache/redis"
	"github.com/openshift/sippy/pkg/dataloader"
	"github.com/openshift/sippy/pkg/dataloader/bugloader"
	"github.com/openshift/sippy/pkg/dataloader/jiraloader"
	"github.com/openshift/sippy/pkg/dataloader/loaderwithmetrics"
	"github.com/openshift/sippy/pkg/dataloader/prowloader"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/gcs"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/github"
	"github.com/openshift/sippy/pkg/dataloader/releaseloader"
	"github.com/openshift/sippy/pkg/dataloader/testownershiploader"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/github/commenter"
	"github.com/openshift/sippy/pkg/sippyserver"
	"github.com/openshift/sippy/pkg/sippyserver/metrics"
	"github.com/openshift/sippy/pkg/snapshot"
	"github.com/openshift/sippy/pkg/synthetictests"
	"github.com/openshift/sippy/pkg/testidentification"
	"github.com/openshift/sippy/pkg/util"
	"github.com/openshift/sippy/pkg/util/sets"
)

//go:embed sippy-ng/build
var sippyNG embed.FS

//go:embed static
var static embed.FS

const (
	defaultLogLevel                = "info"
	defaultDBLogLevel              = "warn"
	commentProcessingDryRunDefault = true
)

// DefaultOpenshiftGCSBucket is the Google cloud storage bucket that will be used if none is provided as a CLI argument.
const DefaultOpenshiftGCSBucket = "test-platform-results"

type Options struct {
	// LocalData is a directory used for storing testgrid data, and then loading into database.
	// Not required when loading db from prow, or when running the server.
	LocalData              string
	OpenshiftReleases      []string
	OpenshiftArchitectures []string
	Dashboards             []string
	// TODO perhaps this could drive the synthetic tests too
	Mode                               []string
	StartDay                           int
	NumDays                            int
	TestSuccessThreshold               float64
	JobFilter                          string
	MinTestRuns                        int
	Output                             string
	FailureClusterThreshold            int
	InitDatabase                       bool
	LoadDatabase                       bool
	ListenAddr                         string
	MetricsAddr                        string
	Server                             bool
	DBOnlyMode                         bool
	SkipBugLookup                      bool
	DSN                                string
	LogLevel                           string
	DBLogLevel                         string
	LoadTestgrid                       bool
	LoadOpenShiftCIBigQuery            bool
	LoadProw                           bool
	LoadGitHub                         bool
	Config                             string
	GoogleServiceAccountCredentialFile string
	GoogleOAuthClientCredentialFile    string
	GoogleStorageBucket                string
	PinnedDateTime                     string
	RedisURL                           string

	DaemonServer            bool
	CommentProcessing       bool
	CommentProcessingDryRun bool
	ExcludeReposCommenting  []string
	IncludeReposCommenting  []string

	CreateSnapshot  bool
	SippyURL        string
	SnapshotName    string
	SnapshotRelease string
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
		MetricsAddr:             ":2112",
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
	flags.StringArrayVar(&opt.Mode, "mode", opt.Mode, "Mode to use: {ocp,kube,none}. Only useful when using --load-database.")
	flags.IntVar(&opt.StartDay, "start-day", opt.StartDay,
		"Most recent day to start processing testgrid results for (moving backward in time). (0 will start from now (default), -1 will start from whatever the most recent test results are) i.e. --start-day 30 --num-days 14 would load test grid results from 30 days ago back to 30+14=44 days ago.")
	flags.IntVar(&opt.NumDays, "num-days", opt.NumDays,
		"Number of days prior to --start-day to analyze testgrid results back to. (default 14 days) i.e. --start-day 30 --num-days 14 would load test grid results from 30 days ago back to 30+14=44 days ago.")
	flags.Float64Var(&opt.TestSuccessThreshold, "test-success-threshold", opt.TestSuccessThreshold, "Filter results for tests that are more than this percent successful")
	flags.StringVar(&opt.JobFilter, "job-filter", opt.JobFilter, "Only analyze jobs that match this regex")
	flags.BoolVar(&opt.LoadDatabase, "load-database", opt.LoadDatabase, "Process testgrid data in --local-data and store in database")
	flags.BoolVar(&opt.InitDatabase, "init-database", opt.InitDatabase, "Initialize postgresql database tables and materialized views")
	flags.IntVar(&opt.MinTestRuns, "min-test-runs", opt.MinTestRuns, "Ignore tests with less than this number of runs")
	flags.IntVar(&opt.FailureClusterThreshold, "failure-cluster-threshold", opt.FailureClusterThreshold, "Include separate report on job runs with more than N test failures, -1 to disable")
	flags.StringVarP(&opt.Output, "output", "o", opt.Output, "Output format for report: json, text")
	flags.StringVar(&opt.ListenAddr, "listen", opt.ListenAddr, "The address to serve analysis reports on (default :8080)")
	flags.StringVar(&opt.MetricsAddr, "listen-metrics", opt.MetricsAddr, "The address to serve prometheus metrics on (default :2112). If blank, metrics won't be served.")
	flags.BoolVar(&opt.Server, "server", opt.Server, "Run in web server mode (serve reports over http)")
	flags.BoolVar(&opt.DBOnlyMode, "db-only-mode", true, "OBSOLETE, this is now the default. Will soon be removed.")
	flags.BoolVar(&opt.SkipBugLookup, "skip-bug-lookup", opt.SkipBugLookup, "Do not attempt to find bugs that match test/job failures")
	flags.StringVar(&opt.LogLevel, "log-level", defaultLogLevel, "Log level (trace,debug,info,warn,error) (default info)")
	flags.StringVar(&opt.DBLogLevel, "db-log-level", defaultDBLogLevel, "gorm database log level (info,warn,error,silent) (default warn)")
	flags.BoolVar(&opt.LoadTestgrid, "load-testgrid", true, "Fetch job and job run data from testgrid")
	flags.BoolVar(&opt.LoadOpenShiftCIBigQuery, "load-openshift-ci-bigquery", false, "Load ProwJobs from OpenShift CI BigQuery")

	// Snapshotter setup, should be a sub-command someday:
	flags.BoolVar(&opt.CreateSnapshot, "create-snapshot", false, "Create snapshots using current sippy overview API json and store in db")
	flags.StringVar(&opt.SippyURL, "snapshot-sippy-url", "https://sippy.dptools.openshift.org", "Sippy endpoint to hit when creating a snapshot")
	flags.StringVar(&opt.SnapshotName, "snapshot-name", "", "Snapshot name (i.e. 4.12 GA)")
	flags.StringVar(&opt.SnapshotRelease, "snapshot-release", "", "Snapshot release (i.e. 4.12)")

	flags.BoolVar(&opt.LoadProw, "load-prow", opt.LoadProw, "Fetch job and job run data from prow")
	flags.BoolVar(&opt.LoadGitHub, "load-github", opt.LoadGitHub, "Fetch PR state data from GitHub, only for use with Prow-based Sippy")
	flags.StringVar(&opt.Config, "config", opt.Config, "Configuration file for Sippy, required if using Prow-based Sippy")

	// google cloud creds
	flags.StringVar(&opt.GoogleServiceAccountCredentialFile, "google-service-account-credential-file", os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"), "location of a credential file described by https://cloud.google.com/docs/authentication/production")
	flags.StringVar(&opt.GoogleOAuthClientCredentialFile, "google-oauth-credential-file", opt.GoogleOAuthClientCredentialFile, "location of a credential file described by https://developers.google.com/people/quickstart/go, setup from https://cloud.google.com/bigquery/docs/authentication/end-user-installed#client-credentials")

	// Google storage bucket to pull CI data from
	flags.StringVar(&opt.GoogleStorageBucket, "google-storage-bucket", DefaultOpenshiftGCSBucket, "GCS bucket to pull artifacts from")

	// caching
	flags.StringVar(&opt.RedisURL, "redis-url", os.Getenv("REDIS_URL"), "Redis caching server URL")

	flags.StringVar(&opt.PinnedDateTime, "pinnedDateTime", opt.PinnedDateTime, "optional value to use in a historical context with a fixed date / time value specified in RFC3339 format - 2006-01-02T15:04:05+00:00")

	flags.BoolVar(&opt.DaemonServer, "daemon-server", opt.DaemonServer, "Run in daemon server mode (background work processing)")
	// which ones do we include, this is likely temporary as we roll this out and may go away
	flags.StringArrayVar(&opt.IncludeReposCommenting, "include-repo-commenting", opt.IncludeReposCommenting, "Which repos do we include for pr commenting (one repo per arg instance  org/repo or just repo if openshift org)")
	flags.StringArrayVar(&opt.ExcludeReposCommenting, "exclude-repo-commenting", opt.ExcludeReposCommenting, "Which repos do we skip for pr commenting (one repo per arg instance  org/repo or just repo if openshift org)")
	flags.BoolVar(&opt.CommentProcessing, "comment-processing", opt.CommentProcessing, "Enable comment processing for github repos")
	flags.BoolVar(&opt.CommentProcessingDryRun, "comment-processing-dry-run", commentProcessingDryRunDefault, "Enable github comment interaction for comment processing, disabled by default")

	if err := cmd.Execute(); err != nil {
		log.Fatalf("error: %v", err)
	}
}

func (o *Options) Complete() {
	for _, openshiftRelease := range o.OpenshiftReleases {
		o.Dashboards = append(o.Dashboards, dashboardArgFromOpenshiftRelease(openshiftRelease))
	}
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

// nolint:gocyclo
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

	if len(o.Mode) > 1 {
		return fmt.Errorf("only one --mode allowed")
	} else if len(o.Mode) == 1 {
		if !sets.NewString("ocp", "kube", "none").Has(o.Mode[0]) {
			return fmt.Errorf("only ocp, kube, or none is allowed")
		}
	}

	if o.Server && o.LoadDatabase {
		return fmt.Errorf("cannot specify --server with --load-database")
	}

	if (o.LoadDatabase || o.Server || o.CreateSnapshot || o.CommentProcessing) && o.DSN == "" {
		return fmt.Errorf("must specify --database-dsn with --load-database, --server, --comment-processing, and --create-snapshot")
	}

	if o.LoadGitHub && !o.LoadProw {
		return fmt.Errorf("--load-github can only be specified with --load-prow")
	}

	if o.LoadProw && o.Config == "" {
		return fmt.Errorf("must specify --config with --load-prow")
	}

	if o.Server && o.DaemonServer {
		return fmt.Errorf("cannot specify --server with --daemon-server")
	}

	if o.DaemonServer && o.LoadDatabase {
		return fmt.Errorf("cannot specify --daemon-server with --load-database")
	}

	if !o.DaemonServer && o.CommentProcessing {
		return fmt.Errorf("cannot specify --comment-processing without --daemon-server")
	}

	// thought here is we may add other daemon process support and
	// not have a direct dependency on comment-processing
	if o.DaemonServer && !o.CommentProcessing {
		return fmt.Errorf("cannot specify --daemon-server without specifying a daemon-process as well (e.g. --comment-processing)")
	}

	if o.CommentProcessing && o.Config == "" {
		return fmt.Errorf("must specify --config with --comment-processing")
	}

	if o.CreateSnapshot {
		if o.SnapshotName == "" {
			return fmt.Errorf("must specify --snapshot-name when creating a snapshot")
		}
		if o.SnapshotRelease == "" {
			return fmt.Errorf("must specify --snapshot-release when creating a snapshot")
		}
	}

	if !o.DBOnlyMode {
		return fmt.Errorf("--db-only-mode cannot be set to false (deprecated flag soon to be removed, feature now mandatory)")
	}

	return nil
}

func (o *Options) Run() error { //nolint:gocyclo
	loaders := make([]dataloader.DataLoader, 0)

	// Set log level
	level, err := log.ParseLevel(o.LogLevel)
	if err != nil {
		log.WithError(err).Fatal("Cannot parse log-level")
	}
	log.SetLevel(level)

	gormLogLevel, err := db.ParseGormLogLevel(o.DBLogLevel)
	if err != nil {
		log.WithError(err).Fatal("Cannot parse db-log-level")
	}

	// Add some millisecond precision to log timestamps, useful for debugging performance.
	formatter := new(log.TextFormatter)
	formatter.TimestampFormat = "2006-01-02T15:04:05.999Z07:00"
	formatter.FullTimestamp = true
	formatter.DisableColors = false
	log.SetFormatter(formatter)

	log.Debug("debug logging enabled")
	sippyConfig := v1.SippyConfig{}
	if o.Config == "" {
		sippyConfig.Prow = v1.ProwConfig{
			URL: "https://prow.ci.openshift.org/prowjobs.js",
		}
	} else {
		data, err := os.ReadFile(o.Config)
		if err != nil {
			log.WithError(err).Fatalf("could not load config")
		}
		if err := yaml.Unmarshal(data, &sippyConfig); err != nil {
			log.WithError(err).Fatalf("could not unmarshal config")
		}
	}

	var pinnedTime *time.Time

	if len(o.PinnedDateTime) > 0 {
		parsedTime, err := time.Parse(time.RFC3339, o.PinnedDateTime)

		if err != nil {
			log.WithError(err).Fatal("Error parsing pinnedDateTime")
		} else {
			log.Infof("Set time now to %s", parsedTime)
		}

		// we made it here so parsedTime represents the pinnedTime and there was no error parsing
		pinnedTime = &parsedTime
	}

	if o.CreateSnapshot {
		dbc, err := db.New(o.DSN, gormLogLevel)
		if err != nil {
			return err
		}

		snapshotter := &snapshot.Snapshotter{
			DBC:      dbc,
			SippyURL: o.SippyURL,
			Name:     o.SnapshotName,
			Release:  o.SnapshotRelease,
		}

		return snapshotter.Create()
	}

	if o.InitDatabase {
		dbc, err := db.New(o.DSN, gormLogLevel)
		if err != nil {
			return err
		}
		if err := dbc.UpdateSchema(pinnedTime); err != nil {
			return err
		}
	}

	if o.LoadDatabase {
		// Cancel syncing after 4 hours
		ctx, cancel := context.WithTimeout(context.Background(), time.Hour*4)
		defer cancel()

		dbc, err := db.New(o.DSN, gormLogLevel)
		if err != nil {
			return err
		}

		start := time.Now()

		// Track all errors we encountered during the update. We'll log each again at the end, and return
		// an overall error to exit non-zero and fail the job.
		allErrs := []error{}

		// Release payload tag loader
		if len(o.OpenshiftReleases) > 0 {
			loaders = append(loaders, releaseloader.New(dbc, o.OpenshiftReleases, o.OpenshiftArchitectures))
		}

		// Prow Loader
		if o.LoadProw {
			prowLoader, err := o.prowLoader(ctx, dbc, sippyConfig)
			if err != nil {
				return err
			}
			loaders = append(loaders, prowLoader)
		}

		// JIRA Loader
		loaders = append(loaders, jiraloader.New(dbc))

		// Load maping for jira components to tests
		if o.LoadOpenShiftCIBigQuery {
			cl, err := testownershiploader.New(ctx, dbc, o.GoogleServiceAccountCredentialFile, o.GoogleOAuthClientCredentialFile)
			if err != nil {
				log.WithError(err).Warningf("failed to create component loader")
			} else {
				loaders = append(loaders, cl)
			}
		}

		// Bug Loader
		loadBugs := !o.SkipBugLookup && len(o.OpenshiftReleases) > 0
		if loadBugs {
			loaders = append(loaders, bugloader.New(dbc))
		}

		// Run loaders with the metrics wrapper
		l := loaderwithmetrics.New(loaders)
		l.Load()
		if len(l.Errors()) > 0 {
			allErrs = append(allErrs, l.Errors()...)
		}

		elapsed := time.Since(start)
		log.WithField("elapsed", elapsed).Info("database load complete")

		sippyserver.RefreshData(dbc, pinnedTime, false)

		if len(allErrs) > 0 {
			log.Warningf("%d errors were encountered while loading database:", len(allErrs))
			for _, err := range allErrs {
				log.Error(err.Error())
			}
			return fmt.Errorf("errors were encountered while loading database, see logs for details")
		}
		log.Info("no errors encountered during db refresh")
		return nil
	}

	// if the daemon server is enabled
	// then the regular server is not
	// so return nil when done
	if o.DaemonServer {
		processes := make([]sippyserver.DaemonProcess, 0)

		if o.CommentProcessing {
			dbc, err := db.New(o.DSN, gormLogLevel)
			if err != nil {
				return err
			}

			githubClient := github.New(context.TODO())
			ghCommenter, err := commenter.NewGitHubCommenter(githubClient, dbc, o.ExcludeReposCommenting, o.IncludeReposCommenting)

			if err != nil {
				log.WithError(err).Error("CRITICAL error initializing GitHub commenter which prevents PR commenting")
				return nil
			}

			gcsClient, err := gcs.NewGCSClient(context.TODO(),
				o.GoogleServiceAccountCredentialFile,
				o.GoogleOAuthClientCredentialFile,
			)
			if err != nil {
				log.WithError(err).Error("CRITICAL error getting GCS client which prevents PR commenting")
				return nil
			}

			// we only process one comment every 5 seconds,
			// 4 potential GitHub calls per comment gives us a safe buffer
			// get comment data, get existing comments, possible delete existing, and adding the comment
			// could  lower to 3 seconds if we need, most writes likely won't have to delete
			processes = append(processes, sippyserver.NewWorkProcessor(dbc, gcsClient.Bucket("origin-ci-test"), 10, 5*time.Minute, 5*time.Second, ghCommenter, o.CommentProcessingDryRun))

		}
		o.runDaemonServer(processes)
		return nil
	}

	if o.Server {
		return o.runServerMode(pinnedTime, gormLogLevel)
	}

	return nil
}

func (o *Options) runDaemonServer(processes []sippyserver.DaemonProcess) {

	daemonServer := sippyserver.NewDaemonServer(processes)

	// Serve our metrics endpoint for prometheus to scrape
	if o.MetricsAddr != "" {
		go func() {
			http.Handle("/metrics", promhttp.Handler())
			err := http.ListenAndServe(o.MetricsAddr, nil) //nolint
			if err != nil {
				panic(err)
			}
		}()
	}

	daemonServer.Serve()
}

func (o *Options) prowLoader(ctx context.Context, dbc *db.DB, sippyConfig v1.SippyConfig) (dataloader.DataLoader, error) {
	gcsClient, err := gcs.NewGCSClient(ctx,
		o.GoogleServiceAccountCredentialFile,
		o.GoogleOAuthClientCredentialFile,
	)
	if err != nil {
		log.WithError(err).Error("CRITICAL error getting GCS client which prevents importing prow jobs")
		return nil, err
	}

	var bigQueryClient *bigquery.Client
	if o.LoadOpenShiftCIBigQuery {
		bigQueryClient, err = bigquery.NewClient(ctx, "openshift-gce-devel",
			option.WithCredentialsFile(o.GoogleServiceAccountCredentialFile))
		if err != nil {
			log.WithError(err).Error("CRITICAL error getting BigQuery client which prevents importing prow jobs")
			return nil, err
		}
	}

	var githubClient *github.Client
	if o.LoadGitHub {
		githubClient = github.New(ctx)
	}

	ghCommenter, err := commenter.NewGitHubCommenter(githubClient, dbc, o.ExcludeReposCommenting, o.IncludeReposCommenting)
	if err != nil {
		log.WithError(err).Error("CRITICAL error initializing GitHub commenter which prevents importing prow jobs")
		return nil, err
	}

	return prowloader.New(
		ctx,
		dbc,
		gcsClient,
		bigQueryClient,
		o.GoogleStorageBucket,
		githubClient,
		o.getVariantManager(),
		o.getSyntheticTestManager(),
		o.OpenshiftReleases,
		&sippyConfig,
		ghCommenter), nil
}

func (o *Options) runServerMode(pinnedDateTime *time.Time, gormLogLevel gormlogger.LogLevel) error {
	var dbc *db.DB
	var err error
	if o.DSN != "" {
		dbc, err = db.New(o.DSN, gormLogLevel)
		if err != nil {
			return err
		}
	}

	// Make sure the db is intialized, otherwise let the user know:
	prowJobs := []models.ProwJob{}
	res := dbc.DB.Find(&prowJobs).Limit(1)
	if res.Error != nil {
		log.WithError(res.Error).Fatal("error querying for a ProwJob, database may need to be initialized with --init-database")
	}

	webRoot, err := fs.Sub(sippyNG, "sippy-ng/build")
	if err != nil {
		return err
	}

	gcsClient, err := gcs.NewGCSClient(context.TODO(),
		o.GoogleServiceAccountCredentialFile,
		o.GoogleOAuthClientCredentialFile,
	)
	if err != nil {
		log.WithError(err).Warn("unable to create GCS client, some APIs may not work")
	}

	var bigQueryClient bqcachedclient.Client
	if o.GoogleServiceAccountCredentialFile != "" {
		bigQueryClient.BQ, err = bigquery.NewClient(context.Background(), "openshift-gce-devel",
			option.WithCredentialsFile(o.GoogleServiceAccountCredentialFile))
		if err != nil {
			log.WithError(err).Error("CRITICAL error getting BigQuery client which prevents component readiness queries from working")
			return err
		}
		// Enable Storage API usage for fetching data
		err = bigQueryClient.BQ.EnableStorageReadClient(context.Background(), option.WithCredentialsFile(o.GoogleServiceAccountCredentialFile))
		if err != nil {
			log.WithError(err).Error("CRITICAL error enabling BigQuery Storage API")
			return err
		}
	}

	var cacheClient cache.Cache
	if o.RedisURL != "" {
		cacheClient, err = redis.NewRedisCache(o.RedisURL)
		if err != nil {
			log.WithError(err).Error("couldn't create redis cache")
			return err
		}
		bigQueryClient.Cache = cacheClient
	}

	server := sippyserver.NewServer(
		o.getServerMode(),
		o.ListenAddr,
		o.getSyntheticTestManager(),
		o.getVariantManager(),
		webRoot,
		&static,
		dbc,
		o.GoogleStorageBucket,
		gcsClient,
		&bigQueryClient,
		pinnedDateTime,
		cacheClient,
	)

	if o.MetricsAddr != "" {
		// Do an immediate metrics update
		err = metrics.RefreshMetricsDB(dbc, &bigQueryClient, o.GoogleStorageBucket, o.getVariantManager(), util.GetReportEnd(pinnedDateTime))
		if err != nil {
			log.WithError(err).Error("error refreshing metrics")
		}

		// Refresh our metrics every 5 minutes:
		ticker := time.NewTicker(5 * time.Minute)
		quit := make(chan struct{})
		go func() {
			for {
				select {
				case <-ticker.C:
					log.Info("tick")
					err := metrics.RefreshMetricsDB(dbc, &bigQueryClient, o.GoogleStorageBucket, o.getVariantManager(), util.GetReportEnd(pinnedDateTime))
					if err != nil {
						log.WithError(err).Error("error refreshing metrics")
					}
				case <-quit:
					ticker.Stop()
					return
				}
			}
		}()

		// Serve our metrics endpoint for prometheus to scrape
		go func() {
			http.Handle("/metrics", promhttp.Handler())
			err := http.ListenAndServe(o.MetricsAddr, nil) //nolint
			if err != nil {
				panic(err)
			}
		}()
	}

	server.Serve()
	return nil
}

func (o *Options) getServerMode() sippyserver.Mode {
	if len(o.Mode) >= 1 && o.Mode[0] == "ocp" {
		return sippyserver.ModeOpenShift
	}
	return sippyserver.ModeKubernetes
}

func (o *Options) getVariantManager() testidentification.VariantManager {
	if len(o.Mode) == 0 {
		if o.getServerMode() == sippyserver.ModeOpenShift {
			return testidentification.NewOpenshiftVariantManager()
		}
		return testidentification.NewEmptyVariantManager()
	}

	// TODO allow more than one with a union
	switch o.Mode[0] {
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
