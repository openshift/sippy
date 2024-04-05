package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/openshift/sippy/pkg/variantregistry"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"google.golang.org/api/option"

	"github.com/openshift/sippy/pkg/flags"
)

type SyncJobVariantsFlags struct {
	BigQueryFlags    *flags.BigQueryFlags
	GoogleCloudFlags *flags.GoogleCloudFlags
}

func NewSyncJobVariantsFlags() *SyncJobVariantsFlags {
	return &SyncJobVariantsFlags{
		BigQueryFlags:    flags.NewBigQueryFlags(),
		GoogleCloudFlags: flags.NewGoogleCloudFlags(),
	}
}

func (f *SyncJobVariantsFlags) BindFlags(fs *pflag.FlagSet) {
	f.BigQueryFlags.BindFlags(fs)
	f.GoogleCloudFlags.BindFlags(fs)
}

func NewSyncJobVariantsCommand() *cobra.Command {
	f := NewSyncJobVariantsFlags()

	cmd := &cobra.Command{
		Use:   "sync-job-variants [flags] [inputfile]",
		Short: "Sync a file containing expected/desired job variant data to the BigQuery tables",
		Args:  cobra.ExactArgs(1), // the json file to read expected variant data from
		RunE: func(cmd *cobra.Command, args []string) error {
			// Cancel syncing after 4 hours
			ctx, cancel := context.WithTimeout(context.Background(), time.Hour*4)
			defer cancel()
			bigQueryClient, err := bigquery.NewClient(ctx, f.BigQueryFlags.BigQueryProject,
				option.WithCredentialsFile(f.GoogleCloudFlags.ServiceAccountCredentialFile))
			if err != nil {
				log.WithError(err).Error("CRITICAL error getting BigQuery client which prevents importing prow jobs")
				return err
			}

			inputFile := args[0]

			file, err := os.Open(inputFile)
			if err != nil {
				return err
			}
			defer file.Close()

			// Read the contents of the file
			jsonData, err := io.ReadAll(file)
			if err != nil {
				return err
			}

			// Unmarshal JSON data into a map
			var expectedVariants map[string]map[string]string
			err = json.Unmarshal(jsonData, &expectedVariants)
			if err != nil {
				return err
			}

			log.Infof("Loaded expected job variant data from: %s", inputFile)
			syncer := variantregistry.NewSyncer(bigQueryClient, f.BigQueryFlags.BigQueryProject,
				f.BigQueryFlags.BigQueryDataset, "job_variants")
			err = syncer.SyncJobVariants(expectedVariants)
			if err != nil {
				log.WithError(err).Fatal("error syncing expected job variants")
			}

			return nil
		},
	}

	f.BindFlags(cmd.Flags())

	return cmd
}
