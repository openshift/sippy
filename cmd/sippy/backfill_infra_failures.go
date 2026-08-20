package main

import (
	"context"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/dataloader/infrafailurebackfill"
	"github.com/openshift/sippy/pkg/flags"
)

type BackfillInfraFailuresFlags struct {
	BigQueryFlags    *flags.BigQueryFlags
	DBFlags          *flags.PostgresFlags
	GoogleCloudFlags *flags.GoogleCloudFlags

	Since     string
	Days      int
	DryRun    bool
	BatchSize int
}

func NewBackfillInfraFailuresFlags() *BackfillInfraFailuresFlags {
	return &BackfillInfraFailuresFlags{
		BigQueryFlags:    flags.NewBigQueryFlags(),
		DBFlags:          flags.NewPostgresDatabaseFlags(),
		GoogleCloudFlags: flags.NewGoogleCloudFlags(),
		Days:             90,
		BatchSize:        100,
	}
}

func (f *BackfillInfraFailuresFlags) BindFlags(fs *pflag.FlagSet) {
	f.BigQueryFlags.BindFlags(fs)
	f.DBFlags.BindFlags(fs)
	f.GoogleCloudFlags.BindFlags(fs)
	fs.StringVar(&f.Since, "since", "", "Start of the time window as a date (YYYY-MM-DD). Takes precedence over --days.")
	fs.IntVar(&f.Days, "days", f.Days, "Look back this many days from now (used when --since is not set)")
	fs.BoolVar(&f.DryRun, "dry-run", false, "Report what would be done without modifying the database")
	fs.IntVar(&f.BatchSize, "batch-size", f.BatchSize, "Number of runs to process per batch")
}

func NewBackfillInfraFailuresCommand() *cobra.Command {
	f := NewBackfillInfraFailuresFlags()

	cmd := &cobra.Command{
		Use:   "backfill-infra-failures",
		Short: "Backfill InfraFailure labels from BigQuery into PostgreSQL",
		Long: `Reads runs labeled InfraFailure from the BigQuery job_labels table within a
time window and, for any run that does not yet carry the label in PostgreSQL,
atomically applies the label and removes the run's contribution from the summary
tables (via pkg/db/infrafailure.RecordInfraFailure). The operation is idempotent,
so it is safe to run repeatedly.

Example:
  sippy backfill-infra-failures --google-service-account-credential-file=creds.json \
    --database-dsn="$DSN" --since=2026-07-01 --batch-size=100 --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Cancel after 4 hours, matching the load command's ceiling.
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Hour)
			defer cancel()

			if f.GoogleCloudFlags.ServiceAccountCredentialFile == "" {
				return fmt.Errorf("--google-service-account-credential-file is required to query BigQuery")
			}

			dbc, err := f.DBFlags.GetDBClient()
			if err != nil {
				return fmt.Errorf("could not get db client: %w", err)
			}

			opCtx, ctx := bqcachedclient.OpCtxForCronEnv(ctx, "backfill-infra-failures")
			bqc, err := bqcachedclient.New(
				ctx, opCtx, nil,
				f.GoogleCloudFlags.ServiceAccountCredentialFile,
				f.BigQueryFlags.BigQueryProject, f.BigQueryFlags.BigQueryDataset, f.BigQueryFlags.ReleasesTable)
			if err != nil {
				return fmt.Errorf("could not get bigquery client: %w", err)
			}

			backfiller := infrafailurebackfill.New(bqc, dbc, infrafailurebackfill.Options{
				Since:     f.Since,
				Days:      f.Days,
				DryRun:    f.DryRun,
				BatchSize: f.BatchSize,
			})

			stats, err := backfiller.Run(ctx)
			if err != nil {
				return fmt.Errorf("backfill failed: %w", err)
			}

			if stats.Errors > 0 {
				return fmt.Errorf("backfill completed with %d errors, see logs for details", stats.Errors)
			}
			log.Info("backfill-infra-failures completed successfully")
			return nil
		},
	}

	f.BindFlags(cmd.Flags())

	return cmd
}
