package api

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	fischer "github.com/glycerine/golang-fisher-exact"
	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/util/sets"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
)

func getSingleColumnResultToSlice(query *bigquery.Query, names *[]string) error {
	it, err := query.Read(context.TODO())
	if err != nil {
		log.WithError(err).Error("error querying test status from bigquery")
		return err
	}

	for {
		row := struct{ Name string }{}
		err := it.Next(&row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.WithError(err).Error("error parsing component from bigquery")
			return err
		}
		*names = append(*names, row.Name)
	}
	return nil
}

func GetComponentTestVariantsFromBigQuery(client *bigquery.Client) (apitype.ComponentReportTestVariants, []error) {
	result := apitype.ComponentReportTestVariants{}
	errs := []error{}
	queryString := `SELECT DISTINCT platform as name FROM ci_analysis_us.junit ORDER BY name`
	query := client.Query(queryString)
	err := getSingleColumnResultToSlice(query, &result.Platform)
	if err != nil {
		log.WithError(err).Error("error querying platforms from bigquery")
		errs = append(errs, err)
		return result, errs
	}
	queryString = `SELECT DISTINCT network as name FROM ci_analysis_us.junit ORDER BY name`
	query = client.Query(queryString)
	err = getSingleColumnResultToSlice(query, &result.Network)
	if err != nil {
		log.WithError(err).Error("error querying networks from bigquery")
		errs = append(errs, err)
		return result, errs
	}
	queryString = `SELECT DISTINCT arch as name FROM ci_analysis_us.junit ORDER BY name`
	query = client.Query(queryString)
	err = getSingleColumnResultToSlice(query, &result.Arch)
	if err != nil {
		log.WithError(err).Error("error querying arches from bigquery")
		errs = append(errs, err)
		return result, errs
	}
	queryString = `SELECT DISTINCT upgrade as name FROM ci_analysis_us.junit ORDER BY name`
	query = client.Query(queryString)
	err = getSingleColumnResultToSlice(query, &result.Upgrade)
	if err != nil {
		log.WithError(err).Error("error querying upgrades from bigquery")
		errs = append(errs, err)
		return result, errs
	}
	queryString = `SELECT DISTINCT flat_variants as name FROM ci_analysis_us.junit ORDER BY name`
	query = client.Query(queryString)
	flatVariants := []string{}
	err = getSingleColumnResultToSlice(query, &flatVariants)
	if err != nil {
		log.WithError(err).Error("error querying platforms from bigquery")
		errs = append(errs, err)
		return result, errs
	}
	uniqueVariants := sets.String{}
	for _, flatVariant := range flatVariants {
		variants := strings.Split(flatVariant, ",")
		for _, variant := range variants {
			uniqueVariants.Insert(variant)
		}
	}
	result.Variant = uniqueVariants.List()

	return result, errs
}

func GetComponentReportFromBigQuery(client *bigquery.Client,
	baseRelease, sampleRelease, component, capability, platform, upgrade, arch, network, testId, groupBy string,
	excludePlatforms, excludeArches, excludeNetworks, excludeUpgrades, excludeVariants string,
	baseStartTime, baseEndTime, sampleStartTime, sampleEndTime time.Time,
	confidence, minFailure, pityFactor int, ignoreMissing, ignoreDisruption bool) (apitype.ComponentReport, []error) {
	generator := componentReportGenerator{
		client:           client,
		baseRelease:      baseRelease,
		sampleRelease:    sampleRelease,
		component:        component,
		capability:       capability,
		platform:         platform,
		upgrade:          upgrade,
		arch:             arch,
		network:          network,
		testId:           testId,
		groupBy:          groupBy,
		excludePlatforms: excludePlatforms,
		excludeArches:    excludeArches,
		excludeNetworks:  excludeNetworks,
		excludeUpgrades:  excludeUpgrades,
		excludeVariants:  excludeVariants,
		baseStartTime:    baseStartTime,
		baseEndTime:      baseEndTime,
		sampleStartTime:  sampleStartTime,
		sampleEndTime:    sampleEndTime,
		confidence:       confidence,
		minimumFailure:   minFailure,
		pityFactor:       pityFactor,
		ignoreMissing:    ignoreMissing,
		ignoreDisruption: ignoreDisruption,
	}
	return generator.GenerateReport()
}

