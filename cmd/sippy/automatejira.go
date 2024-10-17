package main

import (
	"context"
	"flag"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/openshift/sippy/pkg/api"
	"github.com/openshift/sippy/pkg/api/componentreadiness"
	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/cache"
	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/componentreadiness/jiraautomator"
	"github.com/openshift/sippy/pkg/flags"
	"github.com/openshift/sippy/pkg/util/sets"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	prowflagutil "sigs.k8s.io/prow/pkg/flagutil"
)

type AutomateJiraFlags struct {
	BigQueryFlags                                *flags.BigQueryFlags
	GoogleCloudFlags                             *flags.GoogleCloudFlags
	CacheFlags                                   *flags.CacheFlags
	ComponentReadinessFlags                      *flags.ComponentReadinessFlags
	JiraOptions                                  prowflagutil.JiraOptions
	SippyURL                                     string
	ComponentWhiteListStr                        string
	ComponentWhiteList                           sets.String
	VariantBasedComponentRegressionThresholdStrs []string
	VariantBasedComponentRegressionThresholds    map[jiraautomator.Variant]int
	JiraAccount                                  string
}

func NewAutomateJiraFlags() *AutomateJiraFlags {
	return &AutomateJiraFlags{
		BigQueryFlags:           flags.NewBigQueryFlags(),
		GoogleCloudFlags:        flags.NewGoogleCloudFlags(),
		CacheFlags:              flags.NewCacheFlags(),
		ComponentReadinessFlags: flags.NewComponentReadinessFlags(),
		VariantBasedComponentRegressionThresholds: map[jiraautomator.Variant]int{},
	}
}

func (f *AutomateJiraFlags) BindFlags(fs *pflag.FlagSet) {
	f.BigQueryFlags.BindFlags(fs)
	f.GoogleCloudFlags.BindFlags(fs)
	f.CacheFlags.BindFlags(fs)
	f.ComponentReadinessFlags.BindFlags(fs)
	f.JiraOptions.AddFlags(flag.CommandLine)
	fs.AddGoFlagSet(flag.CommandLine)
	fs.StringVar(&f.SippyURL, "sippy-url", f.SippyURL, "The Sippy URL prefix to be used to generate sharable Sippy links")
	fs.StringVar(&f.ComponentWhiteListStr, "component-white-list", f.ComponentWhiteListStr, "The whitelist of comma seperated jira components to file issues against")
	fs.StringArrayVar(&f.VariantBasedComponentRegressionThresholdStrs, "variant-based-component-regression-threshold", f.VariantBasedComponentRegressionThresholdStrs, "A list of variants which we will check for a column of red cells in component readiness report. If the number of red cells of a relevant column is over the threshold, we will a create jira card for the component corresponding to the specified variant (e.g. Bare Metal Hardware Provisioning for metal platform. The format of string is [variant]:[value]:[threshold] (e.g. Platform:metal:3)")
	fs.StringVar(&f.JiraAccount, "jira-account", f.JiraAccount, "The jira account used to automate jira")
}

func (f *AutomateJiraFlags) Validate(allVariants crtype.JobVariants) error {
	if len(f.SippyURL) == 0 {
		return fmt.Errorf("--sippy-url is required")
	}
	if len(f.JiraAccount) == 0 {
		return fmt.Errorf("--jira-account is required")
	}
	f.ComponentWhiteList = sets.NewString(strings.Split(f.ComponentWhiteListStr, ",")...)
	for _, variantThreshold := range f.VariantBasedComponentRegressionThresholdStrs {
		vt := strings.Split(variantThreshold, ":")
		if len(vt) != 3 {
			return fmt.Errorf("--variant-based-component-regression-threshold %s is in wrong format", variantThreshold)
		}
		vs, ok := allVariants.Variants[vt[0]]
		if !ok {
			return fmt.Errorf("--variant-based-component-regression-threshold %s has wrong variant name %s", variantThreshold, vt[0])
		}
		found := false
		for _, v := range vs {
			if v == vt[1] {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("--variant-based-component-regression-threshold %s has wrong variant value %s", variantThreshold, vt[1])
		}
		t, err := strconv.Atoi(vt[2])
		if err != nil {
			return fmt.Errorf("--variant-based-component-regression-threshold %s has wrong threshold %s", variantThreshold, vt[2])
		}
		f.VariantBasedComponentRegressionThresholds[jiraautomator.Variant{Name: vt[0], Value: vt[1]}] = t
	}
	return f.JiraOptions.Validate(true)
}

func NewAutomateJiraCommand() *cobra.Command {
	f := NewAutomateJiraFlags()

	cmd := &cobra.Command{
		Use:   "automate-jira",
		Short: "Automate jira with component readiness regressions",
		Long:  "Check the component report for each view with automate jira enabled. Maintains jira cards for current regressions automatically.",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), time.Hour*1)
			defer cancel()

			cacheClient, err := f.CacheFlags.GetCacheClient()
			if err != nil {
				log.WithError(err).Fatal("couldn't get cache client")
			}

			bigQueryClient, err := bqcachedclient.New(ctx,
				f.GoogleCloudFlags.ServiceAccountCredentialFile,
				f.BigQueryFlags.BigQueryProject,
				f.BigQueryFlags.BigQueryDataset, cacheClient)
			if err != nil {
				log.WithError(err).Fatal("CRITICAL error getting BigQuery client which prevents regression tracking")
			}

			cacheOpts := cache.RequestOptions{CRTimeRoundingFactor: f.ComponentReadinessFlags.CRTimeRoundingFactor}

			views, err := f.ComponentReadinessFlags.ParseViewsFile()
			if err != nil {
				log.WithError(err).Fatal("unable to load views")
			}
			releases, err := api.GetReleases(nil, bigQueryClient)
			if err != nil {
				log.WithError(err).Fatal("error querying releases")
			}

			jiraClient, err := f.JiraOptions.Client()
			if err != nil {
				return errors.WithMessage(err, "couldn't get jira client")
			}

			allVariants, errs := componentreadiness.GetJobVariantsFromBigQuery(bigQueryClient, f.GoogleCloudFlags.StorageBucket)
			if len(errs) > 0 {
				return fmt.Errorf("failed to get variants from bigquery")
			}
			if err := f.Validate(allVariants); err != nil {
				return errors.WithMessage(err, "error validating options")
			}

			j, err := jiraautomator.NewJiraAutomator(jiraClient, bigQueryClient, cacheOpts, views.ComponentReadiness, releases, f.SippyURL, f.JiraAccount, f.ComponentWhiteList, f.VariantBasedComponentRegressionThresholds)
			if err != nil {
				panic(err)
			}
			return j.Run()
		},
	}

	f.BindFlags(cmd.Flags())

	return cmd
}
