package cmd

import (
	"fmt"
	"os"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	gormlogger "gorm.io/gorm/logger"

	"github.com/openshift/sippy/cmd/flags"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/sippyserver"
)

type LoadBugsFlags struct {
	DBFlags *flags.PostgresDatabaseFlags
}

func NewLoadBugsFlags() *LoadBugsFlags {
	return &LoadBugsFlags{
		DBFlags: flags.NewPostgresDatabaseFlags(),
	}
}

func (f *LoadBugsFlags) BindFlags(fs *pflag.FlagSet) {
	f.DBFlags.BindFlags(fs)
}

func init() {
	f := NewLoadBugsFlags()

	cmd := &cobra.Command{
		Use:   "bugs",
		Short: "Load bugs from search.ci.openshift.org",
		Run: func(cmd *cobra.Command, args []string) {
			dbc, err := db.New(f.DBFlags.DSN, gormlogger.LogLevel(f.DBFlags.LogLevel))
			if err != nil {
				fmt.Printf("could not connect to db: %+v", err)
				os.Exit(1)
			}

			bugsStart := time.Now()
			allErrs := sippyserver.LoadBugs(dbc)
			bugsElapsed := time.Since(bugsStart)
			log.Infof("Bugs loaded from search.ci in %s", bugsElapsed)

			// Update the tests watchlist flag. Anything matching one of our configured
			// regexes, or any test linked to a jira with a particular label will land on the watchlist
			// for easier viewing in the UI.
			watchlistErrs := sippyserver.UpdateWatchlist(dbc)
			allErrs = append(allErrs, watchlistErrs...)

			if len(allErrs) > 0 {
				log.Warningf("%d errors were encountered while loading bugs:", len(allErrs))
				for _, err := range allErrs {
					log.Error(err.Error())
				}
				os.Exit(1)
			}
		},
	}

	f.BindFlags(cmd.Flags())
	cmd.MarkFlagRequired("releases")      //nolint:errcheck
	cmd.MarkFlagRequired("architectures") //nolint:errcheck
	loadCmd.AddCommand(cmd)
}
