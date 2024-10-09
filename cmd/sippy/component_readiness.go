package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cloud.google.com/go/storage"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"
	prowflagutil "sigs.k8s.io/prow/pkg/flagutil"

	resources "github.com/openshift/sippy"
	"github.com/openshift/sippy/pkg/api"
	"github.com/openshift/sippy/pkg/apis/cache"
	v1 "github.com/openshift/sippy/pkg/apis/config/v1"
	"github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/componentreadiness/jiraintegrator"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/gcs"
	"github.com/openshift/sippy/pkg/flags"
	"github.com/openshift/sippy/pkg/sippyserver"
	"github.com/openshift/sippy/pkg/sippyserver/metrics"
)

type ComponentReadinessFlags struct {
	GoogleCloudFlags        *flags.GoogleCloudFlags
	BigQueryFlags           *flags.BigQueryFlags
	CacheFlags              *flags.CacheFlags
	ProwFlags               *flags.ProwFlags
	ComponentReadinessFlags *flags.ComponentReadinessFlags
	JiraOptions             prowflagutil.JiraOptions

	Config        string
	LogLevel      string
	ListenAddr    string
	MetricsAddr   string
	RedisURL      string
	SippyURL      string
	IntegrateJira bool
	// TODO: remove this, now a param on a view
	MaintainRegressionTables bool
}

func NewComponentReadinessCommand() *cobra.Command {
	f := &ComponentReadinessFlags{
		LogLevel:    "info",
		ListenAddr:  ":8080",
		MetricsAddr: ":2112",

		ProwFlags:               flags.NewProwFlags(),
		GoogleCloudFlags:        flags.NewGoogleCloudFlags(),
		BigQueryFlags:           flags.NewBigQueryFlags(),
		CacheFlags:              flags.NewCacheFlags(),
		ComponentReadinessFlags: flags.NewComponentReadinessFlags(),
	}

	cmd := &cobra.Command{
		Use: "component-readiness",

		RunE: func(cmd *cobra.Command, arguments []string) error {

			if err := f.Validate(); err != nil {
				return errors.WithMessage(err, "error validating options")
			}
			if err := f.Run(); err != nil {
				return errors.WithMessage(err, "error running command")
			}
			cmd.Context()

			return nil
		},
	}

	f.BindFlags(cmd.Flags())

	return cmd
}

func (f *ComponentReadinessFlags) BindFlags(flagSet *pflag.FlagSet) {
	f.CacheFlags.BindFlags(flagSet)
	f.BigQueryFlags.BindFlags(flagSet)
	f.GoogleCloudFlags.BindFlags(flagSet)
	f.ProwFlags.BindFlags(flagSet)
	f.ComponentReadinessFlags.BindFlags(flagSet)
	f.JiraOptions.AddFlags(flag.CommandLine)
	flagSet.AddGoFlagSet(flag.CommandLine)
	flagSet.StringVar(&f.LogLevel, "log-level", f.LogLevel, "Log level (trace,debug,info,warn,error) (default info)")
	flagSet.StringVar(&f.ListenAddr, "listen", f.ListenAddr, "The address to serve analysis reports on (default :8080)")
	flagSet.StringVar(&f.MetricsAddr, "listen-metrics", f.MetricsAddr, "The address to serve prometheus metrics on (default :2112)")
	flagSet.BoolVar(&f.MaintainRegressionTables, "maintain-regression-tables", false, "Enable maintenance of open regressions table in bigquery.")
	flagSet.BoolVar(&f.IntegrateJira, "integrate-jira", false, "Enable automatic integration with Jira by using Component Readiness result.")
	flagSet.StringVar(&f.SippyURL, "sippy-url", f.SippyURL, "The Sippy URL prefix to be used to generate sharable Sippy links")
}

