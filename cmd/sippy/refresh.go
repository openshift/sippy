package main

import (
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/openshift/sippy/pkg/flags"
	"github.com/openshift/sippy/pkg/sippyserver"
)

type RefreshFlags struct {
	DBFlags            *flags.PostgresFlags
	CacheFlags         *flags.CacheFlags
	RefreshOnlyIfEmpty bool
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
			sippyserver.RefreshData(dbc, cacheClient, f.RefreshOnlyIfEmpty)
			return nil
		},
	}

	f.BindFlags(cmd.Flags())

	return cmd
}
