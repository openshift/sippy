package crcacheloader

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/openshift/sippy/pkg/api"
	"github.com/openshift/sippy/pkg/api/componentreadiness"
	sippytypes "github.com/openshift/sippy/pkg/apis/api"
	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crview"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/apis/cache"
	v1 "github.com/openshift/sippy/pkg/apis/config/v1"
	apiv1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	"github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/db"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// Lets not assume redis can handle infinite regressions, on days where we have a mass outage we can easily see thousands and push
// cache memory pretty high. Shut down test_details caching if we're over this limit, pages will have to load slowly for those
// that actually will be looked at.
const MaxRegressionsToCache = 300

type ComponentReadinessCacheLoader struct {
	dbc                  *db.DB
	errs                 []error
	views                *sippytypes.SippyViews
	cacheClient          cache.Cache
	bqClient             *bigquery.Client
	config               *v1.SippyConfig
	crTimeRoundingFactor time.Duration
}

func New(
	dbc *db.DB,
	cacheClient cache.Cache,
	bqClient *bigquery.Client,
	config *v1.SippyConfig,
	views *sippytypes.SippyViews,
	crTimeRoundingFactor time.Duration) *ComponentReadinessCacheLoader {

	return &ComponentReadinessCacheLoader{
		dbc:                  dbc,
		cacheClient:          cacheClient,
		errs:                 []error{},
		views:                views,
		bqClient:             bqClient,
		config:               config,
		crTimeRoundingFactor: crTimeRoundingFactor,
	}
}

func (l *ComponentReadinessCacheLoader) Name() string {
	return "component-readiness-cache"
}

func (l *ComponentReadinessCacheLoader) Load() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour*1)
	defer cancel()
	// Force a refresh, we want to ensure we update the cache no matter what
	//
	// This command should be called in a kube cronjob matching the time rounding factor.
	// Today we push our Sample end time out to the next even 4 hour interval UTC, i.e. 4am, 8am, 12pm, 4pm, etc.
	// We then use the delta to that time when caching as the duration for that key.
	// This command should be run in a kube cronjob then at those precise times meaning all but the most unlucky
	// requests between say 4:00:00am and 4:00:45am, should always hit the cache.
	cacheOpts := cache.RequestOptions{
		CRTimeRoundingFactor: l.crTimeRoundingFactor,
		ForceRefresh:         true,
	}

	releases, err := api.GetReleases(context.TODO(), l.bqClient, nil)
	if err != nil {
		l.errs = append(l.errs, errors.Wrap(err, "error querying releases"))
		return
	}

	for _, view := range l.views.ComponentReadiness {
		if view.PrimeCache.Enabled {

			err2 := primeCacheForView(ctx, view, releases, cacheOpts, l.bqClient, l.dbc, l.config)
			if err2 != nil {
				l.errs = append(l.errs, err)
				continue
			}

		}
	}
}

func (l *ComponentReadinessCacheLoader) Errors() []error {
	return l.errs
}

func primeCacheForView(ctx context.Context, view crview.View, releases []apiv1.Release, cacheOpts cache.RequestOptions, bigQueryClient *bigquery.Client, dbc *db.DB, config *v1.SippyConfig) error {
	rLog := log.WithField("view", view.Name)

	rLog.Infof("priming cache for view")
	generator, err := buildGenerator(view, releases, cacheOpts, []reqopts.TestIdentification{{}}, bigQueryClient, dbc, config)
	if err != nil {
		return err
	}
	report, err := generateReport(ctx, generator, bigQueryClient)
	if err != nil {
		return err
	}

	// Now that we've got our report, we're going to reconfigure our generator to now request ALL test details
	// reports, for all regressed tests in the main report. This will happen with one query to be very cost-effective,
	// and we'll sort the test/variant combos that come back from bigquery as we go, generating
	// a report with the data for each chunk.

	// All unresolved regressed tests, both triaged and not:
	regressedTestsToCache := []crtype.ReportTestSummary{}
	for _, row := range report.Rows {
		for _, col := range row.Columns {

			for _, reg := range col.RegressedTests {
				// skip if it's resolved, it's far less likely anyone will be loading details for something marked
				// resolved, and this helps reduce the caching memory when we have mass regressions and clean them up:
				if reg.ReportStatus < crtest.FixedRegression {
					regressedTestsToCache = append(regressedTestsToCache, reg)
				}
			}
		}
	}
	rLog.Infof("found %d unresolved regressed tests in report", len(regressedTestsToCache))
	if len(regressedTestsToCache) > MaxRegressionsToCache {
		rLog.Warnf("skipping test_details report caching due to the number of unresolved regressed tests being over the max (%d)", MaxRegressionsToCache)
		return nil
	}
	if len(regressedTestsToCache) == 0 {
		rLog.Infof("skipping test details report as no regressed tests are unresolved")
		return nil
	}
	testIDOptions := []reqopts.TestIdentification{}
	for _, regressedTest := range regressedTestsToCache {
		newTIDOpts := reqopts.TestIdentification{
			TestID:            regressedTest.TestID,
			RequestedVariants: regressedTest.Variants,
			Component:         regressedTest.Component,
			Capability:        regressedTest.Capability,
		}
		if regressedTest.BaseStats != nil && regressedTest.BaseStats.Release != view.BaseRelease.Name {
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
	// Disable cache writes for this mega test details query, it's huge, and it can't possibly be reused because
	// the next time we come through this path, we're force updating anyhow. Below, when we cache each test details
	// sub report, we do so explicitly.
	generator.ReqOptions.CacheOption.SkipCacheWrites = true
	tdReports, errs := generator.GenerateTestDetailsReportMultiTest(ctx)
	if len(errs) > 0 {
		var strErrors []string
		for _, err := range errs {
			strErrors = append(strErrors, err.Error())
		}
		return fmt.Errorf("multi test details report generation encountered errors: %s", strings.Join(strErrors, "; "))
	}
	rLog.Infof("got %d test details reports", len(tdReports))

	// Now we cache each test details report:
	for _, report := range tdReports {
		// manipulate cache key per test options
		genCacheKey := generator.GetCacheKey(ctx)
		newTIDOpts := reqopts.TestIdentification{
			TestID:            report.TestID,
			RequestedVariants: report.Variants,
			Component:         report.Component,
			Capability:        report.Capability,
		}
		// If we overrode the base stats with a prior release, reflect this in our cache key:
		if report.Analyses[0].BaseStats != nil && report.Analyses[0].BaseStats.Release != view.BaseRelease.Name {
			newTIDOpts.BaseOverrideRelease = report.Analyses[0].BaseStats.Release
		}
		genCacheKey.TestIDOptions = []reqopts.TestIdentification{newTIDOpts}
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

func generateReport(ctx context.Context, generator *componentreadiness.ComponentReportGenerator, bigQueryClient *bigquery.Client) (*crtype.ComponentReport, error) {

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
	err := generator.PostAnalysis(&report)
	return &report, err
}

func buildGenerator(
	view crview.View,
	releases []apiv1.Release,
	cacheOpts cache.RequestOptions,
	testIDOpts []reqopts.TestIdentification,
	bigQueryClient *bigquery.Client,
	dbc *db.DB,
	config *v1.SippyConfig) (*componentreadiness.ComponentReportGenerator, error) {

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
	reqOpts := reqopts.RequestOptions{
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
