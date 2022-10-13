package api

import (
	"fmt"
	"net/http"
	"reflect"
	gosort "sort"
	"strconv"
	"time"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/filter"
	log "github.com/sirupsen/logrus"
)

func (runs apiRunResults) sort(req *http.Request) apiRunResults {
	sortField := req.URL.Query().Get("sortField")
	sort := apitype.Sort(req.URL.Query().Get("sort"))

	if sortField == "" {
		sortField = "test_failures"
	}

	if sort == "" {
		sort = apitype.SortDescending
	}

	gosort.Slice(runs, func(i, j int) bool {
		if sort == apitype.SortAscending {
			return filter.Compare(runs[i], runs[j], sortField)
		}
		return filter.Compare(runs[j], runs[i], sortField)
	})

	return runs
}

func (runs apiRunResults) limit(req *http.Request) apiRunResults {
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	if limit > 0 && len(runs) >= limit {
		return runs[:limit]
	}

	return runs
}

type apiRunResults []apitype.JobRun

// JobsRunsReportFromDB renders a filtered summary of matching jobs.
func JobsRunsReportFromDB(dbc *db.DB, filterOpts *filter.FilterOptions, release string, pagination *apitype.Pagination, reportEnd time.Time) (*apitype.PaginationResult, error) {
	jobsResult := make([]apitype.JobRun, 0)
	table := "prow_job_runs_report_matview"
	q, err := filter.FilterableDBResult(dbc.DB.Table(table), filterOpts, apitype.JobRun{})
	if err != nil {
		return nil, err
	}

	if len(release) > 0 {
		q = q.Where("release = ?", release)
	}

	q = q.Where("timestamp < ?", reportEnd.UnixMilli())

	// Get the row count before pagination
	var rowCount int64
	q.Count(&rowCount)

	// Paginate the results:
	if pagination == nil {
		pagination = &apitype.Pagination{
			PerPage: int(rowCount),
			Page:    0,
		}
	} else {
		q = q.Limit(pagination.PerPage).Offset(pagination.Page * pagination.PerPage)
	}

	res := q.Scan(&jobsResult)
	return &apitype.PaginationResult{
		Rows:      jobsResult,
		TotalRows: rowCount,
		PageSize:  pagination.PerPage,
		Page:      pagination.Page,
	}, res.Error
}

func JobRunAnalysis(dbc *db.DB, jobRunID int64) (apitype.ProwJobRunFailureAnalysis, error) {

	jobRun := &models.ProwJobRun{}
	// Load the ProwJobRun, ProwJob, and failed tests:
	// TODO: we may want to expand to analyzing flakes here in the future
	res := dbc.DB.Joins("ProwJob").Preload("Tests", "status = 12").Preload("Tests.Test").First(jobRun, jobRunID)
	if res.Error != nil {
		return apitype.ProwJobRunFailureAnalysis{}, res.Error
	}

	logger := log.WithFields(log.Fields{
		"func":     "jobRunAnalysis",
		"jobRunID": jobRun.ID,
	})

	logger.WithField("url", jobRun.URL).Info("loaded prow job run for analysis")
	logger.Infof("this job run has %d failed tests", len(jobRun.Tests))
	logger.WithField("variants", jobRun.ProwJob.Variants).Debug("job variants")

	response := apitype.ProwJobRunFailureAnalysis{
		ProwJobRunID: jobRun.ProwJobID,
		ProwJobName:  jobRun.ProwJob.Name,
		ProwJobURL:   jobRun.URL,
		Timestamp:    jobRun.Timestamp,
		Tests:        []apitype.ProwJobRunTestFailureAnalysis{},
		OverallRisk: apitype.FailureRisk{
			Level:   apitype.FailureRiskLevelNone,
			Reasons: []string{},
		},
	}

	switch {

	// Return early if no tests failed in this run:
	case len(jobRun.Tests) == 0:
		response.OverallRisk.Level = apitype.FailureRiskLevelNone
		response.OverallRisk.Reasons = append(response.OverallRisk.Reasons,
			"No test failures found in this job run.")
		return response, nil

	// Return early if we see mass test failures:
	case len(jobRun.Tests) > 20:
		response.OverallRisk.Level = apitype.FailureRiskLevelHigh
		response.OverallRisk.Reasons = append(response.OverallRisk.Reasons,
			fmt.Sprintf("%d tests failed in this run: High", len(jobRun.Tests)))
		return response, nil
	}

	// Get the test failures
	for _, ft := range jobRun.Tests {
		// TODO: filter test names we don't care about
		logger.WithFields(log.Fields{
			"testID": ft.TestID,
			"name":   ft.Test.Name,
		}).Debug("failed test")
		release := jobRun.ProwJob.Release
		if release == "Presubmits" {
			release = "4.12" // TODO, how do we know what release a presubmit is for?
		}
		fil := &filter.Filter{
			Items: []filter.FilterItem{
				{
					Field:    "name",
					Not:      false,
					Operator: filter.OperatorEquals,
					Value:    ft.Test.Name,
				},
			},
			LinkOperator: "and",
		}
		trs, _, err := BuildTestsResults(dbc, jobRun.ProwJob.Release, "default", false, false,
			fil)
		if err != nil {
			return apitype.ProwJobRunFailureAnalysis{}, res.Error
		}
		logger.Infof("Got test results: %d", len(trs))
		for _, tr := range trs {
			// TODO: this is a weird way to get the variant we want, should we filter in the query?
			if reflect.DeepEqual(tr.Variants, jobRun.ProwJob.Variants) {
				response.Tests = append(response.Tests, apitype.ProwJobRunTestFailureAnalysis{
					Name: tr.Name,
					// TODO suite?
					Risk: apitype.FailureRisk{
						Level: getSeverityLevelForPassRate(tr.CurrentPassPercentage),
						Reasons: []string{
							fmt.Sprintf("This test has passed %.2f%% of %d runs on release %s %v in the last week.",
								tr.CurrentPassPercentage, tr.CurrentRuns, release, tr.Variants),
						},
					},
				})

			}
		}
	}

	// Set the overall risk level for this job run to the highest we encountered:
	var maxTestRiskReason string
	for _, ta := range response.Tests {
		if ta.Risk.Level.Level >= response.OverallRisk.Level.Level {
			response.OverallRisk.Level = ta.Risk.Level
			maxTestRiskReason = fmt.Sprintf("Maximum failed test risk: %s", ta.Risk.Level.Name)
		}
	}
	response.OverallRisk.Reasons = append(response.OverallRisk.Reasons, maxTestRiskReason)

	// Watchout for presubmits, we need this to work there especially, their release is "Presubmits", but we want to query against latest real release.

	// Should we check if majority of tests actually ran?

	return response, nil
}

func getSeverityLevelForPassRate(passPercentage float64) apitype.RiskLevel {
	switch {
	case passPercentage >= 98.0:
		return apitype.FailureRiskLevelHigh
	case passPercentage >= 80:
		return apitype.FailureRiskLevelMedium
	case passPercentage < 80:
		return apitype.FailureRiskLevelLow
	}
	return apitype.FailureRiskLevelUnknown
}
