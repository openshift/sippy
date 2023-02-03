package cmd

import (
	"context"
	"os"

	"cloud.google.com/go/bigquery"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"google.golang.org/api/option"

	"github.com/openshift/sippy/cmd/flags"
	"github.com/openshift/sippy/pkg/prowloader"
	"github.com/openshift/sippy/pkg/prowloader/gcs"
	"github.com/openshift/sippy/pkg/prowloader/github"
)

type LoadProwFlags struct {
	DBFlags                    *flags.PostgresDatabaseFlags
	GoogleCloudCredentialFlags *flags.GoogleCloudCredentialFlags
	ConfigFlags                *flags.ConfigFlags
	ModeFlags                  *flags.ModeFlags
	LoadFromBigQuery           bool
	LoadGitHub                 bool
	Releases                   []string
}

func NewLoadProwFlags() *LoadProwFlags {
	return &LoadProwFlags{
		DBFlags:                    flags.NewPostgresDatabaseFlags(),
		ModeFlags:                  flags.NewModeFlags(),
		GoogleCloudCredentialFlags: flags.NewGoogleCloudCredentialFlags(),
		ConfigFlags:                flags.NewConfigFlags(),
	}
}

func (f *LoadProwFlags) BindFlags(fs *pflag.FlagSet) {
	f.DBFlags.BindFlags(fs)
	f.GoogleCloudCredentialFlags.BindFlags(fs)
	f.ConfigFlags.BindFlags(fs)
	f.ModeFlags.BindFlags(fs)
	fs.BoolVar(&f.LoadFromBigQuery, "load-openshift-ci-bigquery", f.LoadFromBigQuery, "Load from OpenShift CI BigQuery tables instead of directly from Prow")
	fs.BoolVar(&f.LoadGitHub, "load-github", f.LoadGitHub, "Fetch PR state date from GitHub")
	fs.StringArrayVar(&f.Releases, "releases", f.Releases, "Which releases to load from")
}

func init() {
	f := NewLoadProwFlags()

	cmd := &cobra.Command{
		Use:   "prow",
		Short: "Load job runs and test data from prow",
		Run: func(cmd *cobra.Command, args []string) {
			var allErrs []error

			dbc := f.DBFlags.GetDBClient()

			gcsClient, err := gcs.NewGCSClient(context.TODO(),
				f.GoogleCloudCredentialFlags.ServiceAccountCredentialFile,
				f.GoogleCloudCredentialFlags.OAuthClientCredentialFile,
			)
			if err != nil {
				cmd.Usage() //nolint:errcheck
				log.WithError(err).Fatal("CRITICAL error getting GCS client which prevents importing prow jobs")
			}

			var bigQueryClient *bigquery.Client
			if f.LoadFromBigQuery {
				bigQueryClient, err = bigquery.NewClient(context.Background(), "openshift-gce-devel",
					option.WithCredentialsFile(f.GoogleCloudCredentialFlags.ServiceAccountCredentialFile))
				if err != nil {
					cmd.Usage() //nolint:errcheck
					log.WithError(err).Fatal("CRITICAL error getting BigQuery client which prevents importing prow jobs")
				}
			}

			var githubClient *github.Client
			if f.LoadGitHub {
				githubClient = github.New(context.TODO())
			}

			prowLoader := prowloader.New(dbc,
				gcsClient,
				bigQueryClient,
				"origin-ci-test",
				githubClient,
				f.ModeFlags.GetVariantManager(),
				f.ModeFlags.GetSyntheticTestManager(),
				f.Releases,
				f.ConfigFlags.LoadConfig())

			errs := prowLoader.LoadProwJobsToDB()
			allErrs = append(allErrs, errs...)
			if len(allErrs) > 0 {
				for _, err := range allErrs {
					log.Error("error loading jobs\n")
					log.Error(err)
				}
				os.Exit(1)
			}
		},
	}

	f.BindFlags(cmd.Flags())
	loadCmd.AddCommand(cmd)
}
