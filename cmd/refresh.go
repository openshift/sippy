package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/openshift/sippy/cmd/flags"
)

type RefreshFlags struct {
	DBFlags            *flags.PostgresDatabaseFlags
	RefreshOnlyIfEmpty bool
}

func NewRefreshFlags() *RefreshFlags {
	return &RefreshFlags{
		DBFlags: flags.NewPostgresDatabaseFlags(),
	}
}

func (f *RefreshFlags) BindFlags(fs *pflag.FlagSet) {
	f.DBFlags.BindFlags(fs)
	fs.BoolVar(&f.RefreshOnlyIfEmpty, "refresh-only-if-empty", f.RefreshOnlyIfEmpty, "only refresh matviews if they're empty")
}

func init() {
	f := NewRefreshFlags()

	cmd := &cobra.Command{
		Use:   "refresh",
		Short: "Refresh data in database such as materialized views",
		Run: func(cmd *cobra.Command, args []string) {
			dbc := f.DBFlags.GetDBClient()
			dbc.RefreshData(f.RefreshOnlyIfEmpty)
		},
	}

	f.BindFlags(cmd.Flags())
	rootCmd.AddCommand(cmd)
}
