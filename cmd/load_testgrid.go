package cmd

import (
	"os"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/openshift/sippy/cmd/flags"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridhelpers"
)

type LoadTestGridFlags struct {
	TestGridFlags *flags.TestGridFlags
}

func NewLoadTestGridFlags() *LoadTestGridFlags {
	return &LoadTestGridFlags{
		TestGridFlags: flags.NewTestGridFlags(),
	}
}

func (f *LoadTestGridFlags) BindFlags(fs *pflag.FlagSet) {
	f.TestGridFlags.BindFlags(fs)
}

func init() {
	f := NewLoadTestGridFlags()

	cmd := &cobra.Command{
		Use:   "testgrid",
		Short: "Fetch data from TestGrid into local JSON files on disk",
		Run: func(cmd *cobra.Command, args []string) {
			start := time.Now()
			if err := os.MkdirAll(f.TestGridFlags.LocalData, os.ModePerm); err != nil {
				log.WithError(err).Fatal("could not create testgrid data dir")
			}
			dashboards := []string{}

			for _, dashboardCoordinate := range f.TestGridFlags.TestGridDashboardCoordinates() {
				dashboards = append(dashboards, dashboardCoordinate.TestGridDashboardNames...)
			}
			testgridhelpers.DownloadData(dashboards, f.TestGridFlags.JobFilter, f.TestGridFlags.LocalData)

			elapsed := time.Since(start)
			log.Infof("Testgrid data fetched in: %s", elapsed)
		},
	}

	f.BindFlags(cmd.Flags())
	loadCmd.AddCommand(cmd)
}
