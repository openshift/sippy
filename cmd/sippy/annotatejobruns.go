package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/openshift/sippy/pkg/api/componentreadiness"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/bq"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/cache"
	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/bigquery/bqlabel"
	"github.com/openshift/sippy/pkg/componentreadiness/jobrunannotator"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/gcs"
	"github.com/openshift/sippy/pkg/flags/configflags"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/openshift/sippy/pkg/flags"
)

type AnnotateJobRunsFlags struct {
	BigQueryFlags           *flags.BigQueryFlags
	PostgresFlags           *flags.PostgresFlags
	GoogleCloudFlags        *flags.GoogleCloudFlags
	CacheFlags              *flags.CacheFlags
	ComponentReadinessFlags *flags.ComponentReadinessFlags
	ConfigFlags             *configflags.ConfigFlags
	VariantStr              []string
	Variants                []bq.Variant
	Release                 string
	Label                   string
	BuildClusters           []string
	StartTimeStr            string
	StartTime               time.Time
	Duration                time.Duration
	MinFailures             int
	Execute                 bool
	FlakeAsFailure          bool
	TextContains            string
	TextRegex               string
	PathGlob                string
	JobRunIDs               *[]int64
	Comment                 string
	User                    string
}

func NewAnnotateJobRunsFlags() *AnnotateJobRunsFlags {
	return &AnnotateJobRunsFlags{
		BigQueryFlags:           flags.NewBigQueryFlags(),
		PostgresFlags:           flags.NewPostgresDatabaseFlags(),
		GoogleCloudFlags:        flags.NewGoogleCloudFlags(),
		CacheFlags:              flags.NewCacheFlags(),
		ComponentReadinessFlags: flags.NewComponentReadinessFlags(),
		ConfigFlags:             configflags.NewConfigFlags(),
	}
}

func (f *AnnotateJobRunsFlags) BindFlags(fs *pflag.FlagSet) {
	f.BigQueryFlags.BindFlags(fs)
	f.PostgresFlags.BindFlags(fs)
	f.GoogleCloudFlags.BindFlags(fs)
	f.CacheFlags.BindFlags(fs)
	f.ComponentReadinessFlags.BindFlags(fs)
	f.ConfigFlags.BindFlags(fs)
	fs.StringArrayVar(&f.VariantStr, "variant", f.VariantStr, "A variant to include to search for job runs. The format of the variant string is [variant]:[value] (e.g. Platform:metal).")
	fs.StringVar(&f.Release, "release", f.Release, "Release that applies to job runs.")
	fs.StringVar(&f.Label, "label", f.Label, "Label to be added to selected job runs.")
	fs.StringArrayVar(&f.BuildClusters, "build-cluster", f.BuildClusters, "The build clusters where jobs run.")
	fs.StringVar(&f.StartTimeStr, "start-time", f.StartTimeStr, "Start time to search for job runs. e.g. 2025-06-05T15:00:00Z")
	fs.IntVar(&f.MinFailures, "min-failures", f.MinFailures, "Minimum test failures for job runs.")
	fs.DurationVar(&f.Duration, "duration", f.Duration, "Duration from start-time to search for job runs. e.g. 24h")
	fs.BoolVar(&f.Execute, "execute", f.Execute, "By default, the command only prints the tasks of annotating job runs without really affecting DB items. This option will execute the DB actions.")
	fs.BoolVar(&f.FlakeAsFailure, "flake-as-failure", f.FlakeAsFailure, "Treat flakes as failures when counting failed tests.")
	fs.StringVar(&f.TextContains, "text-contains", f.TextContains, "Text to search in artifact path.")
	fs.StringVar(&f.TextRegex, "text-regex", f.TextRegex, "Regex to use when searching in artifact path.")
	fs.StringVar(&f.PathGlob, "path-glob", f.PathGlob, "The path glob from which to search for artifacts.")
	f.JobRunIDs = fs.Int64Slice("job-run-id", []int64{}, "A list of job runs to apply the label. Can be used if you already know the job IDs you want to apply the label. This list can be further filtered by other arguments")
	fs.StringVar(&f.Comment, "comment", f.Comment, "Comment you want to add with the label. This can serve as breadcrumbs to show where the label is from.")
	fs.StringVar(&f.User, "user", os.Getenv("USER"), "User who is applying the label.")
}

