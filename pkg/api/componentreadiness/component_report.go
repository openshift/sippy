package componentreadiness

import (
	"context"
	"fmt"
	"maps"
	"os"
	"reflect"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/openshift/sippy/pkg/api/componentreadiness/middleware/alltestspassrate"
	"github.com/openshift/sippy/pkg/api/componentreadiness/middleware/fisherexact"
	"github.com/openshift/sippy/pkg/api/componentreadiness/middleware/linkinjector"
	"github.com/openshift/sippy/pkg/api/componentreadiness/middleware/newtestpassrate"
	regressionallowances2 "github.com/openshift/sippy/pkg/api/componentreadiness/middleware/regressionallowances"
	"github.com/openshift/sippy/pkg/api/componentreadiness/middleware/regressiontracker"
	"github.com/openshift/sippy/pkg/api/componentreadiness/middleware/spotcheckjobs"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crstatus"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/testdetails"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/openshift/sippy/pkg/api"
	"github.com/openshift/sippy/pkg/api/componentreadiness/dataprovider"
	"github.com/openshift/sippy/pkg/api/componentreadiness/middleware"
	"github.com/openshift/sippy/pkg/api/componentreadiness/middleware/releasefallback"
	"github.com/openshift/sippy/pkg/api/componentreadiness/utils"
	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/cache"
	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	"github.com/openshift/sippy/pkg/db"
)

const (
	ComponentReportCacheKeyPrefix   = "ComponentReport~"
	TestDetailsReportCacheKeyPrefix = "TestDetailsReportV2~"
)

type GeneratorType string

var (
	// Default parameters, these are also hardcoded in the UI. Both must be updated.
	// TODO: centralize these configurations for consumption by both the front and backends

	DefaultColumnGroupBy = "Platform,Architecture,Network"
	DefaultDBGroupBy     = "Platform,Architecture,Network,Topology,FeatureSet,Upgrade,Suite,Installer,LayeredProduct"
)

// TODO: in several of the below functions we instantiate an entire ComponentReportGenerator
// to fetch some small piece of data. These look like they should be broken out. The partial
// instantiation of a complex object is risky in terms of bugs and maintenance.

func GetComponentTestVariants(ctx context.Context, provider dataprovider.DataProvider) (CacheVariants, []error) {
	generator := ComponentReportGenerator{
		dataProvider: provider,
	}

	return api.GetDataFromCacheOrGenerate[CacheVariants](ctx, provider.Cache(), cache.RequestOptions{},
		api.NewCacheSpec(generator, "TestVariants~", nil), generator.GenerateCacheVariants, CacheVariants{})
}

func GetJobVariants(ctx context.Context, provider dataprovider.DataProvider, reqOptions reqopts.RequestOptions) (crtest.JobVariants, []error) {
	return provider.QueryJobVariants(ctx, reqOptions)
}

func GetComponentReport(
	ctx context.Context,
	provider dataprovider.DataProvider,
	dbc *db.DB,
	reqOptions reqopts.RequestOptions,
	baseURL string,
) (report crtype.ComponentReport, errs []error) {
	releaseConfigs, err := provider.QueryReleases(ctx)
	if err != nil {
		return report, []error{err}
	}

	generator := NewComponentReportGenerator(provider, reqOptions, dbc, releaseConfigs, baseURL)

	if os.Getenv("DEV_MODE") == "1" {
		report, errs = generator.GenerateReport(ctx)
		if errs != nil {
			return report, errs
		}
		err = generator.PostAnalysis(&report)
		if err != nil {
			return report, []error{err}
		}
		return report, []error{}
	}

	report, errs = api.GetDataFromCacheOrGenerate[crtype.ComponentReport](
		ctx,
		generator.getCache(), generator.ReqOptions.CacheOption,
		api.NewCacheSpec(generator.GetCacheKey(), ComponentReportCacheKeyPrefix, nil),
		generator.GenerateReport,
		crtype.ComponentReport{})
	if len(errs) > 0 {
		return report, errs
	}

	err = generator.PostAnalysis(&report)
	if err != nil {
		return report, []error{err}
	}

	return report, []error{}
}

