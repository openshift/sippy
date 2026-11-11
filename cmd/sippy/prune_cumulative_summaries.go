package main

import (
	"fmt"
	"time"

	"cloud.google.com/go/civil"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/openshift/sippy/pkg/dataloader/prowloader/pgwriter"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/flags"
)

// PruneCumulativeSummariesFlags contains the database and optional release
// selection for the prune-cumulative-summaries command.
type PruneCumulativeSummariesFlags struct {
	DBFlags   *flags.PostgresFlags
	Release   string
	BatchSize int
}

// NewPruneCumulativeSummariesFlags returns flags with the standard PostgreSQL
// connection defaults.
func NewPruneCumulativeSummariesFlags() *PruneCumulativeSummariesFlags {
	return &PruneCumulativeSummariesFlags{
		DBFlags:   flags.NewPostgresDatabaseFlags(),
		BatchSize: pgwriter.DefaultCumulativeSummaryPruneBatchSize,
	}
}

// BindFlags registers database connection settings and the optional release
// filter for pruning cumulative summary rows.
func (f *PruneCumulativeSummariesFlags) BindFlags(fs *pflag.FlagSet) {
	f.DBFlags.BindFlags(fs)
	fs.StringVar(&f.Release, "release", "", "Prune only this release (defaults to all configured releases)")
	fs.IntVar(&f.BatchSize, "batch-size", f.BatchSize, "Maximum key combinations to prune per transaction")
}

// NewPruneCumulativeSummariesCommand creates a command that prunes unchanged
// cumulative summary rows without carrying snapshots forward.
func NewPruneCumulativeSummariesCommand() *cobra.Command {
	f := NewPruneCumulativeSummariesFlags()
	cmd := &cobra.Command{
		Use:          "prune-cumulative-summaries",
		Short:        "Prune unchanged cumulative summary rows",
		Long:         "Prunes unchanged cumulative summary keys through each release's latest snapshot on or before tomorrow. Without --release, processes every configured release sequentially. Uses up to 10 parallel workers per release, with each transaction handling up to --batch-size combinations.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if f.BatchSize < 1 {
				return fmt.Errorf("--batch-size must be greater than zero")
			}

			dbc, err := f.DBFlags.GetDBClient()
			if err != nil {
				return fmt.Errorf("getting database client: %w", err)
			}

			var releases []string
			if f.Release == "" {
				if err := dbc.DB.WithContext(cmd.Context()).Model(&models.ReleaseDefinition{}).
					Order("major DESC, minor DESC, release DESC").
					Pluck("release", &releases).Error; err != nil {
					return fmt.Errorf("querying configured releases: %w", err)
				}
			} else {
				releases = []string{f.Release}
			}

			currentDate := civil.DateOf(time.Now().UTC())
			for _, release := range releases {
				result, err := pgwriter.PruneCumulativeSummaries(cmd.Context(), dbc, currentDate, release, f.BatchSize)
				if err != nil {
					return fmt.Errorf("pruning release %s: %w", release, err)
				}
				if result.Skipped {
					log.WithFields(log.Fields{
						"release":        release,
						"retention_from": result.RetentionStart,
					}).Info("skipping cumulative summary pruning because no recent snapshot was found")
					continue
				}
				log.WithFields(log.Fields{
					"release":        release,
					"pruned_rows":    result.RowsDeleted,
					"retention_from": result.RetentionStart,
					"through":        result.ThroughDate,
				}).Info("pruned cumulative summaries")
			}
			return nil
		},
	}
	f.BindFlags(cmd.Flags())
	return cmd
}
