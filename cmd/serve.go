package cmd

import (
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	gormlogger "gorm.io/gorm/logger"

	"github.com/openshift/sippy/cmd/flags"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/sippyserver"
	"github.com/openshift/sippy/pkg/sippyserver/metrics"
	"github.com/openshift/sippy/pkg/util"
)

type ServerFlags struct {
	DBFlags       *flags.PostgresDatabaseFlags
	ModeFlags     *flags.ModeFlags
	TestGridFlags *flags.TestGridFlags
	ListenAddr    string
	MetricsAddr   string
}

func NewServerFlags() *ServerFlags {
	return &ServerFlags{
		DBFlags:       flags.NewPostgresDatabaseFlags(),
		ModeFlags:     flags.NewModeFlags(),
		TestGridFlags: flags.NewTestGridFlags(),
		ListenAddr:    ":8080",
		MetricsAddr:   ":2112",
	}
}

func (f *ServerFlags) BindFlags(fs *pflag.FlagSet) {
	f.DBFlags.BindFlags(fs)
	f.ModeFlags.BindFlags(fs)
	f.TestGridFlags.BindFlags(fs)
	fs.StringVar(&f.ListenAddr, "listen", f.ListenAddr, "The address to serve analysis reports on (default :8080)")
	fs.StringVar(&f.MetricsAddr, "listen-metrics", f.MetricsAddr, "The address to serve prometheus metrics on (default :2112)")
}

func init() {
	f := NewServerFlags()

	cmd := &cobra.Command{
		Use:   "releases",
		Short: "Load releases from the OpenShift release controllers",
		Run: func(cmd *cobra.Command, args []string) {
			dbc, err := db.New(f.DBFlags.DSN, gormlogger.LogLevel(f.DBFlags.LogLevel))
			if err != nil {
				fmt.Printf("could not connect to db: %+v", err)
				os.Exit(1)
			}

			// Make sure the db is intialized, otherwise let the user know:
			prowJobs := []models.ProwJob{}
			res := dbc.DB.Find(&prowJobs).Limit(1)
			if res.Error != nil {
				log.WithError(res.Error).Fatal("error querying for a ProwJob, database may need to be initialized with --init-database")
			}

			webRoot, err := fs.Sub(frontendFS, "sippy-ng/build")
			if err != nil {
				log.WithError(err).Fatal("could not load frontend")
			}

			pinnedTime := time.Time(f.DBFlags.PinnedTime)

			server := sippyserver.NewServer(
				f.ModeFlags.GetServerMode(),
				f.TestGridFlags.TestGridLoadingConfig(),
				f.TestGridFlags.RawJobResultsAnalysisConfig(),
				f.TestGridFlags.DisplayDataConfig(),
				f.TestGridFlags.TestGridDashboardCoordinates(),
				f.ListenAddr,
				f.ModeFlags.GetSyntheticTestManager(),
				f.ModeFlags.GetVariantManager(),
				webRoot,
				staticFS,
				dbc,
				&pinnedTime,
			)

			// Do an immediate metrics update
			err = metrics.RefreshMetricsDB(dbc, util.GetReportEnd(&pinnedTime))
			if err != nil {
				log.WithError(err).Error("error refreshing metrics")
			}

			// Refresh our metrics every 5 minutes:
			ticker := time.NewTicker(5 * time.Minute)
			quit := make(chan struct{})
			go func() {
				for {
					select {
					case <-ticker.C:
						log.Info("tick")
						err := metrics.RefreshMetricsDB(dbc, util.GetReportEnd(&pinnedTime))
						if err != nil {
							log.WithError(err).Error("error refreshing metrics")
						}
					case <-quit:
						ticker.Stop()
						return
					}
				}
			}()

			// Serve our metrics endpoint for prometheus to scrape
			go func() {
				http.Handle("/metrics", promhttp.Handler())
				err := http.ListenAndServe(f.MetricsAddr, nil)
				if err != nil {
					panic(err)
				}
			}()

			server.Serve()
		},
	}

	f.BindFlags(cmd.Flags())
	cmd.MarkFlagRequired("releases")
	cmd.MarkFlagRequired("architectures")
	loadCmd.AddCommand(cmd)
}