func (f *AnnotateJobRunsFlags) Validate(allVariants crtest.JobVariants) error {
	for _, variantStr := range f.VariantStr {
		vt := strings.Split(variantStr, ":")
		if len(vt) != 2 {
			return fmt.Errorf("--variant %s is in wrong format", variantStr)
		}
		vs, ok := allVariants.Variants[vt[0]]
		if !ok {
			return fmt.Errorf("--variant %s has wrong variant name %s", variantStr, vt[0])
		}
		found := false
		for _, v := range vs {
			if v == vt[1] {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("--variant %s has wrong variant value %s", variantStr, vt[1])
		}
		f.Variants = append(f.Variants, bq.Variant{Key: vt[0], Value: vt[1]})
	}
	if len(f.Label) == 0 {
		return fmt.Errorf("--label is required")
	}
	startTime, err := time.Parse(time.RFC3339, f.StartTimeStr)
	if err != nil {
		return fmt.Errorf("--start-time is in wrong format, correct format %s", time.RFC3339)
	}
	f.StartTime = startTime
	if f.Duration == time.Duration(0) {
		return fmt.Errorf("--duration is required")
	}
	if len(f.PathGlob) != 0 && (len(f.TextContains) == 0 && len(f.TextRegex) == 0) {
		return fmt.Errorf("--text-contains or --text-regex must be provided when using --path-glob")
	}
	if len(f.User) == 0 || f.User == "root" {
		return fmt.Errorf("--user is required and cannot be set to root")
	}
	return f.GoogleCloudFlags.Validate()
}

func NewAnnotateJobRunsCommand() *cobra.Command {
	f := NewAnnotateJobRunsFlags()

	cmd := &cobra.Command{
		Use:   "annotate-job-runs",
		Short: "Annotate job runs",
		Long: `Find all job runs that match the passed criteria and annotate them with desired label.
Example run: sippy annotate-job-runs  --google-service-account-credential-file=file.json --database-dsn="$DSN_PROD" --label=test --start-time="2025-05-21T00:00:00Z" --duration=48h --release=4.19 --min-failures=2 --variant=Platform:vsphere --path-glob="build-log.txt" --text-regex='\[error\]|"error"|level=error' --job-run-id=1925488012808949760 --job-run-id=1925488012808949761 --flake-as-failure=true --comment "ken test"  --build-cluster="build01" --build-cluster="vsphere02" --user=ken`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), time.Hour*1)
			defer cancel()
			ctx = context.WithValue(ctx, bqcachedclient.RequestContextKey, bqlabel.RequestContext{User: f.User})

			opCtx := bqlabel.OperationalContext{
				App:         bqlabel.AppSippy,
				Command:     "annotate-job-runs",
				Environment: bqlabel.EnvCli,
			}

			cacheClient, err := f.CacheFlags.GetCacheClient()
			if err != nil {
				log.WithError(err).Fatal("couldn't get cache client")
			}

			bigQueryClient, err := bqcachedclient.New(
				ctx, opCtx, cacheClient,
				f.GoogleCloudFlags.ServiceAccountCredentialFile,
				f.BigQueryFlags.BigQueryProject,
				f.BigQueryFlags.BigQueryDataset,
				f.BigQueryFlags.ReleasesTable)
			if err != nil {
				log.WithError(err).Fatal("error getting BigQuery client")
			}

			gcsClient, err := gcs.NewGCSClient(context.TODO(),
				f.GoogleCloudFlags.ServiceAccountCredentialFile,
				f.GoogleCloudFlags.OAuthClientCredentialFile,
			)
			if err != nil {
				log.WithError(err).Fatal("error getting gcs client")
			}

			if bigQueryClient != nil && f.CacheFlags.EnablePersistentCaching {
				bigQueryClient = f.CacheFlags.DecorateBiqQueryClientWithPersistentCache(bigQueryClient)
			}

			cacheOpts := cache.RequestOptions{CRTimeRoundingFactor: f.ComponentReadinessFlags.CRTimeRoundingFactor}

			dbc, err := f.PostgresFlags.GetDBClient()
			if err != nil {
				return errors.WithMessage(err, "couldn't get DB client")
			}

			allVariants, errs := componentreadiness.GetJobVariantsFromBigQuery(ctx, bigQueryClient)
			if len(errs) > 0 {
				return fmt.Errorf("failed to get variants from bigquery")
			}
			if err = f.Validate(allVariants); err != nil {
				return errors.WithMessage(err, "error validating options")
			}

			jobRunannotator, err := jobrunannotator.NewJobRunAnnotator(
				bigQueryClient,
				cacheOpts,
				gcsClient,
				dbc,
				cacheClient,
				f.Execute,
				f.Release,
				allVariants,
				f.Variants,
				f.Label,
				f.BuildClusters,
				f.StartTime,
				f.Duration,
				f.MinFailures,
				f.FlakeAsFailure,
				f.TextContains,
				f.TextRegex,
				f.PathGlob,
				*f.JobRunIDs,
				f.Comment,
				f.User)
			if err != nil {
				return errors.WithMessage(err, "error creating annotator")
			}
			return jobRunannotator.Run(ctx)
		},
	}

	f.BindFlags(cmd.Flags())

	return cmd
}