// PostAnalysis runs the PostAnalysis method for all middleware on this component report.
// This is done outside the caching mechanism so we can load fresh data from our db (which is fast and cheap),
// and inject it into an expensive / slow report without recalculating everything.
func (c *ComponentReportGenerator) PostAnalysis(report *crtype.ComponentReport) error {

	// Give middleware their chance to adjust the result
	for ri, row := range report.Rows {
		for ci, col := range row.Columns {
			// Recompute cell status from post-analysis results. We start fresh so
			// middleware (e.g. triage) can improve a cell's status, not just worsen it.
			var worstStatus crtest.Status
			for rti := range col.RegressedTests {
				testKey := crtest.Identification{
					RowIdentification:    col.RegressedTests[rti].RowIdentification,
					ColumnIdentification: col.RegressedTests[rti].ColumnIdentification,
				}
				if err := c.middlewares.PostAnalysis(testKey, &report.Rows[ri].Columns[ci].RegressedTests[rti].TestComparison); err != nil {
					return err
				}
				if newStatus := report.Rows[ri].Columns[ci].RegressedTests[rti].ReportStatus; rti == 0 || newStatus < worstStatus {
					worstStatus = newStatus
				}
			}
			if len(col.RegressedTests) > 0 {
				report.Rows[ri].Columns[ci].Status = worstStatus
			}

			if c.includeAllTests() {
				for ati := range col.AllTests {
					testKey := crtest.Identification{
						RowIdentification:    col.AllTests[ati].RowIdentification,
						ColumnIdentification: col.AllTests[ati].ColumnIdentification,
					}
					if err := c.middlewares.PostAnalysis(testKey, &report.Rows[ri].Columns[ci].AllTests[ati].TestComparison); err != nil {
						return err
					}
				}
			}
		}
	}

	return nil
}

func NewComponentReportGenerator(provider dataprovider.DataProvider, reqOptions reqopts.RequestOptions, dbc *db.DB, releaseConfigs []v1.Release, baseURL string) ComponentReportGenerator {
	slices.Sort(reqOptions.Capabilities) // normalize ordering so cache keys match
	generator := ComponentReportGenerator{
		dataProvider:            provider,
		jobRunTestStatusFetcher: (*ComponentReportGenerator).getJobRunTestStatus,
		multiTestStatusBackoff:  defaultMultiTestStatusQueryBackoff(),
		ReqOptions:              reqOptions,
		dbc:                     dbc,
		releaseConfigs:          releaseConfigs,
		baseURL:                 baseURL,
	}
	generator.initializeMiddleware()
	return generator
}

// ComponentReportGenerator contains the information needed to generate a CR report. Do
// not add public fields to this struct if they are not valid as a cache key.
// GeneratorVersion is used to indicate breaking changes in the versions of
// the cached data.  It is used when the struct
// is marshalled for the cache key and should be changed when the object being
// cached changes in a way that will no longer be compatible with any prior cached version.
type ComponentReportGenerator struct {
	dataProvider            dataprovider.DataProvider
	jobRunTestStatusFetcher func(*ComponentReportGenerator, context.Context) (crstatus.TestJobRunStatuses, []error)
	multiTestStatusBackoff  wait.Backoff
	dbc                     *db.DB
	ReqOptions              reqopts.RequestOptions
	middlewares             middleware.List
	releaseConfigs          []v1.Release
	baseURL                 string
}

type GeneratorCacheKey struct {
	ReportModified  *time.Time
	BaseRelease     reqopts.Release
	SampleRelease   reqopts.Release
	VariantOption   reqopts.Variants
	AdvancedOption  reqopts.Advanced
	TestFilters     reqopts.TestFilters
	TestIDOptions   []reqopts.TestIdentification
	IncludeAllTests bool   `json:"include_all_tests,omitempty"`
	DataSource      string `json:",omitempty"`
}

