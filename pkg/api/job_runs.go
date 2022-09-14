package api

import (
	"encoding/json"
	"net/http"
	gosort "sort"
	"strconv"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/filter"
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
func JobsRunsReportFromDB(dbc *db.DB, filterOpts *filter.FilterOptions, release string, pagination *apitype.Pagination) ([]apitype.JobRun, error) {
	jobsResult := make([]apitype.JobRun, 0)
	q, err := filter.FilterableDBResult(dbc.DB, filterOpts, apitype.JobRun{})
	if err != nil {
		return nil, err
	}
	q = q.Table("prow_job_runs_report_matview").Where("release = ?", release)
	if pagination != nil {
		q = q.Limit(pagination.PerPage).Offset(pagination.Offset)
	}

	res := q.Scan(&jobsResult)

	return jobsResult, res.Error
}

// PrintJobsRunsReportFromDB renders a filtered summary of matching jobs.
func PrintJobsRunsReportFromDB(w http.ResponseWriter, req *http.Request, dbc *db.DB) {
	var fil *filter.Filter

	queryFilter := req.URL.Query().Get("filter")
	if queryFilter != "" {
		fil = &filter.Filter{}
		if err := json.Unmarshal([]byte(queryFilter), fil); err != nil {
			RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{"code": http.StatusBadRequest, "message": "Could not marshal query:" + err.Error()})
			return
		}
	}

	filterOpts, err := filter.FilterOptionsFromRequest(req, "timestamp", "desc")
	if err != nil {
		RespondWithJSON(http.StatusInternalServerError, w, map[string]interface{}{"code": http.StatusInternalServerError, "message": "Error building job run report:" + err.Error()})
		return
	}

	rf := releaseFilter(req, dbc.DB)
	q, err := filter.FilterableDBResult(dbc.DB, filterOpts, apitype.JobRun{})
	if err != nil {
		RespondWithJSON(http.StatusInternalServerError, w, map[string]interface{}{"code": http.StatusInternalServerError, "message": "Error building job run report:" + err.Error()})
		return
	}

	q = q.Where(rf)

	jobsResult := make([]apitype.JobRun, 0)
	q.Table("prow_job_runs_report_matview").Scan(&jobsResult)
	if err != nil {
		RespondWithJSON(http.StatusInternalServerError, w, map[string]interface{}{"code": http.StatusInternalServerError, "message": "Error building job report:" + err.Error()})
		return
	}

	RespondWithJSON(http.StatusOK, w, jobsResult)
}
