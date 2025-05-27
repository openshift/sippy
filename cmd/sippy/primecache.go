package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/openshift/sippy/pkg/api"
	"github.com/openshift/sippy/pkg/api/componentreadiness"
	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/cache"
	configv1 "github.com/openshift/sippy/pkg/apis/config/v1"
	apiv1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/flags/configflags"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/openshift/sippy/pkg/flags"
)

type PrimeCacheFlags struct {
	BigQueryFlags           *flags.BigQueryFlags
	PostgresFlags           *flags.PostgresFlags
	GoogleCloudFlags        *flags.GoogleCloudFlags
	CacheFlags              *flags.CacheFlags
	ComponentReadinessFlags *flags.ComponentReadinessFlags
	ConfigFlags             *configflags.ConfigFlags
}

func NewPrimeCacheFlags() *PrimeCacheFlags {
	return &PrimeCacheFlags{
		BigQueryFlags:           flags.NewBigQueryFlags(),
		PostgresFlags:           flags.NewPostgresDatabaseFlags(),
		GoogleCloudFlags:        flags.NewGoogleCloudFlags(),
		CacheFlags:              flags.NewCacheFlags(),
		ComponentReadinessFlags: flags.NewComponentReadinessFlags(),
		ConfigFlags:             configflags.NewConfigFlags(),
	}
}

func (f *PrimeCacheFlags) BindFlags(fs *pflag.FlagSet) {
	f.BigQueryFlags.BindFlags(fs)
	f.PostgresFlags.BindFlags(fs)
	f.GoogleCloudFlags.BindFlags(fs)
	f.CacheFlags.BindFlags(fs)
	f.ComponentReadinessFlags.BindFlags(fs)
	f.ConfigFlags.BindFlags(fs)
}

func (f *PrimeCacheFlags) Validate() error {
	return f.GoogleCloudFlags.Validate()
}

func NewPrimeCacheCommand() *cobra.Command {
	f := NewPrimeCacheFlags()

	cmd := &cobra.Command{
		Use:   "prime-cache",
		Short: "Prime the cache for all views with tracking enabled",
		Long:  "Primes the cache for views with tracking enabled, both top level report as well as all test details reports for regressed tets.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := f.Validate(); err != nil {
				return errors.WithMessage(err, "error validating options")
			}
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

			config, err := f.ConfigFlags.GetConfig()
			if err != nil {
				log.WithError(err).Warn("error reading config file")
			}

			if bigQueryClient != nil && f.CacheFlags.EnablePersistentCaching {
				bigQueryClient = f.CacheFlags.DecorateBiqQueryClientWithPersistentCache(bigQueryClient)
			}

			cacheOpts := cache.RequestOptions{
				CRTimeRoundingFactor: f.ComponentReadinessFlags.CRTimeRoundingFactor,
				// Force a refresh, we want to ensure we update the cache no matter what
				ForceRefresh: true,
			}

			views, err := f.ComponentReadinessFlags.ParseViewsFile()
			if err != nil {
				log.WithError(err).Fatal("unable to load views")
			}
			releases, err := api.GetReleases(context.TODO(), bigQueryClient)
			if err != nil {
				log.WithError(err).Fatal("error querying releases")
			}
			dbc, err := f.PostgresFlags.GetDBClient()
			if err != nil {
				log.WithError(err).Fatal("unable to connect to postgres")
			}

			for _, view := range views.ComponentReadiness {
				if view.RegressionTracking.Enabled {

					err2 := primeCacheForView(view, releases, cacheOpts, ctx, bigQueryClient, dbc, config)
					if err2 != nil {
						return err2
					}

				}
			}
			return err // return last error
		},
	}

	f.BindFlags(cmd.Flags())

	return cmd
}

