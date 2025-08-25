package componentreadiness

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/apache/thrift/lib/go/thrift"
	fischer "github.com/glycerine/golang-fisher-exact"
	regressionallowances2 "github.com/openshift/sippy/pkg/api/componentreadiness/middleware/regressionallowances"
	"github.com/openshift/sippy/pkg/api/componentreadiness/middleware/regressiontracker"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/bq"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/testdetails"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"

	"github.com/openshift/sippy/pkg/api"
	"github.com/openshift/sippy/pkg/api/componentreadiness/middleware"
	"github.com/openshift/sippy/pkg/api/componentreadiness/middleware/releasefallback"
	"github.com/openshift/sippy/pkg/api/componentreadiness/query"
	"github.com/openshift/sippy/pkg/api/componentreadiness/utils"
	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/cache"
	configv1 "github.com/openshift/sippy/pkg/apis/config/v1"
	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/util"
	"github.com/openshift/sippy/pkg/util/sets"
)

const (
	explanationNoRegression       = "No significant regressions found"
	ComponentReportCacheKeyPrefix = "ComponentReport~"
)

type GeneratorType string

var (
	// Default parameters, these are also hardcoded in the UI. Both must be updated.
	// TODO: centralize these configurations for consumption by both the front and backends

	DefaultColumnGroupBy = "Platform,Architecture,Network"
	DefaultDBGroupBy     = "Platform,Architecture,Network,Topology,FeatureSet,Upgrade,Suite,Installer"
)

func getSingleColumnResultToSlice(ctx context.Context, q *bigquery.Query) ([]string, error) {
	names := []string{}
	it, err := q.Read(ctx)
	if err != nil {
		log.WithError(err).Error("error querying test status from bigquery")
		return names, err
	}

	for {
		row := struct{ Name string }{}
		err := it.Next(&row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.WithError(err).Error("error parsing component from bigquery")
			return names, err
		}
		names = append(names, row.Name)
	}
	return names, nil
}

// TODO: in several of the below functions we instantiate an entire ComponentReportGenerator
// to fetch some small piece of data. These look like they should be broken out. The partial
// instantiation of a complex object is risky in terms of bugs and maintenance.

func GetComponentTestVariantsFromBigQuery(ctx context.Context, client *bqcachedclient.Client) (CacheVariants, []error) {
	generator := ComponentReportGenerator{
		client: client,
	}

	return api.GetDataFromCacheOrGenerate[CacheVariants](ctx, client.Cache, cache.RequestOptions{},
		api.GetPrefixedCacheKey("TestVariants~", generator), generator.GenerateCacheVariants, CacheVariants{})
}

func GetJobVariantsFromBigQuery(ctx context.Context, client *bqcachedclient.Client) (crtest.JobVariants,
	[]error) {
	generator := ComponentReportGenerator{
		client: client,
	}

	return api.GetDataFromCacheOrGenerate[crtest.JobVariants](ctx, client.Cache, cache.RequestOptions{},
		api.GetPrefixedCacheKey("TestAllVariants~", generator), generator.GenerateJobVariants, crtest.JobVariants{})
}

func GetComponentReportFromBigQuery(
	ctx context.Context,
	client *bqcachedclient.Client,
	dbc *db.DB,
	reqOptions reqopts.RequestOptions,
	variantJunitTableOverrides []configv1.VariantJunitTableOverride,
) (report crtype.ComponentReport, errs []error) {
	releaseConfigs, err := api.GetReleases(ctx, client, false)
	if err != nil {
		return report, []error{err}
	}

	generator := NewComponentReportGenerator(client, reqOptions, dbc, variantJunitTableOverrides, releaseConfigs)

	if os.Getenv("DEV_MODE") == "1" {
		return generator.GenerateReport(ctx)
	}

	report, errs = api.GetDataFromCacheOrGenerate[crtype.ComponentReport](
		ctx,
		generator.client.Cache, generator.ReqOptions.CacheOption,
		api.GetPrefixedCacheKey(ComponentReportCacheKeyPrefix, generator.GetCacheKey(ctx)),
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
			for rti := range col.RegressedTests {
				// Carefully update the column status. We only hit this loop if there are regressed tests, which is
				// good because we know the cell status can't be improved or missing basis/sample.
				// All we need to do now is track the lowest (i.e. worst) status we see after PostAnalysis,
				// and make that our new cell status.
				var initialStatus crtest.Status
				testKey := crtest.Identification{
					RowIdentification:    col.RegressedTests[rti].RowIdentification,
					ColumnIdentification: col.RegressedTests[rti].ColumnIdentification,
				}
				if err := c.middlewares.PostAnalysis(testKey, &report.Rows[ri].Columns[ci].RegressedTests[rti].TestComparison); err != nil {
					return err
				}
				if newStatus := report.Rows[ri].Columns[ci].RegressedTests[rti].TestComparison.ReportStatus; newStatus < initialStatus {
					// After PostAnalysis this is our new worst status observed, so update the cell's status in the grid
					report.Rows[ri].Columns[ci].Status = newStatus
				}
			}
		}
	}

	return nil
}

