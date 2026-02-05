package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/openshift/sippy/pkg/api"
	"github.com/openshift/sippy/pkg/api/componentreadiness"
	"github.com/openshift/sippy/pkg/apis/cache"
	sippyv1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	"github.com/openshift/sippy/pkg/dataloader/crcacheloader"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/push"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"google.golang.org/api/option"

	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/dataloader/featuregateloader"
	"github.com/openshift/sippy/pkg/dataloader/variantsyncer"
	"github.com/openshift/sippy/pkg/flags/configflags"
	"github.com/openshift/sippy/pkg/sippyserver"
	"github.com/openshift/sippy/pkg/variantregistry"

	v1 "github.com/openshift/sippy/pkg/apis/config/v1"
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
	"github.com/openshift/sippy/pkg/flags"
	"github.com/openshift/sippy/pkg/github/commenter"
)

type LoadFlags struct {
	Loaders []string

	InitDatabase bool

	Architectures []string
	Releases      []string

	BigQueryFlags           *flags.BigQueryFlags
	ConfigFlags             *configflags.ConfigFlags
	DBFlags                 *flags.PostgresFlags
	GithubCommenterFlags    *flags.GithubCommenterFlags
	GoogleCloudFlags        *flags.GoogleCloudFlags
	ModeFlags               *flags.ModeFlags
	CacheFlags              *flags.CacheFlags
	ComponentReadinessFlags *flags.ComponentReadinessFlags
	JiraFlags               *flags.JiraFlags
	JobVariantsInputFile    string
	LogLevel                string
}

// want a single total load and refresh time
var loadMetricGauge = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "sippy_data_load_refresh_minutes",
	Help: "Minutes to load and refresh db",
})

func NewLoadFlags() *LoadFlags {
	return &LoadFlags{
		BigQueryFlags:           flags.NewBigQueryFlags(),
		ConfigFlags:             configflags.NewConfigFlags(),
		DBFlags:                 flags.NewPostgresDatabaseFlags(),
		GithubCommenterFlags:    flags.NewGithubCommenterFlags(),
		GoogleCloudFlags:        flags.NewGoogleCloudFlags(),
		ModeFlags:               flags.NewModeFlags(),
		CacheFlags:              flags.NewCacheFlags(),
		ComponentReadinessFlags: flags.NewComponentReadinessFlags(),
		JiraFlags:               flags.NewJiraFlags(),
	}
}

func (f *LoadFlags) BindFlags(fs *pflag.FlagSet) {
	f.BigQueryFlags.BindFlags(fs)
	f.ConfigFlags.BindFlags(fs)
	f.DBFlags.BindFlags(fs)
	f.GithubCommenterFlags.BindFlags(fs)
	f.GoogleCloudFlags.BindFlags(fs)
	f.ModeFlags.BindFlags(fs)
	f.CacheFlags.BindFlags(fs)
	f.ComponentReadinessFlags.BindFlags(fs)
	f.JiraFlags.BindFlags(fs)

	fs.BoolVar(&f.InitDatabase, "init-database", false, "Migrate the DB before loading")
	fs.StringArrayVar(&f.Loaders, "loader", []string{"prow", "releases", "jira", "github", "bugs", "test-mapping", "feature-gates"}, "Which data sources to use for data loading")
	fs.StringArrayVar(&f.Releases, "release", f.Releases, "Which releases to load (one per arg instance)")
	fs.StringArrayVar(&f.Architectures, "arch", f.Architectures, "Which architectures to load (one per arg instance)")
	fs.StringVar(&f.JobVariantsInputFile, "job-variants-input-file", "expected-job-variants.json", "JSON input file for the job-variants loader")
	fs.StringVar(&f.LogLevel, "log-level", "info", "Log level")
}

