package api

import (
	"net/http"
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

	response := apitype.ProwJobRunFailureAnalysis{
		ProwJobRunID: jobRun.ProwJobID,
		ProwJobName:  jobRun.ProwJob.Name,
		ProwJobURL:   jobRun.URL,
		Timestamp:    jobRun.Timestamp,
	}

	// Get the test failures
	for _, ft := range jobRun.Tests {
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
		tr, _, err := BuildTestsResults(dbc, jobRun.ProwJob.Release, "default", true, false,
			fil)
		if err != nil {
			return apitype.ProwJobRunFailureAnalysis{}, res.Error
		}
		logger.Infof("Got test results: %d", len(tr))
	}

	// Watchout for presubmits, we need this to work there especially, their release is "Presubmits", but we want to query against latest real release.

	// Should we check if majority of tests actually ran?

	return response, nil
}
