package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"cloud.google.com/go/bigquery"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"google.golang.org/api/option"

	"github.com/openshift/sippy/pkg/dataloader/prowloader/gcs"
	"github.com/openshift/sippy/pkg/flags/configflags"
	"github.com/openshift/sippy/pkg/variantregistry"

	"github.com/openshift/sippy/pkg/flags"
)

type LoadVariantsFlags struct {
	BigQueryFlags     *flags.BigQueryFlags
	GoogleCloudFlags  *flags.GoogleCloudFlags
	ConfigFlags       *configflags.ConfigFlags
	OutputFile        string
	Mode              string
	BigqueryJobsTable string
}

func NewLoadVariantsFlags() *LoadVariantsFlags {
	return &LoadVariantsFlags{
		BigQueryFlags:    flags.NewBigQueryFlags(),
		GoogleCloudFlags: flags.NewGoogleCloudFlags(),
		ConfigFlags:      configflags.NewConfigFlags(),
	}
}

func (f *LoadVariantsFlags) BindFlags(fs *pflag.FlagSet) {
	f.BigQueryFlags.BindFlags(fs)
	f.GoogleCloudFlags.BindFlags(fs)
	f.ConfigFlags.BindFlags(fs)
	fs.StringVar(&f.OutputFile, "o", "expected-job-variants.json", "Output json file for job variant data")
	fs.StringVar(&f.Mode, "mode", "ocp", "Implementation of job variant generator")
	fs.StringVar(&f.BigqueryJobsTable, "bigquery-jobs-table", "jobs", "Jobs table to load job names from")
}

func NewLoadJobVariantsCommand() *cobra.Command {
	f := NewLoadVariantsFlags()

	cmd := &cobra.Command{
		Use:   "generate-job-variants",
		Short: "Categorize all known jobs with their appropriate variants",
		Long:  "This command is somewhat OCP specific and will load all job names that have run in the last several months. The command will load a recent job runs artifacts to search for cluster-data.json, and then try to determine what variants the job should be categorized with based on a combination of the job name, and the contents of cluster-data.json. The resulting desired job variants json file is then written to disk and can be provided as input to the sync-job-variants command.",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Cancel syncing after 4 hours
			ctx, cancel := context.WithTimeout(context.Background(), time.Hour*4)
			defer cancel()

			config, err := f.ConfigFlags.GetConfig()
			if err != nil {
				return err
			}

			bigQueryClient, err := bigquery.NewClient(ctx, f.BigQueryFlags.BigQueryProject,
				option.WithCredentialsFile(f.GoogleCloudFlags.ServiceAccountCredentialFile))
			if err != nil {
				log.WithError(err).Error("CRITICAL error getting BigQuery client which prevents generating job variants")
				return err
			}
			gcsClient, err := gcs.NewGCSClient(context.TODO(),
				f.GoogleCloudFlags.ServiceAccountCredentialFile,
				f.GoogleCloudFlags.OAuthClientCredentialFile,
			)
			if err != nil {
				log.WithError(err).Error("CRITICAL error getting GCS client which prevents generating job variants")
				return err
			}

			var jsonData []byte

			switch f.Mode {
			case "ocp":

				jvs := variantregistry.NewOCPVariantLoader(
					bigQueryClient,
					f.BigQueryFlags.BigQueryProject,
					f.BigQueryFlags.BigQueryDataset,
					f.BigqueryJobsTable,
					gcsClient,
					f.GoogleCloudFlags.StorageBucket,
					config)
				expectedVariants, err := jvs.LoadExpectedJobVariants(context.TODO())
				if err != nil {
					return err
				}
				log.WithField("jobs", len(expectedVariants)).Info("calculated expected variants")
				jsonData, err = json.MarshalIndent(expectedVariants, "", "  ")
				if err != nil {
					return err
				}
			default:
				return fmt.Errorf("unknown mode: %s", f.Mode)
			}

			file, err := os.Create(f.OutputFile)
			if err != nil {
				return err
			}
			defer file.Close()

			_, err = file.Write(jsonData)
			if err != nil {
				return err
			}

			log.Infof("Expected OCP job variants written to: %s", f.OutputFile)

			return nil
		},
	}

	f.BindFlags(cmd.Flags())

	return cmd
}
