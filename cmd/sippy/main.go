package main

import (
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func init() {
	// Sippy operates exclusively in UTC. This must be set before any time
	// function runs so that libraries like pgx, which internally call
	// time.Unix() (returning time.Local), produce time.Time values with
	// the UTC location instead of the host's local timezone.
	time.Local = time.UTC
}

var logLevel = "info"

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "sippy",
	Short: "CIPI (Continuous Integration Private Investigator) aka Sippy",
	Long: `Sippy reports on job and test statistics, sliced by various filters
including name, suite, or NURP+ variants (network, upgrade, release,
platform, etc).`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		PrintVersion(cmd, args)
		level, err := log.ParseLevel(logLevel)
		if err != nil {
			log.WithError(err).Fatal("cannot parse log-level")
		}
		log.SetLevel(level)
		log.Debug("debug logging enabled")
	},
}

func main() {

	// Add some millisecond precision to log timestamps, useful for debugging performance.
	formatter := new(log.TextFormatter)
	formatter.TimestampFormat = "2006-01-02T15:04:05.999Z07:00"
	formatter.FullTimestamp = true
	formatter.DisableColors = false
	log.SetFormatter(formatter)

	rootCmd.AddCommand(
		NewServeCommand(),
		NewLoadCommand(),
		NewSnapshotCommand(),
		NewRefreshCommand(),
		NewBackfillCommand(),
		NewBackfillInfraFailuresCommand(),
		NewComponentReadinessCommand(),
		NewAutomateJiraCommand(),
		NewVariantsCommand(),
		NewVersionCommand(),
		NewAnnotateJobRunsCommand(),
		NewSeedDataCommand(),
	)

	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info",
		"Log level (trace,debug,info,warn,error) (default info)")

	err := rootCmd.Execute()
	if err != nil {
		log.WithError(err).Fatal("could not execute root command")
	}
}