// nolint:gocyclo
func NewLoadCommand() *cobra.Command {
	f := NewLoadFlags()

	cmd := &cobra.Command{
		Use:   "load",
		Short: "Load data in the database",
		RunE: func(cmd *cobra.Command, args []string) error {
			level, err := log.ParseLevel(f.LogLevel)
			if err != nil {
				log.WithError(err).Fatal("cannot parse log-level")
			}
			log.SetLevel(level)

			loaders := make([]dataloader.DataLoader, 0)
			allErrs := []error{}

			// Cancel syncing after 4 hours
			ctx, cancel := context.WithTimeout(context.Background(), time.Hour*4)
			defer cancel()

			start := time.Now()

			// Get a DB client if possible. Some loaders do not need one, so this dbErr may end up non-nil,
			// each loader block should check if it needs the db connection.
			var dbErr error
			dbc, err := f.DBFlags.GetDBClient()
			if err != nil {
				dbErr = errors.WithMessage(err, "could not get db client: %+v")
			} else if f.InitDatabase {
				t := f.DBFlags.GetPinnedTime()
				if err := dbc.UpdateSchema(t); err != nil {
					dbErr = errors.WithMessage(err, "could not migrate db")
				}
			}

			cacheClient, cacheErr := f.CacheFlags.GetCacheClient()
			releaseConfigs := []sippyv1.Release{}

			// initializing a different bigquery client to the normal one
			bqc, bigqueryErr := bqcachedclient.New(ctx,
				f.GoogleCloudFlags.ServiceAccountCredentialFile,
				f.BigQueryFlags.BigQueryProject,
				f.BigQueryFlags.BigQueryDataset, cacheClient, f.BigQueryFlags.ReleasesTable)
			if bigqueryErr == nil {
				if f.CacheFlags.EnablePersistentCaching {
					bqc = f.CacheFlags.DecorateBiqQueryClientWithPersistentCache(bqc)
				}
				releaseConfigs, err = api.GetReleasesFromBigQuery(context.Background(), bqc)
				if err != nil {
					return errors.Wrapf(err, "error querying releases from bq")
				}
			}

			// Sippy Config
			config, err := f.ConfigFlags.GetConfig()
			if err != nil {
				return err
			}

			var refreshMatviews bool
			var promPusher *push.Pusher
			if pushgateway := os.Getenv("SIPPY_PROMETHEUS_PUSHGATEWAY"); pushgateway != "" {
				promPusher = push.New(pushgateway, "sippy-prow-job-loader")
				promPusher.Collector(loadMetricGauge)
			}

			for _, l := range f.Loaders {
				if l == "component-readiness-cache" {
					if bigqueryErr != nil {
						return errors.Wrap(bigqueryErr, "CRITICAL error getting BigQuery client which prevents cache loading")
					}
					if dbErr != nil {
						return dbErr
					}
					if cacheErr != nil {
						return errors.Wrap(err, "couldn't get cache client")
					}
					if f.CacheFlags.RedisURL == "" {
						return fmt.Errorf("--redis-url is required")
					}

					views, err := f.ComponentReadinessFlags.ParseViewsFile()
					if err != nil {
						return errors.Wrap(err, "error parsing views file")
					}
					if len(views.ComponentReadiness) == 0 {
						return fmt.Errorf("no component readiness views provided")
					}
					loaders = append(loaders, crcacheloader.New(dbc, cacheClient, bqc, config, views, releaseConfigs,
						f.ComponentReadinessFlags.CRTimeRoundingFactor))

				}

				if l == "releases" {
					if dbErr != nil {
						return dbErr
					}
					loaders = append(loaders, releaseloader.New(dbc, f.Releases, f.Architectures, releaseConfigs))
				}

				// Prow Loader
				if l == "prow" {
					refreshMatviews = true
					if dbErr != nil {
						return dbErr
					}
					prowLoader, err := f.prowLoader(ctx, dbc, config, releaseConfigs, promPusher)
					if err != nil {
						return err
					}

					loaders = append(loaders, prowLoader)
				}

				// JIRA Loader
				if l == "jira" {
					if dbErr != nil {
						return dbErr
					}
					loaders = append(loaders, jiraloader.New(dbc))
				}

				// Load mapping for jira components to tests
				if l == "test-mapping" {
					refreshMatviews = true
					if dbErr != nil {
						return dbErr
					}
					cl, err := testownershiploader.New(ctx,
						dbc,
						f.GoogleCloudFlags.ServiceAccountCredentialFile,
						f.GoogleCloudFlags.OAuthClientCredentialFile)
					if err != nil {
						return errors.WithMessage(err, "failed to create component loader")
					}

					loaders = append(loaders, cl)
				}

				// Bug Loader
				if l == "bugs" {
					if dbErr != nil {
						return dbErr
					}
					if bigqueryErr != nil {
						return errors.WithMessage(err, "could not get bigquery client")
					}
					loaders = append(loaders, bugloader.New(dbc, bqc))
				}

				// Load Job Variants into BigQuery
				if l == "job-variants" {
					variantsLoader, err := f.jobVariantsLoader(ctx)
					if err != nil {
						return err
					}
					loaders = append(loaders, variantsLoader)
				}

				// Sync postgres variants from BigQuery -- directly updates all jobs immediately
				// without us waiting to see the job again.
				if l == "sync-variants" {
					refreshMatviews = true
					if bigqueryErr != nil {
						return errors.WithMessage(err, "could not get bigquery client")
					}
					vs, err := variantsyncer.New(dbc, bqc)
					if err != nil {
						return err
					}
					loaders = append(loaders, vs)
				}

				// Feature gates
				if l == "feature-gates" {
					refreshMatviews = true
					fgLoader := featuregateloader.New(dbc, releaseConfigs)
					loaders = append(loaders, fgLoader)
				}

				if l == "regression-tracker" {
					if bigqueryErr != nil {
						return errors.Wrap(bigqueryErr, "CRITICAL error getting BigQuery client which prevents regression tracking")
					}
					if dbErr != nil {
						return errors.Wrap(dbErr, "CRITICAL error getting postgres client which prevents regression tracking")
					}
					cacheOpts := cache.RequestOptions{CRTimeRoundingFactor: f.ComponentReadinessFlags.CRTimeRoundingFactor}

					views, err := f.ComponentReadinessFlags.ParseViewsFile()
					if err != nil {
						return errors.Wrap(err, "error parsing views file")
					}
					if len(views.ComponentReadiness) == 0 {
						return fmt.Errorf("no component readiness views provided")
					}
					releases, err := api.GetReleases(context.TODO(), bqc, false)
					if err != nil {
						log.WithError(err).Fatal("error querying releases")
					}

					jiraClient, err := f.JiraFlags.GetJiraClient()
					if err != nil {
						return errors.Wrap(err, "CRITICAL error getting jira client which prevents regression tracking")
					}

					regressionTracker := componentreadiness.NewRegressionTracker(
						bqc, dbc, cacheOpts, releases,
						componentreadiness.NewPostgresRegressionStore(dbc, jiraClient),
						views.ComponentReadiness,
						config.ComponentReadinessConfig.VariantJunitTableOverrides,
						false)
					loaders = append(loaders, regressionTracker)
				}
			}

			// Run loaders with the metrics wrapper
			l := loaderwithmetrics.New(loaders)
			l.Load()
			if len(l.Errors()) > 0 {
				allErrs = append(allErrs, l.Errors()...)
			}

			elapsed := time.Since(start)
			log.WithField("elapsed", elapsed).Info("database load complete")

			pinnedTime := f.DBFlags.GetPinnedTime()
			if refreshMatviews {
				sippyserver.RefreshData(dbc, pinnedTime, false)
			}

			elapsed = time.Since(start)
			log.WithField("elapsed", elapsed).Info("load and refresh complete")

			if promPusher != nil {
				loadMetricGauge.Set(float64(elapsed.Minutes()))
				log.Info("pushing metrics to prometheus gateway")
				if err := promPusher.Add(); err != nil {
					log.WithError(err).Error("could not push to prometheus pushgateway")
				} else {
					log.Info("successfully pushed metrics to prometheus gateway")
				}
			}

			if len(allErrs) > 0 {
				log.Warningf("%d errors were encountered while loading database:", len(allErrs))
				for _, err := range allErrs {
					log.Error(err.Error())
				}
				return fmt.Errorf("errors were encountered while loading database, see logs for details")
			}
			log.Info("no errors encountered during db refresh")
			return nil
		},
	}

	f.BindFlags(cmd.Flags())

	return cmd
}