// GetCacheKey creates a cache key using the generator properties that we want included for uniqueness in what
// we cache. This provides a safer option than using the generator previously which carries some public fields
// which would be serialized and thus cause unnecessary cache misses.
// Here we should normalize to output the same cache key regardless of how fields were initialized. (nil vs empty, etc)
func (c *ComponentReportGenerator) GetCacheKey() GeneratorCacheKey {
	cacheKey := GeneratorCacheKey{
		BaseRelease:     c.ReqOptions.BaseRelease,
		SampleRelease:   c.ReqOptions.SampleRelease,
		VariantOption:   c.ReqOptions.VariantOption,
		AdvancedOption:  c.ReqOptions.AdvancedOption,
		TestFilters:     c.ReqOptions.TestFilters,
		TestIDOptions:   c.ReqOptions.TestIDOptions,
		IncludeAllTests: c.ReqOptions.IncludeAllTests,
		DataSource:      c.ReqOptions.DataSource,
	}

	// TestIDOptions initialization differences caused many cache misses. This hacky bit of code attempts to handle
	// them all and ensure we end up with the same cache key if the slice is null, empty, or has one empty element
	if len(c.ReqOptions.TestIDOptions) == 1 && (reflect.DeepEqual(c.ReqOptions.TestIDOptions[0], reqopts.TestIdentification{}) ||
		(c.ReqOptions.TestIDOptions[0].Component == "" &&
			c.ReqOptions.TestIDOptions[0].Capability == "" &&
			c.ReqOptions.TestIDOptions[0].TestID == "" &&
			len(c.ReqOptions.TestIDOptions[0].RequestedVariants) == 0 &&
			c.ReqOptions.TestIDOptions[0].BaseOverrideRelease == "")) {
		// some code instantiates an empty request test ID options, standardize on null if we see this to keep cache keys
		// from missing.
		cacheKey.TestIDOptions = nil
	} else if len(c.ReqOptions.TestIDOptions) == 0 {
		cacheKey.TestIDOptions = nil
	}

	// Ensure string arrays are stable sorted regardless of how the caller / we constructed them.
	for k, vals := range cacheKey.VariantOption.IncludeVariants {
		sort.Strings(vals)
		cacheKey.VariantOption.IncludeVariants[k] = vals
	}
	for k, vals := range cacheKey.VariantOption.CompareVariants {
		sort.Strings(vals)
		cacheKey.VariantOption.CompareVariants[k] = vals
	}
	if len(cacheKey.TestFilters.Capabilities) > 0 { // should already be, but just in case
		sort.Strings(cacheKey.TestFilters.Capabilities)
	}

	return cacheKey
}

// CacheVariants is used only in the cache key, not in the actual report.
type CacheVariants struct {
	Network  []string `json:"network,omitempty"`
	Upgrade  []string `json:"upgrade,omitempty"`
	Arch     []string `json:"arch,omitempty"`
	Platform []string `json:"platform,omitempty"`
	Variant  []string `json:"variant,omitempty"`
}

func (c *ComponentReportGenerator) GenerateCacheVariants(ctx context.Context) (CacheVariants, []error) {
	errs := []error{}
	columns := make(map[string][]string)

	for _, column := range []string{"platform", "network", "arch", "upgrade", "variants"} {
		values, err := c.getUniqueJUnitColumnValuesLast60Days(ctx, column, column == "variants")
		if err != nil {
			wrappedErr := errors.Wrapf(err, "couldn't fetch %s", column)
			log.WithError(wrappedErr).Errorf("error generating variants")
			errs = append(errs, wrappedErr)
		}
		columns[column] = values
	}

	return CacheVariants{
		Platform: columns["platform"],
		Network:  columns["network"],
		Arch:     columns["arch"],
		Upgrade:  columns["upgrade"],
		Variant:  columns["variants"],
	}, errs
}

func (c *ComponentReportGenerator) getCache() cache.Cache {
	return c.dataProvider.Cache()
}

func (c *ComponentReportGenerator) initializeMiddleware() {
	c.middlewares = middleware.List{}

	// Initialize all our middleware applicable to this request.

	// middlewares that inject synthetic tests must run first so results are in place for other middleware.
	if len(c.ReqOptions.SpotCheckJobSamples) > 0 {
		c.middlewares = append(c.middlewares, spotcheckjobs.NewSpotCheckJobsMiddleware(c.ReqOptions))
	}

	// Initialize all our middleware applicable to this request.
	if c.ReqOptions.AdvancedOption.IncludeMultiReleaseAnalysis && c.ReqOptions.SampleRelease.PullRequestOptions == nil {
		c.middlewares = append(c.middlewares, releasefallback.NewReleaseFallbackMiddleware(c.dataProvider, c.ReqOptions, c.releaseConfigs))
	}
	if c.dbc != nil {
		c.middlewares = append(c.middlewares, regressiontracker.NewRegressionTrackerMiddleware(c.dbc, c.ReqOptions))
	} else {
		log.Warnf("no db connection provided, skipping regressiontracker middleware")
	}
	c.middlewares = append(c.middlewares, regressionallowances2.NewRegressionAllowancesMiddleware(c.ReqOptions, c.releaseConfigs))

	// Analysis middleware ordered by priority — first responder wins.
	c.middlewares = append(c.middlewares, newtestpassrate.NewNewTestPassRateMiddleware(c.ReqOptions))
	c.middlewares = append(c.middlewares, alltestspassrate.NewAllTestsPassRateMiddleware(c.ReqOptions))
	c.middlewares = append(c.middlewares, fisherexact.NewFisherExactMiddleware(c.ReqOptions))

	// Initialize LinkInjector middleware
	linkInjector := linkinjector.NewLinkInjectorMiddleware(c.ReqOptions, c.baseURL)
	c.middlewares = append(c.middlewares, linkInjector)
}