func NewComponentReportGenerator(client *bqcachedclient.Client, reqOptions reqopts.RequestOptions, dbc *db.DB, variantJunitTableOverrides []configv1.VariantJunitTableOverride, releaseConfigs []v1.Release) ComponentReportGenerator {
	generator := ComponentReportGenerator{
		client:                     client,
		ReqOptions:                 reqOptions,
		dbc:                        dbc,
		variantJunitTableOverrides: variantJunitTableOverrides,
		releaseConfigs:             releaseConfigs,
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
	client                     *bqcachedclient.Client
	dbc                        *db.DB
	ReqOptions                 reqopts.RequestOptions
	variantJunitTableOverrides []configv1.VariantJunitTableOverride
	middlewares                middleware.List
	releaseConfigs             []v1.Release
}

type GeneratorCacheKey struct {
	ReportModified *time.Time
	BaseRelease    reqopts.Release
	SampleRelease  reqopts.Release
	VariantOption  reqopts.Variants
	AdvancedOption reqopts.Advanced
	TestIDOptions  []reqopts.TestIdentification
}

// GetCacheKey creates a cache key using the generator properties that we want included for uniqueness in what
// we cache. This provides a safer option than using the generator previously which carries some public fields
// which would be serialized and thus cause unnecessary cache misses.
// Here we should normalize to output the same cache key regardless of how fields were initialized. (nil vs empty, etc)
func (c *ComponentReportGenerator) GetCacheKey(ctx context.Context) GeneratorCacheKey {
	cacheKey := GeneratorCacheKey{
		BaseRelease:    c.ReqOptions.BaseRelease,
		SampleRelease:  c.ReqOptions.SampleRelease,
		VariantOption:  c.ReqOptions.VariantOption,
		AdvancedOption: c.ReqOptions.AdvancedOption,
		TestIDOptions:  c.ReqOptions.TestIDOptions,
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

func (c *ComponentReportGenerator) GenerateJobVariants(ctx context.Context) (crtest.JobVariants, []error) {
	errs := []error{}
	variants := crtest.JobVariants{Variants: map[string][]string{}}
	queryString := fmt.Sprintf(`SELECT variant_name, ARRAY_AGG(DISTINCT variant_value ORDER BY variant_value) AS variant_values
					FROM
						%s.job_variants
					WHERE
						variant_value!=""
					GROUP BY
						variant_name`, c.client.Dataset)
	q := c.client.BQ.Query(queryString)
	it, err := q.Read(ctx)
	if err != nil {
		log.WithError(err).Errorf("error querying variants from bigquery for %s", queryString)
		return variants, []error{err}
	}

	floatVariants := sets.NewString("FromRelease", "FromReleaseMajor", "FromReleaseMinor", "Release", "ReleaseMajor", "ReleaseMinor")
	for {
		row := bq.JobVariant{}
		err := it.Next(&row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			wrappedErr := errors.Wrapf(err, "error fetching variant row")
			log.WithError(err).Error("error fetching variants from bigquery")
			errs = append(errs, wrappedErr)
			return variants, errs
		}

		// Sort all releases in proper orders
		if floatVariants.Has(row.VariantName) {
			sort.Slice(row.VariantValues, func(i, j int) bool {
				iStrings := strings.Split(row.VariantValues[i], ".")
				jStrings := strings.Split(row.VariantValues[j], ".")
				for idx, iString := range iStrings {
					if iValue, err := strconv.ParseInt(iString, 10, 32); err == nil {
						if jValue, err := strconv.ParseInt(jStrings[idx], 10, 32); err == nil {
							if iValue != jValue {
								return iValue < jValue
							}
						}
					}
				}
				return false
			})
		}
		variants.Variants[row.VariantName] = row.VariantValues
	}
	return variants, nil
}

func (c *ComponentReportGenerator) initializeMiddleware() {
	c.middlewares = middleware.List{}
	// Initialize all our middleware applicable to this request.
	if c.ReqOptions.AdvancedOption.IncludeMultiReleaseAnalysis {
		c.middlewares = append(c.middlewares, releasefallback.NewReleaseFallbackMiddleware(c.client, c.ReqOptions, c.releaseConfigs))
	}
	if c.dbc != nil {
		c.middlewares = append(c.middlewares, regressiontracker.NewRegressionTrackerMiddleware(c.dbc, c.ReqOptions))
	} else {
		log.Warnf("no db connection provided, skipping regressiontracker middleware")
	}
	c.middlewares = append(c.middlewares, regressionallowances2.NewRegressionAllowancesMiddleware(c.ReqOptions, c.releaseConfigs))
}

// GenerateReport is the main entry point for generation of a component readiness report.
func (c *ComponentReportGenerator) GenerateReport(ctx context.Context) (crtype.ComponentReport, []error) {
	before := time.Now()

	// Load all test pass/fail counts from bigquery, both sample and basis
	componentReportTestStatus, errs := c.getTestStatusFromBigQuery(ctx)
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
	log.Infof("GenerateReport completed in %s with %d sample results and %d base results from db", time.Since(before), sampleLen, len(componentReportTestStatus.BaseStatus))

	return report, nil
}

// getBaseQueryStatus builds the basis query, executes it, and returns the basis test status.
func (c *ComponentReportGenerator) getBaseQueryStatus(ctx context.Context,
	allJobVariants crtest.JobVariants) (map[string]bq.TestStatus, []error) {

	generator := query.NewBaseQueryGenerator(c.client, c.ReqOptions, allJobVariants)

	componentReportTestStatus, errs := api.GetDataFromCacheOrGenerate[bq.ReportTestStatus](ctx, c.client.Cache,
		generator.ReqOptions.CacheOption, api.GetPrefixedCacheKey("BaseTestStatus~", generator), generator.QueryTestStatus, bq.ReportTestStatus{})

	if len(errs) > 0 {
		return nil, errs
	}

	return componentReportTestStatus.BaseStatus, nil
}

// getSampleQueryStatus builds the sample query, executes it, and returns the sample test status.
func (c *ComponentReportGenerator) getSampleQueryStatus(
	ctx context.Context,
	allJobVariants crtest.JobVariants,
	includeVariants map[string][]string,
	start, end time.Time,
	junitTable string) (map[string]bq.TestStatus, []error) {

	generator := query.NewSampleQueryGenerator(c.client, c.ReqOptions, allJobVariants, includeVariants, start, end, junitTable)

	componentReportTestStatus, errs := api.GetDataFromCacheOrGenerate[bq.ReportTestStatus](ctx,
		c.client.Cache, c.ReqOptions.CacheOption,
		api.GetPrefixedCacheKey("SampleTestStatus~", generator),
		generator.QueryTestStatus, bq.ReportTestStatus{})

	if len(errs) > 0 {
		return nil, errs
	}

	return componentReportTestStatus.SampleStatus, nil
}

// getTestStatusFromBigQuery orchestrates the actual fetching of junit test run data for both basis and sample.
// goroutines are used to concurrently request the data for basis, sample, and various other edge cases.
func (c *ComponentReportGenerator) getTestStatusFromBigQuery(ctx context.Context) (bq.ReportTestStatus, []error) {
	before := time.Now()
	fLog := log.WithField("func", "getTestStatusFromBigQuery")
	allJobVariants, errs := GetJobVariantsFromBigQuery(ctx, c.client)
	if len(errs) > 0 {
		fLog.Errorf("failed to get variants from bigquery")
		return bq.ReportTestStatus{}, errs
	}

	var baseStatus, sampleStatus map[string]bq.TestStatus
	baseStatusCh := make(chan map[string]bq.TestStatus) // TODO: not hooked up yet, just in place for the interface for now
	var baseErrs, sampleErrs []error
	wg := &sync.WaitGroup{}

	// channels for status as we may collect status from multiple queries run in separate goroutines
	sampleStatusCh := make(chan map[string]bq.TestStatus)
	errCh := make(chan error)
	statusDoneCh := make(chan struct{})     // To signal when all processing is done
	statusErrsDoneCh := make(chan struct{}) // To signal when all processing is done

	// generate inputs to the channels
	c.middlewares.Query(ctx, wg, allJobVariants, baseStatusCh, sampleStatusCh, errCh)
	goInterruptible(ctx, wg, func() { baseStatus, baseErrs = c.getBaseQueryStatus(ctx, allJobVariants) })
	goInterruptible(ctx, wg, func() {
		includeVariants, skipQuery := copyIncludeVariantsAndRemoveOverrides(c.variantJunitTableOverrides, -1, c.ReqOptions.VariantOption.IncludeVariants)
		if skipQuery {
			fLog.Infof("skipping default sample query as all values for a variant were overridden")
			return
		}
		fLog.Infof("running default sample query with includeVariants: %+v", includeVariants)
		status, errs := c.getSampleQueryStatus(ctx, allJobVariants, includeVariants, c.ReqOptions.SampleRelease.Start, c.ReqOptions.SampleRelease.End, query.DefaultJunitTable)
		fLog.Infof("received %d test statuses and %d errors from default query", len(status), len(errs))
		sampleStatusCh <- status
		for _, err := range errs {
			errCh <- err
		}
	})
	// TODO: move to a variantjunitoverride middleware with Query implemented
	c.goRunOverrideSampleQueries(ctx, wg, fLog, allJobVariants, sampleStatusCh, errCh)

	// clean up channels after all queries are done
	go func() {
		wg.Wait()
		close(baseStatusCh)
		close(sampleStatusCh)
		close(errCh)
	}()

	// manage output from the channels
	go func() {
		for status := range sampleStatusCh {
			fLog.Infof("received %d test statuses over channel", len(status))
			for k, v := range status {
				if sampleStatus == nil {
					fLog.Warnf("initializing sampleStatus map")
					sampleStatus = make(map[string]bq.TestStatus)
				}
				if v2, ok := sampleStatus[k]; ok {
					fLog.Warnf("sampleStatus already had key: %+v", k)
					fLog.Warnf("sampleStatus new value: %+v", v)
					fLog.Warnf("sampleStatus old value: %+v", v2)
				}
				sampleStatus[k] = v
			}
		}
		close(statusDoneCh)
	}()

	go func() {
		for err := range errCh {
			sampleErrs = append(sampleErrs, err)
		}
		close(statusErrsDoneCh)
	}()

	<-statusDoneCh
	<-statusErrsDoneCh
	fLog.Infof("total test statuses: %d", len(sampleStatus))

	if len(baseErrs) != 0 || len(sampleErrs) != 0 {
		errs = append(errs, baseErrs...)
		errs = append(errs, sampleErrs...)
	}
	log.Infof("getTestStatusFromBigQuery completed in %s with %d sample results and %d base results from db",
		time.Since(before), len(sampleStatus), len(baseStatus))
	now := time.Now()
	return bq.ReportTestStatus{BaseStatus: baseStatus, SampleStatus: sampleStatus, GeneratedAt: &now}, errs
}

// fork additional sample queries for the overrides
func (c *ComponentReportGenerator) goRunOverrideSampleQueries(
	ctx context.Context, wg *sync.WaitGroup, fLog *log.Entry,
	allJobVariants crtest.JobVariants,
	sampleStatusCh chan map[string]bq.TestStatus,
	errCh chan error,
) {
	for i, or := range c.variantJunitTableOverrides {
		if !containsOverriddenVariant(c.ReqOptions.VariantOption.IncludeVariants, or.VariantName, or.VariantValue) {
			continue
		}

		index, override := i, or // copy loop vars to avoid them changing during goroutine
		goInterruptible(ctx, wg, func() {
			// only do this additional query if the specified override variant is actually included in this request
			includeVariants, skipQuery := copyIncludeVariantsAndRemoveOverrides(c.variantJunitTableOverrides, index, c.ReqOptions.VariantOption.IncludeVariants)
			if skipQuery {
				fLog.Infof("skipping override sample query as all values for a variant were overridden")
				return
			}
			fLog.Infof("running override sample query for %+v with includeVariants: %+v", override, includeVariants)
			// Calculate a start time relative to the requested end time: (i.e. for rarely run jobs)
			end := c.ReqOptions.SampleRelease.End
			start, err := util.ParseCRReleaseTime([]v1.Release{}, "", override.RelativeStart,
				true, &c.ReqOptions.SampleRelease.End, c.ReqOptions.CacheOption.CRTimeRoundingFactor)
			if err != nil {
				errCh <- err
				return
			}
			status, errs := c.getSampleQueryStatus(ctx, allJobVariants, includeVariants, start, end, override.TableName)
			fLog.Infof("received %d test statuses and %d errors from override query", len(status), len(errs))
			sampleStatusCh <- status
			for _, err := range errs {
				errCh <- err
			}
		})
	}
}

func goInterruptible(ctx context.Context, wg *sync.WaitGroup, closure func()) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
			return
		default:
			closure()
		}
	}()
}

