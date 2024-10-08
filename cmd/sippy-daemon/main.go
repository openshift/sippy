package main

import (
	"context"
	"net/http"
	"time"

	"github.com/openshift/sippy/pkg/api"
	"github.com/openshift/sippy/pkg/apis/cache"
	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/dataloader/regressionloader"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/openshift/sippy/pkg/dataloader/prowloader/gcs"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/github"
	"github.com/openshift/sippy/pkg/flags"
	"github.com/openshift/sippy/pkg/github/commenter"
	"github.com/openshift/sippy/pkg/sippyserver"
)

var logLevel = "info"

type SippyDaemonFlags struct {
	BigQueryFlags    *flags.BigQueryFlags
	DBFlags          *flags.PostgresFlags
	GoogleCloudFlags *flags.GoogleCloudFlags
	CacheFlags       *flags.CacheFlags

	GithubCommenterFlags    *flags.GithubCommenterFlags
	ComponentReadinessFlags *flags.ComponentReadinessFlags
	MetricsAddr             string
}

func NewSippyDaemonFlags() *SippyDaemonFlags {
	return &SippyDaemonFlags{
		BigQueryFlags:           flags.NewBigQueryFlags(),
		DBFlags:                 flags.NewPostgresDatabaseFlags(),
		GithubCommenterFlags:    flags.NewGithubCommenterFlags(),
		GoogleCloudFlags:        flags.NewGoogleCloudFlags(),
		CacheFlags:              flags.NewCacheFlags(),
		ComponentReadinessFlags: flags.NewComponentReadinessFlags(),
	}
}

func (f *SippyDaemonFlags) BindFlags(fs *pflag.FlagSet) {
	f.BigQueryFlags.BindFlags(fs)
	f.DBFlags.BindFlags(fs)
	f.GithubCommenterFlags.BindFlags(fs)
	f.GoogleCloudFlags.BindFlags(fs)
	f.CacheFlags.BindFlags(fs)
	f.ComponentReadinessFlags.BindFlags(fs)

	fs.StringVar(&f.MetricsAddr, "listen-metrics", f.MetricsAddr, "The address to serve prometheus metrics on (default :2112)")
}

func NewSippyDaemonCommand() *cobra.Command {
	f := NewSippyDaemonFlags()

	// rootCmd represents the base command when called without any subcommands
	cmd := &cobra.Command{
		Use:   "sippy-daemon",
		Short: "Sippy daemon is used for on-going tasks like monitoring git repos for reporting risk analysis.",
		RunE: func(cmd *cobra.Command, args []string) error {

			processes := make([]sippyserver.DaemonProcess, 0)

			if f.GithubCommenterFlags.CommentProcessing {
				dbc, err := f.DBFlags.GetDBClient()
				if err != nil {
					return err
				}

				githubClient := github.New(context.TODO())
				ghCommenter, err := commenter.NewGitHubCommenter(githubClient,
					dbc, f.GithubCommenterFlags.ExcludeReposCommenting, f.GithubCommenterFlags.IncludeReposCommenting)
				if err != nil {
					log.WithError(err).Error("CRITICAL error initializing GitHub commenter which prevents PR commenting")
					return nil
				}

				gcsClient, err := gcs.NewGCSClient(context.TODO(),
					f.GoogleCloudFlags.ServiceAccountCredentialFile,
					f.GoogleCloudFlags.OAuthClientCredentialFile,
				)
				if err != nil {
					log.WithError(err).Error("CRITICAL error getting GCS client which prevents PR commenting")
					return nil
				}

				// we only process one comment every 5 seconds,
				// 4 potential GitHub calls per comment gives us a safe buffer
				// get comment data, get existing comments, possible delete existing, and adding the comment
				// could  lower to 3 seconds if we need, most writes likely won't have to delete
				processes = append(processes, sippyserver.NewWorkProcessor(dbc,
					gcsClient.Bucket(f.GoogleCloudFlags.StorageBucket),
					10, 5*time.Minute, 5*time.Second, ghCommenter, f.GithubCommenterFlags.CommentProcessingDryRun))
			}

			if f.ComponentReadinessFlags.ComponentReadinessViewsFile != "" {
				cacheClient, err := f.CacheFlags.GetCacheClient()
				if err != nil {
					log.WithError(err).Fatal("couldn't get cache client")
				}

				bigQueryClient, err := bqcachedclient.New(context.TODO(),
					f.GoogleCloudFlags.ServiceAccountCredentialFile,
					f.BigQueryFlags.BigQueryProject,
					f.BigQueryFlags.BigQueryDataset, cacheClient)
				if err != nil {
					log.WithError(err).Fatal("CRITICAL error getting BigQuery client which prevents regression tracking")
				}

				cacheOpts := cache.RequestOptions{CRTimeRoundingFactor: f.ComponentReadinessFlags.CRTimeRoundingFactor}

				views, err := f.ComponentReadinessFlags.ParseViewsFile()
				if err != nil {
					log.WithError(err).Fatal("unable to load views")
				}
				releases, err := api.GetReleases(bigQueryClient)
				if err != nil {
					log.WithError(err).Fatal("error querying releases")
				}
				regressionLoader := regressionloader.New(bigQueryClient, cacheOpts, views.ComponentReadiness, releases)
				processes = append(processes, regressionLoader)

			}

			daemonServer := sippyserver.NewDaemonServer(processes)

			// Serve our metrics endpoint for prometheus to scrape
			if f.MetricsAddr != "" {
				go func() {
					http.Handle("/metrics", promhttp.Handler())
					err := http.ListenAndServe(f.MetricsAddr, nil) //nolint
					if err != nil {
						panic(err)
					}
				}()
			}

			daemonServer.Serve()

			return nil

		},
	}

	f.BindFlags(cmd.Flags())
	return cmd
}

func main() {
	// Set log level
	level, err := log.ParseLevel(logLevel)
	if err != nil {
		log.WithError(err).Fatal("cannot parse log-level")
	}
	log.SetLevel(level)
	log.Debug("debug logging enabled")

	// Add some millisecond precision to log timestamps, useful for debugging performance.
	formatter := new(log.TextFormatter)
	formatter.TimestampFormat = "2006-01-02T15:04:05.999Z07:00"
	formatter.FullTimestamp = true
	formatter.DisableColors = false
	log.SetFormatter(formatter)

	cmd := NewSippyDaemonCommand()
	cmd.PersistentFlags().StringVar(&logLevel, "log-level", "info",
		"Log level (trace,debug,info,warn,error) (default info)")

	err = cmd.Execute()
	if err != nil {
		log.WithError(err).Fatal("could not execute root command")
	}
}