func (c *ComponentReportGenerator) includeAllTests() bool {
	return c.ReqOptions.IncludeAllTests
}

// GenerateReport is the main entry point for generation of a component readiness report.
func (c *ComponentReportGenerator) GenerateReport(ctx context.Context) (crtype.ComponentReport, []error) {
	before := time.Now()

	// Load all test pass/fail counts, both sample and basis
	componentReportTestStatus, errs := c.getTestStatus(ctx)
	if len(errs) > 0 {
		return crtype.ComponentReport{}, errs
	}

	var err error

	// generateComponentTestReport modifies SampleStatus removing matches from BaseStatus
	// resulting in erroneous sample results count
	// msg="GenerateReport completed in 1m49.528090955s with 0 sample results and 133132 base results from db"
	// get the length before processing
	sampleLen := len(componentReportTestStatus.SampleStatus)

	// perform analysis and generate report:
	report, err := c.generateComponentTestReport(componentReportTestStatus.BaseStatus, componentReportTestStatus.SampleStatus)
	if err != nil {
		log.WithError(err).Error("error generating report")
		errs = append(errs, err)
		return crtype.ComponentReport{}, errs
	}
	report.GeneratedAt = componentReportTestStatus.GeneratedAt
	log.WithField("duration", time.Since(before).String()).
		WithField("sampleResults", sampleLen).
		WithField("baseResults", len(componentReportTestStatus.BaseStatus)).
		Info("GenerateReport completed")

	return report, nil
}

func (c *ComponentReportGenerator) getTestStatus(ctx context.Context) (crstatus.ReportTestStatus, []error) {
	before := time.Now()

	wg := &sync.WaitGroup{}
	errCh := make(chan error)

	var baseStatus, sampleStatus map[string]crstatus.TestStatus
	wg.Go(func() {
		var queryErrs []error
		baseStatus, sampleStatus, queryErrs = c.dataProvider.QueryTestStatus(ctx, c.ReqOptions)
		for _, err := range queryErrs {
			errCh <- err
		}
	})

	// Query spot-check test status concurrently for each configured sample.
	var spotCheckMu sync.Mutex
	var spotCheckResults []map[string]crstatus.TestStatus
	for _, sample := range c.ReqOptions.SpotCheckJobSamples {
		wg.Go(func() {
			results, err := c.dataProvider.QuerySpotCheckTestStatus(ctx, c.ReqOptions,
				sample.Name, sample.IncludeVariants, sample.Start, sample.End)
			if err != nil {
				errCh <- fmt.Errorf("spot-check query for %s: %w", sample.Name, err)
				return
			}
			if len(results) > 0 {
				spotCheckMu.Lock()
				spotCheckResults = append(spotCheckResults, results)
				spotCheckMu.Unlock()
			}
		})
	}

	c.middlewares.Query(ctx, wg, errCh)

	go func() {
		wg.Wait()
		close(errCh)
	}()

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}

	// Merge spot-check results into sample status.
	for _, scResults := range spotCheckResults {
		maps.Copy(sampleStatus, scResults)
	}

	log.WithField("duration", time.Since(before)).
		WithField("sampleResults", len(sampleStatus)).
		WithField("baseResults", len(baseStatus)).
		Info("getTestStatus completed")
	now := time.Now()
	return crstatus.ReportTestStatus{BaseStatus: baseStatus, SampleStatus: sampleStatus, GeneratedAt: &now}, errs
}

var componentAndCapabilityGetter func(stats crstatus.TestStatus) (string, []string)

func testToComponentAndCapability(stats crstatus.TestStatus) (string, []string) {
	return stats.Component, stats.Capabilities
}

