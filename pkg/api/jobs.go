package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	gosort "sort"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/db/query"
	"github.com/openshift/sippy/pkg/filter"
	"github.com/openshift/sippy/pkg/util/param"

	v1sippyprocessing "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
)

type jobsAPIResult []apitype.Job

const periodTwoDay = "twoDay"
const currentPassPercentage = "current_pass_percentage"

func (jobs jobsAPIResult) sort(req *http.Request) jobsAPIResult {
	sortField := param.SafeRead(req, "sortField")
	sort := apitype.Sort(param.SafeRead(req, "sort"))

	if sortField == "" {
		sortField = currentPassPercentage
	}

	if sort == "" {
		sort = apitype.SortAscending
	}

	gosort.Slice(jobs, func(i, j int) bool {
		if sort == apitype.SortAscending {
			return filter.Compare(jobs[i], jobs[j], sortField)
		}
		return filter.Compare(jobs[j], jobs[i], sortField)
	})

	return jobs
}

func (jobs jobsAPIResult) limit(req *http.Request) jobsAPIResult {
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	if limit > 0 && len(jobs) >= limit {
		return jobs[:limit]
	}

	return jobs
}

// PrintVariantReportFromDB
func PrintVariantReportFromDB(w http.ResponseWriter, req *http.Request,
	dbc *db.DB, release string, reportEnd time.Time) {
	// Preferred method of slicing is with start->boundary->end query params in the format ?start=2021-12-02&boundary=2021-12-07.
	// 'end' can be specified if you wish to view historical reports rather than now, which is assumed if end param is absent.
	var start time.Time
	var boundary time.Time
	var end time.Time
	var err error

	startParam := req.URL.Query().Get("start")
	switch {
	case startParam != "":
		start, err = time.Parse("2006-01-02", startParam)
		if err != nil {
			RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{"code": http.StatusBadRequest, "message": fmt.Sprintf("Error decoding start param: %s", err.Error())})
			return
		}
	case req.URL.Query().Get("period") == periodTwoDay:
		// twoDay report period starts 9 days ago, (comparing last 2 days vs previous 7)
		start = reportEnd.Add(-9 * 24 * time.Hour)
	default:
		// Default start to 14 days ago
		start = reportEnd.Add(-14 * 24 * time.Hour)
	}

	// TODO: currently we're assuming dates use the 00:00:00, is it more logical to add 23:23 for boundary and end? or
	// for callers to know to specify one day beyond.
	boundaryParam := req.URL.Query().Get("boundary")
	switch {
	case boundaryParam != "":
		boundary, err = time.Parse("2006-01-02", boundaryParam)
		if err != nil {
			RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{"code": http.StatusBadRequest, "message": fmt.Sprintf("Error decoding boundary param: %s", err.Error())})
			return
		}
	case req.URL.Query().Get("period") == periodTwoDay:
		boundary = reportEnd.Add(-2 * 24 * time.Hour)
	default:
		// Default boundary to 7 days ago
		boundary = reportEnd.Add(-7 * 24 * time.Hour)
	}

	endParam := req.URL.Query().Get("end")
	if endParam != "" {
		end, err = time.Parse("2006-01-02", endParam)
		if err != nil {
			RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{"code": http.StatusBadRequest, "message": fmt.Sprintf("Error decoding end param: %s", err.Error())})
			return
		}
	} else {
		// Default end to now
		end = reportEnd
	}

	log.Debugf("Querying between %s -> %s -> %s", start.Format(time.RFC3339), boundary.Format(time.RFC3339), end.Format(time.RFC3339))

	variantsResult, err := query.VariantReports(dbc, release, start, boundary, end)
	if err != nil {
		RespondWithJSON(http.StatusInternalServerError, w, map[string]interface{}{"code": http.StatusInternalServerError, "message": "Error building variant report:" + err.Error()})
		return
	}

	RespondWithJSON(http.StatusOK, w, variantsResult)
}

// PrintJobsReportFromDB renders a filtered summary of matching jobs.
func PrintJobsReportFromDB(w http.ResponseWriter, req *http.Request,
	dbc *db.DB, release string, reportEnd time.Time) {

	var fil *filter.Filter

	queryFilter := req.URL.Query().Get("filter")
	if queryFilter != "" {
		fil = &filter.Filter{}
		if err := json.Unmarshal([]byte(queryFilter), fil); err != nil {
			RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{"code": http.StatusBadRequest, "message": "Could not marshal query:" + err.Error()})
			return
		}
	}

	// Preferred method of slicing is with start->boundary->end query params in the format ?start=2021-12-02&boundary=2021-12-07.
	// 'end' can be specified if you wish to view historical reports rather than now, which is assumed if end param is absent.
	var start time.Time
	var boundary time.Time
	var end time.Time
	var err error

	startParam := req.URL.Query().Get("start")
	if startParam != "" {
		start, err = time.Parse("2006-01-02", startParam)
		if err != nil {
			RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{"code": http.StatusBadRequest, "message": fmt.Sprintf("Error decoding start param: %s", err.Error())})
			return
		}
	}

	// TODO: currently we're assuming dates use the 00:00:00, is it more logical to add 23:23 for boundary and end? or
	// for callers to know to specify one day beyond.
	boundaryParam := req.URL.Query().Get("boundary")
	if boundaryParam != "" {
		boundary, err = time.Parse("2006-01-02", boundaryParam)
		if err != nil {
			RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{"code": http.StatusBadRequest, "message": fmt.Sprintf("Error decoding boundary param: %s", err.Error())})
			return
		}
	}

	endParam := req.URL.Query().Get("end")
	if endParam != "" {
		end, err = time.Parse("2006-01-02", endParam)
		if err != nil {
			RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{"code": http.StatusBadRequest, "message": fmt.Sprintf("Error decoding end param: %s", err.Error())})
			return
		}
	}

	log.Debugf("Querying between %s -> %s -> %s", start.Format(time.RFC3339), boundary.Format(time.RFC3339), end.Format(time.RFC3339))

	filterOpts, err := filter.FilterOptionsFromRequest(req, currentPassPercentage, apitype.SortDescending)
	if err != nil {
		RespondWithJSON(http.StatusInternalServerError, w, map[string]interface{}{"code": http.StatusInternalServerError, "message": "Error building job report:" + err.Error()})
		return
	}

	jobsResult, err := JobReportsFromDB(dbc, release, req.URL.Query().Get("period"), filterOpts, start, boundary, end, reportEnd)
	if err != nil {
		RespondWithJSON(http.StatusInternalServerError, w, map[string]interface{}{"code": http.StatusInternalServerError, "message": "Error building job report:" + err.Error()})
		return
	}

	RespondWithJSON(http.StatusOK, w, jobsResult)
}

