package main

import (
	"context"
	"time"

	"github.com/openshift/sippy/pkg/api"
	"github.com/openshift/sippy/pkg/api/componentreadiness"
	"github.com/openshift/sippy/pkg/apis/cache"
	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/openshift/sippy/pkg/flags"
)

type TrackRegressionFlags struct {
	BigQueryFlags           *flags.BigQueryFlags
	GoogleCloudFlags        *flags.GoogleCloudFlags
	CacheFlags              *flags.CacheFlags
	ComponentReadinessFlags *flags.ComponentReadinessFlags
	ConfigFlags             *flags.ConfigFlags
}

func NewTrackRegressionFlags() *TrackRegressionFlags {
	return &TrackRegressionFlags{
		BigQueryFlags:           flags.NewBigQueryFlags(),
		GoogleCloudFlags:        flags.NewGoogleCloudFlags(),
		CacheFlags:              flags.NewCacheFlags(),
		ComponentReadinessFlags: flags.NewComponentReadinessFlags(),
		ConfigFlags:             flags.NewConfigFlags(),
	}
}

func (f *TrackRegressionFlags) BindFlags(fs *pflag.FlagSet) {
	f.BigQueryFlags.BindFlags(fs)
	f.GoogleCloudFlags.BindFlags(fs)
	f.CacheFlags.BindFlags(fs)
	f.ComponentReadinessFlags.BindFlags(fs)
	f.ConfigFlags.BindFlags(fs)
}

func NewTrackRegressionsCommand() *cobra.Command {
	f := NewTrackRegressionFlags()

	cmd := &cobra.Command{
		Use:   "track-regressions",
		Short: "Update tracked regressions for each view with tracking enabled",
		Long:  "Check the component report for each view with regression tracking enabled. Maintains tables in bigquery with times we saw regressions appear/disappear.",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), time.Hour*1)
			defer cancel()

			cacheClient, err := f.CacheFlags.GetCacheClient()
			if err != nil {
				log.WithError(err).Fatal("couldn't get cache client")
			}

			bigQueryClient, err := bqcachedclient.New(ctx,
				f.GoogleCloudFlags.ServiceAccountCredentialFile,
				f.BigQueryFlags.BigQueryProject,
				f.BigQueryFlags.BigQueryDataset, cacheClient)
			if err != nil {
				log.WithError(err).Fatal("CRITICAL error getting BigQuery client which prevents regression tracking")
			}

			config, err := f.ConfigFlags.GetConfig()
			if err != nil {
				log.WithError(err).Warn("error reading config file")
			}

			if bigQueryClient != nil && f.CacheFlags.EnablePersistentCaching {
				bigQueryClient = f.CacheFlags.DecorateBiqQueryClientWithPersistentCache(bigQueryClient)
			}

			cacheOpts := cache.RequestOptions{CRTimeRoundingFactor: f.ComponentReadinessFlags.CRTimeRoundingFactor}

			views, err := f.ComponentReadinessFlags.ParseViewsFile()
			if err != nil {
				log.WithError(err).Fatal("unable to load views")
			}
			releases, err := api.GetReleases(context.TODO(), bigQueryClient)
			if err != nil {
				log.WithError(err).Fatal("error querying releases")
			}
			regressionTracker := componentreadiness.NewRegressionTracker(
				bigQueryClient, cacheOpts, releases,
				componentreadiness.NewBigQueryRegressionStore(bigQueryClient),
				views.ComponentReadiness,
				config.ComponentReadinessConfig.VariantJunitTableOverrides,
				false)
			return regressionTracker.Run(ctx)
		},
	}

	f.BindFlags(cmd.Flags())

	return cmd
}
