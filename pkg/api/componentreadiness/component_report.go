package componentreadiness

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/civil"
	"github.com/openshift/sippy/pkg/util"

	"cloud.google.com/go/bigquery"
	"github.com/apache/thrift/lib/go/thrift"
	fischer "github.com/glycerine/golang-fisher-exact"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"

	"github.com/openshift/sippy/pkg/api"
	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/cache"
	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/componentreadiness/resolvedissues"
	"github.com/openshift/sippy/pkg/regressionallowances"
	"github.com/openshift/sippy/pkg/util/sets"
)

const (
	triagedIncidentsTableID = "triaged_incidents"

	ignoredJobsRegexp = `-okd|-recovery|aggregator-|alibaba|-disruptive|-rollback|-out-of-change|-sno-fips-recert`

	// openRegressionConfidenceAdjustment is subtracted from the requested confidence for regressed tests that have
	// an open regression.
	openRegressionConfidenceAdjustment = 5

	explanationNoRegression = "No significant regressions found"
)

type GeneratorType string

var (
	// Default parameters, these are also hardcoded in the UI. Both must be updated.
	// TODO: centralize these configurations for consumption by both the front and backends

	DefaultColumnGroupBy = "Platform,Architecture,Network"
	DefaultDBGroupBy     = "Platform,Architecture,Network,Topology,FeatureSet,Upgrade,Suite,Installer"
)

func newFallbackReleases() crtype.FallbackReleases {
	fb := crtype.FallbackReleases{
		Releases: map[string]crtype.ReleaseTestMap{},
	}
	return fb
}

