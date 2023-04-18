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

func GetComponentReportFromBigQuery(client *bigquery.Client,
	baseRelease, sampleRelease, component, capability, platform, upgrade, arch, network, testId, groupBy string,
	baseStartTime, baseEndTime, sampleStartTime, sampleEndTime time.Time) (apitype.ComponentReport, []error) {
	generator := componentReportGenerator{
		client:          client,
		baseRelease:     baseRelease,
		sampleRelease:   sampleRelease,
		component:       component,
		capability:      capability,
		platform:        platform,
		upgrade:         upgrade,
		arch:            arch,
		network:         network,
		testId:          testId,
		groupBy:         groupBy,
		baseStartTime:   baseStartTime,
		baseEndTime:     baseEndTime,
		sampleStartTime: sampleStartTime,
		sampleEndTime:   sampleEndTime,
		confidence:      95,
		minimumFailure:  3,
		pityFactor:      5,
	}
	return generator.GenerateReport()
}

type componentReportGenerator struct {
	client          *bigquery.Client
	baseRelease     string
	sampleRelease   string
	component       string
	capability      string
	platform        string
	upgrade         string
	arch            string
	network         string
	testId          string
	groupBy         string
	baseStartTime   time.Time
	baseEndTime     time.Time
	sampleStartTime time.Time
	sampleEndTime   time.Time
	ignoreMissing   bool
	minimumFailure  int
	confidence      int
	pityFactor      int
}

func (c *componentReportGenerator) GenerateReport() (apitype.ComponentReport, []error) {
	baseStatus, sampleStatus, errs := c.getTestStatusFromBigQuery()
	report := c.generateComponentTestReport(baseStatus, sampleStatus)
	return report, errs
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
	now := time.Now()
	baseStatus := c.fetchTestStatus(query, errs)
	delta := time.Now().Sub(now)
	fmt.Printf("---- query took %+v\n", delta)
	now = time.Now()

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
	fmt.Printf("---- sample query took %+v\n", delta)
	return baseStatus, sampleStatus, errs
}

