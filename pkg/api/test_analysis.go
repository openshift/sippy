package api

import (
	"strings"
	"time"

	"cloud.google.com/go/civil"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/filter"
)

const testAnalysisLookbackDays = 14

type CountByDate struct {
	Date            civil.Date `json:"date"`
	Group           string     `json:"group"`
	PassPercentage  float64    `json:"pass_percentage"`
	FlakePercentage float64    `json:"flake_percentage"`
	FailPercentage  float64    `json:"fail_percentage"`
	Runs            int        `json:"runs"`
	Passes          int        `json:"passes"`
	Flakes          int        `json:"flakes"`
	Failures        int        `json:"failures"`
}

func extractVariantFilters(filters *filter.Filter) (allowed, blocked []string) {
	if filters == nil {
		return nil, nil
	}
	for _, f := range filters.Items {
		if f.Field == "variants" {
			if f.Not {
				blocked = append(blocked, f.Value)
			} else {
				allowed = append(allowed, f.Value)
			}
		}
	}
	return allowed, blocked
}

func applyVariantCombinationFilters(q *gorm.DB, allowed, blocked []string) *gorm.DB {
	for _, bv := range blocked {
		q = q.Where("NOT EXISTS (SELECT 1 FROM variant_combinations WHERE ? = ANY(variants) AND id = pj.variant_combination_id)", bv)
	}
	for _, av := range allowed {
		q = q.Where("EXISTS (SELECT 1 FROM variant_combinations WHERE ? = ANY(variants) AND id = pj.variant_combination_id)", av)
	}
	return q
}

var testAnalysisAggColumns = []string{
	"SUM(tds.runs) AS runs",
	"SUM(tds.successes) AS passes",
	"SUM(tds.flakes) AS flakes",
	"SUM(tds.failures) AS failures",
	"SUM(tds.successes) * 100.0 / NULLIF(SUM(tds.runs), 0) AS pass_percentage",
	"SUM(tds.flakes) * 100.0 / NULLIF(SUM(tds.runs), 0) AS flake_percentage",
	"SUM(tds.failures) * 100.0 / NULLIF(SUM(tds.runs), 0) AS fail_percentage",
}

// withAggColumns appends the shared test analysis aggregation columns
// to the supplied per-query columns and joins them into a SELECT string.
func withAggColumns(columns ...string) string {
	return strings.Join(append(columns, testAnalysisAggColumns...), ", ")
}

func testAnalysisBaseQuery(dbc *db.DB, filters *filter.Filter, release, testName string, since civil.Date) *gorm.DB {
	q := dbc.DB.Table("test_daily_totals tds").
		Joins("JOIN tests t ON t.id = tds.test_id").
		Joins("JOIN prow_jobs pj ON pj.id = tds.prow_job_id").
		Where("tds.release = ?", release).
		Where("t.name = ?", testName).
		Where("tds.date >= ?", since).
		Order("tds.date ASC")

	allowed, blocked := extractVariantFilters(filters)
	return applyVariantCombinationFilters(q, allowed, blocked)
}

func GetTestAnalysisOverallFromDB(dbc *db.DB, filters *filter.Filter, release, testName string, reportEnd time.Time) (map[string][]CountByDate, error) {
	endDate := civil.DateOf(reportEnd.UTC())
	sinceDate := endDate.AddDays(-testAnalysisLookbackDays)

	var rows []CountByDate
	jq := testAnalysisBaseQuery(dbc, filters, release, testName, sinceDate).
		Select(withAggColumns(
			"tds.test_id",
			`t.name AS test_name`,
			`tds.date AS date`,
			`'overall' AS "group"`,
		)).
		Where("tds.date <= ?", endDate).
		Group("tds.date, tds.test_id, t.name, tds.release")

	r := jq.Scan(&rows)
	if r.Error != nil {
		log.WithError(r.Error).Error("error querying test analysis overall")
		return nil, r.Error
	}

	result := make(map[string][]CountByDate)
	result["overall"] = rows
	return result, nil
}

func GetTestAnalysisByJobFromDB(dbc *db.DB, filters *filter.Filter, release, testName string, reportEnd time.Time) (map[string][]CountByDate, error) {
	var rows []CountByDate
	results := make(map[string][]CountByDate)

	overallResult, err := GetTestAnalysisOverallFromDB(dbc, filters, release, testName, reportEnd)
	if err != nil {
		return nil, err
	}
	if overall, ok := overallResult["overall"]; ok {
		results["overall"] = overall
	}

	endDate := civil.DateOf(reportEnd.UTC())
	sinceDate := endDate.AddDays(-testAnalysisLookbackDays)

	jq := testAnalysisBaseQuery(dbc, filters, release, testName, sinceDate).
		Select(withAggColumns(
			"tds.test_id",
			`t.name AS test_name`,
			`tds.date AS date`,
			"pj.release",
			`pj.name AS "group"`,
			"pj.variants",
		)).
		Where("tds.date <= ?", endDate).
		Group("tds.date, tds.test_id, t.name, pj.release, pj.name, pj.variants")

	r := jq.Scan(&rows)
	if r.Error != nil {
		log.WithError(r.Error).Error("error querying test analysis by job")
		return nil, r.Error
	}

	for _, row := range rows {
		results[row.Group] = append(results[row.Group], row)
	}

	return results, nil
}

func GetTestAnalysisByVariantFromDB(dbc *db.DB, filters *filter.Filter, release, testName string, reportEnd time.Time) (map[string][]CountByDate, error) {
	var rows []CountByDate
	results := make(map[string][]CountByDate)

	overallResult, err := GetTestAnalysisOverallFromDB(dbc, filters, release, testName, reportEnd)
	if err != nil {
		return nil, err
	}
	if overall, ok := overallResult["overall"]; ok {
		results["overall"] = overall
	}

	endDate := civil.DateOf(reportEnd.UTC())
	sinceDate := endDate.AddDays(-testAnalysisLookbackDays)

	inner := dbc.DB.Table("test_daily_totals tds").
		Select(withAggColumns(
			"tds.test_id",
			`t.name AS test_name`,
			`tds.date AS date`,
			`unnest(vc.variants) AS "group"`,
			"tds.release",
		)).
		Joins("JOIN tests t ON t.id = tds.test_id").
		Joins("JOIN prow_jobs pj ON pj.id = tds.prow_job_id").
		Joins("JOIN variant_combinations vc ON vc.id = pj.variant_combination_id").
		Where("tds.release = ?", release).
		Where("t.name = ?", testName).
		Where("tds.date >= ?", sinceDate).
		Where("tds.date <= ?", endDate).
		Group("t.name, t.id, tds.test_id, tds.date, unnest(vc.variants), tds.release").
		Order("tds.date ASC")

	vq := dbc.DB.Table("(?) AS analysis", inner).Select("*")
	allowed, blocked := extractVariantFilters(filters)
	if len(blocked) > 0 {
		vq = vq.Where(`"group" NOT IN ?`, blocked)
	}
	if len(allowed) > 0 {
		vq = vq.Where(`"group" IN ?`, allowed)
	}

	r := vq.Scan(&rows)
	if r.Error != nil {
		log.WithError(r.Error).Error("error querying test analysis by variant")
		return nil, r.Error
	}

	for _, row := range rows {
		results[row.Group] = append(results[row.Group], row)
	}

	return results, nil
}