func (f *LoadFlags) jobVariantsLoader(ctx context.Context) (dataloader.DataLoader, error) {
	bigQueryClient, err := bigquery.NewClient(ctx, f.BigQueryFlags.BigQueryProject,
		option.WithCredentialsFile(f.GoogleCloudFlags.ServiceAccountCredentialFile))
	if err != nil {
		log.WithError(err).Error("CRITICAL error getting BigQuery client which prevents importing prow jobs")
		return nil, err
	}

	inputFile := f.JobVariantsInputFile

	file, err := os.Open(inputFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Read the contents of the file
	jsonData, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	// Unmarshal JSON data into a map
	var expectedVariants map[string]map[string]string
	err = json.Unmarshal(jsonData, &expectedVariants)
	if err != nil {
		return nil, err
	}

	log.Infof("Loaded expected job variant data from: %s", inputFile)
	syncer := variantregistry.NewJobVariantsLoader(bigQueryClient, f.BigQueryFlags.BigQueryProject,
		f.BigQueryFlags.BigQueryDataset, "job_variants", expectedVariants)
	return syncer, nil

}

func (f *LoadFlags) prowLoader(ctx context.Context, dbc *db.DB, sippyConfig *v1.SippyConfig, releaseConfigs []sippyv1.Release, promPusher *push.Pusher) (dataloader.DataLoader, error) {
	gcsClient, err := gcs.NewGCSClient(ctx,
		f.GoogleCloudFlags.ServiceAccountCredentialFile,
		f.GoogleCloudFlags.OAuthClientCredentialFile,
	)
	if err != nil {
		log.WithError(err).Error("CRITICAL error getting GCS client which prevents importing prow jobs")
		return nil, err
	}

	bigQueryClient, err := bqcachedclient.New(ctx, f.GoogleCloudFlags.ServiceAccountCredentialFile, f.BigQueryFlags.BigQueryProject, f.BigQueryFlags.BigQueryDataset, nil, f.BigQueryFlags.ReleasesTable)
	if err != nil {
		log.WithError(err).Error("CRITICAL error getting BigQuery client which prevents importing prow jobs")
		return nil, err
	}

	var githubClient *github.Client
	for _, l := range f.Loaders {
		if l == "github" {
			githubClient = github.New(ctx, github.OpenshiftOrg)
			break
		}
	}

	ghCommenter, err := commenter.NewGitHubCommenter(githubClient, dbc, f.GithubCommenterFlags.ExcludeReposCommenting, f.GithubCommenterFlags.IncludeReposCommenting)
	if err != nil {
		log.WithError(err).Error("CRITICAL error initializing GitHub commenter which prevents importing prow jobs")
		return nil, err
	}

	releases := f.Releases
	if len(releases) == 0 { // if not specified, use those defined in the Releases table
		for _, config := range releaseConfigs {
			releases = append(releases, config.Release) // could filter by capability if needed
		}
	}

	return prowloader.New(
		ctx,
		dbc,
		gcsClient,
		bigQueryClient,
		githubClient,
		f.ModeFlags.GetVariantManager(ctx, bigQueryClient),
		f.ModeFlags.GetSyntheticTestManager(),
		releases,
		sippyConfig,
		ghCommenter,
		promPusher), nil
}
