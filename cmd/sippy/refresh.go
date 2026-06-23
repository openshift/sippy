package main

import (
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/openshift/sippy/pkg/db/dailysummary"
	"github.com/openshift/sippy/pkg/flags"
	"github.com/openshift/sippy/pkg/sippyserver"
)

type RefreshFlags struct {
	DBFlags                 *flags.PostgresFlags
	CacheFlags              *flags.CacheFlags
	RefreshOnlyIfEmpty      bool
	RebuildDailySummaries   bool
	DailySummariesStartDate string
	DailySummariesEndDate   string
}

func NewRefreshFlags() *RefreshFlags {
	return &RefreshFlags{
		DBFlags:    flags.NewPostgresDatabaseFlags(),
		CacheFlags: flags.NewCacheFlags(),
	}
}

func (f *RefreshFlags) BindFlags(fs *pflag.FlagSet) {
	f.DBFlags.BindFlags(fs)
	f.CacheFlags.BindFlags(fs)
	fs.BoolVar(&f.RefreshOnlyIfEmpty, "refresh-only-if-empty", f.RefreshOnlyIfEmpty, "only refresh matviews if they're empty")
	fs.BoolVar(&f.RebuildDailySummaries, "rebuild-daily-summaries", false, "Truncate and rebuild the daily summaries table for the default lookback period (use --daily-summaries-start for older data)")
	fs.StringVar(&f.DailySummariesStartDate, "daily-summaries-start", "", "Override start date for daily summaries (YYYY-MM-DD)")
	fs.StringVar(&f.DailySummariesEndDate, "daily-summaries-end", "", "Override end date for daily summaries (YYYY-MM-DD)")
}

func (f *RefreshFlags) dailySummaryOptions() (dailysummary.Options, error) {
	opts := dailysummary.Options{Rebuild: f.RebuildDailySummaries}
	if f.DailySummariesStartDate != "" {
		t, err := time.Parse("2006-01-02", f.DailySummariesStartDate)
		if err != nil {
			return opts, fmt.Errorf("invalid --daily-summaries-start %q: %w", f.DailySummariesStartDate, err)
		}
		opts.StartOverride = &t
	}
	if f.DailySummariesEndDate != "" {
		t, err := time.Parse("2006-01-02", f.DailySummariesEndDate)
		if err != nil {
			return opts, fmt.Errorf("invalid --daily-summaries-end %q: %w", f.DailySummariesEndDate, err)
		}
		opts.EndOverride = &t
	}
	if opts.StartOverride != nil && opts.EndOverride != nil && opts.StartOverride.After(*opts.EndOverride) {
		return opts, fmt.Errorf("--daily-summaries-start (%s) is after --daily-summaries-end (%s)",
			f.DailySummariesStartDate, f.DailySummariesEndDate)
	}
	return opts, nil
}

func NewRefreshCommand() *cobra.Command {
	f := NewRefreshFlags()

	cmd := &cobra.Command{
		Use:   "refresh",
		Short: "Refresh data in database such as materialized views",
		RunE: func(cmd *cobra.Command, args []string) error {
			dbc, err := f.DBFlags.GetDBClient()
			if err != nil {
				return err
			}
			cacheClient, cacheErr := f.CacheFlags.GetCacheClient()
			if cacheErr != nil {
				logrus.WithError(cacheErr).Warn("failed to get cache client")
				cacheClient = nil
			} else if cacheClient == nil {
				logrus.Warn("no cache provided; refresh will not update cached timestamps, so cached data may not be properly invalidated")
			}
			dailySummaryOpts, err := f.dailySummaryOptions()
			if err != nil {
				return err
			}
			sippyserver.RefreshData(dbc, cacheClient, f.RefreshOnlyIfEmpty, dailySummaryOpts)
			return nil
		},
	}

	f.BindFlags(cmd.Flags())

	return cmd
}
