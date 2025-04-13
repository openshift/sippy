package componentreadiness

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/apache/thrift/lib/go/thrift"
	fischer "github.com/glycerine/golang-fisher-exact"
	"github.com/openshift/sippy/pkg/api/componentreadiness/middleware/regressiontracker"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"

	"github.com/openshift/sippy/pkg/api"
	"github.com/openshift/sippy/pkg/api/componentreadiness/middleware"
	regressionallowances2 "github.com/openshift/sippy/pkg/api/componentreadiness/middleware/regressionallowances"
	"github.com/openshift/sippy/pkg/api/componentreadiness/middleware/releasefallback"
	"github.com/openshift/sippy/pkg/api/componentreadiness/query"
	"github.com/openshift/sippy/pkg/api/componentreadiness/utils"
	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/cache"
	configv1 "github.com/openshift/sippy/pkg/apis/config/v1"
	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/componentreadiness/resolvedissues"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/regressionallowances"
	"github.com/openshift/sippy/pkg/util"
	"github.com/openshift/sippy/pkg/util/sets"
)

const (
	triagedIncidentsTableID = "triaged_incidents"

	explanationNoRegression = "No significant regressions found"
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

func GetComponentTestVariantsFromBigQuery(ctx context.Context, client *bqcachedclient.Client) (crtype.TestVariants, []error) {
	generator := ComponentReportGenerator{
		client: client,
	}

	return api.GetDataFromCacheOrGenerate[crtype.TestVariants](ctx, client.Cache, cache.RequestOptions{},
		api.GetPrefixedCacheKey("TestVariants~", generator), generator.GenerateVariants, crtype.TestVariants{})
}

func GetJobVariantsFromBigQuery(ctx context.Context, client *bqcachedclient.Client) (crtype.JobVariants,
	[]error) {
	generator := ComponentReportGenerator{
		client: client,
	}

	return api.GetDataFromCacheOrGenerate[crtype.JobVariants](ctx, client.Cache, cache.RequestOptions{},
		api.GetPrefixedCacheKey("TestAllVariants~", generator), generator.GenerateJobVariants, crtype.JobVariants{})
}

func GetComponentReportFromBigQuery(
	ctx context.Context,
	client *bqcachedclient.Client,
	dbc *db.DB,
	reqOptions crtype.RequestOptions,
	variantJunitTableOverrides []configv1.VariantJunitTableOverride,
) (crtype.ComponentReport, []error) {

	// TODO: generator is used as a cache key, public fields get included when we serialize it.
	// This muddles cache key with actual public/private fields and complicates use of the object
	// in other packages. Cache key to me looks like it should just be RequestOptions. With exception
	// of cacheOptions which are private, we are otherwise just breaking apart RequestOptions.
	// Watch out for BaseOverrideRelease which is not included here today. May only be used on test details...
	generator := ComponentReportGenerator{
		client:                     client,
		ReqOptions:                 reqOptions,
		triagedIssues:              nil,
		dbc:                        dbc,
		variantJunitTableOverrides: variantJunitTableOverrides,
	}

	if os.Getenv("DEV_MODE") == "1" {
		return generator.GenerateReport(ctx)
	}

	return api.GetDataFromCacheOrGenerate[crtype.ComponentReport](
		ctx,
		generator.client.Cache, generator.ReqOptions.CacheOption,
		// TODO: how are we not specifying anything specific for cache key?
		generator.GetComponentReportCacheKey(ctx, "ComponentReport~"),
		generator.GenerateReport,
		crtype.ComponentReport{})
}

// ComponentReportGenerator contains the information needed to generate a CR report. Do
// not add public fields to this struct if they are not valid as a cache key.
// GeneratorVersion is used to indicate breaking changes in the versions of
// the cached data.  It is used when the struct
// is marshalled for the cache key and should be changed when the object being
// cached changes in a way that will no longer be compatible with any prior cached version.
type ComponentReportGenerator struct {
	ReportModified             *time.Time
	client                     *bqcachedclient.Client
	dbc                        *db.DB
	triagedIssues              *resolvedissues.TriagedIncidentsForRelease
	ReqOptions                 crtype.RequestOptions
	variantJunitTableOverrides []configv1.VariantJunitTableOverride
	middlewares                []middleware.Middleware
}

func (c *ComponentReportGenerator) GetComponentReportCacheKey(ctx context.Context, prefix string) api.CacheData {
	// Make sure we have initialized the report modified field
	if c.ReportModified == nil {
		c.ReportModified = c.GetLastReportModifiedTime(ctx, c.client, c.ReqOptions.CacheOption)
	}
	return api.GetPrefixedCacheKey(prefix, c)
}

func (c *ComponentReportGenerator) GenerateVariants(ctx context.Context) (crtype.TestVariants, []error) {
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

	return crtype.TestVariants{
		Platform: columns["platform"],
		Network:  columns["network"],
		Arch:     columns["arch"],
		Upgrade:  columns["upgrade"],
		Variant:  columns["variants"],
	}, errs
}

func (c *ComponentReportGenerator) GenerateJobVariants(ctx context.Context) (crtype.JobVariants, []error) {
	errs := []error{}
	variants := crtype.JobVariants{Variants: map[string][]string{}}
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
		row := crtype.JobVariant{}
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
	// TODO: move to a constructor or similar
	c.middlewares = []middleware.Middleware{}
	// Initialize all our middleware applicable to this request.
	// TODO: Should middleware constructors do the interpretation of the request
	// and decide if they want to take part? Return nil if not?
	c.middlewares = append(c.middlewares, regressionallowances2.NewRegressionAllowancesMiddleware(c.ReqOptions))
	if c.ReqOptions.AdvancedOption.IncludeMultiReleaseAnalysis || c.ReqOptions.BaseOverrideRelease.Release != c.ReqOptions.BaseRelease.Release {
		c.middlewares = append(c.middlewares, releasefallback.NewReleaseFallbackMiddleware(c.client, c.ReqOptions))
	}
	if c.dbc != nil {
		c.middlewares = append(c.middlewares, regressiontracker.NewRegressionTrackerMiddleware(c.dbc, c.ReqOptions))
	} else {
		log.Warnf("no db connection provided, skipping regressiontracker middleware")
	}
}

// GenerateReport is the main entry point for generation of a component readiness report.
func (c *ComponentReportGenerator) GenerateReport(ctx context.Context) (crtype.ComponentReport, []error) {
	before := time.Now()
	c.initializeMiddleware()

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
	report, err := c.generateComponentTestReport(ctx, componentReportTestStatus.BaseStatus,
		componentReportTestStatus.SampleStatus)
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
	allJobVariants crtype.JobVariants) (map[string]crtype.TestStatus, []error) {

	generator := query.NewBaseQueryGenerator(c.client, c.ReqOptions, allJobVariants)

	componentReportTestStatus, errs := api.GetDataFromCacheOrGenerate[crtype.ReportTestStatus](ctx, c.client.Cache,
		generator.ReqOptions.CacheOption, api.GetPrefixedCacheKey("BaseTestStatus~", generator), generator.QueryTestStatus, crtype.ReportTestStatus{})

	if len(errs) > 0 {
		return nil, errs
	}

	return componentReportTestStatus.BaseStatus, nil
}

// getSampleQueryStatus builds the sample query, executes it, and returns the sample test status.
func (c *ComponentReportGenerator) getSampleQueryStatus(
	ctx context.Context,
	allJobVariants crtype.JobVariants,
	includeVariants map[string][]string,
	start, end time.Time,
	junitTable string) (map[string]crtype.TestStatus, []error) {

	generator := query.NewSampleQueryGenerator(c.client, c.ReqOptions, allJobVariants, includeVariants, start, end, junitTable)

	componentReportTestStatus, errs := api.GetDataFromCacheOrGenerate[crtype.ReportTestStatus](ctx,
		c.client.Cache, c.ReqOptions.CacheOption,
		api.GetPrefixedCacheKey("SampleTestStatus~", generator),
		generator.QueryTestStatus, crtype.ReportTestStatus{})

	if len(errs) > 0 {
		return nil, errs
	}

	return componentReportTestStatus.SampleStatus, nil
}

// getTestStatusFromBigQuery orchestrates the actual fetching of junit test run data for both basis and sample.
// goroutines are used to concurrently request the data for basis, sample, and various other edge cases.
func (c *ComponentReportGenerator) getTestStatusFromBigQuery(ctx context.Context) (crtype.ReportTestStatus, []error) {
	before := time.Now()
	fLog := log.WithField("func", "getTestStatusFromBigQuery")
	allJobVariants, errs := GetJobVariantsFromBigQuery(ctx, c.client)
	if len(errs) > 0 {
		fLog.Errorf("failed to get variants from bigquery")
		return crtype.ReportTestStatus{}, errs
	}

	var baseStatus, sampleStatus map[string]crtype.TestStatus
	baseStatusCh := make(chan map[string]crtype.TestStatus) // TODO: not hooked up yet, just in place for the interface for now
	var baseErrs, sampleErrs []error
	wg := sync.WaitGroup{}

	// channels for status as we may collect status from multiple queries run in separate goroutines
	sampleStatusCh := make(chan map[string]crtype.TestStatus)
	errCh := make(chan error)
	statusDoneCh := make(chan struct{})     // To signal when all processing is done
	statusErrsDoneCh := make(chan struct{}) // To signal when all processing is done

	// Invoke the Query phase for each of our configured middlewares:
	for _, mw := range c.middlewares {
		mw.Query(ctx, &wg, allJobVariants, baseStatusCh, sampleStatusCh, errCh)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
			return
		default:
			baseStatus, baseErrs = c.getBaseQueryStatus(ctx, allJobVariants)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
			return
		default:
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
		}

	}()

	// fork additional sample queries for the overrides
	// TODO: move to a variantjunitoverride middleware with Query implemented
	for i, or := range c.variantJunitTableOverrides {
		if !containsOverriddenVariant(c.ReqOptions.VariantOption.IncludeVariants, or.VariantName, or.VariantValue) {
			continue
		}
		// only do this additional query if the specified override variant is actually included in this request
		wg.Add(1)
		go func(i int, or configv1.VariantJunitTableOverride) {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			default:
				includeVariants, skipQuery := copyIncludeVariantsAndRemoveOverrides(c.variantJunitTableOverrides, i, c.ReqOptions.VariantOption.IncludeVariants)
				if skipQuery {
					fLog.Infof("skipping override sample query as all values for a variant were overridden")
					return
				}
				fLog.Infof("running override sample query for %+v with includeVariants: %+v", or, includeVariants)
				// Calculate a start time relative to the requested end time: (i.e. for rarely run jobs)
				end := c.ReqOptions.SampleRelease.End
				start, err := util.ParseCRReleaseTime([]v1.Release{}, "", or.RelativeStart,
					true, &c.ReqOptions.SampleRelease.End, c.ReqOptions.CacheOption.CRTimeRoundingFactor)
				if err != nil {
					errCh <- err
					return
				}
				status, errs := c.getSampleQueryStatus(ctx, allJobVariants, includeVariants, start, end, or.TableName)
				fLog.Infof("received %d test statuses and %d errors from override query", len(status), len(errs))
				sampleStatusCh <- status
				for _, err := range errs {
					errCh <- err
				}
			}

		}(i, or)
	}

	go func() {
		wg.Wait()
		close(baseStatusCh)
		close(sampleStatusCh)
		close(errCh)
	}()

	go func() {

		for status := range sampleStatusCh {
			fLog.Infof("received %d test statuses over channel", len(status))
			for k, v := range status {
				if sampleStatus == nil {
					fLog.Warnf("initializing sampleStatus map")
					sampleStatus = make(map[string]crtype.TestStatus)
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
	return crtype.ReportTestStatus{BaseStatus: baseStatus, SampleStatus: sampleStatus, GeneratedAt: &now}, errs
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

var componentAndCapabilityGetter func(test crtype.TestWithVariantsKey, stats crtype.TestStatus) (string, []string)

func testToComponentAndCapability(_ crtype.TestWithVariantsKey, stats crtype.TestStatus) (string, []string) {
	return stats.Component, stats.Capabilities
}

// getRowColumnIdentifications defines the rows and columns since they are variable. For rows, different pages have different row titles (component, capability etc)
// Columns titles depends on the columnGroupBy parameter user requests. A particular test can belong to multiple rows of different capabilities.
func (c *ComponentReportGenerator) getRowColumnIdentifications(testIDStr string, stats crtype.TestStatus) ([]crtype.RowIdentification, []crtype.ColumnID, error) {
	var test crtype.TestWithVariantsKey
	columnGroupByVariants := c.ReqOptions.VariantOption.ColumnGroupBy
	// We show column groups by DBGroupBy only for the last page before test details
	if c.ReqOptions.TestIDOption.TestID != "" {
		columnGroupByVariants = c.ReqOptions.VariantOption.DBGroupBy
	}
	// TODO: is this too slow?
	err := json.Unmarshal([]byte(testIDStr), &test)
	if err != nil {
		return []crtype.RowIdentification{}, []crtype.ColumnID{}, err
	}

	component, capabilities := componentAndCapabilityGetter(test, stats)
	rows := []crtype.RowIdentification{}
	// First Page with no component requested
	if c.ReqOptions.TestIDOption.Component == "" {
		rows = append(rows, crtype.RowIdentification{Component: component})
	} else if c.ReqOptions.TestIDOption.Component == component {
		// Exact test match
		if c.ReqOptions.TestIDOption.TestID != "" {
			row := crtype.RowIdentification{
				Component: component,
				TestID:    test.TestID,
				TestName:  stats.TestName,
				TestSuite: stats.TestSuite,
			}
			if c.ReqOptions.TestIDOption.Capability != "" {
				row.Capability = c.ReqOptions.TestIDOption.Capability
			}
			rows = append(rows, row)
		} else {
			for _, capability := range capabilities {
				// Exact capability match only produces one row
				if c.ReqOptions.TestIDOption.Capability != "" {
					if c.ReqOptions.TestIDOption.Capability == capability {
						row := crtype.RowIdentification{
							Component:  component,
							TestID:     test.TestID,
							TestName:   stats.TestName,
							TestSuite:  stats.TestSuite,
							Capability: capability,
						}
						rows = append(rows, row)
						break
					}
				} else {
					rows = append(rows, crtype.RowIdentification{Component: component, Capability: capability})
				}
			}
		}
	}
	columns := []crtype.ColumnID{}
	column := crtype.ColumnIdentification{Variants: map[string]string{}}
	for key, value := range test.Variants {
		if columnGroupByVariants.Has(key) {
			column.Variants[key] = value
		}
	}
	columnKeyBytes, err := json.Marshal(column)
	if err != nil {
		return []crtype.RowIdentification{}, []crtype.ColumnID{}, err
	}
	columns = append(columns, crtype.ColumnID(columnKeyBytes))

	return rows, columns, nil
}

type cellStatus struct {
	status           crtype.Status
	regressedTests   []crtype.ReportTestSummary
	triagedIncidents []crtype.TriageIncidentSummary
}

func getNewCellStatus(testID crtype.ReportTestIdentification,
	testStats crtype.ReportTestStats,
	existingCellStatus *cellStatus,
	triagedIncidents []crtype.TriagedIncident) cellStatus {
	var newCellStatus cellStatus
	if existingCellStatus != nil {
		if (testStats.ReportStatus < crtype.NotSignificant && testStats.ReportStatus < existingCellStatus.status) ||
			(existingCellStatus.status == crtype.NotSignificant && testStats.ReportStatus == crtype.SignificantImprovement) {
			// We want to show the significant improvement if assessment is not regression
			newCellStatus.status = testStats.ReportStatus
		} else {
			newCellStatus.status = existingCellStatus.status
		}
		newCellStatus.regressedTests = existingCellStatus.regressedTests
		newCellStatus.triagedIncidents = existingCellStatus.triagedIncidents
	} else {
		newCellStatus.status = testStats.ReportStatus
	}
	// don't show triaged regressions in the regressed tests
	// need a new UI to show active triaged incidents
	if testStats.ReportStatus < crtype.ExtremeTriagedRegression {
		rt := crtype.ReportTestSummary{
			ReportTestIdentification: testID,
			ReportTestStats:          testStats,
		}
		newCellStatus.regressedTests = append(newCellStatus.regressedTests, rt)
	} else if testStats.ReportStatus < crtype.MissingSample {
		ti := crtype.TriageIncidentSummary{
			TriagedIncidents: triagedIncidents,
			ReportTestSummary: crtype.ReportTestSummary{
				ReportTestIdentification: testID,
				ReportTestStats:          testStats,
			}}
		newCellStatus.triagedIncidents = append(newCellStatus.triagedIncidents, ti)
	}
	return newCellStatus
}

func updateCellStatus(rowIdentifications []crtype.RowIdentification,
	columnIdentifications []crtype.ColumnID,
	testID crtype.ReportTestIdentification,
	testStats crtype.ReportTestStats,
	status map[crtype.RowIdentification]map[crtype.ColumnID]cellStatus,
	allRows map[crtype.RowIdentification]struct{},
	allColumns map[crtype.ColumnID]struct{},
	triagedIncidents []crtype.TriagedIncident) {
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
			row = map[crtype.ColumnID]cellStatus{}
			for _, columnIdentification := range columnIdentifications {
				row[columnIdentification] = getNewCellStatus(testID, testStats, nil, triagedIncidents)
				status[rowIdentification] = row
			}
		} else {
			for _, columnIdentification := range columnIdentifications {
				existing, ok := row[columnIdentification]
				if !ok {
					row[columnIdentification] = getNewCellStatus(testID, testStats, nil, triagedIncidents)
				} else {
					row[columnIdentification] = getNewCellStatus(testID, testStats, &existing, triagedIncidents)
				}
			}
		}
	}
}

// getTriagedIssuesFromBigquery will
// fetch triaged issue from big query if not already present on the ComponentReportGenerator
// and check the triaged issue for matches with the current testID
func (c *ComponentReportGenerator) getTriagedIssuesFromBigQuery(ctx context.Context,
	testID crtype.ReportTestIdentification) (
	int, bool, []crtype.TriagedIncident, []error) {
	generator := triagedIncidentsGenerator{
		ReportModified: c.GetLastReportModifiedTime(ctx, c.client, c.ReqOptions.CacheOption),
		client:         c.client,
		cacheOption:    c.ReqOptions.CacheOption,
		SampleRelease:  c.ReqOptions.SampleRelease,
	}

	// we want to fetch this once per generator instance which should be once per UI load
	// this is the full list from the cache if available that will be subset to specific test
	// in triagedIssuesFor
	if c.triagedIssues == nil {
		releaseTriagedIncidents, errs := api.GetDataFromCacheOrGenerate[resolvedissues.TriagedIncidentsForRelease](
			ctx, generator.client.Cache, generator.cacheOption, api.GetPrefixedCacheKey("TriagedIncidents~", generator),
			generator.generateTriagedIssuesFor, resolvedissues.TriagedIncidentsForRelease{})

		if len(errs) > 0 {
			return 0, false, nil, errs
		}
		c.triagedIssues = &releaseTriagedIncidents
	}
	impactedRuns, activeProductRegression, triagedIncidents := triagedIssuesFor(c.triagedIssues, testID.ColumnIdentification, testID.TestID, c.ReqOptions.SampleRelease.Start, c.ReqOptions.SampleRelease.End)

	return impactedRuns, activeProductRegression, triagedIncidents, nil
}

func (c *ComponentReportGenerator) GetLastReportModifiedTime(ctx context.Context, client *bqcachedclient.Client,
	options cache.RequestOptions) *time.Time {

	if c.ReportModified == nil {

		// check each component of the report that may change asynchronously and require refreshing the report
		// return the most recent time

		// cache only for 5 minutes
		lastModifiedTimeCacheDuration := 5 * time.Minute
		now := time.Now().UTC()
		// Only cache until the next rounding duration
		cacheDuration := now.Truncate(lastModifiedTimeCacheDuration).Add(lastModifiedTimeCacheDuration).Sub(now)

		// default our last modified time to within the last 12 hours
		// any newer modifications will be picked up
		initLastModifiedTime := time.Now().UTC().Truncate(12 * time.Hour)

		generator := triagedIncidentsModifiedTimeGenerator{
			client: client,
			cacheOption: cache.RequestOptions{
				ForceRefresh:         options.ForceRefresh,
				CRTimeRoundingFactor: cacheDuration,
			},
			LastModifiedStartTime: &initLastModifiedTime,
		}

		// this gets called a lot, so we want to set it once on the ComponentReportGenerator
		lastModifiedTime, errs := api.GetDataFromCacheOrGenerate[*time.Time](ctx, generator.client.Cache,
			generator.cacheOption, api.GetPrefixedCacheKey("TriageLastModified~", generator), generator.generateTriagedIssuesLastModifiedTime, generator.LastModifiedStartTime)

		if len(errs) > 0 {
			c.ReportModified = generator.LastModifiedStartTime
		}

		c.ReportModified = lastModifiedTime
	}

	return c.ReportModified
}

type triagedIncidentsModifiedTimeGenerator struct {
	client                *bqcachedclient.Client
	cacheOption           cache.RequestOptions
	LastModifiedStartTime *time.Time
}

func (t *triagedIncidentsModifiedTimeGenerator) generateTriagedIssuesLastModifiedTime(ctx context.Context) (*time.Time,
	[]error) {
	before := time.Now()
	lastModifiedTime, errs := t.queryTriagedIssuesLastModified(ctx)

	log.Infof("generateTriagedIssuesLastModifiedTime query completed in %s ", time.Since(before))

	if errs != nil {
		return nil, errs
	}

	return lastModifiedTime, nil
}

func (t *triagedIncidentsModifiedTimeGenerator) queryTriagedIssuesLastModified(ctx context.Context) (*time.Time, []error) {
	// Look for the most recent modified time after our lastModifiedTime.
	// Using the partition to limit the query, we don't need the actual most recent modified time just need to know
	// if it has changed / is greater than our default
	queryString := fmt.Sprintf("SELECT max(modified_time) as LastModification FROM %s.%s WHERE modified_time > TIMESTAMP(@Last)", t.client.Dataset, triagedIncidentsTableID)

	params := make([]bigquery.QueryParameter, 0)

	params = append(params, []bigquery.QueryParameter{
		{
			Name:  "Last",
			Value: *t.LastModifiedStartTime,
		},
	}...)

	sampleQuery := t.client.BQ.Query(queryString)
	sampleQuery.Parameters = append(sampleQuery.Parameters, params...)

	return t.fetchLastModified(ctx, sampleQuery)
}

func (t *triagedIncidentsModifiedTimeGenerator) fetchLastModified(ctx context.Context,
	q *bigquery.Query) (*time.Time,
	[]error) {
	log.Infof("Fetching triaged incidents last modified time with:\n%s\nParameters:\n%+v\n", q.Q, q.Parameters)

	it, err := q.Read(ctx)
	if err != nil {
		log.WithError(err).Error("error querying triaged incidents last modified time from bigquery")
		return nil, []error{err}
	}

	lastModification := t.LastModifiedStartTime
	var triagedIncidentModifiedTime struct {
		LastModification bigquery.NullTimestamp
	}
	err = it.Next(&triagedIncidentModifiedTime)
	if err != nil && err != iterator.Done {
		log.WithError(err).Error("error parsing triaged incident last modification time from bigquery")
		return nil, []error{err}
	}
	if triagedIncidentModifiedTime.LastModification.Valid {
		lastModification = &triagedIncidentModifiedTime.LastModification.Timestamp
	}

	return lastModification, nil
}

type triagedIncidentsGenerator struct {
	ReportModified *time.Time
	client         *bqcachedclient.Client
	cacheOption    cache.RequestOptions
	SampleRelease  crtype.RequestReleaseOptions
}

func (t *triagedIncidentsGenerator) generateTriagedIssuesFor(ctx context.Context) (resolvedissues.TriagedIncidentsForRelease, []error) {
	before := time.Now()
	incidents, errs := t.queryTriagedIssues(ctx)

	log.Infof("generateTriagedIssuesFor query completed in %s with %d incidents from db", time.Since(before), len(incidents))

	if len(errs) > 0 {
		return resolvedissues.TriagedIncidentsForRelease{}, errs
	}

	triagedIncidents := resolvedissues.NewTriagedIncidentsForRelease(resolvedissues.Release(t.SampleRelease.Release))

	for _, incident := range incidents {
		k := resolvedissues.KeyForTriagedIssue(incident.TestID, incident.Variants)
		triagedIncidents.TriagedIncidents[k] = append(triagedIncidents.TriagedIncidents[k], incident)
	}

	log.Infof("generateTriagedIssuesFor completed in %s with %d incidents from db", time.Since(before), len(incidents))

	return triagedIncidents, nil
}

// triagedIssuesFor will look for triage issues that match the testID and variant group
// it will return the number of triaged job runs that occur within the sample window
// set a triage record as relevant if it has any job run that occurs within the sample window
// as well as return a flag indicating if there is at least 1 active product regression noting
// the UI should show the active triage icon for that cell
func triagedIssuesFor(releaseIncidents *resolvedissues.TriagedIncidentsForRelease, variant crtype.ColumnIdentification, testID string, startTime, endTime time.Time) (int, bool, []crtype.TriagedIncident) {
	if releaseIncidents == nil {
		return 0, false, nil
	}

	inKey := resolvedissues.KeyForTriagedIssue(testID, resolvedissues.TransformVariant(variant))

	triagedIncidents := releaseIncidents.TriagedIncidents[inKey]
	relevantIncidents := []crtype.TriagedIncident{}

	impactedJobRuns := sets.NewString() // because multiple issues could impact the same job run, be sure to count each job run only once
	numJobRunsToSuppress := 0
	activeProductRegression := false
	for _, triagedIncident := range triagedIncidents {
		startNumRunsSuppressed := numJobRunsToSuppress
		for _, impactedJobRun := range triagedIncident.JobRuns {
			if impactedJobRuns.Has(impactedJobRun.URL) {
				continue
			}
			impactedJobRuns.Insert(impactedJobRun.URL)

			compareTime := impactedJobRun.StartTime
			// preference is to compare to CompletionTime as it will more closely match jobrun modified time
			// but, it is optional so default to StartTime and set to CompletionTime when present
			if impactedJobRun.CompletionTime.Valid {
				compareTime = impactedJobRun.CompletionTime.Timestamp
			}

			if compareTime.After(startTime) && compareTime.Before(endTime) {
				numJobRunsToSuppress++
			}
		}

		if numJobRunsToSuppress > startNumRunsSuppressed {
			relevantIncidents = append(relevantIncidents, triagedIncident)

			// If we have any Product regression that has not been marked as resolved then, we consider it active as long as
			// we have some triaged job runs within the current query window.  This mechanism means we still have to update the triage records
			// periodically but not daily.
			// Note: when we want to mark a regression resolved we set the resolution date and update the triage records.  This will flip the triaged icon to green
			// for reports showing after the resolution date.
			//
			// This is a stop gap until we have regression tracking associated with Jiras, and we can use the Jira itself to check for state / recent updates
			if !triagedIncident.Issue.ResolutionDate.Valid && triagedIncident.Issue.URL.Valid && triagedIncident.Issue.Type != string(resolvedissues.TriageIssueTypeProduct) {
				activeProductRegression = true
			}
		}
	}

	// if we didn't have any jobs that matched the compare time then return nil
	if numJobRunsToSuppress == 0 {
		relevantIncidents = nil
	}

	return numJobRunsToSuppress, activeProductRegression, relevantIncidents
}

func (t *triagedIncidentsGenerator) queryTriagedIssues(ctx context.Context) ([]crtype.TriagedIncident, []error) {
	// Look for issue.start_date < TIMESTAMP(@TO) AND
	// (issue.resolution_date IS NULL OR issue.resolution_date >= TIMESTAMP(@FROM))
	// we could add a range for modified_time if we want to leverage the partitions
	// presume modification would be within 6 months of start / end
	// shouldn't be so many of these that would query too much data
	queryString := fmt.Sprintf("SELECT * FROM %s.%s WHERE release = @SampleRelease AND issue.start_date <= TIMESTAMP(@TO) AND (issue.resolution_date IS NULL OR issue.resolution_date >= TIMESTAMP(@FROM))", t.client.Dataset, triagedIncidentsTableID)

	params := make([]bigquery.QueryParameter, 0)

	params = append(params, []bigquery.QueryParameter{
		{
			Name:  "From",
			Value: t.SampleRelease.Start,
		},
		{
			Name:  "To",
			Value: t.SampleRelease.End,
		},
		{
			Name:  "SampleRelease",
			Value: t.SampleRelease.Release,
		},
	}...)

	sampleQuery := t.client.BQ.Query(queryString)
	sampleQuery.Parameters = append(sampleQuery.Parameters, params...)

	return t.fetchTriagedIssues(ctx, sampleQuery)
}

func (t *triagedIncidentsGenerator) fetchTriagedIssues(ctx context.Context,
	q *bigquery.Query) ([]crtype.TriagedIncident,
	[]error) {
	errs := make([]error, 0)
	incidents := make([]crtype.TriagedIncident, 0)
	log.Infof("Fetching triaged incidents with:\n%s\nParameters:\n%+v\n", q.Q, q.Parameters)

	it, err := q.Read(ctx)
	if err != nil {
		log.WithError(err).Error("error querying triaged incidents from bigquery")
		errs = append(errs, err)
		return incidents, errs
	}

	for {
		var triagedIncident crtype.TriagedIncident
		err := it.Next(&triagedIncident)
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.WithError(err).Error("error parsing triaged incident from bigquery")
			errs = append(errs, errors.Wrap(err, "error parsing triaged incident from bigquery"))
			continue
		}
		incidents = append(incidents, triagedIncident)
	}
	return incidents, errs
}

func (c *ComponentReportGenerator) triagedIncidentsFor(ctx context.Context,
	testID crtype.ReportTestIdentification) (int, bool,
	[]crtype.TriagedIncident) {
	// handle test case / missing client
	if c.client == nil {
		return 0, false, nil
	}

	impactedRuns, activeProductRegression, triagedIncidents, errs := c.getTriagedIssuesFromBigQuery(ctx, testID)

	if errs != nil {
		for _, err := range errs {
			log.WithError(err).Error("error getting triaged issues component from bigquery")
		}
		return 0, false, nil
	}

	return impactedRuns, activeProductRegression, triagedIncidents
}

func initTestAnalysisStruct(
	testStats *crtype.ReportTestStats,
	reqOptions crtype.RequestOptions,
	sampleStats crtype.TestStatus,
	baseStats *crtype.TestStatus) {

	// Default to required confidence from request, middleware may adjust later.
	testStats.RequiredConfidence = reqOptions.AdvancedOption.Confidence

	successFailCount := sampleStats.TotalCount - sampleStats.FlakeCount - sampleStats.SuccessCount
	testStats.SampleStats = crtype.TestDetailsReleaseStats{
		Release: reqOptions.SampleRelease.Release,
		Start:   &reqOptions.SampleRelease.Start,
		End:     &reqOptions.SampleRelease.End,
		TestDetailsTestStats: crtype.TestDetailsTestStats{
			SuccessRate:  utils.CalculatePassRate(sampleStats.SuccessCount, successFailCount, sampleStats.FlakeCount, reqOptions.AdvancedOption.FlakeAsFailure),
			SuccessCount: sampleStats.SuccessCount,
			FlakeCount:   sampleStats.FlakeCount,
			FailureCount: successFailCount,
		},
	}
	if baseStats != nil {
		baseRelease := reqOptions.BaseRelease.Release
		baseStart := reqOptions.BaseRelease.Start
		baseEnd := reqOptions.BaseRelease.End

		failCount := baseStats.TotalCount - baseStats.FlakeCount - baseStats.SuccessCount
		testStats.BaseStats = &crtype.TestDetailsReleaseStats{
			Release: baseRelease,
			Start:   &baseStart,
			End:     &baseEnd,
			TestDetailsTestStats: crtype.TestDetailsTestStats{
				SuccessRate:  utils.CalculatePassRate(baseStats.SuccessCount, failCount, baseStats.FlakeCount, reqOptions.AdvancedOption.FlakeAsFailure),
				SuccessCount: baseStats.SuccessCount,
				FlakeCount:   baseStats.FlakeCount,
				FailureCount: failCount,
			},
		}
	}
}

// TODO: break this function down and remove this nolint
// nolint:gocyclo
func (c *ComponentReportGenerator) generateComponentTestReport(ctx context.Context,
	baseStatus map[string]crtype.TestStatus,
	sampleStatus map[string]crtype.TestStatus) (crtype.ComponentReport, error) {
	report := crtype.ComponentReport{
		Rows: []crtype.ReportRow{},
	}
	// aggregatedStatus is the aggregated status based on the requested rows and columns
	aggregatedStatus := map[crtype.RowIdentification]map[crtype.ColumnID]cellStatus{}
	// allRows and allColumns are used to make sure rows are ordered and all rows have the same columns in the same order
	allRows := map[crtype.RowIdentification]struct{}{}
	allColumns := map[crtype.ColumnID]struct{}{}
	// testID is used to identify the most regressed test. With this, we can
	// create a shortcut link from any page to go straight to the most regressed test page.

	// using the baseStatus range here makes it hard to do away with the baseQuery
	// but if we did and just enumerated the sampleStatus instead
	// we wouldn't need the base query each time
	//
	// understand we use this to find tests associated with base that we don't see now in sample
	// meaning they have been renamed or removed

	for testKeyStr, baseStats := range baseStatus {
		testKey, err := utils.DeserializeTestKey(baseStats, testKeyStr)
		if err != nil {
			return crtype.ComponentReport{}, err
		}

		var testStats crtype.ReportTestStats // This is the actual stats we return over the API
		var triagedIncidents []crtype.TriagedIncident
		var resolvedIssueCompensation int
		var activeProductRegression bool

		sampleStats, ok := sampleStatus[testKeyStr]
		if !ok {
			testStats.ReportStatus = crtype.MissingSample
		} else {
			resolvedIssueCompensation, activeProductRegression, triagedIncidents = c.triagedIncidentsFor(ctx, testKey) // triaged job run failures to ignore

			// Initialize the test analysis before we start passing it around to the middleware
			initTestAnalysisStruct(&testStats, c.ReqOptions, sampleStats, &baseStats)

			// Give middleware their chance to adjust parameters prior to analysis
			for _, mw := range c.middlewares {
				err = mw.PreAnalysis(testKey, &testStats)
				if err != nil {
					return crtype.ComponentReport{}, err
				}
			}

			c.assessComponentStatus(
				&testStats,
				nil,
				activeProductRegression,
				resolvedIssueCompensation,
			)

			if !sampleStats.LastFailure.IsZero() {
				testStats.LastFailure = &sampleStats.LastFailure
			}

			if testStats.IsTriaged() {
				// we are within the triage range
				// do we want to show the triage icon or flip reportStatus
				canClearReportStatus := true
				for _, ti := range triagedIncidents {
					if ti.Issue.Type != string(resolvedissues.TriageIssueTypeInfrastructure) {
						// if a non Infrastructure regression isn't marked resolved or the resolution date is after the end of our sample query
						// then we won't clear it.  Otherwise, we can.
						if !ti.Issue.ResolutionDate.Valid || ti.Issue.ResolutionDate.Timestamp.After(c.ReqOptions.SampleRelease.End) {
							canClearReportStatus = false
						}
					}
				}

				// sanity check to make sure we aren't just defaulting to clear without any incidents (not likely)
				if len(triagedIncidents) > 0 && canClearReportStatus {
					testStats.ReportStatus = crtype.NotSignificant
				}
			}
		}
		delete(sampleStatus, testKeyStr)

		rowIdentifications, columnIdentifications, err := c.getRowColumnIdentifications(testKeyStr, baseStats)
		if err != nil {
			return crtype.ComponentReport{}, err
		}
		updateCellStatus(rowIdentifications, columnIdentifications, testKey, testStats, aggregatedStatus,
			allRows, allColumns, triagedIncidents)
	}

	// Anything we saw in the basis was removed above, all that remains are tests with no basis, typically new
	// tests, or tests that were renamed without submitting a rename to the test mapping repo.
	for testKey, sampleStats := range sampleStatus {
		testID, err := utils.DeserializeTestKey(sampleStats, testKey)
		if err != nil {
			return crtype.ComponentReport{}, err
		}

		// Check for approved regressions, and triaged incidents, which may adjust our counts and pass rate:
		var testStats crtype.ReportTestStats
		var triagedIncidents []crtype.TriagedIncident
		var resolvedIssueCompensation int // triaged job run failures to ignore
		var activeProductRegression bool
		resolvedIssueCompensation, activeProductRegression, triagedIncidents = c.triagedIncidentsFor(ctx, testID)

		// Initialize the test analysis before we start passing it around to the middleware and eventual assess:
		initTestAnalysisStruct(&testStats, c.ReqOptions, sampleStats, nil)

		c.assessComponentStatus(&testStats,
			nil,
			activeProductRegression,
			resolvedIssueCompensation,
		)

		if testStats.IsTriaged() {
			// we are within the triage range
			// do we want to show the triage icon or flip reportStatus
			canClearReportStatus := true
			for _, ti := range triagedIncidents {
				if ti.Issue.Type != string(resolvedissues.TriageIssueTypeInfrastructure) {
					// if a non Infrastructure regression isn't marked resolved or the resolution date is after the end of our sample query
					// then we won't clear it.  Otherwise, we can.
					if !ti.Issue.ResolutionDate.Valid || ti.Issue.ResolutionDate.Timestamp.After(c.ReqOptions.SampleRelease.End) {
						canClearReportStatus = false
					}
				}
			}

			// sanity check to make sure we aren't just defaulting to clear without any incidents (not likely)
			if len(triagedIncidents) > 0 && canClearReportStatus {
				testStats.ReportStatus = crtype.NotSignificant
			}
		}
		if !sampleStats.LastFailure.IsZero() {
			lastFailure := sampleStats.LastFailure
			testStats.LastFailure = &lastFailure
		}

		rowIdentifications, columnIdentification, err := c.getRowColumnIdentifications(testKey, sampleStats)
		if err != nil {
			return crtype.ComponentReport{}, err
		}
		updateCellStatus(rowIdentifications, columnIdentification, testID, testStats, aggregatedStatus,
			allRows, allColumns, nil)
	}

	// Sort the row identifications
	sortedRows := []crtype.RowIdentification{}
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

	// Sort the column identifications
	sortedColumns := []crtype.ColumnID{}
	for columnID := range allColumns {
		sortedColumns = append(sortedColumns, columnID)
	}
	sort.Slice(sortedColumns, func(i, j int) bool {
		return sortedColumns[i] < sortedColumns[j]
	})

	rows, err := buildReport(sortedRows, sortedColumns, aggregatedStatus)
	if err != nil {
		return crtype.ComponentReport{}, err
	}
	report.Rows = rows
	return report, nil
}

func buildReport(sortedRows []crtype.RowIdentification, sortedColumns []crtype.ColumnID, aggregatedStatus map[crtype.RowIdentification]map[crtype.ColumnID]cellStatus) ([]crtype.ReportRow, error) {
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
			var colIDStruct crtype.ColumnIdentification
			err := json.Unmarshal([]byte(columnID), &colIDStruct)
			if err != nil {
				return nil, err
			}
			reportColumn := crtype.ReportColumn{ColumnIdentification: colIDStruct}
			status, ok := columns[columnID]
			if !ok {
				reportColumn.Status = crtype.MissingBasisAndSample
			} else {
				reportColumn.Status = status.status
				reportColumn.RegressedTests = status.regressedTests
				sort.Slice(reportColumn.RegressedTests, func(i, j int) bool {
					return reportColumn.RegressedTests[i].ReportStatus < reportColumn.RegressedTests[j].ReportStatus
				})
				reportColumn.TriagedIncidents = status.triagedIncidents
				sort.Slice(reportColumn.TriagedIncidents, func(i, j int) bool {
					return reportColumn.TriagedIncidents[i].ReportStatus < reportColumn.TriagedIncidents[j].ReportStatus
				})
			}
			reportRow.Columns = append(reportRow.Columns, reportColumn)
			if reportColumn.Status <= crtype.SignificantTriagedRegression {
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

func getFailureCount(status crtype.JobRunTestStatusRow) int {
	failure := status.TotalCount - status.SuccessCount - status.FlakeCount
	if failure < 0 {
		failure = 0
	}
	return failure
}

func (c *ComponentReportGenerator) getTestStatusPassRate(testStatus crtype.TestStatus) float64 {
	return c.getPassRate(testStatus.SuccessCount, testStatus.TotalCount-testStatus.SuccessCount-testStatus.FlakeCount, testStatus.FlakeCount)
}

func (c *ComponentReportGenerator) getPassRate(success, failure, flake int) float64 {
	return utils.CalculatePassRate(success, failure, flake, c.ReqOptions.AdvancedOption.FlakeAsFailure)
}

func getRegressionStatus(basisPassPercentage, samplePassPercentage float64, isTriage bool) crtype.Status {
	if (basisPassPercentage - samplePassPercentage) > 0.15 {
		if isTriage {
			return crtype.ExtremeTriagedRegression
		}
		return crtype.ExtremeRegression
	}

	if isTriage {
		return crtype.SignificantTriagedRegression
	}
	return crtype.SignificantRegression
}

func (c *ComponentReportGenerator) getEffectivePityFactor(basisPassPercentage float64, approvedRegression *regressionallowances.IntentionalRegression) int {
	if approvedRegression != nil && approvedRegression.RegressedFailures > 0 {
		regressedPassPercentage := approvedRegression.RegressedPassPercentage(c.ReqOptions.AdvancedOption.FlakeAsFailure)
		if regressedPassPercentage < basisPassPercentage {
			// product owner chose a required pass percentage, so we allow pity to cover that approved pass percent
			// plus the existing pity factor to limit, "well, it's just *barely* lower" arguments.
			effectivePityFactor := int(basisPassPercentage*100) - int(regressedPassPercentage*100) + c.ReqOptions.AdvancedOption.PityFactor

			if effectivePityFactor < c.ReqOptions.AdvancedOption.PityFactor {
				log.Errorf("effective pity factor for %+v is below zero: %d", approvedRegression, effectivePityFactor)
				effectivePityFactor = c.ReqOptions.AdvancedOption.PityFactor
			}

			return effectivePityFactor
		}
	}
	return c.ReqOptions.AdvancedOption.PityFactor
}

// TODO: this will eventually become the analyze step on a Middleware, or possibly a separate
// set of objects relating to analysis, as there's not a lot of overlap between the analyzers
// (fishers, pass rate, bayes (future)) and the middlewares (fallback, intentional regressions,
// cross variant compare, rarely run jobs, etc.)
func (c *ComponentReportGenerator) assessComponentStatus(
	testStats *crtype.ReportTestStats,
	approvedRegression *regressionallowances.IntentionalRegression,
	activeProductRegression bool,
	numberOfIgnoredSampleJobRuns int, // count for triaged failures we can safely omit and ignore
) {
	// Catch unset required confidence, typically unit tests
	if testStats.RequiredConfidence == 0 {
		testStats.RequiredConfidence = c.ReqOptions.AdvancedOption.Confidence
	}

	// TODO: move to triage middleware Analyze eventually
	// preserve the initial sampleTotal, so we can check
	// to see if numberOfIgnoredSampleJobRuns impacts the status
	sampleTotal := testStats.SampleStats.SuccessCount + testStats.SampleStats.FailureCount + testStats.SampleStats.FlakeCount
	initialSampleTotal := sampleTotal
	adjustedSampleTotal := sampleTotal - numberOfIgnoredSampleJobRuns
	if adjustedSampleTotal < testStats.SampleStats.SuccessCount {
		log.Errorf("adjustedSampleTotal is too small: sampleTotal=%d, numberOfIgnoredSampleJobRuns=%d, sampleSuccess=%d",
			sampleTotal, numberOfIgnoredSampleJobRuns, testStats.SampleStats.SuccessCount)
	} else {
		sampleTotal = adjustedSampleTotal
	}
	// Adjust failure count for triaged runs
	testStats.SampleStats.FailureCount = sampleTotal - testStats.SampleStats.SuccessCount - testStats.SampleStats.FlakeCount
	if testStats.SampleStats.FailureCount < 0 {
		// The adjusted total for ignored runs can push failure count into the negatives if there were
		// more ignored runs than actual failures. (or no failures at all)
		testStats.SampleStats.FailureCount = 0
	}

	var baseSuccess, baseFailure, baseFlake, baseTotal int
	if testStats.BaseStats != nil {
		baseSuccess = testStats.BaseStats.SuccessCount
		baseFailure = testStats.BaseStats.FailureCount
		baseFlake = testStats.BaseStats.FlakeCount
		baseTotal = baseSuccess + baseFailure + baseFlake
	}

	if baseTotal == 0 && c.ReqOptions.AdvancedOption.PassRateRequiredNewTests > 0 {
		// If we have no base stats, fall back to a raw pass rate comparison for new or improperly renamed tests:
		c.buildPassRateTestStats(testStats, float64(c.ReqOptions.AdvancedOption.PassRateRequiredNewTests))
		// If a new test reports no regression, and we're not using pass rate mode for all tests, we alter
		// status to be missing basis for the pre-existing Fisher Exact behavior:
		if testStats.ReportStatus == crtype.NotSignificant && c.ReqOptions.AdvancedOption.PassRateRequiredAllTests == 0 {
			testStats.ReportStatus = crtype.MissingBasis
		}
		return
	} else if c.ReqOptions.AdvancedOption.PassRateRequiredAllTests > 0 {
		// If requested, switch to pass rate only testing to see what does not meet the criteria:
		c.buildPassRateTestStats(testStats,
			float64(c.ReqOptions.AdvancedOption.PassRateRequiredAllTests))
		return
	}

	// Otherwise we fall back to default behavior of Fishers Exact test:
	c.buildFisherExactTestStats(
		testStats,
		approvedRegression,
		activeProductRegression,
		initialSampleTotal)
}

func (c *ComponentReportGenerator) buildFisherExactTestStats(testStats *crtype.ReportTestStats,
	approvedRegression *regressionallowances.IntentionalRegression,
	activeProductRegression bool,
	initialSampleTotal int) {

	sampleSuccess := testStats.SampleStats.SuccessCount
	sampleFlake := testStats.SampleStats.FlakeCount

	fisherExact := 0.0
	testStats.Comparison = crtype.FisherExact

	status := crtype.MissingBasis
	// if the unadjusted sample was 0 then nothing to do
	if initialSampleTotal == 0 {
		if c.ReqOptions.AdvancedOption.IgnoreMissing {
			status = crtype.NotSignificant
		} else {
			status = crtype.MissingSample
		}
	} else if testStats.BaseStats.Total() != 0 {
		// see if we had a significant regression prior to adjusting for triage
		basePass := testStats.BaseStats.SuccessCount + testStats.BaseStats.FlakeCount
		samplePass := sampleSuccess + sampleFlake
		if c.ReqOptions.AdvancedOption.FlakeAsFailure {
			basePass = testStats.BaseStats.SuccessCount
			samplePass = sampleSuccess
		}
		basisPassPercentage := float64(basePass) / float64(testStats.BaseStats.Total())
		initialPassPercentage := float64(samplePass) / float64(initialSampleTotal)
		effectivePityFactor := c.getEffectivePityFactor(basisPassPercentage, approvedRegression)

		wasSignificant := false
		// only consider wasSignificant if the sampleTotal has been changed and our sample
		// pass percentage is below the basis
		// SampleStats had it's failure count decremented earlier when we calculated adjusted sampleTotal and subtracted it
		if initialSampleTotal > testStats.SampleStats.Total() && initialPassPercentage < basisPassPercentage {
			if basisPassPercentage-initialPassPercentage > float64(c.ReqOptions.AdvancedOption.PityFactor)/100 {
				wasSignificant, _ = c.fischerExactTest(testStats.RequiredConfidence, initialSampleTotal-samplePass, samplePass,
					testStats.BaseStats.Total()-basePass, basePass)
			}
			// if it was significant without the adjustment use
			// ExtremeTriagedRegression or SignificantTriagedRegression
			if wasSignificant {
				status = getRegressionStatus(basisPassPercentage, initialPassPercentage, true)
			}
		}

		if testStats.SampleStats.Total() == 0 {
			if !wasSignificant {
				if c.ReqOptions.AdvancedOption.IgnoreMissing {
					status = crtype.NotSignificant

				} else {
					status = crtype.MissingSample
				}
			}
			testStats.ReportStatus = status
			testStats.FisherExact = thrift.Float64Ptr(0.0)
			testStats.Explanations = append(testStats.Explanations, explanationNoRegression)
			return
		}

		// if we didn't detect a significant regression prior to adjusting set our default here
		if !wasSignificant {
			status = crtype.NotSignificant
		}

		// now that we know sampleTotal is non zero
		samplePassPercentage := float64(samplePass) / float64(testStats.SampleStats.Total())

		// did we remove enough failures that we are below the MinimumFailure threshold?
		if c.ReqOptions.AdvancedOption.MinimumFailure != 0 &&
			(testStats.SampleStats.Total()-samplePass) < c.ReqOptions.AdvancedOption.MinimumFailure {
			if status <= crtype.SignificantTriagedRegression {
				testStats.Explanations = append(testStats.Explanations,
					fmt.Sprintf("%s regression detected.", crtype.StringForStatus(status)))
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
		} else if basisPassPercentage-samplePassPercentage > float64(effectivePityFactor)/100 {
			significant, fisherExact = c.fischerExactTest(testStats.RequiredConfidence, testStats.SampleStats.Total()-samplePass, samplePass, testStats.BaseStats.Total()-basePass, basePass)
		}
		if significant {
			if improved {
				// only show improvements if we are not dropping out triaged results
				if initialSampleTotal == testStats.SampleStats.Total() {
					status = crtype.SignificantImprovement
				}
			} else {
				status = getRegressionStatus(basisPassPercentage, samplePassPercentage, activeProductRegression)
			}
		}
	}
	testStats.ReportStatus = status
	testStats.FisherExact = thrift.Float64Ptr(fisherExact)

	// If we have a regression, include explanations:
	if testStats.ReportStatus <= crtype.SignificantTriagedRegression {

		if testStats.ReportStatus <= crtype.SignificantRegression {
			testStats.Explanations = []string{
				fmt.Sprintf("%s regression detected.", crtype.StringForStatus(testStats.ReportStatus)),
				fmt.Sprintf("Fishers Exact probability of a regression: %.2f%%.", float64(100)-*testStats.FisherExact),
				fmt.Sprintf("Test pass rate dropped from %.2f%% to %.2f%%.",
					testStats.BaseStats.SuccessRate*float64(100),
					testStats.SampleStats.SuccessRate*float64(100)),
			}
		} else {
			testStats.Explanations = append(testStats.Explanations,
				fmt.Sprintf("%s regression detected.", crtype.StringForStatus(testStats.ReportStatus)))
		}
	}
}

func (c *ComponentReportGenerator) buildPassRateTestStats(testStats *crtype.ReportTestStats, requiredSuccessRate float64) {
	sampleSuccess := testStats.SampleStats.SuccessCount
	sampleFailure := testStats.SampleStats.FailureCount
	sampleFlake := testStats.SampleStats.FlakeCount

	successRate := c.getPassRate(sampleSuccess, sampleFailure, sampleFlake)

	// Assume 2x our allowed failure rate = an extreme regression.
	// i.e. if we require 90%, extreme is anything below 80%
	//      if we require 95%, extreme is anything below 90%
	severeRegressionSuccessRate := requiredSuccessRate - (100 - requiredSuccessRate)

	// Require 7 runs in the sample (typically 1 week) for us to consider a pass rate requirement for a new test:
	sufficientRuns := (sampleSuccess + sampleFailure + sampleFlake) >= 7

	if sufficientRuns && successRate*100 < requiredSuccessRate && sampleFailure >= c.ReqOptions.AdvancedOption.MinimumFailure {
		rStatus := crtype.SignificantRegression
		if successRate*100 < severeRegressionSuccessRate {
			rStatus = crtype.ExtremeRegression
		}
		testStats.ReportStatus = rStatus
		testStats.Explanations = append(testStats.Explanations,
			fmt.Sprintf("Test has a %.2f%% pass rate, but %.2f%% is required.", successRate*100, requiredSuccessRate))
		testStats.Comparison = crtype.PassRate
		testStats.SampleStats.SuccessRate = successRate
		return
	}

	testStats.ReportStatus = crtype.NotSignificant
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