func getSingleColumnResultToSlice(ctx context.Context, query *bigquery.Query) ([]string, error) {
	names := []string{}
	it, err := query.Read(ctx)
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

func GetComponentTestVariantsFromBigQuery(ctx context.Context, client *bqcachedclient.Client,
	gcsBucket string) (crtype.TestVariants, []error) {
	generator := componentReportGenerator{
		client:    client,
		gcsBucket: gcsBucket,
	}

	return api.GetDataFromCacheOrGenerate[crtype.TestVariants](ctx, client.Cache, cache.RequestOptions{},
		api.GetPrefixedCacheKey("TestVariants~", generator), generator.GenerateVariants, crtype.TestVariants{})
}

func GetReleaseDatesFromBigQuery(ctx context.Context, client *bqcachedclient.Client) ([]crtype.Release, []error) {
	generator := componentReportGenerator{
		client: client,
	}

	return api.GetDataFromCacheOrGenerate[[]crtype.Release](ctx, client.Cache, cache.RequestOptions{}, api.GetPrefixedCacheKey("CRReleaseDates~", generator), generator.GenerateReleaseDates, []crtype.Release{})
}

func GetJobVariantsFromBigQuery(ctx context.Context, client *bqcachedclient.Client,
	gcsBucket string) (crtype.JobVariants,
	[]error) {
	generator := componentReportGenerator{
		client:    client,
		gcsBucket: gcsBucket,
	}

	return api.GetDataFromCacheOrGenerate[crtype.JobVariants](ctx, client.Cache, cache.RequestOptions{},
		api.GetPrefixedCacheKey("TestAllVariants~", generator), generator.GenerateJobVariants, crtype.JobVariants{})
}

func GetComponentReportFromBigQuery(ctx context.Context, client *bqcachedclient.Client, prowURL, gcsBucket string,
	reqOptions crtype.RequestOptions,
) (crtype.ComponentReport, []error) {
	generator := componentReportGenerator{
		client:                           client,
		prowURL:                          prowURL,
		gcsBucket:                        gcsBucket,
		cacheOption:                      reqOptions.CacheOption,
		BaseRelease:                      reqOptions.BaseRelease,
		SampleRelease:                    reqOptions.SampleRelease,
		triagedIssues:                    nil,
		RequestTestIdentificationOptions: reqOptions.TestIDOption,
		RequestVariantOptions:            reqOptions.VariantOption,
		RequestAdvancedOptions:           reqOptions.AdvancedOption,
	}

	return api.GetDataFromCacheOrGenerate[crtype.ComponentReport](
		ctx,
		generator.client.Cache, generator.cacheOption,
		generator.GetComponentReportCacheKey(ctx, "ComponentReport~"),
		generator.GenerateReport,
		crtype.ComponentReport{})
}

// componentReportGenerator contains the information needed to generate a CR report. Do
// not add public fields to this struct if they are not valid as a cache key.
// GeneratorVersion is used to indicate breaking changes in the versions of
// the cached data.  It is used when the struct
// is marshalled for the cache key and should be changed when the object being
// cached changes in a way that will no longer be compatible with any prior cached version.
type componentReportGenerator struct {
	ReportModified             *time.Time
	client                     *bqcachedclient.Client
	prowURL                    string
	gcsBucket                  string
	cacheOption                cache.RequestOptions
	BaseRelease                crtype.RequestReleaseOptions
	BaseOverrideRelease        crtype.RequestReleaseOptions
	SampleRelease              crtype.RequestReleaseOptions
	triagedIssues              *resolvedissues.TriagedIncidentsForRelease
	cachedFallbackTestStatuses *crtype.FallbackReleases
	crtype.RequestTestIdentificationOptions
	crtype.RequestVariantOptions
	crtype.RequestAdvancedOptions
	openRegressions []*crtype.TestRegression
}

func (c *componentReportGenerator) GetComponentReportCacheKey(ctx context.Context, prefix string) api.CacheData {
	// Make sure we have initialized the report modified field
	if c.ReportModified == nil {
		c.ReportModified = c.GetLastReportModifiedTime(ctx, c.client, c.cacheOption)
	}
	return api.GetPrefixedCacheKey(prefix, c)
}

func (c *componentReportGenerator) GenerateVariants(ctx context.Context) (crtype.TestVariants, []error) {
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

func (c *componentReportGenerator) GenerateReleaseDates(ctx context.Context) ([]crtype.Release, []error) {
	releases, err := api.GetReleasesFromBigQuery(ctx, c.client)
	if err != nil {
		return nil, []error{err}
	}
	crReleases := []crtype.Release{}
	for _, release := range releases {
		crRelease := crtype.Release{Release: release.Release}
		if release.GADate != nil {
			prior := util.AdjustReleaseTime(*release.GADate, true, "30", c.cacheOption.CRTimeRoundingFactor)
			crRelease.Start = &prior
			crRelease.End = release.GADate
		}
		crReleases = append(crReleases, crRelease)
	}
	return crReleases, nil
}

func (c *componentReportGenerator) GenerateJobVariants(ctx context.Context) (crtype.JobVariants, []error) {
	errs := []error{}
	variants := crtype.JobVariants{Variants: map[string][]string{}}
	queryString := fmt.Sprintf(`SELECT variant_name, ARRAY_AGG(DISTINCT variant_value ORDER BY variant_value) AS variant_values
					FROM
						%s.job_variants
					WHERE
						variant_value!=""
					GROUP BY
						variant_name`, c.client.Dataset)
	query := c.client.BQ.Query(queryString)
	it, err := query.Read(ctx)
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

func (c *componentReportGenerator) GenerateReport(ctx context.Context) (crtype.ComponentReport, []error) {
	before := time.Now()
	componentReportTestStatus, errs := c.GenerateComponentReportTestStatus(ctx)
	if len(errs) > 0 {
		return crtype.ComponentReport{}, errs
	}
	bqs := NewBigQueryRegressionStore(c.client)
	var err error
	allRegressions, err := bqs.ListCurrentRegressions(ctx)
	if err != nil {
		errs = append(errs, err)
		return crtype.ComponentReport{}, errs
	}
	c.openRegressions = FilterRegressionsForRelease(allRegressions, c.SampleRelease.Release)
	report, err := c.generateComponentTestReport(ctx, componentReportTestStatus.BaseStatus,
		componentReportTestStatus.SampleStatus)
	if err != nil {
		errs = append(errs, err)
		return crtype.ComponentReport{}, errs
	}
	report.GeneratedAt = componentReportTestStatus.GeneratedAt
	log.Infof("GenerateReport completed in %s with %d sample results and %d base results from db", time.Since(before), len(componentReportTestStatus.SampleStatus), len(componentReportTestStatus.BaseStatus))

	return report, nil
}

func (c *componentReportGenerator) GenerateComponentReportTestStatus(ctx context.Context) (crtype.ReportTestStatus, []error) {
	before := time.Now()
	componentReportTestStatus, errs := c.getTestStatusFromBigQuery(ctx)
	if len(errs) > 0 {
		return crtype.ReportTestStatus{}, errs
	}
	log.Infof("getTestStatusFromBigQuery completed in %s with %d sample results and %d base results from db", time.Since(before), len(componentReportTestStatus.SampleStatus), len(componentReportTestStatus.BaseStatus))
	now := time.Now()
	componentReportTestStatus.GeneratedAt = &now
	return componentReportTestStatus, nil
}

// getBaseQueryStatus builds the basis query, executes it, and returns the basis test status.
func (c *componentReportGenerator) getBaseQueryStatus(ctx context.Context,
	allJobVariants crtype.JobVariants) (map[string]crtype.TestStatus, []error) {

	generator := newBaseQueryGenerator(c, allJobVariants)

	componentReportTestStatus, errs := api.GetDataFromCacheOrGenerate[crtype.ReportTestStatus](ctx, c.client.Cache,
		generator.cacheOption, api.GetPrefixedCacheKey("BaseTestStatus~", generator), generator.queryTestStatus, crtype.ReportTestStatus{})

	if len(errs) > 0 {
		return nil, errs
	}

	return componentReportTestStatus.BaseStatus, nil
}

func (c *componentReportGenerator) getFallbackBaseQueryStatus(ctx context.Context,
	allJobVariants crtype.JobVariants,
	release string, start, end time.Time) []error {
	generator := newFallbackTestQueryReleasesGenerator(c, allJobVariants, release, start, end)

	cachedFallbackTestStatuses, errs := api.GetDataFromCacheOrGenerate[*crtype.FallbackReleases](
		ctx, c.client.Cache, generator.cacheOption,
		api.GetPrefixedCacheKey("FallbackReleases~", generator),
		generator.getTestFallbackReleases,
		&crtype.FallbackReleases{})

	if len(errs) > 0 {
		return errs
	}

	c.cachedFallbackTestStatuses = cachedFallbackTestStatuses
	return nil
}

// getSampleQueryStatus builds the sample query, executes it, and returns the sample test status.
func (c *componentReportGenerator) getSampleQueryStatus(
	ctx context.Context, allJobVariants crtype.JobVariants) (map[string]crtype.TestStatus, []error) {
	generator := newSampleQueryGenerator(c, allJobVariants)

	componentReportTestStatus, errs := api.GetDataFromCacheOrGenerate[crtype.ReportTestStatus](ctx,
		c.client.Cache, c.cacheOption,
		api.GetPrefixedCacheKey("SampleTestStatus~", generator),
		generator.queryTestStatus, crtype.ReportTestStatus{})

	if len(errs) > 0 {
		return nil, errs
	}

	return componentReportTestStatus.SampleStatus, nil
}

func (c *componentReportGenerator) getTestStatusFromBigQuery(ctx context.Context) (crtype.ReportTestStatus, []error) {
	before := time.Now()
	allJobVariants, errs := GetJobVariantsFromBigQuery(ctx, c.client, c.gcsBucket)
	if len(errs) > 0 {
		log.Errorf("failed to get variants from bigquery")
		return crtype.ReportTestStatus{}, errs
	}

	var baseStatus, sampleStatus map[string]crtype.TestStatus
	var baseErrs, sampleErrs []error
	wg := sync.WaitGroup{}

	if c.IncludeMultiReleaseAnalysis {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-ctx.Done():
				log.Infof("Context canceled while fetching fallback query status")
				return
			default:
				c.getFallbackBaseQueryStatus(ctx, allJobVariants, c.BaseRelease.Release, c.BaseRelease.Start, c.BaseRelease.End)
			}
		}()
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
			sampleStatus, sampleErrs = c.getSampleQueryStatus(ctx, allJobVariants)
		}

	}()

	wg.Wait()
	if len(baseErrs) != 0 || len(sampleErrs) != 0 {
		errs = append(errs, baseErrs...)
		errs = append(errs, sampleErrs...)
	}
	log.Infof("getTestStatusFromBigQuery completed in %s with %d sample results and %d base results from db", time.Since(before), len(sampleStatus), len(baseStatus))

	// Because base and sample status are cached in bigquery, we do filtering on capability and component in go code
	if c.RequestTestIdentificationOptions.Capability != "" {
		baseStatus = filterByCapability(baseStatus, c.RequestTestIdentificationOptions.Capability)
		sampleStatus = filterByCapability(sampleStatus, c.RequestTestIdentificationOptions.Capability)
	}

	if c.RequestTestIdentificationOptions.Component != "" {
		baseStatus = filterByComponent(baseStatus, c.RequestTestIdentificationOptions.Component)
		sampleStatus = filterByComponent(sampleStatus, c.RequestTestIdentificationOptions.Component)
	}

	return crtype.ReportTestStatus{BaseStatus: baseStatus, SampleStatus: sampleStatus}, errs
}

func filterByComponent(status map[string]crtype.TestStatus, component string) map[string]crtype.TestStatus {
	filteredStatusMap := make(map[string]crtype.TestStatus)
	for k, v := range status {
		if v.Component == component {
			filteredStatusMap[k] = v
		}
	}

	return filteredStatusMap
}

func filterByCapability(status map[string]crtype.TestStatus, capability string) map[string]crtype.TestStatus {
	filteredStatusMap := make(map[string]crtype.TestStatus)
outerLoop:
	for k, v := range status {
		for _, c := range v.Capabilities {
			if c == capability {
				filteredStatusMap[k] = v
				continue outerLoop
			}
		}
	}

	return filteredStatusMap
}

var componentAndCapabilityGetter func(test crtype.TestIdentification, stats crtype.TestStatus) (string, []string)

func testToComponentAndCapability(_ crtype.TestIdentification, stats crtype.TestStatus) (string, []string) {
	return stats.Component, stats.Capabilities
}

