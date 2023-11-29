package main

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"os"

	"cloud.google.com/go/bigquery"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"google.golang.org/api/option"
	"gopkg.in/yaml.v3"

	"github.com/openshift/sippy/pkg/apis/cache"
	v1 "github.com/openshift/sippy/pkg/apis/config/v1"
	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/cache/redis"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/gcs"
	"github.com/openshift/sippy/pkg/sippyserver"
)

type ComponentReadinessFlags struct {
	Config                             string
	LogLevel                           string
	ListenAddr                         string
	MetricsAddr                        string
	GoogleServiceAccountCredentialFile string
	RedisURL                           string
}

func NewComponentReadinessCommand() *cobra.Command {
	f := &ComponentReadinessFlags{
		LogLevel:    defaultLogLevel,
		ListenAddr:  ":8080",
		MetricsAddr: ":2112",
	}

	cmd := &cobra.Command{
		Use: "component-readiness",

		Run: func(cmd *cobra.Command, arguments []string) {

			if err := f.Validate(); err != nil {
				log.WithError(err).Fatalf("error validation options")
			}
			if err := f.Run(); err != nil {
				log.WithError(err).Fatalf("error running command")
			}
		},
	}

	f.BindFlags(cmd.Flags())

	return cmd
}

func (f *ComponentReadinessFlags) BindFlags(flags *pflag.FlagSet) {
	flags.StringVar(&f.LogLevel, "log-level", f.LogLevel, "Log level (trace,debug,info,warn,error) (default info)")

	// google cloud creds
	flags.StringVar(&f.GoogleServiceAccountCredentialFile, "google-service-account-credential-file", os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"), "location of a credential file described by https://cloud.google.com/docs/authentication/production")
}

func (f *ComponentReadinessFlags) Validate() error {
	if len(f.GoogleServiceAccountCredentialFile) == 0 {
		return fmt.Errorf("--google-service-account-credential-file is required")
	}
	return nil
}

func (f *ComponentReadinessFlags) Run() error { //nolint:gocyclo
	// Set log level
	level, err := log.ParseLevel(f.LogLevel)
	if err != nil {
		log.WithError(err).Fatal("Cannot parse log-level")
	}
	log.SetLevel(level)

	// Add some millisecond precision to log timestamps, useful for debugging performance.
	formatter := new(log.TextFormatter)
	formatter.TimestampFormat = "2006-01-02T15:04:05.999Z07:00"
	formatter.FullTimestamp = true
	formatter.DisableColors = false
	log.SetFormatter(formatter)

	log.Debug("debug logging enabled")
	sippyConfig := v1.SippyConfig{}
	if f.Config == "" {
		sippyConfig.Prow = v1.ProwConfig{
			URL: "https://prow.ci.openshift.org/prowjobs.js",
		}
	} else {
		data, err := os.ReadFile(f.Config)
		if err != nil {
			log.WithError(err).Fatalf("could not load config")
		}
		if err := yaml.Unmarshal(data, &sippyConfig); err != nil {
			log.WithError(err).Fatalf("could not unmarshal config")
		}
	}

	return f.runServerMode()
}

func (f *ComponentReadinessFlags) runServerMode() error {
	var err error

	webRoot, err := fs.Sub(sippyNG, "sippy-ng/build")
	if err != nil {
		return err
	}

	gcsClient, err := gcs.NewGCSClient(context.TODO(),
		f.GoogleServiceAccountCredentialFile,
		"",
	)
	if err != nil {
		log.WithError(err).Warn("unable to create GCS client, some APIs may not work")
	}

	var bigQueryClient bqcachedclient.Client
	if f.GoogleServiceAccountCredentialFile != "" {
		bigQueryClient.BQ, err = bigquery.NewClient(context.Background(), "openshift-gce-devel",
			option.WithCredentialsFile(f.GoogleServiceAccountCredentialFile))
		if err != nil {
			log.WithError(err).Error("CRITICAL error getting BigQuery client which prevents component readiness queries from working")
			return err
		}
		// Enable Storage API usage for fetching data
		err = bigQueryClient.BQ.EnableStorageReadClient(context.Background(), option.WithCredentialsFile(f.GoogleServiceAccountCredentialFile))
		if err != nil {
			log.WithError(err).Error("CRITICAL error enabling BigQuery Storage API")
			return err
		}
	}

	var cacheClient cache.Cache

	if f.RedisURL != "" {
		cacheClient, err = redis.NewRedisCache(f.RedisURL)
		if err != nil {
			log.WithError(err).Error("couldn't create redis cache")
			return err
		}

	}

	server := sippyserver.NewServer(
		sippyserver.ModeOpenShift,
		f.ListenAddr,
		nil,
		nil,
		webRoot,
		&static,
		nil,
		gcsClient,
		&bigQueryClient,
		nil,
		cacheClient,
	)

	// Serve our metrics endpoint for prometheus to scrape
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		err := http.ListenAndServe(f.MetricsAddr, nil) //nolint
		if err != nil {
			panic(err)
		}
	}()

	server.Serve()
	return nil
}