// getRowColumnIdentifications defines the rows and columns since they are variable. For rows, different pages have different row titles (component, capability etc)
// Columns titles depends on the columnGroupBy parameter user requests. A particular test can belong to multiple rows of different capabilities.
func (c *ComponentReportGenerator) getRowColumnIdentifications(stats crstatus.TestStatus) ([]crtest.RowIdentification, []crtest.ColumnID) {
	columnGroupByVariants := c.ReqOptions.VariantOption.ColumnGroupBy
	// We show column groups by DBGroupBy only for the last page before test details
	if len(c.ReqOptions.TestIDOptions) > 0 && c.ReqOptions.TestIDOptions[0].TestID != "" {
		columnGroupByVariants = c.ReqOptions.VariantOption.DBGroupBy
	}

	testComponent, testCapabilities := componentAndCapabilityGetter(stats)
	rows := []crtest.RowIdentification{}
	// First Page with no component requested
	requestedComponent, requestedCapability, requestedTestID := "", "", ""
	if len(c.ReqOptions.TestIDOptions) > 0 {
		firstTIDOpts := c.ReqOptions.TestIDOptions[0]
		requestedComponent = firstTIDOpts.Component
		requestedCapability = firstTIDOpts.Capability
		requestedTestID = firstTIDOpts.TestID // component reports can filter on test if you drill down far enough
	}

	if requestedComponent == "" {
		// No component filter specified for this report, include a row for all components:
		rows = append(rows, crtest.RowIdentification{Component: testComponent})
	} else if requestedComponent == testComponent {
		// A component filter was specified and this test matches that component:

		if stats.TestName == "" {
			return rows, nil
		}

		row := crtest.RowIdentification{
			Component: testComponent,
			TestID:    stats.TestID,
			TestName:  stats.TestName,
			TestSuite: stats.TestSuite,
		}
		// Exact test match
		if requestedTestID != "" {
			if requestedCapability != "" {
				row.Capability = requestedCapability
			}
			rows = append(rows, row)
		} else {
			for _, capability := range testCapabilities {
				// Exact capability match only produces one row
				if requestedCapability != "" {
					if requestedCapability == capability {
						row.Capability = capability
						rows = append(rows, row)
						break
					}
				} else {
					rows = append(rows, crtest.RowIdentification{Component: testComponent, Capability: capability})
				}
			}
		}
	}

	columns := []crtest.ColumnID{}
	column := crtest.ColumnIdentification{Variants: map[string]string{}}
	for key, value := range stats.Variants {
		if columnGroupByVariants.Has(key) {
			column.Variants[key] = value
		}
	}
	columns = append(columns, column.Encode())

	return rows, columns
}

type cellStatus struct {
	status         crtest.Status
	regressedTests []crtype.ReportTestSummary
	allTests       []crtype.ReportTestSummary
}

func getNewCellStatus(testID crtest.Identification, testStats testdetails.TestComparison, existingCellStatus *cellStatus, includeAllTests bool) cellStatus {
	var newCellStatus cellStatus
	if existingCellStatus != nil {
		if crtest.CompareCellStatus(testStats.ReportStatus, existingCellStatus.status) < 0 {
			newCellStatus.status = testStats.ReportStatus
		} else {
			newCellStatus.status = existingCellStatus.status
		}
		newCellStatus.regressedTests = existingCellStatus.regressedTests
		if includeAllTests {
			newCellStatus.allTests = existingCellStatus.allTests
		}
	} else {
		newCellStatus.status = testStats.ReportStatus
	}

	rt := crtype.ReportTestSummary{
		Identification: testID,
		TestComparison: testStats,
	}
	if includeAllTests {
		newCellStatus.allTests = append(newCellStatus.allTests, rt)
	}

	if testStats.ReportStatus < crtest.MissingSample {
		newCellStatus.regressedTests = append(newCellStatus.regressedTests, rt)
	}
	return newCellStatus
}