func JobReportsFromDB(dbc *db.DB, release, period string, filterOpts *filter.FilterOptions, start, boundary, end, reportEnd time.Time) ([]apitype.Job, error) {

	// set a default filter if none provided
	if filterOpts == nil {
		filterOpts = &filter.FilterOptions{}
		filterOpts.Filter = &filter.Filter{}
	}

	// could refactor to helper methods
	if period == periodTwoDay {
		// twoDay report period starts 9 days ago, (comparing last 2 days vs previous 7)
		if start.IsZero() {
			start = reportEnd.Add(-9 * 24 * time.Hour)
		}
		if boundary.IsZero() {
			boundary = reportEnd.Add(-2 * 24 * time.Hour)
		}
	} else {
		if start.IsZero() {
			start = reportEnd.Add(-14 * 24 * time.Hour)
		}
		if boundary.IsZero() {
			// Default boundary to 7 days ago
			boundary = reportEnd.Add(-7 * 24 * time.Hour)
		}

	}

	if end.IsZero() {
		end = reportEnd
	}

	jobsResult, err := query.JobReports(dbc, filterOpts, release, start, boundary, end)

	if err != nil {
		return nil, err
	}

	return jobsResult, nil
}

type jobDetail struct {
	Name    string                           `json:"name"`
	Results []v1sippyprocessing.JobRunResult `json:"results"`
}

type jobDetailAPIResult struct {
	Jobs  []jobDetail `json:"jobs"`
	Start int         `json:"start"`
	End   int         `json:"end"`
}

func (jobs jobDetailAPIResult) limit(req *http.Request) jobDetailAPIResult {
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	ret := jobs
	if limit > 0 && len(jobs.Jobs) >= limit {
		ret.Jobs = jobs.Jobs[:limit]
	}

	return ret
}

// PrintJobDetailsReportFromDB renders the detailed list of runs for matching jobs.
func PrintJobDetailsReportFromDB(w http.ResponseWriter, req *http.Request, dbc *db.DB, release, jobSearchStr string, reportEnd time.Time) error {
	var start, end int

	// List all ProwJobRuns for the given release in the last two weeks.
	// TODO: 14 days matches orig API behavior, may want to add query params in future to control.
	since := reportEnd.Add(-14 * 24 * time.Hour)

	prowJobRuns := make([]*models.ProwJobRun, 0)
	res := dbc.DB.Joins("ProwJob").
		Where("name LIKE ?", "%"+jobSearchStr+"%").
		Where("timestamp > ?", since).
		Where("release = ?", release).
		Preload("Tests", "status = ?", 12). // Only pre-load test results with failure status.
		Preload("Tests.Test").
		Find(&prowJobRuns)
	if res.Error != nil {
		log.Errorf("error querying %s ProwJobRuns from db: %v", jobSearchStr, res.Error)
		return res.Error
	}
	log.WithFields(log.Fields{"prowJobRuns": len(prowJobRuns), "since": since}).Info("loaded ProwJobRuns from db")

	jobDetails := map[string]*jobDetail{}
	for _, pjr := range prowJobRuns {
		jobName := pjr.ProwJob.Name
		if _, ok := jobDetails[jobName]; !ok {
			jobDetails[jobName] = &jobDetail{Name: jobName, Results: []v1sippyprocessing.JobRunResult{}}
		}

		// Build string array of failed test names for compat with the existing API response:
		failedTestNames := make([]string, 0, len(pjr.Tests))
		for _, t := range pjr.Tests {
			failedTestNames = append(failedTestNames, t.Test.Name)
		}

		newRun := v1sippyprocessing.JobRunResult{
			ProwID:                pjr.ID,
			Job:                   jobName,
			URL:                   pjr.URL,
			TestFailures:          pjr.TestFailures,
			FailedTestNames:       failedTestNames,
			Failed:                pjr.Failed,
			InfrastructureFailure: pjr.InfrastructureFailure,
			KnownFailure:          pjr.KnownFailure,
			Succeeded:             pjr.Succeeded,
			Timestamp:             int(pjr.Timestamp.Unix() * 1000),
			OverallResult:         pjr.OverallResult,
		}
		jobDetails[jobName].Results = append(jobDetails[jobName].Results, newRun)
	}

	// Convert our map to a list for return:
	jobs := make([]jobDetail, 0, len(jobDetails))
	for _, jobDetail := range jobDetails {
		jobs = append(jobs, *jobDetail)
	}

	RespondWithJSON(http.StatusOK, w, jobDetailAPIResult{
		Jobs:  jobs,
		Start: start,
		End:   end,
	}.limit(req))
	return nil
}
