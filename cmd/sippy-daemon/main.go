package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/openshift/sippy/pkg/api/jobartifacts"
	"github.com/openshift/sippy/pkg/api/jobrunscan"
	"github.com/openshift/sippy/pkg/bigquery/bqlabel"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/riverqueue/river"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/gcs"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/github"
	"github.com/openshift/sippy/pkg/flags"
	"github.com/openshift/sippy/pkg/github/commenter"
	"github.com/openshift/sippy/pkg/sippyserver"
	"github.com/openshift/sippy/pkg/sippyserver/workqueue"
	"github.com/openshift/sippy/pkg/version"
)

var logLevel = "info"

type SippyDaemonFlags struct {
	BigQueryFlags    *flags.BigQueryFlags
	CacheFlags       *flags.CacheFlags
	DBFlags          *flags.PostgresFlags
	GoogleCloudFlags *flags.GoogleCloudFlags

	GithubCommenterFlags *flags.GithubCommenterFlags
	MetricsAddr          string
}

func NewSippyDaemonFlags() *SippyDaemonFlags {
	return &SippyDaemonFlags{
		DBFlags:              flags.NewPostgresDatabaseFlags(),
		BigQueryFlags:        flags.NewBigQueryFlags(),
		CacheFlags:           flags.NewCacheFlags(),
		GithubCommenterFlags: flags.NewGithubCommenterFlags(),
		GoogleCloudFlags:     flags.NewGoogleCloudFlags(),
	}
}

func (f *SippyDaemonFlags) BindFlags(fs *pflag.FlagSet) {
	f.BigQueryFlags.BindFlags(fs)
	f.CacheFlags.BindFlags(fs)
	f.DBFlags.BindFlags(fs)
	f.GithubCommenterFlags.BindFlags(fs)
	f.GoogleCloudFlags.BindFlags(fs)

	fs.StringVar(&f.MetricsAddr, "listen-metrics", f.MetricsAddr, "The address to serve prometheus metrics on (default :2112)")
}

func (f *SippyDaemonFlags) Validate() error {
	return f.GoogleCloudFlags.Validate()
}

func NewSippyDaemonCommand() *cobra.Command {
	f := NewSippyDaemonFlags()

	// rootCmd represents the base command when called without any subcommands
	cmd := &cobra.Command{
		Use:   "sippy-daemon",
		Short: "Sippy daemon is used for on-going tasks like monitoring git repos for reporting risk analysis.",
		PersistentPreRun: func(c *cobra.Command, args []string) {
			fmt.Fprintf(os.Stdout, "sippy built from %s\n", version.Get().GitCommit)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := f.Validate(); err != nil {
				return errors.WithMessage(err, "error validating options")
			}

			processes := make([]sippyserver.DaemonProcess, 0)

			if f.GithubCommenterFlags.CommentProcessing {
				dbc, err := f.DBFlags.GetDBClient()
				if err != nil {
					return err
				}

				githubClient := github.New(context.TODO(), github.OpenshiftOrg)
				ghCommenter, err := commenter.NewGitHubCommenter(githubClient,
					dbc, f.GithubCommenterFlags.ExcludeReposCommenting, f.GithubCommenterFlags.IncludeReposCommenting)
				if err != nil {
					log.WithError(err).Error("CRITICAL error initializing GitHub commenter which prevents PR commenting")
					return nil
				}

				cacheClient, err := f.CacheFlags.GetCacheClient()
				if err != nil {
					return errors.WithMessage(err, "couldn't get cache client")
				}

				opCtx := bqlabel.OperationalContext{
					App:         bqlabel.AppSippy,
					Command:     "sippy-daemon",
					Environment: bqlabel.EnvDaemon,
				}
				var bigQueryClient *bigquery.Client
				bigQueryClient, err = f.BigQueryFlags.GetBigQueryClient(context.Background(), opCtx, cacheClient, f.GoogleCloudFlags.ServiceAccountCredentialFile)
				if err != nil {
					return errors.WithMessage(err, "couldn't get bigquery client")
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
				processes = append(processes, sippyserver.NewWorkProcessor(dbc, bigQueryClient, gcsClient.Bucket(f.GoogleCloudFlags.StorageBucket), cacheClient, ghCommenter, 10, 5*time.Minute, 5*time.Second, f.GithubCommenterFlags.CommentProcessingDryRun))
			}

			// Set up River work queue for async job processing
			riverProcess, err := setupRiverProcess(cmd.Context(), f)
			if err != nil {
				log.WithError(err).Error("failed to set up River work queue, async re-evaluation will not be available")
			} else if riverProcess != nil {
				processes = append(processes, riverProcess)
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

const (
	reevaluateWorkerCount = 8
	jobRetentionPeriod    = 8 * 24 * time.Hour // 8 days
)

func setupRiverProcess(ctx context.Context, f *SippyDaemonFlags) (sippyserver.DaemonProcess, error) {
	dsn := f.DBFlags.DSN
	if dsn == "" {
		log.Info("no database DSN configured, skipping River work queue setup")
		return nil, nil
	}

	dbc, err := f.DBFlags.GetDBClient()
	if err != nil {
		return nil, fmt.Errorf("getting DB client for River: %w", err)
	}

	cacheClient, err := f.CacheFlags.GetCacheClient()
	if err != nil {
		return nil, fmt.Errorf("getting cache client for River: %w", err)
	}

	opCtx := bqlabel.OperationalContext{
		App:         bqlabel.AppSippy,
		Command:     "sippy-daemon",
		Environment: bqlabel.EnvDaemon,
	}
	bqClient, err := f.BigQueryFlags.GetBigQueryClient(ctx, opCtx, cacheClient, f.GoogleCloudFlags.ServiceAccountCredentialFile)
	if err != nil {
		return nil, fmt.Errorf("getting BigQuery client for River: %w", err)
	}

	gcsClient, err := gcs.NewGCSClient(ctx,
		f.GoogleCloudFlags.ServiceAccountCredentialFile,
		f.GoogleCloudFlags.OAuthClientCredentialFile,
	)
	if err != nil {
		return nil, fmt.Errorf("getting GCS client for River: %w", err)
	}

	artifactMgr := jobartifacts.NewManager(ctx)
	evaluator := jobrunscan.NewReEvaluator(bqClient, gcsClient, f.GoogleCloudFlags.StorageBucket, dbc, cacheClient, artifactMgr, false)

	workers := river.NewWorkers()
	river.AddWorker(workers, jobrunscan.NewReevaluateWorker(evaluator))

	riverSetup, err := workqueue.Setup(ctx, workqueue.SetupConfig{
		DatabaseDSN: dsn,
		Queues: map[string]river.QueueConfig{
			jobrunscan.ReevaluateQueue: {MaxWorkers: reevaluateWorkerCount},
		},
		Workers:               workers,
		CompletedJobRetention: jobRetentionPeriod,
		DiscardedJobRetention: jobRetentionPeriod,
	})
	if err != nil {
		return nil, fmt.Errorf("setting up River: %w", err)
	}

	return workqueue.NewRiverProcess(riverSetup.Client), nil
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
