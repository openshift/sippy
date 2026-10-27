package main

import (
	"fmt"
	"time"

	"cloud.google.com/go/civil"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/flags"
	"github.com/openshift/sippy/pkg/sippyserver"
)

// BackfillFlags configures a summary-table repair for a date range and release scope.
type BackfillFlags struct {
	DBFlags          *flags.PostgresFlags
	Table            string
	StartDate        string
	EndDate          string
	ExcludedReleases []string
	Release          string
}

// NewBackfillFlags starts with the standard PostgreSQL connection flags.
func NewBackfillFlags() *BackfillFlags {
	return &BackfillFlags{
		DBFlags: flags.NewPostgresDatabaseFlags(),
	}
}

// BindFlags exposes the backfill date, table, and exclusion options.
func (f *BackfillFlags) BindFlags(fs *pflag.FlagSet) {
	f.DBFlags.BindFlags(fs)
	fs.StringVar(&f.Table, "table", "", "Table to backfill (daily-totals, cumulative-summaries)")
	fs.StringVar(&f.Release, "release", "", "Only backfill this release (default: all releases)")
	fs.StringVar(&f.StartDate, "start-date", "", "Start date (YYYY-MM-DD)")
	fs.StringVar(&f.EndDate, "end-date", "", "End date (YYYY-MM-DD; defaults to today, or tomorrow for cumulative summaries)")
	fs.StringArrayVar(&f.ExcludedReleases, "exclude-release", nil, "Release to skip; repeat for multiple releases (for example, Presubmits)")
}

// NewBackfillCommand resolves the included releases once for partitions and summaries.
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
			} else if f.Table == "cumulative-summaries" {
				endDate = endDate.AddDays(1)
			}

			if startDate.After(endDate) {
				return fmt.Errorf("--start-date (%s) is after --end-date (%s)", startDate, endDate)
			}

			dbc, err := f.DBFlags.GetDBClient()
			if err != nil {
				return fmt.Errorf("getting db client: %w", err)
			}

			log.WithFields(log.Fields{
				"table":             f.Table,
				"release":           f.Release,
				"start":             startDate,
				"end":               endDate,
				"excluded_releases": f.ExcludedReleases,
			}).Info("starting backfill")

			var releases []string
			if err := dbc.DB.Table("release_definitions").
				Order("major DESC, minor DESC").
				Pluck("release", &releases).Error; err != nil {
				return fmt.Errorf("querying releases: %w", err)
			}
			releaseSet := sets.New(releases...)
			if f.Release != "" {
				if !releaseSet.Has(f.Release) {
					return fmt.Errorf("release %q not found in release_definitions", f.Release)
				}
				releases = []string{f.Release}
			}
			for _, excluded := range f.ExcludedReleases {
				if !releaseSet.Has(excluded) {
					log.WithField("release", excluded).Warn("excluded release is not present in release definitions")
				}
			}
			releases = db.ExcludeReleases(releases, f.ExcludedReleases)
			startTime := time.Date(startDate.Year, startDate.Month, startDate.Day, 0, 0, 0, 0, time.UTC)
			endTime := time.Date(endDate.Year, endDate.Month, endDate.Day, 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
			if _, err := dbc.EnsurePartitions(releases, startTime, endTime, false); err != nil {
				return fmt.Errorf("ensuring partitions: %w", err)
			}

			return sippyserver.BackfillData(dbc, f.Table, startDate, endDate, releases)
		},
	}

	f.BindFlags(cmd.Flags())

	return cmd
}