type componentReportGenerator struct {
	client           *bigquery.Client
	baseRelease      string
	sampleRelease    string
	component        string
	capability       string
	platform         string
	upgrade          string
	arch             string
	network          string
	testId           string
	groupBy          string
	excludePlatforms string
	excludeArches    string
	excludeNetworks  string
	excludeUpgrades  string
	excludeVariants  string
	baseStartTime    time.Time
	baseEndTime      time.Time
	sampleStartTime  time.Time
	sampleEndTime    time.Time
	minimumFailure   int
	confidence       int
	pityFactor       int
	ignoreMissing    bool
	ignoreDisruption bool
}

func (c *componentReportGenerator) GenerateReport() (apitype.ComponentReport, []error) {
	baseStatus, sampleStatus, errs := c.getTestStatusFromBigQuery()
	if len(errs) > 0 {
		return apitype.ComponentReport{}, errs
	}
	report := c.generateComponentTestReport(baseStatus, sampleStatus)
	return report, nil
}

func (c *componentReportGenerator) getTestStatusFromBigQuery() (
	map[apitype.ComponentTestIdentification]apitype.ComponentTestStats,
	map[apitype.ComponentTestIdentification]apitype.ComponentTestStats,
	[]error,
) {
	errs := []error{}
	// NOTE: casting a couple datetime columns to timestamps, it does appear they go in as UTC, and thus come out
	// as the default UTC correctly.
	// Annotations and labels can be queried here if we need them.
	queryString := `SELECT
			network,
			upgrade,
			arch,
			platform,
			test_id,
            ANY_VALUE(test_name) AS test_name,
            COUNT(test_id) AS total_count,
			SUM(success_val) AS success_count,
			SUM(flake_count) AS flake_count ` +
		"FROM `ci_analysis_us.junit` " +
		`WHERE TIMESTAMP(modified_time) >= @From AND TIMESTAMP(modified_time) < @To `

	if c.upgrade != "" {
		queryString = queryString + ` AND upgrade = "` + c.upgrade + `"`
	}
	if c.arch != "" {
		queryString = queryString + ` AND arch = "` + c.arch + `"`
	}
	if c.network != "" {
		queryString = queryString + ` AND network = "` + c.network + `"`
	}
	if c.platform != "" {
		queryString = queryString + ` AND platform = "` + c.platform + `"`
	}
	if c.testId != "" {
		queryString = queryString + ` AND test_id = "` + c.testId + `"`
	}

	if c.excludePlatforms != "" {
		excludePlatforms := sets.NewString(strings.Split(c.excludePlatforms, ",")...)
		for platform := range excludePlatforms {
			queryString = queryString + ` AND platform != "` + platform + `"`
		}
	}
	if c.excludeArches != "" {
		excludeArches := sets.NewString(strings.Split(c.excludeArches, ",")...)
		for arch := range excludeArches {
			queryString = queryString + ` AND arch != "` + arch + `"`
		}
	}
	if c.excludeNetworks != "" {
		excludeNetworks := sets.NewString(strings.Split(c.excludeNetworks, ",")...)
		for network := range excludeNetworks {
			queryString = queryString + ` AND network != "` + network + `"`
		}
	}
	if c.excludeUpgrades != "" {
		excludeUpgrades := sets.NewString(strings.Split(c.excludeUpgrades, ",")...)
		for upgrade := range excludeUpgrades {
			queryString = queryString + ` AND upgrade != "` + upgrade + `"`
		}
	}
	if c.excludeVariants != "" {
		excludeVariants := sets.NewString(strings.Split(c.excludeVariants, ",")...)
		for variant := range excludeVariants {
			queryString = queryString + ` AND variant != "` + variant + `"`
		}
	}

	groupString := `
		GROUP BY
		network,
		upgrade,
		arch,
		platform,
		test_id `

	baseString := queryString + ` AND branch = "` + c.baseRelease + `"`
	query := c.client.Query(baseString + groupString)
	query.Parameters = []bigquery.QueryParameter{
		{
			Name:  "From",
			Value: c.baseStartTime,
		},
		{
			Name:  "To",
			Value: c.baseEndTime,
		},
	}
	baseStatus := c.fetchTestStatus(query, errs)

	sampleString := queryString + ` AND branch = "` + c.sampleRelease + `"`
	query = c.client.Query(sampleString + groupString)
	query.Parameters = []bigquery.QueryParameter{
		{
			Name:  "From",
			Value: c.sampleStartTime,
		},
		{
			Name:  "To",
			Value: c.sampleEndTime,
		},
	}
	sampleStatus := c.fetchTestStatus(query, errs)
	return baseStatus, sampleStatus, errs
}

