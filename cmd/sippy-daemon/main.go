package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"cloud.google.com/go/storage"
	"github.com/openshift/sippy/pkg/api/jobartifacts"
	"github.com/openshift/sippy/pkg/api/jobrunscan"
	"github.com/openshift/sippy/pkg/apis/cache"
	"github.com/openshift/sippy/pkg/bigquery/bqlabel"
	"github.com/openshift/sippy/pkg/db"
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
	"github.com/openshift/sippy/pkg/sippyserver/workqueue/symptomre"
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

			// Shared clients used by multiple daemon processes.
			dbc, err := f.DBFlags.GetDBClient()
			if err != nil {
				return errors.WithMessage(err, "couldn't get DB client")
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
			bigQueryClient, err := f.BigQueryFlags.GetBigQueryClient(context.Background(), opCtx, cacheClient, f.GoogleCloudFlags.ServiceAccountCredentialFile)
			if err != nil {
				return errors.WithMessage(err, "couldn't get bigquery client")
			}

			gcsClient, err := gcs.NewGCSClient(context.TODO(),
				f.GoogleCloudFlags.ServiceAccountCredentialFile,
				f.GoogleCloudFlags.OAuthClientCredentialFile,
			)
			if err != nil {
				return errors.WithMessage(err, "couldn't get GCS client")
			}

			processes := make([]sippyserver.DaemonProcess, 0)

			// PR commenting process (optional, gated by flag).
			if f.GithubCommenterFlags.CommentProcessing {
				githubClient := github.New(context.TODO(), github.OpenshiftOrg)
				ghCommenter, err := commenter.NewGitHubCommenter(githubClient,
					dbc, f.GithubCommenterFlags.ExcludeReposCommenting, f.GithubCommenterFlags.IncludeReposCommenting)
				if err != nil {
					log.WithError(err).Error("CRITICAL error initializing GitHub commenter which prevents PR commenting")
					return nil
				}

				// we only process one comment every 5 seconds,
				// 4 potential GitHub calls per comment gives us a safe buffer
				// get comment data, get existing comments, possible delete existing, and adding the comment
				// could  lower to 3 seconds if we need, most writes likely won't have to delete
				processes = append(processes, sippyserver.NewWorkProcessor(dbc, bigQueryClient, gcsClient.Bucket(f.GoogleCloudFlags.StorageBucket), cacheClient, ghCommenter, 10, 5*time.Minute, 5*time.Second, f.GithubCommenterFlags.CommentProcessingDryRun))
			}

			// River work queue process for async symptom re-evaluation.
			riverProcess, err := setupRiverProcess(context.Background(), f, dbc, bigQueryClient, gcsClient, cacheClient)
			if err != nil {
				return errors.WithMessage(err, "couldn't set up River work queue")
			}
			processes = append(processes, riverProcess)

			// Periodic cleanup of old completed batches and stale non-terminal batches.
			processes = append(processes, symptomre.NewBatchCleanupProcess(dbc.DB))

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

// setupRiverProcess creates and configures the River work queue process for
// async symptom re-evaluation. It creates a pgx/v5 pool, runs River migrations,
// registers workers, and returns a DaemonProcess adapter.
// TODO: [lmeyer] I'd like to disentangle the process definition from the workers definition but
// that seems hard; punting until we need to add unrelated workers in the future.
func setupRiverProcess(ctx context.Context, f *SippyDaemonFlags, dbc *db.DB, bigQueryClient *bigquery.Client, gcsClient *storage.Client, cacheClient cache.Cache) (sippyserver.DaemonProcess, error) {
	pgxPool, err := workqueue.NewPgxV5Pool(ctx, f.DBFlags.DSN)
	if err != nil {
		return nil, fmt.Errorf("creating pgx/v5 pool for River: %w", err)
	}

	// River schema is also migrated by "sippy migrate" so the API server can
	// enqueue jobs. The daemon runs it too in case it starts before or without
	// the migrate step (e.g. local development).
	if err := workqueue.MigrateRiverSchema(ctx, pgxPool); err != nil {
		return nil, err
	}

	artifactMgr := jobartifacts.NewManager(ctx)
	reEvaluator := jobrunscan.NewReEvaluator(
		bigQueryClient, gcsClient, f.GoogleCloudFlags.StorageBucket,
		dbc, cacheClient, artifactMgr,
	)

	workers := river.NewWorkers()
	batchWorker := symptomre.NewProcessBatchWorker(reEvaluator, dbc.DB)
	reEvalWorker := symptomre.NewReevaluateWorker(reEvaluator)
	river.AddWorker(workers, batchWorker)
	river.AddWorker(workers, reEvalWorker)

	riverConfig := &river.Config{
		Queues: map[string]river.QueueConfig{
			symptomre.BatchQueue: {MaxWorkers: 1},
			symptomre.ItemQueue:  {MaxWorkers: 12},
		},
		// Keep completed and discarded River jobs for 8 days, slightly longer
		// than the 7-day batch retention so status queries on recent batches
		// can still resolve individual job outcomes.
		CompletedJobRetentionPeriod: 8 * 24 * time.Hour,
		DiscardedJobRetentionPeriod: 8 * 24 * time.Hour,
	}
	riverClient, err := workqueue.NewWorkerClient(pgxPool, workers, riverConfig)
	if err != nil {
		return nil, fmt.Errorf("creating River worker client: %w", err)
	}

	// The ProcessBatchWorker needs the River client to insert individual jobs.
	// Wire it after client creation to avoid a circular dependency.
	batchWorker.SetRiverClient(riverClient)

	return workqueue.NewRiverProcess(pgxPool, riverClient, reEvaluator), nil
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