func (c *componentReportGenerator) getTestStatusFromBigQueryGrouped() (
	map[apitype.ComponentReportRowIdentification]map[apitype.ComponentReportColumnIdentification]*apitype.ComponentTestStats,
	map[apitype.ComponentReportRowIdentification]map[apitype.ComponentReportColumnIdentification]*apitype.ComponentTestStats,
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
	now := time.Now()
	baseStatus := c.fetchTestStatusGrouped(query, errs)
	delta := time.Now().Sub(now)
	fmt.Printf("---- query took %+v\n", delta)

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
	sampleStatus := c.fetchTestStatusGrouped(query, errs)
	return baseStatus, sampleStatus, errs
}

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
	component, capabilities := testToComponentAndCapability(test.TestName)
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
	if groups.Has("platform") {
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

// getRowColumnIdentificationsGrouped defines the rows and columns since they are variable. For rows, different pages have different row titles (component, capability etc)
// Columns titles depends on the groupBy parameter user requests. A particular test can belong to multiple rows of different capabilities.
func (c *componentReportGenerator) getRowColumnIdentificationsGrouped(test *apitype.ComponentTestStatus) ([]apitype.ComponentReportRowIdentification, apitype.ComponentReportColumnIdentification) {
	component, capabilities := testToComponentAndCapability(test.TestName)
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
	if groups.Has("platform") {
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

func (c *componentReportGenerator) fetchTestStatusGrouped(query *bigquery.Query, errs []error) map[apitype.ComponentReportRowIdentification]map[apitype.ComponentReportColumnIdentification]*apitype.ComponentTestStats {
	status := map[apitype.ComponentReportRowIdentification]map[apitype.ComponentReportColumnIdentification]*apitype.ComponentTestStats{}
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

		rowIdentifications, columnIdentification := c.getRowColumnIdentificationsGrouped(&testStatus)
		for _, rowIdentification := range rowIdentifications {
			row, ok := status[rowIdentification]
			if !ok {
				row = map[apitype.ComponentReportColumnIdentification]*apitype.ComponentTestStats{}
				test := &apitype.ComponentTestStats{}
				test.FlakeCount = testStatus.FlakeCount
				test.TotalCount = testStatus.TotalCount
				test.SuccessCount = testStatus.SuccessCount
				row[columnIdentification] = test
				status[rowIdentification] = row
			} else {
				test, ok := row[columnIdentification]
				if !ok {
					test = &apitype.ComponentTestStats{}
					test.FlakeCount = testStatus.FlakeCount
					test.TotalCount = testStatus.TotalCount
					test.SuccessCount = testStatus.SuccessCount
					row[columnIdentification] = test
				} else {
					test.FlakeCount += testStatus.FlakeCount
					test.TotalCount += testStatus.TotalCount
					test.SuccessCount += testStatus.SuccessCount
				}
			}
		}
	}
	return status
}

func updateStatus(rowIdentifications []apitype.ComponentReportRowIdentification,
	columnIdentification apitype.ComponentReportColumnIdentification,
	reportStatus apitype.ComponentReportStatus,
	status map[apitype.ComponentReportRowIdentification]map[apitype.ComponentReportColumnIdentification]apitype.ComponentReportStatus) {
	for _, rowIdentification := range rowIdentifications {
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
	allStatus := map[apitype.ComponentReportRowIdentification]map[apitype.ComponentReportColumnIdentification]apitype.ComponentReportStatus{}
	// allColumns is used to make sure all rows have the same columns in the same order
	allColumns := map[apitype.ComponentReportColumnIdentification]struct{}{}
	now := time.Now()
	for testIdentification, baseStats := range baseStatus {
		reportStatus := apitype.NotSignificant
		sampleStats, ok := sampleStatus[testIdentification]
		if !ok {
			reportStatus = apitype.MissingSample
		} else {
			reportStatus = c.categorizeComponentStatus(&sampleStats, &baseStats)
		}
		delete(sampleStatus, testIdentification)

		rowIdentifications, columnIdentification := c.getRowColumnIdentifications(&testIdentification)
		if _, ok := allColumns[columnIdentification]; !ok {
			allColumns[columnIdentification] = struct{}{}
		}
		updateStatus(rowIdentifications, columnIdentification, reportStatus, allStatus)
	}
	delta := time.Now().Sub(now)
	fmt.Printf("--------- Calculating fischer for %d items for %+v\n", len(baseStatus), delta)
	// Those sample ones are missing base stats
	for testIdentification, _ := range sampleStatus {
		rowIdentifications, columnIdentification := c.getRowColumnIdentifications(&testIdentification)
		if _, ok := allColumns[columnIdentification]; !ok {
			allColumns[columnIdentification] = struct{}{}
		}
		updateStatus(rowIdentifications, columnIdentification, apitype.MissingBasis, allStatus)
	}
	// Sort the column identifications
	sortedColumns := []apitype.ComponentReportColumnIdentification{}
	for columnID := range allColumns {
		sortedColumns = append(sortedColumns, columnID)
	}
	sort.Slice(sortedColumns, func(i, j int) bool {
		return sortedColumns[i].Platform < sortedColumns[i].Platform ||
			sortedColumns[i].Arch < sortedColumns[j].Arch ||
			sortedColumns[i].Network < sortedColumns[i].Network ||
			sortedColumns[i].Upgrade < sortedColumns[i].Upgrade ||
			sortedColumns[i].Variant < sortedColumns[i].Variant
	})
	fmt.Printf("--------- sorting columns for %+v\n", time.Now().Sub(now))

	// Now build the report
	for rowID, columns := range allStatus {
		if report.Rows == nil {
			report.Rows = []apitype.ComponentReportRow{}
		}
		reportRow := apitype.ComponentReportRow{ComponentReportRowIdentification: rowID}
		for _, columnID := range sortedColumns {
			if reportRow.Columns == nil {
				reportRow.Columns = []apitype.ComponentReportColumn{}
			}
			reportColumn := apitype.ComponentReportColumn{ComponentReportColumnIdentification: columnID, Status: apitype.NotSignificant}
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
	fmt.Printf("--------- building report for %+v\n", time.Now().Sub(now))

	return report
}

func (c *componentReportGenerator) generateComponentTestReportGrouped(baseStatus map[apitype.ComponentReportRowIdentification]map[apitype.ComponentReportColumnIdentification]*apitype.ComponentTestStats,
	sampleStatus map[apitype.ComponentReportRowIdentification]map[apitype.ComponentReportColumnIdentification]*apitype.ComponentTestStats) apitype.ComponentReport {
	report := apitype.ComponentReport{}
	for rowID, column := range sampleStatus {
		if report.Rows == nil {
			report.Rows = []apitype.ComponentReportRow{}
		}
		reportRow := apitype.ComponentReportRow{ComponentReportRowIdentification: rowID}
		for columnID, stats := range column {
			if reportRow.Columns == nil {
				reportRow.Columns = []apitype.ComponentReportColumn{}
			}
			reportColumn := apitype.ComponentReportColumn{ComponentReportColumnIdentification: columnID, Status: apitype.ComponentGreen}
			if baseRow, ok := baseStatus[rowID]; ok {
				if baseStats, ok := baseRow[columnID]; ok {
					reportColumn.Status = c.categorizeComponentStatus(stats, baseStats)
				}
			}
			reportRow.Columns = append(reportRow.Columns, reportColumn)
		}
		report.Rows = append(report.Rows, reportRow)
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
				basisPassPercentage := baseStats.SuccessCount / baseStats.TotalCount
				samplePassPercentage := sampleStats.SuccessCount / sampleStats.TotalCount
				significant := false
				improved := samplePassPercentage >= basisPassPercentage
				if improved {
					_, _, r, _ := fischer.FisherExactTest(baseStats.TotalCount-baseStats.SuccessCount-baseStats.FlakeCount,
						baseStats.SuccessCount+baseStats.FlakeCount,
						sampleStats.TotalCount-sampleStats.SuccessCount-sampleStats.FlakeCount,
						sampleStats.SuccessCount+sampleStats.FlakeCount)
					significant = r < float64(1-c.confidence/100)

				} else {
					if basisPassPercentage-samplePassPercentage > c.pityFactor {
						_, _, r, _ := fischer.FisherExactTest(sampleStats.TotalCount-sampleStats.SuccessCount-sampleStats.FlakeCount,
							sampleStats.SuccessCount+sampleStats.FlakeCount,
							baseStats.TotalCount-baseStats.SuccessCount-baseStats.FlakeCount,
							baseStats.SuccessCount+baseStats.FlakeCount)
						significant = r < float64(1-c.confidence/100)
					}
				}
				if significant {
					if improved {
						ret = apitype.SignificantImprovement
					} else {
						if (basisPassPercentage - samplePassPercentage) > 15 {
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