var comonentAndCapabilityGetter func(string) (string, []string)

func testToComponentAndCapability(name string) (string, []string) {
	component := "other"
	capability := "other"
	r := regexp.MustCompile(`.*(?P<component>\[sig-[A-Za-z]*\]).*(?P<feature>\[Feature:[A-Za-z]*\]).*`)
	subMatches := r.FindStringSubmatch(name)
	if len(subMatches) >= 2 {
		subNames := r.SubexpNames()
		for i, sName := range subNames {
			switch sName {
			case "component":
				component = subMatches[i]
			case "feature":
				capability = subMatches[i]
			}
		}
	}
	return component, []string{capability}
}

// getRowColumnIdentifications defines the rows and columns since they are variable. For rows, different pages have different row titles (component, capability etc)
// Columns titles depends on the groupBy parameter user requests. A particular test can belong to multiple rows of different capabilities.
func (c *componentReportGenerator) getRowColumnIdentifications(test *apitype.ComponentTestIdentification) ([]apitype.ComponentReportRowIdentification, apitype.ComponentReportColumnIdentification) {
	component, capabilities := comonentAndCapabilityGetter(test.TestName)
	rows := []apitype.ComponentReportRowIdentification{}
	// First Page with no component requested
	if c.component == "" {
		rows = append(rows, apitype.ComponentReportRowIdentification{Component: component})
	} else if c.component == component {
		// Exact test match
		if c.testId != "" {
			row := apitype.ComponentReportRowIdentification{
				Component: component,
				TestID:    test.TestID,
				TestName:  test.TestName,
			}
			rows = append(rows, row)
		} else {
			for _, capability := range capabilities {
				// Exact capability match only produces one row
				if c.capability != "" {
					if c.capability == capability {
						row := apitype.ComponentReportRowIdentification{
							Component:  component,
							TestID:     test.TestID,
							TestName:   test.TestName,
							Capability: capability,
						}
						rows = append(rows, row)
						break
					}
				} else {
					rows = append(rows, apitype.ComponentReportRowIdentification{Component: component, Capability: capability})
				}
			}
		}
	}
	column := apitype.ComponentReportColumnIdentification{}
	groups := sets.NewString(strings.Split(c.groupBy, ",")...)
	if groups.Has("cloud") {
		column.Platform = test.Platform
	}
	if groups.Has("network") {
		column.Network = test.Network
	}
	if groups.Has("arch") {
		column.Arch = test.Arch
	}
	if groups.Has("upgrade") {
		column.Upgrade = test.Upgrade
	}
	if groups.Has("variant") {
		column.Variant = test.Variant
	}

	return rows, column
}

func (c *componentReportGenerator) fetchTestStatus(query *bigquery.Query, errs []error) map[apitype.ComponentTestIdentification]apitype.ComponentTestStats {
	status := map[apitype.ComponentTestIdentification]apitype.ComponentTestStats{}
	it, err := query.Read(context.TODO())
	if err != nil {
		log.WithError(err).Error("error querying test status from bigquery")
		errs = append(errs, err)
		return status
	}

	for {
		testStatus := apitype.ComponentTestStatus{}
		err := it.Next(&testStatus)
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.WithError(err).Error("error parsing component from bigquery")
			errs = append(errs, errors.Wrap(err, "error parsing prowjob from bigquery"))
			continue
		}
		testIdentification := apitype.ComponentTestIdentification{
			TestName: testStatus.TestName,
			TestID:   testStatus.TestID,
			Network:  testStatus.Network,
			Upgrade:  testStatus.Upgrade,
			Arch:     testStatus.Arch,
			Platform: testStatus.Platform,
			Variant:  testStatus.Variant,
		}
		status[testIdentification] = apitype.ComponentTestStats{
			TotalCount:   testStatus.TotalCount,
			FlakeCount:   testStatus.FlakeCount,
			SuccessCount: testStatus.SuccessCount,
		}
	}
	return status
}

