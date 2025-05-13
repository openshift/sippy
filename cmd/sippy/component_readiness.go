package main

import (
	"context"
	"io/fs"
	"net/http"
	"os"
	"time"

	"cloud.google.com/go/storage"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"

	resources "github.com/openshift/sippy"
	"github.com/openshift/sippy/pkg/apis/cache"
	v1 "github.com/openshift/sippy/pkg/apis/config/v1"
	"github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/gcs"
	"github.com/openshift/sippy/pkg/flags"
	"github.com/openshift/sippy/pkg/flags/configflags"
	"github.com/openshift/sippy/pkg/sippyserver"
	"github.com/openshift/sippy/pkg/sippyserver/metrics"
)

type ComponentReadinessFlags struct {
	GoogleCloudFlags        *flags.GoogleCloudFlags
	BigQueryFlags           *flags.BigQueryFlags
	PostgresFlags           *flags.PostgresFlags
	CacheFlags              *flags.CacheFlags
	ComponentReadinessFlags *flags.ComponentReadinessFlags
	ConfigFlags             *configflags.ConfigFlags
	APIFlags                *flags.APIFlags

	Config   string
	LogLevel string
}

func NewComponentReadinessCommand() *cobra.Command {
	f := &ComponentReadinessFlags{
		LogLevel: "info",

		GoogleCloudFlags:        flags.NewGoogleCloudFlags(),
		BigQueryFlags:           flags.NewBigQueryFlags(),
		PostgresFlags:           flags.NewPostgresDatabaseFlags(),
		CacheFlags:              flags.NewCacheFlags(),
		ComponentReadinessFlags: flags.NewComponentReadinessFlags(),
		ConfigFlags:             configflags.NewConfigFlags(),
		APIFlags:                flags.NewAPIFlags(),
	}

	cmd := &cobra.Command{
		Use: "component-readiness",

		RunE: func(cmd *cobra.Command, arguments []string) error {

			if err := f.Validate(); err != nil {
				return errors.WithMessage(err, "error validating options")
			}
			if err := f.Run(); err != nil {
				return errors.WithMessage(err, "error running command")
			}
			cmd.Context()

			return nil
		},
	}

	f.BindFlags(cmd.Flags())

	return cmd
}

func (f *ComponentReadinessFlags) BindFlags(flagSet *pflag.FlagSet) {
	f.CacheFlags.BindFlags(flagSet)
	f.BigQueryFlags.BindFlags(flagSet)
	f.PostgresFlags.BindFlags(flagSet)
	f.GoogleCloudFlags.BindFlags(flagSet)
	f.ComponentReadinessFlags.BindFlags(flagSet)
	f.ConfigFlags.BindFlags(flagSet)
	f.APIFlags.BindFlags(flagSet)
	flagSet.StringVar(&f.LogLevel, "log-level", f.LogLevel, "Log level (trace,debug,info,warn,error) (default info)")
}

func (f *ComponentReadinessFlags) Validate() error {
	return f.GoogleCloudFlags.Validate()
}

func (f *ComponentReadinessFlags) Run() error { //nolint:gocyclo
	// Set log level
	level, err := log.ParseLevel(f.LogLevel)
	if err != nil {
		log.WithError(err).Fatal("Cannot parse log-level")
	}
	log.SetLevel(level)

	// Add some millisecond precision to log timestamps, useful for debugging performance.
	formatter := new(log.TextFormatter)
	formatter.TimestampFormat = "2006-01-02T15:04:05.999Z07:00"
	formatter.FullTimestamp = true
	formatter.DisableColors = false
	log.SetFormatter(formatter)

	log.Debug("debug logging enabled")
	sippyConfig := v1.SippyConfig{}
	if f.Config != "" {
		data, err := os.ReadFile(f.Config)
		if err != nil {
			log.WithError(err).Fatalf("could not load config")
		}
		if err := yaml.Unmarshal(data, &sippyConfig); err != nil {
			log.WithError(err).Fatalf("could not unmarshal config")
		}
	}

	return f.runServerMode()
}

func (f *ComponentReadinessFlags) runServerMode() error {
	var err error

	webRoot, err := fs.Sub(resources.SippyNG, "sippy-ng/build")
	if err != nil {
		log.WithError(err).Fatal("could not load frontend")
	}

	cacheClient, err := f.CacheFlags.GetCacheClient()
	if err != nil {
		return errors.WithMessage(err, "couldn't get cache client")
	}

	var bigQueryClient *bigquery.Client
	var gcsClient *storage.Client
	if f.GoogleCloudFlags.ServiceAccountCredentialFile != "" {
		bigQueryClient, err = f.BigQueryFlags.GetBigQueryClient(context.Background(),
			cacheClient, f.GoogleCloudFlags.ServiceAccountCredentialFile)
		if err != nil {
			return errors.WithMessage(err, "couldn't get bigquery client")
		}

		gcsClient, err = gcs.NewGCSClient(context.TODO(),
			f.GoogleCloudFlags.ServiceAccountCredentialFile,
			f.GoogleCloudFlags.OAuthClientCredentialFile,
		)
		if err != nil {
			log.WithError(err).Warn("unable to create GCS client, some APIs may not work")
		}

		if bigQueryClient != nil && f.CacheFlags.EnablePersistentCaching {
			bigQueryClient = f.CacheFlags.DecorateBiqQueryClientWithPersistentCache(bigQueryClient)
		}
	}

	config, err := f.ConfigFlags.GetConfig()
	if err != nil {
		log.WithError(err).Warn("error reading config file")
	}

	views, err := f.ComponentReadinessFlags.ParseViewsFile()
	if err != nil {
		log.WithError(err).Fatal("unable to load views")

	}

	dbc, err := f.PostgresFlags.GetDBClient()
	if err != nil {
		log.WithError(err).Warn("unable to connect to postgres, regression tracking will be disabled")
	}
	server := sippyserver.NewServer(
		sippyserver.ModeOpenShift,
		f.APIFlags.ListenAddr,
		nil,
		nil,
		webRoot,
		&resources.Static,
		dbc,
		gcsClient,
		f.GoogleCloudFlags.StorageBucket,
		bigQueryClient,
		nil,
		cacheClient,
		f.ComponentReadinessFlags.CRTimeRoundingFactor,
		views,
		config,
		f.APIFlags.EnableWriteEndpoints,
		nil, // No AI use yet in Component Readiness
	)

	if f.APIFlags.MetricsAddr != "" {
		// Do an immediate metrics update
		err = metrics.RefreshMetricsDB(
			context.Background(),
			dbc,
			bigQueryClient,
			time.Time{},
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
						time.Time{},
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
}