func updateCellStatus(
	rowIdentifications []crtest.RowIdentification,
	columnIdentifications []crtest.ColumnID,
	testID crtest.Identification,
	testStats testdetails.TestComparison,
	includeAllTests bool,
	// use the inputs above to update the maps below (golang passes maps by reference)
	status map[crtest.RowIdentification]map[crtest.ColumnID]cellStatus,
	allRows map[crtest.RowIdentification]struct{},
	allColumns map[crtest.ColumnID]struct{},
) {
	for _, columnIdentification := range columnIdentifications {
		if _, ok := allColumns[columnIdentification]; !ok {
			allColumns[columnIdentification] = struct{}{}
		}
	}

	for _, rowIdentification := range rowIdentifications {
		// Each test might have multiple Capabilities. Initial ID just pick the first on
		// the list. If we are on a page with specific capability, this needs to be rewritten.
		if rowIdentification.Capability != "" {
			testID.Capability = rowIdentification.Capability
		}
		if _, ok := allRows[rowIdentification]; !ok {
			allRows[rowIdentification] = struct{}{}
		}
		row, ok := status[rowIdentification]
		if !ok {
			row = map[crtest.ColumnID]cellStatus{}
			for _, columnIdentification := range columnIdentifications {
				row[columnIdentification] = getNewCellStatus(testID, testStats, nil, includeAllTests)
				status[rowIdentification] = row
			}
		} else {
			for _, columnIdentification := range columnIdentifications {
				existing, ok := row[columnIdentification]
				if !ok {
					row[columnIdentification] = getNewCellStatus(testID, testStats, nil, includeAllTests)
				} else {
					row[columnIdentification] = getNewCellStatus(testID, testStats, &existing, includeAllTests)
				}
			}
		}
	}
}

func initTestAnalysisStruct(
	testStats *testdetails.TestComparison,
	reqOptions reqopts.RequestOptions,
	sampleStatus crstatus.TestStatus,
	baseStatus *crstatus.TestStatus) {

	// Default to required confidence from request, middleware may adjust later.
	testStats.RequiredConfidence = reqOptions.AdvancedOption.Confidence

	testStats.SampleStats = testdetails.ReleaseStats{
		Release: reqOptions.SampleRelease.Name,
		Start:   &reqOptions.SampleRelease.Start,
		End:     &reqOptions.SampleRelease.End,
		Stats:   sampleStatus.ToTestStats(reqOptions.AdvancedOption.FlakeAsFailure),
	}
	if baseStatus != nil {
		testStats.BaseStats = &testdetails.ReleaseStats{
			Release: reqOptions.BaseRelease.Name,
			Start:   &reqOptions.BaseRelease.Start,
			End:     &reqOptions.BaseRelease.End,
			Stats:   baseStatus.ToTestStats(reqOptions.AdvancedOption.FlakeAsFailure),
		}
	}
}

func (c *ComponentReportGenerator) generateComponentTestReport(basisStatusMap, sampleStatusMap map[string]crstatus.TestStatus) (crtype.ComponentReport, error) {
	includeAllTests := c.includeAllTests()
	// aggregatedStatus is the aggregated status based on the requested rows and columns
	aggregatedStatus := map[crtest.RowIdentification]map[crtest.ColumnID]cellStatus{}
	// allRows and allColumns are used to make sure rows are ordered and all rows have the same columns in the same order
	allRows := map[crtest.RowIdentification]struct{}{}
	allColumns := map[crtest.ColumnID]struct{}{}

	// merge basis and sample map keys and evaluate each key once
	keySet := sets.New(slices.Collect(maps.Keys(basisStatusMap))...)
	keySet.Insert(slices.Collect(maps.Keys(sampleStatusMap))...)
	for testKeyStr := range keySet {
		cellReport := testdetails.TestComparison{Explanations: []string{}} // The actual stats we return over the API
		sampleStatus, sampleThere := sampleStatusMap[testKeyStr]
		basisStatus, basisThere := basisStatusMap[testKeyStr]

		// Deserialize the test key from its string form; need sample or base status to do this
		status := sampleStatus
		if !sampleThere {
			status = basisStatus
		}
		testKey := utils.IdentificationFromStatus(status)

		if !sampleThere {
			// we use this to find tests associated with the basis that we don't see now in sample,
			// meaning they have been renamed or removed. no further analysis is needed.
			cellReport.ReportStatus = crtest.MissingSample
		} else {
			// Initialize the test analysis before we start passing it around to the middleware
			if basisThere {
				initTestAnalysisStruct(&cellReport, c.ReqOptions, sampleStatus, &basisStatus)
			} else {
				initTestAnalysisStruct(&cellReport, c.ReqOptions, sampleStatus, nil)
			}

			// Give middleware a chance to adjust parameters prior to analysis
			if err := c.middlewares.PreAnalysis(testKey, &cellReport); err != nil {
				return crtype.ComponentReport{}, err
			}

			if _, err := c.middlewares.Analyze(testKey, &cellReport); err != nil {
				return crtype.ComponentReport{}, err
			}
			if lastFailure := sampleStatus.LastFailure; !lastFailure.IsZero() {
				cellReport.LastFailure = &lastFailure // it's a copy, for pointer hygiene
			}
		}

		rowIdentifications, columnIdentifications := c.getRowColumnIdentifications(status)
		updateCellStatus(
			rowIdentifications, columnIdentifications, testKey, cellReport, includeAllTests, // inputs
			aggregatedStatus, allRows, allColumns, // these three are maps to be updated
		)
	}

	rows := buildReport(sortRowIdentifications(allRows), sortColumnIdentifications(allColumns), aggregatedStatus, includeAllTests)
	return crtype.ComponentReport{Rows: rows}, nil
}