func updateStatus(rowIdentifications []apitype.ComponentReportRowIdentification,
	columnIdentification apitype.ComponentReportColumnIdentification,
	reportStatus apitype.ComponentReportStatus,
	status map[apitype.ComponentReportRowIdentification]map[apitype.ComponentReportColumnIdentification]apitype.ComponentReportStatus,
	allRows map[apitype.ComponentReportRowIdentification]struct{},
	allColumns map[apitype.ComponentReportColumnIdentification]struct{}) {
	if _, ok := allColumns[columnIdentification]; !ok {
		allColumns[columnIdentification] = struct{}{}
	}
	for _, rowIdentification := range rowIdentifications {
		if _, ok := allRows[rowIdentification]; !ok {
			allRows[rowIdentification] = struct{}{}
		}
		row, ok := status[rowIdentification]
		if !ok {
			row = map[apitype.ComponentReportColumnIdentification]apitype.ComponentReportStatus{}
			row[columnIdentification] = reportStatus
			status[rowIdentification] = row
		} else {
			existing, ok := row[columnIdentification]
			if !ok {
				row[columnIdentification] = reportStatus
			} else if (reportStatus < apitype.NotSignificant && reportStatus < existing) ||
				(existing == apitype.NotSignificant && reportStatus == apitype.SignificantImprovement) {
				// We want to show the significant improvement if assessment is not regression
				row[columnIdentification] = reportStatus
			}
		}
	}
}

func (c *componentReportGenerator) generateComponentTestReport(baseStatus map[apitype.ComponentTestIdentification]apitype.ComponentTestStats,
	sampleStatus map[apitype.ComponentTestIdentification]apitype.ComponentTestStats) apitype.ComponentReport {
	report := apitype.ComponentReport{}
	// aggregatedStatus is the aggregated status based on the requested rows and columns
	aggregatedStatus := map[apitype.ComponentReportRowIdentification]map[apitype.ComponentReportColumnIdentification]apitype.ComponentReportStatus{}
	// allRows and allColumns are used to make sure rows are ordered and all rows have the same columns in the same order
	allRows := map[apitype.ComponentReportRowIdentification]struct{}{}
	allColumns := map[apitype.ComponentReportColumnIdentification]struct{}{}
	for testIdentification, baseStats := range baseStatus {
		reportStatus := apitype.NotSignificant
		fmt.Printf("------ analyzing %+v\n", testIdentification)
		sampleStats, ok := sampleStatus[testIdentification]
		if !ok {
			reportStatus = apitype.MissingSample
		} else {
			reportStatus = c.categorizeComponentStatus(&sampleStats, &baseStats)
		}
		delete(sampleStatus, testIdentification)

		rowIdentifications, columnIdentification := c.getRowColumnIdentifications(&testIdentification)
		updateStatus(rowIdentifications, columnIdentification, reportStatus, aggregatedStatus, allRows, allColumns)
	}
	// Those sample ones are missing base stats
	for testIdentification, _ := range sampleStatus {
		rowIdentifications, columnIdentification := c.getRowColumnIdentifications(&testIdentification)
		updateStatus(rowIdentifications, columnIdentification, apitype.MissingBasis, aggregatedStatus, allRows, allColumns)
	}
	// Sort the row identifications
	sortedRows := []apitype.ComponentReportRowIdentification{}
	for rowID := range allRows {
		sortedRows = append(sortedRows, rowID)
	}
	sort.Slice(sortedRows, func(i, j int) bool {
		return sortedRows[i].Component < sortedRows[j].Component ||
			sortedRows[i].Capability < sortedRows[j].Capability ||
			sortedRows[i].TestName < sortedRows[j].TestName ||
			sortedRows[i].TestID < sortedRows[j].TestID
	})

	// Sort the column identifications
	sortedColumns := []apitype.ComponentReportColumnIdentification{}
	for columnID := range allColumns {
		sortedColumns = append(sortedColumns, columnID)
	}
	sort.Slice(sortedColumns, func(i, j int) bool {
		return sortedColumns[i].Platform < sortedColumns[j].Platform ||
			sortedColumns[i].Arch < sortedColumns[j].Arch ||
			sortedColumns[i].Network < sortedColumns[j].Network ||
			sortedColumns[i].Upgrade < sortedColumns[j].Upgrade ||
			sortedColumns[i].Variant < sortedColumns[j].Variant
	})

	// Now build the report
	for _, rowID := range sortedRows {
		if columns, ok := aggregatedStatus[rowID]; ok {
			if report.Rows == nil {
				report.Rows = []apitype.ComponentReportRow{}
			}
			reportRow := apitype.ComponentReportRow{ComponentReportRowIdentification: rowID}
			for _, columnID := range sortedColumns {
				if reportRow.Columns == nil {
					reportRow.Columns = []apitype.ComponentReportColumn{}
				}
				reportColumn := apitype.ComponentReportColumn{ComponentReportColumnIdentification: columnID}
				status, ok := columns[columnID]
				if !ok {
					reportColumn.Status = apitype.MissingBasisAndSample
				} else {
					reportColumn.Status = status
				}
				reportRow.Columns = append(reportRow.Columns, reportColumn)
			}
			report.Rows = append(report.Rows, reportRow)
		}
	}

	return report
}