// copyIncludeVariantsAndRemoveOverrides is used when VariantJunitTableOverrides are in play, and we'll be merging in
// some results from separate junit tables. In this case, when we do the normal default query, we want to remove those
// overridden variants just in case, to make sure no results slip in that shouldn't be there.
//
// An index into the overrides slice can be provided if we're copying the include variants for that subquery. This is
// just to be careful for any future cases where we might have multiple overrides in play, and want to make sure we
// don't accidentally pull data for one, from the others junit table.
//
// Return includes a bool which may indicate to skip the query entirely because we've overridden all values for a variant.
func copyIncludeVariantsAndRemoveOverrides(
	overrides []configv1.VariantJunitTableOverride,
	currOverride int, // index into the overrides if we're copying for that specific override query
	includeVariants map[string][]string) (map[string][]string, bool) {

	cp := make(map[string][]string)
	for key, values := range includeVariants {
		newSlice := []string{}
		for _, v := range values {
			if !shouldSkipVariant(overrides, currOverride, key, v) {
				newSlice = append(newSlice, v)
			}

		}
		if len(newSlice) == 0 {
			// If we overrode a value for a variant, and no other values are specified for that
			// variant, we want to skip this query entirely.
			// i.e. if we include JobTier blocking, informing, and rare, we still want to do the default
			// query for blocking and informing even though rare was overridden.
			// However if we specify only JobTier rare, this leaves no JobTier's left in the default query resulting
			// in a normal query without considering JobTier and thus duplicate results we don't want. In this case,
			// we want to skip the default.
			//
			// TODO: With two overridden variants in one query, we could easily get into a problem
			// where no results are returned, because we AND the include variants. If JobTier rare is in table1, and
			// Foo=bar is in table2, both queries would be skipped because neither contains data for the other and we're
			// doing an AND. For now, I think this is a limitation we'll have to live with
			return cp, true
		}
		cp[key] = newSlice
	}
	return cp, false
}

