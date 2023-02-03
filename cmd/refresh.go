package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	gormlogger "gorm.io/gorm/logger"

	"github.com/openshift/sippy/pkg/db"
)

type RefreshFlags struct {
	DBFlags            *PostgresDatabaseFlags
	RefreshOnlyIfEmpty bool
}

func NewRefreshFlags() *RefreshFlags {
	return &RefreshFlags{
		DBFlags: NewPostgresDatabaseFlags(),
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
			dbc, err := db.New(f.DBFlags.DSN, gormlogger.LogLevel(f.DBFlags.LogLevel))
			if err != nil {
				fmt.Printf("could not connect to db: %+v", err)
				os.Exit(1)
			}

			dbc.RefreshData(f.RefreshOnlyIfEmpty)
		},
	}

	f.BindFlags(cmd.Flags())
	rootCmd.AddCommand(cmd)
}
