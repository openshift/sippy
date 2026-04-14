package regressioncacheloader

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/openshift/sippy/pkg/api"
	"github.com/openshift/sippy/pkg/api/componentreadiness"
	"github.com/openshift/sippy/pkg/api/componentreadiness/utils"
	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crview"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/testdetails"
	"github.com/openshift/sippy/pkg/apis/cache"
	configv1 "github.com/openshift/sippy/pkg/apis/config/v1"
	apiv1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	"github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
)

// MaxRegressionsToCache caps the number of test details reports we cache to protect Redis memory.
// When regressions exceed this limit, test details caching and job run tracking are both skipped.
const MaxRegressionsToCache = 300

// RegressionCacheLoader is a unified loader that handles both component readiness cache priming
// and regression tracking in a single pass. This eliminates duplicate BigQuery queries that
// occurred when these were separate loaders.
//
// Behavior is controlled by per-view flags:
//   - view.PrimeCache.Enabled: cache component report and test details in Redis
//   - view.RegressionTracking.Enabled: sync regressions to Postgres, track job runs
//
// For backward compatibility, the legacy --loader flags "component-readiness-cache" and
// "regression-tracker" both map to this single loader.
type RegressionCacheLoader struct {
	dbc    *db.DB
	errs   []error
	views  []crview.View
	logger *log.Entry

	// Cache priming deps
	bqClient             *bigquery.Client
	config               *configv1.SippyConfig
	releases             []apiv1.Release
	crTimeRoundingFactor time.Duration

	// Regression tracking deps (nil if not needed)
	regressionStore            componentreadiness.RegressionStore
	variantJunitTableOverrides []configv1.VariantJunitTableOverride
}

func New(
	dbc *db.DB,
	bqClient *bigquery.Client,
	config *configv1.SippyConfig,
	views []crview.View,
	releases []apiv1.Release,
	crTimeRoundingFactor time.Duration,
	regressionStore componentreadiness.RegressionStore,
	variantJunitTableOverrides []configv1.VariantJunitTableOverride,
) *RegressionCacheLoader {

	return &RegressionCacheLoader{
		dbc:                        dbc,
		bqClient:                   bqClient,
		config:                     config,
		views:                      views,
		releases:                   releases,
		crTimeRoundingFactor:       crTimeRoundingFactor,
		regressionStore:            regressionStore,
		variantJunitTableOverrides: variantJunitTableOverrides,
		logger:                     log.WithField("loader", "regression-cache"),
	}
}

func (l *RegressionCacheLoader) Name() string {
	return "regression-cache"
}

func (l *RegressionCacheLoader) Errors() []error {
	return l.errs
}

func (l *RegressionCacheLoader) Load() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour*1)
	defer cancel()

	cacheOpts := cache.RequestOptions{
		CRTimeRoundingFactor: l.crTimeRoundingFactor,
		RefreshRecent:        true,
		StableAge:            cache.StandardStableAgeCR,
		StableExpiry:         cache.StandardStableExpiryCR,
	}

	// Group views by sample release so we can handle regression closing per-release
	releaseResults := map[string]*struct {
		activeIDs sets.Set[uint]
		hadErrors bool
	}{}

	for _, view := range l.views {
		if !view.PrimeCache.Enabled && !view.RegressionTracking.Enabled {
			continue
		}
		vLog := l.logger.WithField("view", view.Name)
		release := view.SampleRelease.Name

		if _, ok := releaseResults[release]; !ok {
			releaseResults[release] = &struct {
				activeIDs sets.Set[uint]
				hadErrors bool
			}{
				activeIDs: make(sets.Set[uint]),
			}
		}

		activeRegs, err := l.processView(ctx, view, cacheOpts, vLog)
		if err != nil {
			vLog.WithError(err).Error("error processing view")
			l.errs = append(l.errs, err)
			releaseResults[release].hadErrors = true
			continue
		}
		for _, reg := range activeRegs {
			releaseResults[release].activeIDs.Insert(reg.ID)
		}
	}

	// Close regressions and resolve triages per-release (only if no errors for that release)
	if l.regressionStore != nil {
		for release, result := range releaseResults {
			if result.hadErrors {
				l.logger.Infof("skipping regression closing for release %s due to errors", release)
				continue
			}
			if err := l.closeStaleRegressions(release, result.activeIDs); err != nil {
				l.errs = append(l.errs, err)
			}
		}
	}
}