// getRowColumnIdentifications defines the rows and columns since they are variable. For rows, different pages have different row titles (component, capability etc)
// Columns titles depends on the columnGroupBy parameter user requests. A particular test can belong to multiple rows of different capabilities.
func (c *componentReportGenerator) getRowColumnIdentifications(testIDStr string, stats crtype.TestStatus) ([]crtype.RowIdentification, []crtype.ColumnID, error) {
	var test crtype.TestIdentification
	columnGroupByVariants := c.ColumnGroupBy
	// We show column groups by DBGroupBy only for the last page before test details
	if c.TestID != "" {
		columnGroupByVariants = c.DBGroupBy
	}
	// TODO: is this too slow?
	err := json.Unmarshal([]byte(testIDStr), &test)
	if err != nil {
		return []crtype.RowIdentification{}, []crtype.ColumnID{}, err
	}

	component, capabilities := componentAndCapabilityGetter(test, stats)
	rows := []crtype.RowIdentification{}
	// First Page with no component requested
	if c.Component == "" {
		rows = append(rows, crtype.RowIdentification{Component: component})
	} else if c.Component == component {
		// Exact test match
		if c.TestID != "" {
			row := crtype.RowIdentification{
				Component: component,
				TestID:    test.TestID,
				TestName:  stats.TestName,
				TestSuite: stats.TestSuite,
			}
			if c.Capability != "" {
				row.Capability = c.Capability
			}
			rows = append(rows, row)
		} else {
			for _, capability := range capabilities {
				// Exact capability match only produces one row
				if c.Capability != "" {
					if c.Capability == capability {
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

// deserializeRowToTestStatus deserializes a single row into a testID string and matching status.
// This is where we handle the dynamic variant_ columns, parsing these into a map on the test identification.
// Other fixed columns we expect are serialized directly to their appropriate columns.
func deserializeRowToTestStatus(row []bigquery.Value, schema bigquery.Schema) (string, crtype.TestStatus, error) {
	if len(row) != len(schema) {
		log.Infof("row is %+v, schema is %+v", row, schema)
		return "", crtype.TestStatus{}, fmt.Errorf("number of values in row doesn't match schema length")
	}

	// Expect:
	//
	// INFO[2024-04-22T13:31:23.123-03:00] test_id = openshift-tests:75895eeec137789cab3570a252306058
	// INFO[2024-04-22T13:31:23.123-03:00] variants = [standard]
	// INFO[2024-04-22T13:31:23.123-03:00] variant_Network = ovn
	// INFO[2024-04-22T13:31:23.123-03:00] variant_Upgrade = none
	// INFO[2024-04-22T13:31:23.123-03:00] variant_Architecture = amd64
	// INFO[2024-04-22T13:31:23.123-03:00] variant_Platform = gcp
	// INFO[2024-04-22T13:31:23.123-03:00] flat_variants = fips,serial
	// INFO[2024-04-22T13:31:23.123-03:00] variants = [fips serial]
	// INFO[2024-04-22T13:31:23.123-03:00] total_count = %!s(int64=1)
	// INFO[2024-04-22T13:31:23.123-03:00] success_count = %!s(int64=1)
	// INFO[2024-04-22T13:31:23.123-03:00] flake_count = %!s(int64=0)
	// INFO[2024-04-22T13:31:23.124-03:00] component = Cluster Version Operator
	// INFO[2024-04-22T13:31:23.124-03:00] capabilities = [Other]
	// INFO[2024-04-22T13:31:23.124-03:00] jira_component = Cluster Version Operator
	// INFO[2024-04-22T13:31:23.124-03:00] jira_component_id = 12367602000000000/1000000000
	// INFO[2024-04-22T13:31:23.124-03:00] test_name = [sig-storage] [Serial] Volume metrics Ephemeral should create volume metrics in Volume Manager [Suite:openshift/conformance/serial] [Suite:k8s]
	// INFO[2024-04-22T13:31:23.124-03:00] test_suite = openshift-tests
	tid := crtype.TestIdentification{
		Variants: map[string]string{},
	}
	cts := crtype.TestStatus{}
	for i, fieldSchema := range schema {
		col := fieldSchema.Name
		// Some rows we know what to expect, others are dynamic (variants) and go into the map.
		switch {
		case col == "test_id":
			tid.TestID = row[i].(string)
		case col == "test_name":
			cts.TestName = row[i].(string)
		case col == "test_suite":
			cts.TestSuite = row[i].(string)
		case col == "total_count":
			cts.TotalCount = int(row[i].(int64))
		case col == "success_count":
			cts.SuccessCount = int(row[i].(int64))
		case col == "flake_count":
			cts.FlakeCount = int(row[i].(int64))
		case col == "last_failure":
			// ignore when we cant parse, its usually null
			var err error
			if row[i] != nil {
				layout := "2006-01-02T15:04:05"
				lftCivilDT := row[i].(civil.DateTime)
				cts.LastFailure, err = time.Parse(layout, lftCivilDT.String())
				if err != nil {
					log.WithError(err).Error("error parsing last failure time from bigquery")
				}
			}
		case col == "component":
			cts.Component = row[i].(string)
		case col == "capabilities":
			capArr := row[i].([]bigquery.Value)
			cts.Capabilities = make([]string, len(capArr))
			for i := range capArr {
				cts.Capabilities[i] = capArr[i].(string)
			}
		case strings.HasPrefix(col, "variant_"):
			variantName := col[len("variant_"):]
			if row[i] != nil {
				tid.Variants[variantName] = row[i].(string)
			}
		default:
			log.Warnf("ignoring column in query: %s", col)
		}
	}

	// Create a string representation of the test ID so we can use it as a map key throughout:
	// TODO: json better? reversible if we do...
	testIDBytes, err := json.Marshal(tid)

	return string(testIDBytes), cts, err
}

func getMajor(in string) (int, error) {
	major, err := strconv.ParseInt(strings.Split(in, ".")[0], 10, 32)
	if err != nil {
		return 0, err
	}
	return int(major), err
}

func getMinor(in string) (int, error) {
	minor, err := strconv.ParseInt(strings.Split(in, ".")[1], 10, 32)
	if err != nil {
		return 0, err
	}
	return int(minor), err
}

func previousRelease(release string) (string, error) {
	prev := release
	var err error
	var major, minor int
	if major, err = getMajor(release); err == nil {
		if minor, err = getMinor(release); err == nil && minor > 0 {
			prev = fmt.Sprintf("%d.%d", major, minor-1)
		}
	}

	return prev, err
}

func (c *componentReportGenerator) normalizeProwJobName(prowName string) string {
	name := prowName
	if c.BaseRelease.Release != "" {
		name = strings.ReplaceAll(name, c.BaseRelease.Release, "X.X")
		if prev, err := previousRelease(c.BaseRelease.Release); err == nil {
			name = strings.ReplaceAll(name, prev, "X.X")
		}
	}
	if c.BaseOverrideRelease.Release != "" {
		name = strings.ReplaceAll(name, c.BaseOverrideRelease.Release, "X.X")
		if prev, err := previousRelease(c.BaseOverrideRelease.Release); err == nil {
			name = strings.ReplaceAll(name, prev, "X.X")
		}
	}
	if c.SampleRelease.Release != "" {
		name = strings.ReplaceAll(name, c.SampleRelease.Release, "X.X")
		if prev, err := previousRelease(c.SampleRelease.Release); err == nil {
			name = strings.ReplaceAll(name, prev, "X.X")
		}
	}
	// Some jobs encode frequency in their name, which can change
	re := regexp.MustCompile(`-f\d+`)
	name = re.ReplaceAllString(name, "-fXX")

	return name
}

func (c *componentReportGenerator) fetchJobRunTestStatusResults(ctx context.Context,
	query *bigquery.Query) (map[string][]crtype.JobRunTestStatusRow, []error) {
	errs := []error{}
	status := map[string][]crtype.JobRunTestStatusRow{}
	log.Infof("Fetching job run test details with:\n%s\nParameters:\n%+v\n", query.Q, query.Parameters)

	it, err := query.Read(ctx)
	if err != nil {
		log.WithError(err).Error("error querying job run test status from bigquery")
		errs = append(errs, err)
		return status, errs
	}

	for {
		testStatus := crtype.JobRunTestStatusRow{}
		err := it.Next(&testStatus)
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.WithError(err).Error("error parsing component from bigquery")
			errs = append(errs, errors.Wrap(err, "error parsing prowjob from bigquery"))
			continue
		}
		prowName := c.normalizeProwJobName(testStatus.ProwJob)
		rows, ok := status[prowName]
		if !ok {
			status[prowName] = []crtype.JobRunTestStatusRow{testStatus}
		} else {
			rows = append(rows, testStatus)
			status[prowName] = rows
		}
	}
	return status, errs
}

type cellStatus struct {
	status           crtype.Status
	regressedTests   []crtype.ReportTestSummary
	triagedIncidents []crtype.TriageIncidentSummary
}

func getNewCellStatus(testID crtype.ReportTestIdentification,
	testStats crtype.ReportTestStats,
	existingCellStatus *cellStatus,
	triagedIncidents []crtype.TriagedIncident,
	openRegressions []*crtype.TestRegression) cellStatus {
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
		if len(openRegressions) > 0 {
			view := openRegressions[0].View // grab view from first regression, they were queried only for sample release
			or := FindOpenRegression(view, rt.TestID, rt.Variants, openRegressions)
			if or != nil {
				rt.Opened = &or.Opened
			}
		}
		newCellStatus.regressedTests = append(newCellStatus.regressedTests, rt)
	} else if testStats.ReportStatus < crtype.MissingSample {
		ti := crtype.TriageIncidentSummary{
			TriagedIncidents: triagedIncidents,
			ReportTestSummary: crtype.ReportTestSummary{
				ReportTestIdentification: testID,
				ReportTestStats:          testStats,
			}}
		if len(openRegressions) > 0 {
			view := openRegressions[0].View // grab view from first regression, they were queried only for sample release
			or := FindOpenRegression(view, ti.ReportTestSummary.TestID,
				ti.ReportTestSummary.Variants, openRegressions)
			if or != nil {
				ti.ReportTestSummary.Opened = &or.Opened
			}
		}
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
	triagedIncidents []crtype.TriagedIncident,
	openRegressions []*crtype.TestRegression) {
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
				row[columnIdentification] = getNewCellStatus(testID, testStats, nil, triagedIncidents, openRegressions)
				status[rowIdentification] = row
			}
		} else {
			for _, columnIdentification := range columnIdentifications {
				existing, ok := row[columnIdentification]
				if !ok {
					row[columnIdentification] = getNewCellStatus(testID, testStats, nil, triagedIncidents, openRegressions)
				} else {
					row[columnIdentification] = getNewCellStatus(testID, testStats, &existing, triagedIncidents, openRegressions)
				}
			}
		}
	}
}

func (c *componentReportGenerator) getTriagedIssuesFromBigQuery(ctx context.Context,
	testID crtype.ReportTestIdentification) (
	int, []crtype.TriagedIncident, []error) {
	generator := triagedIncidentsGenerator{
		ReportModified: c.GetLastReportModifiedTime(ctx, c.client, c.cacheOption),
		client:         c.client,
		cacheOption:    c.cacheOption,
		SampleRelease:  c.SampleRelease,
	}

	// we want to fetch this once per generator instance which should be once per UI load
	// this is the full list from the cache if available that will be subset to specific test
	// in triagedIssuesFor
	if c.triagedIssues == nil {
		releaseTriagedIncidents, errs := api.GetDataFromCacheOrGenerate[resolvedissues.TriagedIncidentsForRelease](
			ctx, generator.client.Cache, generator.cacheOption, api.GetPrefixedCacheKey("TriagedIncidents~", generator),
			generator.generateTriagedIssuesFor, resolvedissues.TriagedIncidentsForRelease{})

		if len(errs) > 0 {
			return 0, nil, errs
		}
		c.triagedIssues = &releaseTriagedIncidents
	}
	impactedRuns, triagedIncidents := triagedIssuesFor(c.triagedIssues, testID.ColumnIdentification, testID.TestID, c.SampleRelease.Start, c.SampleRelease.End)

	return impactedRuns, triagedIncidents, nil
}

func (c *componentReportGenerator) GetLastReportModifiedTime(ctx context.Context, client *bqcachedclient.Client,
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

		// this gets called a lot, so we want to set it once on the componentReportGenerator
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
	query *bigquery.Query) (*time.Time,
	[]error) {
	log.Infof("Fetching triaged incidents last modified time with:\n%s\nParameters:\n%+v\n", query.Q, query.Parameters)

	it, err := query.Read(ctx)
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

func triagedIssuesFor(releaseIncidents *resolvedissues.TriagedIncidentsForRelease, variant crtype.ColumnIdentification, testID string, startTime, endTime time.Time) (int, []crtype.TriagedIncident) {
	if releaseIncidents == nil {
		return 0, nil
	}

	inKey := resolvedissues.KeyForTriagedIssue(testID, resolvedissues.TransformVariant(variant))

	triagedIncidents := releaseIncidents.TriagedIncidents[inKey]
	relevantIncidents := []crtype.TriagedIncident{}

	impactedJobRuns := sets.NewString() // because multiple issues could impact the same job run, be sure to count each job run only once
	numJobRunsToSuppress := 0
	for _, triagedIncident := range triagedIncidents {
		startNumRunsSuppressed := numJobRunsToSuppress
		for _, impactedJobRun := range triagedIncident.JobRuns {
			if impactedJobRuns.Has(impactedJobRun.URL) {
				continue
			}
			impactedJobRuns.Insert(impactedJobRun.URL)

			compareTime := impactedJobRun.StartTime
			// preference is to compare to CompletedTime as it will more closely match jobrun modified time
			// but, it is optional so default to StartTime and set to CompletedTime when present
			if impactedJobRun.CompletedTime.Valid {
				compareTime = impactedJobRun.CompletedTime.Timestamp
			}

			if compareTime.After(startTime) && compareTime.Before(endTime) {
				numJobRunsToSuppress++
			}
		}

		if numJobRunsToSuppress > startNumRunsSuppressed {
			relevantIncidents = append(relevantIncidents, triagedIncident)
		}
	}

	// if we didn't have any jobs that matched the compare time then return nil
	if numJobRunsToSuppress == 0 {
		relevantIncidents = nil
	}

	return numJobRunsToSuppress, relevantIncidents
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
	query *bigquery.Query) ([]crtype.TriagedIncident,
	[]error) {
	errs := make([]error, 0)
	incidents := make([]crtype.TriagedIncident, 0)
	log.Infof("Fetching triaged incidents with:\n%s\nParameters:\n%+v\n", query.Q, query.Parameters)

	it, err := query.Read(ctx)
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

func (c *componentReportGenerator) triagedIncidentsFor(ctx context.Context,
	testID crtype.ReportTestIdentification) (int,
	[]crtype.TriagedIncident) {
	// handle test case / missing client
	if c.client == nil {
		return 0, nil
	}

	impactedRuns, triagedIncidents, errs := c.getTriagedIssuesFromBigQuery(ctx, testID)

	if errs != nil {
		for _, err := range errs {
			log.WithError(err).Error("error getting triaged issues component from bigquery")
		}
		return 0, nil
	}

	return impactedRuns, triagedIncidents
}

// getRequiredConfidence returns the required certainty of a regression before we include it in the report as a
// regressed test. This is to introduce some hysteresis into the process so once a regression creeps over the 95%
// confidence we typically use, dropping to 94.9% should not make the cell immediately green.
//
// Instead, once you cross the confidence threshold and a regression begins tracking in the openRegressions list,
// we'll require less confidence for that test until the regression is closed. (-5%) Once the certainty drops below that
// modified confidence, the regression will be closed and the -5% adjuster is gone.
//
// ie. if the request was for 95% confidence, but we see that a test has an open regression (meaning at some point recently
// we were over 95% certain of a regression), we're going to only require 90% certainty to mark that test red.
func (c *componentReportGenerator) getRequiredConfidence(testID string, variants map[string]string) int {
	if len(c.openRegressions) > 0 {
		view := c.openRegressions[0].View // grab view from first regression, they were queried only for sample release
		or := FindOpenRegression(view, testID, variants, c.openRegressions)
		if or != nil {
			log.Debugf("adjusting required regression confidence from %d to %d because %s (%v) has an open regression since %s",
				c.RequestAdvancedOptions.Confidence,
				c.RequestAdvancedOptions.Confidence-openRegressionConfidenceAdjustment,
				testID,
				variants,
				or.Opened)
			return c.RequestAdvancedOptions.Confidence /*- openRegressionConfidenceAdjustment*/
		}
	}
	return c.RequestAdvancedOptions.Confidence
}

// matchBaseRegression returns a testStatus that reflects the allowances specified
// in an intentional regression that accepted a lower threshold but maintains the higher
// threshold when used as a basis.  It will ignore intentional regressions if we are relying
// on fallback to find the highest threshold.  It will return the original testStatus if there
// is no intentional regression or the testStatus has a higher threshold
func (c *componentReportGenerator) matchBaseRegression(testID crtype.ReportTestIdentification, baseRelease string, baseStats crtype.TestStatus) (crtype.TestStatus, string) {
	var baseRegression *regressionallowances.IntentionalRegression
	if !c.IncludeMultiReleaseAnalysis && len(c.VariantCrossCompare) == 0 {
		// only really makes sense when not cross-comparing variants:
		// look for corresponding regressions we can account for in the analysis
		// only if we are ignoring fallback, otherwise we will let fallback determine the threshold
		baseRegression = regressionallowances.IntentionalRegressionFor(baseRelease, testID.ColumnIdentification, testID.TestID)

		// This could go away if we remove the option for ignoring fallback
		if baseRegression != nil && baseRegression.PreviousPassPercentage(c.FlakeAsFailure) > c.getTestStatusPassRate(baseStats) {
			// override with  the basis regression previous values
			// testStats will reflect the expected threshold, not the computed values from the release with the allowed regression
			baseRegressionPreviousRelease, err := previousRelease(c.BaseRelease.Release)
			if err != nil {
				log.WithError(err).Error("Failed to determine the previous release for baseRegression")
			} else {
				// create a clone since we might be updating a cached item though the same regression would likely apply each time...
				updatedStats := crtype.TestStatus{TestName: baseStats.TestName, TestSuite: baseStats.TestSuite, Capabilities: baseStats.Capabilities,
					Component: baseStats.Component, Variants: baseStats.Variants,
					FlakeCount:   baseRegression.PreviousFlakes,
					SuccessCount: baseRegression.PreviousSuccesses,
					TotalCount:   baseRegression.PreviousFailures + baseRegression.PreviousFlakes + baseRegression.PreviousSuccesses,
				}
				baseStats = updatedStats
				baseRelease = baseRegressionPreviousRelease
				log.Infof("BaseRegression - PreviousPassPercentage overrides baseStats.  Release: %s, Successes: %d, Flakes: %d, Total: %d", baseRelease, baseStats.SuccessCount, baseStats.FlakeCount, baseStats.TotalCount)
			}
		}
	}

	return baseStats, baseRelease
}

// matchBestBaseStats returns the testStatus, release and reportTestStatus
// that has the highest threshold across the basis release and previous releases included
// in fallback comparison
func (c *componentReportGenerator) matchBestBaseStats(testID crtype.ReportTestIdentification, testIdentification, baseRelease string, baseStats, sampleStats crtype.TestStatus, requiredConfidence int, approvedRegression *regressionallowances.IntentionalRegression, numberOfIgnoredSampleJobRuns int) (crtype.TestStatus, string, crtype.ReportTestStats) {

	// The hope is that this goes away
	// once we agree we don't need to honor a higher intentional regression pass percentage
	baseStats, baseRelease = c.matchBaseRegression(testID, baseRelease, baseStats)
	baseStatsTotal := baseStats.TotalCount

	baseTestStats := c.assessComponentStatus(requiredConfidence, sampleStats.TotalCount, sampleStats.SuccessCount,
		sampleStats.FlakeCount, baseStats.TotalCount, baseStats.SuccessCount,
		baseStats.FlakeCount, approvedRegression, numberOfIgnoredSampleJobRuns, baseRelease, nil, nil)

	if !c.IncludeMultiReleaseAnalysis {
		return baseStats, baseRelease, baseTestStats
	}

	if c.cachedFallbackTestStatuses == nil {
		log.Errorf("Invalid fallback test statuses")
		return baseStats, baseRelease, baseTestStats
	}

	var priorRelease = baseRelease
	var err error
	for err == nil {
		var cachedTestStatuses crtype.ReleaseTestMap
		var cTestStats crtype.TestStatus
		ok := false
		priorRelease, err = previousRelease(priorRelease)
		// if we fail to determine the previous release then stop
		if err != nil {
			return baseStats, baseRelease, baseTestStats
		}
		// if we hit a missing release then stop
		if cachedTestStatuses, ok = c.cachedFallbackTestStatuses.Releases[priorRelease]; !ok {
			return baseStats, baseRelease, baseTestStats
		}
		// it's ok if we don't have a testIdentification for this release
		// we likely won't have it for earlier releases either, but we can keep going
		if cTestStats, ok = cachedTestStatuses.Tests[testIdentification]; ok {

			// what is our base total compared to the original base
			// this happens when jobs shift like sdn -> ovn
			// if we get below threshold that's a sign we are reducing our base signal
			if float64(cTestStats.TotalCount)/float64(baseStatsTotal) < .6 {
				log.Debugf("Fallback base total: %d to low for fallback analysis compared to original: %d", cTestStats.TotalCount, baseStatsTotal)
				return baseStats, baseRelease, baseTestStats
			}

			cTestStats, priorRelease = c.matchBaseRegression(testID, priorRelease, cTestStats)

			priorTestStats := c.assessComponentStatus(requiredConfidence, sampleStats.TotalCount, sampleStats.SuccessCount,
				sampleStats.FlakeCount, cTestStats.TotalCount, cTestStats.SuccessCount,
				cTestStats.FlakeCount, approvedRegression, numberOfIgnoredSampleJobRuns, priorRelease, cachedTestStatuses.Start, cachedTestStatuses.End)

			if priorTestStats.ReportStatus < baseTestStats.ReportStatus {
				baseStats = cTestStats
				baseTestStats = priorTestStats
				baseRelease = priorRelease
			}
		}
	}

	return baseStats, baseRelease, baseTestStats
}

// TODO: break this function down and remove this nolint
// nolint:gocyclo
func (c *componentReportGenerator) generateComponentTestReport(ctx context.Context,
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
	baseReleaseMatches := 0
	baseReleaseMisses := 0
	overriddenBaseMatches := 0

	for testIdentification, baseStats := range baseStatus {
		testID, err := buildTestID(baseStats, testIdentification)
		if err != nil {
			return crtype.ComponentReport{}, err
		}

		var testStats crtype.ReportTestStats
		var triagedIncidents []crtype.TriagedIncident
		var resolvedIssueCompensation int // triaged job run failures to ignore
		sampleStats, ok := sampleStatus[testIdentification]
		if !ok {
			testStats.ReportStatus = crtype.MissingSample
		} else {
			// requiredConfidence is lowered for on-going regressions to prevent cells from flapping:
			requiredConfidence := c.getRequiredConfidence(testID.TestID, testID.Variants)
			var approvedRegression *regressionallowances.IntentionalRegression
			if len(c.VariantCrossCompare) == 0 { // only really makes sense when not cross-comparing variants:
				// look for corresponding regressions we can account for in the analysis
				approvedRegression = regressionallowances.IntentionalRegressionFor(c.SampleRelease.Release, testID.ColumnIdentification, testID.TestID)
				// ignore triage if we have an intentional regression
				if approvedRegression == nil {
					resolvedIssueCompensation, triagedIncidents = c.triagedIncidentsFor(ctx, testID)
				}
			}

			// this is where we look to see if a previous release has a higher pass rate
			matchedBaseRelease := c.BaseRelease.Release
			baseStats, matchedBaseRelease, testStats = c.matchBestBaseStats(testID, testIdentification, matchedBaseRelease, baseStats, sampleStats, requiredConfidence, approvedRegression, resolvedIssueCompensation)

			if matchedBaseRelease != c.BaseRelease.Release {
				log.Infof("Overrode base stats using release %s for Test: %s - %s", matchedBaseRelease, baseStats.TestName, testIdentification)
				overriddenBaseMatches++
			}
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
						if !ti.Issue.ResolutionDate.Valid || ti.Issue.ResolutionDate.Timestamp.After(c.SampleRelease.End) {
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
		delete(sampleStatus, testIdentification)

		rowIdentifications, columnIdentifications, err := c.getRowColumnIdentifications(testIdentification, baseStats)
		if err != nil {
			return crtype.ComponentReport{}, err
		}
		updateCellStatus(rowIdentifications, columnIdentifications, testID, testStats, aggregatedStatus, allRows, allColumns, triagedIncidents, c.openRegressions)
	}

	log.Infof("BaseStats: %d, baseMatches: %d, baseMisses: %d.  Overridden base stats: %d", len(baseStatus), baseReleaseMatches, baseReleaseMisses, overriddenBaseMatches)

	// Anything we saw in the basis was removed above, all that remains are tests with no basis, typically new
	// tests, or tests that were renamed without submitting a rename to the test mapping repo.
	for testIdentification, sampleStats := range sampleStatus {
		testID, err := buildTestID(sampleStats, testIdentification)
		if err != nil {
			return crtype.ComponentReport{}, err
		}

		// Check for approved regressions, and triaged incidents, which may adjust our counts and pass rate:
		var testStats crtype.ReportTestStats
		var triagedIncidents []crtype.TriagedIncident
		var resolvedIssueCompensation int // triaged job run failures to ignore
		// look for corresponding regressions we can account for in the analysis
		approvedRegression := regressionallowances.IntentionalRegressionFor(c.SampleRelease.Release, testID.ColumnIdentification, testID.TestID)
		// ignore triage if we have an intentional regression
		if approvedRegression == nil {
			resolvedIssueCompensation, triagedIncidents = c.triagedIncidentsFor(ctx, testID)
		}

		requiredConfidence := 0 // irrelevant for pass rate comparison
		testStats = c.assessComponentStatus(requiredConfidence, sampleStats.TotalCount, sampleStats.SuccessCount,
			sampleStats.FlakeCount, 0, 0, 0, // pass 0s for base stats
			approvedRegression, resolvedIssueCompensation, "", nil, nil)

		if testStats.IsTriaged() {
			// we are within the triage range
			// do we want to show the triage icon or flip reportStatus
			canClearReportStatus := true
			for _, ti := range triagedIncidents {
				if ti.Issue.Type != string(resolvedissues.TriageIssueTypeInfrastructure) {
					// if a non Infrastructure regression isn't marked resolved or the resolution date is after the end of our sample query
					// then we won't clear it.  Otherwise, we can.
					if !ti.Issue.ResolutionDate.Valid || ti.Issue.ResolutionDate.Timestamp.After(c.SampleRelease.End) {
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

		rowIdentifications, columnIdentification, err := c.getRowColumnIdentifications(testIdentification, sampleStats)
		if err != nil {
			return crtype.ComponentReport{}, err
		}
		updateCellStatus(rowIdentifications, columnIdentification, testID, testStats, aggregatedStatus, allRows, allColumns, nil, c.openRegressions)
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

func buildTestID(stats crtype.TestStatus, testIdentificationStr string) (crtype.ReportTestIdentification, error) {
	// TODO: function needs a rename, there's a lot of references to test ID/identification around.
	var testIdentification crtype.TestIdentification
	// TODO: is this too slow?
	err := json.Unmarshal([]byte(testIdentificationStr), &testIdentification)
	if err != nil {
		log.WithError(err).Errorf("trying to unmarshel %s", testIdentificationStr)
		return crtype.ReportTestIdentification{}, err
	}
	testID := crtype.ReportTestIdentification{
		RowIdentification: crtype.RowIdentification{
			Component: stats.Component,
			TestName:  stats.TestName,
			TestSuite: stats.TestSuite,
			TestID:    testIdentification.TestID,
		},
		ColumnIdentification: crtype.ColumnIdentification{
			Variants: testIdentification.Variants,
		},
	}
	// Take the first cap for now. When we reach to a cell with specific capability, we will override the value.
	if len(stats.Capabilities) > 0 {
		testID.Capability = stats.Capabilities[0]
	}
	return testID, nil
}

func getFailureCount(status crtype.JobRunTestStatusRow) int {
	failure := status.TotalCount - status.SuccessCount - status.FlakeCount
	if failure < 0 {
		failure = 0
	}
	return failure
}

func (c *componentReportGenerator) getTestStatusPassRate(testStatus crtype.TestStatus) float64 {
	return c.getPassRate(testStatus.SuccessCount, testStatus.TotalCount-testStatus.SuccessCount-testStatus.FlakeCount, testStatus.FlakeCount)
}

func (c *componentReportGenerator) getPassRate(success, failure, flake int) float64 {
	total := success + failure + flake
	if total == 0 {
		return 0.0
	}
	if c.FlakeAsFailure {
		return float64(success) / float64(total)
	}
	return float64(success+flake) / float64(total)
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

func (c *componentReportGenerator) getEffectivePityFactor(basisPassPercentage float64, approvedRegression *regressionallowances.IntentionalRegression) int {
	if approvedRegression != nil && approvedRegression.RegressedFailures > 0 {
		regressedPassPercentage := approvedRegression.RegressedPassPercentage(c.FlakeAsFailure)
		if regressedPassPercentage < basisPassPercentage {
			// product owner chose a required pass percentage, so we allow pity to cover that approved pass percent
			// plus the existing pity factor to limit, "well, it's just *barely* lower" arguments.
			effectivePityFactor := int(basisPassPercentage*100) - int(regressedPassPercentage*100) + c.PityFactor

			if effectivePityFactor < c.PityFactor {
				log.Errorf("effective pity factor for %+v is below zero: %d", approvedRegression, effectivePityFactor)
				effectivePityFactor = c.PityFactor
			}

			return effectivePityFactor
		}
	}
	return c.PityFactor
}

func (c *componentReportGenerator) assessComponentStatus(
	requiredConfidence,
	sampleTotal,
	sampleSuccess,
	sampleFlake,
	baseTotal,
	baseSuccess,
	baseFlake int,
	approvedRegression *regressionallowances.IntentionalRegression,
	numberOfIgnoredSampleJobRuns int,
	baseRelease string,
	baseStart,
	baseEnd *time.Time) crtype.ReportTestStats {

	// if we don't have a valid set of start and end dates we default to the baseRelease values
	if baseStart == nil || baseEnd == nil {
		baseStart = &c.BaseRelease.Start
		baseEnd = &c.BaseRelease.End
	}
	// preserve the initial sampleTotal, so we can check
	// to see if numberOfIgnoredSampleJobRuns impacts the status
	initialSampleTotal := sampleTotal
	adjustedSampleTotal := sampleTotal - numberOfIgnoredSampleJobRuns
	if adjustedSampleTotal < sampleSuccess {
		log.Errorf("adjustedSampleTotal is too small: sampleTotal=%d, numberOfIgnoredSampleJobRuns=%d, sampleSuccess=%d", sampleTotal, numberOfIgnoredSampleJobRuns, sampleSuccess)
	} else {
		sampleTotal = adjustedSampleTotal
	}

	sampleFailure := sampleTotal - sampleSuccess - sampleFlake
	// The adjusted total for ignored runs can push failure count into the negatives if there were
	// more ignored runs than actual failures. (or no failures at all)
	if sampleFailure < 0 {
		sampleFailure = 0
	}
	baseFailure := baseTotal - baseSuccess - baseFlake

	if baseTotal == 0 && c.RequestAdvancedOptions.PassRateRequiredNewTests > 0 {
		// If we have no base stats, fall back to a raw pass rate comparison for new or improperly renamed tests:
		testStats := c.buildPassRateTestStats(sampleSuccess, sampleFailure, sampleFlake,
			float64(c.RequestAdvancedOptions.PassRateRequiredNewTests))
		// If a new test reports no regression, and we're not using pass rate mode for all tests, we alter
		// status to be missing basis for the pre-existing Fisher Exact behavior:
		if testStats.ReportStatus == crtype.NotSignificant && c.RequestAdvancedOptions.PassRateRequiredAllTests == 0 {
			testStats.ReportStatus = crtype.MissingBasis
		}
		return testStats
	} else if c.RequestAdvancedOptions.PassRateRequiredAllTests > 0 {
		// If requested, switch to pass rate only testing to see what does not meet the criteria:
		testStats := c.buildPassRateTestStats(sampleSuccess, sampleFailure, sampleFlake,
			float64(c.RequestAdvancedOptions.PassRateRequiredAllTests))
		// include base stats even though we didn't do fishers exact here, this is helpful
		// for the test details page to give a visual on how the test behaved in the basis
		testStats.BaseStats = &crtype.TestDetailsReleaseStats{
			Release: baseRelease,
			Start:   baseStart,
			End:     baseEnd,
			TestDetailsTestStats: crtype.TestDetailsTestStats{
				SuccessRate:  c.getPassRate(baseSuccess, baseFailure, baseFlake),
				SuccessCount: baseSuccess,
				FailureCount: baseFailure,
				FlakeCount:   baseFlake,
			},
		}

		return testStats
	}

	// Otherwise we fall back to default behavior of Fishers Exact test:
	testStats := c.buildFisherExactTestStats(requiredConfidence, sampleTotal, sampleSuccess, sampleFlake, sampleFailure, baseTotal, baseSuccess, baseFlake, baseFailure, approvedRegression, initialSampleTotal, baseRelease, baseStart, baseEnd)
	return testStats
}

func (c *componentReportGenerator) buildFisherExactTestStats(requiredConfidence, sampleTotal, sampleSuccess, sampleFlake, sampleFailure, baseTotal, baseSuccess, baseFlake, baseFailure int, approvedRegression *regressionallowances.IntentionalRegression, initialSampleTotal int, baseRelease string, baseStart, baseEnd *time.Time) crtype.ReportTestStats {

	fisherExact := 0.0
	baseStats := &crtype.TestDetailsReleaseStats{
		Release: baseRelease,
		Start:   baseStart,
		End:     baseEnd,
		TestDetailsTestStats: crtype.TestDetailsTestStats{
			SuccessRate:  c.getPassRate(baseSuccess, baseFailure, baseFlake),
			SuccessCount: baseSuccess,
			FailureCount: baseFailure,
			FlakeCount:   baseFlake,
		},
	}

	testStats := crtype.ReportTestStats{
		Comparison: crtype.FisherExact,
		SampleStats: crtype.TestDetailsReleaseStats{
			Release: c.SampleRelease.Release,
			Start:   &c.SampleRelease.Start,
			End:     &c.SampleRelease.End,
			TestDetailsTestStats: crtype.TestDetailsTestStats{
				SuccessRate:  c.getPassRate(sampleSuccess, sampleFailure, sampleFlake),
				SuccessCount: sampleSuccess,
				FailureCount: sampleFailure,
				FlakeCount:   sampleFlake,
			},
		},
		BaseStats:    baseStats,
		Explanations: []string{explanationNoRegression},
	}
	status := crtype.MissingBasis
	// if the unadjusted sample was 0 then nothing to do
	if initialSampleTotal == 0 {
		if c.IgnoreMissing {
			status = crtype.NotSignificant
		} else {
			status = crtype.MissingSample
		}
	} else {
		// see if we had a significant regression prior to adjusting
		basePass := baseSuccess + baseFlake
		samplePass := sampleSuccess + sampleFlake
		if c.FlakeAsFailure {
			basePass = baseSuccess
			samplePass = sampleSuccess
		}
		basisPassPercentage := float64(basePass) / float64(baseTotal)
		initialPassPercentage := float64(samplePass) / float64(initialSampleTotal)
		effectivePityFactor := c.getEffectivePityFactor(basisPassPercentage, approvedRegression)

		wasSignificant := false
		// only consider wasSignificant if the sampleTotal has been changed and our sample
		// pass percentage is below the basis
		if initialSampleTotal > sampleTotal && initialPassPercentage < basisPassPercentage {
			if basisPassPercentage-initialPassPercentage > float64(c.PityFactor)/100 {
				wasSignificant, _ = c.fischerExactTest(requiredConfidence, initialSampleTotal-samplePass, samplePass, baseTotal-basePass, basePass)
			}
			// if it was significant without the adjustment use
			// ExtremeTriagedRegression or SignificantTriagedRegression
			if wasSignificant {
				status = getRegressionStatus(basisPassPercentage, initialPassPercentage, true)
			}
		}

		if sampleTotal == 0 {
			if !wasSignificant {
				if c.IgnoreMissing {
					status = crtype.NotSignificant

				} else {
					status = crtype.MissingSample
				}
			}
			return crtype.ReportTestStats{
				Comparison:   crtype.FisherExact,
				ReportStatus: status,
				FisherExact:  thrift.Float64Ptr(0.0),
				Explanations: []string{explanationNoRegression},
			}
		}

		// if we didn't detect a significant regression prior to adjusting set our default here
		if !wasSignificant {
			status = crtype.NotSignificant
		}

		// now that we know sampleTotal is non zero
		samplePassPercentage := float64(samplePass) / float64(sampleTotal)

		// did we remove enough failures that we are below the MinimumFailure threshold?
		if c.MinimumFailure != 0 && (sampleTotal-samplePass) < c.MinimumFailure {
			testStats.ReportStatus = status
			testStats.FisherExact = thrift.Float64Ptr(0.0)
			return testStats
		}
		significant := false
		improved := samplePassPercentage >= basisPassPercentage

		if improved {
			// flip base and sample when improved
			significant, fisherExact = c.fischerExactTest(requiredConfidence, baseTotal-basePass, basePass, sampleTotal-samplePass, samplePass)
		} else if basisPassPercentage-samplePassPercentage > float64(effectivePityFactor)/100 {
			significant, fisherExact = c.fischerExactTest(requiredConfidence, sampleTotal-samplePass, samplePass, baseTotal-basePass, basePass)
		}
		if significant {
			if improved {
				// only show improvements if we are not dropping out triaged results
				if initialSampleTotal == sampleTotal {
					status = crtype.SignificantImprovement
				}
			} else {
				status = getRegressionStatus(basisPassPercentage, samplePassPercentage, false)
			}
		}
	}
	testStats.ReportStatus = status
	testStats.FisherExact = thrift.Float64Ptr(fisherExact)

	// If we have a regression, include explanations:
	if testStats.ReportStatus <= crtype.SignificantTriagedRegression {
		testStats.Explanations = []string{
			fmt.Sprintf("%s regression detected.", crtype.StringForStatus(testStats.ReportStatus)),
			fmt.Sprintf("Fishers Exact probability of a regression: %.2f%%.", float64(100)-*testStats.FisherExact),
			fmt.Sprintf("Test pass rate dropped from %.2f%% to %.2f%%.",
				testStats.BaseStats.SuccessRate*float64(100),
				testStats.SampleStats.SuccessRate*float64(100)),
		}
		// check for override
		if baseRelease != c.BaseRelease.Release {
			testStats.Explanations = append(testStats.Explanations, fmt.Sprintf("Overrode base stats using release %s", baseRelease))
		}
	}

	return testStats
}

func (c *componentReportGenerator) buildPassRateTestStats(sampleSuccess, sampleFailure, sampleFlake int, requiredSuccessRate float64) crtype.ReportTestStats {
	successRate := c.getPassRate(sampleSuccess, sampleFailure, sampleFlake)

	// Assume 2x our allowed failure rate = an extreme regression.
	// i.e. if we require 90%, extreme is anything below 80%
	//      if we require 95%, extreme is anything below 90%
	severeRegressionSuccessRate := requiredSuccessRate - (100 - requiredSuccessRate)

	// Require 7 runs in the sample (typically 1 week) for us to consider a pass rate requirement for a new test:
	sufficientRuns := (sampleSuccess + sampleFailure + sampleFlake) >= 7

	if sufficientRuns && successRate*100 < requiredSuccessRate && sampleFailure >= c.MinimumFailure {
		rStatus := crtype.SignificantRegression
		if successRate*100 < severeRegressionSuccessRate {
			rStatus = crtype.ExtremeRegression
		}
		return crtype.ReportTestStats{
			ReportStatus: rStatus,
			Explanations: []string{
				fmt.Sprintf("Test has a %.2f%% pass rate, but %.2f%% is required.", successRate*100, requiredSuccessRate),
			},
			Comparison: crtype.PassRate,
			SampleStats: crtype.TestDetailsReleaseStats{
				Release: c.SampleRelease.Release,
				Start:   &c.SampleRelease.Start,
				End:     &c.SampleRelease.End,
				TestDetailsTestStats: crtype.TestDetailsTestStats{
					SuccessRate:  successRate,
					SuccessCount: sampleSuccess,
					FailureCount: sampleFailure,
					FlakeCount:   sampleFlake,
				},
			},
		}
	}
	return crtype.ReportTestStats{
		ReportStatus: crtype.NotSignificant,
		Explanations: []string{explanationNoRegression},
	}
}

func (c *componentReportGenerator) fischerExactTest(confidenceRequired, sampleFailure, sampleSuccess, baseFailure, baseSuccess int) (bool, float64) {
	_, _, r, _ := fischer.FisherExactTest(sampleFailure,
		sampleSuccess,
		baseFailure,
		baseSuccess)
	return r < 1-float64(confidenceRequired)/100, r
}

func (c *componentReportGenerator) getUniqueJUnitColumnValuesLast60Days(ctx context.Context, field string,
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
						NOT REGEXP_CONTAINS(prowjob_name, @IgnoredJobs)
						AND modified_time > DATETIME_SUB(CURRENT_DATETIME(), INTERVAL 60 DAY)
					ORDER BY
						name`, field, c.client.Dataset, unnest)

	query := c.client.BQ.Query(queryString)
	query.Parameters = []bigquery.QueryParameter{
		{
			Name:  "IgnoredJobs",
			Value: ignoredJobsRegexp,
		},
	}

	return getSingleColumnResultToSlice(ctx, query)
}

func init() {
	componentAndCapabilityGetter = testToComponentAndCapability
}