func shouldSkipVariant(overrides []configv1.VariantJunitTableOverride, currOverride int, key, value string) bool {
	for i, override := range overrides {
		// if we're building a list of include variants for an override, then don't skip that variants inclusion
		if i == currOverride {
			return false
		}
		if override.VariantName == key && override.VariantValue == value {
			return true
		}
	}
	return false
}

func containsOverriddenVariant(includeVariants map[string][]string, key, value string) bool {
	for k, v := range includeVariants {
		if k != key {
			continue
		}
		for _, vv := range v {
			if vv == value {
				return true
			}
		}
	}
	return false
}

var componentAndCapabilityGetter func(test crtest.KeyWithVariants, stats bq.TestStatus) (string, []string)

func testToComponentAndCapability(_ crtest.KeyWithVariants, stats bq.TestStatus) (string, []string) {
	return stats.Component, stats.Capabilities
}

// getRowColumnIdentifications defines the rows and columns since they are variable. For rows, different pages have different row titles (component, capability etc)
// Columns titles depends on the columnGroupBy parameter user requests. A particular test can belong to multiple rows of different capabilities.
func (c *ComponentReportGenerator) getRowColumnIdentifications(testIDStr string, stats bq.TestStatus) ([]crtest.RowIdentification, []crtest.ColumnID, error) {
	var test crtest.KeyWithVariants
	columnGroupByVariants := c.ReqOptions.VariantOption.ColumnGroupBy
	// We show column groups by DBGroupBy only for the last page before test details
	if len(c.ReqOptions.TestIDOptions) > 0 && c.ReqOptions.TestIDOptions[0].TestID != "" {
		columnGroupByVariants = c.ReqOptions.VariantOption.DBGroupBy
	}

	// TODO: is this too slow?
	err := json.Unmarshal([]byte(testIDStr), &test)
	if err != nil {
		return []crtest.RowIdentification{}, []crtest.ColumnID{}, err
	}

	testComponent, testCapabilities := componentAndCapabilityGetter(test, stats)
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

		row := crtest.RowIdentification{
			Component: testComponent,
			TestID:    test.TestID,
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
	for key, value := range test.Variants {
		if columnGroupByVariants.Has(key) {
			column.Variants[key] = value
		}
	}
	columnKeyBytes, err := json.Marshal(column)
	if err != nil {
		return []crtest.RowIdentification{}, []crtest.ColumnID{}, err
	}
	columns = append(columns, crtest.ColumnID(columnKeyBytes))

	return rows, columns, nil
}

type cellStatus struct {
	status         crtest.Status
	regressedTests []crtype.ReportTestSummary
}

func getNewCellStatus(testID crtest.Identification, testStats testdetails.TestComparison, existingCellStatus *cellStatus) cellStatus {
	var newCellStatus cellStatus
	if existingCellStatus != nil {
		if (testStats.ReportStatus < crtest.NotSignificant && testStats.ReportStatus < existingCellStatus.status) ||
			(existingCellStatus.status == crtest.NotSignificant && testStats.ReportStatus == crtest.SignificantImprovement) {
			// We want to show the significant improvement if assessment is not regression
			newCellStatus.status = testStats.ReportStatus
		} else {
			newCellStatus.status = existingCellStatus.status
		}
		newCellStatus.regressedTests = existingCellStatus.regressedTests
	} else {
		newCellStatus.status = testStats.ReportStatus
	}
	if testStats.ReportStatus < crtest.MissingSample {
		rt := crtype.ReportTestSummary{
			Identification: testID,
			TestComparison: testStats,
		}
		newCellStatus.regressedTests = append(newCellStatus.regressedTests, rt)
	}
	return newCellStatus
}

func updateCellStatus(
	rowIdentifications []crtest.RowIdentification,
	columnIdentifications []crtest.ColumnID,
	testID crtest.Identification,
	testStats testdetails.TestComparison,
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
				row[columnIdentification] = getNewCellStatus(testID, testStats, nil)
				status[rowIdentification] = row
			}
		} else {
			for _, columnIdentification := range columnIdentifications {
				existing, ok := row[columnIdentification]
				if !ok {
					row[columnIdentification] = getNewCellStatus(testID, testStats, nil)
				} else {
					row[columnIdentification] = getNewCellStatus(testID, testStats, &existing)
				}
			}
		}
	}
}

