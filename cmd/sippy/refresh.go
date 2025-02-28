package main

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/openshift/sippy/pkg/flags"
	"github.com/openshift/sippy/pkg/sippyserver"
)

type RefreshFlags struct {
	DBFlags            *flags.PostgresFlags
	RefreshOnlyIfEmpty bool
}

func NewRefreshFlags() *RefreshFlags {
	return &RefreshFlags{
		DBFlags: flags.NewPostgresDatabaseFlags(""),
	}
}

func (f *RefreshFlags) BindFlags(fs *pflag.FlagSet) {
	f.DBFlags.BindFlags(fs)
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
			pinnedDateTime := f.DBFlags.GetPinnedTime()
			sippyserver.RefreshData(dbc, pinnedDateTime, f.RefreshOnlyIfEmpty)
			return nil
		},
	}

	f.BindFlags(cmd.Flags())

	return cmd
}
