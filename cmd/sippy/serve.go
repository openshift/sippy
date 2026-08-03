package main

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"slices"
	"time"

	"cloud.google.com/go/storage"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	resources "github.com/openshift/sippy"
	"github.com/openshift/sippy/pkg/api/componentreadiness/dataprovider"
	bqprovider "github.com/openshift/sippy/pkg/api/componentreadiness/dataprovider/bigquery"
	mixedprovider "github.com/openshift/sippy/pkg/api/componentreadiness/dataprovider/mixed"
	pgprovider "github.com/openshift/sippy/pkg/api/componentreadiness/dataprovider/postgres"
	"github.com/openshift/sippy/pkg/apis/cache"
	"github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/bigquery/bqlabel"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/gcs"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/flags"
	"github.com/openshift/sippy/pkg/flags/configflags"
	"github.com/openshift/sippy/pkg/sippyserver"
	"github.com/openshift/sippy/pkg/sippyserver/metrics"
	"github.com/openshift/sippy/pkg/testidentification"
	"github.com/openshift/sippy/pkg/util"
)

type ServerFlags struct {
	BigQueryFlags           *flags.BigQueryFlags
	CacheFlags              *flags.CacheFlags
	DBFlags                 *flags.PostgresFlags
	GoogleCloudFlags        *flags.GoogleCloudFlags
	ModeFlags               *flags.ModeFlags
	ComponentReadinessFlags *flags.ComponentReadinessFlags
	ConfigFlags             *configflags.ConfigFlags
	APIFlags                *flags.APIFlags
	JiraFlags               *flags.JiraFlags
	DataProvider            string
}

func NewServerFlags() *ServerFlags {
	return &ServerFlags{
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
	f.BigQueryFlags.BindFlags(flagSet)
	f.CacheFlags.BindFlags(flagSet)
	f.DBFlags.BindFlags(flagSet)
	f.GoogleCloudFlags.BindFlags(flagSet)
	f.ModeFlags.BindFlags(flagSet)
	f.ComponentReadinessFlags.BindFlags(flagSet)
	f.ConfigFlags.BindFlags(flagSet)
	f.APIFlags.BindFlags(flagSet)
	f.JiraFlags.BindFlags(flagSet)
	flagSet.StringVar(&f.DataProvider, "data-provider", "default", "Data provider: default, bigquery, or postgres")
}

func (f *ServerFlags) Validate() error {
	if f.DataProvider == "postgres" {
		return nil
	}
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
			var crDataProvider dataprovider.DataProvider
			switch f.DataProvider {
			case "default", "bigquery":
				if f.GoogleCloudFlags.ServiceAccountCredentialFile != "" {
					opCtx := bqlabel.OperationalContext{
						App:     bqlabel.AppSippy,
						Command: "serve",
						// outside prod, defaults to CLI as env and USER env var as operator
						Environment: bqlabel.EnvCli,
						Operator:    os.Getenv("USER"),
					}
					env := bqlabel.EnvValue(os.Getenv("SIPPY_WEB_ENV")) // set in prod
					if slices.Contains([]bqlabel.EnvValue{bqlabel.EnvWeb, bqlabel.EnvWebAuth, bqlabel.EnvWebQE}, env) {
						opCtx.Environment = env
						opCtx.Operator = string(env)
					}
					bigQueryClient, err = f.BigQueryFlags.GetBigQueryClient(context.Background(), opCtx, cacheClient, f.GoogleCloudFlags.ServiceAccountCredentialFile)
					if err != nil {
						return errors.WithMessage(err, "couldn't get bigquery client")
					}

					if bigQueryClient != nil && f.CacheFlags.EnablePersistentCaching {
						bigQueryClient = f.CacheFlags.DecorateBiqQueryClientWithPersistentCache(bigQueryClient)
					}
				}
			}

			crDataProvider, err = newDataProvider(f.DataProvider, bigQueryClient, dbc, cacheClient)
			if err != nil {
				return err
			}

			gcsClient, err = gcs.NewGCSClient(context.TODO(),
				f.GoogleCloudFlags.ServiceAccountCredentialFile,
				f.GoogleCloudFlags.OAuthClientCredentialFile,
			)
			if err != nil {
				log.WithError(err).Warn("unable to create GCS client, some APIs may not work")
			}

			// Make sure the db is initialized, otherwise let the user know:
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

			var variantManager testidentification.VariantManager
			if bigQueryClient != nil {
				variantManager = f.ModeFlags.GetVariantManager(context.Background(), bigQueryClient)
			}
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
				f.ComponentReadinessFlags.CORSAllowedOrigin,
				f.ModeFlags.GetSyntheticTestManager(),
				variantManager,
				webRoot,
				&resources.Static,
				dbc,
				gcsClient,
				f.GoogleCloudFlags.StorageBucket,
				bigQueryClient,
				crDataProvider,
				pinnedDateTime,
				cacheClient,
				f.ComponentReadinessFlags.CRTimeRoundingFactor,
				f.ComponentReadinessFlags.CRTimeRoundingOffset,
				views,
				config,
				f.APIFlags.EnableWriteEndpoints,
				f.APIFlags.ChatAPIURL,
				jiraClient,
			)
			fmt.Println("this is purely a junk PR to test ai review")

			if f.APIFlags.MetricsAddr != "" {
				// Do an immediate metrics update
				err = metrics.RefreshMetricsDB(
					context.Background(),
					dbc,
					bigQueryClient,
					crDataProvider,
					util.GetReportEnd(pinnedDateTime),
					cache.NewStandardCROptions(f.ComponentReadinessFlags.CRTimeRoundingFactor, f.ComponentReadinessFlags.CRTimeRoundingOffset),
					views.ComponentReadiness)
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
								crDataProvider,
								util.GetReportEnd(pinnedDateTime),
								cache.NewStandardCROptions(f.ComponentReadinessFlags.CRTimeRoundingFactor, f.ComponentReadinessFlags.CRTimeRoundingOffset),
								views.ComponentReadiness)
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

func newDataProvider(name string, bigQueryClient *bigquery.Client, dbc *db.DB, cacheClient cache.Cache) (dataprovider.DataProvider, error) {
	switch name {
	case "default":
		if bigQueryClient != nil && dbc != nil {
			return mixedprovider.NewMixedProvider(bigQueryClient, dbc, cacheClient), nil
		} else if bigQueryClient != nil {
			return bqprovider.NewBigQueryProvider(bigQueryClient), nil
		} else if dbc != nil {
			return pgprovider.NewPostgresProvider(dbc, cacheClient), nil
		}
		return nil, fmt.Errorf("default data provider requires at least one of BigQuery or PostgreSQL to be configured")

	case "bigquery":
		if bigQueryClient != nil {
			return bqprovider.NewBigQueryProvider(bigQueryClient), nil
		}
		return nil, fmt.Errorf("bigquery data provider requires google-service-account-credential-file to be configured")

	case "postgres":
		if dbc == nil {
			return nil, fmt.Errorf("postgres data provider requires a database connection")
		}
		log.Info("Using Postgres data provider for component readiness")
		return pgprovider.NewPostgresProvider(dbc, cacheClient), nil

	default:
		return nil, fmt.Errorf("unknown --data-provider %q, must be default, bigquery, or postgres", name)
	}
}
