package api

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
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
}

func (c *componentReportGenerator) GenerateReport() (apitype.ComponentReport, []error) {
	baseStatus, sampleStatus, errs := c.getTestStatusFromBigQuery()
	report := c.generateComponentTestReport(baseStatus, sampleStatus)
	return report, errs
}

func (c *componentReportGenerator) getTestStatusFromBigQuery() (
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
	baseStatus := c.fetchTestStatus(query, errs)
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
	sampleStatus := c.fetchTestStatus(query, errs)
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
func (c *componentReportGenerator) getRowColumnIdentifications(test *apitype.ComponentTestStatus) ([]apitype.ComponentReportRowIdentification, apitype.ComponentReportColumnIdentification) {
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
				TestID: test.TestID,
				TestName: test.TestName,
			}
			rows = append(rows, row)
		} else {
			for _, capability := range capabilities {
				// Exact capability match only produces one row
				if c.capability != "" {
					if c.capability == capability {
						row := apitype.ComponentReportRowIdentification{
							Component: component,
							TestID: test.TestID,
							TestName: test.TestName,
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

func (c *componentReportGenerator) fetchTestStatus(query *bigquery.Query, errs []error) map[apitype.ComponentReportRowIdentification]map[apitype.ComponentReportColumnIdentification]*apitype.ComponentTestStats {
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

		rowIdentifications, columnIdentification := c.getRowColumnIdentifications(&testStatus)
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

func (c *componentReportGenerator) generateComponentTestReport(baseStatus map[apitype.ComponentReportRowIdentification]map[apitype.ComponentReportColumnIdentification]*apitype.ComponentTestStats,
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
	if sampleStats.TotalCount != 0 && baseStats.TotalCount != 0 &&
		(sampleStats.SuccessCount+sampleStats.FlakeCount)/sampleStats.TotalCount < (baseStats.SuccessCount+baseStats.FlakeCount)/baseStats.TotalCount {
		return apitype.ComponentRed
	}
	return apitype.ComponentGreen
}
