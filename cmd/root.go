package cmd

import (
	"io/fs"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "sippy",
	Short: "CIPI (Continuous Integration Private Investigator) aka Sippy",
	Long: `Sippy reports on job and test statistics, sliced by various filters
including name, suite, or NURP+ variants (network, upgrade, release,
platform, etc).`,
}

var frontendFS, staticFS fs.FS
var logLevel string

func Execute(frontend, staticAssets fs.FS) {
	frontendFS = frontend
	staticFS = staticAssets

	// Set log level
	level, err := log.ParseLevel(logLevel)
	if err != nil {
		log.WithError(err).Fatal("cannot parse log-level")
	}
	log.SetLevel(level)

	err = rootCmd.Execute()
	if err != nil {
		log.WithError(err).Fatal("could not execute root command")
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info",
		"Log level (trace,debug,info,warn,error) (default info)")
}