func (f *ComponentReadinessFlags) Validate() error {
	err := f.JiraOptions.Validate(true)
	if err != nil {
		return err
	}
	if f.IntegrateJira && len(f.SippyURL) == 0 {
		return fmt.Errorf("--sippy-url missing when --integrate-jira is specified")
	}
	return f.ProwFlags.Validate()
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
			URL: f.ProwFlags.URL + "/prowjobs.js",
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

	webRoot, err := fs.Sub(resources.SippyNG, "sippy-ng/build")
	if err != nil {
		log.WithError(err).Fatal("could not load frontend")
	}

	cacheClient, err := f.CacheFlags.GetCacheClient()
	if err != nil {
		return errors.WithMessage(err, "couldn't get cache client")
	}

	var bigQueryClient *bigquery.Client
	var gcsClient *storage.Client
	if f.GoogleCloudFlags.ServiceAccountCredentialFile != "" {
		bigQueryClient, err = f.BigQueryFlags.GetBigQueryClient(context.Background(),
			cacheClient, f.GoogleCloudFlags.ServiceAccountCredentialFile)
		if err != nil {
			return errors.WithMessage(err, "couldn't get bigquery client")
		}

		gcsClient, err = gcs.NewGCSClient(context.TODO(),
			f.GoogleCloudFlags.ServiceAccountCredentialFile,
			f.GoogleCloudFlags.OAuthClientCredentialFile,
		)
		if err != nil {
			log.WithError(err).Warn("unable to create GCS client, some APIs may not work")
		}
	}

	views, err := f.ComponentReadinessFlags.ParseViewsFile()
	if err != nil {
		log.WithError(err).Fatal("unable to load views")

	}

	server := sippyserver.NewServer(
		sippyserver.ModeOpenShift,
		f.ListenAddr,
		nil,
		nil,
		webRoot,
		&resources.Static,
		nil,
		f.ProwFlags.URL,
		f.GoogleCloudFlags.StorageBucket,
		gcsClient,
		bigQueryClient,
		nil,
		cacheClient,
		f.ComponentReadinessFlags.CRTimeRoundingFactor,
		views,
	)

	ctx, cancelFn := context.WithCancel(context.Background())
	defer cancelFn()
	abortCh := make(chan os.Signal, 1)
	go func() {
		sig := <-abortCh
		cancelFn()
		switch sig {
		case syscall.SIGINT:
			os.Exit(130)
		default:
			os.Exit(0)
		}
	}()
	signal.Notify(abortCh, syscall.SIGINT, syscall.SIGTERM)

	if f.MetricsAddr != "" {
		// Do an immediate metrics update
		err = metrics.RefreshMetricsDB(nil,
			bigQueryClient,
			f.ProwFlags.URL,
			f.GoogleCloudFlags.StorageBucket,
			nil,
			time.Time{},
			cache.RequestOptions{CRTimeRoundingFactor: f.ComponentReadinessFlags.CRTimeRoundingFactor},
			views.ComponentReadiness,
			f.MaintainRegressionTables)
		if err != nil {
			log.WithError(err).Error("error refreshing metrics")
		}

		// Refresh our metrics every 5 minutes:
		ticker := time.NewTicker(5 * time.Minute)
		go func() {
			for {
				select {
				case <-ticker.C:
					log.Info("tick")
					err := metrics.RefreshMetricsDB(
						nil,
						bigQueryClient,
						f.ProwFlags.URL,
						f.GoogleCloudFlags.StorageBucket,
						nil,
						time.Time{},
						cache.RequestOptions{CRTimeRoundingFactor: f.ComponentReadinessFlags.CRTimeRoundingFactor},
						views.ComponentReadiness,
						f.MaintainRegressionTables)
					if err != nil {
						log.WithError(err).Error("error refreshing metrics")
					}
				case <-ctx.Done():
					ticker.Stop()
					return
				}
			}
		}()

		// Serve our metrics endpoint for prometheus to scrape
		go func() {
			http.Handle("/metrics", promhttp.Handler())
			err := http.ListenAndServe(f.MetricsAddr, nil) //nolint
			if err != nil {
				panic(err)
			}
		}()

		if f.IntegrateJira {
			jiraClient, err := f.JiraOptions.Client()
			if err != nil {
				return errors.WithMessage(err, "couldn't get jira client")
			}
			releases, err := api.GetReleases(nil, bigQueryClient)
			if err != nil {
				return errors.WithMessage(err, "couldn't get releases")
			}
			j, err := jiraintegrator.NewJiraIntegrator(jiraClient, bigQueryClient, f.ProwFlags.URL, f.GoogleCloudFlags.StorageBucket,
				cache.RequestOptions{CRTimeRoundingFactor: f.ComponentReadinessFlags.CRTimeRoundingFactor}, views.ComponentReadiness, releases, f.SippyURL)
			if err != nil {
				panic(err)
			}

			// Do an immediate jira integration
			err = j.IntegrateJira()
			if err != nil {
				log.WithError(err).Error("error integrating with jira")
			}
			ticker := time.NewTicker(24 * time.Hour)
			go func() {
				for {
					select {
					case <-ticker.C:
						err = j.IntegrateJira()
						if err != nil {
							log.WithError(err).Error("error integrating with jira")
						}
					case <-ctx.Done():
						ticker.Stop()
						return
					}
				}
			}()
		}
	}

	server.Serve()
	return nil
}
