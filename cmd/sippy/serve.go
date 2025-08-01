package main

import (
	"context"
	"io/fs"
	"net/http"
	"time"

	"cloud.google.com/go/storage"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	resources "github.com/openshift/sippy"
	"github.com/openshift/sippy/pkg/ai"
	"github.com/openshift/sippy/pkg/apis/cache"
	"github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/gcs"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/flags"
	"github.com/openshift/sippy/pkg/flags/configflags"
	"github.com/openshift/sippy/pkg/sippyserver"
	"github.com/openshift/sippy/pkg/sippyserver/metrics"
	"github.com/openshift/sippy/pkg/util"
)

type ServerFlags struct {
	AIFlags                 *flags.AIFlags
	BigQueryFlags           *flags.BigQueryFlags
	CacheFlags              *flags.CacheFlags
	DBFlags                 *flags.PostgresFlags
	GoogleCloudFlags        *flags.GoogleCloudFlags
	ModeFlags               *flags.ModeFlags
	ComponentReadinessFlags *flags.ComponentReadinessFlags
	ConfigFlags             *configflags.ConfigFlags
	APIFlags                *flags.APIFlags
	JiraFlags               *flags.JiraFlags
}

func NewServerFlags() *ServerFlags {
	return &ServerFlags{
		AIFlags:                 flags.NewAIFlags(),
		BigQueryFlags:           flags.NewBigQueryFlags(),
		CacheFlags:              flags.NewCacheFlags(),
		DBFlags:                 flags.NewPostgresDatabaseFlags(),
		GoogleCloudFlags:        flags.NewGoogleCloudFlags(),
		ModeFlags:               flags.NewModeFlags(),
		ComponentReadinessFlags: flags.NewComponentReadinessFlags(),
		ConfigFlags:             configflags.NewConfigFlags(),
		APIFlags:                flags.NewAPIFlags(),
		JiraFlags:               flags.NewJiraFlags(),
	}
}

func (f *ServerFlags) BindFlags(flagSet *pflag.FlagSet) {
	f.AIFlags.BindFlags(flagSet)
	f.BigQueryFlags.BindFlags(flagSet)
	f.CacheFlags.BindFlags(flagSet)
	f.DBFlags.BindFlags(flagSet)
	f.GoogleCloudFlags.BindFlags(flagSet)
	f.ModeFlags.BindFlags(flagSet)
	f.ComponentReadinessFlags.BindFlags(flagSet)
	f.ConfigFlags.BindFlags(flagSet)
	f.APIFlags.BindFlags(flagSet)
	f.JiraFlags.BindFlags(flagSet)
}

func (f *ServerFlags) Validate() error {
	return f.GoogleCloudFlags.Validate()
}

func NewServeCommand() *cobra.Command {
	f := NewServerFlags()

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the sippy server",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := f.Validate(); err != nil {
				return errors.WithMessage(err, "error validating options")
			}

			dbc, err := f.DBFlags.GetDBClient()
			if err != nil {
				return errors.WithMessage(err, "couldn't get DB client")
			}

			cacheClient, err := f.CacheFlags.GetCacheClient()
			if err != nil {
				return errors.WithMessage(err, "couldn't get cache client")
			}

			config, err := f.ConfigFlags.GetConfig()
			if err != nil {
				log.WithError(err).Warn("error reading config file")
			}

			var bigQueryClient *bigquery.Client
			var gcsClient *storage.Client
			if f.GoogleCloudFlags.ServiceAccountCredentialFile != "" {
				bigQueryClient, err = f.BigQueryFlags.GetBigQueryClient(context.Background(),
					cacheClient, f.GoogleCloudFlags.ServiceAccountCredentialFile)
				if err != nil {
					return errors.WithMessage(err, "couldn't get bigquery client")
				}

				if bigQueryClient != nil && f.CacheFlags.EnablePersistentCaching {
					bigQueryClient = f.CacheFlags.DecorateBiqQueryClientWithPersistentCache(bigQueryClient)
				}

				gcsClient, err = gcs.NewGCSClient(context.TODO(),
					f.GoogleCloudFlags.ServiceAccountCredentialFile,
					f.GoogleCloudFlags.OAuthClientCredentialFile,
				)
				if err != nil {
					log.WithError(err).Warn("unable to create GCS client, some APIs may not work")
				}
			}

			var llmClient *ai.LLMClient
			if f.AIFlags.Endpoint != "" {
				llmClient = f.AIFlags.GetLLMClient()
			}

			// Make sure the db is intialized, otherwise let the user know:
			prowJobs := []models.ProwJob{}
			res := dbc.DB.Find(&prowJobs).Limit(1)
			if res.Error != nil {
				return errors.WithMessage(err, "error querying for a ProwJob, database may need to be initialized with --init-database")
			}

			webRoot, err := fs.Sub(resources.SippyNG, "sippy-ng/build")
			if err != nil {
				log.WithError(err).Fatal("could not load frontend")
			}

			pinnedDateTime := f.DBFlags.GetPinnedTime()

			variantManager := f.ModeFlags.GetVariantManager(context.Background(), bigQueryClient)
			views, err := f.ComponentReadinessFlags.ParseViewsFile()
			if err != nil {
				log.WithError(err).Fatal("unable to load views")

			}

			jiraClient, err := f.JiraFlags.GetJiraClient()
			if err != nil {
				log.WithError(err).Warn("unable to initialize Jira client, bug filing will be disabled")
			}

			server := sippyserver.NewServer(
				f.ModeFlags.GetServerMode(),
				f.APIFlags.ListenAddr,
				f.ModeFlags.GetSyntheticTestManager(),
				variantManager,
				webRoot,
				&resources.Static,
				dbc,
				gcsClient,
				f.GoogleCloudFlags.StorageBucket,
				bigQueryClient,
				pinnedDateTime,
				cacheClient,
				f.ComponentReadinessFlags.CRTimeRoundingFactor,
				views,
				config,
				f.APIFlags.EnableWriteEndpoints,
				llmClient,
				jiraClient,
			)

			if f.APIFlags.MetricsAddr != "" {
				// Do an immediate metrics update
				err = metrics.RefreshMetricsDB(
					context.Background(),
					dbc,
					bigQueryClient,
					util.GetReportEnd(pinnedDateTime),
					cache.RequestOptions{CRTimeRoundingFactor: f.ComponentReadinessFlags.CRTimeRoundingFactor},
					views.ComponentReadiness,
					config.ComponentReadinessConfig.VariantJunitTableOverrides)
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
							err := metrics.RefreshMetricsDB(
								context.Background(),
								dbc,
								bigQueryClient,
								util.GetReportEnd(pinnedDateTime),
								cache.RequestOptions{CRTimeRoundingFactor: f.ComponentReadinessFlags.CRTimeRoundingFactor},
								views.ComponentReadiness,
								config.ComponentReadinessConfig.VariantJunitTableOverrides)
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
					err := http.ListenAndServe(f.APIFlags.MetricsAddr, nil) // nolint
					if err != nil {
						panic(err)
					}
				}()
			}

			server.Serve()
			return nil
		},
	}

	f.BindFlags(cmd.Flags())
	return cmd
}