// processView handles a single view: generates the component report, caches it if needed,
// syncs regressions if needed, generates test details, caches them, and syncs job runs.
// Returns the list of active regressions for this view (nil if regression tracking is disabled).
func (l *RegressionCacheLoader) processView(
	ctx context.Context,
	view crview.View,
	cacheOpts cache.RequestOptions,
	vLog *log.Entry,
) ([]*models.TestRegression, error) {

	generator, err := l.buildGenerator(view, cacheOpts, []reqopts.TestIdentification{{}})
	if err != nil {
		return nil, err
	}

	// Step 1: Generate the component report (one BQ query, cached)
	report, err := l.generateAndCacheReport(ctx, generator)
	if err != nil {
		return nil, err
	}

	// Step 2: Sync regressions if enabled
	var activeRegressions []*models.TestRegression
	if view.RegressionTracking.Enabled && l.regressionStore != nil {
		rLog := l.logger.WithField("release", view.SampleRelease.Name)
		activeRegressions, err = componentreadiness.SyncRegressionsForReport(
			l.regressionStore, view, rLog, report)
		if err != nil {
			return nil, fmt.Errorf("error syncing regressions for view %s: %w", view.Name, err)
		}
	}

	// Step 3: Collect all regressed tests that need test details
	regressedTests := l.collectRegressedTests(report)
	vLog.Infof("found %d unresolved regressed tests in report", len(regressedTests))
	if len(regressedTests) > MaxRegressionsToCache {
		vLog.Warnf("skipping test_details caching and job run tracking: %d regressions exceeds max (%d)",
			len(regressedTests), MaxRegressionsToCache)
		return activeRegressions, nil
	}
	if len(regressedTests) == 0 {
		vLog.Infof("no regressed tests need test details")
		return activeRegressions, nil
	}

	// Step 4: Build test ID options for the multi-test details query
	testIDOptions := l.buildTestIDOptions(regressedTests, view)

	// Step 5: Generate test details in one BQ query
	tdGenerator, err := l.buildGenerator(view, cacheOpts, testIDOptions)
	if err != nil {
		return nil, err
	}
	tdGenerator.ReqOptions.TestIDOptions = testIDOptions
	// Don't cache the mega query itself - we cache each individual report below
	tdGenerator.ReqOptions.CacheOption.SkipCacheWrites = true
	tdReports, tdErrs := tdGenerator.GenerateTestDetailsReportMultiTest(ctx)

	var strErrors []string
	if len(tdErrs) > 0 {
		for _, e := range tdErrs {
			strErrors = append(strErrors, e.Error())
		}
	}
	vLog.Infof("got %d test details reports", len(tdReports))
	if len(testIDOptions) != len(tdReports) {
		strErrors = append(strErrors, fmt.Sprintf(
			"test details returned %d reports for %d requests", len(tdReports), len(testIDOptions)))
	}

	// Step 6: Cache each test details report individually
	if view.PrimeCache.Enabled {
		l.cacheTestDetailsReports(ctx, tdGenerator, tdReports, view, cacheOpts, &strErrors)
	}

	// Step 7: Sync job runs from the in-memory test details directly
	if view.RegressionTracking.Enabled && l.regressionStore != nil && len(activeRegressions) > 0 {
		l.syncJobRunsFromReports(vLog, activeRegressions, tdReports)
	}

	if len(strErrors) > 0 {
		return activeRegressions, fmt.Errorf("test details for view '%s' had errors:\n%s",
			view.Name, strings.Join(strErrors, "\n"))
	}
	return activeRegressions, nil
}