func sortRowIdentifications(allRows map[crtest.RowIdentification]struct{}) []crtest.RowIdentification {
	sortedRows := []crtest.RowIdentification{}
	for rowID := range allRows {
		sortedRows = append(sortedRows, rowID)
	}
	sort.Slice(sortedRows, func(i, j int) bool {
		less := sortedRows[i].Component < sortedRows[j].Component
		if sortedRows[i].Component == sortedRows[j].Component {
			less = sortedRows[i].Capability < sortedRows[j].Capability
			if sortedRows[i].Capability == sortedRows[j].Capability {
				less = sortedRows[i].TestName < sortedRows[j].TestName
				if sortedRows[i].TestName == sortedRows[j].TestName {
					less = sortedRows[i].TestID < sortedRows[j].TestID
				}
			}
		}
		return less
	})
	return sortedRows
}

func sortColumnIdentifications(allColumns map[crtest.ColumnID]struct{}) []crtest.ColumnID {
	sortedColumns := []crtest.ColumnID{}
	for columnID := range allColumns {
		sortedColumns = append(sortedColumns, columnID)
	}
	sort.Slice(sortedColumns, func(i, j int) bool {
		return sortedColumns[i] < sortedColumns[j]
	})
	return sortedColumns
}

func buildReport(sortedRows []crtest.RowIdentification, sortedColumns []crtest.ColumnID, aggregatedStatus map[crtest.RowIdentification]map[crtest.ColumnID]cellStatus, includeAllTests bool) []crtype.ReportRow {
	// Now build the report
	var regressionRows, goodRows []crtype.ReportRow
	for _, rowID := range sortedRows {
		columns, ok := aggregatedStatus[rowID]
		if !ok {
			continue
		}
		reportRow := crtype.ReportRow{RowIdentification: rowID}
		hasRegression := false
		for _, columnID := range sortedColumns {
			if reportRow.Columns == nil {
				reportRow.Columns = []crtype.ReportColumn{}
			}
			colIDStruct := crtest.DecodeColumnID(columnID)
			reportColumn := crtype.ReportColumn{ColumnIdentification: colIDStruct}
			status, ok := columns[columnID]
			if !ok {
				reportColumn.Status = crtest.MissingBasisAndSample
			} else {
				reportColumn.Status = status.status
				reportColumn.RegressedTests = status.regressedTests
				sort.Slice(reportColumn.RegressedTests, func(i, j int) bool {
					return reportColumn.RegressedTests[i].ReportStatus < reportColumn.RegressedTests[j].ReportStatus
				})
				if includeAllTests {
					reportColumn.AllTests = status.allTests
					sort.Slice(reportColumn.AllTests, func(i, j int) bool {
						return reportColumn.AllTests[i].ReportStatus < reportColumn.AllTests[j].ReportStatus
					})
				}
			}
			reportRow.Columns = append(reportRow.Columns, reportColumn)
			if reportColumn.Status <= crtest.SignificantTriagedRegression {
				hasRegression = true
			}
		}
		// Any rows with regression should appear first, so make two slices
		// and assemble them later.
		if hasRegression {
			regressionRows = append(regressionRows, reportRow)
		} else {
			goodRows = append(goodRows, reportRow)
		}
	}

	regressionRows = append(regressionRows, goodRows...)
	return regressionRows
}

func (c *ComponentReportGenerator) getUniqueJUnitColumnValuesLast60Days(ctx context.Context, field string,
	nested bool) ([]string,
	error) {
	return c.dataProvider.QueryUniqueVariantValues(ctx, c.ReqOptions, field, nested)
}

func init() {
	componentAndCapabilityGetter = testToComponentAndCapability
}