func (c *componentReportGenerator) categorizeComponentStatus(sampleStats *apitype.ComponentTestStats, baseStats *apitype.ComponentTestStats) apitype.ComponentReportStatus {
	ret := apitype.MissingBasis
	if baseStats.TotalCount != 0 {
		if sampleStats.TotalCount == 0 {
			if c.ignoreMissing {
				ret = apitype.NotSignificant

			} else {
				ret = apitype.MissingSample
			}
		} else {
			if c.minimumFailure != 0 && (sampleStats.TotalCount-sampleStats.SuccessCount-sampleStats.FlakeCount) < c.minimumFailure {
				return apitype.NotSignificant
			} else {
				basisPassPercentage := float64(baseStats.SuccessCount+baseStats.FlakeCount) / float64(baseStats.TotalCount)
				samplePassPercentage := float64(sampleStats.SuccessCount+sampleStats.FlakeCount) / float64(sampleStats.TotalCount)
				significant := false
				improved := samplePassPercentage >= basisPassPercentage
				if improved {
					_, _, r, _ := fischer.FisherExactTest(baseStats.TotalCount-baseStats.SuccessCount-baseStats.FlakeCount,
						baseStats.SuccessCount+baseStats.FlakeCount,
						sampleStats.TotalCount-sampleStats.SuccessCount-sampleStats.FlakeCount,
						sampleStats.SuccessCount+sampleStats.FlakeCount)
					significant = r < 1-float64(c.confidence)/100
					fmt.Printf("-------- imporved, base p %v, sample p %v, r: %v\n", basisPassPercentage, samplePassPercentage, r)

				} else {
					if basisPassPercentage-samplePassPercentage > float64(c.pityFactor)/100 {
						_, _, r, _ := fischer.FisherExactTest(sampleStats.TotalCount-sampleStats.SuccessCount-sampleStats.FlakeCount,
							sampleStats.SuccessCount+sampleStats.FlakeCount,
							baseStats.TotalCount-baseStats.SuccessCount-baseStats.FlakeCount,
							baseStats.SuccessCount+baseStats.FlakeCount)
						significant = r < 1-float64(c.confidence)/100
						fmt.Printf("-------- regressed, base p %v, sample p %v, r: %v\n", basisPassPercentage, samplePassPercentage, r)
					}
				}
				if significant {
					if improved {
						ret = apitype.SignificantImprovement
					} else {
						if (basisPassPercentage - samplePassPercentage) > 0.15 {
							ret = apitype.ExtremeRegression
						} else {
							ret = apitype.SignificantRegression
						}
					}
				} else {
					ret = apitype.NotSignificant
				}
			}
		}
	}
	return ret
}

func init() {
	comonentAndCapabilityGetter = testToComponentAndCapability
}