func (l *RegressionCacheLoader) generateAndCacheReport(
	ctx context.Context,
	generator *componentreadiness.ComponentReportGenerator,
) (*crtype.ComponentReport, error) {

	report, errs := api.GetDataFromCacheOrGenerate(
		ctx,
		l.bqClient.Cache, generator.ReqOptions.CacheOption,
		api.NewCacheSpec(generator.GetCacheKey(ctx), componentreadiness.ComponentReportCacheKeyPrefix, nil),
		generator.GenerateReport,
		crtype.ComponentReport{})
	if len(errs) > 0 {
		var strErrors []string
		for _, err := range errs {
			strErrors = append(strErrors, err.Error())
		}
		return nil, fmt.Errorf("component report generation encountered errors: %s",
			strings.Join(strErrors, "; "))
	}
	err := generator.PostAnalysis(&report)
	return &report, err
}

// collectRegressedTests returns unresolved regressed tests from the report.
func (l *RegressionCacheLoader) collectRegressedTests(
	report *crtype.ComponentReport,
) []crtype.ReportTestSummary {
	var regressedTests []crtype.ReportTestSummary
	for _, row := range report.Rows {
		for _, col := range row.Columns {
			for _, reg := range col.RegressedTests {
				if reg.ReportStatus < crtest.FixedRegression {
					regressedTests = append(regressedTests, reg)
				}
			}
		}
	}
	return regressedTests
}

func (l *RegressionCacheLoader) buildTestIDOptions(
	regressedTests []crtype.ReportTestSummary,
	view crview.View,
) []reqopts.TestIdentification {
	testIDOptions := make([]reqopts.TestIdentification, 0, len(regressedTests))
	for _, reg := range regressedTests {
		tid := reqopts.TestIdentification{
			TestID:            reg.TestID,
			RequestedVariants: reg.Variants,
			Component:         reg.Component,
			Capability:        reg.Capability,
		}
		if reg.BaseStats != nil && reg.BaseStats.Release != view.BaseRelease.Name {
			tid.BaseOverrideRelease = reg.BaseStats.Release
		}
		testIDOptions = append(testIDOptions, tid)
	}
	return testIDOptions
}

func (l *RegressionCacheLoader) cacheTestDetailsReports(
	ctx context.Context,
	generator *componentreadiness.ComponentReportGenerator,
	tdReports []testdetails.Report,
	view crview.View,
	cacheOpts cache.RequestOptions,
	strErrors *[]string,
) {
	for _, report := range tdReports {
		genCacheKey := generator.GetCacheKey(ctx)
		newTIDOpts := reqopts.TestIdentification{
			TestID:            report.TestID,
			RequestedVariants: report.Variants,
			Component:         report.Component,
			Capability:        report.Capability,
		}
		if report.Analyses[0].BaseStats != nil && report.Analyses[0].BaseStats.Release != view.BaseRelease.Name {
			newTIDOpts.BaseOverrideRelease = report.Analyses[0].BaseStats.Release
		}
		genCacheKey.TestIDOptions = []reqopts.TestIdentification{newTIDOpts}
		tempKey := api.NewCacheSpec(genCacheKey, "TestDetailsReport~", nil)
		cacheKey, err := tempKey.GetCacheKey()
		if err != nil {
			*strErrors = append(*strErrors, fmt.Sprintf("error getting cache key: %s", err.Error()))
			continue
		}
		cacheDuration := api.CalculateRoundedCacheDuration(cacheOpts)
		api.CacheSet(ctx, l.bqClient.Cache, report, cacheKey, cacheDuration)
	}
}