func primeCacheForView(view crtype.View, releases []apiv1.Release, cacheOpts cache.RequestOptions, ctx context.Context, bigQueryClient *bqcachedclient.Client, dbc *db.DB, config *configv1.SippyConfig) error {
	rLog := log.WithField("view", view.Name)

	rLog.Infof("priming cache for view")
	generator, err := buildGenerator(view, releases, cacheOpts, []crtype.RequestTestIdentificationOptions{{}}, bigQueryClient, dbc, config)
	if err != nil {
		return err
	}
	// 	empty test ID options needed to match cache keys with the default path through
	report, err := generateReport(ctx, generator, bigQueryClient)
	if err != nil {
		return err
	}

	// Now that we've got our report, we're going to reconfigure our generator to now request ALL test details
	// reports, for all regressed tests in the main report. This will happen with one query to be very cost-effective,
	// and we'll sort the test/variant combos that come back from bigquery as we go, generating
	// a report with the data for each chunk.

	// All regressed tests, both triaged and not:
	allRegressedTests := []crtype.ReportTestSummary{}
	for _, row := range report.Rows {
		for _, col := range row.Columns {
			allRegressedTests = append(allRegressedTests, col.RegressedTests...)
			// Once triaged, regressions move to this list, we want to still consider them an open regression until
			// the report says they're cleared and they disappear fully. Triaged does not imply fixed or no longer
			// a regression.
			for _, triaged := range col.TriagedIncidents {
				allRegressedTests = append(allRegressedTests, triaged.ReportTestSummary)
			}
		}
	}
	rLog.Infof("found %d regressed tests in report", len(allRegressedTests))
	testIDOptions := []crtype.RequestTestIdentificationOptions{}
	for _, regressedTest := range allRegressedTests {
		// regressedTest.BaseStats.Release // start // end
		newTIDOpts := crtype.RequestTestIdentificationOptions{
			TestID:            regressedTest.TestID,
			RequestedVariants: regressedTest.Variants,
			Component:         regressedTest.Component,
			Capability:        regressedTest.Capability,
		}
		if regressedTest.BaseStats != nil && regressedTest.BaseStats.Release != view.BaseRelease.Release {
			// releasefallback was enabled and this particular regressed test was using a prior
			// release because it had a better pass rate.
			newTIDOpts.BaseOverrideRelease = regressedTest.BaseStats.Release
		}
		rLog.Infof("adding test details request options for %+v", newTIDOpts)
		testIDOptions = append(testIDOptions, newTIDOpts)
	}

	// make a fresh generator for the test details report to avoid state issues in middleware etc.
	generator, err = buildGenerator(view, releases, cacheOpts, testIDOptions, bigQueryClient, dbc, config)
	if err != nil {
		return err
	}
	generator.ReqOptions.TestIDOptions = testIDOptions
	tdReports, errs := generator.GenerateTestDetailsReportMultiTest(ctx)
	if len(errs) > 0 {
		var strErrors []string
		for _, err := range errs {
			strErrors = append(strErrors, err.Error())
		}
		return fmt.Errorf("mutli test details report generation encountered errors: %s", strings.Join(strErrors, "; "))
	}
	rLog.Infof("got %d test details reports", len(tdReports))

	// Now we cache each test details report:
	for _, report := range tdReports {
		// manipulate cache key per test options
		genCacheKey := generator.GetCacheKey(ctx)
		newTIDOpts := crtype.RequestTestIdentificationOptions{
			TestID:            report.TestID,
			RequestedVariants: report.Variants,
			Component:         report.Component,
			Capability:        report.Capability,
		}
		// If we overrode the base stats with a prior release, reflect this in our cache key:
		if report.Analyses[0].BaseStats != nil && report.Analyses[0].BaseStats.Release != view.BaseRelease.Release {
			newTIDOpts.BaseOverrideRelease = report.Analyses[0].BaseStats.Release
		}
		genCacheKey.TestIDOptions = []crtype.RequestTestIdentificationOptions{newTIDOpts}
		tempKey := api.GetPrefixedCacheKey("TestDetailsReport~", genCacheKey)
		cacheKey, err := tempKey.GetCacheKey()
		if err != nil {
			return err
		}
		cacheDuration := api.CalculateRoundedCacheDuration(cacheOpts)
		api.CacheSet(ctx, bigQueryClient.Cache, report, cacheKey, cacheDuration)

	}

	return nil
}

func generateReport(ctx context.Context, generator *componentreadiness.ComponentReportGenerator, bigQueryClient *bqcachedclient.Client) (*crtype.ComponentReport, error) {

	// Update the cache for the main report
	report, errs := api.GetDataFromCacheOrGenerate[crtype.ComponentReport](
		ctx,
		bigQueryClient.Cache, generator.ReqOptions.CacheOption,
		api.GetPrefixedCacheKey(componentreadiness.ComponentReportCacheKeyPrefix, generator.GetCacheKey(ctx)),
		generator.GenerateReport,
		crtype.ComponentReport{})
	if len(errs) > 0 {
		var strErrors []string
		for _, err := range errs {
			strErrors = append(strErrors, err.Error())
		}
		return nil, fmt.Errorf("component report generation encountered errors: %s", strings.Join(strErrors, "; "))
	}
	return &report, nil
}

func buildGenerator(
	view crtype.View,
	releases []apiv1.Release,
	cacheOpts cache.RequestOptions,
	testIDOpts []crtype.RequestTestIdentificationOptions,
	bigQueryClient *bqcachedclient.Client,
	dbc *db.DB,
	config *configv1.SippyConfig) (*componentreadiness.ComponentReportGenerator, error) {

	baseRelease, err := componentreadiness.GetViewReleaseOptions(
		releases, "basis", view.BaseRelease, cacheOpts.CRTimeRoundingFactor)
	if err != nil {
		return nil, err
	}

	sampleRelease, err := componentreadiness.GetViewReleaseOptions(
		releases, "sample", view.SampleRelease, cacheOpts.CRTimeRoundingFactor)
	if err != nil {
		return nil, err
	}

	variantOption := view.VariantOptions
	advancedOption := view.AdvancedOptions

	// Get component readiness report
	reqOpts := crtype.RequestOptions{
		BaseRelease:    baseRelease,
		SampleRelease:  sampleRelease,
		VariantOption:  variantOption,
		AdvancedOption: advancedOption,
		CacheOption:    cacheOpts,
		TestIDOptions:  testIDOpts,
	}

	// Making a generator directly as we are going to bypass the caching to ensure we get fresh report,
	// explicitly set our reports in the cache, thus resetting the timer for all expiry and keeping the cache
	// primed.
	generator := componentreadiness.NewComponentReportGenerator(bigQueryClient, reqOpts, dbc, config.ComponentReadinessConfig.VariantJunitTableOverrides)
	return &generator, nil
}
