package main

import (
	"fmt"
	"time"

	"cloud.google.com/go/civil"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openshift/sippy/pkg/flags"
	"github.com/openshift/sippy/pkg/sippyserver"
)

type BackfillFlags struct {
	DBFlags   *flags.PostgresFlags
	Table     string
	StartDate string
	EndDate   string
}

func NewBackfillFlags() *BackfillFlags {
	return &BackfillFlags{
		DBFlags: flags.NewPostgresDatabaseFlags(),
	}
}

func (f *BackfillFlags) BindFlags(fs *pflag.FlagSet) {
	f.DBFlags.BindFlags(fs)
	fs.StringVar(&f.Table, "table", "", "Table to backfill (daily-totals, cumulative-summaries)")
	fs.StringVar(&f.StartDate, "start-date", "", "Start date (YYYY-MM-DD)")
	fs.StringVar(&f.EndDate, "end-date", "", "End date (YYYY-MM-DD)")
}

func NewBackfillCommand() *cobra.Command {
	f := NewBackfillFlags()

	cmd := &cobra.Command{
		Use:   "backfill",
		Short: "Backfill a specific summary table for a date range",
		RunE: func(cmd *cobra.Command, args []string) error {
			validTables := sets.New[string]("daily-totals", "cumulative-summaries")
			if f.Table == "" {
				return fmt.Errorf("--table is required")
			}
			if !validTables.Has(f.Table) {
				return fmt.Errorf("invalid --table %q: must be one of %v", f.Table, sets.List(validTables))
			}
			if f.StartDate == "" {
				return fmt.Errorf("--start-date is required")
			}

			startDate, err := civil.ParseDate(f.StartDate)
			if err != nil {
				return fmt.Errorf("invalid --start-date %q: %w", f.StartDate, err)
			}

			endDate := civil.DateOf(time.Now().UTC())
			if f.EndDate != "" {
				endDate, err = civil.ParseDate(f.EndDate)
				if err != nil {
					return fmt.Errorf("invalid --end-date %q: %w", f.EndDate, err)
				}
			}

			if startDate.After(endDate) {
				return fmt.Errorf("--start-date (%s) is after --end-date (%s)", startDate, endDate)
			}

			dbc, err := f.DBFlags.GetDBClient()
			if err != nil {
				return fmt.Errorf("getting db client: %w", err)
			}

			log.WithFields(log.Fields{
				"table": f.Table,
				"start": startDate,
				"end":   endDate,
			}).Info("starting backfill")

			var releases []string
			if err := dbc.DB.Table("release_definitions").
				Order("major DESC, minor DESC").
				Pluck("release", &releases).Error; err != nil {
				return fmt.Errorf("querying releases: %w", err)
			}
			startTime := time.Date(startDate.Year, startDate.Month, startDate.Day, 0, 0, 0, 0, time.UTC)
			endTime := time.Date(endDate.Year, endDate.Month, endDate.Day, 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
			if _, err := dbc.EnsurePartitions(releases, startTime, endTime, false); err != nil {
				return fmt.Errorf("ensuring partitions: %w", err)
			}

			return sippyserver.BackfillData(dbc, f.Table, startDate, endDate)
		},
	}

	f.BindFlags(cmd.Flags())

	return cmd
}
