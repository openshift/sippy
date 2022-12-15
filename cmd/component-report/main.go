package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/openshift/sippy/cmd/component-report/componentreportserver"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/openshift/sippy/pkg/db"
)

const (
	defaultLogLevel = "info"
)

type Options struct {
	// TODO perhaps this could drive the synthetic tests too
	ListenAddr string
	DSN        string
	LogLevel   string
}

func main() {
	opt := &Options{
		ListenAddr: ":8080",
	}

	cmd := &cobra.Command{
		Run: func(cmd *cobra.Command, arguments []string) {
			opt.Complete()

			if err := opt.Validate(); err != nil {
				log.WithError(err).Fatalf("error validation options")
			}
			if err := opt.Run(); err != nil {
				log.WithError(err).Fatalf("error running command")
			}
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&opt.DSN, "database-dsn", os.Getenv("SIPPY_DATABASE_DSN"), "Database DSN for storage of some types of data")
	flags.StringVar(&opt.ListenAddr, "listen", opt.ListenAddr, "The address to serve analysis reports on (default :8080)")
	flags.StringVar(&opt.LogLevel, "log-level", defaultLogLevel, "Log level (trace,debug,info,warn,error)")

	if err := cmd.Execute(); err != nil {
		log.Fatalf("error: %v", err)
	}
}

func (o *Options) Complete() {
}

func (o *Options) Validate() error {
	if len(o.DSN) == 0 {
		return fmt.Errorf("must specify --database-dsn with --load-database and --server")
	}

	return nil
}

func (o *Options) Run() error { //nolint:gocyclo
	// Set log level
	level, err := log.ParseLevel(o.LogLevel)
	if err != nil {
		log.WithError(err).Fatal("Cannot parse log level")
	}
	log.SetLevel(level)

	// Add some millisecond precision to log timestamps, useful for debugging performance.
	formatter := new(log.TextFormatter)
	formatter.TimestampFormat = "2006-01-02T15:04:05.999Z07:00"
	formatter.FullTimestamp = true
	formatter.DisableColors = false
	log.SetFormatter(formatter)

	log.Debug("debug logging enabled")

	return o.runServerMode()
}

func (o *Options) runServerMode() error {
	var dbc *db.DB
	var err error
	dbc, err = db.New(o.DSN)
	if err != nil {
		return err
	}

	server := componentreportserver.NewServer(dbc)

	return http.ListenAndServe(o.ListenAddr, server)
}