// syncJobRunsFromReports matches in-memory test details reports to active regressions
// and merges job runs directly, with no cache or BQ lookups.
func (l *RegressionCacheLoader) syncJobRunsFromReports(
	vLog *log.Entry,
	regressions []*models.TestRegression,
	tdReports []testdetails.Report,
) {
	// Build a lookup map: (testID, sorted variants key) -> test details report
	type reportKey struct {
		testID   string
		variants string
	}
	reportMap := make(map[reportKey]testdetails.Report, len(tdReports))
	for _, report := range tdReports {
		key := reportKey{
			testID:   report.TestID,
			variants: variantsMapKey(report.Variants),
		}
		reportMap[key] = report
	}

	var mergedTotal, matched, unmatched int
	for _, reg := range regressions {
		key := reportKey{
			testID:   reg.TestID,
			variants: variantsSliceKey(reg.Variants),
		}
		report, ok := reportMap[key]
		if !ok {
			unmatched++
			continue
		}
		matched++

		jobRuns := componentreadiness.FailedJobRunsFromTestDetails(report)
		if len(jobRuns) == 0 {
			continue
		}
		if err := l.regressionStore.MergeJobRuns(reg.ID, jobRuns); err != nil {
			vLog.WithError(err).Errorf("error merging job runs for regression %d", reg.ID)
			continue
		}
		mergedTotal += len(jobRuns)
	}
	vLog.Infof("merged %d job runs across %d regressions (matched=%d, unmatched=%d)",
		mergedTotal, len(regressions), matched, unmatched)
}

func (l *RegressionCacheLoader) closeStaleRegressions(release string, activeIDs sets.Set[uint]) error {
	rLog := l.logger.WithField("release", release)

	regressions, err := l.regressionStore.ListCurrentRegressionsForRelease(release)
	if err != nil {
		return fmt.Errorf("error listing regressions for release %s: %w", release, err)
	}

	closedCount := 0
	now := time.Now()
	rLog.Infof("checking %d regressions against %d active IDs for closing", len(regressions), activeIDs.Len())
	for _, reg := range regressions {
		if !activeIDs.Has(reg.ID) && !reg.Closed.Valid {
			rLog.Infof("closing regression no longer in any report: %v", reg)
			reg.Closed.Valid = true
			reg.Closed.Time = now
			if err := l.regressionStore.UpdateRegression(reg); err != nil {
				rLog.WithError(err).Errorf("error closing regression: %v", reg)
				continue
			}
			closedCount++
		}
	}
	rLog.Infof("closed %d regressions", closedCount)

	rLog.Infof("resolving triages with all regressions closed")
	if err := l.regressionStore.ResolveTriages(); err != nil {
		return fmt.Errorf("error resolving triages for release %s: %w", release, err)
	}
	return nil
}

func (l *RegressionCacheLoader) buildGenerator(
	view crview.View,
	cacheOpts cache.RequestOptions,
	testIDOpts []reqopts.TestIdentification,
) (*componentreadiness.ComponentReportGenerator, error) {

	baseRelease, err := utils.GetViewReleaseOptions(
		l.releases, "basis", view.BaseRelease, cacheOpts.CRTimeRoundingFactor)
	if err != nil {
		return nil, err
	}

	sampleRelease, err := utils.GetViewReleaseOptions(
		l.releases, "sample", view.SampleRelease, cacheOpts.CRTimeRoundingFactor)
	if err != nil {
		return nil, err
	}

	reqOpts := reqopts.RequestOptions{
		BaseRelease:    baseRelease,
		SampleRelease:  sampleRelease,
		VariantOption:  view.VariantOptions,
		AdvancedOption: view.AdvancedOptions,
		CacheOption:    cacheOpts,
		TestIDOptions:  testIDOpts,
		TestFilters:    view.TestFilters,
	}

	generator := componentreadiness.NewComponentReportGenerator(
		l.bqClient, reqOpts, l.dbc,
		l.config.ComponentReadinessConfig.VariantJunitTableOverrides,
		l.releases, "")
	return &generator, nil
}

// variantsMapKey creates a stable string key from a variant map for lookup matching.
func variantsMapKey(variants map[string]string) string {
	parts := utils.VariantsMapToStringSlice(variants)
	sort.Strings(parts)
	return strings.Join(parts, "|")
}

// variantsSliceKey creates a stable string key from a variant slice for lookup matching.
func variantsSliceKey(variants []string) string {
	sorted := make([]string, len(variants))
	copy(sorted, variants)
	sort.Strings(sorted)
	return strings.Join(sorted, "|")
}
