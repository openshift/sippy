package main

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"google.golang.org/api/option"

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
	"github.com/openshift/sippy/pkg/sippyserver"
)

type LoadFlags struct {
	LoadOpenShiftCIBigQuery bool
	Releases                []string
	Architectures           []string
	Loaders                 []string

	DBFlags              *flags.PostgresFlags
	ConfigFlags          *flags.ConfigFlags
	ModeFlags            *flags.ModeFlags
	GoogleCloudFlags     *flags.GoogleCloudFlags
	BigQueryFlags        *flags.BigQueryFlags
	GithubCommenterFlags *flags.GithubCommenterFlags
	InitDatabase         bool
}

func NewLoadFlags() *LoadFlags {
	return &LoadFlags{
		// DefaultOpenshiftGCSBucket is the Google cloud storage bucket that will be used if none is provided as a CLI argument.
		DBFlags:              flags.NewPostgresDatabaseFlags(),
		ConfigFlags:          flags.NewConfigFlags(),
		ModeFlags:            flags.NewModeFlags(),
		GoogleCloudFlags:     flags.NewGoogleCloudFlags(),
		BigQueryFlags:        flags.NewBigQueryFlags(),
		GithubCommenterFlags: flags.NewGithubCommenterFlags(),
	}
}

func (f *LoadFlags) BindFlags(fs *pflag.FlagSet) {
	f.DBFlags.BindFlags(fs)
	f.ConfigFlags.BindFlags(fs)
	f.ModeFlags.BindFlags(fs)
	f.BigQueryFlags.BindFlags(fs)
	f.GoogleCloudFlags.BindFlags(fs)
	f.GithubCommenterFlags.BindFlags(fs)

	fs.BoolVar(&f.InitDatabase, "init-database", false, "Migrate the DB before loading")
	fs.BoolVar(&f.LoadOpenShiftCIBigQuery, "load-openshift-ci-bigquery", false, "Load ProwJobs from OpenShift CI BigQuery")
	fs.StringSliceVar(&f.Loaders, "loaders", []string{"prow", "releases", "jira", "github", "bugs", "test-mapping"}, "Which data sources to use for data loading")
	fs.StringArrayVar(&f.Releases, "release", f.Releases, "Which releases to load (one per arg instance)")
	fs.StringArrayVar(&f.Architectures, "arch", f.Architectures, "Which architectures to load (one per arg instance)")
}

func NewLoadCommand() *cobra.Command {
	f := NewLoadFlags()

	cmd := &cobra.Command{
		Use:   "load",
		Short: "Load data in the database",
		RunE: func(cmd *cobra.Command, args []string) error {
			loaders := make([]dataloader.DataLoader, 0)
			allErrs := []error{}

			// Cancel syncing after 4 hours
			ctx, cancel := context.WithTimeout(context.Background(), time.Hour*4)
			defer cancel()

			// Get a DB client
			dbc, err := f.DBFlags.GetDBClient()
			if err != nil {
				return errors.WithMessage(err, "could not get db client: %+v")
			}

			start := time.Now()

			if f.InitDatabase {
				t := time.Time(f.DBFlags.PinnedTime)
				if err := dbc.UpdateSchema(&t); err != nil {
					return errors.WithMessage(err, "could not migrate db")
				}
			}

			// Sippy Config
			config, err := f.ConfigFlags.GetConfig()
			if err != nil {
				return err
			}

			for _, l := range f.Loaders {
				// Release payload tag loader
				if l == "releases" {
					loaders = append(loaders, releaseloader.New(dbc, f.Releases, f.Architectures))
				}

				// Prow Loader
				if l == "prow" {
					prowLoader, err := f.prowLoader(ctx, dbc, config)
					if err != nil {
						return err
					}
					loaders = append(loaders, prowLoader)
				}

				// JIRA Loader
				if l == "jira" {
					loaders = append(loaders, jiraloader.New(dbc))
				}

				// Load maping for jira components to tests
				if l == "test-mapping" {
					cl, err := testownershiploader.New(ctx,
						dbc,
						f.GoogleCloudFlags.ServiceAccountCredentialFile,
						f.GoogleCloudFlags.OAuthClientCredentialFile)
					if err != nil {
						return errors.WithMessage(err, "failed to create component loader")
					} else {
						loaders = append(loaders, cl)
					}
				}

				// Bug Loader
				if l == "bugs" {
					loaders = append(loaders, bugloader.New(dbc))
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

			sippyserver.RefreshData(dbc, f.DBFlags.PinnedTime.Time(), false)

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

func (f *LoadFlags) prowLoader(ctx context.Context, dbc *db.DB, sippyConfig *v1.SippyConfig) (dataloader.DataLoader, error) {
	gcsClient, err := gcs.NewGCSClient(ctx,
		f.GoogleCloudFlags.ServiceAccountCredentialFile,
		f.GoogleCloudFlags.OAuthClientCredentialFile,
	)
	if err != nil {
		log.WithError(err).Error("CRITICAL error getting GCS client which prevents importing prow jobs")
		return nil, err
	}

	var bigQueryClient *bigquery.Client
	if f.LoadOpenShiftCIBigQuery {
		bigQueryClient, err = bigquery.NewClient(ctx, f.BigQueryFlags.BigQueryProject,
			option.WithCredentialsFile(f.GoogleCloudFlags.ServiceAccountCredentialFile))
		if err != nil {
			log.WithError(err).Error("CRITICAL error getting BigQuery client which prevents importing prow jobs")
			return nil, err
		}
	}

	var githubClient *github.Client
	for _, l := range f.Loaders {
		if l == "github" {
			githubClient = github.New(ctx)
			break
		}
	}

	ghCommenter, err := commenter.NewGitHubCommenter(githubClient, dbc, f.GithubCommenterFlags.ExcludeReposCommenting, f.GithubCommenterFlags.IncludeReposCommenting)
	if err != nil {
		log.WithError(err).Error("CRITICAL error initializing GitHub commenter which prevents importing prow jobs")
		return nil, err
	}

	return prowloader.New(
		ctx,
		dbc,
		gcsClient,
		bigQueryClient,
		f.GoogleCloudFlags.StorageBucket,
		githubClient,
		f.ModeFlags.GetVariantManager(),
		f.ModeFlags.GetSyntheticTestManager(),
		f.Releases,
		sippyConfig,
		ghCommenter), nil
}