func initTestAnalysisStruct(
	testStats *testdetails.TestComparison,
	reqOptions reqopts.RequestOptions,
	sampleStatus bq.TestStatus,
	baseStatus *bq.TestStatus) {

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

func (c *ComponentReportGenerator) generateComponentTestReport(basisStatusMap, sampleStatusMap map[string]bq.TestStatus) (crtype.ComponentReport, error) {
	// aggregatedStatus is the aggregated status based on the requested rows and columns
	aggregatedStatus := map[crtest.RowIdentification]map[crtest.ColumnID]cellStatus{}
	// allRows and allColumns are used to make sure rows are ordered and all rows have the same columns in the same order
	allRows := map[crtest.RowIdentification]struct{}{}
	allColumns := map[crtest.ColumnID]struct{}{}

	// merge basis and sample map keys and evaluate each key once
	keySet := sets.NewString(slices.Collect(maps.Keys(basisStatusMap))...)
	keySet.Insert(slices.Collect(maps.Keys(sampleStatusMap))...)
	for testKeyStr := range keySet {
		var cellReport testdetails.TestComparison // The actual stats we return over the API
		sampleStatus, sampleThere := sampleStatusMap[testKeyStr]
		basisStatus, basisThere := basisStatusMap[testKeyStr]

		// Deserialize the test key from its string form; need sample or base status to do this
		status := sampleStatus
		if !sampleThere {
			status = basisStatus
		}
		testKey, err := utils.DeserializeTestKey(status, testKeyStr)
		if err != nil {
			return crtype.ComponentReport{}, err
		}

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

			c.assessComponentStatus(&cellReport)
			if lastFailure := sampleStatus.LastFailure; !lastFailure.IsZero() {
				cellReport.LastFailure = &lastFailure // it's a copy, for pointer hygiene
			}
		}

		rowIdentifications, columnIdentifications, err := c.getRowColumnIdentifications(testKeyStr, status)
		if err != nil {
			return crtype.ComponentReport{}, err
		}
		updateCellStatus(
			rowIdentifications, columnIdentifications, testKey, cellReport, // inputs
			aggregatedStatus, allRows, allColumns, // these three are maps to be updated
		)
	}

	rows, err := buildReport(sortRowIdentifications(allRows), sortColumnIdentifications(allColumns), aggregatedStatus)
	if err != nil {
		return crtype.ComponentReport{}, err
	}
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

func buildReport(sortedRows []crtest.RowIdentification, sortedColumns []crtest.ColumnID, aggregatedStatus map[crtest.RowIdentification]map[crtest.ColumnID]cellStatus) ([]crtype.ReportRow, error) {
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
			var colIDStruct crtest.ColumnIdentification
			err := json.Unmarshal([]byte(columnID), &colIDStruct)
			if err != nil {
				return nil, err
			}
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
	return regressionRows, nil
}

func getRegressionStatus(basisPassPercentage, samplePassPercentage float64) crtest.Status {
	if (basisPassPercentage - samplePassPercentage) > 0.15 {
		return crtest.ExtremeRegression
	}

	return crtest.SignificantRegression
}

// TODO: this will eventually become the analyze step on a Middleware, or possibly a separate
// set of objects relating to analysis, as there's not a lot of overlap between the analyzers
// (fishers, pass rate, bayes (future)) and the middlewares (fallback, intentional regressions,
// cross variant compare, rarely run jobs, etc.)
func (c *ComponentReportGenerator) assessComponentStatus(testStats *testdetails.TestComparison) {
	// Catch unset required confidence, typically unit tests
	opts := c.ReqOptions.AdvancedOption
	if testStats.RequiredConfidence == 0 {
		testStats.RequiredConfidence = opts.Confidence
	}

	if (testStats.BaseStats == nil || testStats.BaseStats.Total() == 0) && opts.PassRateRequiredNewTests > 0 {
		// If we have no base stats, fall back to a raw pass rate comparison for new or improperly renamed tests:
		c.buildPassRateTestStats(testStats, float64(opts.PassRateRequiredNewTests))
		// If a new test reports no regression, and we're not using pass rate mode for all tests, we alter
		// status to be missing basis for the pre-existing Fisher Exact behavior:
		if testStats.ReportStatus == crtest.NotSignificant && opts.PassRateRequiredAllTests == 0 {
			testStats.ReportStatus = crtest.MissingBasis
		}
		return
	} else if opts.PassRateRequiredAllTests > 0 {
		// If requested, switch to pass rate only testing to see what does not meet the criteria:
		c.buildPassRateTestStats(testStats, float64(opts.PassRateRequiredAllTests))
		return
	}

	// Otherwise we fall back to default behavior of Fishers Exact test:
	c.buildFisherExactTestStats(testStats)
}

func (c *ComponentReportGenerator) buildFisherExactTestStats(testStats *testdetails.TestComparison) {

	fisherExact := 0.0
	testStats.Comparison = crtest.FisherExact

	status := crtest.MissingBasis
	opts := c.ReqOptions.AdvancedOption
	if testStats.SampleStats.Total() == 0 {
		if opts.IgnoreMissing {
			status = crtest.NotSignificant
		} else {
			status = crtest.MissingSample
		}
		testStats.ReportStatus = status
		testStats.FisherExact = thrift.Float64Ptr(0.0)
		testStats.Explanations = append(testStats.Explanations, explanationNoRegression)
	} else if testStats.BaseStats != nil && testStats.BaseStats.Total() != 0 {
		samplePass := testStats.SampleStats.Passes(opts.FlakeAsFailure)
		basePass := testStats.BaseStats.Passes(opts.FlakeAsFailure)
		basisPassPercentage := float64(basePass) / float64(testStats.BaseStats.Total())
		effectivePityFactor := float64(opts.PityFactor) + testStats.PityAdjustment

		// default starting status now that we know we have basis and sample
		status = crtest.NotSignificant

		// now that we know sampleTotal is non zero
		samplePassPercentage := float64(samplePass) / float64(testStats.SampleStats.Total())

		// are we below the MinimumFailure threshold?
		if opts.MinimumFailure != 0 &&
			(testStats.SampleStats.Total()-samplePass) < opts.MinimumFailure {
			if status <= crtest.SignificantTriagedRegression {
				testStats.Explanations = append(testStats.Explanations,
					fmt.Sprintf("%s regression detected.", crtest.StringForStatus(status)))
			}
			testStats.ReportStatus = status
			testStats.FisherExact = thrift.Float64Ptr(0.0)
			return
		}
		significant := false
		improved := samplePassPercentage >= basisPassPercentage

		if improved {
			// flip base and sample when improved
			significant, fisherExact = c.fischerExactTest(testStats.RequiredConfidence, testStats.BaseStats.Total()-basePass, basePass, testStats.SampleStats.Total()-samplePass, samplePass)
		} else if basisPassPercentage-samplePassPercentage > effectivePityFactor/100 {
			significant, fisherExact = c.fischerExactTest(testStats.RequiredConfidence, testStats.SampleStats.Total()-samplePass, samplePass, testStats.BaseStats.Total()-basePass, basePass)
		}
		if significant {
			if improved {
				status = crtest.SignificantImprovement
			} else {
				status = getRegressionStatus(basisPassPercentage, samplePassPercentage)
			}
		}
	}
	testStats.ReportStatus = status
	testStats.FisherExact = thrift.Float64Ptr(fisherExact)

	// If we have a regression, include explanations:
	if testStats.ReportStatus <= crtest.SignificantTriagedRegression {

		if testStats.ReportStatus <= crtest.SignificantRegression {
			testStats.Explanations = append(testStats.Explanations,
				fmt.Sprintf("%s regression detected.", crtest.StringForStatus(testStats.ReportStatus)))
			testStats.Explanations = append(testStats.Explanations,
				fmt.Sprintf("Fishers Exact probability of a regression: %.2f%%.", float64(100)-*testStats.FisherExact))
			testStats.Explanations = append(testStats.Explanations,
				fmt.Sprintf("Test pass rate dropped from %.2f%% to %.2f%%.",
					testStats.BaseStats.SuccessRate*float64(100),
					testStats.SampleStats.SuccessRate*float64(100)))
		} else {
			testStats.Explanations = append(testStats.Explanations,
				fmt.Sprintf("%s regression detected.", crtest.StringForStatus(testStats.ReportStatus)))
		}
	}
}

func (c *ComponentReportGenerator) buildPassRateTestStats(testStats *testdetails.TestComparison, requiredSuccessRate float64) {

	effectiveSuccessReq := requiredSuccessRate + testStats.RequiredPassRateAdjustment

	// Assume 2x our allowed failure rate = an extreme regression.
	// i.e. if we require 90%, extreme is anything below 80%
	//      if we require 95%, extreme is anything below 90%
	// if an adjustment is applied, still use the configured success rate to define extreme regression.
	severeRegressionSuccessRate := effectiveSuccessReq - (100 - requiredSuccessRate)

	// Require 7 runs in the sample (typically 1 week) for us to consider a pass rate requirement for a new test:
	sufficientRuns := testStats.SampleStats.Total() >= 7

	opts := c.ReqOptions.AdvancedOption
	successRate := testStats.SampleStats.PassRate(opts.FlakeAsFailure)
	if sufficientRuns && successRate*100 < effectiveSuccessReq && testStats.SampleStats.FailureCount >= opts.MinimumFailure {
		rStatus := crtest.SignificantRegression
		if successRate*100 < severeRegressionSuccessRate {
			rStatus = crtest.ExtremeRegression
		}
		testStats.ReportStatus = rStatus
		testStats.Explanations = append(testStats.Explanations,
			fmt.Sprintf("Test has a %.2f%% pass rate, but %.2f%% is required.", successRate*100, effectiveSuccessReq))
		testStats.Comparison = crtest.PassRate
		testStats.SampleStats.SuccessRate = successRate
		return
	}

	testStats.ReportStatus = crtest.NotSignificant
	testStats.Explanations = append(testStats.Explanations, explanationNoRegression)
}

func (c *ComponentReportGenerator) fischerExactTest(confidenceRequired, sampleFailure, sampleSuccess, baseFailure, baseSuccess int) (bool, float64) {
	_, _, r, _ := fischer.FisherExactTest(sampleFailure,
		sampleSuccess,
		baseFailure,
		baseSuccess)
	return r < 1-float64(confidenceRequired)/100, r
}

func (c *ComponentReportGenerator) getUniqueJUnitColumnValuesLast60Days(ctx context.Context, field string,
	nested bool) ([]string,
	error) {
	unnest := ""
	if nested {
		unnest = fmt.Sprintf(", UNNEST(%s) nested", field)
		field = "nested"
	}

	queryString := fmt.Sprintf(`SELECT
						DISTINCT %s as name
					FROM
						%s.junit %s
					WHERE
						modified_time > DATETIME_SUB(CURRENT_DATETIME(), INTERVAL 60 DAY)
					ORDER BY
						name`, field, c.client.Dataset, unnest)

	q := c.client.BQ.Query(queryString)
	return getSingleColumnResultToSlice(ctx, q)
}

func init() {
	componentAndCapabilityGetter = testToComponentAndCapability
}
