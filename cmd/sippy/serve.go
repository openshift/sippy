package main

import (
	"context"
	"fmt"
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
	"github.com/openshift/sippy/pkg/apis/cache"
	"github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/gcs"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/flags"
	"github.com/openshift/sippy/pkg/sippyserver"
	"github.com/openshift/sippy/pkg/sippyserver/metrics"
	"github.com/openshift/sippy/pkg/util"
)

var (
	defaultCRTimeRoundingFactor = 4 * time.Hour
	maxCRTimeRoundingFactor     = 12 * time.Hour
)

type ServerFlags struct {
	BigQueryFlags    *flags.BigQueryFlags
	CacheFlags       *flags.CacheFlags
	DBFlags          *flags.PostgresFlags
	GoogleCloudFlags *flags.GoogleCloudFlags
	ModeFlags        *flags.ModeFlags
	ProwFlags        *flags.ProwFlags

	ListenAddr               string
	MetricsAddr              string
	MaintainRegressionTables bool
	CRTimeRoundingFactor     time.Duration
}

func NewServerFlags() *ServerFlags {
	return &ServerFlags{
		BigQueryFlags:    flags.NewBigQueryFlags(),
		CacheFlags:       flags.NewCacheFlags(),
		DBFlags:          flags.NewPostgresDatabaseFlags(),
		GoogleCloudFlags: flags.NewGoogleCloudFlags(),
		ModeFlags:        flags.NewModeFlags(),
		ProwFlags:        flags.NewProwFlags(),
		ListenAddr:       ":8080",
		MetricsAddr:      ":2112",
	}
}

func (f *ServerFlags) BindFlags(flagSet *pflag.FlagSet) {
	f.BigQueryFlags.BindFlags(flagSet)
	f.CacheFlags.BindFlags(flagSet)
	f.DBFlags.BindFlags(flagSet)
	f.GoogleCloudFlags.BindFlags(flagSet)
	f.ModeFlags.BindFlags(flagSet)
	f.ProwFlags.BindFlags(flagSet)

	flagSet.StringVar(&f.ListenAddr, "listen", f.ListenAddr, "The address to serve analysis reports on (default :8080)")
	flagSet.StringVar(&f.MetricsAddr, "listen-metrics", f.MetricsAddr, "The address to serve prometheus metrics on (default :2112)")
	factorUsage := fmt.Sprintf("Set the rounding factor for component readiness release time. The time will be rounded down to the nearest multiple of the factor. Maximum value is %v", maxCRTimeRoundingFactor)
	flagSet.DurationVar(&f.CRTimeRoundingFactor, "component-readiness-time-rounding-factor", defaultCRTimeRoundingFactor, factorUsage)
	flagSet.BoolVar(&f.MaintainRegressionTables, "maintain-regression-tables", false, "Enable maintenance of open regressions table in bigquery.")
}

func (f *ServerFlags) Validate() error {
	// TODO: Validate other flags
	return f.ProwFlags.Validate()
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

			server := sippyserver.NewServer(
				f.ModeFlags.GetServerMode(),
				f.ListenAddr,
				f.ModeFlags.GetSyntheticTestManager(),
				f.ModeFlags.GetVariantManager(),
				webRoot,
				&resources.Static,
				dbc,
				f.ProwFlags.URL,
				f.GoogleCloudFlags.StorageBucket,
				gcsClient,
				bigQueryClient,
				pinnedDateTime,
				cacheClient,
				f.CRTimeRoundingFactor,
			)

			if f.MetricsAddr != "" {
				// Do an immediate metrics update
				err = metrics.RefreshMetricsDB(dbc, bigQueryClient, f.ProwFlags.URL, f.GoogleCloudFlags.StorageBucket, f.ModeFlags.GetVariantManager(), util.GetReportEnd(pinnedDateTime), cache.RequestOptions{CRTimeRoundingFactor: f.CRTimeRoundingFactor}, f.MaintainRegressionTables)
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
							err := metrics.RefreshMetricsDB(dbc, bigQueryClient, f.ProwFlags.URL, f.GoogleCloudFlags.StorageBucket, f.ModeFlags.GetVariantManager(), util.GetReportEnd(pinnedDateTime), cache.RequestOptions{CRTimeRoundingFactor: f.CRTimeRoundingFactor}, f.MaintainRegressionTables)
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
					err := http.ListenAndServe(f.MetricsAddr, nil) //nolint
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
